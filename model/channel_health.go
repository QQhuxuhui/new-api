package model

import (
	"context"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/go-redis/redis/v8"
)

// Redis keys written by service/channel_health.go. Selection only reads the
// suspended and warning flags, so the formats live here to avoid an import
// cycle with the service package.
const (
	ChannelHealthSuspendedKey = "channel:health:%d:suspended"
	ChannelHealthWarningKey   = "channel:health:%d:warning"
)

// alwaysHealthyChannels holds the channels with setting.always_healthy in
// memory-cache mode, rebuilt with the channel cache.
var alwaysHealthyChannels atomic.Pointer[map[int]bool]

func rebuildAlwaysHealthyChannels(channels map[int]*Channel) {
	set := make(map[int]bool)
	for id, channel := range channels {
		if channel != nil && channel.GetSetting().AlwaysHealthy {
			set[id] = true
		}
	}
	alwaysHealthyChannels.Store(&set)
}

// IsChannelAlwaysHealthy reports whether the channel opted out of health
// warning and suspension. Without the memory cache it reads the database, so
// callers only use it for channels that already carry a health flag.
func IsChannelAlwaysHealthy(channelID int) bool {
	if common.MemoryCacheEnabled {
		if set := alwaysHealthyChannels.Load(); set != nil {
			return (*set)[channelID]
		}
		return false
	}
	channel, err := GetChannelById(channelID, true)
	if err != nil || channel == nil {
		return false
	}
	return channel.GetSetting().AlwaysHealthy
}

type channelHealthFlags struct {
	suspended bool
	warning   bool
}

// channelHealthLookupTimeout bounds the Redis lookup made while selecting a
// channel. Memory-cache selection holds the channel cache read lock during it,
// so a slow Redis must not stall selection or cache refreshes.
const channelHealthLookupTimeout = 200 * time.Millisecond

// loadChannelHealthFlags reads the suspended and warning flags of the
// candidates in one Redis round trip. Without Redis, or on a Redis error or
// timeout, every channel counts as healthy so selection fails open.
func loadChannelHealthFlags(channelIDs []int) map[int]channelHealthFlags {
	flags := make(map[int]channelHealthFlags, len(channelIDs))
	if common.RDB == nil || len(channelIDs) == 0 {
		return flags
	}
	ctx, cancel := context.WithTimeout(context.Background(), channelHealthLookupTimeout)
	defer cancel()
	pipe := common.RDB.Pipeline()
	suspended := make([]*redis.IntCmd, len(channelIDs))
	warning := make([]*redis.IntCmd, len(channelIDs))
	for i, id := range channelIDs {
		suspended[i] = pipe.Exists(ctx, fmt.Sprintf(ChannelHealthSuspendedKey, id))
		warning[i] = pipe.Exists(ctx, fmt.Sprintf(ChannelHealthWarningKey, id))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		common.SysLog(common.LogText("channel health lookup failed, selecting without health: %v", err))
		return map[int]channelHealthFlags{}
	}
	for i, id := range channelIDs {
		state := channelHealthFlags{
			suspended: suspended[i].Val() > 0,
			warning:   warning[i].Val() > 0,
		}
		if (state.suspended || state.warning) && IsChannelAlwaysHealthy(id) {
			continue
		}
		if state.suspended || state.warning {
			flags[id] = state
		}
	}
	return flags
}

// ChannelPick is what a request already knows when it selects a channel:
// the channels it has tried, the warning channels whose probe roll already
// missed during this request, and whether this pass ignores the warning roll.
type ChannelPick struct {
	Tried          map[int]bool
	WarningMissed  map[int]bool
	IncludeWarning bool
}

// pickPriorityLevel walks the priority levels, highest first, and returns the
// candidates of the first level that still has an untried channel that is not
// suspended. Each warning channel rolls against WarningProbePercent; a channel
// whose roll misses is skipped and reported, so the request leaves it for the
// pass that ignores the roll after every other candidate is exhausted.
// Suspended channels are never returned: nil means nothing is left to try.
func pickPriorityLevel(levels [][]int, pick ChannelPick, flags map[int]channelHealthFlags) (selected []int, missed []int) {
	for _, level := range levels {
		var kept []int
		for _, id := range level {
			state := flags[id]
			if pick.Tried[id] || state.suspended {
				continue
			}
			if state.warning && !pick.IncludeWarning {
				if pick.WarningMissed[id] {
					continue
				}
				if rand.Intn(100) >= common.WarningProbePercent {
					missed = append(missed, id)
					continue
				}
			}
			kept = append(kept, id)
		}
		if len(kept) > 0 {
			return kept, missed
		}
	}
	return nil, missed
}

// IsChannelSuspended reports whether health tracking currently suspends the
// channel. Always-healthy channels and an unavailable Redis count as not
// suspended, the same as during selection.
func IsChannelSuspended(channelID int) bool {
	return loadChannelHealthFlags([]int{channelID})[channelID].suspended
}
