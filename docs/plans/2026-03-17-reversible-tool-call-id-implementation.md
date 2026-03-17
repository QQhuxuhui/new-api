# Reversible Tool Call ID Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Keep outward-facing `tool_call.id` values in `fc_` format while preserving the original Responses API `call_id` for round-trip tool execution.

**Architecture:** Replace the one-way hash conversion with a reversible `fc_`-prefixed encoding. Decode the ID only at the Chat-to-Responses boundary so upstream still receives the original `call_id`, and add regression tests for both assistant tool calls and tool output messages.

**Tech Stack:** Go, standard library `encoding/base64`, existing `dto` request/response types

---

### Task 1: Add regression coverage

**Files:**
- Create: `service/openaicompat/tool_call_id_compat_test.go`
- Test: `service/openaicompat/tool_call_id_compat_test.go`

**Step 1: Write the failing test**

Add tests that assert:
- assistant `tool_calls[].id` values produced with `ConvertCallIDToOpenAIFormat` are decoded back to the original Responses `call_id`
- tool message `tool_call_id` values are decoded back to the original Responses `call_id`
- already-compliant `fc_` IDs remain unchanged

**Step 2: Run test to verify it fails**

Run: `go test ./service/openaicompat -run 'Test(ChatCompletionsToResponsesRequest_DecodesToolCallIDs|ConvertCallIDFormat_RoundTrip)'`
Expected: FAIL because the current implementation forwards the encoded `fc_` ID back upstream.

### Task 2: Implement reversible conversion

**Files:**
- Modify: `service/openaicompat/responses_to_chat.go`
- Modify: `service/openaicompat/chat_to_responses.go`

**Step 1: Write minimal implementation**

Replace the one-way hash helper with a reversible encoder/decoder pair, then use the decoder when building Responses API `function_call` and `function_call_output` items.

**Step 2: Run focused tests**

Run: `go test ./service/openaicompat -run 'Test(ChatCompletionsToResponsesRequest_DecodesToolCallIDs|ConvertCallIDFormat_RoundTrip)'`
Expected: PASS

### Task 3: Verify package behavior

**Files:**
- Modify: `service/openaicompat/responses_to_chat.go`
- Modify: `service/openaicompat/chat_to_responses.go`
- Create: `service/openaicompat/tool_call_id_compat_test.go`

**Step 1: Run package verification**

Run: `go test ./service/openaicompat ./relay/channel/openai`
Expected: PASS
