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

// 审查问题：健康记录必须先排除 SkipRetry 错误。
// 复现审查场景：管理员配置了匹配 400 的故障转移（server）规则时，
// 非法图片输入这类 400 + SkipRetry 的请求级错误也绝不能计入渠道健康，
// 否则重复的非法请求会给正常渠道累计失败甚至触发暂停。
func TestShouldRecordChannelFailureExcludesSkipRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayClientErrorTestDB(t)

	rule := &model.ChannelDisableRule{
		Name:        "failover-on-400",
		StatusCodes: []int{400},
		MatchType:   model.MatchTypeOR,
		Enabled:     true,
		Priority:    10,
		ErrorType:   model.RuleErrorTypeServer,
	}
	if err := model.DB.Create(rule).Error; err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}
	model.InvalidateDisableRulesCache()

	// 规则命中：不带 SkipRetry 的上游 400 会被记录（现状语义）
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	recordable := types.NewErrorWithStatusCode(errors.New("upstream rejected"),
		types.ErrorCodeBadResponse, http.StatusBadRequest)
	if !shouldRecordChannelFailure(c, recordable) {
		t.Fatal("400 matching an admin failover rule should be recorded when not SkipRetry")
	}

	// 同样命中规则的 400，带 SkipRetry 后必须被排除
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	skip := types.NewErrorWithStatusCode(errors.New("bad image input"),
		types.ErrorCodeBadResponse, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	if shouldRecordChannelFailure(c2, skip) {
		t.Fatal("SkipRetry error must never be recorded to channel health, even when an admin rule matches 400")
	}

	if shouldRecordChannelFailure(c2, nil) {
		t.Fatal("nil error must not be recorded")
	}

	// 无完整错误对象的路径（midjourney/task）沿用原始判定
	c3, _ := gin.CreateTestContext(httptest.NewRecorder())
	c3.Request = httptest.NewRequest("POST", "/mj/submit/imagine", nil)
	if !shouldRecordChannelFailureRaw(c3, http.StatusGatewayTimeout, "upstream timeout") {
		t.Fatal("raw variant should keep recording 504 timeouts")
	}
}

// 只有明确标记为本地准备阶段的转换失败才不得计入渠道健康；转换错误码本身
// 不能作为豁免依据，因为部分适配器会在转换阶段调用所选渠道上传文件。
func TestShouldRecordChannelFailureUsesExplicitHealthExemption(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayClientErrorTestDB(t)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", nil)
	channelConvertErr := types.NewErrorWithStatusCode(errors.New("replicate upload failed: HTTP 500"),
		types.ErrorCodeConvertRequestFailed, http.StatusInternalServerError)
	if !shouldRecordChannelFailure(c, channelConvertErr) {
		t.Fatal("unmarked conversion failures must remain eligible for channel health recording")
	}

	localPrepareErr := types.NewErrorWithStatusCode(errors.New("failed to download source image: timeout"),
		types.ErrorCodeConvertRequestFailed, http.StatusInternalServerError,
		types.ErrOptionWithNoRecordChannelHealth())
	if shouldRecordChannelFailure(c, localPrepareErr) {
		t.Fatal("explicitly exempt local preparation errors must not be recorded")
	}
}
