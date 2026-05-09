# 绘图工厂菜单外链化设计

- 日期：2026-05-09
- 范围：将侧边栏"绘图工厂"菜单项从站内路由改造为可配置的外链跳转
- 目标：管理员在后台填写外部绘图平台 URL 后，普通用户点击菜单项在新标签页打开该 URL；URL 未配置时菜单项不显示

---

## 1. 背景

仓内已存在站内绘图工厂功能：
- 路由：`/console/draw-factory` → `App.jsx:160` → `<DrawFactory />`
- 菜单：`SiderBar.jsx:239-242`，`to: '/console/draw-factory'`
- 已有配置 `DrawFactoryApiBase`（`model/option.go:73`、`controller/misc.go:112`、`OperationSetting.jsx:139`）—— 这是**站内绘图页内部调 API 的 base**，与本次需求是两件不同的事。

需求方使用形态：将站内绘图入口替换为外部独立绘图平台。本设计在保留站内功能不变的前提下，增加一个独立的外链配置项，由该配置驱动菜单行为。

---

## 2. 数据模型

### 2.1 新增 OptionMap 字段

`model/option.go` 在 `DrawFactoryApiBase` 同位置追加：

```go
common.OptionMap["DrawFactoryExternalUrl"] = ""
```

默认值空字符串，等同于"未配置"。

### 2.2 通过 status 接口暴露

`controller/misc.go` 在返回 status 的 map 中追加：

```go
"DrawFactoryExternalUrl": common.OptionMap["DrawFactoryExternalUrl"],
```

前端通过 `statusState?.status?.DrawFactoryExternalUrl` 读取，复用现有 status 拉取与分发管道，无新增传输层。

---

## 3. 后台管理 UI

### 3.1 新增编辑器组件

新建 `web/src/pages/Setting/Operation/SettingsDrawFactoryExternalUrl.jsx`，仿造 `SettingsDrawFactoryApiBase.jsx` 的结构（同目录已有此文件作为模板）：

- 单输入框，绑定 `inputs.DrawFactoryExternalUrl`
- 占位文案：例 `https://your-drawing-platform.example.com`
- 帮助文案：说明"配置后侧边栏'绘图工厂'菜单将跳转到此 URL；留空则隐藏菜单项"
- 保存时调用现有 `/api/option/` 接口（`updateOption` 模式），与 `DrawFactoryApiBase` 完全一致

### 3.2 挂入 OperationSetting

`web/src/components/settings/OperationSetting.jsx` 三处改动：

1. import 行追加 `SettingsDrawFactoryExternalUrl`
2. `inputs` 默认对象追加 `DrawFactoryExternalUrl: ''`
3. 渲染区在 `<SettingsDrawFactoryApiBase>` 之后追加 `<SettingsDrawFactoryExternalUrl options={inputs} refresh={onRefresh} />`

---

## 4. 侧边栏菜单行为

### 4.1 条件构造菜单项

`web/src/components/layout/SiderBar.jsx` 的 `chatMenuItems` useMemo（当前 231-257 行）：

```jsx
const drawFactoryExternalUrl =
  statusState?.status?.DrawFactoryExternalUrl?.trim() || '';

const items = [
  { text: t('操练场'), itemKey: 'playground', to: '/playground' },
  ...(drawFactoryExternalUrl
    ? [{
        text: t('绘图工厂'),
        itemKey: 'drawFactory',
        externalLink: drawFactoryExternalUrl,
      }]
    : []),
  { text: t('聊天'), itemKey: 'chat', items: chatItems },
];
```

`useMemo` 依赖数组追加 `statusState?.status?.DrawFactoryExternalUrl` 以保证状态变化时重新计算。

### 4.2 渲染分支

定位到 SideBar 把 `item.to` 转 `<Link>` 的渲染处，加一个 `item.externalLink` 优先分支：

```jsx
if (item.externalLink) {
  return (
    <a
      href={item.externalLink}
      target="_blank"
      rel="noopener noreferrer"
    >
      {/* 原来 Link 内的图标 + 文本结构 */}
    </a>
  );
}
```

`rel="noopener noreferrer"` 防 reverse-tabnabbing。

### 4.3 模块可见性叠加

现有 `isModuleVisible('chat', 'drawFactory')` 模块开关保持生效 —— 管理员通过 sidebar 模块可见性配置可强制隐藏该菜单，即便 URL 已配置。优先级：`isModuleVisible == false` → 隐藏；否则看 URL 是否为空。

---

## 5. 行为矩阵

| `DrawFactoryExternalUrl` | `isModuleVisible('chat','drawFactory')` | 表现 |
|---|---|---|
| 空 | true | 菜单项不显示 |
| 空 | false | 菜单项不显示 |
| 非空 | true | 菜单项显示，点击新标签页打开外链 |
| 非空 | false | 菜单项不显示（模块开关优先） |

---

## 6. 兼容性 / 边界

| 项 | 说明 |
|---|---|
| 站内 `/console/draw-factory` 路由 | **不动**。菜单隐藏后页面成为不可达 deadcode，但保留路由零回归风险；删除属另议 |
| `DrawFactoryApiBase` 选项 | **不动**。语义独立，仍服务站内绘图页 |
| URL 校验 | 前端 `.trim()`，**不**强制 http/https 协议（管理员可能填相对路径或子路径） |
| 多语言 | 菜单文本保持 `t('绘图工厂')` i18n key 不变；自定义菜单名属另议 |
| 数据迁移 | 无；新增 OptionMap 字段，gorm 自动以默认值"" 创建 option 行 |
| 响应缓存 | status 接口本身已被前端缓存；新字段跟随同生命周期 |

---

## 7. 测试计划

- 手动：管理员先在"运营设置"留空 URL → 菜单不显示；填入 https URL → 重新刷新页面后菜单显示，点击新标签页打开
- 自动：本变更逻辑薄，依赖前端 status 状态。可补一个 React 组件级测试验证：给定不同 status 时菜单条件渲染是否正确。但仓内 SiderBar 暂无测试基础设施（搜 SiderBar.test.jsx → 无），新增测试需单独搭测试桩；本设计不强制要求

---

## 8. 文件清单

| 文件 | 改动 |
|---|---|
| `model/option.go` | +1 行 OptionMap 默认值 |
| `controller/misc.go` | +1 行 status 暴露 |
| `web/src/components/settings/OperationSetting.jsx` | +import / +默认值 / +渲染 |
| `web/src/pages/Setting/Operation/SettingsDrawFactoryExternalUrl.jsx` | 新建 |
| `web/src/components/layout/SiderBar.jsx` | 菜单条件构造 + 外链渲染分支 |

---

## 9. 不在本设计内

- 删除站内 `/console/draw-factory` 页面与相关 hooks/services（独立清理）
- 管理员可配置菜单名称
- 多个外链平台切换 / 链接列表
