package service

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

func ReturnPreConsumedQuota(c *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if relayInfo.FinalPreConsumedQuota != 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费额度 %s (计费来源: %s)", relayInfo.UserId, logger.FormatQuota(relayInfo.FinalPreConsumedQuota), relayInfo.BillingSource))

		// Mixed billing: return user-balance remainder (if deducted) and token quota (if deducted).
		if relayInfo.BillingSource == BillingSourcePlanAndUserBalance {
			// Return wallet part.
			if relayInfo.UserBalancePreConsumedQuota > 0 {
				gopool.Go(func() {
					if err := model.IncreaseUserQuota(relayInfo.UserId, relayInfo.UserBalancePreConsumedQuota, false); err != nil {
						common.SysLog("error return pre-consumed user quota (mixed): " + err.Error())
					}
				})
			}

			// Return token part (token was pre-consumed for the whole request in plan/mixed billing).
			if !relayInfo.IsPlayground && relayInfo.FinalPreConsumedQuota > 0 {
				gopool.Go(func() {
					if err := model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, relayInfo.FinalPreConsumedQuota); err != nil {
						common.SysLog("error return pre-consumed token quota (mixed): " + err.Error())
					}
				})
			}
			return
		}

		// For plan billing: Only return token quota, NOT plan/user balance
		// Because in plan billing path, we only pre-consumed token quota, not plan or user balance
		if relayInfo.BillingSource == BillingSourcePlan {
			if !relayInfo.IsPlayground && relayInfo.FinalPreConsumedQuota > 0 {
				gopool.Go(func() {
					// Only return token quota
					err := model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, relayInfo.FinalPreConsumedQuota)
					if err != nil {
						common.SysLog("error return pre-consumed token quota for plan billing: " + err.Error())
					}
				})
			}
			return
		}

		// For daily pool billing: Only return token quota, NOT daily pool
		// Because in daily pool billing path, we only pre-consumed token quota, not daily pool
		// Daily pool quota is only deducted in PostConsumeQuota
		if relayInfo.BillingSource == BillingSourceDailyPool {
			if !relayInfo.IsPlayground && relayInfo.FinalPreConsumedQuota > 0 {
				gopool.Go(func() {
					// Only return token quota (daily pool was NOT pre-consumed)
					err := model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, relayInfo.FinalPreConsumedQuota)
					if err != nil {
						common.SysLog("error return pre-consumed token quota for daily pool billing: " + err.Error())
					}
				})
			}
			return
		}

		// For user balance billing: Use normal refund flow
		gopool.Go(func() {
			relayInfoCopy := *relayInfo

			err := PostConsumeQuota(&relayInfoCopy, -relayInfoCopy.FinalPreConsumedQuota, 0, false)
			if err != nil {
				common.SysLog("error return pre-consumed quota: " + err.Error())
			}
		})
	}
}

// PreConsumeQuota checks if the user has enough quota to pre-consume.
// It implements three-level billing priority: Daily Pool → Plan → User Balance
// Uses skip-level billing - if a source is insufficient, skip entirely to next level
// Sets relayInfo.BillingSource to indicate the quota source.
func PreConsumeQuota(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	requiredQuota := int64(preConsumedQuota)
	// Reset per-request mixed-billing fields (relayInfo can be reused during retries/failover).
	relayInfo.UserBalancePreConsumedQuota = 0
	relayInfo.PlanPreConsumeQuota = 0

	// Priority 1: Check Daily Pool
	dailyPoolRemaining, err := model.GetDailyPoolRemaining(relayInfo.UserId)
	if err == nil && dailyPoolRemaining >= requiredQuota {
		// Daily pool has sufficient quota
		relayInfo.BillingSource = BillingSourceDailyPool
		relayInfo.UserPlanId = 0
		logger.LogInfo(c, fmt.Sprintf("用户 %d 使用日卡额度, 需要: %s, 可用: %s", relayInfo.UserId, logger.FormatQuota(preConsumedQuota), logger.FormatQuota(int(dailyPoolRemaining))))

		// Pre-consume from token quota
		if !relayInfo.IsPlayground && !relayInfo.TokenUnlimited {
			err := PreConsumeTokenQuota(relayInfo, preConsumedQuota)
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
		}

		relayInfo.FinalPreConsumedQuota = preConsumedQuota
		return nil
	}

	// Priority 2: Check Plan Quota
	if relayInfo.UserPlanId > 0 {
		userPlan, planErr := model.GetUserPlanById(relayInfo.UserPlanId)
		if planErr != nil {
			logger.LogInfo(c, fmt.Sprintf("用户 %d 套餐额度检查失败: %v, 回退到用户余额", relayInfo.UserId, planErr))
		} else if userPlan == nil || !userPlan.IsValid() {
			logger.LogInfo(c, fmt.Sprintf("用户 %d 套餐 %d 不可用, 回退到用户余额", relayInfo.UserId, relayInfo.UserPlanId))
		} else {
			// Plan is valid; decide whether to use it fully or split with wallet.
			planAvailable := userPlan.Quota
			if planAvailable >= requiredQuota {
				// Also check daily quota limit before using plan
				dailyQuotaErr := CheckDailyQuotaBeforeConsume(relayInfo.UserPlanId, requiredQuota)
				if dailyQuotaErr != nil {
					logger.LogInfo(c, fmt.Sprintf("用户 %d 套餐 %d 每日额度不足: %v, 回退到用户余额", relayInfo.UserId, relayInfo.UserPlanId, dailyQuotaErr))
					// Continue to user balance fallback
				} else {
					// Plan quota sufficient - use plan billing
					relayInfo.BillingSource = BillingSourcePlan
					logger.LogInfo(c, fmt.Sprintf("用户 %d 使用套餐 %d 额度, 需要: %s", relayInfo.UserId, relayInfo.UserPlanId, logger.FormatQuota(preConsumedQuota)))

					// For plan billing, SKIP user balance check and deduction entirely
					// Only pre-consume from token quota (if not unlimited/playground)
					if !relayInfo.IsPlayground && !relayInfo.TokenUnlimited {
						err := PreConsumeTokenQuota(relayInfo, preConsumedQuota)
						if err != nil {
							return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
						}
					}

					relayInfo.FinalPreConsumedQuota = preConsumedQuota
					logger.LogInfo(c, fmt.Sprintf("用户 %d 使用套餐计费, 预记录 %s (不扣除用户余额)", relayInfo.UserId, logger.FormatQuota(preConsumedQuota)))
					return nil
				}
			} else if planAvailable > 0 {
				// Split billing: plan (up to remaining) + wallet (remainder).
				planMax := planAvailable

				// Respect daily quota limit: only the remaining daily quota can be charged to the plan.
				dailyLimit, hasLimit := userPlan.GetEffectiveDailyQuotaLimit()
				if hasLimit {
					canProceed, remaining, err := CheckDailyQuotaWithLimit(userPlan.Id, dailyLimit, 0)
					if err == nil {
						if !canProceed || remaining <= 0 {
							planMax = 0
						} else if remaining < planMax {
							planMax = remaining
						}
					}
				}

				if planMax > 0 {
					remainder := requiredQuota - planMax

					userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
					if err != nil {
						return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
					}
					if userQuota <= 0 || int64(userQuota) < remainder {
						// Not enough wallet to cover the remainder; fall back to wallet-only error path.
						logger.LogInfo(c, fmt.Sprintf("用户 %d 套餐 %d 余额不足以覆盖预扣费, 且钱包余额不足, 回退到用户余额校验失败", relayInfo.UserId, relayInfo.UserPlanId))
					} else {
						relayInfo.BillingSource = BillingSourcePlanAndUserBalance
						relayInfo.PlanPreConsumeQuota = int(planMax)
						relayInfo.UserBalancePreConsumedQuota = int(remainder)

						// Pre-consume token quota for the whole request (if not unlimited/playground).
						if !relayInfo.IsPlayground && !relayInfo.TokenUnlimited {
							if err := PreConsumeTokenQuota(relayInfo, preConsumedQuota); err != nil {
								return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
							}
						}

						// Deduct the wallet remainder now (plan part is deducted after completion).
						if remainder > 0 {
							if err := model.DecreaseUserQuota(relayInfo.UserId, int(remainder)); err != nil {
								return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
							}
						}

						relayInfo.UserQuota = userQuota
						relayInfo.FinalPreConsumedQuota = preConsumedQuota
						logger.LogInfo(c, fmt.Sprintf("用户 %d 使用套餐+余额混合计费, 套餐预占 %s, 钱包预扣 %s",
							relayInfo.UserId, logger.FormatQuota(int(planMax)), logger.FormatQuota(int(remainder))))
						return nil
					}
				}

				logger.LogInfo(c, fmt.Sprintf("用户 %d 套餐 %d 额度不足, 回退到用户余额", relayInfo.UserId, relayInfo.UserPlanId))
			} else {
				logger.LogInfo(c, fmt.Sprintf("用户 %d 套餐 %d 额度不足, 回退到用户余额", relayInfo.UserId, relayInfo.UserPlanId))
			}
		}
	}

	// Priority 3: Fall back to user balance
	relayInfo.BillingSource = BillingSourceUserBalance
	relayInfo.UserPlanId = 0 // 清零套餐ID，确保消费日志不会错误地关联到套餐

	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}
	if userQuota <= 0 {
		return types.NewErrorWithStatusCode(fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota(userQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if userQuota-preConsumedQuota < 0 {
		return types.NewErrorWithStatusCode(fmt.Errorf("预扣费额度失败, 用户剩余额度: %s, 需要预扣费额度: %s", logger.FormatQuota(userQuota), logger.FormatQuota(preConsumedQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}

	relayInfo.UserQuota = userQuota
	return preConsumeFromUserAndToken(c, preConsumedQuota, relayInfo, userQuota)
}

// checkPlanQuotaSufficient checks if the user plan has sufficient quota
func checkPlanQuotaSufficient(userPlanId int, requiredQuota int64) (bool, error) {
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return false, err
	}
	if userPlan == nil {
		return false, fmt.Errorf("user plan not found")
	}
	if !userPlan.IsValid() {
		return false, fmt.Errorf("user plan is not valid")
	}
	return userPlan.Quota >= requiredQuota, nil
}

// preConsumeFromUserAndToken handles the actual pre-consumption from user quota and token quota
func preConsumeFromUserAndToken(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo, userQuota int) *types.NewAPIError {
	trustQuota := common.GetTrustQuota()

	if userQuota > trustQuota {
		// 用户额度充足，判断令牌额度是否充足
		if !relayInfo.TokenUnlimited {
			// 非无限令牌，判断令牌额度是否充足
			tokenQuota := c.GetInt("token_quota")
			if tokenQuota > trustQuota {
				// 令牌额度充足，信任令牌
				preConsumedQuota = 0
				logger.LogInfo(c, fmt.Sprintf("用户 %d 剩余额度 %s 且令牌 %d 额度 %d 充足, 信任且不需要预扣费 (计费来源: %s)", relayInfo.UserId, logger.FormatQuota(userQuota), relayInfo.TokenId, tokenQuota, relayInfo.BillingSource))
			}
		} else {
			// in this case, we do not pre-consume quota
			// because the user has enough quota
			preConsumedQuota = 0
			logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足且为无限额度令牌, 信任且不需要预扣费 (计费来源: %s)", relayInfo.UserId, relayInfo.BillingSource))
		}
	}

	if preConsumedQuota > 0 {
		err := PreConsumeTokenQuota(relayInfo, preConsumedQuota)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		err = model.DecreaseUserQuota(relayInfo.UserId, preConsumedQuota)
		if err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		logger.LogInfo(c, fmt.Sprintf("用户 %d 预扣费 %s, 预扣费后剩余额度: %s (计费来源: %s)", relayInfo.UserId, logger.FormatQuota(preConsumedQuota), logger.FormatQuota(userQuota-preConsumedQuota), relayInfo.BillingSource))
	}
	relayInfo.FinalPreConsumedQuota = preConsumedQuota
	return nil
}
