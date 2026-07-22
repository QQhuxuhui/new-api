package gemini

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// 图生视频多图：req.Images 里的每一张都要转发到上游——
// 第一张放 instances[0].image，其余放 referenceImages。
func TestBuildRequestBody_ForwardsMultipleImages(t *testing.T) {
	// 三张不同颜色的 1x1 PNG 的 data URL
	imgs := []string{
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC",
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==",
	}
	req := relaycommon.TaskSubmitReq{Prompt: "animate", Seconds: "4", Size: "1280x720", Images: imgs}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", nil)
	c.Set("task_request", req)
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}

	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody returned error: %v", err)
	}
	data := readAll(t, body)
	var payload VeoRequestPayload
	if err := common.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	inst := payload.Instances[0]
	if inst.Image == nil {
		t.Fatalf("expected first image on instances[0].image, body: %s", data)
	}
	if len(inst.ReferenceImages) != 2 {
		t.Fatalf("expected 2 reference images, got %d, body: %s", len(inst.ReferenceImages), data)
	}
}

func TestBuildRequestBody_SingleImageHasNoReferenceImages(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt: "animate", Seconds: "4",
		Images: []string{"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC"},
	}
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", nil)
	c.Set("task_request", req)
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}

	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody returned error: %v", err)
	}
	var payload VeoRequestPayload
	if err := common.Unmarshal(readAll(t, body), &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if payload.Instances[0].Image == nil {
		t.Fatalf("expected image set")
	}
	if len(payload.Instances[0].ReferenceImages) != 0 {
		t.Fatalf("expected no reference images for single upload")
	}
}

func readAll(t *testing.T, r interface{ Read([]byte) (int, error) }) []byte {
	t.Helper()
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 512)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf
}
