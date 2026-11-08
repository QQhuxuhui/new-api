package model

import (
	"context"
	"fmt"
	"math/rand"
	"sync/atomic"

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

// loadChannelHealthFlags reads the suspended and warning flags of the
// candidates in one Redis round trip. Without Redis, or on a Redis error, every
// channel counts as healthy so selection fails open.
func loadChannelHealthFlags(channelIDs []int) map[int]channelHealthFlags {
	flags := make(map[int]channelHealthFlags, len(channelIDs))
	if common.RDB == nil || len(channelIDs) == 0 {
		return flags
	}
	ctx := context.Background()
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

// selectHealthyPriorityLevel picks the candidates for one retry. levels holds
// the channel IDs of each priority, highest first, built from every candidate
// so suspensions never shift which level a retry index points at. Starting at
// the retry's level it takes the first level that still has a channel that is
// not suspended; when all of them are suspended it keeps the retry's level as
// is, so a model never loses every channel at once. Warning channels in the
// chosen level are then thinned to their probe share.
func selectHealthyPriorityLevel(levels [][]int, retry int, flags map[int]channelHealthFlags) []int {
	start := min(retry, len(levels)-1)
	for _, level := range levels[start:] {
		healthy := make([]int, 0, len(level))
		for _, id := range level {
			if !flags[id].suspended {
				healthy = append(healthy, id)
			}
		}
		if len(healthy) > 0 {
			return thinOutWarningChannels(healthy, flags)
		}
	}
	return thinOutWarningChannels(levels[start], flags)
}

// thinOutWarningChannels lets each warning candidate through with probability
// WarningProbePercent so its recovery is still probed. When that leaves no
// candidate it keeps them all.
func thinOutWarningChannels(channelIDs []int, flags map[int]channelHealthFlags) []int {
	kept := make([]int, 0, len(channelIDs))
	for _, id := range channelIDs {
		if flags[id].warning && rand.Intn(100) >= common.WarningProbePercent {
			continue
		}
		kept = append(kept, id)
	}
	if len(kept) == 0 {
		return channelIDs
	}
	return kept
}
