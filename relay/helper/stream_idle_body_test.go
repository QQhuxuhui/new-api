package helper

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

func newIdleTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	return c
}

// 原始扫描循环（ollama/zhipu/coze/tencent/cohere）通过包装 Body 获得空闲超时：
// 已读到完整事件后挂起 → 超时关闭 Body 解除阻塞，并按中途超时上报
func TestIdleTimeoutBodyUnblocksBlockedRead(t *testing.T) {
	c := newIdleTestContext(t)

	pr, pw := io.Pipe()
	resp := &http.Response{Body: pr}
	ApplyStreamIdleTimeout(c, resp, infoWithStreamTimeout(1))

	go func() { _, _ = pw.Write([]byte("data: x\n")) }()
	buf := make([]byte, 16)
	n, err := resp.Body.Read(buf)
	if n == 0 || err != nil {
		t.Fatalf("first read: n=%d err=%v", n, err)
	}
	// 中途超时的判据是已经向客户端写出业务事件，而不是底层只读到一行。
	MarkPayloadWritten(c)

	start := time.Now()
	_, err = resp.Body.Read(buf) // 无后续数据，等待 idle 超时
	if err == nil {
		t.Fatal("expected read to fail after idle timeout")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("read unblocked after %v, want ~1s", elapsed)
	}
	if !StreamIdleTimedOut(c) {
		t.Fatal("idle timeout should be reported as fired")
	}
	if !MidStreamTimeoutOccurred(c) {
		t.Fatal("a complete event was delivered, so this is a mid-stream timeout")
	}
	if err := RawStreamFirstByteTimeoutError(c); err != nil {
		t.Fatalf("mid-stream timeout must not surface as a first-byte error, got %v", err)
	}
	_ = resp.Body.Close()
}

// 完整行若被适配器丢弃（空行、心跳、非法 JSON 等），客户端仍未收到业务
// 事件；随后挂起应保持首事件超时语义，允许外层切换渠道。
func TestIdleTimeoutBodyCompleteUnrenderedLineIsFirstByteTimeout(t *testing.T) {
	c := newIdleTestContext(t)

	pr, pw := io.Pipe()
	resp := &http.Response{Body: pr}
	ApplyStreamIdleTimeout(c, resp, infoWithStreamTimeout(1))

	go func() { _, _ = pw.Write([]byte("data: not-json\n")) }()
	buf := make([]byte, 64)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if _, err := resp.Body.Read(buf); err == nil {
		t.Fatal("expected timeout after the unrendered line")
	}

	if MidStreamTimeoutOccurred(c) {
		t.Fatal("an unrendered line must not count as a mid-stream timeout")
	}
	timeoutErr := RawStreamFirstByteTimeoutError(c)
	if timeoutErr == nil || timeoutErr.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("unrendered line should preserve first-byte 504 semantics, got %v", timeoutErr)
	}
	_ = resp.Body.Close()
}

// 永不超时（0）：不包装 Body，行为与原来完全一致
func TestIdleTimeoutBodyNeverTimeoutLeavesBodyUntouched(t *testing.T) {
	c := newIdleTestContext(t)

	pr, _ := io.Pipe()
	resp := &http.Response{Body: pr}
	ApplyStreamIdleTimeout(c, resp, infoWithStreamTimeout(0))

	if resp.Body != io.ReadCloser(pr) {
		t.Fatal("never-timeout must not wrap the body")
	}
	if StreamIdleTimedOut(c) || MidStreamTimeoutOccurred(c) {
		t.Fatal("no timeout state expected")
	}
	_ = pr.Close()
}

// 计时按"完成一行"重置，不按"读到字节"重置：上游每隔小于超时值滴一个
// 非换行字节不能无限续命
func TestIdleTimeoutBodySlowDripWithoutNewlineStillTimesOut(t *testing.T) {
	c := newIdleTestContext(t)

	pr, pw := io.Pipe()
	resp := &http.Response{Body: pr}
	ApplyStreamIdleTimeout(c, resp, infoWithStreamTimeout(1))

	// 先发一行完整数据（重置一次计时）
	go func() { _, _ = pw.Write([]byte("data: x\n")) }()
	buf := make([]byte, 64)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatalf("first read: %v", err)
	}

	stopDrip := make(chan struct{})
	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if _, err := pw.Write([]byte("x")); err != nil {
					return
				}
			case <-stopDrip:
				return
			}
		}
	}()
	defer close(stopDrip)

	start := time.Now()
	for {
		if _, err := resp.Body.Read(buf); err != nil {
			break // 超时关闭 Body
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("slow drip without newline kept the stream alive past the timeout")
		}
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("timeout fired after %v, want ~1s", elapsed)
	}
	_ = resp.Body.Close()
}

// 半行首事件后停住：扫描器不会向客户端输出任何完整事件，因此必须按
// 首包超时处理（504 可切换渠道），而不是中途截断
func TestIdleTimeoutBodyPartialFirstEventIsFirstByteTimeout(t *testing.T) {
	c := newIdleTestContext(t)

	pr, pw := io.Pipe()
	resp := &http.Response{Body: pr}
	ApplyStreamIdleTimeout(c, resp, infoWithStreamTimeout(1))

	// 只发半行（无换行）后停住
	go func() { _, _ = pw.Write([]byte("data: {\"partial\":")) }()
	buf := make([]byte, 64)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if _, err := resp.Body.Read(buf); err == nil {
		t.Fatal("expected timeout on the stalled partial line")
	}

	if !StreamIdleTimedOut(c) {
		t.Fatal("idle timeout should be reported as fired")
	}
	if MidStreamTimeoutOccurred(c) {
		t.Fatal("a partial line is not a delivered event; must not count as mid-stream timeout")
	}
	timeoutErr := RawStreamFirstByteTimeoutError(c)
	if timeoutErr == nil || timeoutErr.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("partial first event should map to a 504 empty-stream error, got %v", timeoutErr)
	}
	_ = resp.Body.Close()
}

// 定时器不得在请求结束后继续存活：正常 EOF 或 Close 都要停表，
// 且回调本身绝不写 gin.Context（Context 会被 gin 复用给其他请求）
func TestIdleTimeoutBodyStopsTimerOnEOFAndClose(t *testing.T) {
	c := newIdleTestContext(t)

	pr, pw := io.Pipe()
	resp := &http.Response{Body: pr}
	ApplyStreamIdleTimeout(c, resp, infoWithStreamTimeout(1))
	wrapped, ok := resp.Body.(*idleTimeoutBody)
	if !ok {
		t.Fatalf("expected wrapped body, got %T", resp.Body)
	}

	go func() {
		_, _ = pw.Write([]byte("data: x\n"))
		_ = pw.Close() // 正常 EOF
	}()
	buf := make([]byte, 64)
	for {
		if _, err := resp.Body.Read(buf); err != nil {
			break
		}
	}
	if !wrapped.closed.Load() {
		t.Fatal("timer must be stopped once the stream ends, even without an explicit Close")
	}
	// EOF 后即使等过超时窗口也不得触发
	time.Sleep(1200 * time.Millisecond)
	if wrapped.fired.Load() {
		t.Fatal("timer fired after the stream already ended")
	}
	if err := resp.Body.Close(); err != nil && err != io.ErrClosedPipe {
		t.Fatalf("close: %v", err)
	}
}

// 重试复用同一个 gin.Context：新一次尝试必须从干净状态开始，
// 否则上一次的超时状态会让这次即使成功也被判成 504 / 错记渠道失败
func TestApplyStreamIdleTimeoutResetsPreviousAttemptState(t *testing.T) {
	c := newIdleTestContext(t)

	// 第一次尝试：中途超时
	pr1, pw1 := io.Pipe()
	resp1 := &http.Response{Body: pr1}
	ApplyStreamIdleTimeout(c, resp1, infoWithStreamTimeout(1))
	go func() { _, _ = pw1.Write([]byte("data: x\n")) }()
	buf := make([]byte, 64)
	_, _ = resp1.Body.Read(buf)
	MarkPayloadWritten(c)
	for {
		if _, err := resp1.Body.Read(buf); err != nil {
			break
		}
	}
	if !MidStreamTimeoutOccurred(c) {
		t.Fatal("first attempt should record a mid-stream timeout")
	}
	_ = resp1.Body.Close()

	// 第二次尝试：新包装器建立时必须清掉残留状态
	pr2, _ := io.Pipe()
	resp2 := &http.Response{Body: pr2}
	ApplyStreamIdleTimeout(c, resp2, infoWithStreamTimeout(60))
	if StreamIdleTimedOut(c) {
		t.Fatal("stale idle-timeout state leaked into the retry")
	}
	if MidStreamTimeoutOccurred(c) {
		t.Fatal("stale mid-stream state leaked into the retry")
	}
	if err := RawStreamFirstByteTimeoutError(c); err != nil {
		t.Fatalf("retry must not inherit the previous 504, got %v", err)
	}
	_ = resp2.Body.Close()

	// ResetStreamTimeoutState 同样能清掉扫描器路径写入的标记
	common.SetContextKey(c, constant.ContextKeyMidStreamTimeout, true)
	ResetStreamTimeoutState(c)
	if MidStreamTimeoutOccurred(c) {
		t.Fatal("ResetStreamTimeoutState must clear the scanner-path flag")
	}
}
