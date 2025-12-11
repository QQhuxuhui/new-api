# Admin Order Management

## ADDED Requirements

### Requirement: Administrators MUST be able to view all plan orders

Administrators MUST have access to a comprehensive order list with search, filtering, and pagination capabilities for monitoring and support.

#### Scenario: List all orders with pagination

**Given** an administrator is logged in
**And** the system has 150 total orders
**When** the admin calls GET /api/admin/plan-orders?page=1&page_size=20
**Then** the system returns:
- orders: array of 20 orders ordered by created_at DESC
- total: 150
- page: 1
- page_size: 20
**And** each order includes:
- order_id, order_no, user_id, username, plan_name
- plan_price, final_price, status
- created_at, paid_at, delivered_at
- payment_method, payment_trade_no

#### Scenario: Filter orders by status

**Given** an administrator is logged in
**And** orders exist with various statuses: pending (5), paid (2), delivered (30), expired (10)
**When** the admin calls GET /api/admin/plan-orders?status=delivered
**Then** the system returns only the 30 orders with status="delivered"
**And** total count reflects filtered results

#### Scenario: Search orders by user ID

**Given** an administrator is logged in
**And** user_id=5 has 12 orders
**When** the admin calls GET /api/admin/plan-orders?user_id=5
**Then** the system returns only the 12 orders for user_id=5
**And** orders include username for display

#### Scenario: Search orders by order number

**Given** an administrator is logged in
**And** an order exists with order_no="PO5NO1705023456789"
**When** the admin calls GET /api/admin/plan-orders?order_no=PO5NO1705023456789
**Then** the system returns exactly 1 order matching the order_no
**And** displays full order details

### Requirement: Administrators MUST be able to manually complete orders

Administrators MUST be able to manually mark orders as paid and deliver plans for failed automatic deliveries or special circumstances.

#### Scenario: Manually complete paid order with failed delivery

**Given** an administrator is logged in
**And** an order exists with:
- order_id = 123
- status = "paid"
- paid_at = 10 minutes ago
- user_plan_id = 0 (delivery failed)
**When** the admin calls POST /api/admin/plan-orders/123/complete
**Then** the system:
- Calls DeliverPlan(123)
- Creates UserPlan instance
- Updates order status = "delivered"
- Updates order.user_plan_id and delivered_at
**And** returns { success: true, message: "订单已手动完成" }

#### Scenario: Manually complete pending order (mark as paid + deliver)

**Given** an administrator is logged in
**And** an order exists with:
- order_id = 456
- status = "pending"
- (payment received offline, needs manual confirmation)
**When** the admin calls POST /api/admin/plan-orders/456/complete
**Then** the system:
- Updates order status = "paid", paid_at = current time
- Calls DeliverPlan(456)
- Updates order status = "delivered"
**And** returns success

#### Scenario: Reject manual completion for already delivered order

**Given** an administrator is logged in
**And** an order exists with:
- order_id = 789
- status = "delivered"
- user_plan_id = 1001
**When** the admin calls POST /api/admin/plan-orders/789/complete
**Then** the system returns error 400 "订单已完成，无需重复操作"
**And** no changes occur

### Requirement: Order list MUST display related user and plan information

Order lists MUST join user and plan data to provide context for administrators without requiring additional queries.

#### Scenario: Display username and plan display name in order list

**Given** an order exists with:
- order_id = 123
- user_id = 5
- plan_id = "monthly"
**And** user_id=5 has username="user@example.com"
**And** plan "monthly" has display_name="月卡套餐"
**When** the admin retrieves the order list
**Then** each order in the response includes:
- username = "user@example.com"
- plan_display_name = "月卡套餐"
**And** administrators can identify orders without looking up IDs separately

### Requirement: Admin order operations MUST be logged for audit

All administrative actions on orders MUST be logged with operator ID and timestamp for compliance and accountability.

#### Scenario: Log manual order completion

**Given** an administrator with user_id=1 (username="admin")
**And** an order with order_id=123
**When** the admin manually completes the order
**Then** the system logs:
- action = "manual_order_completion"
- order_id = 123
- operator_id = 1
- operator_username = "admin"
- timestamp = current time
**And** the log is persisted for future audit

### Requirement: Admin interface MUST enforce permission checks

Order management endpoints MUST verify administrator privileges before allowing access to sensitive operations.

#### Scenario: Block non-admin user from accessing order list

**Given** a non-admin user is logged in (role != "admin")
**When** the user calls GET /api/admin/plan-orders
**Then** the system returns error 403 "权限不足"
**And** no order data is exposed

#### Scenario: Block non-admin user from manual completion

**Given** a non-admin user is logged in
**When** the user calls POST /api/admin/plan-orders/123/complete
**Then** the system returns error 403 "权限不足"
**And** the order is not modified

## MODIFIED Requirements

None - this is a new capability.

## REMOVED Requirements

None - this is a new capability.
