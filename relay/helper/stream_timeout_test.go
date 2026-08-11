package helper

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 渠道级流式超时：未设置回退全局；N 秒生效；0（或负数）永不超时；
// 全局 STREAMING_TIMEOUT=0 也按永不超时处理（修复 NewTicker(0) panic 隐患）
func TestResolveStreamTimeout(t *testing.T) {
	prev := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = prev })

	// nil info / 无 ChannelMeta / 未设置渠道超时 → 全局默认
	if got := resolveStreamTimeout(nil); got != 300*time.Second {
		t.Fatalf("nil info = %v, want 300s", got)
	}
	if got := resolveStreamTimeout(&relaycommon.RelayInfo{}); got != 300*time.Second {
		t.Fatalf("no channel meta = %v, want 300s", got)
	}
	noOverride := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	if got := resolveStreamTimeout(noOverride); got != 300*time.Second {
		t.Fatalf("no override = %v, want 300s", got)
	}

	// 渠道级自定义秒数
	custom := 30
	withCustom := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &custom},
	}}
	if got := resolveStreamTimeout(withCustom); got != 30*time.Second {
		t.Fatalf("custom = %v, want 30s", got)
	}

	// 渠道级 0 → 永不超时
	zero := 0
	withZero := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &zero},
	}}
	if got := resolveStreamTimeout(withZero); got != neverStreamTimeout {
		t.Fatalf("zero = %v, want never", got)
	}

	// 全局 0 → 永不超时
	constant.StreamingTimeout = 0
	if got := resolveStreamTimeout(&relaycommon.RelayInfo{}); got != neverStreamTimeout {
		t.Fatalf("global zero = %v, want never", got)
	}

	// 超限钳制：绕过校验的超大正数被钳制到上限（7 天），不会静默变成
	// 永不超时，更不会乘法溢出 panic
	huge := 9223372037 // > MaxInt64 / 1e9
	withHuge := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &huge},
	}}
	if got := resolveStreamTimeout(withHuge); got != time.Duration(constant.MaxStreamTimeoutSeconds)*time.Second {
		t.Fatalf("oversized value = %v, want clamp to %ds", got, constant.MaxStreamTimeoutSeconds)
	}
}
