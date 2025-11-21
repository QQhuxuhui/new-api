# Capability: Customer Service Wallet Display

## Capability Overview
在钱包充值页面的充值区域下方,展示客服二维码和闲鱼店铺二维码,为用户提供多种联系和购买渠道,增强用户信任和转化。

---

## ADDED Requirements

### Requirement: Display QR code section below topup area in wallet page
**Priority**: High

The system SHALL display a QR code section below the topup area in the wallet/topup page, showing customer service and Xianyu shop QR codes to provide users with multiple contact and purchase channels.

#### Scenario: User views wallet page with both QR codes configured
**Given** 管理员已配置客服二维码 URL (`CustomerServiceQRCode` option 非空)
**And** 管理员已配置闲鱼店铺二维码 URL (`XianyuQRCode` option 非空)
**When** 用户访问钱包充值页面 (`/topup` 或 `/wallet`)
**And** 用户滚动到充值表单区域下方
**Then** 应显示二维码展示区域,包含标题"更多服务"或"联系我们"
**And** 应并排显示两个二维码卡片:左侧为客服二维码,右侧为闲鱼店铺二维码
**And** 每个二维码卡片应包含:
- 卡片标题("客服二维码" 和 "闲鱼店铺")
- 二维码图片(建议尺寸 150x150 至 200x200 像素)
- 可选的辅助说明文字

#### Scenario: Only customer service QR code is configured
**Given** 管理员已配置客服二维码 URL
**But** 管理员未配置闲鱼店铺二维码 URL (`XianyuQRCode` option 为空)
**When** 用户访问钱包充值页面
**Then** 二维码展示区域应只显示客服二维码卡片
**And** 卡片应居中或靠左显示,不留空白占位

#### Scenario: Only Xianyu shop QR code is configured
**Given** 管理员已配置闲鱼店铺二维码 URL
**But** 管理员未配置客服二维码 URL (`CustomerServiceQRCode` option 为空)
**When** 用户访问钱包充值页面
**Then** 二维码展示区域应只显示闲鱼店铺二维码卡片
**And** 卡片应居中或靠左显示,不留空白占位

#### Scenario: No QR codes configured
**Given** 管理员未配置客服二维码 URL
**And** 管理员未配置闲鱼店铺二维码 URL
**When** 用户访问钱包充值页面
**Then** 不应显示二维码展示区域
**And** 充值表单下方应正常显示其他内容(如邀请链接卡片)

---

### Requirement: Responsive layout for QR code cards
**Priority**: High

The QR code section SHALL support responsive layout to provide good visual experience across different devices and screen sizes.

#### Scenario: Desktop user views QR codes
**Given** 用户在桌面设备(屏幕宽度 ≥ 1024px)上访问钱包页面
**And** 两个二维码都已配置
**When** 用户查看二维码展示区域
**Then** 两个二维码卡片应并排横向排列
**And** 每个卡片应占据大约 50% 的可用宽度(考虑间距)
**And** 卡片之间应有适当的间距(如 16-24px)

#### Scenario: Tablet user views QR codes
**Given** 用户在平板设备(屏幕宽度 768px - 1023px)上访问钱包页面
**And** 两个二维码都已配置
**When** 用户查看二维码展示区域
**Then** 两个二维码卡片可以并排显示或堆叠显示(根据具体宽度)
**And** 卡片应适配屏幕宽度,保持比例和清晰度

#### Scenario: Mobile user views QR codes
**Given** 用户在移动设备(屏幕宽度 < 768px)上访问钱包页面
**And** 两个二维码都已配置
**When** 用户查看二维码展示区域
**Then** 两个二维码卡片应纵向堆叠显示
**And** 每个卡片应占据接近全宽(考虑页面边距)
**And** 卡片之间应有适当的垂直间距

---

### Requirement: QR code image interaction and preview
**Priority**: Medium

Users SHALL be able to interact with QR code images, with support for click-to-enlarge preview functionality for easier scanning.

#### Scenario: User clicks QR code image to preview
**Given** 用户在钱包页面查看二维码展示区域
**When** 用户点击任一二维码图片
**Then** 应打开图片预览模态框或浮层
**And** 模态框应显示放大的二维码图片(建议尺寸 300x300 至 400x400 像素)
**And** 模态框应包含关闭按钮
**And** 用户应能通过点击外部区域或关闭按钮关闭预览

#### Scenario: QR code image has hover effect
**Given** 用户在桌面设备上查看二维码展示区域
**When** 用户将鼠标悬浮在二维码图片上
**Then** 应显示视觉反馈(如透明度变化、边框高亮或阴影加深)
**And** 鼠标指针应变为可点击状态(cursor: pointer)

---

### Requirement: Support internationalization for wallet QR code section
**Priority**: Medium

The QR code section text SHALL support Chinese and English internationalization.

#### Scenario: Chinese user views QR code section
**Given** 用户当前语言设置为中文
**When** 用户查看钱包页面的二维码展示区域
**Then** 区域标题应显示"更多服务" 或 "联系我们"
**And** 客服二维码卡片标题应显示"客服二维码" 或 "扫码联系客服"
**And** 闲鱼店铺卡片标题应显示"闲鱼店铺" 或 "闲鱼购买"
**And** 可选说明文字应为中文

#### Scenario: English user views QR code section
**Given** 用户当前语言设置为英文
**When** 用户查看钱包页面的二维码展示区域
**Then** 区域标题应显示"More Services" 或 "Contact Us"
**And** 客服二维码卡片标题应显示"Customer Service" 或 "Scan to Contact"
**And** 闲鱼店铺卡片标题应显示"Xianyu Shop" 或 "Shop on Xianyu"
**And** 可选说明文字应为英文

---

### Requirement: Wallet page layout includes QR code section
**Priority**: Low

The wallet page layout SHALL be adjusted to include the QR code section between the recharge area and invitation link area.

#### Scenario: QR code section positioned correctly in page flow
**Given** 用户访问钱包充值页面
**When** 用户查看页面整体布局
**Then** 页面内容顺序应为:
1. 页面顶部导航栏
2. 用户余额信息卡片(如果存在)
3. 充值表单区域(RechargeCard 组件)
4. **二维码展示区域(新增)**
5. 邀请链接区域(InvitationCard 组件)
**And** 各区域之间应有合适的垂直间距(如 24-32px)
