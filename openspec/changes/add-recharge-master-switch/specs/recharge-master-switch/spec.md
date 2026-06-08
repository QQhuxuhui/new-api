## ADDED Requirements

### Requirement: Recharge Master Switch Configuration
The system SHALL provide a single system-level boolean setting `RechargeDisabled` (default `false`) that, when enabled, disables all outbound recharge/top-up entrances site-wide. The setting SHALL be persisted and hot-reloaded like other system options, and SHALL be editable by administrators in the payment settings page.

#### Scenario: Admin enables the master switch
- **WHEN** an administrator turns on「关闭所有充值入口」in payment settings
- **THEN** the option `RechargeDisabled` SHALL be persisted as `true`
- **AND** the new value SHALL take effect at runtime without a restart

#### Scenario: Default disabled (no behavior change)
- **GIVEN** a system where the option was never set
- **WHEN** the system loads options at boot
- **THEN** `RechargeDisabled` SHALL default to `false`
- **AND** all recharge entrances SHALL behave exactly as before this feature

### Requirement: Wallet Page Renders As Not-Configured When Disabled
When `RechargeDisabled` is enabled, `GET /api/user/topup/info` SHALL report every payment method as unavailable so that the 钱包管理 (wallet) page renders identically to the state where no recharge address/credential is configured. The wallet page itself SHALL remain accessible so users can still view balance and bills.

#### Scenario: Topup info forced empty
- **GIVEN** `RechargeDisabled` is `true`
- **WHEN** a user requests `GET /api/user/topup/info`
- **THEN** `enable_online_topup`, `enable_stripe_topup`, `enable_creem_topup`, and `enable_usdt_topup` SHALL all be `false`
- **AND** `pay_methods` SHALL be empty
- **AND** the wallet page SHALL show its existing empty/not-configured state (the「管理员未开启在线充值功能」banner)

#### Scenario: Redemption code stays available
- **GIVEN** `RechargeDisabled` is `true`
- **WHEN** the user views the wallet page
- **THEN** the 兑换码充值 (redemption code) entry SHALL remain visible and usable

### Requirement: Hard-Block New Recharge And Plan-Purchase Orders When Disabled
When `RechargeDisabled` is enabled, the system SHALL reject all server-side endpoints that initiate a new recharge or create/pay a top-up or plan-purchase order, even when called directly (bypassing the UI). The block SHALL return a clear error response and SHALL NOT credit any balance.

#### Scenario: Direct call to a recharge-initiation endpoint is rejected
- **GIVEN** `RechargeDisabled` is `true`
- **WHEN** an authenticated user POSTs to any recharge-initiation endpoint (易支付 `/api/user/pay`, Stripe `/api/user/stripe/pay`, Creem `/api/user/creem/pay`, USDT `/api/user/pay/usdt`, `/api/user/topup/order/create`, `/api/user/topup/order/pay`)
- **THEN** the system SHALL reject the request with an error response indicating recharge is disabled
- **AND** no order SHALL be created and no balance SHALL be credited

#### Scenario: Plan-purchase order creation is rejected
- **GIVEN** `RechargeDisabled` is `true`
- **WHEN** an authenticated user POSTs to `/api/user/plan/purchase/create` or `/api/user/plan/purchase/pay`
- **THEN** the system SHALL reject the request with an error response indicating recharge/purchase is disabled

### Requirement: Already-Paid Settlement And Admin Top-Up Are Never Blocked
The master switch SHALL only stop the initiation of new recharges. Payment provider callbacks/webhooks and administrator manual completion SHALL continue to function while `RechargeDisabled` is `true`, so that already-paid or in-flight orders still settle and no funds are lost.

#### Scenario: Payment callback still credits a paid order
- **GIVEN** `RechargeDisabled` is `true`
- **AND** a user had already paid for an order before/around the switch was enabled
- **WHEN** the payment provider invokes the callback/notify endpoint (e.g. `EpayNotify`, `StripeWebhook`, `CreemWebhook`, `EpUsdtNotify`, top-up/plan order notify)
- **THEN** the callback SHALL be processed normally and the user's balance/plan SHALL be credited

#### Scenario: Admin manual completion still works
- **GIVEN** `RechargeDisabled` is `true`
- **WHEN** an administrator manually completes a top-up or plan order (补单)
- **THEN** the completion SHALL succeed and credit the user

### Requirement: Frontend Hides All Standalone Recharge Entrances When Disabled
The system SHALL expose the master switch state to the frontend via `GET /api/status` as `recharge_disabled`, and the frontend SHALL hide every standalone「充值」/plan-purchase entry point when it is `true`, while keeping the 钱包管理 page reachable.

#### Scenario: Standalone recharge CTAs hidden
- **GIVEN** `/api/status` reports `recharge_disabled: true`
- **WHEN** the user browses the app
- **THEN** the dashboard「充值」button, the affiliate-reward card recharge entry, the MyPlans「充值」buttons, the PlanPricing「按量付费-钱包充值」tab, and the plan「立即购买」buttons SHALL be hidden
- **AND** the 钱包管理 page SHALL remain reachable, showing balance/bills with an empty recharge area

#### Scenario: Switch off restores everything
- **GIVEN** `/api/status` reports `recharge_disabled: false`
- **WHEN** the user browses the app
- **THEN** all recharge and plan-purchase entrances SHALL be visible and functional as before
