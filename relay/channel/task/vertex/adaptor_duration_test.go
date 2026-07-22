package vertex

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	geminitask "github.com/QuantumNous/new-api/relay/channel/task/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// 与 gemini 适配器同理：上游请求的 durationSeconds 必须与计费一致。
func TestBuildRequestBody_DurationMatchesBilledSeconds(t *testing.T) {
	cases := []struct {
		name string
		req  relaycommon.TaskSubmitReq
		want int
	}{
		{"seconds string field", relaycommon.TaskSubmitReq{Prompt: "p", Seconds: "4", Size: "1280x720"}, 4},
		{"default matches billing default", relaycommon.TaskSubmitReq{Prompt: "p"}, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/videos", nil)
			c.Set("task_request", tc.req)
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}

			a := &TaskAdaptor{}
			body, err := a.BuildRequestBody(c, info)
			if err != nil {
				t.Fatalf("BuildRequestBody returned error: %v", err)
			}
			data, _ := io.ReadAll(body)
			var payload geminitask.VeoRequestPayload
			if err := common.Unmarshal(data, &payload); err != nil {
				t.Fatalf("unmarshal payload failed: %v", err)
			}
			if payload.Parameters == nil || payload.Parameters.DurationSeconds != tc.want {
				t.Fatalf("expected durationSeconds %d, got %+v (body: %s)", tc.want, payload.Parameters, data)
			}
		})
	}
}
