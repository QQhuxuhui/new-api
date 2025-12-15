## ADDED Requirements

### Requirement: Display pay-as-you-go topup options in pricing page

The system SHALL display wallet topup amount options when user selects the "pay-as-you-go" category on the pricing page, reusing the existing topup configuration.

#### Scenario: User views pay-as-you-go category with preset amounts configured

**Given** 管理员已配置预设充值金额选项
**And** 用户访问产品定价页面 `/plans`
**When** 用户点击"按量付费"分类筛选
**Then** 页面应展示预设充值金额卡片（而非套餐卡片）
**And** 每个卡片显示：充值金额、实付金额、折扣信息（如有）
**And** 卡片样式与套餐卡片保持一致的设计语言

#### Scenario: User views pay-as-you-go with discount configured

**Given** 管理员配置了充值 100 享 9 折优惠
**And** 用户访问产品定价页面并选择"按量付费"
**When** 页面加载充值金额卡片
**Then** 100 元充值卡片应显示折扣标签（如 "-10%"）
**And** 应显示原价和折后实付金额

#### Scenario: No preset amounts configured

**Given** 管理员未配置任何预设充值金额
**When** 用户选择"按量付费"分类
**Then** 页面应显示空状态提示"暂无可用的充值选项"
**And** 提示用户联系管理员或访问钱包页面

---

### Requirement: Create topup order from pricing page

The system SHALL allow users to create a topup order directly from the pricing page and redirect to order confirmation.

#### Scenario: Logged-in user clicks purchase on topup amount

**Given** 用户已登录
**And** 用户在产品定价页面查看"按量付费"选项
**When** 用户点击某个充值金额卡片的"立即购买"按钮
**Then** 系统调用 `/api/user/topup/order/create` 创建充值订单
**And** 跳转到订单确认页 `/console/order-confirm/:order_id?type=topup`
**And** 订单确认页显示充值详情和支付方式选择

#### Scenario: Anonymous user clicks purchase on topup amount

**Given** 用户未登录
**And** 用户在产品定价页面查看"按量付费"选项
**When** 用户点击某个充值金额卡片的"立即购买"按钮
**Then** 系统跳转到 `/login?redirect=/plans`
**And** 用户登录成功后返回产品定价页面
**And** 用户可以重新选择充值金额进行购买

---

### Requirement: Topup order confirmation and payment

The system SHALL support topup order confirmation and payment flow through the existing order confirmation page.

#### Scenario: User views topup order confirmation

**Given** 用户已创建充值订单
**When** 用户访问订单确认页 `/console/order-confirm/:order_id?type=topup`
**Then** 页面显示充值订单详情：
  - 充值金额（如 "$100"）
  - 实付金额（考虑折扣后）
  - 折扣信息（如有）
**And** 页面显示可用支付方式列表

#### Scenario: User completes topup payment

**Given** 用户在订单确认页选择支付方式
**When** 用户点击"确认支付"
**Then** 系统调用支付接口发起支付
**And** 支付成功后，用户钱包余额增加相应金额
**And** 订单状态更新为"已支付"
**And** 用户收到充值成功提示

---

### Requirement: Display wallet balance as pay-as-you-go plan in my plans page

The system SHALL display the user's wallet balance as a virtual "pay-as-you-go" plan card in the My Plans page, providing a unified view of all user's spending capabilities.

#### Scenario: User views my plans page with wallet balance

**Given** 用户已登录
**And** 用户钱包余额为 $30
**When** 用户访问"我的套餐"页面 `/console/myplans`
**Then** 页面应展示钱包余额卡片，包含：
  - 类型标签："按量付费"
  - 余额显示：$30（或等值本币金额）
  - 有效期标签："永不过期"
  - 操作按钮："充值"
**And** 卡片位置在真实套餐卡片之后

#### Scenario: User with zero wallet balance

**Given** 用户已登录
**And** 用户钱包余额为 $0
**When** 用户访问"我的套餐"页面
**Then** 页面仍应展示钱包余额卡片
**And** 余额显示为 $0
**And** "充值"按钮仍可点击

#### Scenario: User clicks topup button on wallet balance card

**Given** 用户在"我的套餐"页面查看钱包余额卡片
**When** 用户点击"充值"按钮
**Then** 系统跳转到产品定价页面 `/plans?category=payg`
**And** 产品定价页面自动选中"按量付费"分类
**And** 展示预设充值金额选项
