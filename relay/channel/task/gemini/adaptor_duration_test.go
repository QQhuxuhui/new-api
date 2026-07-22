package gemini

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// BuildRequestBody 发给上游的 durationSeconds 必须与计费用的
// ResolveVeoDuration 结果一致，否则会出现按 4 秒计费、上游按默认 8 秒生成的偏差。
func TestBuildRequestBody_DurationMatchesBilledSeconds(t *testing.T) {
	cases := []struct {
		name string
		req  relaycommon.TaskSubmitReq
		want int
	}{
		{"seconds string field", relaycommon.TaskSubmitReq{Prompt: "p", Seconds: "4", Size: "1280x720"}, 4},
		{"duration int field", relaycommon.TaskSubmitReq{Prompt: "p", Duration: 6}, 6},
		{"default matches billing default", relaycommon.TaskSubmitReq{Prompt: "p"}, 8},
		{"metadata wins", relaycommon.TaskSubmitReq{Prompt: "p", Seconds: "4", Metadata: map[string]interface{}{"durationSeconds": 6}}, 6},
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
			var payload VeoRequestPayload
			if err := common.Unmarshal(data, &payload); err != nil {
				t.Fatalf("unmarshal payload failed: %v", err)
			}
			if payload.Parameters == nil || payload.Parameters.DurationSeconds != tc.want {
				t.Fatalf("expected durationSeconds %d, got %+v (body: %s)", tc.want, payload.Parameters, data)
			}

			billed := ResolveVeoDuration(tc.req.Metadata, tc.req.Duration, tc.req.Seconds)
			if payload.Parameters.DurationSeconds != billed {
				t.Fatalf("request duration %d diverges from billed duration %d", payload.Parameters.DurationSeconds, billed)
			}
		})
	}
}
