# /v1/responses API 转换功能设计

## 概述

为 dev 分支实现三个 API 格式转换能力：

1. `/v1/chat/completions` → `/v1/responses` 转换（策略驱动）
2. `/v1/messages` (Claude) → `/v1/responses` 转换（经 chat 中转）
3. `/v1/responses/compact` 独立端点

dev 分支已有完整的 `/v1/responses` 直通能力（路由、请求处理、流式/非流式响应、用量追踪），本设计在此基础上增加格式转换层。

## 架构设计

### 方案选择

采用**独立转换服务层**方案：创建 `service/openaicompat/` 包集中实现所有转换逻辑，与 relay/adaptor 解耦。

理由：
- 转换逻辑可独立测试
- 新增渠道适配器无需重复实现
- 与上游 main 分支架构一致，降低后续维护成本

### 数据流

```
┌─ chat/completions 请求 ─┐     ┌─ Claude /v1/messages 请求 ─┐
│  TextHelper()            │     │  ClaudeHelper()             │
│  ↓ 策略检查              │     │  ↓ 策略检查                 │
│  ShouldUseResponses()    │     │  ShouldUseResponses()       │
│  ↓                       │     │  ↓                          │
│  chatViaResponses()      │     │  ClaudeToOpenAI → chatViaR. │
└──────────┬───────────────┘     └──────────┬──────────────────┘
           │                                │
           ▼                                ▼
   ┌───────────────────────────────────────────────┐
   │  service/openaicompat/chat_to_responses.go    │
   │  ChatCompletionsToResponsesRequest()          │
   └───────────────────┬───────────────────────────┘
                       ▼
           adaptor.ConvertOpenAIResponsesRequest()
           adaptor.DoRequest()  → 上游 /v1/responses
                       ▼
   ┌───────────────────────────────────────────────┐
   │  service/openaicompat/responses_to_chat.go    │
   │  ResponsesToChatResponse() (非流式)            │
   │  ResponsesToChatStream()  (流式)               │
   └───────────────────┬───────────────────────────┘
                       ▼
              返回 chat/completions 或 Claude 格式
```

## 文件变更

### 新增文件

| 文件路径 | 职责 |
|---------|------|
| `service/openaicompat/chat_to_responses.go` | chat completions → responses 请求转换 |
| `service/openaicompat/responses_to_chat.go` | responses → chat completions 响应转换（流式+非流式） |
| `service/openaicompat/policy.go` | 策略评估逻辑（渠道+模型匹配） |
| `service/openaicompat/regex.go` | 带缓存的正则匹配工具 |
| `relay/chat_completions_via_responses.go` | relay 层 chat→responses 处理入口 |
| `relay/channel/openai/chat_via_responses.go` | OpenAI 适配器的 responses→chat 响应处理 |
| `dto/openai_responses_compact.go` | compact 端点请求/响应 DTO |
| `relay/channel/openai/relay_responses_compact.go` | compact 响应处理器 |
| `setting/ratio_setting/compact_suffix.go` | compact 后缀常量 |

### 修改文件

| 文件路径 | 改动内容 |
|---------|---------|
| `relay/compatible_handler.go` | TextHelper 入口增加策略判断和路由 |
| `relay/claude_handler.go` | ClaudeHelper 入口增加策略判断 |
| `relay/constant/relay_mode.go` | 新增 `RelayModeResponsesCompact`，更新 `Path2RelayMode` |
| `types/relay_format.go` | 新增 `RelayFormatOpenAIResponsesCompaction` |
| `router/relay-router.go` | 注册 `/v1/responses/compact` 路由 |
| `controller/relay.go` | 添加 compact 模式分发 |
| `middleware/distributor.go` | compact 端点模型名后缀追加 |
| `relay/helper/valid_request.go` | compact 请求校验 |
| `setting/model_setting/global.go` | 新增策略配置结构 |

## 请求转换逻辑 (chat → responses)

### 消息角色映射

| Chat Completions 角色 | Responses 格式 |
|----------------------|---------------|
| `system` / `developer` | 提取到 `instructions` 字段，多条用 `\n\n` 拼接 |
| `user` | `input[]` 中 `role: "user"` 条目 |
| `assistant` | `input[]` 中 `role: "assistant"` 条目 |
| `assistant` (含 tool_calls) | 拆分为多个 `function_call` 类型条目 |
| `tool` / `function` | `function_call_output` 类型条目，关联 `call_id` |

### 参数映射

| Chat Completions 字段 | Responses 字段 |
|----------------------|---------------|
| `messages` | `input` + `instructions` |
| `tools[].function` | `tools[]`（去掉外层 function 包装） |
| `tool_choice` | `tool_choice`（简化格式） |
| `response_format.json_schema` | `text.format` |
| `reasoning_effort` | `reasoning.effort` |
| `max_tokens` / `max_completion_tokens` | `max_output_tokens` |
| `temperature` | `temperature` |
| `top_p` | `top_p` |
| `stream` | `stream` |
| `model` | `model` |

### 多模态内容映射

| Chat 内容类型 | Responses 内容类型 |
|-------------|-------------------|
| `text` | `input_text` |
| `image_url` (URL) | `input_image` (URL) |
| `image_url` (base64) | `input_image` (base64 data) |

### messages → responses 转换

Claude `/v1/messages` 请求走三层链路：Claude → OpenAI Chat → Responses。

复用已有的 `ClaudeToOpenAIRequest()`（`service/convert.go`）完成第一层，再调用 `ChatCompletionsRequestToResponsesRequest()` 完成第二层。

## 响应转换逻辑 (responses → chat)

### 非流式响应

`ResponsesResponseToChatCompletionsResponse()` 映射规则：

| Responses Output 类型 | Chat Completion 映射 |
|----------------------|---------------------|
| `message` (含 `output_text`) | `choices[0].message.content`（多段拼接） |
| `function_call` | `choices[0].message.tool_calls[]` |
| `reasoning` | `choices[0].message.reasoning_content` |

Usage 映射：

| Responses 字段 | Chat Completion 字段 |
|---------------|---------------------|
| `input_tokens` | `prompt_tokens` |
| `output_tokens` | `completion_tokens` |
| `input_tokens + output_tokens` | `total_tokens` |
| `input_tokens_details.cached_tokens` | `prompt_tokens_details.cached_tokens` |
| `output_tokens_details.reasoning_tokens` | `completion_tokens_details.reasoning_tokens` |

Finish Reason 逻辑：
- 有 `function_call` 输出 → `tool_calls`
- 仅 text 输出 → `stop`
- text + tool_calls 共存 → `stop`（文本优先）

### 流式响应

SSE 事件实时转换为 chat completion chunks：

| Responses 流事件 | Chat Chunk 行为 |
|-----------------|----------------|
| `response.created` | 初始化响应元数据 |
| `response.output_text.delta` | content delta chunk |
| `response.reasoning_summary_text.delta` | reasoning content delta（段落间插入换行） |
| `response.output_item.added` (function_call) | tool_call 初始 chunk（含 name） |
| `response.function_call_arguments.delta` | tool arguments delta chunk |
| `response.output_item.done` | 标记 tool call 完成 |
| `response.completed` | final usage chunk + `[DONE]` |

Tool Call 状态追踪：
- 按 call_id 索引的 tool call map
- 每个 tool call 分配递增 index
- name 在 `output_item.added` 时发送一次
- arguments 分多个 delta 发送

格式回转：
- Claude 格式：转换后经 `ResponseOpenAI2Claude()` 回转
- Gemini 格式：经 Gemini 转换器回转

## 策略配置

### 数据结构

```go
type ChatCompletionsToResponsesPolicy struct {
    Enabled       bool     `json:"enabled"`        // 总开关
    AllChannels   bool     `json:"all_channels"`   // 适用所有渠道
    ChannelIDs    []int    `json:"channel_ids"`    // 指定渠道ID
    ChannelTypes  []int    `json:"channel_types"`  // 指定渠道类型
    ModelPatterns []string `json:"model_patterns"` // 模型名正则
}
```

### 评估逻辑

1. `Enabled` 为 true
2. 渠道匹配：`AllChannels=true` 或 渠道ID匹配 或 渠道类型匹配
3. 模型匹配：至少命中一个 `ModelPatterns` 正则
4. PassThrough 设置排除时跳过

正则匹配使用编译缓存避免重复编译。

## `/v1/responses/compact` 端点

### 路由

`POST /v1/responses/compact`，注册为 `RelayFormatOpenAIResponsesCompaction`。

路径检测优先级：`/v1/responses/compact` 在 `/v1/responses` 之前匹配。

### 请求 DTO

```go
type OpenAIResponsesCompactionRequest struct {
    Model              string          `json:"model"`
    Input              json.RawMessage `json:"input"`
    Instructions       json.RawMessage `json:"instructions"`
    PreviousResponseID string          `json:"previous_response_id"`
}
```

### 响应 DTO

```go
type OpenAIResponsesCompactionResponse struct {
    ID        string          `json:"id"`
    Object    string          `json:"object"`
    CreatedAt int64           `json:"created_at"`
    Output    json.RawMessage `json:"output"`
    Usage     CompactionUsage `json:"usage"`
    Error     json.RawMessage `json:"error,omitempty"`
}
```

### 定价机制

- 中间件检测路径，追加 `-openai-compact` 后缀到模型名
- 使用 compact 后缀模型定价扣费
- 处理完成后恢复原始模型名用于日志
- 仅 OpenAI 渠道真正支持此端点

## 错误处理

| 场景 | 处理方式 |
|------|---------|
| 策略未启用/不匹配 | 走正常 chat/completions 流程 |
| 请求转换失败 | 400 + 错误描述，跳过重试 |
| 上游返回非 200 | 现有 `RelayErrorHandler` 处理 |
| 响应转换失败 | 500 + 转换错误，记录日志 |
| 流式事件解析失败 | 跳过该事件，继续处理 |
| Usage 缺失 | 回退到文本长度估算 |
| compact 不支持的渠道 | 400 "endpoint not supported" |

## 测试计划

### 单元测试

`service/openaicompat/` 下的转换函数：
- 各种消息角色组合的映射正确性
- 多模态内容（图片、文件）的转换
- tool call 的双向转换
- JSON schema 的格式转换
- 边界情况（空消息、空 tools）

### 集成测试

通过渠道测试功能验证：
- chat→responses 非流式/流式
- messages→responses 非流式/流式
- compact 端点正常调用
- 策略配置生效/不生效
