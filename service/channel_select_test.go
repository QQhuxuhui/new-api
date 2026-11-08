package service

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinnedTaskPluginChannelTypesUsesPinnedGenerationIndex(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectTaskPluginSource("legacy-select", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "legacy-select")
	assert.Equal(t, []int{constant.ChannelTypeKling}, types)
	assert.Equal(t, []string{"legacy-select"}, keys)
	types, keys = pinnedTaskPluginIdentities(c, "another-plugin")
	assert.Empty(t, types)
	assert.Empty(t, keys)
	types, keys = pinnedTaskPluginIdentities(nil, "legacy-select")
	assert.Empty(t, types)
	assert.Empty(t, keys)
}

func TestPinnedTaskPluginChannelTypesLeavesGenericChannelsKeyed(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectTaskPluginSource("generic-select", constant.ChannelTypeTaskPlugin), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "generic-select")
	assert.Empty(t, types)
	assert.Equal(t, []string{"generic-select"}, keys)
}

func TestPinnedTaskPluginChannelTypesIncludesSharedEndpointProviders(t *testing.T) {
	registry := jsplugin.NewRegistry()
	_, err := registry.Register(channelSelectEndpointPluginSource("gemini-select", constant.ChannelTypeGemini), jsplugin.Options{})
	require.NoError(t, err)
	_, err = registry.Register(channelSelectEndpointPluginSource("vertex-select", constant.ChannelTypeVertexAi), jsplugin.Options{})
	require.NoError(t, err)
	candidates := registry.Generation().LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
	})
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
		Protocol:   candidates[0].Protocol,
		Operation:  candidates[0].Operation,
		Model:      "task-model",
		Candidates: candidates,
	})

	AppendTaskPluginIdentityFilter(c, candidates[0].Plugin.Meta.Key)
	filters := GetChannelConstraints(c).Filters
	require.Len(t, filters, 1)
	assert.Equal(t, []int{constant.ChannelTypeGemini, constant.ChannelTypeVertexAi}, filters[0].TaskPluginChannelTypes)
	assert.Equal(t, []string{"gemini-select", "vertex-select"}, filters[0].TaskPluginKeys)
}

func channelSelectTaskPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  %s
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelSelectChannelTypesField(channelType))
}

func channelSelectEndpointPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  %s
  models: ["task-model"],
  fetchMode: "per_task",
  protocols: [{name: "openai_responses", supports: ["stream", "sync", "background"]}],
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
export const protocols = {openai_responses: {
  decodeRequest: function(ctx) { return {kind: "submit", model: "task-model", requestBody: ctx.body.value}; },
  renderEvents: function() { return {events: [], state: null, done: false}; },
  renderFinal: function() { return {output: []}; },
}};
`, key, key, channelSelectChannelTypesField(channelType))
}

func channelSelectChannelTypesField(channelType int) string {
	if channelType <= 0 || channelType == constant.ChannelTypeTaskPlugin {
		return ""
	}
	return fmt.Sprintf("channelTypes: [%d],", channelType)
}

func TestPinnedTaskPluginChannelTypesIncludesCompatibleTypes(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectCompatiblePluginSource("sora-select", constant.ChannelTypeSora, constant.ChannelTypeOpenAI), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "sora-select")
	assert.Equal(t, []int{constant.ChannelTypeSora, constant.ChannelTypeOpenAI}, types)
	assert.Equal(t, []string{"sora-select"}, keys)
}

func channelSelectCompatiblePluginSource(key string, channelType, compatibleType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d, %d],
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelType, compatibleType)
}

func TestSharedType61IdentityFilterContainsAllCandidateKeys(t *testing.T) {
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"alpha", "beta"} {
		_, err := registry.Register(channelSelectEndpointPluginSource(key, 0), jsplugin.Options{})
		require.NoError(t, err)
	}
	generation := registry.Generation()
	candidates := generation.LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)
	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Generation: generation, Plugin: candidates[0].Plugin, Candidates: candidates})
	AppendTaskPluginIdentityFilter(c, "alpha")
	filters := GetChannelConstraints(c).Filters
	require.Len(t, filters, 1)
	assert.Equal(t, "alpha", filters[0].TaskPluginKey)
	assert.Equal(t, []string{"alpha", "beta"}, filters[0].TaskPluginKeys)
	assert.Empty(t, filters[0].TaskPluginChannelTypes)
}

func TestSharedNativeRouteIdentityFilterContainsAllCandidateKeys(t *testing.T) {
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"route-alpha", "route-beta"} {
		_, err := registry.Register(fmt.Sprintf(`
export const meta = {apiVersion: 1, key: %q, name: %q, version: "1.0.0", author: {name: "Test"}, models: ["task-model"], fetchMode: "per_task",
	routes: [{method: "POST", path: "/probe/v1/jobs", type: "submit", decode: "decode", render: "render"}]};
export const native = {decode() { return {}; }, render() { return {}; }};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {}; }
`, key, key), jsplugin.Options{})
		require.NoError(t, err)
	}
	generation := registry.Generation()
	candidates := generation.LookupDeclaredRoutes("POST", "/probe/v1/jobs")
	require.Len(t, candidates, 2)
	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedRoute, jsplugin.PinnedRoute{Generation: generation, Plugin: candidates[0].Plugin, Route: candidates[0].Route, Candidates: candidates})
	AppendTaskPluginIdentityFilter(c, "route-alpha")
	filters := GetChannelConstraints(c).Filters
	require.Len(t, filters, 1)
	assert.Equal(t, "route-alpha", filters[0].TaskPluginKey)
	assert.Equal(t, []string{"route-alpha", "route-beta"}, filters[0].TaskPluginKeys)
}

func useChannelHealthRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	oldRDB, oldMemoryCache := common.RDB, common.MemoryCacheEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	// Memory-cache mode answers always-healthy lookups without a database.
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB, common.MemoryCacheEnabled = oldRDB, oldMemoryCache
	})
	return server
}

func TestChannelHealthSuspendsAfterConsecutiveHighFailureRate(t *testing.T) {
	server := useChannelHealthRedis(t)
	const id = 950001
	suspended := fmt.Sprintf(model.ChannelHealthSuspendedKey, id)
	warning := fmt.Sprintf(model.ChannelHealthWarningKey, id)

	RecordChannelFailure(id, http.StatusServiceUnavailable, "upstream unavailable")
	assert.False(t, server.Exists(warning), "one failure is too small a sample")
	RecordChannelFailure(id, http.StatusServiceUnavailable, "upstream unavailable")
	assert.True(t, server.Exists(warning), "2/2 failures flag the channel as warning")
	RecordChannelFailure(id, http.StatusServiceUnavailable, "upstream unavailable")
	assert.False(t, server.Exists(suspended), "two high-failure-rate periods are not enough to suspend")
	RecordChannelFailure(id, http.StatusServiceUnavailable, "upstream unavailable")
	assert.True(t, server.Exists(suspended), "the third consecutive high-failure-rate period suspends")
	assert.Equal(t, FixedSuspensionDuration, server.TTL(suspended))

	RecordChannelSuccess(id)
	assert.False(t, server.Exists(suspended), "a success lifts the suspension")
	assert.False(t, server.Exists(warning))
}

func TestChannelHealthSuspendsImmediatelyOnInvalidKey(t *testing.T) {
	server := useChannelHealthRedis(t)
	const id = 950002

	RecordChannelFailure(id, http.StatusUnauthorized, "Incorrect API key provided: invalid")
	assert.True(t, server.Exists(fmt.Sprintf(model.ChannelHealthSuspendedKey, id)))

	require.NoError(t, ResetChannelHealth(id))
	assert.False(t, server.Exists(fmt.Sprintf(model.ChannelHealthSuspendedKey, id)))
}

func TestShouldRecordChannelHealthFailure(t *testing.T) {
	useChannelErrorRules(t, `[
		{"id":"client","name":"prompt too long","keywords":["prompt is too long"],"match_type":"KEYWORD_ONLY","enabled":true,"error_type":"client"},
		{"id":"server","name":"key not valid","status_codes":[400],"keywords":["api key not valid"],"match_type":"AND","enabled":true,"error_type":"server"}
	]`)
	for _, test := range []struct {
		name string
		err  *types.NewAPIError
		want bool
	}{
		{name: "upstream 5xx", err: types.NewOpenAIError(errors.New("unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable), want: true},
		{name: "upstream 429", err: types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests), want: true},
		{name: "plain 400 is the request's fault", err: types.NewOpenAIError(errors.New("bad request"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)},
		{name: "server rule makes a 400 count", err: types.NewOpenAIError(errors.New("API key not valid"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest), want: true},
		{name: "client rule never counts", err: types.NewOpenAIError(errors.New("Prompt is too long"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)},
		{name: "skip-retry error", err: types.NewOpenAIError(errors.New("unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())},
		{name: "local conversion failure", err: types.NewError(errors.New("convert failed"), types.ErrorCodeConvertRequestFailed)},
		{name: "transport failure", err: types.NewError(errors.New("dial tcp: connection refused"), types.ErrorCodeDoRequestFailed), want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, ShouldRecordChannelHealthFailure(test.err))
		})
	}
}

func TestChannelHealthStaleFailureCannotResuspendAfterReset(t *testing.T) {
	server := useChannelHealthRedis(t)
	const id = 950003
	suspended := fmt.Sprintf(model.ChannelHealthSuspendedKey, id)
	for range 3 {
		RecordChannelFailure(id, http.StatusServiceUnavailable, "upstream unavailable")
	}
	require.False(t, server.Exists(suspended), "two high-failure-rate periods so far")

	// A reset lands between the earlier failures and the next one; the next
	// failure starts a new consecutive count instead of reusing the old one.
	require.NoError(t, ResetChannelHealth(id))
	RecordChannelFailure(id, http.StatusServiceUnavailable, "upstream unavailable")
	assert.False(t, server.Exists(suspended))
	got, err := server.Get(fmt.Sprintf(keyFailures, id))
	require.NoError(t, err)
	assert.Equal(t, "1", got)
}

func TestChannelHealthOutcomeJudgesStreamEnd(t *testing.T) {
	stream := func(reason relaycommon.StreamEndReason) *relaycommon.StreamStatus {
		status := relaycommon.NewStreamStatus()
		status.SetEndReason(reason, nil)
		return status
	}
	for _, test := range []struct {
		name         string
		stream       *relaycommon.StreamStatus
		wantSuccess  bool
		wantFailures bool
	}{
		{name: "non-stream success", wantSuccess: true},
		{name: "stream finished normally", stream: stream(relaycommon.StreamEndReasonDone), wantSuccess: true},
		{name: "upstream stream timeout", stream: stream(relaycommon.StreamEndReasonTimeout), wantFailures: true},
		{name: "upstream read error", stream: stream(relaycommon.StreamEndReasonScannerErr), wantFailures: true},
		{name: "client went away", stream: stream(relaycommon.StreamEndReasonClientGone)},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := useChannelHealthRedis(t)
			const id = 950004
			bucket := currentHealthBucket()
			totalKey := fmt.Sprintf(keyBucketTotal, id, bucket)
			failureKey := fmt.Sprintf(keyBucketFailures, id, bucket)
			successKey := fmt.Sprintf(keyTotalSuccesses, id)

			RecordChannelHealthOutcome(id, nil, test.stream)

			if !test.wantSuccess && !test.wantFailures {
				// Nothing is scheduled for a client disconnect.
				assert.False(t, server.Exists(totalKey))
				return
			}
			require.Eventually(t, func() bool { return server.Exists(totalKey) }, time.Second, 5*time.Millisecond)
			assert.Equal(t, test.wantFailures, server.Exists(failureKey))
			if test.wantSuccess {
				require.Eventually(t, func() bool { return server.Exists(successKey) }, time.Second, 5*time.Millisecond)
			}
		})
	}
}
