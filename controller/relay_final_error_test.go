package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"
)

func TestResolveFinalRelayError(t *testing.T) {
	upstream429 := types.NewErrorWithStatusCode(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	exhausted := types.NewError(errors.New("获取分组 default 下模型 gpt 的可用渠道耗尽"), types.ErrorCodeGetChannelFailed)
	canceled := types.NewError(errors.New("client disconnected"), types.ErrorCodeContextCanceled)

	// 渠道耗尽 → 换回最后一次真实上游错误（保留 A 的状态码）
	if got := resolveFinalRelayError(exhausted, upstream429); got != upstream429 {
		t.Fatalf("expected exhaustion token replaced by last upstream error")
	}
	// 没有上游错误可换（如首轮就耗尽）→ 保留耗尽错误
	if got := resolveFinalRelayError(exhausted, nil); got != exhausted {
		t.Fatalf("expected exhaustion error kept when no upstream error recorded")
	}
	// 最终错误本身就是真实错误（SetupContext 渠道错误 / 上游错误）→ 不替换
	if got := resolveFinalRelayError(upstream429, exhausted); got != upstream429 {
		t.Fatalf("expected non-token final error kept as-is")
	}
	// 客户端断开错误不受影响
	if got := resolveFinalRelayError(canceled, upstream429); got != canceled {
		t.Fatalf("expected context-canceled error kept as-is")
	}
	// 成功路径
	if got := resolveFinalRelayError(nil, upstream429); got != nil {
		t.Fatalf("expected nil final error to stay nil")
	}
}
