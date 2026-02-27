# Fix Context Canceled Retry Bug

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修复客户端断开连接后，所有重试因复用已取消的 context 而瞬间失败的 bug。

**Architecture:** 在 `doRequest` 中，发请求前检测下游 context 是否已取消，若已取消则提前返回带 `skipRetry` 标记的错误，阻止无意义的重试。同时在重试循环入口处增加 context 取消检测，避免进入后续的跨计划/钱包降级流程。

**Tech Stack:** Go, gin, net/http, context

---

## 问题分析

`relay/channel/api_request.go:454` 将下游 `c.Request.Context()` 绑定到上游请求：

```go
req = req.WithContext(c.Request.Context())
```

当客户端断开时，这个 context 被 cancel。重试循环中每次调用 `doRequest` 都复用同一个已 cancel 的 context，导致 `client.Do(req)` 瞬间返回 `context canceled`，所有重试全部无效，且错误被记录到渠道健康统计中，导致健康渠道被误判暂停。

## 修复策略

不改变 context 绑定机制（它对首次请求是正确的），而是：

1. 在 `doRequest` 发请求前检测 context 是否已取消，若已取消则返回带 `skipRetry` 标记的错误
2. 在重试循环入口处检测 context 取消，提前退出

---

### Task 1: 在 doRequest 中增加 context 取消的提前检测

**Files:**
- Modify: `relay/channel/api_request.go:450-455`
- Modify: `types/error.go` (新增 ErrorCode)

**Step 1: 在 `types/error.go` 新增 ErrorCode**

在 `ErrorCodeDoRequestFailed` 下方添加：

```go
ErrorCodeContextCanceled    ErrorCode = "context_canceled"
```

**Step 2: 修改 `doRequest` 函数**

将 `relay/channel/api_request.go` 第 450-455 行：

```go
// Bind upstream request lifecycle to downstream request context:
// - client disconnect / reverse proxy timeout should cancel upstream ASAP
// This is critical to avoid goroutine/connection buildup when upstream is slow.
if c != nil && c.Request != nil {
    req = req.WithContext(c.Request.Context())
}
```

替换为：

```go
// Bind upstream request lifecycle to downstream request context:
// - client disconnect / reverse proxy timeout should cancel upstream ASAP
// This is critical to avoid goroutine/connection buildup when upstream is slow.
if c != nil && c.Request != nil {
    // 如果下游 context 已经取消（客户端断开），直接返回不可重试的错误，
    // 避免重试循环中所有请求因复用已取消的 context 而瞬间失败。
    if err := c.Request.Context().Err(); err != nil {
        return nil, types.NewError(
            fmt.Errorf("downstream context already canceled: %w", err),
            types.ErrorCodeContextCanceled,
            types.ErrOptionWithSkipRetry(),
            types.ErrOptionWithHideErrMsg("client disconnected"),
        )
    }
    req = req.WithContext(c.Request.Context())
}
```

**Step 3: 运行现有测试确认不破坏**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go test ./relay/channel/ -run TestDoRequest -v -count=1`
Expected: PASS（现有测试中 context 在请求发出后才 cancel，不受影响）

**Step 4: Commit**

```bash
git add types/error.go relay/channel/api_request.go
git commit -m "fix: detect canceled context before upstream request to prevent futile retries"
```

---

### Task 2: 在重试循环入口处检测 context 取消

**Files:**
- Modify: `controller/relay.go:217` (重试循环开头)

**Step 1: 在重试循环开头添加 context 检测**

在 `controller/relay.go` 第 217 行 `for priorityIndex := ...` 循环体内部最开头（第 218 行之前），添加：

```go
// 客户端已断开，不再重试，避免无意义的上游请求和错误的渠道健康记录
if c.Request.Context().Err() != nil {
    logger.LogWarn(c, "client disconnected, stopping retry loop")
    break
}
```

**Step 2: 在跨计划降级前也添加检测**

在 `controller/relay.go` 第 301 行 `if newAPIError != nil {`（跨计划降级入口）之前添加：

```go
// 客户端已断开，跳过跨计划和钱包降级
if c.Request.Context().Err() != nil {
    logger.LogWarn(c, "client disconnected, skipping cross-plan and wallet fallback")
}
```

并将后续的 `if newAPIError != nil {` 改为 `if newAPIError != nil && c.Request.Context().Err() == nil {`

同样，钱包降级处（约第 388 行）的条件也加上 `&& c.Request.Context().Err() == nil`。

**Step 3: Commit**

```bash
git add controller/relay.go
git commit -m "fix: skip retry and fallback when client context is canceled"
```

---

### Task 3: 编写针对 bug 修复的测试

**Files:**
- Modify: `relay/channel/api_request_test.go`

**Step 1: 添加测试：context 已取消时 doRequest 应立即返回 skipRetry 错误**

在 `api_request_test.go` 末尾添加：

```go
func TestDoRequest_AlreadyCanceledContext_ReturnsSkipRetryError(t *testing.T) {
	service.InitHttpClient()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called when context is already canceled")
	}))
	t.Cleanup(upstream.Close)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 创建一个已经取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	c.Request = httptest.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", nil).WithContext(ctx)

	upstreamReq, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{},
		},
	}

	start := time.Now()
	_, err = DoRequest(c, upstreamReq, info)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 应该在极短时间内返回（不应该尝试连接上游）
	if elapsed > 100*time.Millisecond {
		t.Fatalf("expected immediate return, took %v", elapsed)
	}

	// 验证错误带有 skipRetry 标记
	var apiErr *types.NewAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected NewAPIError, got %T: %v", err, err)
	}
	if !types.IsSkipRetryError(apiErr) {
		t.Fatal("expected skipRetry error, but IsSkipRetryError returned false")
	}
}
```

需要在 import 中添加：
```go
"errors"
"github.com/QuantumNous/new-api/types"
```

**Step 2: 运行测试**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go test ./relay/channel/ -run TestDoRequest -v -count=1`
Expected: 两个测试都 PASS

**Step 3: Commit**

```bash
git add relay/channel/api_request_test.go
git commit -m "test: add test for context-canceled skip-retry behavior"
```

---

### Task 4: 验证编译和全量测试

**Step 1: 编译检查**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./...`
Expected: 无错误

**Step 2: 运行相关测试**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go test ./relay/channel/ ./controller/ ./types/ -v -count=1 2>&1 | tail -30`
Expected: 全部 PASS
