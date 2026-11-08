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
// non-retryable errors and broken response bodies, and after the client
// disconnects.
func relayWithSameChannelRetry(c *gin.Context, attempt func() *types.NewAPIError) *types.NewAPIError {
	err := attempt()
	if !canReplayOnSameChannel(c, err) {
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
		if !canReplayOnSameChannel(c, err) {
			return err
		}
	}
	return err
}

// canReplayOnSameChannel reports whether a failed attempt may be sent again.
// A broken response body may follow a completed upstream generation, so
// replaying it could bill the upstream twice.
func canReplayOnSameChannel(c *gin.Context, err *types.NewAPIError) bool {
	return err != nil &&
		!types.IsSkipRetryError(err) &&
		!operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode()) &&
		!helper.ResponseBusinessStarted(c)
}
