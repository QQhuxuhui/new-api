package model

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const (
	BatchUpdateTypeUserQuota = iota
	BatchUpdateTypeTokenQuota
	BatchUpdateTypeUsedQuota
	BatchUpdateTypeChannelUsedQuota
	BatchUpdateTypeRequestCount
	BatchUpdateTypeCount // if you add a new type, you need to add a new map and a new lock
)

var batchUpdateStores []map[int]int
var batchUpdateLocks []sync.Mutex

// pollingIndexStore stores the latest polling index for each channel (uses override, not accumulation)
var pollingIndexStore = make(map[int]int) // channelId -> pollingIndex
var pollingIndexLock sync.Mutex

func init() {
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateStores = append(batchUpdateStores, make(map[int]int))
		batchUpdateLocks = append(batchUpdateLocks, sync.Mutex{})
	}
}

func InitBatchUpdater() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Duration(common.BatchUpdateInterval) * time.Second)
			batchUpdate()
		}
	})
	// Periodically cleanup stale channel polling locks (every hour)
	gopool.Go(func() {
		for {
			time.Sleep(1 * time.Hour)
			CleanupChannelPollingLocks()
			common.SysLog("channel polling locks cleanup completed")
		}
	})
}

func addNewRecord(type_ int, id int, value int) {
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	if _, ok := batchUpdateStores[type_][id]; !ok {
		batchUpdateStores[type_][id] = value
	} else {
		batchUpdateStores[type_][id] += value
	}
}

// UpdatePollingIndexAsync queues a polling index update for async batch persistence
// If batch update is disabled, it falls back to synchronous database update
func UpdatePollingIndexAsync(channelId int, index int) {
	if !common.BatchUpdateEnabled {
		// Batch update disabled, fall back to sync update
		if err := updateChannelPollingIndex(channelId, index); err != nil {
			common.SysLog(fmt.Sprintf("failed to sync update polling index: channel_id=%d, error=%v", channelId, err))
		}
		return
	}
	pollingIndexLock.Lock()
	defer pollingIndexLock.Unlock()
	pollingIndexStore[channelId] = index
}

func batchUpdate() {
	// check if there's any data to update
	hasData := false
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		if len(batchUpdateStores[i]) > 0 {
			hasData = true
			batchUpdateLocks[i].Unlock()
			break
		}
		batchUpdateLocks[i].Unlock()
	}

	// also check polling index store
	pollingIndexLock.Lock()
	hasPollingData := len(pollingIndexStore) > 0
	pollingIndexLock.Unlock()

	if !hasData && !hasPollingData {
		return
	}

	common.SysLog("batch update started")
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		store := batchUpdateStores[i]
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()

		if len(store) == 0 {
			continue
		}

		// Update records one by one; failed records are re-queued for next batch
		for key, value := range store {
			var err error
			switch i {
			case BatchUpdateTypeUserQuota:
				err = increaseUserQuota(key, value)
				if err != nil {
					common.SysLog("failed to batch update user quota: " + err.Error())
				}
			case BatchUpdateTypeTokenQuota:
				err = increaseTokenQuota(key, value)
				if err != nil {
					common.SysLog("failed to batch update token quota: " + err.Error())
				}
			case BatchUpdateTypeUsedQuota:
				updateUserUsedQuota(key, value)
			case BatchUpdateTypeRequestCount:
				updateUserRequestCount(key, value)
			case BatchUpdateTypeChannelUsedQuota:
				updateChannelUsedQuota(key, value)
			}
			// Re-queue failed records for retry in next batch
			if err != nil {
				addNewRecord(i, key, value)
			}
		}
	}

	// Persist polling indexes
	pollingIndexLock.Lock()
	pollingStore := pollingIndexStore
	pollingIndexStore = make(map[int]int)
	pollingIndexLock.Unlock()
	for channelId, index := range pollingStore {
		err := updateChannelPollingIndex(channelId, index)
		if err != nil {
			common.SysLog("failed to batch update polling index: " + err.Error())
		}
	}

	common.SysLog("batch update finished")
}

// updateChannelPollingIndex persists the polling index for a channel to database
func updateChannelPollingIndex(channelId int, index int) error {
	var channel Channel
	err := DB.Select("id", "channel_info").First(&channel, channelId).Error
	if err != nil {
		return err
	}
	channel.ChannelInfo.MultiKeyPollingIndex = index
	return DB.Model(&channel).Update("channel_info", channel.ChannelInfo).Error
}

func RecordExist(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func shouldUpdateRedis(fromDB bool, err error) bool {
	return common.RedisEnabled && fromDB && err == nil
}
