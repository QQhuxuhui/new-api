# Change: 在产品定价页面集成按量付费（复用钱包充值）

## Why

目前产品定价页面只展示订阅类套餐（日卡、周卡、月卡等），缺少按量付费选项。用户需要单独访问钱包管理页面才能进行充值。另外，用户的"消费能力"分散在两个地方（我的套餐 + 钱包余额），体验割裂。

将按量付费集成到产品定价页面，并在我的套餐页面统一展示钱包余额，可以：
- 统一用户的购买入口，提升体验
- 复用现有钱包充值逻辑，避免重复开发
- 让用户在一个页面就能看到所有定价选项
- 统一展示用户所有的"消费能力"

## What Changes

### 前端 - 产品定价页面
- 修改产品定价页面 (`PlanPricing`)：当选择"按量付费"分类时，展示预设充值金额卡片
- 调用 `/api/user/topup/info` API 获取预设充值金额配置
- 点击充值金额卡片后，创建"充值订单"并跳转到订单确认页
- 订单确认页支持处理充值类型订单，展示支付方式选择

### 前端 - 我的套餐页面
- 修改我的套餐页面 (`MyPlans`)：展示钱包余额作为虚拟"按量付费"卡片
- 卡片展示：余额、"永不过期"标签、"充值"操作按钮
- 点击"充值"按钮跳转到产品定价页面的按量付费分类

### 后端
- 新增创建充值订单 API：`POST /api/user/topup/order/create`
- 订单确认页 API 支持返回充值订单详情
- 充值订单支付完成后，直接给用户钱包增加余额

### 不变的部分
- 钱包管理页面保持原样（两个入口并存）
- 侧边栏菜单保持不变
- 现有套餐购买流程不变

## Impact

- Affected specs: `plan-pricing-display`, `plan-purchase-flow`, `user-plan-system`
- Affected code:
  - `web/src/pages/PlanPricing/index.jsx`
  - `web/src/pages/MyPlans/index.jsx`
  - `web/src/pages/OrderConfirm/index.jsx`
  - `controller/topup.go` (新增)
  - `model/topup_order.go` (新增)
