# Channel User Prompt Injection — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow channel administrators to configure a custom user prompt that is always injected as the first `role=user` message (after any system messages) in every request routed through that channel.

**Architecture:** Add a `UserPrompt` field to `ChannelSettings`. In each relay handler (OpenAI-compatible, Claude native, Gemini), after the existing SystemPrompt injection block, insert the UserPrompt as a new user message at the front of the message list. Frontend gets a new TextArea in the channel settings form.

**Tech Stack:** Go (Gin framework), React (Semi Design)

---

### Task 1: Add UserPrompt field to ChannelSettings

**Files:**
- Modify: `dto/channel_settings.go:3-12`

**Step 1: Add the field**

In `dto/channel_settings.go`, add `UserPrompt` after `SystemPromptOverride`:

```go
type ChannelSettings struct {
	ForceFormat                  bool   `json:"force_format,omitempty"`
	ThinkingToContent            bool   `json:"thinking_to_content,omitempty"`
	Proxy                        string `json:"proxy"`
	PassThroughBodyEnabled       bool   `json:"pass_through_body_enabled,omitempty"`
	PassThroughMetadataMasquerade bool   `json:"pass_through_metadata_masquerade,omitempty"`
	SystemPrompt                 string `json:"system_prompt,omitempty"`
	SystemPromptOverride         bool   `json:"system_prompt_override,omitempty"`
	UserPrompt                   string `json:"user_prompt,omitempty"`
}
```

**Step 2: Verify build**

Run: `go build ./dto/...`
Expected: Success, no errors.

**Step 3: Commit**

```bash
git add dto/channel_settings.go
git commit -m "feat: add UserPrompt field to ChannelSettings"
```

---

### Task 2: Add UserPrompt injection in compatible_handler.go (OpenAI + Claude paths)

**Files:**
- Modify: `relay/compatible_handler.go:158` (after the SystemPrompt `}` closing brace at line 158)

**Step 1: Add injection logic**

After the closing `}` of the `if info.ChannelSetting.SystemPrompt != ""` block (line 158) and before `jsonData, err := common.Marshal(convertedRequest)` (line 160), insert:

```go
		// 注入渠道自定义用户提示词
		if info.ChannelSetting.UserPrompt != "" {
			if openaiReq, ok := convertedRequest.(*dto.GeneralOpenAIRequest); ok {
				userMessage := dto.Message{
					Role:    "user",
					Content: info.ChannelSetting.UserPrompt,
				}
				// 插入到 system 消息之后、其他消息之前
				insertIdx := 0
				for i, msg := range openaiReq.Messages {
					if msg.Role == openaiReq.GetSystemRoleName() {
						insertIdx = i + 1
						break
					}
				}
				openaiReq.Messages = append(openaiReq.Messages[:insertIdx], append([]dto.Message{userMessage}, openaiReq.Messages[insertIdx:]...)...)
			} else if claudeReq, ok := convertedRequest.(*dto.ClaudeRequest); ok {
				userMessage := dto.ClaudeMessage{
					Role:    "user",
					Content: info.ChannelSetting.UserPrompt,
				}
				claudeReq.Messages = append([]dto.ClaudeMessage{userMessage}, claudeReq.Messages...)
			}
		}
```

**Step 2: Verify build**

Run: `go build ./relay/...`
Expected: Success.

**Step 3: Commit**

```bash
git add relay/compatible_handler.go
git commit -m "feat: inject UserPrompt in compatible handler (OpenAI + Claude paths)"
```

---

### Task 3: Add UserPrompt injection in claude_handler.go

**Files:**
- Modify: `relay/claude_handler.go:108` (after the SystemPrompt block closing `}` at line 108)

**Step 1: Add injection logic**

After the closing `}` of `if info.ChannelSetting.SystemPrompt != ""` (line 108) and before `var requestBody io.Reader` (line 110), insert:

```go
	// 注入渠道自定义用户提示词
	if info.ChannelSetting.UserPrompt != "" {
		userMessage := dto.ClaudeMessage{
			Role:    "user",
			Content: info.ChannelSetting.UserPrompt,
		}
		request.Messages = append([]dto.ClaudeMessage{userMessage}, request.Messages...)
	}
```

**Step 2: Verify build**

Run: `go build ./relay/...`
Expected: Success.

**Step 3: Commit**

```bash
git add relay/claude_handler.go
git commit -m "feat: inject UserPrompt in Claude native handler"
```

---

### Task 4: Add UserPrompt injection in gemini_handler.go

**Files:**
- Modify: `relay/gemini_handler.go:136` (after the SystemInstructions cleanup block ending at line 136)

**Step 1: Add injection logic**

After the `if !hasContent { ... }` block (line 136) and before `var requestBody io.Reader` (line 138), insert:

```go
	// 注入渠道自定义用户提示词
	if info.ChannelSetting.UserPrompt != "" {
		userMessage := dto.GeminiChatContent{
			Role:  "user",
			Parts: []dto.GeminiPart{{Text: info.ChannelSetting.UserPrompt}},
		}
		request.Contents = append([]dto.GeminiChatContent{userMessage}, request.Contents...)
	}
```

**Step 2: Verify build**

Run: `go build ./relay/...`
Expected: Success.

**Step 3: Commit**

```bash
git add relay/gemini_handler.go
git commit -m "feat: inject UserPrompt in Gemini handler"
```

---

### Task 5: Add UserPrompt TextArea to frontend channel settings form

**Files:**
- Modify: `web/src/components/table/channels/modals/EditChannelModal.jsx:337-344` (state init)
- Modify: `web/src/components/table/channels/modals/EditChannelModal.jsx:3538` (after system_prompt_override Switch)

**Step 1: Add `user_prompt` to channelSettings state**

At line 343, after `system_prompt: '',` add:

```javascript
    user_prompt: '',
```

So the state becomes:
```javascript
  const [channelSettings, setChannelSettings] = useState({
    force_format: false,
    thinking_to_content: false,
    proxy: '',
    pass_through_body_enabled: false,
    pass_through_metadata_masquerade: false,
    system_prompt: '',
    user_prompt: '',
  });
```

**Step 2: Add TextArea form field**

After the `system_prompt_override` `<Form.Switch>` closing tag (line 3538) and before `</Card>` (line 3539), insert:

```jsx
                    <Form.TextArea
                      field='user_prompt'
                      label={t('用户提示词')}
                      placeholder={t(
                        '输入用户提示词，将作为第一条用户消息插入到请求中',
                      )}
                      onChange={(value) =>
                        handleChannelSettingsChange('user_prompt', value)
                      }
                      autosize
                      showClear
                      extraText={t(
                        '始终生效：此提示词将作为一条独立的用户消息插入到所有用户消息之前',
                      )}
                    />
```

**Step 3: Verify frontend builds**

Run: `cd web && npm run build` (or check for syntax errors)
Expected: Build success.

**Step 4: Commit**

```bash
git add web/src/components/table/channels/modals/EditChannelModal.jsx
git commit -m "feat: add UserPrompt TextArea to channel settings UI"
```

---

### Task 6: Final build verification

**Step 1: Full Go build**

Run: `go build ./...`
Expected: Success.

**Step 2: Run existing tests**

Run: `go test ./relay/... -v -count=1`
Expected: All existing tests pass.

**Step 3: Final commit (if any formatting needed)**

```bash
git add -A
git commit -m "chore: format and cleanup"
```

---

## Manual Testing Checklist

1. Create/edit a channel, set `user_prompt` to "你好，请用中文回答"
2. Send a request through the channel with a normal user message
3. Verify the API receives two user messages: the injected one first, then the original
4. Test with OpenAI-compatible, Claude native, and Gemini channel types
5. Test with empty `user_prompt` — should have no effect
6. Test with both `system_prompt` and `user_prompt` set — both should inject correctly
