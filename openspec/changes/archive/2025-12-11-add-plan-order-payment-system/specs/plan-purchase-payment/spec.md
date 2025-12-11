# Plan Purchase Payment

## ADDED Requirements

### Requirement: Users can initiate Epay payment for orders

The system MUST allow users to pay for pending orders using Epay (Alipay/WeChat Pay) through a secure payment gateway integration.

#### Scenario: Initiate Alipay payment for pending order

**Given** a user has created an order with:
- order_id = 123
- order_no = "PO5NO1705023456789"
- status = "pending"
- final_price = 88.00
- created_at = 5 minutes ago (not expired)
**When** the user calls POST /api/plan/purchase/pay with { order_id: 123, payment_method: "alipay" }
**Then** the system:
- Calls Epay API with amount=88.00, out_trade_no="PO5NO1705023456789"
- Updates order.payment_method = "alipay"
- Stores payment_trade_no from Epay response
**And** returns { payment_url: "https://epay.example.com/..." }

#### Scenario: Reject payment for expired order

**Given** an order exists with:
- order_id = 456
- status = "pending"
- created_at = 31 minutes ago (expired)
**When** the user calls POST /api/plan/purchase/pay with { order_id: 456, payment_method: "alipay" }
**Then** the system returns error 400 "订单已过期，请重新购买"
**And** no payment is initiated

#### Scenario: Reject payment for already-paid order

**Given** an order exists with:
- order_id = 789
- status = "delivered"
**When** the user calls POST /api/plan/purchase/pay with { order_id: 789, payment_method: "alipay" }
**Then** the system returns error 400 "订单状态异常"
**And** no payment is initiated

### Requirement: Payment callbacks MUST be idempotent

Payment gateway callbacks can be retried multiple times; the system MUST process each callback exactly once to prevent duplicate charges or deliveries.

#### Scenario: Process payment callback once for pending order

**Given** an order exists with:
- order_no = "PO5NO1705023456789"
- status = "pending"
**When** Epay sends a callback to GET /api/plan/purchase/epay/notify with valid signature and trade_no
**Then** the system:
- Acquires order lock using sync.Map
- Begins database transaction with SELECT ... FOR UPDATE
- Updates order status = "paid", paid_at = current time
- Calls DeliverPlan() to create UserPlan
- Updates order status = "delivered", user_plan_id, delivered_at
- Commits transaction
- Returns "success" to Epay

#### Scenario: Ignore duplicate payment callback for already-paid order

**Given** an order exists with:
- order_no = "PO5NO1705023456789"
- status = "paid"
**When** Epay sends a duplicate callback (retry)
**Then** the system:
- Acquires order lock
- Begins database transaction with SELECT ... FOR UPDATE
- Detects order status != "pending"
- Commits transaction without changes
- Returns "success" to Epay
**And** no duplicate UserPlan is created

### Requirement: Payment signatures MUST be verified for security

All payment callbacks MUST have valid signatures to prevent fraud and tampering.

#### Scenario: Accept callback with valid signature

**Given** Epay callback arrives with:
- trade_no = "EP123456"
- out_trade_no = "PO5NO1705023456789"
- money = "88.00"
- sign = MD5(concatenated params + EPAY_KEY)
**When** the system verifies the signature
**Then** the signature validation passes
**And** the callback is processed

#### Scenario: Reject callback with invalid signature

**Given** Epay callback arrives with:
- trade_no = "EP123456"
- out_trade_no = "PO5NO1705023456789"
- money = "88.00"
- sign = "invalid_signature"
**When** the system verifies the signature
**Then** the signature validation fails
**And** the system returns "fail"
**And** the order is not updated

#### Scenario: Reject callback with mismatched payment amount

**Given** an order exists with:
- order_no = "PO5NO1705023456789"
- final_price = 88.00
**And** Epay callback arrives with:
- trade_no = "EP123456"
- out_trade_no = "PO5NO1705023456789"
- money = "0.01" (mismatched amount)
- sign = valid signature
**When** the system processes the callback
**Then** the system detects amount mismatch (0.01 != 88.00)
**And** the system logs error "Payment amount mismatch"
**And** the system returns "fail"
**And** the order status remains "pending"

### Requirement: Failed payment deliveries MUST be retried automatically

If plan delivery fails after successful payment, the system MUST retry automatically to ensure users receive their purchased plans.

#### Scenario: Retry failed delivery for paid order

**Given** an order exists with:
- order_id = 123
- status = "paid"
- paid_at = 6 minutes ago
- user_plan_id = 0 (delivery failed)
**When** the background task RetryFailedDeliveries() runs
**Then** the system calls DeliverPlan(123)
**And** if delivery succeeds:
- order status updates to "delivered"
- order.user_plan_id is set
- order.delivered_at is recorded

#### Scenario: Stop retrying after 3 failed delivery attempts

**Given** an order exists with:
- order_id = 123
- status = "paid"
- user_plan_id = 0
- delivery_retry_count = 3 (already tried 3 times)
**When** the background task RetryFailedDeliveries() runs
**Then** the system skips this order (delivery_retry_count >= 3)
**And** the system logs error "Delivery failed after 3 retries"
**And** the system sends an admin notification
**And** the order remains in "paid" status for manual intervention
**And** no further automatic retries are attempted

### Requirement: Plans table MUST have purchasable configuration

Each plan MUST specify whether it can be purchased online to control sales channels appropriately.

#### Scenario: Filter API returns only purchasable plans

**Given** plans exist:
- Plan A: type="subscription", purchasable=1
- Plan B: type="trial", purchasable=0
- Plan C: type="consumption", purchasable=1
**When** the user calls GET /api/plan/enabled?purchasable=true
**Then** the system returns only Plan A and Plan C
**And** Plan B is excluded from results

## MODIFIED Requirements

None - this is a new capability.

## REMOVED Requirements

None - this is a new capability.
