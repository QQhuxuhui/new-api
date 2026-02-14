# 钱包充值账单与订阅套餐账单实现分析

## 概述

本项目实现了两套独立的订单系统：
1. **钱包充值订单** (Topup Orders) - 按量付费充值
2. **订阅套餐订单** (Plan Orders) - 套餐购买订单

两套系统使用不同的数据表、API端点和前端页面，但共享相同的支付网关（Epay、Stripe、Creem）。

---

## 一、数据库模型/表结构

### 1.1 钱包充值订单

#### 旧版充值表：`topups`
**文件位置**: `/model/topup.go`

```go
type TopUp struct {
    Id            int     `json:"id"`
    UserId        int     `json:"user_id" gorm:"index"`
    Amount        int64   `json:"amount"`           // 充值额度（内部单位）
    Money         float64 `json:"money"`            // 支付金额（人民币）
    TradeNo       string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
    PaymentMethod string  `json:"payment_method" gorm:"type:varchar(50)"`
    CreateTime    int64   `json:"create_time"`
    CompleteTime  int64   `json:"complete_time"`
    Status        string  `json:"status"`           // success, pending
}
```

**用途**: 历史兼容，用于记录充值历史

#### 新版充值订单表：`topup_orders`
**文件位置**: `/model/topup_order.go`

```go
type TopupOrder struct {
    Id      int    `json:"id" gorm:"primaryKey;autoIncrement"`
    OrderNo string `json:"order_no" gorm:"type:varchar(64);uniqueIndex;not null"`

    UserId int `json:"user_id" gorm:"not null;index"`

    // 充值金额信息
    Amount        float64 `json:"amount" gorm:"type:decimal(10,2);not null"`        // 充值金额（USD）
    Quota         int64   `json:"quota" gorm:"not null"`                            // 充值额度（内部单位）
    OriginalPrice float64 `json:"original_price" gorm:"type:decimal(10,2);not null"` // 原价
    FinalPrice    float64 `json:"final_price" gorm:"type:decimal(10,2);not null"`   // 实付金额
    DiscountRate  float64 `json:"discount_rate" gorm:"type:decimal(5,4);default:1"` // 折扣率

    // 支付信息
    PaymentMethod  string `json:"payment_method" gorm:"type:varchar(50)"`         // alipay, wxpay, stripe, creem
    PaymentTradeNo string `json:"payment_trade_no" gorm:"type:varchar(255);index"`

    // 状态管理
    Status string `json:"status" gorm:"type:varchar(20);default:'pending';index"` // pending, paid, cancelled, expired

    // 时间戳（毫秒）
    CreatedAt   int64 `json:"created_at" gorm:"index;not null"`
    ExpiredAt   int64 `json:"expired_at"`      // 订单过期时间（30分钟）
    PaidAt      int64 `json:"paid_at"`
    CancelledAt int64 `json:"cancelled_at"`
}
```

**订单号格式**: `TO{userId}NO{timestamp}{4位随机数}`
**过期时间**: 30分钟

---

### 1.2 订阅套餐订单

#### 套餐订单表：`plan_orders`
**文件位置**: `/model/plan_order.go`

```go
type PlanOrder struct {
    Id      int    `json:"id" gorm:"primaryKey;autoIncrement"`
    OrderNo string `json:"order_no" gorm:"type:varchar(64);uniqueIndex;not null"`

    UserId int  `json:"user_id" gorm:"not null;index"`
    PlanId *int `json:"plan_id" gorm:"index"` // 可为空（套餐删除后）

    // 价格快照（购买时保存）
    PlanPrice         float64 `json:"plan_price" gorm:"type:decimal(10,2);not null"`
    PlanOriginalPrice float64 `json:"plan_original_price" gorm:"type:decimal(10,2);default:0"`
    FinalPrice        float64 `json:"final_price" gorm:"type:decimal(10,2);not null"`

    // 套餐信息快照
    PlanName         string `json:"plan_name" gorm:"type:varchar(255)"`
    PlanDisplayName  string `json:"plan_display_name" gorm:"type:varchar(255)"`
    PlanQuota        int64  `json:"plan_quota"`
    PlanValidityDays int    `json:"plan_validity_days"`
    PlanCategory     string `json:"plan_category" gorm:"type:varchar(20)"`  // daily/weekly/monthly/payg
    PlanType         string `json:"plan_type" gorm:"type:varchar(20)"`      // subscription/consumption/trial/enterprise

    // 支付信息
    PaymentMethod  string `json:"payment_method" gorm:"type:varchar(50)"`
    PaymentTradeNo string `json:"payment_trade_no" gorm:"type:varchar(255);index"`

    // 状态管理
    Status string `json:"status" gorm:"type:varchar(20);default:'pending';index"` // pending, paid, delivered, expired, cancelled

    // 时间戳（毫秒）
    CreatedAt   int64 `json:"created_at" gorm:"index;not null"`
    ExpiredAt   int64 `json:"expired_at"`
    PaidAt      int64 `json:"paid_at"`
    DeliveredAt int64 `json:"delivered_at"`    // 套餐发放时间
    CancelledAt int64 `json:"cancelled_at"`

    // 关联关系
    UserPlanId         *int `json:"user_plan_id"`                      // 已发放的套餐ID
    DeliveryRetryCount int  `json:"delivery_retry_count" gorm:"default:0"` // 发放重试次数
}
```

**订单号格式**: `PO{userId}NO{timestamp}{4位随机数}`
**过期时间**: 30分钟
**状态流转**: pending → paid → delivered

---

## 二、后端API端点

### 2.1 钱包充值订单API

**文件位置**: `/controller/topup_order.go`

| 端点 | 方法 | 认证 | 说明 |
|------|------|------|------|
| `/api/user/topup/order/create` | POST | 用户 | 创建充值订单 |
| `/api/user/topup/order/:id` | GET | 用户 | 获取订单详情 |
| `/api/user/topup/order/pay` | POST | 用户 | 发起支付 |
| `/api/user/topup/order/my-orders` | GET | 用户 | 获取我的充值订单列表 |
| `/api/user/topup/order/cancel` | POST | 用户 | 取消订单 |
| `/api/user/topup/order/epay/notify` | GET | 公开 | Epay支付回调 |

**旧版充值API（兼容）**:
- `/api/user/topup/self` - 获取充值历史（TopUp表）
- `/api/user/topup` - 管理员查看所有充值记录
- `/api/user/topup/complete` - 管理员手动补单

---

### 2.2 订阅套餐订单API

**文件位置**: `/controller/plan_purchase.go`

| 端点 | 方法 | 认证 | 说明 |
|------|------|------|------|
| `/api/user/plan/purchase/create` | POST | 用户 | 创建套餐订单 |
| `/api/user/plan/purchase/order/:id` | GET | 用户 | 获取订单详情 |
| `/api/user/plan/purchase/pay` | POST | 用户 | 发起支付 |
| `/api/user/plan/purchase/my-orders` | GET | 用户 | 获取我的订单列表（支持合并显示） |
| `/api/user/plan/purchase/cancel` | POST | 用户 | 取消订单 |
| `/api/plan/purchase/epay/notify` | GET | 公开 | Epay支付回调 |

**管理员API**:
- `/api/user/plan-orders` - 查看所有订单
- `/api/user/plan-orders/:id` - 查看订单详情
- `/api/user/plan-orders/:id/complete` - 手动完成订单
- `/api/user/plan-orders/:id/cancel` - 取消订单
- `/api/user/plan-orders/:id` - 删除订单

---

### 2.3 统一订单查询API

**文件位置**: `/controller/plan_purchase.go` - `GetMyPlanOrders`

```
GET /api/user/plan/purchase/my-orders?order_type={all|plan|topup}&page=1&page_size=20
```

**功能**: 合并显示钱包充值订单和套餐订单
- `order_type=all`: 显示所有订单（默认）
- `order_type=plan`: 仅显示套餐订单
- `order_type=topup`: 仅显示充值订单

**返回数据**:
```json
{
  "success": true,
  "data": {
    "orders": [...],
    "total": 100,
    "plan_total": 60,
    "topup_total": 40,
    "page": 1,
    "page_size": 20
  }
}
```

---

## 三、前端页面

### 3.1 钱包充值相关页面

#### 充值页面组件
**文件位置**: `/web/src/components/topup/`

- `index.jsx` - 充值主页面
- `RechargeCard.jsx` - 充值卡片组件
- `modals/TopupHistoryModal.jsx` - 充值历史弹窗（显示旧版TopUp表数据）
- `modals/PaymentConfirmModal.jsx` - 支付确认弹窗

**API调用**:
```javascript
// 获取充值历史（旧版）
GET /api/user/topup/self?p=${page}&page_size=${pageSize}&keyword=${keyword}

// 管理员补单
POST /api/user/topup/complete
```

---

### 3.2 订阅套餐相关页面

#### 套餐购买页面
**文件位置**: `/web/src/pages/PlanPricing/index.jsx`

**功能**:
- 显示所有可购买套餐
- 创建套餐订单
- 创建充值订单（按量付费）

---

### 3.3 统一订单管理页面

#### 我的订单页面
**文件位置**: `/web/src/pages/MyOrders/index.jsx`

**功能**:
- 统一显示钱包充值订单和套餐订单
- 支持按类型筛选（全部/套餐/充值）
- 支持继续支付、取消订单

**API调用**:
```javascript
// 获取订单列表（合并）
GET /api/user/plan/purchase/my-orders?page=${page}&page_size=${pageSize}&order_type=${type}

// 取消充值订单
POST /api/user/topup/order/cancel

// 取消套餐订单
POST /api/user/plan/purchase/cancel
```

**订单类型标识**:
```javascript
{
  order_type: "topup",  // 充值订单
  plan_name: "钱包充值"
}

{
  order_type: "plan",   // 套餐订单
  plan_name: "套餐名称"
}
```

---

#### 订单确认页面
**文件位置**: `/web/src/pages/OrderConfirm/index.jsx`

**功能**:
- 显示订单详情
- 选择支付方式
- 发起支付
- 倒计时显示（30分钟）

**路由**:
- 套餐订单: `/console/order-confirm/:orderId`
- 充值订单: `/console/order-confirm/:orderId?type=topup`

**API调用**:
```javascript
// 获取充值订单详情
GET /api/user/topup/order/${orderId}

// 获取套餐订单详情
GET /api/user/plan/purchase/order/${orderId}

// 发起充值支付
POST /api/user/topup/order/pay

// 发起套餐支付
POST /api/user/plan/purchase/pay
```

---

## 四、数据结构差异对比

### 4.1 核心差异

| 特性 | 钱包充值订单 | 订阅套餐订单 |
|------|-------------|-------------|
| **表名** | `topup_orders` | `plan_orders` |
| **订单号前缀** | `TO` | `PO` |
| **主要字段** | Amount, Quota | PlanId, PlanName, PlanQuota |
| **状态** | pending, paid, cancelled, expired | pending, paid, delivered, expired, cancelled |
| **完成标志** | paid（支付即完成） | delivered（需要发放套餐） |
| **关联关系** | 无 | UserPlanId（关联user_plans表） |
| **快照数据** | 无 | 保存套餐信息快照 |
| **重试机制** | 无 | DeliveryRetryCount（发放失败重试） |

---

### 4.2 状态流转

#### 充值订单状态
```
pending → paid → (完成)
   ↓
cancelled / expired
```

#### 套餐订单状态
```
pending → paid → delivered → (完成)
   ↓
cancelled / expired
```

**关键区别**: 套餐订单支付后需要额外的"发放"步骤（创建UserPlan记录）

---

### 4.3 支付回调处理

#### 充值订单回调
**文件**: `/controller/topup_order.go` - `EpayTopupOrderNotify`

**处理流程**:
1. 验证签名
2. 锁定订单（防并发）
3. 验证金额
4. 更新订单状态为paid
5. 增加用户余额（quota字段）
6. 创建TopUp历史记录（兼容旧版）
7. 记录日志

#### 套餐订单回调
**文件**: `/controller/plan_purchase.go` - `EpayPlanOrderNotify`

**处理流程**:
1. 验证签名
2. 锁定订单（防并发）
3. 验证金额
4. 更新订单状态为paid
5. **同步发放套餐**（调用DeliverPlan服务）
6. 如果发放失败，订单保持paid状态，后台重试

---

## 五、关键业务逻辑

### 5.1 充值订单创建

**文件**: `/model/topup_order.go` - `CreateTopupOrder`

```go
func CreateTopupOrder(userId int, amount float64, priceRatio float64, discountRate float64) (*TopupOrder, error)
```

**参数**:
- `amount`: 充值金额（USD）
- `priceRatio`: 汇率（CNY/USD）
- `discountRate`: 折扣率（0.9 = 9折）

**计算逻辑**:
```go
quota = amount * QuotaPerUnit              // 内部额度
originalPrice = amount * priceRatio        // 原价（人民币）
finalPrice = originalPrice * discountRate  // 实付金额
```

---

### 5.2 套餐订单创建

**文件**: `/model/plan_order.go` - `CreatePlanOrder`

```go
func CreatePlanOrder(userId int, planId int) (*PlanOrder, error)
```

**验证逻辑**:
1. 检查套餐是否存在且启用
2. 检查套餐是否可购买（Purchasable=1）
3. 验证用户套餐队列容量（最多10个active套餐）
4. 验证价格（不能为0）
5. 保存套餐信息快照

---

### 5.3 订单过期处理

**定时任务**: 后台定期执行

```go
// 充值订单过期
func ExpireOldTopupOrders() error {
    now := time.Now().UnixMilli()
    DB.Model(&TopupOrder{}).
        Where("status = ? AND expired_at < ?", "pending", now).
        Update("status", "expired")
}

// 套餐订单过期
func ExpireOldOrders() error {
    now := time.Now().UnixMilli()
    DB.Model(&PlanOrder{}).
        Where("status = ? AND expired_at < ?", "pending", now).
        Update("status", "expired")
}
```

---

## 六、支付方式支持

### 6.1 支持的支付方式

| 支付方式 | 充值订单 | 套餐订单 | 说明 |
|---------|---------|---------|------|
| **Epay** | ✅ | ✅ | 支付宝、微信支付 |
| **Stripe** | ✅ | ❌ | 仅充值订单（旧版） |
| **Creem** | ✅ | ❌ | 仅充值订单（旧版） |

**注意**: 新版订单系统（topup_orders、plan_orders）目前仅支持Epay支付方式

---

### 6.2 Epay支付流程

1. 用户创建订单
2. 选择支付方式（alipay/wxpay）
3. 后端调用Epay SDK生成支付表单
4. 前端提交表单跳转到支付页面
5. 用户完成支付
6. Epay回调通知后端
7. 后端验证签名并处理订单

**回调端点**:
- 充值订单: `/api/user/topup/order/epay/notify`
- 套餐订单: `/api/plan/purchase/epay/notify`

---

## 七、前端路由配置

```javascript
// 套餐购买页面
/plans

// 我的订单页面（统一）
/console/my-orders

// 订单确认页面
/console/order-confirm/:orderId          // 套餐订单
/console/order-confirm/:orderId?type=topup  // 充值订单

// 我的套餐页面
/console/myplans
```

---

## 八、数据库索引

### topup_orders表索引
- `order_no` - 唯一索引
- `user_id` - 普通索引
- `payment_trade_no` - 普通索引
- `status` - 普通索引
- `created_at` - 普通索引

### plan_orders表索引
- `order_no` - 唯一索引
- `user_id` - 普通索引
- `plan_id` - 普通索引
- `payment_trade_no` - 普通索引
- `status` - 普通索引
- `created_at` - 普通索引

---

## 九、总结

### 设计特点

1. **双表设计**: 充值订单和套餐订单完全独立，便于扩展和维护
2. **快照机制**: 套餐订单保存购买时的套餐信息，避免套餐修改/删除影响历史订单
3. **统一展示**: 前端统一订单页面可以合并显示两种订单
4. **幂等处理**: 支付回调支持重复调用，避免重复扣款/发放
5. **状态管理**: 清晰的状态流转，支持订单取消、过期处理
6. **重试机制**: 套餐订单发放失败支持后台重试

### 关键文件清单

**后端**:
- `/model/topup_order.go` - 充值订单模型
- `/model/plan_order.go` - 套餐订单模型
- `/controller/topup_order.go` - 充值订单控制器
- `/controller/plan_purchase.go` - 套餐订单控制器
- `/router/api-router.go` - API路由配置

**前端**:
- `/web/src/pages/MyOrders/index.jsx` - 统一订单页面
- `/web/src/pages/OrderConfirm/index.jsx` - 订单确认页面
- `/web/src/components/topup/modals/TopupHistoryModal.jsx` - 充值历史弹窗
