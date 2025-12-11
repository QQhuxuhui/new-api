# Plan Order Management

## ADDED Requirements

### Requirement: Users can create plan purchase orders

The system MUST allow users to create orders for purchasing plans directly through the API, with proper validation and price calculation.

#### Scenario: Create order for purchasable subscription plan

**Given** a user is logged in
**And** a subscription plan "monthly" exists with purchasable=1, price=88.00, original_price=100.00
**And** the user has fewer than 10 active plans
**When** the user calls POST /api/plan/purchase/create with { plan_id: "monthly" }
**Then** the system creates a PlanOrder record with:
- order_no = unique identifier (format: PO{userId}NO{timestamp})
- user_id = current user's ID
- plan_id = "monthly"
- plan_price = 88.00 (snapshot from Plan.price)
- plan_original_price = 100.00 (snapshot from Plan.original_price)
- final_price = 88.00
- status = "pending"
- expired_at = current time + 30 minutes
**And** returns the order details with order_id and order_no

#### Scenario: Reject order creation for non-purchasable plan

**Given** a user is logged in
**And** a trial plan "trial-7day" exists with purchasable=0
**When** the user calls POST /api/plan/purchase/create with { plan_id: "trial-7day" }
**Then** the system returns error 400 "该套餐不支持在线购买"
**And** no order is created

#### Scenario: Reject order creation when queue is full

**Given** a user is logged in
**And** the user already has 10 active plans (status='active')
**And** a subscription plan "monthly" exists with purchasable=1
**When** the user calls POST /api/plan/purchase/create with { plan_id: "monthly" }
**Then** the system returns error 400 "队列已满，最多拥有10个套餐"
**And** no order is created

### Requirement: Users can view their order history

The system MUST allow users to retrieve a paginated list of their purchase orders with status information.

#### Scenario: Retrieve user's order history with pagination

**Given** a user is logged in with user_id=5
**And** the user has created 25 orders in total
**When** the user calls GET /api/plan/purchase/my-orders?page=1&page_size=20
**Then** the system returns:
- orders: array of 20 order objects ordered by created_at DESC
- total: 25
- page: 1
- page_size: 20
**And** each order includes: order_id, order_no, plan_name, final_price, status, created_at, paid_at, delivered_at

### Requirement: Orders SHALL automatically expire after 30 minutes if unpaid

Unpaid orders MUST transition to expired status to prevent indefinite pending orders.

#### Scenario: Mark pending order as expired after 30 minutes

**Given** an order exists with:
- order_id = 123
- status = "pending"
- created_at = 30 minutes ago
**When** the background task ExpireOldOrders() runs
**Then** the order status is updated to "expired"
**And** the order can no longer be paid

#### Scenario: Do not expire paid orders

**Given** an order exists with:
- order_id = 456
- status = "paid"
- created_at = 35 minutes ago
**When** the background task ExpireOldOrders() runs
**Then** the order status remains "paid"

### Requirement: Order numbers MUST be unique and traceable

Each order MUST have a unique identifier that can be traced to the user and payment gateway.

#### Scenario: Generate unique order number

**Given** a user with user_id=5 creates an order
**And** the current timestamp is 1705023456789
**And** a random number 1234 is generated
**When** the system generates an order_no
**Then** the order_no follows format "PO5NO17050234567891234" (userId + timestamp + 4-digit random)
**And** the order_no is unique in the plan_orders table
**And** the random suffix prevents collision in high-concurrency scenarios

### Requirement: Price snapshots MUST preserve transaction details

Order records MUST snapshot plan pricing at purchase time to maintain accurate transaction history even if plan prices change later.

#### Scenario: Snapshot plan prices when creating order

**Given** a plan "monthly" has:
- price = 88.00
- original_price = 100.00
**When** a user creates an order for this plan
**Then** the order record stores:
- plan_price = 88.00 (snapshot)
- plan_original_price = 100.00 (snapshot)
- final_price = 88.00
**And** if the plan price changes to 90.00 later
**Then** the order still shows plan_price = 88.00

## MODIFIED Requirements

None - this is a new capability.

## REMOVED Requirements

None - this is a new capability.
