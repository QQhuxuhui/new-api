package common

import "testing"

func TestMaskUpstreamSensitiveError_PreservesLastRequestId(t *testing.T) {
	input := "用户额度不足, 剩余额度: -0.01 (request id: upstream123) (request id: downstream456)"
	got := MaskUpstreamSensitiveError(input)
	want := "模型负载过高，请稍后重试 (request id: downstream456)"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
