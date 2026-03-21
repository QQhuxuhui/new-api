package service

import (
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func TestGenerateClaudeOtherInfoDoesNotAddTotalInputTokens(t *testing.T) {
	ctx, _ := gin.CreateTestContext(nil)
	info := GenerateClaudeOtherInfo(
		ctx,
		&relaycommon.RelayInfo{
			StartTime:         time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC),
			FirstResponseTime: time.Date(2026, 3, 19, 10, 0, 1, 0, time.UTC),
			ChannelMeta:       &relaycommon.ChannelMeta{},
		},
		1.0,
		1.0,
		1.0,
		120,
		0.1,
		80,
		1.25,
		30,
		1.25,
		50,
		2.0,
		-1,
		1.0,
	)

	if _, ok := info["total_input_tokens"]; ok {
		t.Fatalf("expected total_input_tokens to be omitted")
	}
}
