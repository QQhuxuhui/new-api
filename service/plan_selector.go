package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// PlanSelectionResult contains the result of plan selection
type PlanSelectionResult struct {
	UserPlan       *model.UserPlan
	Plan           *model.Plan
	PlanId         int      // Plan ID for context
	UserPlanId     int      // UserPlan ID for context
	PlanName       string   // Plan name for logging
	ChannelGroup   string   // Deprecated: use ChannelGroups
	ChannelGroups  []string // List of allowed channel groups
	Switched       bool     // True if auto-switched to a higher priority plan
	AutoSwitched   bool     // Alias for Switched, for clearer context key
}

// ErrNoPlanAvailable indicates no valid plan is available
var ErrNoPlanAvailable = errors.New("no valid plan available for user")

// ErrPlanQuotaExhausted indicates the current plan has no quota
var ErrPlanQuotaExhausted = errors.New("current plan quota exhausted")

// ErrPlanLocked indicates the plan is locked by admin
var ErrPlanLocked = errors.New("plan is locked by administrator")

// ErrDailyQuotaExhausted indicates the daily quota limit has been reached
var ErrDailyQuotaExhausted = errors.New("daily quota limit exhausted")

// RateLimitError represents a rate limit error with wait time information
type RateLimitError struct {
	WaitSeconds int64
	Message     string
}

func (e *RateLimitError) Error() string {
	return e.Message
}

// newPlanSelectionResult creates a PlanSelectionResult from a UserPlan
func newPlanSelectionResult(up *model.UserPlan, switched bool) *PlanSelectionResult {
	planId := 0
	if up.PlanId != nil {
		planId = *up.PlanId
	}
	result := &PlanSelectionResult{
		UserPlan:     up,
		Plan:         up.Plan, // Keep for admin reference, but don't depend on it
		PlanId:       planId,
		UserPlanId:   up.Id,
		Switched:     switched,
		AutoSwitched: switched,
	}

	// Use UserPlan snapshot fields (decoupled from Plan template)
	// This allows routing to work even if Plan is deleted/disabled
	result.PlanName = up.GetDisplayName() // Use display name for logging
	result.ChannelGroup = up.GetChannelGroup() // Deprecated, keep for compatibility
	result.ChannelGroups = up.GetChannelGroups() // Use snapshot

	return result
}

// SelectPlanForRequest selects the appropriate plan for a user request
// This is the main entry point for plan-based routing
func SelectPlanForRequest(userId int, modelName string) (*PlanSelectionResult, error) {
	// 1. Get user's valid plans (active, not expired, not locked)
	// Use cached version for better performance
	validPlans, err := model.CachedGetUserValidPlans(userId)
	if err != nil {
		return nil, fmt.Errorf("failed to get user plans: %w", err)
	}

	if len(validPlans) == 0 {
		return nil, ErrNoPlanAvailable
	}

	// 2. Find current plan
	var currentPlan *model.UserPlan
	for _, up := range validPlans {
		if up.IsCurrent == 1 {
			currentPlan = up
			break
		}
	}

	// 3. If no current plan, select highest priority with quota
	if currentPlan == nil {
		selectedPlan := selectHighestPriorityWithQuota(validPlans)
		if selectedPlan == nil {
			return nil, ErrNoPlanAvailable
		}

		// Set as current - use SwitchToUserPlan (works with NULL plan_id)
		if err := model.SwitchToUserPlan(userId, selectedPlan.Id); err != nil {
			common.SysLog(fmt.Sprintf("failed to set initial current plan: %v", err))
		}

		return newPlanSelectionResult(selectedPlan, true), nil
	}

	// 4. Check if locked
	if currentPlan.IsLocked() {
		return nil, ErrPlanLocked
	}

	// 5. Check if current plan has quota (including daily quota)
	if !hasPlanAvailableQuota(currentPlan) {
		// Current plan exhausted (total quota or daily quota) - try to find another plan with quota
		if currentPlan.AutoSwitch == 1 {
			// First try higher priority plans
			higherPlan := findHigherPriorityPlanWithQuota(validPlans, currentPlan)
			if higherPlan != nil {
				// Use SwitchToUserPlan (works with NULL plan_id)
				if err := model.SwitchToUserPlan(userId, higherPlan.Id); err != nil {
					common.SysLog(fmt.Sprintf("failed to auto-switch to higher priority plan: %v", err))
				} else {
					common.SysLog(fmt.Sprintf("user %d auto-switched from exhausted plan %d to higher priority plan %d",
						userId, currentPlan.Id, higherPlan.Id))
					return newPlanSelectionResult(higherPlan, true), nil
				}
			}

			// If no higher priority, try any plan with quota (including lower priority)
			anyPlanWithQuota := selectHighestPriorityWithQuota(validPlans)
			if anyPlanWithQuota != nil && anyPlanWithQuota.Id != currentPlan.Id {
				// Use SwitchToUserPlan (works with NULL plan_id)
				if err := model.SwitchToUserPlan(userId, anyPlanWithQuota.Id); err != nil {
					common.SysLog(fmt.Sprintf("failed to auto-switch to available plan: %v", err))
				} else {
					common.SysLog(fmt.Sprintf("user %d auto-switched from exhausted plan %d to available plan %d",
						userId, currentPlan.Id, anyPlanWithQuota.Id))
					return newPlanSelectionResult(anyPlanWithQuota, true), nil
				}
			}
		}
		// No available plan found or auto-switch disabled
		// Check if it's daily quota exhausted vs total quota exhausted
		if currentPlan.HasQuota() {
			// Has total quota but daily quota exhausted
			return nil, ErrDailyQuotaExhausted
		}
		return nil, ErrPlanQuotaExhausted
	}

	// 6. Check for smart auto-switch (upgrade to higher priority if available)
	if currentPlan.AutoSwitch == 1 {
		higherPlan := findHigherPriorityPlanWithQuota(validPlans, currentPlan)
		if higherPlan != nil {
			// Auto-switch to higher priority plan using user_plan.id (not plan_id)
			// This works even when plan_id is NULL after snapshot migration
			if err := model.SwitchToUserPlan(userId, higherPlan.Id); err != nil {
				common.SysLog(fmt.Sprintf("failed to auto-switch plan: %v", err))
				// Continue with current plan on error
			} else {
				common.SysLog(fmt.Sprintf("user %d auto-switched from user_plan %d to user_plan %d",
					userId, currentPlan.Id, higherPlan.Id))
				return newPlanSelectionResult(higherPlan, true), nil
			}
		}
	}

	// 7. Return current plan
	return newPlanSelectionResult(currentPlan, false), nil
}

// selectHighestPriorityWithQuota selects the highest priority plan that has available quota
// This includes checking both total quota and daily quota limit
func selectHighestPriorityWithQuota(plans []*model.UserPlan) *model.UserPlan {
	// Plans are already sorted by priority DESC
	for _, plan := range plans {
		if plan.IsValid() && hasPlanAvailableQuota(plan) {
			return plan
		}
	}
	return nil
}

// findHigherPriorityPlanWithQuota finds a plan with higher priority than current that has quota
// This includes checking both total quota and daily quota limit
func findHigherPriorityPlanWithQuota(plans []*model.UserPlan, current *model.UserPlan) *model.UserPlan {
	if current == nil {
		return nil
	}
	currentPriority := current.GetPriority()

	// Plans are sorted by priority DESC, so first match with higher priority wins
	for _, plan := range plans {
		if plan.GetPriority() > currentPriority && plan.IsValid() && hasPlanAvailableQuota(plan) {
			return plan
		}
	}
	return nil
}

// hasPlanAvailableQuota checks if a plan has available quota
// This checks both:
// 1. Total quota (plan.HasQuota())
// 2. Daily quota limit (if applicable)
func hasPlanAvailableQuota(plan *model.UserPlan) bool {
	// Check total quota first
	if !plan.HasQuota() {
		return false
	}

	// Check daily quota limit
	dailyLimit, hasLimit := plan.GetEffectiveDailyQuotaLimit()
	if !hasLimit {
		// No daily limit, total quota is sufficient
		return true
	}

	// Check if daily quota is exhausted
	// Use 0 as request amount to just check if already exhausted
	canProceed, _, err := CheckDailyQuotaWithLimit(plan.Id, dailyLimit, 0)
	if err != nil {
		// On error, assume quota is available (graceful degradation)
		common.SysLog(fmt.Sprintf("hasPlanAvailableQuota: error checking daily quota for user_plan %d: %v", plan.Id, err))
		return true
	}

	return canProceed
}

// GetPlanChannelGroup returns the channel group for a user's current plan
// This is a lightweight version that just returns the channel group
func GetPlanChannelGroup(userId int) (string, error) {
	result, err := SelectPlanForRequest(userId, "")
	if err != nil {
		return "", err
	}
	return result.ChannelGroup, nil
}

// UserSwitchPlan allows a user to manually switch their current plan
// Returns error if user doesn't have permission
func UserSwitchPlan(userId int, targetPlanId int) error {
	// Get current plan to check permissions
	currentPlan, err := model.GetUserCurrentPlan(userId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get current plan: %w", err)
	}

	// Get target plan
	targetUserPlan, err := model.GetUserPlanByUserAndPlan(userId, targetPlanId)
	if err != nil {
		return fmt.Errorf("target plan not found: %w", err)
	}

	// Check if target plan is valid
	if !targetUserPlan.IsValid() {
		return errors.New("target plan is not available")
	}

	// Check permission - either from current plan or target plan
	canSwitch := false
	if currentPlan != nil && currentPlan.CanUserSwitch() {
		canSwitch = true
	}
	if targetUserPlan.CanUserSwitch() {
		canSwitch = true
	}

	if !canSwitch {
		return errors.New("you don't have permission to switch plans")
	}

	// Perform switch
	return model.SwitchUserCurrentPlan(userId, targetPlanId)
}

// UserSwitchPlanByUserPlanId allows a user to manually switch to a specific user_plan instance
// This is preferred over UserSwitchPlan as it supports plans where the template was deleted
func UserSwitchPlanByUserPlanId(userId int, targetUserPlanId int) error {
	// Get current plan to check permissions
	currentPlan, err := model.GetUserCurrentPlan(userId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get current plan: %w", err)
	}

	// Get target user plan by ID
	targetUserPlan, err := model.GetUserPlanById(targetUserPlanId)
	if err != nil {
		return fmt.Errorf("target plan not found: %w", err)
	}

	// Verify ownership
	if targetUserPlan.UserId != userId {
		return errors.New("plan does not belong to user")
	}

	// Check if target plan is valid
	if !targetUserPlan.IsValid() {
		return errors.New("target plan is not available")
	}

	// Check permission - either from current plan or target plan
	canSwitch := false
	if currentPlan != nil && currentPlan.CanUserSwitch() {
		canSwitch = true
	}
	if targetUserPlan.CanUserSwitch() {
		canSwitch = true
	}

	if !canSwitch {
		return errors.New("you don't have permission to switch plans")
	}

	// Perform switch using user_plan_id (supports NULL plan_id)
	return model.SwitchToUserPlan(userId, targetUserPlanId)
}

// UserToggleAutoSwitch allows a user to toggle auto-switch on their current plan
func UserToggleAutoSwitch(userId int, userPlanId int, enabled bool) error {
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return fmt.Errorf("plan not found: %w", err)
	}

	// Verify ownership
	if userPlan.UserId != userId {
		return errors.New("plan does not belong to user")
	}

	// Check permission
	if !userPlan.CanUserToggleAuto() {
		return errors.New("you don't have permission to toggle auto-switch")
	}

	autoSwitch := 0
	if enabled {
		autoSwitch = 1
	}

	return model.ToggleUserPlanAutoSwitch(userPlanId, autoSwitch)
}

// GetUserPlanSummary returns a summary of user's plans for display
type UserPlanSummary struct {
	Plans       []*model.UserPlan `json:"plans"`
	CurrentPlan *model.UserPlan   `json:"current_plan"`
	TotalQuota  int64             `json:"total_quota"`
	TotalUsed   int64             `json:"total_used"`
}

func GetUserPlanSummary(userId int) (*UserPlanSummary, error) {
	plans, err := model.GetAllUserPlans(userId)
	if err != nil {
		return nil, err
	}

	summary := &UserPlanSummary{
		Plans: plans,
	}

	for _, plan := range plans {
		summary.TotalQuota += plan.Quota
		summary.TotalUsed += plan.UsedQuota
		if plan.IsCurrent == 1 {
			summary.CurrentPlan = plan
		}
	}

	return summary, nil
}

// ConsumeFromUserPlan consumes quota from the user's current plan
// Returns error if insufficient quota
func ConsumeFromUserPlan(userId int, amount int64) error {
	// Get current plan
	currentPlan, err := model.GetUserCurrentPlan(userId)
	if err != nil {
		return fmt.Errorf("no current plan: %w", err)
	}

	// Check quota
	if currentPlan.Quota < amount {
		return fmt.Errorf("insufficient quota: have %d, need %d", currentPlan.Quota, amount)
	}

	// Consume
	return model.DecreaseUserPlanQuota(currentPlan.Id, amount)
}

// RefundToUserPlan refunds quota to a user plan (e.g., on request failure)
func RefundToUserPlan(userPlanId int, amount int64) error {
	return model.IncreaseUserPlanQuota(userPlanId, amount)
}

// PostConsumePlanQuota consumes quota from user's plan after request completion
// This should be called in addition to the regular quota consumption
// Parameters:
//   - ctx: gin context containing plan_id and user_plan_id
//   - quota: amount of quota to consume
//
// Returns error if consumption fails
func PostConsumePlanQuota(ctx interface{ Get(key string) (value interface{}, exists bool) }, quota int) error {
	// Get user_plan_id from context
	userPlanIdVal, exists := ctx.Get(string(constant.ContextKeyUserPlanId))
	if !exists || userPlanIdVal == nil {
		// No plan selected, skip plan quota consumption
		return nil
	}

	userPlanId, ok := userPlanIdVal.(int)
	if !ok || userPlanId <= 0 {
		return nil
	}

	// Consume from user_plan
	return model.DecreaseUserPlanQuota(userPlanId, int64(quota))
}

// RefundPlanQuota refunds quota to user's plan (e.g., on request failure)
// Parameters:
//   - ctx: gin context containing plan_id and user_plan_id
//   - quota: amount of quota to refund
func RefundPlanQuota(ctx interface{ Get(key string) (value interface{}, exists bool) }, quota int) error {
	// Get user_plan_id from context
	userPlanIdVal, exists := ctx.Get(string(constant.ContextKeyUserPlanId))
	if !exists || userPlanIdVal == nil {
		return nil
	}

	userPlanId, ok := userPlanIdVal.(int)
	if !ok || userPlanId <= 0 {
		return nil
	}

	// Refund to user_plan
	return model.IncreaseUserPlanQuota(userPlanId, int64(quota))
}

// PreConsumePlanQuota pre-validates and optionally pre-consumes plan quota
// Returns error if the plan doesn't have enough quota
func PreConsumePlanQuota(ctx interface{ Get(key string) (value interface{}, exists bool) }, quota int) error {
	// Get user_plan_id from context
	userPlanIdVal, exists := ctx.Get(string(constant.ContextKeyUserPlanId))
	if !exists || userPlanIdVal == nil {
		// No plan selected, skip plan quota validation
		return nil
	}

	userPlanId, ok := userPlanIdVal.(int)
	if !ok || userPlanId <= 0 {
		return nil
	}

	// Get the user plan
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return fmt.Errorf("failed to get user plan: %w", err)
	}

	// Check if plan has enough quota
	if userPlan.Quota < int64(quota) {
		return fmt.Errorf("plan quota insufficient: have %d, need %d", userPlan.Quota, quota)
	}

	return nil
}

// ========================
// Plan Selection Metrics & Logging
// ========================

// PlanSelectionEventType defines the type of plan selection event
type PlanSelectionEventType string

const (
	PlanEventSelected      PlanSelectionEventType = "selected"       // Normal selection
	PlanEventAutoSwitch    PlanSelectionEventType = "auto_switch"    // Auto-switched to higher priority
	PlanEventInitialSet    PlanSelectionEventType = "initial_set"    // First plan set as current
	PlanEventManualSwitch  PlanSelectionEventType = "manual_switch"  // User manually switched
	PlanEventQuotaExhaust  PlanSelectionEventType = "quota_exhaust"  // Plan quota exhausted
	PlanEventLocked        PlanSelectionEventType = "locked"         // Plan locked by admin
	PlanEventNoPlan        PlanSelectionEventType = "no_plan"        // No plan available
	PlanEventError         PlanSelectionEventType = "error"          // Error during selection
	PlanEventQuotaConsume  PlanSelectionEventType = "quota_consume"  // Quota consumed
	PlanEventQuotaRefund   PlanSelectionEventType = "quota_refund"   // Quota refunded
)

// PlanSelectionEvent represents a plan selection event for logging
type PlanSelectionEvent struct {
	Timestamp     time.Time              `json:"timestamp"`
	EventType     PlanSelectionEventType `json:"event_type"`
	UserId        int                    `json:"user_id"`
	PlanId        int                    `json:"plan_id,omitempty"`
	UserPlanId    int                    `json:"user_plan_id,omitempty"`
	PlanName      string                 `json:"plan_name,omitempty"`
	FromPlanId    int                    `json:"from_plan_id,omitempty"`
	FromPlanName  string                 `json:"from_plan_name,omitempty"`
	ToPlanId      int                    `json:"to_plan_id,omitempty"`
	ToPlanName    string                 `json:"to_plan_name,omitempty"`
	ChannelGroup  string                 `json:"channel_group,omitempty"`
	Model         string                 `json:"model,omitempty"`
	QuotaAmount   int64                  `json:"quota_amount,omitempty"`
	QuotaBefore   int64                  `json:"quota_before,omitempty"`
	QuotaAfter    int64                  `json:"quota_after,omitempty"`
	ErrorMessage  string                 `json:"error,omitempty"`
	DurationMs    int64                  `json:"duration_ms,omitempty"`
}

// LogPlanSelectionEvent logs a plan selection event
func LogPlanSelectionEvent(event *PlanSelectionEvent) {
	if event == nil {
		return
	}

	var logMsg string
	switch event.EventType {
	case PlanEventSelected:
		logMsg = fmt.Sprintf("[Plan] user=%d selected plan=%s(id=%d) group=%s",
			event.UserId, event.PlanName, event.PlanId, event.ChannelGroup)

	case PlanEventAutoSwitch:
		logMsg = fmt.Sprintf("[Plan] user=%d auto-switched from plan=%s(id=%d) to plan=%s(id=%d)",
			event.UserId, event.FromPlanName, event.FromPlanId, event.ToPlanName, event.ToPlanId)

	case PlanEventInitialSet:
		logMsg = fmt.Sprintf("[Plan] user=%d initial plan set to plan=%s(id=%d)",
			event.UserId, event.PlanName, event.PlanId)

	case PlanEventManualSwitch:
		logMsg = fmt.Sprintf("[Plan] user=%d manual-switched from plan=%s(id=%d) to plan=%s(id=%d)",
			event.UserId, event.FromPlanName, event.FromPlanId, event.ToPlanName, event.ToPlanId)

	case PlanEventQuotaExhaust:
		logMsg = fmt.Sprintf("[Plan] user=%d plan=%s(id=%d) quota exhausted, remaining=%d",
			event.UserId, event.PlanName, event.PlanId, event.QuotaAfter)

	case PlanEventLocked:
		logMsg = fmt.Sprintf("[Plan] user=%d plan=%s(id=%d) is locked",
			event.UserId, event.PlanName, event.PlanId)

	case PlanEventNoPlan:
		logMsg = fmt.Sprintf("[Plan] user=%d has no available plan",
			event.UserId)

	case PlanEventError:
		logMsg = fmt.Sprintf("[Plan] user=%d error: %s",
			event.UserId, event.ErrorMessage)

	case PlanEventQuotaConsume:
		logMsg = fmt.Sprintf("[Plan] user=%d plan=%s(id=%d) consumed=%d before=%d after=%d",
			event.UserId, event.PlanName, event.PlanId, event.QuotaAmount, event.QuotaBefore, event.QuotaAfter)

	case PlanEventQuotaRefund:
		logMsg = fmt.Sprintf("[Plan] user=%d plan=%s(id=%d) refunded=%d before=%d after=%d",
			event.UserId, event.PlanName, event.PlanId, event.QuotaAmount, event.QuotaBefore, event.QuotaAfter)

	default:
		logMsg = fmt.Sprintf("[Plan] user=%d event=%s plan=%d",
			event.UserId, event.EventType, event.PlanId)
	}

	if event.DurationMs > 0 {
		logMsg += fmt.Sprintf(" duration=%dms", event.DurationMs)
	}

	if common.DebugEnabled {
		common.SysLog(logMsg)
	}
}

// SelectPlanForRequestWithMetrics wraps SelectPlanForRequest with metrics and logging
func SelectPlanForRequestWithMetrics(userId int, modelName string) (*PlanSelectionResult, error) {
	startTime := time.Now()
	result, err := SelectPlanForRequest(userId, modelName)
	duration := time.Since(startTime).Milliseconds()

	event := &PlanSelectionEvent{
		Timestamp:  startTime,
		UserId:     userId,
		Model:      modelName,
		DurationMs: duration,
	}

	if err != nil {
		switch {
		case errors.Is(err, ErrNoPlanAvailable):
			event.EventType = PlanEventNoPlan
		case errors.Is(err, ErrPlanQuotaExhausted):
			event.EventType = PlanEventQuotaExhaust
		case errors.Is(err, ErrPlanLocked):
			event.EventType = PlanEventLocked
		default:
			event.EventType = PlanEventError
			event.ErrorMessage = err.Error()
		}
		LogPlanSelectionEvent(event)
		return nil, err
	}

	event.PlanId = result.PlanId
	event.UserPlanId = result.UserPlanId
	event.PlanName = result.PlanName
	event.ChannelGroup = result.ChannelGroup

	if result.AutoSwitched {
		event.EventType = PlanEventAutoSwitch
	} else {
		event.EventType = PlanEventSelected
	}

	LogPlanSelectionEvent(event)
	return result, nil
}

// LogPlanSwitch logs a plan switch event
func LogPlanSwitch(userId int, fromPlan, toPlan *model.UserPlan, isManual bool) {
	event := &PlanSelectionEvent{
		Timestamp: time.Now(),
		UserId:    userId,
	}

	if isManual {
		event.EventType = PlanEventManualSwitch
	} else {
		event.EventType = PlanEventAutoSwitch
	}

	if fromPlan != nil {
		if fromPlan.PlanId != nil {
			event.FromPlanId = *fromPlan.PlanId
		}
		// Use snapshot fields first (works even when Plan is deleted)
		if fromPlan.PlanName != "" {
			event.FromPlanName = fromPlan.PlanName
		} else if fromPlan.Plan != nil {
			event.FromPlanName = fromPlan.Plan.Name
		}
	}

	if toPlan != nil {
		if toPlan.PlanId != nil {
			event.ToPlanId = *toPlan.PlanId
		}
		// Use snapshot fields first (works even when Plan is deleted)
		if toPlan.PlanName != "" {
			event.ToPlanName = toPlan.PlanName
		} else if toPlan.Plan != nil {
			event.ToPlanName = toPlan.Plan.Name
		}
		// Use snapshot for channel group
		channelGroups := toPlan.GetChannelGroups()
		if len(channelGroups) > 0 {
			event.ChannelGroup = channelGroups[0]
		} else if toPlan.Plan != nil {
			event.ChannelGroup = toPlan.Plan.ChannelGroup
		}
	}

	LogPlanSelectionEvent(event)
}

// LogPlanQuotaChange logs quota consumption or refund
func LogPlanQuotaChange(userId, planId, userPlanId int, planName string, amount, quotaBefore, quotaAfter int64, isRefund bool) {
	event := &PlanSelectionEvent{
		Timestamp:   time.Now(),
		UserId:      userId,
		PlanId:      planId,
		UserPlanId:  userPlanId,
		PlanName:    planName,
		QuotaAmount: amount,
		QuotaBefore: quotaBefore,
		QuotaAfter:  quotaAfter,
	}

	if isRefund {
		event.EventType = PlanEventQuotaRefund
	} else {
		event.EventType = PlanEventQuotaConsume
	}

	LogPlanSelectionEvent(event)
}

// SelectPlanWithQuotaChecks selects a plan and validates rate limits and daily quota
// This is the enhanced version that includes all quota checks
// Parameters:
//   - userId: the user ID
//   - modelName: the model being requested
//   - estimatedCostUSD: estimated cost of the request in USD (for rate limit check)
//   - estimatedQuota: estimated quota consumption (for daily quota check)
func SelectPlanWithQuotaChecks(userId int, modelName string, estimatedCostUSD float64, estimatedQuota int64) (*PlanSelectionResult, error) {
	// First, select the plan using existing logic
	result, err := SelectPlanForRequest(userId, modelName)
	if err != nil {
		return nil, err
	}

	plan := result.Plan
	if plan == nil {
		// Plan template可能被删除，使用UserPlan的快照字段构造一个虚拟Plan以继续限流/日配额校验
		up := result.UserPlan
		plan = &model.Plan{
			Id:              result.PlanId,
			Name:            up.PlanName,
			DisplayName:     up.GetDisplayName(),
			Category:        up.GetCategory(),
			Type:            up.GetType(),
			Priority:        up.GetPriority(),
			ChannelGroup:    up.GetChannelGroup(),
			ChannelGroups:   up.PlanChannelGroups,
			DailyQuotaLimit: up.GetPlanDailyQuotaLimit(),
			RateLimitRules:  up.GetRateLimitRules(),
		}
		result.Plan = plan
	}

	// Check rate limits first (most restrictive)
	if plan.HasRateLimits() {
		canProceed, waitSec, message := CheckRateLimits(plan, result.UserPlanId, estimatedCostUSD)
		if !canProceed {
			return nil, &RateLimitError{
				WaitSeconds: waitSec,
				Message:     message,
			}
		}
	}

	// Check daily quota limit using effective limit (UserPlan override > Plan default)
	// Get user plan to check for override
	userPlan, err := model.GetUserPlanById(result.UserPlanId)
	if err == nil && userPlan != nil {
		userPlan.Plan = plan
		dailyLimit, hasLimit := userPlan.GetEffectiveDailyQuotaLimit()
		if hasLimit {
			canProceed, _, err := CheckDailyQuotaWithLimit(result.UserPlanId, dailyLimit, estimatedQuota)
			if err != nil {
				common.SysLog(fmt.Sprintf("daily quota check error for user %d: %v", userId, err))
				// Allow on error (graceful degradation)
			} else if !canProceed {
				return nil, ErrDailyQuotaExhausted
			}
		}
	} else if plan.HasDailyQuotaLimit() {
		// Fallback to plan's daily quota limit if can't get user plan
		canProceed, _, err := CheckDailyQuota(plan, result.UserPlanId, estimatedQuota)
		if err != nil {
			common.SysLog(fmt.Sprintf("daily quota check error for user %d: %v", userId, err))
			// Allow on error (graceful degradation)
		} else if !canProceed {
			return nil, ErrDailyQuotaExhausted
		}
	}

	return result, nil
}

// PostConsumePlanQuotaWithTracking consumes quota and records for rate limiting
// This enhanced version also records consumption for rate limit tracking
// Parameters:
//   - ctx: gin context containing plan_id, user_plan_id, and plan info
//   - quota: amount of quota to consume
//   - costUSD: cost in USD for rate limit tracking
//   - requestId: unique request ID for rate limit tracking
func PostConsumePlanQuotaWithTracking(ctx interface {
	Get(key string) (value interface{}, exists bool)
}, quota int, costUSD float64, requestId string) error {
	// Get user_plan_id from context
	userPlanIdVal, exists := ctx.Get(string(constant.ContextKeyUserPlanId))
	if !exists || userPlanIdVal == nil {
		return nil
	}

	userPlanId, ok := userPlanIdVal.(int)
	if !ok || userPlanId <= 0 {
		return nil
	}

	// Consume from user_plan quota
	if err := model.DecreaseUserPlanQuota(userPlanId, int64(quota)); err != nil {
		return err
	}

	// Get plan info for tracking decisions
	planIdVal, _ := ctx.Get(string(constant.ContextKeyPlanId))
	planId, _ := planIdVal.(int)

	if planId > 0 {
		plan, err := model.GetPlanById(planId)
		if err == nil && plan != nil {
			// Record daily quota usage (subscription plans only)
			if plan.HasDailyQuotaLimit() {
				if err := IncrDailyQuotaUsage(userPlanId, int64(quota)); err != nil {
					common.SysLog(fmt.Sprintf("failed to record daily quota for user_plan %d: %v", userPlanId, err))
				}
			}

			// Record rate limit consumption (if rate limits configured)
			if plan.HasRateLimits() && costUSD > 0 {
				if err := RecordConsumptionForRateLimit(userPlanId, costUSD, requestId); err != nil {
					common.SysLog(fmt.Sprintf("failed to record rate limit for user_plan %d: %v", userPlanId, err))
				}
			}
		}
	}

	return nil
}

// HasChannelGroupAccess checks if a plan selection result allows access to a specific channel group
func (r *PlanSelectionResult) HasChannelGroupAccess(channelGroup string) bool {
	if r == nil || r.Plan == nil {
		return false
	}
	// If no channel groups specified, allow all
	if len(r.ChannelGroups) == 0 {
		return true
	}
	for _, g := range r.ChannelGroups {
		if g == channelGroup || g == "" {
			return true
		}
	}
	return false
}
