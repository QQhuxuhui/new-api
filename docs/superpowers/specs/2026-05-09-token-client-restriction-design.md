# Token 级客户端限制设计

- 日期：2026-05-09
- 作用域：Token 级（`model.Token`）
- 验证强度档位：B —— UA 子串 + 辅助请求头组合校验
- 参考：`claude-relay-service` 的 per-key 客户端限制；本仓被 `2d0c2fd8` revert 掉的渠道级实现 + `fc3c826e` 归档的 token 级 openspec

---

## 1. 背景与定位

### 1.1 历史沿革

仓内此前存在过两版客户端限制实现：

- **渠道级**（commit `da87707c`）：`Channel.EnableClientRestriction` + `Channel.AllowedClients`，JSON 数组 + 子串包含匹配
- **Token 级**（commit `1c5d4907` + 归档 openspec `fc3c826e`）：`Token.ClientRestrictionEnabled` + `Token.AllowedClients`，换行分隔，三种匹配模式（精确 / 通配 / regex）

`2d0c2fd8` revert 整体撤销了"identity masquerade"栈，但保留了上述四个 DB 列以避免 migration churn。`clearMasqueradeLegacyFlags()` 一次性把存量数据清零，由 Option `MasqueradeLegacyFlagsCleared` 守护不再重跑。

### 1.2 本设计要解决什么

把客户端限制能力以**最简单可用**的形态恢复在 **Token 级**。验证强度定为 B 档：UA 子串匹配为主，对预设客户端附加请求头验证以提高伪造门槛。

### 1.3 安全边界声明

**这是软限制，不是安全边界**。任何客户端发出的 HTTP 信号都可以被另一个客户端 1:1 复刻：

- A 档（仅 UA）：5 秒 `curl -H` 即可绕过
- B 档（UA + 辅助头）：5 分钟抓包 + 加 2-3 个 `-H` 即可绕过
- C 档（B + 路径限定）：与本仓"同 token 跨接口格式"的能力不兼容，舍弃
- D 档（TLS 指纹）：本仓不自终结 TLS（`http.ListenAndServe` 走纯 HTTP），且 JA3/JA4 无法区分 Claude Code 与任意 Node.js 脚本，舍弃

本设计采纳 B 档。客户端限制的合理定位是 **"路由提示 / 使用规范"**，安全边界依赖 token 保密、IP 白名单、速率限制、审计日志的组合。

---

## 2. 数据模型

### 2.1 复用现有 Schema

`model/token.go:34-35` 已存在以下字段，本设计直接复用，**不做 DDL 变更**：

```go
ClientRestrictionEnabled bool    `json:"client_restriction_enabled" gorm:"default:0"`
AllowedClients           *string `json:"allowed_clients" gorm:"type:text;default:''"`
```

`Token.Update()`（`model/token.go:193-196`）的 `Select(...)` 调用已包含上述两列，无需修改。Redis 缓存层因 Token 整体序列化覆盖，亦无需改动。

### 2.2 `AllowedClients` 存储格式

JSON 字符串数组，元素带前缀区分预设与自定义：

```json
["preset:claude-code","preset:codex-cli","preset:gemini-cli","custom:my-internal-bot"]
```

| 前缀 | 语义 | 匹配方式 |
|---|---|---|
| `preset:<id>` | 后端硬编码的客户端指纹规则 | UA 子串 AND 辅助头校验 |
| `custom:<sub>` | 用户自由输入的 UA 子串 | 仅 UA 子串校验（无辅助头） |
| 无前缀（兼容） | 历史数据兜底 | 等价 `custom:<sub>` |

### 2.3 预设客户端指纹表

硬编码于 `middleware/client_restriction.go`：

```go
type clientFingerprint struct {
    UASubstrings []string     // 任一 contains 即满足 UA 条件
    AuxHeaders   []headerRule // 任一 match 即满足辅助头条件
}

type headerRule struct {
    Name     string
    Contains string // 空字符串表示"该 header 存在即可"
}

var presetFingerprints = map[string]clientFingerprint{
    "claude-code": {
        UASubstrings: []string{"claude-cli"},
        AuxHeaders: []headerRule{
            {Name: "anthropic-beta", Contains: ""},
            {Name: "x-app", Contains: "cli"},
        },
    },
    "codex-cli": {
        UASubstrings: []string{"codex_cli_rs", "codex/"},
        AuxHeaders: []headerRule{
            {Name: "originator", Contains: "codex_cli_rs"},
            {Name: "openai-beta", Contains: "responses"},
        },
    },
    "gemini-cli": {
        UASubstrings: []string{"GeminiCLI"},
        AuxHeaders: []headerRule{
            {Name: "x-goog-api-client", Contains: "genai-js"},
        },
    },
}
```

预设的选取依据：

- **Claude Code**：UA `claude-cli/x.y.z (external, cli)`；`anthropic-beta` 头几乎随每次请求附带；`x-app: cli` 是 Claude Code 自带 marker
- **Codex CLI**：UA 含 `codex_cli_rs`；`originator: codex_cli_rs` 几乎专属
- **Gemini CLI**：UA 含 `GeminiCLI`；`x-goog-api-client` 含 `genai-js`（Google SDK 通用）

每个预设的 AuxHeaders 列表内部为 OR；UA 与 AuxHeaders 之间为 AND。

---

## 3. 验证逻辑

### 3.1 新增文件 `middleware/client_restriction.go`

```go
package middleware

import (
    "encoding/json"
    "strings"

    "github.com/gin-gonic/gin"
)

func matchClient(c *gin.Context, allowedJSON *string) bool {
    if allowedJSON == nil || *allowedJSON == "" {
        return false
    }
    var entries []string
    if err := json.Unmarshal([]byte(*allowedJSON), &entries); err != nil {
        return false
    }
    ua := strings.ToLower(c.GetHeader("User-Agent"))
    if ua == "" {
        return false
    }
    for _, e := range entries {
        e = strings.TrimSpace(e)
        if e == "" {
            continue
        }
        switch {
        case strings.HasPrefix(e, "preset:"):
            if fp, ok := presetFingerprints[strings.TrimPrefix(e, "preset:")]; ok {
                if matchPreset(ua, c, fp) {
                    return true
                }
            }
        case strings.HasPrefix(e, "custom:"):
            sub := strings.ToLower(strings.TrimPrefix(e, "custom:"))
            if sub != "" && strings.Contains(ua, sub) {
                return true
            }
        default:
            sub := strings.ToLower(e)
            if strings.Contains(ua, sub) {
                return true
            }
        }
    }
    return false
}

func matchPreset(ua string, c *gin.Context, fp clientFingerprint) bool {
    uaHit := false
    for _, sub := range fp.UASubstrings {
        if strings.Contains(ua, strings.ToLower(sub)) {
            uaHit = true
            break
        }
    }
    if !uaHit {
        return false
    }
    if len(fp.AuxHeaders) == 0 {
        return true
    }
    for _, hr := range fp.AuxHeaders {
        v := c.GetHeader(hr.Name)
        if v == "" {
            continue
        }
        if hr.Contains == "" || strings.Contains(strings.ToLower(v), strings.ToLower(hr.Contains)) {
            return true
        }
    }
    return false
}
```

### 3.2 接入点 `middleware/auth.go`

`TokenAuth()` 中 IP 校验之后（当前 `auth.go:243-250` 之后）插入：

```go
if token.ClientRestrictionEnabled {
    if !matchClient(c, token.AllowedClients) {
        ua := c.Request.Header.Get("User-Agent")
        logger.LogWarn(c, fmt.Sprintf(
            "client restriction blocked: token=%d ua=%q ip=%s",
            token.Id, ua, c.ClientIP()))
        abortWithOpenAiMessage(c, http.StatusForbidden,
            "该令牌仅允许在指定客户端中使用")
        return
    }
}
```

执行顺序：token 验证 → IP 校验 → **客户端校验** → 用户/分组校验 → `c.Next()`。

### 3.3 失败语义（fail-closed）

启用限制后，下列情形一律 403：

| 情形 | 行为 |
|---|---|
| `AllowedClients` 为 `nil` 或空字符串 | 拒绝 |
| JSON 解析失败 | 拒绝 |
| `User-Agent` 头缺失或为空 | 拒绝 |
| 所有条目无一命中 | 拒绝 |

未启用限制（`ClientRestrictionEnabled == false`）则跳过整段，不读取 UA、不消耗任何资源。

### 3.4 错误响应与信息泄露护栏

- HTTP 状态码：403
- 响应体：`abortWithOpenAiMessage(c, http.StatusForbidden, "该令牌仅允许在指定客户端中使用")` 标准 OpenAI 错误体（`type: invalid_request_error`）
- 响应体**不**回显 UA、不回显白名单、不区分失败原因 —— 防止攻击者通过错误消息枚举允许的客户端

### 3.5 审计日志

失败时统一格式：

```
client restriction blocked: token=<id> ua=<quoted> ip=<ip>
```

通过 `logger.LogWarn(c, ...)` 写入。管理员据此反推真实 UA，决定加预设还是 custom 条目。

---

## 4. 前端 UI

### 4.1 位置

`web/src/components/table/tokens/modals/EditTokenModal.jsx` 的"访问限制"卡片（`allow_ips` 之后）。

### 4.2 预设映射常量

```jsx
const PRESET_CLIENTS = [
  { value: 'preset:claude-code', label: t('Claude Code') },
  { value: 'preset:codex-cli',   label: t('Codex CLI') },
  { value: 'preset:gemini-cli',  label: t('Gemini CLI') },
];
```

### 4.3 表单控件

```jsx
<Col span={24}>
  <Form.Switch
    field='client_restriction_enabled'
    label={t('客户端限制')}
    size='large'
    extraText={t('启用后，仅命中下方任一客户端指纹的请求会被放行')}
  />
</Col>
<Col span={24}>
  <Form.Select
    field='allowed_clients'
    label={t('允许的客户端')}
    placeholder={t('选择预设客户端或输入自定义 UA 子串后回车')}
    multiple
    allowCreate
    filter
    optionList={PRESET_CLIENTS}
    extraText={t(
      '预设会同时校验 User-Agent 和辅助请求头；自定义条目仅按 UA 子串匹配。启用限制但留空将拒绝所有请求。',
    )}
    showClear
    style={{ width: '100%' }}
    disabled={!values?.client_restriction_enabled}
    onChange={(arr) => {
      const normalized = (arr || []).map((v) => {
        if (typeof v !== 'string') return v;
        if (v.startsWith('preset:') || v.startsWith('custom:')) return v;
        return 'custom:' + v;
      });
      formApiRef.current?.setValue('allowed_clients', normalized);
    }}
    renderSelectedItem={(option) => {
      const v = String(option.value || '');
      if (v.startsWith('custom:')) {
        return { isRenderInTag: true, content: v.slice('custom:'.length) };
      }
      return { isRenderInTag: true, content: option.label };
    }}
  />
</Col>
```

### 4.4 JSON ↔ Array 转换

```js
// originInputs
client_restriction_enabled: false,
allowed_clients: [],

// 加载已有 token（GET 之后）
if (typeof data.allowed_clients === 'string' && data.allowed_clients) {
  try { data.allowed_clients = JSON.parse(data.allowed_clients); }
  catch { data.allowed_clients = []; }
} else {
  data.allowed_clients = [];
}

// 提交前（PUT/POST 前）
localInputs.allowed_clients = JSON.stringify(localInputs.allowed_clients || []);
```

### 4.5 用户体验

| 用户操作 | 表单 state | UI 标签 |
|---|---|---|
| 勾选 "Claude Code" | `preset:claude-code` | `Claude Code` |
| 输入 `my-bot` 回车 | `custom:my-bot` | `my-bot` |
| 三项混合 | `["preset:claude-code","preset:codex-cli","custom:my-bot"]` | 三个 tag |

`custom:` 前缀对用户透明。

### 4.6 列表/详情展示

不在 token 列表新增展示列，与 `allow_ips` 保持一致。详情和编辑入口仅在弹窗内。

---

## 5. 测试计划

### 5.1 单元测试 `middleware/client_restriction_test.go`

| 用例 | 期望 |
|---|---|
| `AllowedClients` 为 `nil` | `false` |
| JSON `[]` | `false` |
| JSON 解析失败 | `false` |
| UA 为空 | `false` |
| `preset:claude-code`，UA = `claude-cli/1.0.0`，无辅助头 | `false` |
| `preset:claude-code`，UA = `claude-cli/1.0.0`，`anthropic-beta: prompt-caching-2024-07-31` | `true` |
| `preset:claude-code`，UA = `claude-cli/1.0.0`，`x-app: cli` | `true` |
| `preset:claude-code`，UA = `Mozilla/5.0`，`x-app: cli` | `false`（UA 不命中） |
| `preset:codex-cli`，UA = `codex_cli_rs/0.5`，`originator: codex_cli_rs` | `true` |
| `preset:codex-cli`，UA = `codex_cli_rs/0.5`，`openai-beta: responses=experimental` | `true` |
| `preset:gemini-cli`，UA = `GeminiCLI/v0.1.0 (linux; x64)`，`x-goog-api-client: genai-js/0.21.0` | `true` |
| `custom:my-bot`，UA = `My-Bot/1.0` | `true` |
| `custom:my-bot`，UA = `claude-cli/1.0` | `false` |
| 同时配 `preset:claude-code` + `custom:my-bot`，UA = `My-Bot/1.0` | `true` |
| 大小写：`preset:claude-code` + `User-Agent: CLAUDE-CLI/1.0` + `anthropic-beta: x` | `true` |
| 兜底：无前缀条目，UA contains 命中 | `true` |

### 5.2 集成测试

`TokenAuth()` 路径下：

- `ClientRestrictionEnabled = false` → 不读取 UA，直接放行
- 启用 + 命中 → `c.Next()` 被调用
- 启用 + 未命中 → 写 403，后续中间件不执行；日志输出一行 warn

---

## 6. 兼容性与边界

| 项 | 说明 |
|---|---|
| DDL | 无。Token 表两列已存在（`ClientRestrictionEnabled`、`AllowedClients`）。 |
| `Token.Update()` 列 | 已包含两列（`model/token.go:195`），无需改。 |
| Redis 缓存 | Token 整体序列化覆盖，自动跟随。 |
| 存量行 | `clearMasqueradeLegacyFlags()` 已运行（Option key 守护），存量为 `false`/`''`，对应 UI 默认状态。 |
| WebSocket 路径 | `TokenAuth()` 已统一规整化 Authorization；启用限制时浏览器 UA 不会命中 CLI 预设，符合预期。 |
| 管理后台路径 | 走 `UserAuth/AdminAuth` + session，**不**经过 `TokenAuth()`，不受影响。 |
| `/v1/messages`、`/v1/responses`、`/v1beta/...` | 三个 API 都走 `TokenAuth()`，本设计统一覆盖，无需按 path 分支。 |
| 渠道级遗留列 | 不动。Channel 表 `enable_client_restriction` / `allowed_clients` / `masquerade_hash` 保持空、无 UI、无运行时引用。 |
| 渠道级未来重启用 | Schema 仍在；如未来需要可在另一个 spec 中独立设计，不与本 spec 耦合。 |
| 数据迁移 | 无。 |

---

## 7. 文件清单

| 文件 | 改动 | 说明 |
|---|---|---|
| `middleware/client_restriction.go` | 新建 | `matchClient` / `matchPreset` / `presetFingerprints` |
| `middleware/client_restriction_test.go` | 新建 | §5.1 用例覆盖 |
| `middleware/auth.go` | 修改 | `TokenAuth()` IP 校验后插入 §3.2 hook |
| `web/src/components/table/tokens/modals/EditTokenModal.jsx` | 修改 | originInputs 加 2 字段；新增 Switch + Select；JSON ↔ Array 转换 |
| `model/token.go` | 不改 | 列已存在，`Update()` 已含两列 |
| `model/channel.go` | 不改 | 渠道遗留字段保留 |
| `model/main.go` | 不改 | 旧清零迁移保持原样 |

---

## 8. 不在本设计范围内的事

- 渠道级客户端限制的恢复或清理
- TLS 指纹（JA3/JA4）能力
- 身份伪装（masquerade）相关功能（被 `2d0c2fd8` 撤销，不在本 spec 内重启）
- Sticky session（同次 revert 拆除，单独议题）
- 渠道级 `masquerade_hash` 列的清理或重启用
