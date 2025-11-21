# Capability: Customer Service Header Button

## Purpose
在页面顶部导航栏增加"联系客服"按钮,用户通过鼠标悬浮或点击查看管理员配置的客服二维码,提供快速便捷的客服联系入口。

## Requirements

### Requirement: Display customer service button in header navigation
**Priority**: High

The system SHALL display a "Contact Customer Service" button in the top-right header navigation area on all pages when configured, providing users with a globally accessible customer service contact entry point.

#### Scenario: User sees customer service button when QR code is configured
**Given** 管理员已在后台配置了客服二维码 URL (`CustomerServiceQRCode` option 存在且非空)
**When** 用户访问任意页面并查看页面顶部导航栏
**Then** 应在顶部导航栏右侧区域(用户头像左侧或附近)显示"联系客服"按钮
**And** 按钮应使用合适的图标(如消息图标或客服图标)和文字标签
**And** 按钮样式应与现有导航栏按钮保持一致

#### Scenario: Customer service button is hidden when not configured
**Given** 管理员未配置客服二维码 URL (`CustomerServiceQRCode` option 为空或不存在)
**When** 用户访问任意页面并查看页面顶部导航栏
**Then** 不应显示"联系客服"按钮
**And** 导航栏布局应正常显示其他元素

---

### Requirement: Show QR code popover on user interaction
**Priority**: High

The system SHALL display the customer service QR code in a popover/modal when users interact with the customer service button, providing clear visual feedback.

#### Scenario: Desktop user hovers over customer service button
**Given** 用户在桌面设备上访问页面
**And** 管理员已配置客服二维码 URL
**When** 用户将鼠标悬浮在"联系客服"按钮上
**Then** 应在按钮下方或附近弹出浮层(Popover)
**And** 浮层应显示客服二维码图片
**And** 浮层应包含标题"客服二维码"或"扫码联系客服"
**And** 二维码图片应清晰可见,建议尺寸为 200x200 至 300x300 像素

#### Scenario: Mobile user taps customer service button
**Given** 用户在移动设备上访问页面
**And** 管理员已配置客服二维码 URL
**When** 用户点击"联系客服"按钮
**Then** 应打开模态框(Modal)或全屏浮层显示客服二维码
**And** 模态框应包含关闭按钮
**And** 二维码图片应适配移动设备屏幕宽度

#### Scenario: User closes QR code popover
**Given** 客服二维码浮层已打开
**When** 用户执行以下任一操作:
- 将鼠标移出浮层区域(桌面端悬浮模式)
- 点击浮层外部区域
- 点击关闭按钮(移动端模态框)
**Then** 浮层应关闭并消失
**And** 页面应恢复正常交互状态

---

### Requirement: Support internationalization for UI text
**Priority**: Medium

The customer service button and related UI text SHALL support Chinese and English internationalization to ensure users of different languages can understand the feature.

#### Scenario: User switches language between Chinese and English
**Given** 用户当前语言设置为中文
**When** 用户查看"联系客服"按钮和二维码浮层
**Then** 按钮文字应显示"联系客服"
**And** 浮层标题应显示"客服二维码"或"扫码联系客服"
**When** 用户切换语言为英文
**Then** 按钮文字应更新为"Contact Support" 或 "Customer Service"
**And** 浮层标题应更新为"Customer Service QR Code" 或 "Scan to Contact"

---
