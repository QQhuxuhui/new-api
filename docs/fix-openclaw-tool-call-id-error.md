# 修复 OpenClaw 工具调用 ID 格式错误

## 问题

OpenClaw 调用时报错：
```
Invalid 'input[2].id': 'callhNZ5vIxhxhA6BlDrQdD0Tsp4'. Expected an ID that begins with 'fc'.
```

## 原因

OpenAI Responses API 要求所有 input 项的 `id` 字段必须以 `fc_` 开头，但 `function_call_output` 项缺少 `id` 字段。

## 修复

### 1. buildToolOutputItem 添加 id 字段

```go
func buildToolOutputItem(msg dto.Message) map[string]interface{} {
    item := map[string]interface{}{
        "type":   "function_call_output",
        "output": text,
    }
    if msg.ToolCallId != "" {
        originalCallID := ConvertCallIDFromOpenAIFormat(msg.ToolCallId)
        item["call_id"] = originalCallID
        item["id"] = ConvertCallIDToOpenAIFormat(originalCallID)  // 新增
    }
    return item
}
```

### 2. NormalizeResponsesInputToolCallIDs 处理 function_call_output

```go
if itemType == "function_call" || itemType == "function_call_output" {
    // 确保 id 字段是 fc_ 格式
}
```

## 测试

重新编译部署后，OpenClaw 的工具调用应该不再报错。
