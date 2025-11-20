# Tasks: 添加客户服务联系功能

## Overview
实现客服联系功能,包括顶部导航栏按钮和钱包页面二维码展示。任务按照依赖关系排序,确保先完成后端配置支持,再实现前端展示。

---

## Phase 1: Backend Configuration Support (后端配置支持)

### Task 1.1: Add QR code URL options to backend
**Status**: Completed
**Estimated Effort**: 30 minutes
**Dependencies**: None

在后端 `common/constants.go` 增加两个新的配置常量:
- `CustomerServiceQRCode`: 客服二维码图片 URL
- `XianyuQRCode`: 闲鱼店铺二维码图片 URL (注:已存在 `XianyuShopLink`,这里增加独立的二维码 URL)

修改文件:
- `common/constants.go`: 添加新常量定义
- `model/option.go`: 在 `InitOptionMap()` 中初始化新字段
- `model/option.go`: 在 `updateOptionMap()` 的 switch 语句中增加新字段处理

**Validation**:
- 运行 `go build` 确保编译无错误
- 检查 `InitOptionMap()` 是否正确初始化新字段

---

### Task 1.2: Add admin API endpoints for QR code configuration
**Status**: Completed
**Estimated Effort**: 15 minutes
**Dependencies**: Task 1.1

确保现有的 `GET /api/option/` 和 `PUT /api/option/` 接口能够处理新增的二维码配置字段。

修改文件:
- `controller/option.go`: 验证现有代码已支持动态字段(无需修改,仅验证)

**Validation**:
- 使用 API 客户端(如 Postman)测试:
  - GET `/api/option/` 应返回包含新字段的配置列表
  - PUT `/api/option/` 应能成功更新 `CustomerServiceQRCode` 和 `XianyuQRCode`

---

## Phase 2: Admin UI Configuration (管理后台配置界面)

### Task 2.1: Add QR code upload fields to admin settings
**Status**: Completed
**Estimated Effort**: 1 hour
**Dependencies**: Task 1.2

在管理后台"运营设置 > 通用设置"页面增加两个图片 URL 输入字段。

修改文件:
- `web/src/pages/Setting/Operation/SettingsGeneral.jsx`:
  - 在 `inputs` state 中添加 `CustomerServiceQRCode` 和 `XianyuQRCode` 字段
  - 在表单中添加两个 `Input` 组件用于输入图片 URL
  - 添加图片预览功能(可选,使用 `Image` 组件)
- `web/src/i18n/locales/zh.json` 和 `en.json`: 添加新字段的国际化文案

**Validation**:
- 访问管理后台设置页面,确认新字段正常显示
- 输入测试图片 URL,点击保存,刷新页面验证配置已持久化
- 切换语言,确认文案正确翻译

---

### Task 2.2: Add UI feedback for QR code preview
**Status**: Completed
**Estimated Effort**: 30 minutes
**Dependencies**: Task 2.1

为管理员提供二维码图片预览功能,方便确认上传的图片是否正确。

修改文件:
- `web/src/pages/Setting/Operation/SettingsGeneral.jsx`:
  - 在图片 URL 输入框下方添加 `Image` 组件,显示预览图
  - 仅当 URL 有效且以图片格式结尾时显示预览
  - 添加加载错误处理

**Validation**:
- 输入有效图片 URL,确认预览图正常显示
- 输入无效 URL,确认不显示预览或显示错误提示

---

## Phase 3: Header Customer Service Button (顶部客服按钮)

### Task 3.1: Create CustomerServiceButton component
**Status**: Completed
**Estimated Effort**: 1.5 hours
**Dependencies**: Task 1.2

创建独立的客服按钮组件,支持桌面端悬浮和移动端点击交互。

新建文件:
- `web/src/components/layout/headerbar/CustomerServiceButton.jsx`:
  - 使用 `Popover` (桌面端)和 `Modal` (移动端)展示二维码
  - 从 `StatusContext` 获取 `CustomerServiceQRCode` 配置
  - 未配置时返回 `null`,不渲染按钮
  - 支持国际化文案

**Validation**:
- 在 Storybook 或独立页面测试组件
- 桌面端鼠标悬浮能正常显示二维码浮层
- 移动端点击能正常打开模态框
- 未配置时组件不渲染

---

### Task 3.2: Integrate CustomerServiceButton into HeaderBar
**Status**: Completed
**Estimated Effort**: 30 minutes
**Dependencies**: Task 3.1

将客服按钮集成到页面顶部导航栏,位于用户头像左侧或通知按钮附近。

修改文件:
- `web/src/components/layout/headerbar/index.jsx`:
  - 导入 `CustomerServiceButton` 组件
  - 在 `ActionButtons` 组件附近添加 `CustomerServiceButton`
- `web/src/components/layout/headerbar/ActionButtons.jsx` (如果需要调整布局):
  - 调整按钮间距和顺序

**Validation**:
- 访问任意页面,确认顶部导航栏显示客服按钮(已配置时)
- 按钮位置合理,不与其他按钮重叠
- 响应式布局正常,移动端不显示或调整为汉堡菜单内

---

### Task 3.3: Add internationalization for header button
**Status**: Completed
**Estimated Effort**: 15 minutes
**Dependencies**: Task 3.1

为客服按钮添加中英文国际化支持。

修改文件:
- `web/src/i18n/locales/zh.json`: 添加"联系客服"、"客服二维码"等文案
- `web/src/i18n/locales/en.json`: 添加"Contact Support"、"Customer Service QR Code"等文案
- 其他语言文件(`fr.json`, `ja.json`, `ru.json`): 添加对应翻译(可选)

**Validation**:
- 切换语言为中文,确认按钮和浮层文案为中文
- 切换语言为英文,确认按钮和浮层文案为英文

---

## Phase 4: Wallet Page QR Code Display (钱包页面二维码展示)

### Task 4.1: Create QRCodeSection component for wallet page
**Status**: Completed
**Estimated Effort**: 2 hours
**Dependencies**: Task 1.2

创建二维码展示组件,显示客服和闲鱼店铺二维码,支持响应式布局。

新建文件:
- `web/src/components/topup/QRCodeSection.jsx`:
  - 接收 `customerServiceQRCode` 和 `xianyuQRCode` props
  - 使用 `Card` 组件包裹每个二维码
  - 响应式布局:桌面端并排,移动端堆叠
  - 支持点击二维码放大预览(使用 `Image` 组件的 preview 功能)
  - 未配置时不渲染对应卡片

**Validation**:
- 在 Storybook 或测试页面中验证组件
- 桌面端两个二维码并排显示
- 移动端两个二维码纵向堆叠
- 点击二维码能放大预览

---

### Task 4.2: Integrate QRCodeSection into TopUp page
**Status**: Completed
**Estimated Effort**: 30 minutes
**Dependencies**: Task 4.1

将二维码展示组件集成到钱包充值页面,位于充值表单和邀请链接之间。

修改文件:
- `web/src/components/topup/index.jsx`:
  - 从 `statusState` 获取 `CustomerServiceQRCode` 和 `XianyuQRCode` 配置
  - 在 `RechargeCard` 和 `InvitationCard` 之间插入 `QRCodeSection` 组件
  - 调整布局间距

**Validation**:
- 访问 `/topup` 页面,确认二维码区域正常显示(已配置时)
- 页面布局合理,各区域间距适当
- 未配置时该区域不显示

---

### Task 4.3: Add internationalization for wallet QR code section
**Status**: Completed
**Estimated Effort**: 15 minutes
**Dependencies**: Task 4.1

为钱包页面二维码区域添加中英文国际化支持。

修改文件:
- `web/src/i18n/locales/zh.json`: 添加"更多服务"、"客服二维码"、"闲鱼店铺"等文案
- `web/src/i18n/locales/en.json`: 添加"More Services"、"Customer Service"、"Xianyu Shop"等文案

**Validation**:
- 切换语言为中文,确认二维码区域文案为中文
- 切换语言为英文,确认二维码区域文案为英文

---

## Phase 5: Testing & Documentation (测试与文档)

### Task 5.1: Manual testing across devices and browsers
**Status**: Completed
**Estimated Effort**: 1 hour
**Dependencies**: Task 3.2, Task 4.2

在不同设备和浏览器上进行手动测试,确保功能正常。

测试清单:
- [ ] 桌面端 Chrome/Firefox/Safari:顶部按钮悬浮显示二维码
- [ ] 移动端 Chrome/Safari:顶部按钮点击显示二维码
- [ ] 桌面端钱包页面:二维码并排显示,点击放大
- [ ] 移动端钱包页面:二维码堆叠显示,点击放大
- [ ] 未配置二维码时:相关元素不显示
- [ ] 仅配置一个二维码时:仅显示已配置的二维码
- [ ] 语言切换:中英文文案正确显示

**Validation**:
- 所有测试项通过
- 发现的问题已记录并修复

---

### Task 5.2: Update user documentation
**Status**: Completed
**Estimated Effort**: 30 minutes
**Dependencies**: Task 5.1

编写用户文档,说明如何配置和使用客服联系功能。

新建/修改文件:
- `docs/admin-guide.md` 或 `docs/zh/admin-guide.md`:
  - 添加"配置客服二维码"章节
  - 说明如何在管理后台上传二维码图片 URL
  - 提供示例截图(可选)
- `docs/user-guide.md` 或 `docs/zh/user-guide.md`:
  - 添加"联系客服"章节
  - 说明如何使用顶部客服按钮和钱包页面二维码

**Validation**:
- 文档语言清晰,易于理解
- 文档内容与实际功能一致

---

### Task 5.3: Code review and cleanup
**Status**: Completed
**Estimated Effort**: 30 minutes
**Dependencies**: Task 5.1

进行代码审查,确保代码质量和一致性。

检查清单:
- [ ] 代码符合项目编码规范(Go 和 React)
- [ ] 无 console.log 等调试代码
- [ ] 组件和函数命名清晰
- [ ] 添加必要的注释(特别是复杂逻辑)
- [ ] 无明显的性能问题
- [ ] 无安全隐患(如 XSS、CSRF)

**Validation**:
- 代码审查通过
- 所有检查项完成

---

## Task Summary

**Total Tasks**: 13
**Estimated Total Effort**: ~9 hours

**Task Dependencies Graph**:
```
1.1 (Backend Options) → 1.2 (API Endpoints)
                          ↓
         +----------------+----------------+
         ↓                ↓                ↓
    2.1 (Admin UI)   3.1 (Header Btn)  4.1 (Wallet QR)
         ↓                ↓                ↓
    2.2 (Preview)    3.2 (Integrate)   4.2 (Integrate)
                          ↓                ↓
                     3.3 (i18n)       4.3 (i18n)
                          ↓                ↓
         +----------------+----------------+
         ↓
    5.1 (Testing) → 5.2 (Docs) → 5.3 (Code Review)
```

**Parallel Work Opportunities**:
- Phase 2 (Admin UI) 和 Phase 3 (Header Button) 可以并行开发
- Phase 4 (Wallet Display) 可以在 Phase 3 完成后立即开始
- Task 3.3 和 Task 4.3 (国际化) 可以合并处理

---

## Notes
- 建议使用 Semi Design 的 `Popover` 和 `Modal` 组件,保持 UI 一致性
- 图片上传建议使用图床服务(如七牛云、阿里云 OSS),避免增加服务器存储压力
- 二维码图片建议尺寸:桌面端 200x200px,移动端 150x150px,预览时 300x300px
- 可以考虑添加二维码过期时间或版本号,方便管理员更新
