package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

func CacheGetRandomSatisfiedChannel(c *gin.Context, group string, modelName string, retry int) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := group
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if group == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		var lastErr error
		for _, autoGroup := range GetUserAutoGroup(userGroup) {
			logger.LogDebug(c, "Auto selecting group:", autoGroup)
			channel, lastErr = model.GetRandomSatisfiedChannel(autoGroup, modelName, retry)
			// If we hit priority exhaustion, track it but continue checking other auto groups
			if lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
				continue
			}
			if channel == nil {
				continue
			} else {
				c.Set("auto_group", autoGroup)
				selectGroup = autoGroup
				logger.LogDebug(c, "Auto selected group:", autoGroup)
				break
			}
		}
		// If no channel found and we saw exhausted error, return it
		if channel == nil && lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
			return nil, selectGroup, lastErr
		}
	} else {
		channel, err = model.GetRandomSatisfiedChannel(group, modelName, retry)
		if err != nil {
			return nil, group, err
		}
	}
	return channel, selectGroup, nil
}

// CacheGetRandomSatisfiedChannelExcluding is like CacheGetRandomSatisfiedChannel but
// excludes channels that have already been tried. This ensures all channels at the
// same priority level are attempted before moving to the next priority.
func CacheGetRandomSatisfiedChannelExcluding(c *gin.Context, group string, modelName string, retry int, excludeIds map[int]bool) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := group
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if group == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		var lastErr error
		for _, autoGroup := range GetUserAutoGroup(userGroup) {
			logger.LogDebug(c, "Auto selecting group (excluding tried):", autoGroup)
			channel, lastErr = model.GetRandomSatisfiedChannelExcluding(autoGroup, modelName, retry, excludeIds)
			// If we hit priority exhaustion, track it but continue checking other auto groups
			if lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
				continue
			}
			if channel == nil {
				continue
			} else {
				c.Set("auto_group", autoGroup)
				selectGroup = autoGroup
				logger.LogDebug(c, "Auto selected group:", autoGroup)
				break
			}
		}
		// If no channel found and we saw exhausted error, return it
		if channel == nil && lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
			return nil, selectGroup, lastErr
		}
	} else {
		channel, err = model.GetRandomSatisfiedChannelExcluding(group, modelName, retry, excludeIds)
		if err != nil {
			return nil, group, err
		}
	}
	return channel, selectGroup, nil
}
