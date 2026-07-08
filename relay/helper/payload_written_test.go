package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newHelperTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

func TestIsPayloadWrittenSemantics(t *testing.T) {
	// 未写任何字节
	c := newHelperTestContext()
	if IsPayloadWritten(c) {
		t.Fatal("fresh context should not be payload-written")
	}

	// 仅写出 ping 注释：不算 payload，可安全切换渠道重放
	c = newHelperTestContext()
	_ = PingData(c)
	if IsPayloadWritten(c) {
		t.Fatal("ping-only writes should not block cross-channel retry")
	}

	// 写出业务数据（SSE data 块）：必须阻止重试
	c = newHelperTestContext()
	_ = StringData(c, `{"choices":[]}`)
	if !IsPayloadWritten(c) {
		t.Fatal("StringData must mark payload written")
	}

	// ping 之后又写业务数据：仍必须阻止重试
	c = newHelperTestContext()
	_ = PingData(c)
	_ = StringData(c, `{"choices":[]}`)
	if !IsPayloadWritten(c) {
		t.Fatal("payload after ping must mark payload written")
	}

	// 未经标记的直接写出：保守视为已写（宁可少重试也不双写）
	c = newHelperTestContext()
	_, _ = c.Writer.Write([]byte("raw bytes"))
	if !IsPayloadWritten(c) {
		t.Fatal("unmarked raw writes must be treated as payload conservatively")
	}
}
