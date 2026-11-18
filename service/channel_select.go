package service

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func GetChannelConstraints(c *gin.Context) *dto.ChannelConstraints {
	if c == nil {
		return &dto.ChannelConstraints{}
	}
	if existing, ok := common.GetContextKeyType[*dto.ChannelConstraints](c, constant.ContextKeyChannelConstraints); ok && existing != nil {
		return existing
	}
	constraints := &dto.ChannelConstraints{}
	common.SetContextKey(c, constant.ContextKeyChannelConstraints, constraints)
	return constraints
}

func AppendTaskPluginIdentityFilter(c *gin.Context, pluginKey string) {
	if c == nil {
		return
	}
	channelTypes, pluginKeys := pinnedTaskPluginIdentities(c, pluginKey)
	GetChannelConstraints(c).AddFilter(dto.ChannelFilter{
		Kind:                   dto.FilterTaskPluginIdentity,
		TaskPluginKey:          pluginKey,
		TaskPluginChannelTypes: channelTypes,
		TaskPluginKeys:         pluginKeys,
	})
}

type RetryParam struct {
	Ctx         *gin.Context
	TokenGroup  string
	ModelName   string
	RequestPath string
	// Retry is the attempt index of the request: 0 for the first attempt,
	// then one more per attempt. It no longer selects a priority level.
	Retry *int
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

const (
	// ginKeyWarningMissedChannels holds the warning channels whose probe roll
	// missed during this request.
	ginKeyWarningMissedChannels = "channel_warning_missed"
	// ginKeyAutoGroupStart holds the auto group of the request's first
	// selected channel; later selections start from it.
	ginKeyAutoGroupStart = "auto_group_start"
)

// TriedChannels returns the channels the request already attempted, read
// from the trail AppendUsedChannel writes.
func TriedChannels(c *gin.Context) map[int]bool {
	tried := make(map[int]bool)
	for _, idStr := range c.GetStringSlice("use_channel") {
		if id, err := strconv.Atoi(idStr); err == nil {
			tried[id] = true
		}
	}
	return tried
}

func warningMissedChannels(c *gin.Context) map[int]bool {
	if missed, ok := c.Get(ginKeyWarningMissedChannels); ok {
		if set, ok := missed.(map[int]bool); ok {
			return set
		}
	}
	return map[int]bool{}
}

func markAutoGroupStart(c *gin.Context, group string) {
	if _, exists := c.Get(ginKeyAutoGroupStart); !exists {
		c.Set(ginKeyAutoGroupStart, group)
	}
}

// CacheGetRandomSatisfiedChannel picks the next channel of the request.
//
// Every call skips the channels the request already tried and the suspended
// ones, and takes the highest priority level that still has a candidate, so
// a failed attempt moves to another channel and suspended levels never use
// up attempts. The groups it may use are the token group, or for "auto" the
// auto groups: all of them until a channel was selected, then the group of
// that first channel, followed by the later groups only when the token
// allows cross-group retry. The groups are first scanned with the warning
// probe roll, then once more ignoring it, so a warning channel whose roll
// missed is tried only after every other candidate failed. A nil channel
// with a nil error means no candidate is left.
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	c := param.Ctx
	groups, err := retryCandidateGroups(param)
	if err != nil {
		return nil, param.TokenGroup, err
	}

	filters := GetChannelConstraints(c).Filters
	missed := warningMissedChannels(c)
	pick := model.ChannelPick{Tried: TriedChannels(c), WarningMissed: missed}
	for _, includeWarning := range []bool{false, true} {
		pick.IncludeWarning = includeWarning
		for _, group := range groups {
			channel, newlyMissed, err := model.GetRandomSatisfiedChannel(group, param.ModelName, pick, filters)
			for _, id := range newlyMissed {
				missed[id] = true
			}
			c.Set(ginKeyWarningMissedChannels, missed)
			if err != nil {
				return nil, group, err
			}
			if channel == nil {
				logger.LogDebug(c, "No channel left in group %s for model %s (include warning: %v)", group, param.ModelName, includeWarning)
				continue
			}
			if param.TokenGroup == "auto" {
				common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
				markAutoGroupStart(c, group)
			}
			return channel, group, nil
		}
	}
	return nil, param.TokenGroup, nil
}

// retryCandidateGroups lists the groups a retry may select from: the token
// group, or for an auto token every auto group until a channel was picked,
// then that group and, with cross-group retry, the groups after it.
func retryCandidateGroups(param *RetryParam) ([]string, error) {
	c := param.Ctx
	if param.TokenGroup != "auto" {
		return []string{param.TokenGroup}, nil
	}
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	autoGroups := GetRequestAutoGroups(c, userGroup)
	if len(autoGroups) == 0 {
		return nil, errors.New("auto groups is not enabled")
	}
	start := c.GetString(ginKeyAutoGroupStart)
	if start == "" {
		return autoGroups, nil
	}
	i := slices.Index(autoGroups, start)
	if i < 0 {
		return nil, nil
	}
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry) {
		return autoGroups[i : i+1], nil
	}
	return autoGroups[i:], nil
}

// StopWhenCandidatesExhausted turns a retry decision into a stop when no
// untried channel is left, so the decision recorded with the failed attempt
// (and the error log written for it) is the one that actually ended routing.
// A lookup error leaves the retry in place for the next selection to report.
func StopWhenCandidatesExhausted(param *RetryParam, decision PolicyDecision) PolicyDecision {
	if decision.Action != "retry" {
		return decision
	}
	groups, err := retryCandidateGroups(param)
	if err != nil {
		return decision
	}
	pick := model.ChannelPick{Tried: TriedChannels(param.Ctx), IncludeWarning: true}
	filters := GetChannelConstraints(param.Ctx).Filters
	for _, group := range groups {
		channel, _, err := model.GetRandomSatisfiedChannel(group, param.ModelName, pick, filters)
		if err != nil || channel != nil {
			return decision
		}
	}
	return PolicyDecision{Action: "stop", Reason: "candidates_exhausted", Source: "system"}
}

func pinnedTaskPluginIdentities(c *gin.Context, expected string) ([]int, []string) {
	if c == nil || expected == "" {
		return nil, nil
	}
	// A shared host endpoint and a native route several plugins declare both
	// admit a channel of any candidate.
	var generation *jsplugin.RoutingGeneration
	var candidates []*jsplugin.LoadedPlugin
	if value, exists := c.Get(jsplugin.ContextKeyPinnedEndpoint); exists {
		if pinned, ok := value.(jsplugin.PinnedEndpoint); ok && len(pinned.Candidates) > 1 {
			generation = pinned.Generation
			for _, candidate := range pinned.Candidates {
				candidates = append(candidates, candidate.Plugin)
			}
		}
	} else if value, exists := c.Get(jsplugin.ContextKeyPinnedRoute); exists {
		if pinned, ok := value.(jsplugin.PinnedRoute); ok && len(pinned.Candidates) > 1 {
			generation = pinned.Generation
			for _, candidate := range pinned.Candidates {
				candidates = append(candidates, candidate.Plugin)
			}
		}
	}
	if generation != nil {
		expectedFound := false
		channelTypes := make([]int, 0, len(candidates))
		pluginKeys := make([]string, 0, len(candidates))
		seen := make(map[int]struct{}, len(candidates))
		for _, candidate := range candidates {
			if candidate == nil {
				continue
			}
			if candidate.Meta.Key == expected {
				expectedFound = true
			}
			pluginKeys = append(pluginKeys, candidate.Meta.Key)
			for _, channelType := range candidate.Meta.ChannelTypes {
				if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
					continue
				}
				if _, duplicate := seen[channelType]; duplicate {
					continue
				}
				if plugin, indexed := generation.GetByChannelType(channelType); indexed && plugin == candidate {
					seen[channelType] = struct{}{}
					channelTypes = append(channelTypes, channelType)
				}
			}
		}
		if expectedFound {
			return channelTypes, pluginKeys
		}
	}
	value, exists := c.Get(jsplugin.ContextKeyPinnedPlugin)
	pinned, ok := value.(jsplugin.PinnedPlugin)
	if !exists || !ok || pinned.Generation == nil || pinned.Plugin == nil || pinned.Plugin.Meta.Key != expected {
		return nil, nil
	}
	channelTypes := make([]int, 0, len(pinned.Plugin.Meta.ChannelTypes))
	for _, channelType := range pinned.Plugin.Meta.ChannelTypes {
		if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
			continue
		}
		channelTypes = append(channelTypes, channelType)
	}
	return channelTypes, []string{expected}
}

// ChannelSelectError explains why SelectChannelForRequest found no channel.
// Callers render it for their transport: the HTTP distributor localizes
// MessageID with its own helpers and the Responses WebSocket relay wraps it in
// a NewAPIError. Message is set instead of MessageID when the text is a fixed
// error code that clients match on.
type ChannelSelectError struct {
	StatusCode int
	Code       types.ErrorCode
	MessageID  string
	Params     map[string]any
	Message    string
	// FilterKind and Channel identify a candidate rejected by request filters.
	FilterKind dto.ChannelFilterKind
	Channel    *model.Channel
	// NoAvailableChannel marks the "no channel for this group and model"
	// outcome so the distributor can name the claiming task plugin.
	NoAvailableChannel bool
}

// SelectChannelForRequest resolves the channel for one attempt with the rules
// shared by the HTTP distributor and the Responses WebSocket relay: a pinned
// channel wins, then session affinity (first attempt only), then a random
// eligible channel; every candidate must satisfy the request's channel
// filters. The group the channel was chosen from is returned for auto-group
// callers. The caller still applies SetupContextForSelectedChannel.
func SelectChannelForRequest(c *gin.Context, modelName string, retry *RetryParam) (*model.Channel, string, *ChannelSelectError) {
	constraints := GetChannelConstraints(c)
	if pin, found, overridden := constraints.ResolvedPin(); found {
		for _, lost := range overridden {
			logger.LogWarn(c, common.LogText(
				"channel pin overridden: winning_source=%s winning_channel_id=%d overridden_source=%s overridden_channel_id=%d",
				pin.Source, pin.ChannelId, lost.Source, lost.ChannelId,
			))
		}
		channel, err := model.CacheGetChannel(pin.ChannelId)
		if err != nil {
			return nil, "", pinnedChannelUnavailable(pin, http.StatusBadRequest, i18n.MsgDistributorInvalidChannelId)
		}
		if channel.Status != common.ChannelStatusEnabled {
			return nil, "", pinnedChannelUnavailable(pin, http.StatusForbidden, i18n.MsgDistributorChannelDisabled)
		}
		if ok, kind := model.ChannelSatisfiesFilters(channel, modelName, constraints.Filters); !ok {
			return nil, "", &ChannelSelectError{
				StatusCode: http.StatusBadRequest, Code: types.ErrorCode(kind), MessageID: i18n.MsgDistributorNoAvailableChannel,
				Params:     map[string]any{"Group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup), "Model": modelName},
				FilterKind: kind, Channel: channel,
			}
		}
		return channel, "", nil
	}

	usingGroup := retry.TokenGroup
	var channel *model.Channel
	var selectGroup string
	if retry.GetRetry() == 0 {
		if preferredChannelID, found := GetPreferredChannelByAffinity(c, modelName, usingGroup); found {
			affinityUsable := false
			preferred, err := model.CacheGetChannel(preferredChannelID)
			// The group the bound channel still serves for this model; a
			// channel that left every allowed group no longer satisfies it.
			memberGroup := ""
			if err == nil && preferred != nil && preferred.Status == common.ChannelStatusEnabled {
				if satisfied, _ := model.ChannelSatisfiesFilters(preferred, modelName, constraints.Filters); satisfied {
					groups := []string{usingGroup}
					if usingGroup == "auto" {
						groups = GetRequestAutoGroups(c, common.GetContextKeyString(c, constant.ContextKeyUserGroup))
					}
					for _, g := range groups {
						serves, lookupErr := model.ChannelServesGroupModel(g, modelName, preferred.Id)
						if lookupErr != nil {
							// Membership is unknown: keep the binding and fail
							// the selection rather than treat it as removed.
							return nil, g, &ChannelSelectError{
								StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeGetChannelFailed, MessageID: i18n.MsgDistributorGetChannelFailed,
								Params: map[string]any{"Group": g, "Model": modelName, "Error": lookupErr.Error()},
							}
						}
						if serves {
							memberGroup = g
							break
						}
					}
				}
			}
			// A suspension is temporary: the request goes elsewhere but the
			// binding stays, so the session returns once the channel recovers.
			affinitySuspended := memberGroup != "" && model.IsChannelSuspended(preferred.Id)
			if memberGroup != "" && !affinitySuspended {
				channel = preferred
				selectGroup = memberGroup
				affinityUsable = true
				if usingGroup == "auto" {
					common.SetContextKey(c, constant.ContextKeyAutoGroup, memberGroup)
					markAutoGroupStart(c, memberGroup)
				}
				MarkChannelAffinityUsed(c, memberGroup, preferred.Id)
			}
			if !affinityUsable && !affinitySuspended && !ShouldKeepChannelAffinityOnChannelDisabled() {
				ClearCurrentChannelAffinityCache(c)
			}
			if !affinityUsable && RequestPolicy(c).SessionMode == "strict" {
				return nil, "", &ChannelSelectError{StatusCode: http.StatusServiceUnavailable, Message: "strict_session_binding_unavailable"}
			}
		}
	}

	if channel == nil {
		var err error
		channel, selectGroup, err = CacheGetRandomSatisfiedChannel(retry)
		if err != nil {
			showGroup := usingGroup
			if usingGroup == "auto" {
				showGroup = fmt.Sprintf("auto(%s)", selectGroup)
			}
			return nil, selectGroup, &ChannelSelectError{
				StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeModelNotFound, MessageID: i18n.MsgDistributorGetChannelFailed,
				Params: map[string]any{"Group": showGroup, "Model": modelName, "Error": err.Error()},
			}
		}
		if channel == nil {
			return nil, selectGroup, &ChannelSelectError{
				StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeModelNotFound, MessageID: i18n.MsgDistributorNoAvailableChannel,
				Params: map[string]any{"Group": usingGroup, "Model": modelName}, NoAvailableChannel: true,
			}
		}
	}
	if ok, kind := model.ChannelSatisfiesFilters(channel, modelName, constraints.Filters); !ok {
		return nil, selectGroup, &ChannelSelectError{
			StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeModelNotFound, MessageID: i18n.MsgDistributorNoAvailableChannel,
			Params:     map[string]any{"Group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup), "Model": modelName},
			FilterKind: kind, Channel: channel, NoAvailableChannel: true,
		}
	}
	return channel, selectGroup, nil
}

// Origin-task pins report a fixed code so task polling can tell a retired
// channel from a malformed request.
func pinnedChannelUnavailable(pin dto.ChannelPin, statusCode int, messageID string) *ChannelSelectError {
	if pin.Source == dto.PinSourceOriginTask {
		return &ChannelSelectError{StatusCode: http.StatusBadRequest, Code: "origin_task_channel_disabled", Message: "origin_task_channel_disabled"}
	}
	return &ChannelSelectError{StatusCode: statusCode, MessageID: messageID}
}

// AppendUsedChannel records an attempted channel in the request's channel
// trail, which the retry log and the consume log's admin_info both read.
func AppendUsedChannel(c *gin.Context, channelID int) {
	c.Set("use_channel", append(c.GetStringSlice("use_channel"), fmt.Sprintf("%d", channelID)))
}
