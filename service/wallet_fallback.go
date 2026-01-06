package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// AttemptWalletFallbackAfterRetry tries to find a channel using wallet billing (child groups of token group)
// when plan-based retries and cross-plan failover have exhausted.
// It returns (channel, group, error). A nil channel with nil error means "no wallet option found".
func AttemptWalletFallbackAfterRetry(c *gin.Context, modelName string) (*model.Channel, string, error) {
	tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	userQuota := common.GetContextKeyInt(c, constant.ContextKeyUserQuota)

	// Require wallet balance
	if userQuota <= 0 {
		return nil, "", nil
	}

	// Only parent groups have child groups to try
	if !ratio_setting.IsParentGroup(tokenGroup) {
		return nil, "", nil
	}

	allChildGroups := ratio_setting.GetChildGroups(tokenGroup)
	if len(allChildGroups) == 0 {
		return nil, "", nil
	}

	// Groups already tried in plan/cross-plan
	triedGroups := make(map[string]bool)
	if planGroups, exists := c.Get(string(constant.ContextKeyPlanGroups)); exists {
		if groups, ok := planGroups.([]string); ok {
			for _, g := range groups {
				triedGroups[g] = true
			}
		}
	}
	if using := common.GetContextKeyString(c, constant.ContextKeyUsingGroup); using != "" {
		triedGroups[using] = true
	}

	var untried []string
	for _, child := range allChildGroups {
		if !triedGroups[child] {
			untried = append(untried, child)
		}
	}
	if len(untried) == 0 {
		return nil, "", nil
	}

	// Permission: user must be allowed to use tokenGroup or any child group
	walletGroupAllowed := GroupInUserUsableGroups(userGroup, tokenGroup)
	if !walletGroupAllowed {
		for _, child := range untried {
			if GroupInUserUsableGroups(userGroup, child) {
				walletGroupAllowed = true
				break
			}
		}
	}
	if !walletGroupAllowed {
		return nil, "", nil
	}

	// Try each untried child group for a channel
	for _, childGroup := range untried {
		// Override plan groups context so selector will use this group
		common.SetContextKey(c, constant.ContextKeyPlanGroups, []string{childGroup})
		common.SetContextKey(c, constant.ContextKeyUsingGroup, childGroup)

		for retry := 0; retry < 1000; retry++ {
			channel, _, err := CacheGetRandomSatisfiedChannel(c, childGroup, modelName, retry)

			if err != nil && errors.Is(err, model.ErrPriorityExhausted) {
				err = nil
				break
			}
			if err != nil {
				// System error, stop wallet fallback
				return nil, "", fmt.Errorf("wallet fallback selection error: %w", err)
			}
			if channel != nil {
				return channel, childGroup, nil
			}
		}
	}

	return nil, "", nil
}
