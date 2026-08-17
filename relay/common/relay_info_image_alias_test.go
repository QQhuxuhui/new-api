package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

func TestGenRelayInfoImageCanonicalizesEditsAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/edits?trace=1", nil)

	info := GenRelayInfoImage(c, &dto.ImageRequest{Model: "gpt-image-2"})
	if info.RelayMode != relayconstant.RelayModeImagesEdits {
		t.Fatalf("relay mode = %d, want RelayModeImagesEdits", info.RelayMode)
	}
	if info.RequestURLPath != "/v1/images/edits?trace=1" {
		t.Fatalf("request URL path = %q, want canonical image edits path", info.RequestURLPath)
	}
}
