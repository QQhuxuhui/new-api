# 渠道自定义用户提示词

## 概述

为渠道设置新增 `UserPrompt` 字段，允许管理员配置自定义的用户提示词。该提示词始终作为一条独立的 `role=user` 消息插入到消息列表最前面（system 消息之后、用户原有消息之前）。

## 需求

- 新增 `user_prompt` 字段到渠道设置
- 始终插入，无需覆盖开关
- 对所有渠道类型（OpenAI、Claude、Gemini）生效
- 前端提供 TextArea 输入

## 数据模型

`dto/channel_settings.go` 新增字段：

```go
UserPrompt string `json:"user_prompt,omitempty"`
```

无需数据库迁移（ChannelSettings 存储为 JSON）。

## 注入逻辑

在 SystemPrompt 注入逻辑之后，当 `UserPrompt != ""` 时：

### OpenAI 兼容路径 (compatible_handler.go)

- `*dto.GeneralOpenAIRequest`：找到第一个 system 消息的位置，在其之后插入 `{Role: "user", Content: userPrompt}`；无 system 消息则插入到列表最前面
- `*dto.ClaudeRequest`：在 `Messages` 最前面插入 `ClaudeMessage{Role: "user", Content: userPrompt}`

### Claude 原生路径 (claude_handler.go)

在 `request.Messages` 最前面插入 `ClaudeMessage{Role: "user", Content: userPrompt}`

### Gemini 路径 (gemini_handler.go)

在 `request.Contents` 最前面插入 `GeminiChatContent{Role: "user", Parts: [{Text: userPrompt}]}`

## 前端

`EditChannelModal.jsx`：

1. `channelSettings` state 新增 `user_prompt: ''`
2. 在系统提示词拼接 Switch 之后新增 TextArea 表单项

## 涉及文件

| 文件 | 改动 |
|------|------|
| `dto/channel_settings.go` | 新增 `UserPrompt` 字段 |
| `relay/compatible_handler.go` | UserPrompt 注入逻辑 |
| `relay/claude_handler.go` | UserPrompt 注入逻辑 |
| `relay/gemini_handler.go` | UserPrompt 注入逻辑 |
| `web/.../EditChannelModal.jsx` | 表单 UI |
