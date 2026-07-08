package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// 渠道级配置错误（model_mapping 解析失败、param_override 非法）是 channel: 前缀错误码，
// 摘除 SkipRetry 后应走 shouldRetry 的 IsChannelError 必重试分支切换渠道。
func TestShouldRetryChannelConfigErrorsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayClientErrorTestDB(t)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	for _, code := range []types.ErrorCode{
		types.ErrorCodeChannelModelMappedError,
		types.ErrorCodeChannelParamOverrideInvalid,
	} {
		err := types.NewError(errors.New("unmarshal_model_mapping_failed"), code)
		if !types.IsChannelError(err) {
			t.Fatalf("expected %s to be a channel error", code)
		}
		if !shouldRetry(c, err, 2) {
			t.Fatalf("expected %s to trigger channel failover", code)
		}
	}
}

// 转换失败等非 channel: 错误码保留 SkipRetry，仍应被 Priority 1 短路，
// 避免用户输入触发的确定性失败在全部渠道上重放并污染健康统计。
func TestShouldRetrySkipRetryStillShortCircuitsConvertErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayClientErrorTestDB(t)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	for _, code := range []types.ErrorCode{
		types.ErrorCodeConvertRequestFailed,
		types.ErrorCodeInvalidApiType,
	} {
		err := types.NewError(errors.New("unsupported parameter"), code, types.ErrOptionWithSkipRetry())
		if shouldRetry(c, err, 2) {
			t.Fatalf("expected skip-retry %s to stop the retry loop", code)
		}
	}
}

// 配置错误重试期间预扣费不受影响由 Relay 的 defer 保证；此处兜底验证
// SkipRetry 摘除后错误仍会被记录错误日志（管理员定位配置问题的主要途径）。
func TestChannelConfigErrorStillRecordsErrorLog(t *testing.T) {
	err := types.NewError(errors.New("model_mapping_contains_cycle"), types.ErrorCodeChannelModelMappedError)
	if !types.IsRecordErrorLog(err) {
		t.Fatalf("expected channel config errors to keep error-log recording")
	}
	_ = model.ChannelDisableRule{} // keep model import for test db helper parity
}
