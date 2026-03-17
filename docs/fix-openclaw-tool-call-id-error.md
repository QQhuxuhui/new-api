# 修复 OpenClaw 工具调用 ID 格式错误

## 问题描述

OpenClaw 用户在调用 GPT-5.4 模型接口时遇到错误：

```
HTTP 400: Invalid 'input[4].id': 'callJUbXhgF7Trf966NxflYuip2S'. Expected an ID that begins with 'fc'.
```

## 根本原因

OpenAI Chat Completions API 要求工具调用的 `tool_call_id` 必须以 `fc_` 开头。

当平台使用 Responses API 作为后端，然后转换回 Chat Completions API 格式时，直接透传了 Responses API 的 `call_id`，而没有转换为 OpenAI 兼容的格式。

## 解决方案

在 Responses API 转换为 Chat Completions API 时，将原始 `call_id` 编码成 `fc_` 前缀格式；在客户端提交工具结果时，再把这个 ID 还原为原始 `call_id` 发回 Responses API。

### 修改的文件

1. `service/openaicompat/responses_to_chat.go`
   - 添加 `ConvertCallIDToOpenAIFormat()` 函数
   - 添加 `ConvertCallIDFromOpenAIFormat()` 函数
   - 在非流式响应转换中应用 ID 转换

2. `relay/channel/openai/chat_via_responses.go`
   - 在流式响应转换中应用 ID 转换

3. `service/openaicompat/chat_to_responses.go`
   - 在 assistant tool call 和 tool output 回传时，将 `fc_` ID 还原为原始 `call_id`

### ID 转换逻辑

使用可逆的 Base64URL 编码生成 `fc_` 前缀 ID，既满足 OpenAI 格式要求，也保留与原始 Responses `call_id` 的映射关系：

```go
func ConvertCallIDToOpenAIFormat(callID string) string {
    if strings.HasPrefix(callID, "fc_") {
        return callID
    }
    return "fc_map_" + base64.RawURLEncoding.EncodeToString([]byte(callID))
}

func ConvertCallIDFromOpenAIFormat(callID string) string {
    if !strings.HasPrefix(callID, "fc_map_") {
        return callID
    }
    decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(callID, "fc_map_"))
    if err != nil {
        return callID
    }
    return string(decoded)
}
```

## 影响范围

此修复仅影响使用 Responses API 作为后端的模型（如 GPT-5.4），不影响其他模型的正常使用。

## 测试建议

1. 使用 OpenClaw 调用带工具调用的请求
2. 验证返回的 `tool_call_id` 以 `fc_` 开头
3. 验证后续的工具结果提交不再报错
4. 验证多轮 tool calling 时，上游收到的仍是原始 `call_id`
