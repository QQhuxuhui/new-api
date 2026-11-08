package controller

import (
	"time"

	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// relayWithSameChannelRetry runs attempt once and, when the failure matches a
// channel error rule with RetryCount > 0, replays it on the same channel up to
// RetryCount more times, waiting RetryIntervalMs between tries. Only the final
// outcome is returned, so the cross-channel retry decision, channel health and
// auto-disable observe a single attempt per channel.
//
// A replay is skipped once response payload reached the client, for
// non-retryable errors, and after the client disconnects.
func relayWithSameChannelRetry(c *gin.Context, attempt func() *types.NewAPIError) *types.NewAPIError {
	err := attempt()
	if err == nil || types.IsSkipRetryError(err) || helper.ResponseBusinessStarted(c) {
		return err
	}
	rule := operation_setting.MatchChannelErrorRule(err.StatusCode, err.RuleMatchInput())
	if rule == nil || rule.RetryCount <= 0 {
		return err
	}
	interval := time.Duration(rule.RetryIntervalMs) * time.Millisecond
	for range rule.RetryCount {
		if interval > 0 {
			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
			case <-c.Request.Context().Done():
				timer.Stop()
				return err
			}
		} else if c.Request.Context().Err() != nil {
			return err
		}
		if helper.ResponseBusinessStarted(c) {
			return err
		}
		err = attempt()
		if err == nil || types.IsSkipRetryError(err) || helper.ResponseBusinessStarted(c) {
			return err
		}
	}
	return err
}
