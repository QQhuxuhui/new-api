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
	UserPlan     *model.UserPlan
	Plan         *model.Plan
	PlanId       int    // Plan ID for context
	UserPlanId   int    // UserPlan ID for context
	PlanName     string // Plan name for logging
	ChannelGroup string
	Switched     bool // True if auto-switched to a higher priority plan
	AutoSwitched bool // Alias for Switched, for clearer context key
}

// ErrNoPlanAvailable indicates no valid plan is available
var ErrNoPlanAvailable = errors.New("no valid plan available for user")

// ErrPlanQuotaExhausted indicates the current plan has no quota
var ErrPlanQuotaExhausted = errors.New("current plan quota exhausted")

// ErrPlanLocked indicates the plan is locked by admin
var ErrPlanLocked = errors.New("plan is locked by administrator")

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

		// Set as current
		if err := model.SwitchUserCurrentPlan(userId, selectedPlan.PlanId); err != nil {
			common.SysLog(fmt.Sprintf("failed to set initial current plan: %v", err))
		}

		return &PlanSelectionResult{
			UserPlan:     selectedPlan,
			Plan:         selectedPlan.Plan,
			PlanId:       selectedPlan.PlanId,
			UserPlanId:   selectedPlan.Id,
			PlanName:     selectedPlan.Plan.Name,
			ChannelGroup: selectedPlan.Plan.ChannelGroup,
			Switched:     true,
			AutoSwitched: true,
		}, nil
	}

	// 4. Check if locked
	if currentPlan.IsLocked() {
		return nil, ErrPlanLocked
	}

	// 5. Check if current plan has quota
	if !currentPlan.HasQuota() {
		// Current plan exhausted - try to find another plan with quota
		if currentPlan.AutoSwitch == 1 {
			// First try higher priority plans
			higherPlan := findHigherPriorityPlanWithQuota(validPlans, currentPlan)
			if higherPlan != nil {
				if err := model.SwitchUserCurrentPlan(userId, higherPlan.PlanId); err != nil {
					common.SysLog(fmt.Sprintf("failed to auto-switch to higher priority plan: %v", err))
				} else {
					common.SysLog(fmt.Sprintf("user %d auto-switched from exhausted plan %d to higher priority plan %d",
						userId, currentPlan.PlanId, higherPlan.PlanId))
					return &PlanSelectionResult{
						UserPlan:     higherPlan,
						Plan:         higherPlan.Plan,
						PlanId:       higherPlan.PlanId,
						UserPlanId:   higherPlan.Id,
						PlanName:     higherPlan.Plan.Name,
						ChannelGroup: higherPlan.Plan.ChannelGroup,
						Switched:     true,
						AutoSwitched: true,
					}, nil
				}
			}

			// If no higher priority, try any plan with quota (including lower priority)
			anyPlanWithQuota := selectHighestPriorityWithQuota(validPlans)
			if anyPlanWithQuota != nil && anyPlanWithQuota.Id != currentPlan.Id {
				if err := model.SwitchUserCurrentPlan(userId, anyPlanWithQuota.PlanId); err != nil {
					common.SysLog(fmt.Sprintf("failed to auto-switch to available plan: %v", err))
				} else {
					common.SysLog(fmt.Sprintf("user %d auto-switched from exhausted plan %d to available plan %d",
						userId, currentPlan.PlanId, anyPlanWithQuota.PlanId))
					return &PlanSelectionResult{
						UserPlan:     anyPlanWithQuota,
						Plan:         anyPlanWithQuota.Plan,
						PlanId:       anyPlanWithQuota.PlanId,
						UserPlanId:   anyPlanWithQuota.Id,
						PlanName:     anyPlanWithQuota.Plan.Name,
						ChannelGroup: anyPlanWithQuota.Plan.ChannelGroup,
						Switched:     true,
						AutoSwitched: true,
					}, nil
				}
			}
		}
		// No available plan found or auto-switch disabled
		return nil, ErrPlanQuotaExhausted
	}

	// 6. Check for smart auto-switch (upgrade to higher priority if available)
	if currentPlan.AutoSwitch == 1 {
		higherPlan := findHigherPriorityPlanWithQuota(validPlans, currentPlan)
		if higherPlan != nil {
			// Auto-switch to higher priority plan
			if err := model.SwitchUserCurrentPlan(userId, higherPlan.PlanId); err != nil {
				common.SysLog(fmt.Sprintf("failed to auto-switch plan: %v", err))
				// Continue with current plan on error
			} else {
				common.SysLog(fmt.Sprintf("user %d auto-switched from plan %d to plan %d",
					userId, currentPlan.PlanId, higherPlan.PlanId))
				return &PlanSelectionResult{
					UserPlan:     higherPlan,
					Plan:         higherPlan.Plan,
					PlanId:       higherPlan.PlanId,
					UserPlanId:   higherPlan.Id,
					PlanName:     higherPlan.Plan.Name,
					ChannelGroup: higherPlan.Plan.ChannelGroup,
					Switched:     true,
					AutoSwitched: true,
				}, nil
			}
		}
	}

	// 7. Return current plan
	return &PlanSelectionResult{
		UserPlan:     currentPlan,
		Plan:         currentPlan.Plan,
		PlanId:       currentPlan.PlanId,
		UserPlanId:   currentPlan.Id,
		PlanName:     currentPlan.Plan.Name,
		ChannelGroup: currentPlan.Plan.ChannelGroup,
		Switched:     false,
		AutoSwitched: false,
	}, nil
}

// selectHighestPriorityWithQuota selects the highest priority plan that has available quota
func selectHighestPriorityWithQuota(plans []*model.UserPlan) *model.UserPlan {
	// Plans are already sorted by priority DESC
	for _, plan := range plans {
		if plan.HasQuota() && plan.IsValid() {
			return plan
		}
	}
	return nil
}

// findHigherPriorityPlanWithQuota finds a plan with higher priority than current that has quota
func findHigherPriorityPlanWithQuota(plans []*model.UserPlan, current *model.UserPlan) *model.UserPlan {
	if current == nil || current.Plan == nil {
		return nil
	}
	currentPriority := current.Plan.Priority

	// Plans are sorted by priority DESC, so first match with higher priority wins
	for _, plan := range plans {
		if plan.Plan == nil {
			continue
		}
		if plan.Plan.Priority > currentPriority && plan.HasQuota() && plan.IsValid() {
			return plan
		}
	}
	return nil
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

	if fromPlan != nil && fromPlan.Plan != nil {
		event.FromPlanId = fromPlan.PlanId
		event.FromPlanName = fromPlan.Plan.Name
	}

	if toPlan != nil && toPlan.Plan != nil {
		event.ToPlanId = toPlan.PlanId
		event.ToPlanName = toPlan.Plan.Name
		event.ChannelGroup = toPlan.Plan.ChannelGroup
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
