# Implementation Tasks

## Phase 1: Database Schema (0.5 day)

- [ ] Add `purchasable` column to `plans` table
  - [ ] Write migration script (up migration)
  - [ ] Write rollback script (down migration: DROP COLUMN purchasable)
  - [ ] Set default values: subscription/consumption=1, trial/enterprise=0
  - [ ] Test migration on staging database
  - [ ] Test rollback on staging database

- [ ] Create `plan_orders` table
  - [ ] Define complete schema with all fields
  - [ ] Add indexes (user_id, order_no, status, created_at, payment_trade_no)
  - [ ] Add delivery_retry_count field (default 0) for retry tracking
  - [ ] Write migration script (up migration)
  - [ ] Write rollback script (down migration: DROP TABLE plan_orders)
  - [ ] Test table creation
  - [ ] Test rollback on staging database

- [ ] Create Go model `PlanOrder` in `model/plan_order.go`
  - [ ] Define struct with GORM tags
  - [ ] Add JSON serialization tags
  - [ ] Include helper methods (GetOrderById, GetOrderByTradeNo, etc.)

## Phase 2: Backend Core (2 days)

### 2.1 Order Management (model/plan_order.go)
- [ ] Implement `CreatePlanOrder(userId, planId) (*PlanOrder, error)`
  - [ ] Validate plan exists and purchasable=1
  - [ ] Validate queue capacity (<10 active plans)
  - [ ] Calculate price (get Plan.Price, Plan.OriginalPrice)
  - [ ] Generate unique order_no (format: PO{userId}NO{timestamp}{4-digit-random})
  - [ ] Set status='pending', expired_at=now()+30min
  - [ ] Insert into database

- [ ] Implement `GetUserOrders(userId, page, pageSize) ([]PlanOrder, int, error)`
  - [ ] Query with pagination
  - [ ] Order by created_at DESC
  - [ ] Include JOIN with plans table for display_name

- [ ] Implement `GetOrderById(orderId) (*PlanOrder, error)`
- [ ] Implement `GetOrderByTradeNo(tradeNo) (*PlanOrder, error)`
- [ ] Implement `UpdateOrderStatus(orderId, status, tx) error`

### 2.2 Payment Integration (controller/plan_purchase.go)
- [ ] Create `InitiateEpayPayment(c *gin.Context)` handler
  - [ ] Load order by ID
  - [ ] Validate order status='pending' and not expired
  - [ ] Call Epay API (reuse code from topup.go)
  - [ ] Update order.payment_method, payment_trade_no
  - [ ] Return payment URL/form

- [ ] Create `HandleEpayCallback(c *gin.Context)` handler
  - [ ] Verify Epay signature
  - [ ] Verify payment amount matches order.final_price
  - [ ] Lock order using sync.Map
  - [ ] Begin database transaction
  - [ ] Load order with SELECT FOR UPDATE
  - [ ] Idempotency check: if status != 'pending', return "success"
  - [ ] Update status='paid', paid_at=now()
  - [ ] Call DeliverPlan() synchronously
  - [ ] Commit transaction
  - [ ] Return "success" plain text

- [ ] Implement `VerifyEpaySignature(params) bool`
  - [ ] Reuse from existing topup.go implementation

### 2.3 Plan Delivery Service (service/plan_delivery.go)
- [ ] Create `DeliverPlan(orderId, tx) error` function
  - [ ] Load order (verify status='paid')
  - [ ] Idempotency check: if user_plan_id > 0, return nil
  - [ ] Load plan details
  - [ ] Create UserPlan instance
    - [ ] Set source='purchase', source_order_id=order_no
    - [ ] Snapshot all plan fields
    - [ ] Set quota = plan.default_quota
    - [ ] Determine queue position (check current plan exists)
    - [ ] Insert into user_plans table
  - [ ] Update order status='delivered', user_plan_id, delivered_at
  - [ ] Return success

- [ ] Create `ValidateQueueCapacity(userId) error`
  - [ ] Count active user plans
  - [ ] Return error if >= 10

### 2.4 API Controllers (controller/plan_purchase.go)
- [ ] `CreateOrder(c *gin.Context)` - POST /api/plan/purchase/create
  - [ ] Parse request: { plan_id }
  - [ ] Call model.CreatePlanOrder()
  - [ ] Return order details

- [ ] `PayOrder(c *gin.Context)` - POST /api/plan/purchase/pay
  - [ ] Parse request: { order_id, payment_method }
  - [ ] Call InitiateEpayPayment()
  - [ ] Return payment URL

- [ ] `GetMyOrders(c *gin.Context)` - GET /api/plan/purchase/my-orders
  - [ ] Get user ID from session
  - [ ] Parse pagination params
  - [ ] Call model.GetUserOrders()
  - [ ] Return order list

- [ ] Update `GetEnabledPlans(c *gin.Context)` in controller/plan.go
  - [ ] Add optional query param purchasable=true
  - [ ] Filter plans by purchasable column

### 2.5 Admin API Controllers (controller/admin_plan_order.go)
- [ ] `GetAllOrders(c *gin.Context)` - GET /api/admin/plan-orders
  - [ ] Verify admin permission
  - [ ] Parse query params: page, page_size, status, user_id, order_no
  - [ ] Query with filters and pagination
  - [ ] JOIN with users and plans tables
  - [ ] Return order list with user info

- [ ] `ManualCompleteOrder(c *gin.Context)` - POST /api/admin/plan-orders/:id/complete
  - [ ] Verify admin permission
  - [ ] Load order
  - [ ] Validate status (must be 'paid' or 'pending')
  - [ ] If status='pending', update to 'paid'
  - [ ] Call DeliverPlan()
  - [ ] Log admin operation (action, order_id, operator_id, timestamp)
  - [ ] Return success

### 2.6 Admin Operation Logging (model/admin_log.go or reuse existing)
- [ ] Implement `LogAdminOperation(action, orderId, operatorId, operatorUsername)` function
  - [ ] Check if admin_logs table exists, or reuse existing log system
  - [ ] Store: action="manual_order_completion", order_id, operator_id, operator_username, timestamp
  - [ ] Ensure logs are persisted for audit purposes

### 2.7 Route Registration (router/router.go)
- [ ] Register user routes
  - [ ] POST /api/plan/purchase/create → CreateOrder
  - [ ] POST /api/plan/purchase/pay → PayOrder
  - [ ] GET /api/plan/purchase/my-orders → GetMyOrders
  - [ ] GET /api/plan/purchase/epay/notify → HandleEpayCallback (no auth)

- [ ] Register admin routes
  - [ ] GET /api/admin/plan-orders → GetAllOrders
  - [ ] POST /api/admin/plan-orders/:id/complete → ManualCompleteOrder

## Phase 3: Background Tasks (0.5 day)

- [ ] Create scheduled task runner in `service/scheduled_tasks.go`
  - [ ] ExpireOldOrders() - run every 5 minutes
    - [ ] Find orders: status='pending', created_at < now()-30min
    - [ ] Update status='expired'
  - [ ] RetryFailedDeliveries() - run every 1 minute
    - [ ] Find orders: status='paid', paid_at < now()-5min, user_plan_id=0, delivery_retry_count<3
    - [ ] Increment delivery_retry_count for each order
    - [ ] Call DeliverPlan() for each
    - [ ] Log errors for manual intervention
    - [ ] Send admin notification after 3rd failed retry

- [ ] Register tasks in main.go initialization
  - [ ] Start goroutines for both tasks
  - [ ] Use time.Ticker for periodic execution

## Phase 4: Frontend Purchase Flow (1.5 days)

### 4.1 Plan Pricing Page Updates (web/src/pages/PlanPricing/index.jsx)
- [ ] Update API call to filter purchasable plans
  - [ ] Change GET /api/plan/enabled?purchasable=true

- [ ] Update plan card to show discount
  - [ ] Display OriginalPrice with strikethrough if > Price
  - [ ] Display Price prominently
  - [ ] Show "立省¥X" badge
  - [ ] Calculate discount percentage

- [ ] Update "立即购买" button handler
  - [ ] Call POST /api/plan/purchase/create
  - [ ] Navigate to order confirmation page

### 4.2 Order Confirmation Page (web/src/pages/OrderConfirmation/index.jsx)
- [ ] Create new page component
- [ ] Display order details
  - [ ] Plan name, description
  - [ ] Original price (strikethrough)
  - [ ] Final price (large, bold)
  - [ ] Expiration countdown (30 minutes)
- [ ] Payment method selection
  - [ ] Radio buttons: Alipay, WeChat
- [ ] "确认支付" button
  - [ ] Call POST /api/plan/purchase/pay
  - [ ] Redirect to payment gateway

- [ ] Add route in router
  - [ ] /console/order-confirmation/:order_id

### 4.3 User Order History Page (web/src/pages/MyOrders/index.jsx)
- [ ] Create new page component
- [ ] Fetch orders: GET /api/plan/purchase/my-orders
- [ ] Display order list table
  - [ ] Columns: 订单号, 套餐, 金额, 状态, 创建时间, 支付时间
  - [ ] Status badges with colors (pending=yellow, delivered=green, expired=gray)
- [ ] Pagination controls
- [ ] Add navigation link in sidebar

## Phase 5: Admin Order Management (1.5 days)

### 5.1 Admin Order List Page (web/src/pages/AdminOrders/index.jsx)
- [ ] Create admin page component
- [ ] Fetch orders: GET /api/admin/plan-orders
- [ ] Search and filter controls
  - [ ] Input: 用户ID, 订单号
  - [ ] Dropdown: 状态筛选
  - [ ] Date range picker (optional)
- [ ] Display order table
  - [ ] Columns: 订单号, 用户, 套餐, 金额, 状态, 创建时间, 操作
  - [ ] Actions: 查看详情, 手动完成 (for paid orders)
- [ ] Pagination controls
- [ ] Add to admin sidebar navigation

### 5.2 Manual Order Completion
- [ ] Add "手动完成" button for orders in 'paid' status
- [ ] Confirmation modal: "确认手动完成该订单并发放套餐？"
- [ ] Call POST /api/admin/plan-orders/:id/complete
- [ ] Show success/error toast
- [ ] Refresh order list

## Phase 6: Testing and Bug Fixes (1 day)

### 6.1 Backend Testing
- [ ] Test order creation with valid plan
- [ ] Test order creation with invalid plan (should fail)
- [ ] Test order creation when queue is full (should fail)
- [ ] Test payment initiation
- [ ] Test payment callback with valid signature
- [ ] Test payment callback idempotency (duplicate calls)
- [ ] Test plan delivery success
- [ ] Test plan delivery idempotency
- [ ] Test order expiration task
- [ ] Test delivery retry task

### 6.2 Frontend Testing
- [ ] Test plan pricing page displays correctly
- [ ] Test order creation flow
- [ ] Test payment redirection
- [ ] Test order history displays correctly
- [ ] Test admin order list with filters
- [ ] Test manual order completion

### 6.3 Integration Testing
- [ ] End-to-end: Create order → Pay → Deliver → Verify UserPlan created
- [ ] Test concurrent payment callbacks (verify no duplicate deliveries)
- [ ] Test order expiration (wait 30min or mock time)
- [ ] Test queue full scenario (buy 10 plans, try 11th)

### 6.4 Edge Cases
- [ ] Payment callback arrives before order is fully created (should retry)
- [ ] Plan is deleted after order created but before payment (handle gracefully)
- [ ] User has exactly 10 plans, tries to buy 11th (should fail)
- [ ] Order expires while user is on payment page (show error on return)

## Phase 7: Documentation and Deployment (included in other phases)

- [ ] Update API documentation with new endpoints
- [ ] Add user guide: "如何购买套餐"
- [ ] Add admin guide: "订单管理说明"
- [ ] Update CHANGELOG.md
- [ ] Create deployment checklist
  - [ ] Run database migrations
  - [ ] Verify Epay configuration
  - [ ] Test payment callback URL is accessible
  - [ ] Monitor logs during first orders

## Validation Checklist

Before marking this change as complete:

- [ ] All database migrations run successfully
- [ ] All API endpoints return expected responses
- [ ] Payment callback processes correctly
- [ ] Plans are delivered immediately after payment
- [ ] Orders expire after 30 minutes
- [ ] Queue capacity validation works
- [ ] Admin can view and manage orders
- [ ] Frontend displays orders correctly
- [ ] No duplicate plan deliveries occur
- [ ] Error messages are user-friendly
- [ ] All tests pass

## Rollback Plan

If issues are discovered after deployment:

1. **Database Rollback**:
   - Option A (Soft rollback): Keep `plan_orders` table but disable order creation via feature flag
   - Option B (Hard rollback): Run down migration scripts to revert database changes
     - Run: `DROP TABLE plan_orders`
     - Run: `ALTER TABLE plans DROP COLUMN purchasable`
   - Note: Only use hard rollback if no orders have been created yet

2. **Feature Flag**:
   - Add environment variable `ENABLE_PLAN_PURCHASE=false`
   - Hide purchase buttons in frontend
   - Return error for order creation API

3. **Manual Intervention**:
   - Process pending orders manually
   - Refund stuck payments (if any)
   - Communicate with affected users
