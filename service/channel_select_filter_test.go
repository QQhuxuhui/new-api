package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

// 两个 selectFrom 闭包是全仓唯一进 model 层选路的入口，六条调用路径都靠
// 这个函数把 distributor 算好的档位翻译成过滤条件；它返回 nil 就等于全链路失效
func TestChannelSelectFilterFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	if filter := channelSelectFilterFromContext(nil); filter != nil {
		t.Fatalf("nil context should yield nil filter, got %+v", filter)
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if filter := channelSelectFilterFromContext(c); filter != nil {
		t.Fatalf("context without tier should yield nil filter, got %+v", filter)
	}

	common.SetContextKey(c, constant.ContextKeyImageSizeTier, "")
	if filter := channelSelectFilterFromContext(c); filter != nil {
		t.Fatalf("empty tier must fail open (nil filter), got %+v", filter)
	}

	common.SetContextKey(c, constant.ContextKeyImageHighQuality, true)
	qualityFilter := channelSelectFilterFromContext(c)
	if qualityFilter == nil || !qualityFilter.ImageHighQuality || qualityFilter.ImageSizeTier != "" {
		t.Fatalf("filter = %+v, want independent ImageHighQuality constraint", qualityFilter)
	}
	if !qualityFilter.Active() {
		t.Fatal("high-quality-only filter must be Active")
	}

	common.SetContextKey(c, constant.ContextKeyImageSizeTier, "4K")
	filter := channelSelectFilterFromContext(c)
	if filter == nil || filter.ImageSizeTier != "4K" || !filter.ImageHighQuality {
		t.Fatalf("filter = %+v, want ImageSizeTier=4K plus ImageHighQuality", filter)
	}
	if !filter.Active() {
		t.Fatal("filter with a tier must be Active")
	}
}
