# plan-delivery Specification

## Purpose
TBD - created by archiving change add-plan-order-payment-system. Update Purpose after archive.
## Requirements
### Requirement: Paid orders MUST deliver plans synchronously

After payment is confirmed, the system MUST immediately create a UserPlan instance to fulfill the purchase within the payment callback transaction.

#### Scenario: Deliver plan immediately after payment confirmation

**Given** an order exists with:
- order_id = 123
- user_id = 5
- plan_id = "monthly"
- status = "paid"
- user_plan_id = 0 (not yet delivered)
**And** the plan "monthly" has:
- default_quota = 1000000
- validity_days = 30
- type = "subscription"
**When** DeliverPlan(123) is called within the payment callback transaction
**Then** the system creates a UserPlan instance with:
- user_id = 5
- plan_id = "monthly"
- source = "purchase"
- source_order_id = order.order_no
- quota = 1000000
- status = "active"
- Snapshot fields: plan_name, plan_display_name, plan_category, etc.
**And** determines queue position:
- If user has no current plan: is_current=1, queue_position=0
- Else: is_current=0, queue_position=MAX+1
**And** updates order:
- status = "delivered"
- user_plan_id = new UserPlan ID
- delivered_at = current time
**And** commits the transaction atomically

#### Scenario: Deliver plan to queue when user has active plan

**Given** a user with user_id=5 has an active plan:
- is_current = 1
- queue_position = 0
**And** 2 other plans queued with queue_position = 1, 2
**And** an order for this user is paid
**When** DeliverPlan() is called
**Then** the new UserPlan is created with:
- is_current = 0
- queue_position = 3 (MAX queued position + 1)
**And** the new plan waits in queue for activation

### Requirement: Plan delivery MUST be idempotent

Multiple calls to deliver the same order MUST NOT create duplicate UserPlan instances.

#### Scenario: Skip delivery if order already delivered

**Given** an order exists with:
- order_id = 123
- status = "delivered"
- user_plan_id = 456 (already has UserPlan)
**When** DeliverPlan(123) is called again
**Then** the system detects user_plan_id > 0
**And** returns success without creating a new UserPlan
**And** no database changes occur

### Requirement: Queue capacity validation MUST prevent over-delivery

The system MUST validate queue capacity before delivery to enforce the 10-plan limit per user.

#### Scenario: Successful delivery when user has 9 active plans

**Given** a user has 9 active plans (status='active')
**And** an order for this user is paid
**When** DeliverPlan() is called
**Then** queue capacity validation passes (9 < 10)
**And** the new UserPlan is created successfully
**And** the user now has 10 active plans

#### Scenario: Delivery fails when user has 10 active plans

**Given** a user has 10 active plans (status='active')
**And** an order for this user is paid (edge case: queue was not full at order creation, but became full before payment)
**When** DeliverPlan() is called
**Then** queue capacity validation fails
**And** the system logs error "Queue capacity exceeded during delivery"
**And** the order status remains "paid"
**And** the delivery_retry_count is incremented
**And** automatic retry will attempt delivery again (up to 3 times)
**And** if queue space opens up before max retries, delivery succeeds
**And** otherwise, admin intervention is required

### Requirement: Plan snapshots MUST preserve original configuration

UserPlan instances MUST snapshot all relevant plan configuration at purchase time to handle plan template changes gracefully.

#### Scenario: Snapshot plan fields during delivery

**Given** a plan "monthly" has:
- display_name = "月卡套餐"
- category = "monthly"
- priority = 10
- channel_groups = ["group1", "group2"]
- daily_quota_limit = 50000
- rate_limit_rules = JSON string
**When** a UserPlan is created from this plan
**Then** the UserPlan snapshots:
- plan_name = "monthly"
- plan_display_name = "月卡套餐"
- plan_category = "monthly"
- plan_priority = 10
- plan_channel_groups = ["group1", "group2"]
- plan_daily_quota_limit = 50000
- plan_rate_limit_rules = JSON string
**And** if the plan template changes later, the UserPlan retains original values

### Requirement: Delivery source MUST be traceable

Each UserPlan instance MUST record its creation source for audit and support purposes.

#### Scenario: Record purchase source in UserPlan

**Given** an order with order_no = "PO5NO1705023456789"
**When** a UserPlan is created from this order
**Then** the UserPlan has:
- source = "purchase"
- source_order_id = "PO5NO1705023456789"
- purchased_at = order.paid_at
**And** administrators can trace the UserPlan back to the original order

