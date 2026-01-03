# Tasks

## 1. 后端：充值订单系统

- [ ] 1.1 创建充值订单模型 `model/topup_order.go`
  - 订单字段：id, user_id, amount, quota, status, payment_method, created_at, paid_at
  - 状态：pending, paid, cancelled, expired

- [ ] 1.2 创建充值订单 API `POST /api/user/topup/order/create`
  - 请求参数：`{ amount: number }` (充值金额)
  - 响应：`{ order_id: string, amount: number, quota: number, price: number }`
  - 计算实付金额（考虑折扣）

- [ ] 1.3 获取充值订单详情 API `GET /api/user/topup/order/:id`
  - 返回订单详情，包括可用支付方式

- [ ] 1.4 充值订单支付 API `POST /api/user/topup/order/:id/pay`
  - 请求参数：`{ payment_method: string }`
  - 调用现有支付网关逻辑
  - 支付成功后给用户钱包增加余额

- [ ] 1.5 充值订单支付回调处理
  - 复用现有支付回调逻辑
  - 支付成功后更新订单状态并增加用户余额

## 2. 前端：产品定价页面集成

- [ ] 2.1 修改 `PlanPricing` 组件
  - 当 `filter === 'payg'` 时，调用 `/api/user/topup/info` 获取预设金额
  - 展示充值金额卡片（复用钱包页的卡片样式）

- [ ] 2.2 实现按量付费卡片组件
  - 展示金额、折扣信息
  - 点击后调用 `/api/user/topup/order/create` 创建订单
  - 跳转到 `/console/order-confirm/:order_id?type=topup`

- [ ] 2.3 未登录用户处理
  - 点击购买时跳转 `/login?redirect=/plans`
  - 登录后返回产品定价页

## 3. 前端：订单确认页适配

- [ ] 3.1 修改 `OrderConfirm` 组件
  - 支持 `type=topup` 参数
  - 根据类型调用不同的订单详情 API

- [ ] 3.2 充值订单确认页 UI
  - 展示充值金额、实付金额、折扣信息
  - 展示支付方式选择（支付宝、微信、Stripe 等）

- [ ] 3.3 充值订单支付流程
  - 调用 `/api/user/topup/order/:id/pay` 发起支付
  - 处理支付结果

## 4. 前端：我的套餐页面展示钱包余额

- [ ] 4.1 添加钱包余额虚拟卡片组件 `PaygBalanceCard`
  - 展示当前钱包余额
  - 显示"按量付费"类型标签
  - 显示"永不过期"有效期标签
  - 添加"充值"操作按钮

- [ ] 4.2 修改 `MyPlans` 组件
  - 获取用户钱包余额（从 userState 或新增 API）
  - 在套餐列表中展示钱包余额卡片
  - 卡片位置：放在真实套餐卡片之后

- [ ] 4.3 充值按钮跳转逻辑
  - 点击"充值"跳转到 `/plans?category=payg`
  - 产品定价页面支持 URL 参数预选分类

## 5. 测试与优化

- [ ] 5.1 测试完整购买流程
- [ ] 5.2 测试未登录用户流程
- [ ] 5.3 测试各支付方式
- [ ] 5.4 测试折扣计算
- [ ] 5.5 测试我的套餐页面钱包余额展示
