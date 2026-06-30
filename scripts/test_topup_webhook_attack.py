#!/usr/bin/env python3
"""
Verify the Stripe/Creem webhook forgery defenses added in v249.

Runs only against loopback / private addresses by default. Pass --force
if you really mean to point it at a non-local host.

Fixtures you must create before running (via SQL against the TEST DB):

  -- a Stripe-gateway pending order (used by tests 3, 5)
  INSERT INTO topups (user_id, amount, money, trade_no, payment_method,
                      create_time, status)
  VALUES (<UID>, 10, 10.0, 'TEST_STRIPE_PENDING', 'stripe',
          UNIX_TIMESTAMP(), 'pending');

  -- an Epay-gateway pending order (used by tests 2, 4)
  INSERT INTO topups (user_id, amount, money, trade_no, payment_method,
                      create_time, status)
  VALUES (<UID>, 10, 10.0, 'TEST_EPAY_PENDING', 'wxpay',
          UNIX_TIMESTAMP(), 'pending');

After each test, check the user quota and the topups.status:
  SELECT id, quota FROM users WHERE id = <UID>;
  SELECT trade_no, status FROM topups WHERE trade_no IN
    ('TEST_STRIPE_PENDING', 'TEST_EPAY_PENDING');

Only test 5 (happy path) should flip an order to 'success' and bump
quota. Tests 2/3/4 must leave both untouched.
"""

import argparse
import hashlib
import hmac
import ipaddress
import json
import socket
import sys
import time
import urllib.parse
import urllib.request


def is_local(url: str) -> bool:
    host = urllib.parse.urlparse(url).hostname or ""
    if host in ("localhost", "127.0.0.1", "::1"):
        return True
    try:
        ip = ipaddress.ip_address(socket.gethostbyname(host))
    except (socket.gaierror, ValueError):
        return False
    return ip.is_loopback or ip.is_private


def build_event(trade_no: str, payment_status: str = "paid",
                event_type: str = "checkout.session.completed",
                session_status: str = "complete") -> bytes:
    event = {
        "id": f"evt_test_{int(time.time())}",
        "type": event_type,
        "data": {
            "object": {
                "id": f"cs_test_{int(time.time())}",
                "object": "checkout.session",
                "client_reference_id": trade_no,
                "customer": "cus_test_attacker",
                "status": session_status,
                "payment_status": payment_status,
                "amount_total": 1000,
                "currency": "usd",
            },
        },
    }
    return json.dumps(event, separators=(",", ":")).encode()


def sign(payload: bytes, secret: str) -> str:
    ts = str(int(time.time()))
    mac = hmac.new(secret.encode(), f"{ts}.".encode() + payload,
                   hashlib.sha256).hexdigest()
    return f"t={ts},v1={mac}"


def post(url: str, payload: bytes, signature: str | None) -> tuple[int, str]:
    headers = {"Content-Type": "application/json"}
    if signature is not None:
        headers["Stripe-Signature"] = signature
    req = urllib.request.Request(url, data=payload, headers=headers,
                                 method="POST")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status, resp.read(500).decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read(500).decode("utf-8", "replace")


def run_test(name: str, expected_status: int | set[int],
             url: str, payload: bytes, signature: str | None):
    code, body = post(url, payload, signature)
    ok = code in (expected_status if isinstance(expected_status, set)
                  else {expected_status})
    mark = "PASS" if ok else "FAIL"
    print(f"  [{mark}] {name}: got HTTP {code} (expected {expected_status})")
    if body.strip():
        print(f"         body: {body.strip()[:200]}")
    return ok


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--url", default="http://localhost:3000",
                   help="base URL of the test instance")
    p.add_argument("--stripe-secret", default="",
                   help="value of StripeWebhookSecret on the test server "
                        "(empty means test 1 scenario)")
    p.add_argument("--stripe-trade-no", default="TEST_STRIPE_PENDING")
    p.add_argument("--epay-trade-no", default="TEST_EPAY_PENDING")
    p.add_argument("--force", action="store_true",
                   help="allow running against non-local hosts")
    p.add_argument("--skip", nargs="*", default=[],
                   help="test numbers to skip, e.g. --skip 5")
    args = p.parse_args()

    if not is_local(args.url) and not args.force:
        print(f"refusing to target non-local host {args.url}; pass --force "
              "only if you are certain this is a test instance.",
              file=sys.stderr)
        return 2

    stripe_url = args.url.rstrip("/") + "/api/user/stripe/webhook"
    creem_url = args.url.rstrip("/") + "/api/user/creem/webhook"
    results: list[bool] = []

    print(f"target: {args.url}")
    print(f"stripe secret length: {len(args.stripe_secret)} chars")
    print()

    # ---- Test 1: empty-secret bypass -----------------------------------
    if "1" not in args.skip:
        print("Test 1: forge event with HMAC computed from EMPTY secret")
        payload = build_event(args.stripe_trade_no)
        forged_sig = sign(payload, "")
        # v249 short-circuits with 403 if server's secret is empty.
        # If server has a real secret configured, it returns 400 (sig mismatch).
        results.append(run_test("empty-secret forge", {400, 403},
                                stripe_url, payload, forged_sig))
        print()

    # ---- Test 2: cross-gateway (Stripe webhook -> Epay order) ----------
    if "2" not in args.skip:
        print("Test 2: valid Stripe signature but order is Epay-gateway")
        if not args.stripe_secret:
            print("  [SKIP] need --stripe-secret to sign a valid event")
        else:
            payload = build_event(args.epay_trade_no)
            results.append(run_test("cross-gateway forge",
                                    200,  # server returns 200 but logs mismatch
                                    stripe_url, payload,
                                    sign(payload, args.stripe_secret)))
            print("  -> confirm in DB: TEST_EPAY_PENDING.status still 'pending'")
        print()

    # ---- Test 3: payment_status != paid --------------------------------
    if "3" not in args.skip:
        print("Test 3: valid signature, payment_status=unpaid")
        if not args.stripe_secret:
            print("  [SKIP] need --stripe-secret")
        else:
            payload = build_event(args.stripe_trade_no, payment_status="unpaid")
            results.append(run_test("unpaid event", 200, stripe_url, payload,
                                    sign(payload, args.stripe_secret)))
            print("  -> confirm in DB: TEST_STRIPE_PENDING.status still 'pending'")
        print()

    # ---- Test 4: Creem webhook targeting a Stripe order ----------------
    if "4" not in args.skip:
        print("Test 4: Creem webhook referencing a Stripe-gateway order")
        # Creem webhook signature format differs; send unsigned to observe
        # rejection path. Server should reject before touching the order.
        payload = json.dumps({
            "eventType": "checkout.completed",
            "object": {"id": args.stripe_trade_no,
                       "metadata": {"user_id": "1"},
                       "customer": {"email": "", "name": ""},
                       "status": "succeeded"}
        }, separators=(",", ":")).encode()
        # Expect non-2xx OR 200 with mismatch logged; DB must not change.
        code, body = post(creem_url, payload, None)
        print(f"  [INFO] creem webhook returned HTTP {code}")
        print(f"  -> confirm in DB: TEST_STRIPE_PENDING.status still 'pending'")
        results.append(True)  # verification is via DB, not HTTP code
        print()

    # ---- Test 5: happy path --------------------------------------------
    if "5" not in args.skip:
        print("Test 5: happy path — legit Stripe event on a Stripe order")
        if not args.stripe_secret:
            print("  [SKIP] need --stripe-secret")
        else:
            payload = build_event(args.stripe_trade_no, payment_status="paid")
            results.append(run_test("happy path", 200, stripe_url, payload,
                                    sign(payload, args.stripe_secret)))
            print("  -> confirm in DB: TEST_STRIPE_PENDING.status now 'success',")
            print("     user quota increased by topUp.Money * QuotaPerUnit")
        print()

    print(f"summary: {sum(results)}/{len(results)} HTTP assertions passed")
    print("remember to verify DB state for tests 2/3/4 (status unchanged)")
    print("and for test 5 (status flipped + quota bumped)")
    return 0 if all(results) else 1


if __name__ == "__main__":
    sys.exit(main())
