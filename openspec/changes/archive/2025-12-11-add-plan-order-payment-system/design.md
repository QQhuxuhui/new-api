# Design: Plan Order Payment System

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        Frontend                              │
├─────────────────────────────────────────────────────────────┤
│  Plan Pricing Page → Order Confirmation → Payment Selection │
│         ↓                    ↓                      ↓        │
│  Display Plans        Create Order         Pay with Epay    │
└─────────────────────────────────────────────────────────────┘
                              │
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                      Backend APIs                            │
├─────────────────────────────────────────────────────────────┤
│  GET /api/plan/enabled?purchasable=true                     │
│  POST /api/plan/purchase/create   (create order)            │
│  POST /api/plan/purchase/pay      (initiate payment)        │
│  GET /api/plan/purchase/epay/notify (payment callback)      │
│  GET /api/plan/purchase/my-orders                           │
│  GET /api/admin/plan-orders       (admin list)              │
│  POST /api/admin/plan-orders/:id/complete (manual delivery) │
└─────────────────────────────────────────────────────────────┘
                              │
                              ↓
┌──────────────────────────────────────────────────────��──────┐
│                      Core Services                           │
├─────────────────────────────────────────────────────────────┤
│  PlanOrderService                                            │
│  ├─ CreateOrder() - validate, calculate price, create       │
│  ├─ CalculatePrice() - apply discounts, snapshot            │
│  └─ GetUserOrders() - query with filters                    │
│                                                              │
│  PaymentService                                              │
│  ├─ InitiateEpayPayment() - call Epay API                  │
│  ├─ HandleEpayCallback() - verify, update order             │
│  └─ VerifyEpaySignature() - security check                  │
│                                                              │
│  PlanDeliveryService                                         │
│  ├─ DeliverPlan() - create UserPlan, update order           │
│  ├─ ValidateDelivery() - check queue, validate              │
│  └─ RetryFailedDeliveries() - background task               │
└─────────────────────────────────────────────────────────────┘
                              │
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                      Data Layer                              │
├─────────────────────────────────────────────────────────────┤
│  plan_orders table (new)                                     │
│  ├─ Order management (status, pricing, timestamps)          │
│  └─ Foreign keys: user_id, plan_id, user_plan_id           │
│                                                              │
│  plans table (modified)                                      │
│  └─ + purchasable column                                    │
│                                                              │
│  user_plans table (existing, reused)                         │
│  └─ Plan instances with queue management                    │
└─────────────────────────────────────────────────────────────┘
                              │
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                   External Services                          │
├─────────────────────────────────────────────────────────────┤
│  Epay Payment Gateway                                        │
│  ├─ Payment request API                                     │
│  └─ Payment callback webhook                                │
└─────────────────────────────────────────────────────────────┘
```

## Database Schema

### New Table: `plan_orders`

```sql
CREATE TABLE plan_orders (
  id INT AUTO_INCREMENT PRIMARY KEY,
  order_no VARCHAR(64) UNIQUE NOT NULL COMMENT 'Unique order number',
  user_id INT NOT NULL COMMENT 'Buyer user ID',
  plan_id VARCHAR(255) NOT NULL COMMENT 'Plan identifier',

  -- Price snapshot (preserve at purchase time)
  plan_price DECIMAL(10,2) NOT NULL COMMENT 'Actual sale price',
  plan_original_price DECIMAL(10,2) DEFAULT 0 COMMENT 'Original price before discount',
  final_price DECIMAL(10,2) NOT NULL COMMENT 'Final payment amount',

  -- Payment information
  payment_method VARCHAR(50) COMMENT 'alipay, wechat, stripe, creem',
  payment_trade_no VARCHAR(255) COMMENT 'Payment gateway transaction ID',

  -- Status management
  status VARCHAR(20) DEFAULT 'pending' COMMENT 'pending, paid, delivered, expired, cancelled',

  -- Timestamps (milliseconds)
  created_at BIGINT NOT NULL COMMENT 'Order creation time',
  expired_at BIGINT COMMENT 'Expiration time for pending orders',
  paid_at BIGINT COMMENT 'Payment completion time',
  delivered_at BIGINT COMMENT 'Plan delivery time',
  cancelled_at BIGINT COMMENT 'Cancellation time',

  -- Relationships
  user_plan_id INT COMMENT 'Created UserPlan instance ID',

  -- Delivery retry tracking
  delivery_retry_count INT DEFAULT 0 COMMENT 'Number of delivery retry attempts',

  -- Indexes
  INDEX idx_user_id (user_id),
  INDEX idx_order_no (order_no),
  INDEX idx_status (status),
  INDEX idx_created_at (created_at),
  INDEX idx_payment_trade_no (payment_trade_no)
) COMMENT='Plan purchase orders';
```

### Modified Table: `plans`

```sql
ALTER TABLE plans ADD COLUMN purchasable TINYINT(1) DEFAULT 1
  COMMENT 'Can be purchased online: 1=yes, 0=no';

-- Set default values based on plan type
UPDATE plans SET purchasable = 1 WHERE type IN ('subscription', 'consumption');
UPDATE plans SET purchasable = 0 WHERE type IN ('trial', 'enterprise');
```

## Data Flow

### 1. Order Creation Flow

```
User clicks "Buy Now"
    ↓
Frontend: GET /api/plan/enabled?purchasable=true
    ↓
Display available plans with Price/OriginalPrice
    ↓
User selects plan → navigate to order confirmation
    ↓
Frontend: POST /api/plan/purchase/create
    body: { plan_id: "monthly" }
    ↓
Backend: CreateOrder()
    ├─ Validate: plan exists, purchasable, queue not full
    ├─ Calculate price: get Plan.Price, Plan.OriginalPrice
    ├─ Generate order_no: PO{userId}NO{timestamp}
    ├─ Create PlanOrder record (status='pending')
    └─ Return order details
    ↓
Frontend: Display order confirmation page
    ├─ Plan name, description
    ├─ ~~Original Price~~ Final Price
    └─ Payment method selection
```

### 2. Payment Flow

```
User selects payment method (Alipay/WeChat)
    ↓
Frontend: POST /api/plan/purchase/pay
    body: { order_id: 123, payment_method: "alipay" }
    ↓
Backend: InitiateEpayPayment()
    ├─ Load order (verify status='pending', not expired)
    ├─ Call Epay API
    │   ├─ amount = order.final_price
    │   ├─ out_trade_no = order.order_no
    │   ├─ notify_url = /api/plan/purchase/epay/notify
    │   └─ return_url = /console/orders/{order_id}
    ├─ Update order.payment_method, payment_trade_no
    └─ Return payment URL/form
    ↓
User redirected to Epay gateway → completes payment
    ↓
Epay callback: GET /api/plan/purchase/epay/notify?trade_no=xxx&...
    ↓
Backend: HandleEpayCallback()
    ├─ Verify signature
    ├─ Lock order (sync.Map)
    ├─ Begin database transaction
    │   ├─ SELECT * FROM plan_orders WHERE order_no=? FOR UPDATE
    │   ├─ Idempotency check: if status != 'pending', return success
    │   ├─ Update status='paid', paid_at=now()
    │   ├─ Call DeliverPlan(order_id) synchronously
    │   └─ Commit transaction
    └─ Return "success" to Epay
```

### 3. Plan Delivery Flow

```
DeliverPlan(order_id)
    ├─ Load order (must be status='paid')
    ├─ Idempotency check: if user_plan_id > 0, already delivered
    ├─ Load plan details (Plan.GetById)
    ├─ Validate queue capacity
    │   ├─ Count active user plans
    │   └─ If >= 10, log error (should not happen, validated earlier)
    ├─ Create UserPlan instance
    │   ├─ user_id = order.user_id
    │   ├─ plan_id = order.plan_id
    │   ├─ source = 'purchase'
    │   ├─ source_order_id = order.order_no
    │   ├─ Snapshot plan fields (name, type, category, etc.)
    │   ├─ Set quota = plan.default_quota
    │   ├─ Determine queue position
    │   │   ├─ If no current plan: is_current=1, queue_position=0, status='active'
    │   │   └─ Else: is_current=0, queue_position=MAX+1, status='active'
    │   └─ Insert into user_plans table
    ├─ Update order
    │   ├─ status = 'delivered'
    │   ├─ user_plan_id = new_plan.id
    │   └─ delivered_at = now()
    └─ Return success
```

## API Specifications

### User APIs

#### 1. Get Purchasable Plans
```
GET /api/plan/enabled?purchasable=true

Response:
{
  "success": true,
  "data": [
    {
      "id": 1,
      "name": "monthly",
      "display_name": "月卡套餐",
      "price": 88.00,
      "original_price": 100.00,
      "default_quota": 1000000,
      "validity_days": 30,
      "purchasable": true,
      ...
    }
  ]
}
```

#### 2. Create Order
```
POST /api/plan/purchase/create
Content-Type: application/json

Request:
{
  "plan_id": "monthly"
}

Response:
{
  "success": true,
  "data": {
    "order_id": 123,
    "order_no": "PO5NO1705023456789",
    "plan_name": "月卡套餐",
    "plan_price": 88.00,
    "original_price": 100.00,
    "final_price": 88.00,
    "status": "pending",
    "expired_at": 1705025256789
  }
}

Errors:
- 400: "套餐不存在" / "套餐不可购买" / "队列已满"
```

#### 3. Pay Order
```
POST /api/plan/purchase/pay
Content-Type: application/json

Request:
{
  "order_id": 123,
  "payment_method": "alipay"
}

Response:
{
  "success": true,
  "data": {
    "payment_url": "https://epay.example.com/...",
    "payment_form": {...}  // HTML form params
  }
}

Errors:
- 400: "订单不存在" / "订单已过期" / "订单状态异常"
```

#### 4. Get My Orders
```
GET /api/plan/purchase/my-orders?page=1&page_size=20

Response:
{
  "success": true,
  "data": {
    "orders": [
      {
        "order_id": 123,
        "order_no": "PO5NO1705023456789",
        "plan_name": "月卡套餐",
        "final_price": 88.00,
        "status": "delivered",
        "created_at": 1705023456789,
        "paid_at": 1705023556789,
        "delivered_at": 1705023557000
      }
    ],
    "total": 5,
    "page": 1,
    "page_size": 20
  }
}
```

### Admin APIs

#### 5. List All Orders
```
GET /api/admin/plan-orders?page=1&page_size=20&status=&user_id=&order_no=

Query Params:
- page: page number (default 1)
- page_size: items per page (default 20)
- status: filter by status (optional)
- user_id: filter by user (optional)
- order_no: search by order number (optional)

Response: (same structure as user orders, with user info)
{
  "success": true,
  "data": {
    "orders": [
      {
        "order_id": 123,
        "user_id": 5,
        "username": "test@example.com",
        "plan_name": "月卡套餐",
        "final_price": 88.00,
        "status": "delivered",
        ...
      }
    ],
    "total": 150,
    "page": 1,
    "page_size": 20
  }
}
```

#### 6. Manual Order Completion
```
POST /api/admin/plan-orders/:id/complete
Content-Type: application/json

Description: Manually mark order as paid and deliver plan (for failed deliveries)

Response:
{
  "success": true,
  "message": "订单已手动完成"
}

Errors:
- 400: "订单不存在" / "订单状态不允许操作"
```

### Payment Callback

#### 7. Epay Notify Callback
```
GET /api/plan/purchase/epay/notify?trade_no=xxx&out_trade_no=PO5NO...&money=88.00&sign=...

Query Params: (from Epay gateway)
- trade_no: Epay transaction ID
- out_trade_no: our order_no
- money: payment amount
- sign: signature for verification

Response: (plain text)
"success" or "fail"

Internal Logic:
1. Verify signature
2. Lock order by order_no
3. Load order with SELECT FOR UPDATE
4. Verify payment amount matches order.final_price
5. If status != 'pending', return "success" (idempotency)
6. Update status='paid', paid_at=now()
7. Call DeliverPlan() synchronously
8. If delivery succeeds, update status='delivered'
9. Commit transaction
10. Return "success"
```

## State Machine

### Order Status Transitions

```
[pending] ─────────────────────────────────────┐
    │                                           │
    ├─ User pays ──→ [paid] ──→ [delivered]   │
    │                   │                       │
    ├─ 30min timeout ──────────────────────────┼──→ [expired]
    │                                           │
    └─ User cancels ────────────────────────────┘
                                                │
                                                ↓
                                          [cancelled]

Final States: delivered, expired, cancelled
Transient States: pending, paid
```

### State Transitions

| From | Event | To | Action |
|------|-------|-------|--------|
| pending | User pays | paid | Update paid_at, call DeliverPlan |
| paid | Delivery success | delivered | Update user_plan_id, delivered_at |
| pending | 30min timeout | expired | Background task updates |
| pending | User cancels | cancelled | Update cancelled_at |

## Idempotency Guarantees

### Payment Callback
```go
func HandleEpayCallback(tradeNo string) error {
    // 1. Order-level lock
    lock := getOrCreateOrderLock(tradeNo)
    lock.Lock()
    defer lock.Unlock()

    // 2. Database transaction + row lock
    tx := db.Begin()
    defer tx.Rollback()

    order := GetOrderByTradeNo(tradeNo, tx)  // SELECT ... FOR UPDATE

    // 3. Verify payment amount matches order
    if callbackAmount != order.FinalPrice {
        log.Error("Payment amount mismatch", "expected", order.FinalPrice, "got", callbackAmount)
        tx.Rollback()
        return errors.New("payment amount mismatch")
    }

    // 4. Idempotency check
    if order.Status != "pending" {
        tx.Commit()
        return nil  // Already processed
    }

    // 5. Update and deliver
    order.Status = "paid"
    order.PaidAt = now()
    UpdateOrder(order, tx)

    // 6. Synchronous delivery
    err := DeliverPlan(order.Id, tx)
    if err != nil {
        // Transaction rolls back, will retry later
        return err
    }

    tx.Commit()
    return nil
}
```

### Plan Delivery
```go
func DeliverPlan(orderId int, tx *gorm.DB) error {
    order := GetOrderById(orderId, tx)

    // Idempotency check
    if order.UserPlanId > 0 {
        return nil  // Already delivered
    }

    // Create UserPlan...
    userPlan := CreateUserPlan(order, tx)

    // Update order
    order.Status = "delivered"
    order.UserPlanId = userPlan.Id
    order.DeliveredAt = now()
    UpdateOrder(order, tx)

    return nil
}
```

## Background Tasks

### 1. Order Expiration Task
```go
// Run every 5 minutes
func ExpireOldOrders() {
    cutoff := now() - 30*60*1000  // 30 minutes ago

    orders := db.Where("status = ? AND created_at < ?", "pending", cutoff).Find()

    for _, order := range orders {
        order.Status = "expired"
        db.Save(&order)
    }
}
```

### 2. Delivery Retry Task
```go
// Run every 1 minute
func RetryFailedDeliveries() {
    cutoff := now() - 5*60*1000  // 5 minutes ago
    maxRetries := 3

    // Find paid orders that were not delivered
    orders := db.Where(
        "status = ? AND paid_at < ? AND user_plan_id = 0 AND delivery_retry_count < ?",
        "paid",
        cutoff,
        maxRetries,
    ).Find()

    for _, order := range orders {
        // Increment retry count first
        order.DeliveryRetryCount++
        db.Save(&order)

        err := DeliverPlan(order.Id, nil)
        if err != nil {
            log.Error("Delivery retry failed", order.Id, order.DeliveryRetryCount, err)

            // If max retries reached, send admin notification
            if order.DeliveryRetryCount >= maxRetries {
                SendAdminNotification("Delivery failed after max retries", order)
            }
        }
    }
}
```

**Note**: After 3 failed attempts, the order remains in `paid` status and requires manual admin intervention via the admin order completion endpoint.

## Security Considerations

### 1. Payment Signature Verification
```go
func VerifyEpaySignature(params map[string]string) bool {
    sign := params["sign"]
    delete(params, "sign")

    // Sort params and concatenate
    keys := sortKeys(params)
    data := ""
    for _, key := range keys {
        data += key + "=" + params[key] + "&"
    }
    data += "key=" + EPAY_KEY

    expectedSign := md5(data)
    return sign == expectedSign
}
```

### 2. Order Number Generation
```go
func GenerateOrderNo(userId int) string {
    timestamp := time.Now().UnixMilli()
    // Add 4-digit random number to prevent collision in high-concurrency scenarios
    random := rand.Intn(10000)
    return fmt.Sprintf("PO%dNO%d%04d", userId, timestamp, random)
}
```

**Rationale**: Using timestamp + random suffix prevents order number collision when the same user creates multiple orders within the same millisecond.

### 3. Queue Validation
```go
func ValidateQueueCapacity(userId int) error {
    count := db.Model(&UserPlan{}).Where(
        "user_id = ? AND status IN (?)",
        userId,
        []string{"active"},
    ).Count()

    if count >= 10 {
        return errors.New("队列已满，最多拥有10个套餐")
    }

    return nil
}
```

**Dual-Check Strategy**: Queue validation is performed at TWO checkpoints:

1. **Order Creation (Preventive)**: Before creating the order, validate that the user has < 10 active plans. This prevents users from creating orders they cannot fulfill.

2. **Plan Delivery (Final Guard)**: Before delivering the plan, validate again. This handles edge cases where multiple orders are created/paid concurrently.

**Edge Case Handling**: If validation fails at delivery time (queue became full between order creation and payment):
- The order remains in `paid` status
- Automatic delivery retries are attempted (up to 3 times)
- If queue space opens up, delivery succeeds automatically
- Otherwise, admin manually handles via order completion endpoint
- User is notified to clear queue space or contact support

## Error Handling

### Frontend Error Messages

| Error Code | Message | User Action |
|------------|---------|-------------|
| PLAN_NOT_FOUND | 套餐不存在 | Contact support |
| PLAN_NOT_PURCHASABLE | 该套餐不支持在线购买 | Choose another plan |
| QUEUE_FULL | 您已拥有10个套餐，无法继续购买 | Wait for current plans to complete |
| ORDER_EXPIRED | 订单已过期，请重新购买 | Create new order |
| PAYMENT_FAILED | 支付失败，请重试 | Retry payment |
| INSUFFICIENT_FUNDS | 支付金额不足 | Check payment method |

### Retry Strategy

- **Payment callback failures**: Epay will retry automatically (3 times, exponential backoff)
- **Delivery failures**: Background task retries every minute (max 3 attempts)
- **After 3 failed deliveries**: Admin notification + manual intervention

## Performance Considerations

### Database Indexes
- `idx_user_id`: Fast lookup of user orders
- `idx_order_no`: Fast lookup by order number (callback)
- `idx_status`: Filter orders by status
- `idx_created_at`: Expiration task queries
- `idx_payment_trade_no`: Payment provider reconciliation

### Query Optimization
```sql
-- Order list query (with pagination)
SELECT o.*, u.username, p.display_name
FROM plan_orders o
LEFT JOIN users u ON o.user_id = u.id
LEFT JOIN plans p ON o.plan_id = p.name
WHERE o.status = ? AND o.user_id = ?
ORDER BY o.created_at DESC
LIMIT ? OFFSET ?;
```

### Caching Strategy
- Plans data: Cache in Redis (TTL: 5 minutes)
- User order counts: Cache temporarily during queue validation
- No caching for order status (must be real-time)

## Testing Strategy

### Unit Tests
- Price calculation logic
- Order number generation
- Signature verification
- Queue capacity validation

### Integration Tests
- Order creation → payment → delivery flow
- Payment callback idempotency
- Expiration task execution
- Delivery retry mechanism

### Manual Testing Checklist
- [ ] Create order for subscription plan
- [ ] Create order for consumption plan
- [ ] Try purchasing when queue is full (should fail)
- [ ] Complete payment and verify plan delivery
- [ ] Test order expiration (mock time)
- [ ] Test payment callback retry (duplicate calls)
- [ ] Admin: view order list, search, manual completion
- [ ] User: view order history

## Monitoring and Logging

### Key Metrics
- Order creation rate
- Payment success rate
- Delivery success rate
- Average order completion time
- Queue capacity utilization

### Logging Points
```go
log.Info("Order created", order.Id, order.OrderNo, order.PlanId)
log.Info("Payment initiated", order.Id, paymentMethod)
log.Info("Payment callback received", tradeNo, status)
log.Info("Plan delivered", order.Id, userPlanId)
log.Error("Delivery failed", order.Id, error)
```

### Admin Operation Audit Logging
All administrative actions on orders MUST be logged for compliance and accountability:

```go
func LogAdminOperation(action string, orderId int, operatorId int, operatorUsername string) {
    // Store in admin_logs table or existing audit system
    log := AdminLog{
        Action:           action,
        ResourceType:     "plan_order",
        ResourceId:       orderId,
        OperatorId:       operatorId,
        OperatorUsername: operatorUsername,
        Timestamp:        now(),
    }
    db.Create(&log)
}

// Usage in ManualCompleteOrder
LogAdminOperation("manual_order_completion", order.Id, currentUser.Id, currentUser.Username)
```

**Logged Actions:**
- `manual_order_completion` - Admin manually completes an order
- `order_status_override` - Admin changes order status (if implemented)
- `order_cancellation` - Admin cancels an order (if implemented)

## Migration Plan

### Phase 1: Database Schema
1. Add `purchasable` column to `plans` table
2. Create `plan_orders` table
3. Run data migration script (set default purchasable values)

### Phase 2: Backend Implementation
1. Implement order creation and payment APIs
2. Implement Epay callback handling
3. Implement plan delivery service
4. Implement background tasks

### Phase 3: Frontend Implementation
1. Update plan pricing page (show purchasable plans only)
2. Create order confirmation page
3. Create user order history page
4. Create admin order management page

### Phase 4: Testing and Rollout
1. Test in staging environment
2. Soft launch (enable for subset of users)
3. Monitor metrics and fix issues
4. Full rollout

## Future Enhancements (Phase 2)

1. **Multiple Payment Methods**: Stripe, Creem integration
2. **Refund System**: Partial refunds based on usage
3. **Coupon System**: Discount codes, promotional campaigns
4. **Subscription Auto-Renewal**: Automatic plan renewal before expiration
5. **Order Analytics**: Dashboard with sales metrics
6. **Invoice Generation**: PDF invoices for enterprise customers
7. **Notification System**: Email/SMS order confirmations
