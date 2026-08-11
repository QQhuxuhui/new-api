package helper

import (
	"bytes"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// ApplyStreamIdleTimeout 给不经过 StreamScannerHandler 的原始 scanner.Scan()
// 流式循环（ollama/zhipu/coze/tencent/cohere 等）补上渠道级/全局空闲超时：
// 空闲超过阈值即关闭上游 Body，让阻塞中的 Read/Scan 立即返回。
//
// 超时状态只保存在包装器自身的原子变量里，定时器回调**绝不写 gin.Context**
// ——gin 在请求结束后会把 Context 放回池子复用，跨请求写入会污染其他请求。
// 调用方通过 StreamIdleTimedOut / MidStreamTimeoutOccurred 同步查询。
func ApplyStreamIdleTimeout(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) {
	if c == nil || resp == nil || resp.Body == nil {
		return
	}
	// 每次尝试都以干净状态开始：重试复用同一个 gin.Context，
	// 上一次尝试的超时状态不能影响这一次
	ResetStreamTimeoutState(c)

	timeout := resolveStreamTimeout(info)
	if timeout == neverStreamTimeout {
		return
	}
	body := &idleTimeoutBody{
		inner:   resp.Body,
		timeout: timeout,
	}
	body.timer = time.AfterFunc(timeout, body.onIdleTimeout)
	resp.Body = body
	c.Set(string(constant.ContextKeyStreamIdleTimeoutState), body)
}

// ResetStreamTimeoutState 清除本请求已记录的流式超时状态。
// 每次上游尝试开始前调用，避免重试读到上一次尝试的残留状态。
func ResetStreamTimeoutState(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(string(constant.ContextKeyStreamIdleTimeoutState), (*idleTimeoutBody)(nil))
	c.Set(string(constant.ContextKeyMidStreamTimeout), false)
}

type idleTimeoutBody struct {
	inner   io.ReadCloser
	timer   *time.Timer
	timeout time.Duration

	fired  atomic.Bool
	closed atomic.Bool
}

func (b *idleTimeoutBody) onIdleTimeout() {
	b.fired.Store(true)
	// 只关闭上游连接解除阻塞；状态留在原子变量里由读取侧同步查询
	_ = b.inner.Close()
}

func (b *idleTimeoutBody) Read(p []byte) (int, error) {
	n, err := b.inner.Read(p)
	if n > 0 {
		// 按“完成一行/事件”重置计时，而不是按“读到任意字节”——否则上游可以
		// 每隔略小于超时值滴一个字节、永不换行，无限延长请求
		if bytes.IndexByte(p[:n], '\n') >= 0 {
			if !b.fired.Load() && !b.closed.Load() {
				b.timer.Reset(b.timeout)
			}
		}
	}
	if err != nil {
		// 流已结束（EOF 或出错）：立即停表，避免调用方忘记 Close 时定时器
		// 空转到超时上限（最长 7 天）
		b.stopTimer()
	}
	return n, err
}

func (b *idleTimeoutBody) Close() error {
	b.stopTimer()
	return b.inner.Close()
}

func (b *idleTimeoutBody) stopTimer() {
	if b.closed.CompareAndSwap(false, true) {
		b.timer.Stop()
	}
}

func streamIdleTimeoutState(c *gin.Context) *idleTimeoutBody {
	if c == nil {
		return nil
	}
	value, ok := c.Get(string(constant.ContextKeyStreamIdleTimeoutState))
	if !ok {
		return nil
	}
	body, _ := value.(*idleTimeoutBody)
	return body
}

// StreamIdleTimedOut 报告原始扫描循环的空闲超时是否已触发（含首包超时）
func StreamIdleTimedOut(c *gin.Context) bool {
	body := streamIdleTimeoutState(c)
	return body != nil && body.fired.Load()
}

// MidStreamTimeoutOccurred 报告本次尝试是否发生了"已输出部分内容后超时"。
// 覆盖两条流式路径：StreamScannerHandler 写 ContextKeyMidStreamTimeout；
// 原始扫描循环则要求包装器已超时且 PayloadWritten 已置位。完整但被丢弃的
// 空行、心跳或非法事件不算中途输出，仍保留首事件超时的重试能力。
func MidStreamTimeoutOccurred(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if common.GetContextKeyBool(c, constant.ContextKeyMidStreamTimeout) {
		return true
	}
	body := streamIdleTimeoutState(c)
	return body != nil && body.fired.Load() && common.GetContextKeyBool(c, constant.ContextKeyPayloadWritten)
}

// RawStreamFirstByteTimeoutError 原始扫描循环的收尾判定：空闲超时已触发且
// 尚未向客户端写出业务事件（首包超时）时返回空流 504 错误（可切换渠道）；
// 已输出过内容的中途超时返回 nil，由调用方按既有语义收尾计费，
// 渠道健康由外层依据 MidStreamTimeoutOccurred 改记失败。
func RawStreamFirstByteTimeoutError(c *gin.Context) *types.NewAPIError {
	body := streamIdleTimeoutState(c)
	if body == nil || !body.fired.Load() || common.GetContextKeyBool(c, constant.ContextKeyPayloadWritten) {
		return nil
	}
	return NewEmptyStreamError(StreamExitTimeout)
}
