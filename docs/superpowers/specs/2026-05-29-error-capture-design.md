# 错误抓取（Error Capture）设计文档

日期：2026-05-29
状态：待评审

## 1. 背景与目标

运营/排障时需要看到**特定报错下用户实际发来的完整请求数据**，但又不能对所有请求都记录（成本、隐私、存储）。

因此需要：管理员可在一个独立页面配置若干「错误关键词规则」；当上游报错日志的**详情**包含某条规则的关键词时，额外完整保存该次请求的请求体与关键元数据；每条规则只保留最近 100 条，超出自动过期；并能在页面查看完整请求体。

## 2. 已确认的需求边界

| 维度 | 决定 |
|------|------|
| 触发范围 | **仅上游报错日志**（`LogTypeError`，`controller/relay.go` 的 `processChannelError`） |
| 匹配对象 | 错误日志详情 `err.MaskSensitiveError()`（即日志「详情列」`Log.Content`） |
| 匹配方式 | **大小写不敏感子串匹配**；命中任一启用规则即抓取，命中多个规则各存一条 |
| 抓取内容 | 原始请求体 `request_body` + 关键元数据（不含上游响应、不含请求头） |
| 过期策略 | **按条数**，每条规则保留最近 `max_records`（默认 100），写入后异步裁剪 |
| 页面形态 | **独立新管理菜单页**：上半配置规则、下半按规则查看抓取记录、点开看完整请求体 |
| 访问权限 | **全部 RootAuth（仅超级管理员）** —— 配置与查看均限超管 |
| 去重 | **每请求最多 1 条**（跨渠道重试只在首次命中时捕获，context 标记） |
| 开关依赖 | **独立开关**，不依赖全局 `ErrorLogEnabled`；捕获仅受 `error_capture_setting.Enabled` + `IsRecordErrorLog(err)` 约束 |
| 配置持久化 | **复用现有 `Option`/`UpdateOption` 机制**，不新建保存接口 |

YAGNI：不做上游响应抓取、不做请求头抓取、不做按时间过期、不做用户侧可见。

## 3. 架构与方案

### 3.1 选型结论

捕获记录使用**独立新表 `ErrorCaptureLog`，落在 `LOG_DB`**（与 `Log` 同库）。理由：
- 与主 `logs` 表解耦，`request_body` 大字段、`rule_id` 分组、按规则裁剪都不影响主日志表查询；
- 沿用 `Log` 的 `LOG_DB` 通道，兼容独立日志库部署。

否决「复用 logs 表加列」方案：会给本就庞大的主表加大字段，且按规则裁剪会与正常日志删除逻辑纠缠。

### 3.2 配置存储

新增注册配置 `error_capture_setting`（沿用 `setting/config` 的 `GlobalConfig.Register` 模式，持久化为 `Option`）：

```go
type ErrorCaptureSetting struct {
    Enabled bool   `json:"enabled"` // 总开关，默认 false
    Rules   string `json:"rules"`   // JSON 数组字符串，见下
}
```

`Rules` 反序列化后的单条规则：

```go
type ErrorCaptureRule struct {
    Id         string `json:"id"`          // 稳定唯一 id（新增时后端生成），用于给捕获记录分组
    Keyword    string `json:"keyword"`     // 匹配关键词（子串，大小写不敏感）
    Label      string `json:"label"`       // 备注名
    Enabled    bool   `json:"enabled"`     // 单条启用
    MaxRecords int    `json:"max_records"` // 保留条数，默认 100，下限 1、上限可设（如 1000）
}
```

放在 `setting/console_setting/`（或新建 `setting/console_setting/error_capture.go`），提供 `GetErrorCaptureSetting()` 及解析 `Rules` 为 `[]ErrorCaptureRule` 的辅助函数。

**持久化复用现有机制（不新建保存接口）**：沿用 `setting/config` 的 `GlobalConfig`，配置字段以扁平 key `error_capture_setting.enabled` / `error_capture_setting.rules` 存为 `Option`，读写直接走现有 `GET /api/option/`（`GetOptions`）与 `PUT /api/option/`（`UpdateOption`）。在 `controller/option.go` 的 `UpdateOption` 中为 `error_capture_setting.rules` 增加一个校验/归一 case：解析 JSON、给缺失 `id` 的规则生成 `id`、夹紧 `max_records`（下限 1、上限 1000）、去除空白关键词。规则 `id` 由前端保留已有 / 给新行生成，后端归一兜底。

### 3.3 捕获表模型

`model/error_capture_log.go`：

```go
type ErrorCaptureLog struct {
    Id          int    `json:"id"`
    RuleId      string `json:"rule_id" gorm:"index:idx_rule_id_id,priority:1;index"`
    Keyword     string `json:"keyword"`                                  // 命中时的关键词快照
    CreatedAt   int64  `json:"created_at" gorm:"bigint;index:idx_rule_id_id,priority:2"`
    UserId      int    `json:"user_id" gorm:"index"`
    Username    string `json:"username"`
    ModelName   string `json:"model_name"`
    ChannelId   int    `json:"channel_id"`
    TokenName   string `json:"token_name"`
    StatusCode  int    `json:"status_code"`
    ErrorType   string `json:"error_type"`
    ErrorCode   string `json:"error_code"`
    RequestPath string `json:"request_path"`
    Content     string `json:"content"`      // 匹配到的错误详情
    RequestBody string `json:"request_body" gorm:"type:text"` // 截断后的请求体
    Other       string `json:"other"`        // 预留元数据 json
}
```

- 联合索引 `idx_rule_id_id (rule_id, id)`：支撑「按规则取最新 N 条」列表与裁剪。
- 注册进 `migrateLOGDB()`（`LOG_DB.AutoMigrate(&Log{}, &ErrorCaptureLog{})`）。

请求体截断上限常量 `ErrorCaptureBodyMaxBytes = 64 * 1024`；超出按字节截断并追加 `...[truncated]` 标记。

### 3.4 捕获钩子

**位置**：放在 `controller/relay.go` 重试循环内、`processChannelError(c, ...)` 调用点（约 line 338）**之后**，而**不是**放进共享的 `processChannelError` 函数体。

理由：`processChannelError` 还被 `controller/channel-test.go:602`（管理员测试渠道）调用——那里用的是测试请求体而非真实用户数据。把捕获逻辑放在 relay 调用点之后，channel-test 路径天然不触发捕获，无需额外来源判断。

```go
// relay.go retryLoop 内，processChannelError(...) 之后
captureErrorRequestIfMatched(c, newAPIError)
```

`captureErrorRequestIfMatched` 逻辑：

```go
func captureErrorRequestIfMatched(c *gin.Context, err *types.NewAPIError) {
    s := console_setting.GetErrorCaptureSetting()
    if !s.Enabled || !types.IsRecordErrorLog(err) {  // 独立于全局 ErrorLogEnabled
        return
    }
    if c.GetBool(constant.ContextKeyErrorCaptureDone) { // 每请求只抓一次
        return
    }
    content := err.MaskSensitiveError()
    matched := matchErrorCaptureRules(content, s.ParsedRules()) // 纯函数
    if len(matched) == 0 {
        return
    }
    c.Set(constant.ContextKeyErrorCaptureDone, true)
    body, _ := common.GetRequestBody(c) // 已缓存
    payload := buildCapturePayload(c, err, content, body) // 主协程取值快照
    gopool.Go(func() {
        model.RecordErrorCaptureLogs(matched, payload) // 逐规则插入 + 裁剪
    })
}
```

设计要点（已确认）：
- **独立开关**：仅看 `error_capture_setting.Enabled && types.IsRecordErrorLog(err)`，不依赖全局 `constant.ErrorLogEnabled`。全局错误日志关闭时捕获仍可工作。
- **每请求去重**：首次命中后置 `c.Set(ContextKeyErrorCaptureDone, true)`，同一请求后续渠道重试不再重复捕获，避免重复请求体刷爆 100 条配额。新增常量 `constant.ContextKeyErrorCaptureDone`。
- `matchErrorCaptureRules(content, rules) []ErrorCaptureRule`：纯函数，遍历启用规则做 `strings.Contains(strings.ToLower(content), strings.ToLower(keyword))`，空关键词跳过。**可独立单测**。
- `buildCapturePayload`：在主协程从 `c` 取出所有需要的值（userId/username/model/channel/token/status/error_type/path/body）做值快照，异步闭包内**不再触碰 `c`**。
- `RecordErrorCaptureLogs`：对每条命中规则插入一行（共享同一份请求体/元数据快照），插入后触发该规则裁剪。并发场景下（虽已每请求去重，仍可能多请求并发）裁剪为幂等的"保留最新 N"，竞态无害。

### 3.5 保留裁剪（按条数）

每条规则写入后异步执行裁剪，保证 `count(rule_id) <= max_records`：

```sql
DELETE FROM error_capture_logs
WHERE rule_id = ?
  AND id NOT IN (
    SELECT id FROM (
      SELECT id FROM error_capture_logs
      WHERE rule_id = ? ORDER BY id DESC LIMIT ?
    ) t
  )
```

- 包一层子查询（`SELECT * FROM (...) t`）规避 MySQL「不能 DELETE 同时在子查询引用的表」限制；SQLite/Postgres 同样兼容。
- 仅命中的报错才触发，量小，逐次裁剪成本可忽略。

### 3.6 API

**配置读写复用现有接口**（见 3.2）：`GET/PUT /api/option/` 处理 `error_capture_setting.enabled` / `error_capture_setting.rules`，不新增配置接口。

**仅新增「查看捕获记录」路由**（前缀 `/api/error_capture`，全部 `RootAuth`）：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/error_capture/logs?rule_id=&p=&page_size=` | 按规则分页列出捕获记录（不返回完整 body，仅摘要） |
| GET | `/api/error_capture/logs/:id` | 取单条完整记录（含 `request_body`） |
| DELETE | `/api/error_capture/logs?rule_id=` | 清空某规则下记录（可选便捷操作） |

控制器：`controller/error_capture.go`。模型查询函数放 `model/error_capture_log.go`。`error_capture_setting.rules` 的归一/校验放在 `controller/option.go` 的 `UpdateOption` 对应 case（见 3.2）。

### 3.7 前端

新增管理菜单页「错误抓取」，参照现有 `web/src/pages/FailoverRules` 的接入方式（页面组件 + 路由 + SiderBar 菜单项 + i18n），仅超管可见：

- **上半 — 规则配置**：总开关；规则表格（关键词、备注、启用开关、保留条数），支持增/删/改/保存。
- **下半 — 抓取记录**：规则下拉选择 → 分页表格（时间、用户、模型、状态码、错误类型、错误详情摘要）→ 点击行打开抽屉/弹窗，显示完整请求体（JSON 美化）与全部元数据。

新增 i18n 文案（`web/src/i18n/locales/zh.json` / `en.json`）。

### 3.8 测试

Go 单测（紧贴现有 `model/*_test.go`、`controller/*_test.go` 风格）：
- `matchErrorCaptureRules`：大小写不敏感、命中多规则、未命中、空关键词跳过。
- 请求体截断：超限按字节截断且不破坏 UTF-8 末尾（按字节截断后做 `valid` 修正）。
- 裁剪：插入 > N 条后保留最新 N 条、不同 `rule_id` 互不影响。
- 规则保存校验：`max_records` 越界归一、关键词去空白。

钩子集成（端到端）不在单测范围，靠纯函数拆分保证可测性。

## 4. 数据流

```
relay 重试循环内，每次渠道失败:
  → processChannelError（现有：可选写入 Log(type=5)，不变）
  → captureErrorRequestIfMatched(c, err):
       if !Enabled || !IsRecordErrorLog(err) || c.GetBool(ErrorCaptureDone): return
       matched = matchErrorCaptureRules(错误详情, 启用规则)
       命中 → c.Set(ErrorCaptureDone,true)（每请求一次）
            → 取 request_body(截断) + 元数据快照
            → gopool.Go: 逐规则插入 ErrorCaptureLog → 按规则裁剪到 max_records
（channel-test 直接调 processChannelError，不经过捕获，故测试请求不被捕获）

管理员(超管) → 错误抓取页
  → GET/PUT /api/option/  读写 error_capture_setting.*（开关 + 规则）
  → GET /api/error_capture/logs?rule_id  列表
  → GET /api/error_capture/logs/:id      看完整请求体
```

## 5. 影响面与风险

- **改动点**：新增 1 个 model + migration（注册进 `migrateLOGDB()`）、1 个 setting、1 个 controller（仅查看记录）、`UpdateOption` 增 1 个校验 case、若干查看路由、1 个前端页面与菜单；在 `relay.go` 重试循环 `processChannelError` 调用点后追加一行 `captureErrorRequestIfMatched`（不改 `processChannelError` 本身，不影响 channel-test 路径）。
- **性能**：仅命中报错且每请求一次才异步写库 + 裁剪，主请求路径零额外阻塞。
- **隐私**：完整请求体仅超管（RootAuth）可见；request_body 截断上限 64KB 防超大 base64 撑爆存储。
- **兼容**：捕获表进 `LOG_DB`，兼容独立日志库；裁剪 SQL 包子查询，SQLite/MySQL/Postgres 均兼容。
- **去重 / 开关语义**：每请求最多 1 条捕获（context 标记）；捕获开关独立于全局 `ErrorLogEnabled`。
