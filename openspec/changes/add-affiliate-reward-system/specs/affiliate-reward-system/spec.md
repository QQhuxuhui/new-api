# Affiliate Reward System Specification

## ADDED Requirements

### Requirement: Affiliate Audit Log Creation On Real Payment
The system SHALL create one `aff_audit_logs` row whenever a real payment from an invited user (i.e., a user with non-zero `inviter_id`) succeeds via one of the supported payment channels.

#### Scenario: Top-up payment success creates audit log
- **GIVEN** user B has `inviter_id = A` and `users.aff_status = 0` for A
- **AND** B initiates a top-up via Stripe / Creem / EPay that writes to `top_ups` with `status = 'success'`
- **WHEN** the payment success handler completes
- **THEN** the system SHALL insert one `aff_audit_logs` row with:
  - `inviter_user_id = A.id`, `invitee_user_id = B.id`
  - `source_type = 'topup'`, `source_id = top_ups.id`
  - `amount_native = top_ups.money`, `currency = 'USD'`
  - `amount_usd = top_ups.money` (no conversion needed)
  - `reward_usd = amount_usd * InviterRewardDefaultPercent / 100`
  - `status = 'pending'`
  - `eligible_at = paid_at_ms + InviterRewardCooldownDays * 86400000`

#### Scenario: Topup order success creates audit log with currency conversion
- **GIVEN** user B has `inviter_id = A`
- **AND** B completes a `topup_orders` purchase with `status = 'paid'` and `final_price` in CNY
- **WHEN** the order paid handler completes
- **THEN** the system SHALL insert one audit log with:
  - `source_type = 'topup_order'`, `currency = 'CNY'`
  - `amount_native = topup_orders.final_price`
  - `price_ratio_used = current setting.GetPriceRatio()` (frozen at write time)
  - `amount_usd = amount_native / price_ratio_used`
  - `reward_usd = amount_usd * InviterRewardDefaultPercent / 100`

#### Scenario: Plan order success creates audit log with currency conversion
- **GIVEN** user B has `inviter_id = A`
- **AND** B purchases a non-trial plan with `final_price > 1` (CNY)
- **WHEN** `plan_orders.status` transitions to `'paid'`
- **THEN** the system SHALL insert one audit log with:
  - `source_type = 'plan_order'`, `currency = 'CNY'`
  - Currency conversion same as topup order
  - `eligible_at = paid_at + cooldown` (using `paid_at`, NOT `delivered_at`)

#### Scenario: Non-payment quota changes do not create audit log
- **GIVEN** user B has `inviter_id = A`
- **WHEN** B's quota is increased via redemption code, admin manual adjustment, task refund, or `IncreaseUserQuota` direct call
- **THEN** the system SHALL NOT create any `aff_audit_logs` row

#### Scenario: Admin manual top-up completion creates audit log
- **GIVEN** user B has `inviter_id = A`
- **AND** B has a `top_ups` row created by a real payment whose webhook callback failed, leaving the row in non-success status
- **WHEN** an administrator invokes `AdminCompleteTopUp` to mark `top_ups.status='success'`
- **THEN** the audit log SHALL be created normally, identical to the "Top-up payment success creates audit log" scenario
- **AND** if the administrator wishes to exclude this specific record from auto-settlement, they SHALL use the `mark-offline-paid` flow on the resulting `pending` log

#### Scenario: Trial plan does not trigger audit log
- **GIVEN** user B has `inviter_id = A`
- **WHEN** B purchases a `plan_orders` row with `plan_type = 'trial'` OR `final_price ≤ 1`
- **THEN** the system SHALL NOT create an audit log
- **AND** the system MAY log a debug entry for traceability

#### Scenario: User without inviter does not trigger audit log
- **GIVEN** user B has `inviter_id = 0` or NULL
- **WHEN** B makes any kind of payment
- **THEN** no audit log SHALL be created

#### Scenario: Inviter chain stops at level 1
- **GIVEN** user B has `inviter_id = A`, and user A has `inviter_id = C` (C is a grand-inviter)
- **WHEN** B makes a real payment
- **THEN** exactly ONE audit log SHALL be created with `inviter_user_id = A`
- **AND** NO audit log SHALL be created for C, regardless of A's `aff_status`

#### Scenario: Promotional bonus excluded from reward base
- **GIVEN** B's payment includes a promotional doubling bonus (e.g., 充 100 送 100,实际入账 200)
- **WHEN** the audit log is created
- **THEN** `amount_native` SHALL equal the **actually paid** amount (即 100), NOT the post-bonus quota
- **AND** documentation/code SHALL clearly distinguish the "paid" amount from the "received quota" amount when reading source rows

#### Scenario: Duplicate write for same source is rejected by unique constraint
- **GIVEN** an audit log already exists for `(source_type='topup', source_id=X)`
- **WHEN** a duplicate write attempt occurs (e.g., webhook retry, admin manual completion of a row already auto-completed)
- **THEN** the system SHALL detect the unique constraint violation and silently skip
- **AND** SHALL log a warning entry for traceability

### Requirement: Anti-Fraud Pre-check At Audit Log Insertion
The system SHALL check anti-fraud rules before inserting a new audit log; on rule hit, the log is inserted with `status='rejected'` and the reject reason recorded, instead of `status='pending'`.

#### Scenario: Same IP rejection (driven by user_login_ip_logs)
- **GIVEN** user B has `inviter_id = A`
- **AND** the `user_login_ip_logs` table contains at least one IP that appears in BOTH A's and B's records within the last 24 hours
- **WHEN** B makes a real payment that would trigger an audit log
- **THEN** the audit log SHALL be inserted with `status = 'rejected'` and `reject_reason = 'same_ip'`

#### Scenario: Same payment account rejection (driven by user_payment_accounts)
- **GIVEN** user B has `inviter_id = A`
- **AND** B's payment account record (in `user_payment_accounts`, keyed by `(user_id, provider, account_id)`) shares the same `(provider, account_id)` with any of A's records
- **WHEN** B's payment succeeds
- **THEN** the audit log SHALL be inserted with `status = 'rejected'` and `reject_reason = 'same_payment_account'`

#### Scenario: Frozen distributor rejection
- **GIVEN** user A has `aff_status = 1` (frozen)
- **AND** the inviter status is read in the same transaction or via fresh read at audit log creation time
- **WHEN** any of A's invitees makes a payment
- **THEN** the audit log SHALL be inserted with `status = 'rejected'` and `reject_reason = 'inviter_frozen'`

#### Scenario: Anti-fraud not triggered returns clean log
- **GIVEN** no IP overlap, different payment account, and inviter not frozen
- **WHEN** an invitee makes a payment
- **THEN** the audit log SHALL be inserted with `status = 'pending'` normally

### Requirement: User Login IP Logging
The system SHALL persist user login IP addresses to enable the same-IP anti-fraud check.

#### Scenario: Login appends IP record
- **GIVEN** any successful login(password / GitHub / Passkey / LinuxDo / Telegram / 2FA — all paths converge at `setupLogin`)
- **WHEN** `setupLogin` completes
- **THEN** the system SHALL insert one row into `user_login_ip_logs` with `(user_id, ip, logged_at)`

#### Scenario: Old IP records are cleaned up
- **GIVEN** `user_login_ip_logs` rows older than 30 days
- **WHEN** the cleanup cron runs (daily)
- **THEN** the system SHALL delete those rows to prevent unbounded growth

### Requirement: Payment Account Tracking
The system SHALL persist a normalized record of payment accounts used by each user, to enable the same-payment-account anti-fraud check across multiple payment providers.

#### Scenario: Payment account upserted on successful payment
- **GIVEN** a payment success handler for any of: Stripe / Alipay (EPay) / WeChat (EPay) / Creem
- **WHEN** the handler extracts the upstream account identifier (Stripe customer_id / Alipay buyer_id / WeChat openid / Creem customer)
- **THEN** the system SHALL upsert a row into `user_payment_accounts` with `(user_id, provider, account_id, last_seen_at)`
- **AND** SHALL update `last_seen_at` if the row already exists

#### Scenario: Account ID extraction failure does not block payment
- **GIVEN** the upstream payment provider does not return a usable account identifier (e.g., legacy EPay without buyer_id)
- **WHEN** the payment success handler runs
- **THEN** the upsert SHALL be skipped silently
- **AND** the same-payment-account anti-fraud check SHALL fall back to "no rejection" for that record

### Requirement: Cooldown And Auto-Settlement
The system SHALL automatically settle eligible audit logs to the inviter's `AffQuota` after the cooldown period, in a scheduled background job.

#### Scenario: Cron scans and settles eligible logs with quota unit conversion
- **GIVEN** an audit log with `status = 'pending'` and `eligible_at <= now()`
- **AND** `EnableAffAutoSettle` is true
- **WHEN** the auto-settle cron job runs (hourly)
- **THEN** the system SHALL within a single transaction:
  - Compute `quota_delta = int(reward_usd * QuotaPerUnit)` (USD → token units; e.g., $0.5 × 500000 = 250000 tokens)
  - Increment `users.aff_quota` and `users.aff_history_quota` by `quota_delta` for the inviter
  - Update the audit log to `status = 'settled'` and `settled_at = now()`
  - Create one `inviter_reward_payouts` row with `settle_mode = 'auto'` and link via `settle_payout_id`

#### Scenario: Cron does not settle non-pending logs
- **GIVEN** audit logs with `status` in `'rejected'`, `'refunded'`, `'offline_paid'`, or `'settled'`
- **WHEN** the cron job runs
- **THEN** the system SHALL NOT modify these logs and SHALL NOT add quota

#### Scenario: Cooldown not yet elapsed
- **GIVEN** an audit log with `status = 'pending'` and `eligible_at > now()`
- **WHEN** the cron job runs
- **THEN** the system SHALL NOT settle this log

#### Scenario: Auto-settle disabled by global flag
- **GIVEN** `EnableAffAutoSettle = false`
- **WHEN** the cron job is invoked
- **THEN** the system SHALL exit immediately without scanning or modifying any data
- **AND** SHALL log a notice that auto-settle is disabled

#### Scenario: Settled rewards aggregate per inviter per cron run
- **GIVEN** multiple eligible logs for the same inviter
- **WHEN** the cron job runs
- **THEN** the system SHOULD batch-update them in one transaction per inviter, creating one `inviter_reward_payouts` row covering all logs in the batch

#### Scenario: Reward percent change does not affect already-pending logs
- **GIVEN** existing audit logs in `pending` status with `reward_usd` already computed
- **WHEN** the administrator changes `InviterRewardDefaultPercent` from 10 to 5
- **THEN** the cron SHALL settle the existing `pending` logs with the original `reward_usd` (frozen at write time)
- **AND** new audit logs created after the config change SHALL use the new percent

### Requirement: Refund Hook For Audit Log Status Reversal
The system SHALL provide a `MarkRefunded` hook for future refund flows; the hook is implemented in v1 but explicitly NOT wired into any controller, since the project currently has no refund webhook (`PlanOrder.Status` lacks a `refunded` value).

#### Scenario: Refund of pending audit log is fully reversed
- **GIVEN** an audit log with `status = 'pending'` for source `top_ups.id = X`
- **WHEN** `MarkRefunded(source_type='topup', source_id=X)` is called
- **THEN** the audit log status SHALL change to `'refunded'`
- **AND** no quota adjustment SHALL occur (because nothing was settled)

#### Scenario: Refund of settled audit log goes to admin queue
- **GIVEN** an audit log with `status = 'settled'`, `reward_usd = 0.50`, and the corresponding `aff_quota` increment was already applied
- **WHEN** the refund hook is called for the same source
- **THEN** the audit log status SHALL change to `'refunded'`
- **AND** the inviter's `aff_quota` SHALL NOT be automatically decremented (to avoid negative balance)
- **AND** the system SHALL surface this record in an admin "post-settlement refund" review list for manual reconciliation

### Requirement: Admin Manual Offline-Paid Marking
The system SHALL provide an admin-only API and UI to mark `pending` audit logs as `offline_paid` so they are excluded from auto-settlement, recording the actual offline CNY amount paid.

#### Scenario: Admin batch marks logs as offline paid
- **GIVEN** an admin views the audit log list for inviter `A` and selects 3 `pending` logs
- **WHEN** the admin submits `POST /api/user/manage/:id/aff-audit-logs/mark-offline-paid` with `{log_ids:[…], offline_amount_cny: 200, note:"线下微信转账"}`
- **THEN** all 3 logs SHALL have `status='offline_paid'`, `offline_paid_at=now()`, `offline_paid_amount_cny=200`, `offline_paid_admin_id=adminUserId`, and `offline_paid_note='线下微信转账'`
- **AND** the operation SHALL be atomic — if any selected log is not in `pending` status the whole batch SHALL be rolled back

#### Scenario: Offline-paid logs are excluded from auto-settle
- **GIVEN** an audit log with `status = 'offline_paid'` and `eligible_at <= now()`
- **WHEN** the auto-settle cron job runs
- **THEN** the system SHALL NOT settle this log (no `aff_quota` increment, no status change)

#### Scenario: Operation is recorded in admin log
- **GIVEN** an admin marks logs as offline paid
- **WHEN** the API call succeeds
- **THEN** a `LogTypeManage` entry SHALL be written for the inviter user, recording the admin id, log ids covered, and CNY amount

### Requirement: User-Facing Aggregated Summary API
The system SHALL provide a user-facing summary endpoint that returns only aggregate numbers, with no information about individual invitees.

#### Scenario: Authenticated user fetches summary
- **GIVEN** an authenticated user U requests `GET /api/user/aff/summary`
- **WHEN** the request is processed
- **THEN** the response SHALL contain exactly:
  - `aff_count` (integer): U's invitee count
  - `aff_quota` (integer, token units): currently spendable reward balance
  - `aff_history_quota` (integer, token units): cumulative settled rewards
  - `aff_quota_usd` (float USD, derived = aff_quota / QuotaPerUnit): for human-readable display
  - `pending_amount_usd` (float USD): sum of `reward_usd` over U's `status='pending'` logs
  - `this_month_earned_usd` (float USD): sum of `reward_usd` over U's `status='settled'` logs settled in current calendar month, **using server timezone (Asia/Shanghai)**
  - `reward_percent` (float): the configured `InviterRewardDefaultPercent`
  - `cooldown_days` (integer): the configured `InviterRewardCooldownDays`
  - `aff_status` (string, "normal" | "frozen"): U's current distributor status
- **AND** the response SHALL NOT contain any field that exposes individual invitee identity, order numbers, payment methods, or per-transaction details

#### Scenario: Frozen user sees status in summary
- **GIVEN** user U has `aff_status = 1`
- **WHEN** U fetches the summary
- **THEN** `aff_status` field in the response SHALL equal `"frozen"`
- **AND** rejected_count SHALL NOT be exposed (privacy: do not surface anti-fraud activity to user)

#### Scenario: Offline-paid logs not visible to user
- **GIVEN** user U has logs with `status='offline_paid'`
- **WHEN** U fetches the summary
- **THEN** these logs SHALL NOT contribute to any field in the response (they are an admin-side concept only)

### Requirement: Admin Audit Log Inspection API
The system SHALL provide admin-only endpoints to inspect and reconcile any inviter's full audit log history.

#### Scenario: Admin views audit logs filtered by status
- **GIVEN** an admin requests `GET /api/user/manage/:id/aff-audit-logs?status=pending&page=1&page_size=20`
- **WHEN** the request is authorized
- **THEN** the response SHALL return paginated audit logs for inviter `:id` filtered by status, including fields:
  - `id`, `invitee_user_id`, `invitee_username`, `source_type`, `source_id`
  - `amount_native`, `currency`, `amount_usd`, `price_ratio_used`, `reward_usd`
  - `status`, `reject_reason`, `eligible_at`, `created_at`, `settled_at`
  - For `offline_paid` rows: `offline_paid_at`, `offline_paid_amount_cny`, `offline_paid_note`, `offline_paid_admin_id`

#### Scenario: Soft-deleted invitees still show in admin view
- **GIVEN** an audit log whose `invitee_user_id` references a soft-deleted user
- **WHEN** the admin fetches the audit log list
- **THEN** the row SHALL still appear with `invitee_username = '[deleted user #ID]'`
- **AND** the deletion SHALL NOT cascade to delete the audit log

#### Scenario: Admin views aggregated affiliate summary
- **GIVEN** an admin requests `GET /api/user/manage/:id/aff-summary`
- **WHEN** the request is authorized
- **THEN** the response SHALL include for inviter `:id`:
  - `invitee_count`, `pending_total_usd`, `settled_total_usd`
  - `offline_paid_total_cny`, `rejected_count`, `refunded_count`
  - Counts grouped by `reject_reason`

### Requirement: Affiliate Status Field For Distributor Freezing
The `users` table SHALL have an `aff_status` field that controls whether the user's referrals continue to generate audit logs.

#### Scenario: Default users have normal affiliate status
- **GIVEN** a newly created user
- **WHEN** the user record is inserted
- **THEN** `aff_status` SHALL default to `0` (normal)

#### Scenario: Frozen distributor produces rejected logs
- **GIVEN** user A has `aff_status = 1`
- **WHEN** any invitee of A makes a real payment
- **THEN** the resulting audit log SHALL have `status = 'rejected'` and `reject_reason = 'inviter_frozen'`

#### Scenario: Admin sets affiliate status via user edit modal
- **GIVEN** an administrator is editing user A in the user management interface (`EditUserModal`)
- **WHEN** the administrator changes A's `aff_status` from `0` to `1` (or vice versa) and saves
- **THEN** A's `aff_status` SHALL be persisted via the existing admin user-update API
- **AND** subsequent audit logs for A's invitees SHALL respect the new status (read with row-lock or fresh read inside the audit log insertion transaction)
- **AND** existing audit logs (in any status) SHALL NOT be retroactively modified by this status change

### Requirement: Inviter Reward Payout Settle Mode Field
The `inviter_reward_payouts` table SHALL distinguish payouts originating from the new auto-settle cron from existing manual admin batches.

#### Scenario: New auto batches are tagged
- **GIVEN** the auto-settle cron creates a payout
- **WHEN** the row is inserted
- **THEN** `settle_mode` SHALL be `'auto'`

#### Scenario: Existing admin manual flow remains unchanged
- **GIVEN** an admin uses the existing `CreateInviterRewardPayoutHandler` endpoint
- **WHEN** the payout is created
- **THEN** `settle_mode` SHALL default to `'manual'`
- **AND** the existing `pendingMoneyQueries` and admin UI behavior SHALL remain functionally unchanged for backward compatibility

### Requirement: Configuration Flags Control Behavior At Runtime
The system SHALL expose configuration flags so that affiliate behavior can be tuned without code changes.

#### Scenario: Reward percent is read from configuration
- **GIVEN** an audit log is being created
- **WHEN** computing `reward_usd`
- **THEN** the system SHALL use the current value of `InviterRewardDefaultPercent` (default 10.0)

#### Scenario: Cooldown days is read from configuration
- **GIVEN** an audit log is being created
- **WHEN** computing `eligible_at`
- **THEN** the system SHALL use the current value of `InviterRewardCooldownDays` (default 7) multiplied by 86400000 ms

#### Scenario: Auto-settle global kill switch
- **GIVEN** `EnableAffAutoSettle = false`
- **WHEN** the cron job fires
- **THEN** it SHALL exit immediately without modifying any audit log
- **AND** audit log writing on payment success SHALL continue normally (so resuming the cron later still has data to settle)

#### Scenario: Server-side range validation for financial config
- **GIVEN** any client (admin UI, curl, internal tool) calls `PUT /api/option/` with one of the affiliate config keys
- **WHEN** the value is parsed at the `model.UpdateOption` entry point
- **THEN** the following ranges SHALL be enforced **before any DB write**:
  - `InviterRewardDefaultPercent`: finite number in `[0, 100]` (0 is legal — operator may temporarily disable new reward accrual)
  - `InviterRewardCooldownDays`: integer in `[1, 365]`
  - `InviterRewardCutoffMs`: non-negative integer (`0` = disabled)
- **AND** invalid values SHALL return an error without persisting (so corrupt frontend or scripted clients cannot poison the option table with out-of-range values)
- **AND** front-end validation alone is NOT sufficient for these financial parameters

### Requirement: Manual Settlement Recovery API
The system SHALL provide an admin API to manually settle a specific stuck audit log, for use when the cron has missed a record due to bugs or temporary failures.

#### Scenario: Admin triggers settlement for a specific log
- **GIVEN** an audit log with `status = 'pending'` and `eligible_at <= now()` that was missed by the cron
- **WHEN** an admin calls `POST /api/user/manage/aff-audit-logs/:log_id/settle`
- **THEN** the system SHALL settle this single log using the same transaction logic as the cron (quota conversion, payout row, status update)
- **AND** SHALL refuse if the log is not in `pending` status (return 422)

### Requirement: Monthly Reconciliation Report
The system SHALL provide a monthly reconciliation report endpoint for finance/audit use.

#### Scenario: Admin fetches monthly aggregate
- **GIVEN** an admin requests `GET /api/user/manage/aff-monthly-report?year=2026&month=5`
- **WHEN** the request is authorized
- **THEN** the response SHALL include for the specified month (server timezone Asia/Shanghai):
  - `total_audit_logs_created` (count by status: pending/settled/rejected/refunded/offline_paid)
  - `total_settled_reward_usd`
  - `total_offline_paid_cny`
  - `total_rejected_count_by_reason`
  - `top_inviters` (top 10 by settled USD; admin view only, NOT exposed to users)

### Requirement: Historical Cutoff Migration (Legacy Status)
The system SHALL support a one-shot migration that flips all
`status='pending'` audit logs created before a given cutoff timestamp
into a new `status='legacy'` so they never enter the auto-settle pool,
yet remain visible in admin lists for audit and historical reference.

#### Scenario: Admin migrates pending logs created before cutoff
- **GIVEN** the platform has audit logs in `status='pending'` with various `created_at` values
- **WHEN** an admin calls `POST /api/user/manage/aff-audit-logs/mark-legacy` with body `{cutoff_ms: <timestamp>}` where `cutoff_ms > 0`
- **THEN** all `aff_audit_logs` rows with `status='pending' AND created_at < cutoff_ms` SHALL be updated to `status='legacy'`
- **AND** the response SHALL return `{migrated: <count>, cutoff_ms: <ts>}`
- **AND** a `LogTypeManage` entry SHALL be written recording the admin id, cutoff timestamp, and migrated row count

#### Scenario: Cutoff zero or negative is rejected
- **GIVEN** an admin attempts the mark-legacy endpoint with `cutoff_ms <= 0`
- **WHEN** the request is processed
- **THEN** the system SHALL reject with an error message (防止误操作迁移全表)
- **AND** no rows SHALL be modified

#### Scenario: Cutoff in the future is rejected
- **GIVEN** an admin attempts the mark-legacy endpoint with `cutoff_ms` greater than current server time
- **WHEN** the request is processed
- **THEN** the system SHALL reject with an error message indicating future cutoff is not allowed (防止误选未来时间把所有当前 pending 一键归档)
- **AND** no rows SHALL be modified
- **AND** the frontend datepicker SHALL also disable future dates as a first line of defense

#### Scenario: Legacy logs excluded from cron auto-settlement
- **GIVEN** an audit log with `status='legacy'` and `eligible_at <= now()`
- **WHEN** the auto-settle cron job runs
- **THEN** the system SHALL NOT settle this log (already filtered by `WHERE status='pending'`)

#### Scenario: Legacy not exposed in user-facing summary
- **GIVEN** a user U has audit logs with `status='legacy'` as inviter
- **WHEN** U fetches `/api/user/aff/summary`
- **THEN** these logs SHALL NOT contribute to `pending_amount_usd`, `this_month_earned_usd`, or any other field
- **AND** the user SHALL NOT be informed of legacy log existence (admin-side concept only)

#### Scenario: Legacy filterable in admin audit log list
- **GIVEN** an inviter has logs in mixed statuses including `legacy`
- **WHEN** an admin queries `GET /api/user/manage/:id/aff-audit-logs?status=legacy`
- **THEN** the response SHALL return only legacy rows with full audit fields

#### Scenario: Other statuses are not affected
- **GIVEN** audit logs in `settled`, `rejected`, `refunded`, or `offline_paid` states with `created_at < cutoff_ms`
- **WHEN** mark-legacy is called
- **THEN** these logs SHALL NOT be modified; only `status='pending'` logs are migrated

### Requirement: User Agreement Affiliate Clause
The platform's user agreement SHALL include a dedicated section describing the affiliate reward rules, anti-fraud actions, and use of station-internal credit.

#### Scenario: Agreement contains affiliate clause
- **GIVEN** the user-agreement document maintained by the platform
- **WHEN** users register or first access the affiliate page
- **THEN** the agreement SHALL clearly state:
  - Reward is station-internal credit (`AffQuota`), not redeemable for cash
  - Anti-fraud rules and the consequence of being frozen (`aff_status=1`)
  - Reward percent and cooldown days are configurable and current values are displayed in the affiliate page
  - Platform reserves the right to suspend the program with `EnableAffAutoSettle=false`
