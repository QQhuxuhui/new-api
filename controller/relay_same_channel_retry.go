package controller

import (
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// retryRuleLookup returns the in-place retry budget for (statusCode, message).
// The bool indicates whether a retry rule matched; count/intervalMs are only
// meaningful when matched is true. Values are expected to be pre-clamped.
type retryRuleLookup func(statusCode int, message string) (count int, intervalMs int, matched bool)

// defaultRetryRuleLookup is the production lookup backed by channel_disable_rules.
func defaultRetryRuleLookup(statusCode int, message string) (int, int, bool) {
	rule := model.MatchRetryRule(statusCode, message)
	if rule == nil {
		return 0, 0, false
	}
	count, intervalMs := rule.ClampedRetryBudget()
	if count <= 0 {
		return 0, 0, false
	}
	return count, intervalMs, true
}

// executeSameChannelRetry runs doCall once. On error, if lookup reports a
// matching retry rule with a positive count (and the response has not started
// streaming), it sleeps intervalMs between attempts and retries on the same
// channel up to count additional times.
//
// The final outcome (success or the most recent error) is returned unchanged,
// so callers can feed it into the existing cross-channel failover / health
// tracking pipeline. During in-place retries no side-effects (health stats,
// failover bookkeeping) are performed here — callers should only observe the
// final result.
func executeSameChannelRetry(
	c *gin.Context,
	lookup retryRuleLookup,
	doCall func() *types.NewAPIError,
) *types.NewAPIError {
	err := doCall()
	if err == nil {
		return nil
	}

	// Cannot retry if bytes have already been written to the client
	// (e.g. an SSE stream that emitted a chunk before failing).
	if c.Writer != nil && c.Writer.Written() {
		return err
	}

	count, intervalMs, matched := lookup(err.StatusCode, err.Error())
	if !matched || count <= 0 {
		return err
	}

	interval := time.Duration(intervalMs) * time.Millisecond

	for i := 0; i < count; i++ {
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

		// Bail out if the response started writing between attempts.
		if c.Writer != nil && c.Writer.Written() {
			return err
		}

		if next := doCall(); next == nil {
			return nil
		} else {
			err = next
		}
	}

	return err
}
