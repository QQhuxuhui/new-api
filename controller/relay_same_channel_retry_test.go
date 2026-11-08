package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayWithSameChannelRetry(t *testing.T) {
	require.NoError(t, operation_setting.ChannelErrorRulesFromString(`[
		{"id":"retry","name":"upstream busy","keywords":["upstream busy"],"match_type":"KEYWORD_ONLY","enabled":true,"error_type":"client","retry_count":2},
		{"id":"slow","name":"slow retry","keywords":["slow retry"],"match_type":"KEYWORD_ONLY","enabled":true,"error_type":"client","retry_count":2,"retry_interval_ms":30000}
	]`))
	t.Cleanup(func() {
		require.NoError(t, operation_setting.ChannelErrorRulesFromString(operation_setting.DefaultChannelErrorRulesToString()))
	})
	busy := func() *types.NewAPIError {
		return types.NewOpenAIError(errors.New("upstream busy"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable)
	}

	for _, test := range []struct {
		name         string
		results      []*types.NewAPIError
		writeFirst   bool
		pin          *dto.ChannelPin
		cancelled    bool
		wantAttempts int
		wantErr      bool
	}{
		{name: "matched rule succeeds on second attempt", results: []*types.NewAPIError{busy(), nil}, wantAttempts: 2},
		{name: "matched rule stops after retry count", results: []*types.NewAPIError{busy(), busy(), busy(), nil}, wantAttempts: 3, wantErr: true},
		{
			name:         "unmatched error is not replayed",
			results:      []*types.NewAPIError{types.NewOpenAIError(errors.New("other"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable), nil},
			wantAttempts: 1, wantErr: true,
		},
		{
			name:         "skip-retry error is not replayed",
			results:      []*types.NewAPIError{types.NewOpenAIError(errors.New("upstream busy"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry()), nil},
			wantAttempts: 1, wantErr: true,
		},
		{
			name:         "broken response body is not replayed",
			results:      []*types.NewAPIError{types.NewOpenAIError(errors.New("upstream busy"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError), nil},
			wantAttempts: 1, wantErr: true,
		},
		{
			name:         "replay stops when a retry returns a broken response body",
			results:      []*types.NewAPIError{busy(), types.NewOpenAIError(errors.New("upstream busy"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError), nil},
			wantAttempts: 2, wantErr: true,
		},
		{
			name:         "single-attempt pinned request is not replayed",
			results:      []*types.NewAPIError{busy(), nil},
			pin:          &dto.ChannelPin{ChannelId: 1, Source: dto.PinSourceToken, Rank: dto.PinRankToken, RetryMode: dto.PinRetrySingleAttempt},
			wantAttempts: 1, wantErr: true,
		},
		{
			name:         "same-channel pinned request is still replayed",
			results:      []*types.NewAPIError{busy(), nil},
			pin:          &dto.ChannelPin{ChannelId: 1, Source: dto.PinSourceOriginTask, Rank: dto.PinRankOriginTask, RetryMode: dto.PinRetrySameChannel},
			wantAttempts: 2,
		},
		{name: "written response is not replayed", results: []*types.NewAPIError{busy(), nil}, writeFirst: true, wantAttempts: 1, wantErr: true},
		{
			name:         "client disconnect during the interval stops replay",
			results:      []*types.NewAPIError{types.NewOpenAIError(errors.New("slow retry"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable), nil},
			cancelled:    true,
			wantAttempts: 1, wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
			if test.pin != nil {
				service.GetChannelConstraints(c).AddPin(*test.pin)
			}
			if test.cancelled {
				cancel()
			}

			attempts := 0
			err := relayWithSameChannelRetry(c, func() *types.NewAPIError {
				result := test.results[attempts]
				attempts++
				if test.writeFirst {
					_, _ = c.Writer.WriteString("data: partial\n\n")
				}
				return result
			})

			assert.Equal(t, test.wantAttempts, attempts)
			assert.Equal(t, test.wantErr, err != nil)
		})
	}
}
