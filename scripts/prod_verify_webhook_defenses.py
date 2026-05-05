#!/usr/bin/env python3
"""
Production-safe verification of the v249 webhook defenses.

Every request uses a trade_no that does NOT exist in the database, so
even if any validation layer silently failed there is no order to
credit and no quota is moved. We only observe HTTP status codes and,
optionally, server logs.

What this checks:

  T1  Stripe webhook rejects unsigned requests (400)
  T2  Stripe webhook rejects requests signed with a wrong secret (400)
  T3  Stripe webhook with a valid signature + fake trade_no + paid ->
      HTTP 200 and server logs 'model.Recharge ... 充值订单不存在'
  T4  Stripe webhook with a valid signature + fake trade_no + unpaid ->
      HTTP 200 and server logs '支付尚未完成, payment_status: unpaid'
  T5  Creem webhook rejects unsigned/malformed requests
  T6  Plan-purchase Epay notify returns 'fail' on unsigned request

T3 and T4 need the real StripeWebhookSecret (pass via env
NEW_API_STRIPE_SECRET) so the server's signature check passes and we
actually reach the business logic for observation. Skip them with
--safe-only if you do not want to put the secret on this machine.

No DB state is created, read, or modified. No quota is credited.
"""

import argparse
import hashlib
import hmac
import json
import os
import secrets as _secrets
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


FAKE_TRADE_NO = f"SECTEST_{_secrets.token_hex(8).upper()}"


def build_stripe_event(trade_no: str, payment_status: str = "paid") -> bytes:
    event = {
        "id": f"evt_sectest_{int(time.time())}",
        "type": "checkout.session.completed",
        "data": {
            "object": {
                "id": f"cs_sectest_{int(time.time())}",
                "object": "checkout.session",
                "client_reference_id": trade_no,
                "customer": "cus_sectest",
                "status": "complete",
                "payment_status": payment_status,
                "amount_total": 1,
                "currency": "usd",
            }
        },
    }
    return json.dumps(event, separators=(",", ":")).encode()


def sign_stripe(payload: bytes, secret: str) -> str:
    ts = str(int(time.time()))
    mac = hmac.new(secret.encode(), f"{ts}.".encode() + payload,
                   hashlib.sha256).hexdigest()
    return f"t={ts},v1={mac}"


def post(url: str, data: bytes, headers: dict[str, str]) -> tuple[int, str]:
    req = urllib.request.Request(url, data=data, headers=headers,
                                 method="POST")
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            return resp.status, resp.read(400).decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read(400).decode("utf-8", "replace")


def get(url: str) -> tuple[int, str]:
    try:
        with urllib.request.urlopen(url, timeout=15) as resp:
            return resp.status, resp.read(400).decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read(400).decode("utf-8", "replace")


def check(label: str, actual: int, expected: set[int]) -> bool:
    ok = actual in expected
    mark = "PASS" if ok else "FAIL"
    exp_str = "/".join(str(c) for c in sorted(expected))
    print(f"  [{mark}] {label}: HTTP {actual} (expected {exp_str})")
    return ok


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--url", required=True,
                   help="production base URL, e.g. https://api.example.com")
    p.add_argument("--safe-only", action="store_true",
                   help="skip T3/T4 which need the real Stripe secret")
    p.add_argument("--confirm-production", action="store_true",
                   help="required acknowledgement that --url is production")
    args = p.parse_args()

    if not args.confirm_production:
        print("refusing to run without --confirm-production. Pass this flag "
              "only after you have verified --url is the intended target.",
              file=sys.stderr)
        return 2

    base = args.url.rstrip("/")
    print(f"target: {base}")
    print(f"fake trade_no used by every request: {FAKE_TRADE_NO}")
    print("(this value does not exist in your DB, so no order can be credited)")
    print()

    stripe_url = f"{base}/api/user/stripe/webhook"
    creem_url = f"{base}/api/user/creem/webhook"
    results: list[bool] = []

    # -----------------------------------------------------------------
    print("T1  Stripe webhook: no signature header")
    code, _ = post(stripe_url, b"{}", {"Content-Type": "application/json"})
    results.append(check("unsigned rejected", code, {400, 403}))

    print("T2  Stripe webhook: signature computed with a WRONG secret")
    payload = build_stripe_event(FAKE_TRADE_NO)
    bogus = sign_stripe(payload, "wrong-secret-" + _secrets.token_hex(8))
    code, _ = post(stripe_url, payload,
                   {"Content-Type": "application/json",
                    "Stripe-Signature": bogus})
    results.append(check("wrong-secret rejected", code, {400, 403}))
    print()

    # -----------------------------------------------------------------
    real_secret = os.environ.get("NEW_API_STRIPE_SECRET", "")

    if args.safe_only or not real_secret:
        print("T3/T4 SKIPPED (pass secret via env NEW_API_STRIPE_SECRET "
              "and drop --safe-only to run).")
        print()
    else:
        print("T3  Stripe webhook: valid signature + fake trade_no + paid")
        payload = build_stripe_event(FAKE_TRADE_NO, "paid")
        code, body = post(stripe_url, payload,
                          {"Content-Type": "application/json",
                           "Stripe-Signature": sign_stripe(payload,
                                                           real_secret)})
        ok = check("accepted-and-rejected at DB lookup", code, {200})
        print("       -> check server logs for: '充值订单不存在'")
        print(f"          body: {body.strip()[:160]}")
        results.append(ok)

        print("T4  Stripe webhook: valid signature + fake trade_no + unpaid")
        payload = build_stripe_event(FAKE_TRADE_NO, "unpaid")
        code, body = post(stripe_url, payload,
                          {"Content-Type": "application/json",
                           "Stripe-Signature": sign_stripe(payload,
                                                           real_secret)})
        ok = check("unpaid early-exit", code, {200})
        print("       -> check server logs for: "
              "'Stripe Checkout 支付尚未完成'")
        print(f"          body: {body.strip()[:160]}")
        results.append(ok)
        print()

    # -----------------------------------------------------------------
    print("T5  Creem webhook: unsigned junk payload")
    code, body = post(creem_url, b"{}", {"Content-Type": "application/json"})
    # In production, the Creem handler aborts with 401 when the signature
    # header is missing (or 400 if the body fails to parse). Anything else
    # — including 200 (would mean signature bypass), 404 (route mismatch
    # masking the test), or 5xx (server error masking the defense) — is a
    # red flag, not a pass.
    ok = check("creem rejects junk", code, {400, 401, 403, 422})
    if not ok:
        print(f"       !! unexpected response: code={code} "
              f"body={body.strip()[:160]!r}")
    print("       -> server should log 'Creem Webhook缺少签名头' "
          "or signature verification failure")
    results.append(ok)
    print()

    # -----------------------------------------------------------------
    print("T6  Plan-purchase Epay notify: no signature params")
    code, body = get(f"{base}/api/plan/purchase/epay/notify?"
                     f"out_trade_no={FAKE_TRADE_NO}"
                     f"&trade_status=TRADE_SUCCESS&money=0.01")
    ok = check("plan epay notify unsigned", code, {200})  # returns text 'fail'
    snippet = body.strip().lower()
    looks_rejected = "fail" in snippet
    if not looks_rejected:
        print(f"       !! unexpected body: {body.strip()[:160]}")
    else:
        print("       body contains 'fail' as expected")
    results.append(ok and looks_rejected)
    print()

    # -----------------------------------------------------------------
    passed = sum(results)
    total = len(results)
    print(f"summary: {passed}/{total} checks passed")
    print()
    print("What to do next:")
    print("  - Read your server access log for the fake trade_no:")
    print(f"      grep '{FAKE_TRADE_NO}' <your-access-log>")
    print("  - Read your app log for the expected rejection messages.")
    print("  - Confirm in DB that this trade_no never appeared:")
    print(f"      SELECT * FROM topups WHERE trade_no = '{FAKE_TRADE_NO}';")
    print("    (should return zero rows)")
    return 0 if passed == total else 1


if __name__ == "__main__":
    sys.exit(main())
