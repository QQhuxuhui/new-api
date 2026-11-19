package relay

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRelayChannelDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousCache := common.MemoryCacheEnabled
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}))
	model.DB = database
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.MemoryCacheEnabled = previousCache
		require.NoError(t, sqlDB.Close())
	})
	return database
}

func TestRealtimeFetchAppliesChannelHeaderOverrides(t *testing.T) {
	database := setupRelayChannelDB(t)
	service.InitHttpClient()
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		_, _ = w.Write([]byte(`{"name":"operations/task-1","done":false}`))
	}))
	t.Cleanup(server.Close)
	headerOverride := `{"X-API-Key":"{api_key}"}`
	channel := &model.Channel{
		Name: "realtime-header-override", Type: constant.ChannelTypeGemini,
		Key: "channel-key", BaseURL: &server.URL, HeaderOverride: &headerOverride,
	}
	require.NoError(t, database.Create(channel).Error)
	task := &model.Task{
		TaskID: "public-task", ChannelId: channel.Id,
		Status: model.TaskStatusInProgress, Progress: "50%",
		PrivateData: model.TaskPrivateData{
			Key: "saved-key", UpstreamTaskID: base64.RawURLEncoding.EncodeToString([]byte("operations/task-1")),
		},
	}

	tryRealtimeFetch(task, true)

	require.Len(t, requests, 1)
	request := <-requests
	assert.Equal(t, "/v1beta/operations/task-1", request.URL.Path)
	assert.Equal(t, "saved-key", request.Header.Get("X-API-Key"))
	assert.Equal(t, "saved-key", request.Header.Get("X-Goog-Api-Key"))
}

func TestRealtimeFetchRefundsTaskItFindsFailed(t *testing.T) {
	database := setupRelayChannelDB(t)
	require.NoError(t, database.AutoMigrate(&model.Task{}, &model.User{}, &model.Log{}))
	previousLogDB := model.LOG_DB
	previousFactory := service.GetTaskAdaptorFunc
	previousRedis := common.RedisEnabled
	model.LOG_DB = database
	common.RedisEnabled = false
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		if adaptor := GetTaskAdaptor(platform); adaptor != nil {
			return adaptor
		}
		return nil
	}
	t.Cleanup(func() {
		model.LOG_DB = previousLogDB
		service.GetTaskAdaptorFunc = previousFactory
		common.RedisEnabled = previousRedis
	})
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"operations/task-1","done":true,"error":{"code":3,"message":"blocked by safety"}}`))
	}))
	t.Cleanup(server.Close)
	channel := &model.Channel{Name: "realtime-refund", Type: constant.ChannelTypeGemini, Key: "channel-key", BaseURL: &server.URL}
	require.NoError(t, database.Create(channel).Error)
	require.NoError(t, database.Create(&model.User{Id: 7, Username: "realtime-owner", Quota: 100}).Error)
	task := &model.Task{
		TaskID: "public-task", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeGemini)),
		UserId: 7, ChannelId: channel.Id, Quota: 1000,
		Status: model.TaskStatusInProgress, Progress: "50%",
		PrivateData: model.TaskPrivateData{
			Key: "channel-key", UpstreamTaskID: base64.RawURLEncoding.EncodeToString([]byte("operations/task-1")),
		},
	}
	require.NoError(t, database.Create(task).Error)

	tryRealtimeFetch(task, true)

	var stored model.Task
	require.NoError(t, database.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), stored.Status)
	assert.Equal(t, "blocked by safety", stored.FailReason)
	assert.Zero(t, stored.Quota, "the failed task is refunded")
	var user model.User
	require.NoError(t, database.First(&user, 7).Error)
	assert.Equal(t, 1100, user.Quota)
}

func TestApplyChannelPinPreservesOriginTasksAndRetryMode(t *testing.T) {
	database := setupRelayChannelDB(t)
	channel := &model.Channel{Name: "origin-channel", Key: "sk-test", Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeDoubaoVideo}
	require.NoError(t, database.Create(channel).Error)
	originTask := &model.Task{
		TaskID: "task-lock", ChannelId: channel.Id, Action: "text_to_video", Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "upstream-task-lock"},
		Data:        []byte(`{"id":"upstream-task-lock"}`),
	}

	for _, tc := range []struct {
		name     string
		tokenPin bool
		apply    func(*gin.Context, *relaycommon.RelayInfo) *dto.TaskError
	}{
		{name: "origin affinity", apply: ApplyOriginTaskAffinity},
		{name: "same channel retry", apply: ApplyChannelPin},
		{name: "token pin suppresses channel lock", tokenPin: true, apply: ApplyChannelPin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(c, constant.ContextKeyOriginTasks, []*model.Task{originTask})
			constraints := service.GetChannelConstraints(c)
			constraints.AddPin(dto.ChannelPin{ChannelId: channel.Id, Source: dto.PinSourceOriginTask, Rank: dto.PinRankOriginTask, RetryMode: dto.PinRetrySameChannel})
			if tc.tokenPin {
				constraints.AddPin(dto.ChannelPin{ChannelId: channel.Id, Source: dto.PinSourceToken, Rank: dto.PinRankToken, RetryMode: dto.PinRetrySingleAttempt})
			}
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			require.Nil(t, tc.apply(c, info))
			if tc.tokenPin {
				assert.Nil(t, info.LockedChannel)
			} else {
				locked, ok := info.LockedChannel.(*model.Channel)
				require.True(t, ok)
				require.NotNil(t, locked)
				assert.Equal(t, channel.Id, locked.Id)
			}
			require.Len(t, info.OriginTasks, 1)
			assert.Equal(t, "task-lock", info.OriginTasks[0].TaskID)
			assert.Equal(t, "upstream-task-lock", info.OriginTasks[0].UpstreamTaskID)
			assert.Equal(t, "text_to_video", info.OriginTasks[0].Action)
			assert.Equal(t, string(model.TaskStatusSuccess), info.OriginTasks[0].Status)
			assert.Equal(t, []byte(originTask.Data), info.OriginTasks[0].Data)
		})
	}
}

func TestTaskModel2DtoNormalizesLegacyAction(t *testing.T) {
	task := &model.Task{Action: "firstTailGenerate"}

	dtoTask := TaskModel2Dto(task)

	assert.Equal(t, constant.TaskActionFirstTailToVideo, dtoTask.Action)
	assert.Equal(t, "firstTailGenerate", task.Action)
}

const mappingOrderSubmitPlugin = `
export const meta = {apiVersion:1,key:"maporder",name:"Map Order",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel, model: ctx.model}, action:"text_to_video"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

const mappingOrderRewritePlugin = `
export const meta = {apiVersion:1,key:"maporder-rw",name:"Map Order RW",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel}, rewriteModel:"rewritten"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

func pinMappingOrderPlugin(t *testing.T, c *gin.Context, source string) {
	t.Helper()
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
}

func newTaskSubmitContext(t *testing.T, originalModel, mapping string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, originalModel)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://provider.example")
	if mapping != "" {
		c.Set("model_mapping", mapping)
	}
	c.Set("task_request", map[string]any{"prompt": "p"})
	return c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
}

func TestRelayTaskSubmitMapsBeforeValidateWhenOriginSet(t *testing.T) {
	const mapping = `{"alias-model":"mid-model","mid-model":"declared-model"}`

	c, info := newTaskSubmitContext(t, "alias-model", mapping)
	pinMappingOrderPlugin(t, c, mappingOrderSubmitPlugin)
	info.OriginModelName = "alias-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "alias-model", info.OriginModelName)
	assert.Equal(t, "declared-model", info.UpstreamModelName)
	assert.True(t, info.IsModelMapped)
}

func TestRelayTaskSubmitDeclaredNameWithoutMappingIsUnchanged(t *testing.T) {
	c, info := newTaskSubmitContext(t, "declared-model", "")
	pinMappingOrderPlugin(t, c, mappingOrderSubmitPlugin)
	info.OriginModelName = "declared-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "declared-model", info.OriginModelName)
	assert.Equal(t, "declared-model", info.UpstreamModelName)
	assert.False(t, info.IsModelMapped)
}

func TestRelayTaskSubmitDoesNotApplyMappingTwice(t *testing.T) {
	c, info := newTaskSubmitContext(t, "alias-model", `{"alias-model":"declared-model"}`)
	pinMappingOrderPlugin(t, c, mappingOrderRewritePlugin)
	info.OriginModelName = "alias-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "rewritten", info.UpstreamModelName, "late mapping would overwrite rewriteModel with the chain tail")
	assert.Equal(t, "alias-model", info.OriginModelName)
}

func TestRelayTaskSubmitEmptyOriginKeepsLateMapping(t *testing.T) {
	plugin, err := pluginruntime.NewRegistry().Register(mappingOrderSubmitPlugin, pluginruntime.Options{})
	require.NoError(t, err)
	synthesized := service.CoverTaskActionToModelName(constant.TaskPlatform(plugin.Meta.Key), "text_to_video")
	c, info := newTaskSubmitContext(t, "pre-validate-upstream",
		`{"pre-validate-upstream":"should-not-apply-early","`+synthesized+`":"legacy-tail"}`)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
	info.OriginModelName = ""

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, synthesized, info.OriginModelName)
	assert.Equal(t, "legacy-tail", info.UpstreamModelName)
	assert.True(t, info.IsModelMapped)
}

const billingFallbackPlugin = `
export const meta = {apiVersion:1,key:"bill-fallback",name:"Bill Fallback",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel, model: ctx.model}, action:"text_to_video"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

func saveBillingConfig(t *testing.T) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
}

func TestRelayTaskSubmitAliasBillingIdentityAndExprFallback(t *testing.T) {
	const mapping = `{"alias-model":"declared-model"}`
	const aliasExpr = `tier("alias", 2)`
	const tailExpr = `tier("tail", 3)`
	const legacyExpr = `tier("legacy", u("old_units") * 2)`

	tests := []struct {
		name       string
		modes      map[string]string
		exprs      map[string]string
		wantTiered bool
		wantExpr   string
		source     string
	}{
		{
			name:       "alias own tiered wins",
			modes:      map[string]string{"alias-model": "tiered_expr", "declared-model": "tiered_expr"},
			exprs:      map[string]string{"alias-model": aliasExpr, "declared-model": tailExpr},
			wantTiered: true,
			wantExpr:   aliasExpr,
		},
		{
			name:       "fallback uses tail expr",
			modes:      map[string]string{"declared-model": "tiered_expr"},
			exprs:      map[string]string{"declared-model": tailExpr},
			wantTiered: true,
			wantExpr:   tailExpr,
		},
		{
			name:       "stored expression keeps running after schema narrows",
			modes:      map[string]string{"declared-model": "tiered_expr"},
			exprs:      map[string]string{"declared-model": legacyExpr},
			wantTiered: true,
			wantExpr:   legacyExpr,
			source: strings.Replace(billingFallbackPlugin, `fetchMode:"per_task"`, `fetchMode:"per_task", usageSchema:{old_units:{type:"number",unit:"count"}}, usageProfiles:[{models:["declared-model"],schema:{seconds:{type:"number",unit:"second"}}}]`, 1) + `
export function extractUsage(){return {old_units:2};}
`,
		},
		{
			name:       "neither tiered uses ordinary pricing",
			wantTiered: false,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			saveBillingConfig(t)
			if len(testCase.modes) > 0 {
				modeJSON, marshalErr := common.Marshal(testCase.modes)
				require.NoError(t, marshalErr)
				exprJSON, marshalErr := common.Marshal(testCase.exprs)
				require.NoError(t, marshalErr)
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
					"billing_setting.billing_mode": string(modeJSON),
					"billing_setting.billing_expr": string(exprJSON),
				}))
				if testCase.wantExpr == aliasExpr {
					require.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("alias-model"))
				} else {
					require.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode("alias-model"))
					require.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("declared-model"))
				}
			}

			c, info := newTaskSubmitContext(t, "alias-model", mapping)
			c.Set("group", "default")
			info.UserGroup = "default"
			info.UsingGroup = "default"
			source := testCase.source
			if source == "" {
				source = billingFallbackPlugin
			}
			pinMappingOrderPlugin(t, c, source)
			info.OriginModelName = "alias-model"

			_, taskErr := RelayTaskSubmit(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, "alias-model", info.OriginModelName)
			assert.Equal(t, "declared-model", info.UpstreamModelName)
			assert.True(t, info.IsModelMapped)

			task := model.InitTask(constant.TaskPlatform("bill-fallback"), info)
			assert.Equal(t, "alias-model", task.Properties.OriginModelName)
			assert.Equal(t, "declared-model", task.Properties.UpstreamModelName)

			if testCase.wantTiered {
				require.NotNil(t, info.TieredBillingSnapshot, "submission error: %+v", taskErr)
				assert.Equal(t, "alias-model", info.TieredBillingSnapshot.ModelName)
				assert.Equal(t, testCase.wantExpr, info.TieredBillingSnapshot.ExprString)
				assert.Equal(t, billingexpr.ExprHashString(testCase.wantExpr), info.TieredBillingSnapshot.ExprHash)
				assert.NotEqual(t, "model_price_error", taskErr.Code)
				if testCase.wantExpr == legacyExpr {
					assert.Equal(t, 4*common.QuotaPerUnit, info.TieredBillingSnapshot.EstimatedQuotaBeforeGroup)
				}
			} else {
				assert.Nil(t, info.TieredBillingSnapshot)
				assert.Equal(t, "model_price_error", taskErr.Code)
			}
		})
	}
}

func TestSharedTaskBillingExpressionSelectionAndFrozenSettlement(t *testing.T) {
	const baseExpr = `tier("base", u("seconds") * 2)`
	const alphaExpr = `tier("alpha", u("seconds") * 3)`
	const betaExpr = `tier("beta", u("credits") * 5)`
	const aliasExpr = `tier("alias", u("credits") * 7)`
	for _, tc := range []struct {
		name, plugin, model, mapping, modelExpr, mode, wantExpr string
		variants                                                map[string]string
		wantPriceError, profiled                                bool
	}{
		{name: "executing plugin override", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-alpha::declared-model": alphaExpr, "billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "override ignores model mode", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "ratio", variants: map[string]string{"billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "model expression fallback", plugin: "billing-alpha", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", wantExpr: baseExpr},
		{name: "alias override precedes mapped override", plugin: "billing-beta", model: "alias-model", mapping: `{"alias-model":"declared-model"}`, modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr, "billing-beta::alias-model": aliasExpr}, wantExpr: aliasExpr},
		{name: "mapped override precedes model fallback", plugin: "billing-beta", model: "alias-model", mapping: `{"alias-model":"declared-model"}`, modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "unconfigured plugin cannot use another schema", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", wantPriceError: true},
		{name: "missing usage in skipped branch remains incompatible", plugin: "billing-beta", model: "declared-model", modelExpr: `true ? tier("free", 0) : tier("missing", u("seconds"))`, mode: "tiered_expr", wantPriceError: true},
		{name: "fixed pricing is still rejected", plugin: "billing-beta", model: "declared-model", variants: map[string]string{"billing-beta::declared-model": `tier("fixed", fixed(1))`}, wantPriceError: true},
		{name: "endpoint mapping keeps the declared profile", plugin: "billing-beta", model: "declared-model", mapping: `{"declared-model":"ep-endpoint"}`, modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr}, wantExpr: betaExpr, profiled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveBillingConfig(t)
			registry := pluginruntime.NewRegistry()
			for _, spec := range []struct{ key, field, unit string }{{"billing-alpha", "seconds", "second"}, {"billing-beta", "credits", "credit"}} {
				source := strings.ReplaceAll(billingFallbackPlugin, "bill-fallback", spec.key)
				schema := `usageSchema:{` + spec.field + `:{type:"number",unit:"` + spec.unit + `"}}`
				if tc.profiled && spec.key == "billing-beta" {
					// The endpoint ID is undeclared, so only the declared model's profile carries this schema.
					schema = `usageSchema:{seconds:{type:"number",unit:"second"}},usageProfiles:[{models:["declared-model"],schema:{` + spec.field + `:{type:"number",unit:"` + spec.unit + `"}}}]`
				}
				source = strings.Replace(source, `fetchMode:"per_task"`, `fetchMode:"per_task",`+schema, 1)
				source += `export function extractUsage(){return {` + spec.field + `:2};}`
				_, err := registry.Register(source, pluginruntime.Options{})
				require.NoError(t, err)
			}
			variants := tc.variants
			if variants == nil {
				variants = map[string]string{}
			}
			rawVariants, err := common.Marshal(variants)
			require.NoError(t, err)
			modes, err := common.Marshal(map[string]string{"declared-model": tc.mode})
			require.NoError(t, err)
			expressions, err := common.Marshal(map[string]string{"declared-model": tc.modelExpr})
			require.NoError(t, err)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				billing_setting.PluginBillingExprOption: string(rawVariants), "billing_setting.billing_mode": string(modes), "billing_setting.billing_expr": string(expressions),
			}))
			c, info := newTaskSubmitContext(t, tc.model, tc.mapping)
			c.Set("group", "default")
			c.Set("task_plugin_key", tc.plugin)
			info.UserGroup = "default"
			info.UsingGroup = "default"
			info.OriginModelName = tc.model
			plugin, ok := registry.Generation().Get(tc.plugin)
			require.True(t, ok)
			c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
			_, taskErr := RelayTaskSubmit(c, info)
			require.NotNil(t, taskErr) // This fixture stops at reservation, before upstream submission.
			if tc.wantPriceError {
				assert.Equal(t, "model_price_error", taskErr.Code)
				assert.Nil(t, info.TieredBillingSnapshot)
				return
			}
			require.NotNil(t, info.TieredBillingSnapshot, "submission error: %+v", taskErr)
			assert.Equal(t, tc.wantExpr, info.TieredBillingSnapshot.ExprString)
			assert.NotEqual(t, "model_price_error", taskErr.Code)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{billing_setting.PluginBillingExprOption: `{}`, "billing_setting.billing_expr": `{}`}))
			field := "seconds"
			if tc.plugin == "billing-beta" {
				field = "credits"
			}
			result, usage, err := service.EvaluateTaskCompletionUsage(info.TieredBillingSnapshot, map[string]any{field: float64(4)})
			require.NoError(t, err)
			assert.Equal(t, float64(4), usage[field])
			assert.Equal(t, float64(2), info.TieredBillingSnapshot.UsageFacts[field])
			assert.Equal(t, 2*info.TieredBillingSnapshot.EstimatedQuotaAfterGroup, result.ActualQuotaAfterGroup)
			assert.Equal(t, tc.wantExpr, info.TieredBillingSnapshot.ExprString)
		})
	}
}

func TestSharedEndpointBillingKeepsOrdinaryAndPluginPricesSeparate(t *testing.T) {
	require.NoError(t, i18n.Init())
	const taskExpr = `tier("images", u("image_count") * 0.05)`
	const ordinaryExpr = `tier("images", fixed(0.04)) * image_count`
	savedPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices)) })
	for _, tc := range []struct {
		name, expression, override          string
		ordinaryQuota, taskQuota, taskFinal int
		wantError                           string
		alias, shared, legacy               bool
	}{
		{name: "task usage price is rejected by ordinary relay", expression: taskExpr, taskQuota: 50000, taskFinal: 75000, wantError: "task usage expression"},
		{name: "enum fallback cannot silently bill zero", expression: `u("output_image_type") == "2k" ? tier("2k", u("image_count") * 0.08) : tier("1k", 0.04)`, taskQuota: 20000, taskFinal: 20000, wantError: "task usage expression"},
		{name: "unselected usage branch is rejected", expression: `true ? tier("flat", 0.04) : tier("images", u("image_count"))`, taskQuota: 20000, taskFinal: 20000, wantError: "task usage expression"},
		{name: "dynamic usage key is rejected", expression: `tier("images", u(u("unit")) * 0.05)`, taskQuota: 50000, taskFinal: 75000, wantError: "task usage expression"},
		{name: "plain task USD price is ambiguous", expression: `tier("flat", 0.04)`, taskQuota: 20000, taskFinal: 20000, wantError: "shares its billing expression"},
		{name: "alias retains task USD price", expression: `tier("flat", 0.04)`, taskQuota: 20000, taskFinal: 20000, wantError: "shares its billing expression", alias: true},
		{name: "per image relay and plugin overrides", expression: ordinaryExpr, override: taskExpr, ordinaryQuota: 40000, taskQuota: 50000, taskFinal: 75000},
		{name: "mapped plugin override", expression: ordinaryExpr, override: taskExpr, ordinaryQuota: 40000, taskQuota: 50000, taskFinal: 75000, alias: true},
		{name: "token relay and plugin overrides", expression: `tier("tokens", p * 2)`, override: taskExpr, ordinaryQuota: 1000, taskQuota: 50000, taskFinal: 75000},
		{name: "ordinary constant has ordinary units with plugin override", expression: `tier("flat", 80000)`, override: taskExpr, ordinaryQuota: 40000, taskQuota: 50000, taskFinal: 75000},
		{name: "another plugin still inherits task USD units", expression: `tier("flat", 0.04)`, override: taskExpr, taskQuota: 50000, taskFinal: 75000, wantError: "shares its billing expression", shared: true},
		{name: "legacy per image pricing stays shared", ordinaryQuota: 40000, taskQuota: 40000, legacy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveBillingConfig(t)
			name, mapping, mappedModel := "declared-model", "", ""
			if tc.alias {
				name, mapping, mappedModel = "alias-model", `{"alias-model":"declared-model"}`, "declared-model"
			}
			mode := billing_setting.BillingModeTieredExpr
			if tc.legacy {
				mode = billing_setting.BillingModeRatio
			}
			variants := map[string]string{}
			if tc.override != "" {
				variants["bill-fallback::declared-model"] = tc.override
			}
			modes, err := common.Marshal(map[string]string{name: mode})
			require.NoError(t, err)
			expressions, err := common.Marshal(map[string]string{name: tc.expression})
			require.NoError(t, err)
			overrides, err := common.Marshal(variants)
			require.NoError(t, err)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_mode": string(modes), "billing_setting.billing_expr": string(expressions),
				billing_setting.PluginBillingExprOption: string(overrides), "group_ratio_setting.group_ratio": `{"default":1}`,
			}))
			prices, err := common.Marshal(map[string]float64{name: 0.04})
			require.NoError(t, err)
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
			registry := pluginruntime.NewRegistry()
			source := strings.Replace(billingFallbackPlugin, `fetchMode:"per_task"`, `fetchMode:"per_task",usageSchema:{image_count:{type:"number",unit:"count",description:"Image generation unit price"},output_image_type:{enum:["1k","2k"]},unit:{enum:["image_count"]}}`, 1)
			source += `export function extractUsage(){return {image_count:2,output_image_type:"1k",unit:"image_count"};}`
			plugin, err := registry.Register(source, pluginruntime.Options{})
			require.NoError(t, err)
			candidates := []pluginruntime.ProtocolBinding{{Plugin: plugin}}
			if tc.shared {
				other, err := registry.Register(strings.ReplaceAll(source, "bill-fallback", "other-images"), pluginruntime.Options{})
				require.NoError(t, err)
				candidates = append(candidates, pluginruntime.ProtocolBinding{Plugin: other})
			}
			ordinaryContext, _ := gin.CreateTestContext(httptest.NewRecorder())
			ordinaryContext.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			ordinaryContext.Request.Header.Set("Accept-Language", "en")
			ordinaryContext.Set(pluginruntime.ContextKeyRelayEndpoint, pluginruntime.PinnedEndpoint{Generation: registry.Generation(), Model: name, MappedModel: mappedModel, Candidates: candidates})
			ordinary := &relaycommon.RelayInfo{
				OriginModelName: name, UserGroup: "default", UsingGroup: "default", RelayFormat: kittypes.RelayFormatOpenAIImage,
				Request: &kitdto.ImageRequest{Model: name, N: common.GetPointer(uint(2))}, BillingRequestInput: &billingexpr.RequestInput{},
			}
			ordinaryPrice, err := helper.ModelPriceHelper(ordinaryContext, ordinary, 1000, &kittypes.TokenCountMeta{})
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				assert.Nil(t, ordinary.TieredBillingSnapshot)
				assert.Nil(t, ordinary.Billing, "pricing must fail before reservation")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.ordinaryQuota, ordinaryPrice.QuotaToPreConsume)
			}

			pluginContext, task := newTaskSubmitContext(t, name, mapping)
			pluginContext.Set("group", "default")
			pluginContext.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
			task.OriginModelName, task.UserGroup, task.UsingGroup = name, "default", "default"
			_, taskErr := RelayTaskSubmit(pluginContext, task)
			require.NotNil(t, taskErr) // This fixture stops at reservation, before upstream submission.
			assert.NotEqual(t, "model_price_error", taskErr.Code)
			assert.Equal(t, tc.taskQuota, task.PriceData.Quota)
			if tc.legacy {
				assert.Nil(t, task.TieredBillingSnapshot)
				assert.Nil(t, ordinary.TieredBillingSnapshot)
				return
			}
			require.NotNil(t, task.TieredBillingSnapshot)
			assert.True(t, task.TieredBillingSnapshot.TaskUsageBilling)
			// Updating live prices must not change either in-flight meter.
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_expr": `{}`, billing_setting.PluginBillingExprOption: `{}`,
			}))
			settlement, _, err := service.EvaluateTaskCompletionUsage(task.TieredBillingSnapshot, map[string]any{"image_count": float64(3)})
			require.NoError(t, err)
			assert.Equal(t, tc.taskFinal, settlement.ActualQuotaAfterGroup)
			if tc.wantError != "" {
				return
			}
			require.NotNil(t, ordinary.TieredBillingSnapshot)
			assert.False(t, ordinary.TieredBillingSnapshot.TaskUsageBilling)
			settlement, err = billingexpr.ComputeTieredQuotaWithRequest(ordinary.TieredBillingSnapshot, billingexpr.TokenParams{P: 1000, Len: 1000}, *ordinary.BillingRequestInput)
			require.NoError(t, err)
			assert.Equal(t, tc.ordinaryQuota, settlement.ActualQuotaAfterGroup)
		})
	}
}

// Issue #7478: task APIs answer 201 Created or 202 Accepted on submission.
// Every 2xx must reach parseSubmitResponse; only other statuses are upstream
// failures that keep the upstream status and body.
func TestRelayTaskSubmitAcceptsAnySuccessfulUpstreamStatus(t *testing.T) {
	service.InitHttpClient()
	const source = `
export const meta = {apiVersion:1,key:"status-echo",name:"Status Echo",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) { return {url: ctx.baseUrl+"/submit", method:"POST", body:{model: ctx.model}, action:"text_to_video"}; }
export function parseSubmitResponse(ctx, response) { return {taskId: response.body.id, taskData: {status: response.statusCode}}; }
export function buildQueryRequest(ctx) { return {url: ctx.baseUrl+"/query"}; }
export function parseTaskResult() { return {status:"SUCCESS"}; }
`
	for _, tc := range []struct {
		status   int
		wantCode string
	}{
		{status: http.StatusOK},
		{status: http.StatusCreated},
		{status: http.StatusAccepted},
		{status: http.StatusBadRequest, wantCode: "fail_to_fetch_task"},
		{status: http.StatusBadGateway, wantCode: "fail_to_fetch_task"},
	} {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			saveBillingConfig(t)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_mode": `{"declared-model":"tiered_expr"}`,
				"billing_setting.billing_expr": `{"declared-model":"tier(\"flat\", 3)"}`,
			}))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"id":"job-42","message":"upstream body"}`))
			}))
			defer server.Close()

			c, info := newTaskSubmitContext(t, "declared-model", "")
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
			c.Set("group", "default")
			info.UserGroup, info.UsingGroup = "default", "default"
			info.OriginModelName = "declared-model"
			// An existing reservation stands in for the wallet so the fixture reaches the upstream call.
			info.Billing = &imageReservation{limit: 1 << 30}
			pinMappingOrderPlugin(t, c, source)

			result, taskErr := RelayTaskSubmit(c, info)
			if tc.wantCode != "" {
				require.NotNil(t, taskErr)
				assert.Equal(t, tc.wantCode, taskErr.Code)
				assert.Equal(t, tc.status, taskErr.StatusCode)
				assert.Contains(t, taskErr.Message, "upstream body")
				assert.Nil(t, result)
				return
			}
			require.Nil(t, taskErr, "submission error: %+v", taskErr)
			require.NotNil(t, result)
			assert.Equal(t, "job-42", result.UpstreamTaskID)
			assert.JSONEq(t, `{"status":`+strconv.Itoa(tc.status)+`}`, string(result.TaskData))
		})
	}
}

// A retry prices the plugin it lands on from scratch and holds that price
// before the upstream sees the request. Plugin A fails upstream on the first
// attempt; the retry runs on plugin B, pinned the way
// SetupContextForSelectedChannel pins another candidate plugin's channel.
func TestRelayTaskSubmitRetryRepricesAndReservesBeforeUpstream(t *testing.T) {
	service.InitHttpClient()
	const source = `
export const meta = {apiVersion:1,key:"KEY",name:"KEY",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task",usageSchema:{seconds:{type:"number",unit:"second",description:"Video generation unit price"}}};
export function buildSubmitRequest(ctx) { return {url: ctx.baseUrl+"/submit", method:"POST", body:{model: ctx.model}, action:"text_to_video"}; }
export function parseSubmitResponse(ctx, response) { return {taskId: response.body.id}; }
export function buildQueryRequest(ctx) { return {url: ctx.baseUrl+"/query"}; }
export function parseTaskResult() { return {status:"SUCCESS"}; }
export function extractUsage() { return {seconds:2}; }
`
	pricier := map[string]string{"retry-a::declared-model": `tier("a", u("seconds") * 0.05)`, "retry-b::declared-model": `tier("b", u("seconds") * 0.4)`}
	for _, tc := range []struct {
		name, perCallPrice, wantCode string
		expressions                  map[string]string
		wallet, wantQuota            int
		wantSubmitted                []string
	}{
		{name: "per-call plugin drops the previous expression", expressions: map[string]string{"retry-a::declared-model": `tier("a", u("seconds") * 0.02)`}, perCallPrice: `{"declared-model":0.04}`, wallet: 1 << 30, wantQuota: 40000, wantSubmitted: []string{"a", "b"}},
		{name: "pricier plugin is held before submission", expressions: pricier, perCallPrice: `{}`, wallet: 1 << 30, wantQuota: 400000, wantSubmitted: []string{"a", "b"}},
		{name: "unaffordable plugin is never submitted", expressions: pricier, perCallPrice: `{}`, wallet: 100000, wantCode: "insufficient_user_quota", wantSubmitted: []string{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveBillingConfig(t)
			savedPrices := ratio_setting.ModelPrice2JSONString()
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices)) })
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(tc.perCallPrice))
			expressions, err := common.Marshal(tc.expressions)
			require.NoError(t, err)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				billing_setting.PluginBillingExprOption: string(expressions),
				"billing_setting.billing_mode":          `{}`,
				"billing_setting.billing_expr":          `{}`,
				"group_ratio_setting.group_ratio":       `{"default":1}`,
			}))
			registry := pluginruntime.NewRegistry()
			for _, key := range []string{"retry-a", "retry-b"} {
				_, err := registry.Register(strings.ReplaceAll(source, "KEY", key), pluginruntime.Options{})
				require.NoError(t, err)
			}
			// The fake stands in for the session the first attempt created; its
			// limit is the most the wallet can hold.
			billing := &imageReservation{limit: tc.wallet}
			var submitted []string
			var heldAtSubmit []int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
				submitted = append(submitted, attempt)
				heldAtSubmit = append(heldAtSubmit, billing.held)
				w.Header().Set("Content-Type", "application/json")
				if attempt == "a" {
					w.WriteHeader(http.StatusInternalServerError)
				}
				_, _ = w.Write([]byte(`{"id":"job-` + attempt + `"}`))
			}))
			defer server.Close()

			c, info := newTaskSubmitContext(t, "declared-model", "")
			c.Set("group", "default")
			info.UserGroup, info.UsingGroup, info.OriginModelName = "default", "default", "declared-model"
			info.Billing = billing
			var result *TaskSubmitResult
			var taskErr *dto.TaskError
			for _, attempt := range []string{"a", "b"} {
				plugin, ok := registry.Generation().Get("retry-" + attempt)
				require.True(t, ok)
				c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
				c.Set("task_plugin_key", plugin.Meta.Key)
				common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL+"/"+attempt)
				result, taskErr = RelayTaskSubmit(c, info)
				if attempt == "a" {
					require.NotNil(t, taskErr)
					require.Equal(t, http.StatusInternalServerError, taskErr.StatusCode, "the first attempt fails upstream and is retried")
				}
			}

			require.Equal(t, tc.wantSubmitted, submitted)
			if tc.wantCode != "" {
				require.NotNil(t, taskErr)
				assert.Equal(t, tc.wantCode, taskErr.Code)
				assert.Equal(t, http.StatusForbidden, taskErr.StatusCode)
				return
			}
			require.Nil(t, taskErr, "submission error: %+v", taskErr)
			assert.Equal(t, tc.wantQuota, result.Quota)
			assert.Equal(t, tc.wantQuota, heldAtSubmit[1], "plugin B must not see the request before its price is held")
			if tc.perCallPrice != `{}` {
				assert.Nil(t, info.TieredBillingSnapshot, "a per-call plugin must not settle with the previous plugin's expression")
				return
			}
			require.NotNil(t, info.TieredBillingSnapshot)
			assert.Equal(t, tc.expressions["retry-b::declared-model"], info.TieredBillingSnapshot.ExprString)
		})
	}
}

// A remix arrives without the distributor and before any channel meta. It
// locks the origin task's channel and is submitted to the vendor as a remix
// of the origin task's upstream video, not as a new generation.
func TestRemixSubmitsRemixOfOriginUpstreamVideo(t *testing.T) {
	database := setupRelayChannelDB(t)
	require.NoError(t, database.AutoMigrate(&model.Task{}))
	saveBillingConfig(t)
	// Priced per second of video: a remix is billed on its origin's length.
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		billing_setting.PluginBillingExprOption: `{"sora::sora-2":"tier(\"base\", u(\"seconds\") * 0.1)"}`,
		"billing_setting.billing_mode":          `{}`,
		"billing_setting.billing_expr":          `{}`,
		"group_ratio_setting.group_ratio":       `{"default":1}`,
	}))
	service.InitHttpClient()
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"video_remixed","status":"queued"}`))
	}))
	t.Cleanup(server.Close)
	origin := &model.Channel{Name: "sora", Type: constant.ChannelTypeSora, Key: "origin-key", Status: common.ChannelStatusEnabled, BaseURL: &server.URL}
	require.NoError(t, database.Create(origin).Error)
	require.NoError(t, database.Create(&model.Task{TaskID: "task_public", UserId: 7, ChannelId: origin.Id, Platform: constant.TaskPlatform("sora"),
		Properties:  model.Properties{OriginModelName: "sora-2"},
		Data:        []byte(`{"id":"video_upstream","status":"completed","seconds":"8","size":"1280x720"}`),
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "video_upstream"}}).Error)
	source, err := builtinplugins.Source("sora")
	require.NoError(t, err)
	plugin, err := pluginruntime.NewRegistry().RegisterFactory(source, pluginruntime.Options{Key: "sora"})
	require.NoError(t, err)

	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/task_public/remix", strings.NewReader(`{"prompt":"make it blue"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "video_id", Value: "task_public"}}
	c.Set("group", "default")
	info, err := relaycommon.GenRelayInfo(c, kittypes.RelayFormatTask, nil, nil)
	require.NoError(t, err)
	require.False(t, info.HasChannelMeta())
	info.UserId, info.UserGroup, info.UsingGroup = 7, "default", "default"

	require.Nil(t, ResolveOriginTask(c, info))
	locked, ok := info.LockedChannel.(*model.Channel)
	require.True(t, ok)
	assert.Equal(t, origin.Id, locked.Id)
	assert.Equal(t, "sora-2", info.OriginModelName)
	assert.Zero(t, common.GetContextKeyInt(c, constant.ContextKeyChannelId), "the submission loop sets the locked channel up")
	assert.Empty(t, c.GetString("expected_task_plugin_key"), "a Sora channel executes its own plugin")

	// On a New API channel, which hosts several plugins, the origin task's
	// plugin is the one the channel setup must bind.
	require.NoError(t, database.Model(origin).Update("type", constant.ChannelTypeNewAPI).Error)
	gateway, _ := gin.CreateTestContext(httptest.NewRecorder())
	gateway.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/task_public/remix", strings.NewReader(`{"prompt":"make it blue"}`))
	gateway.Params = c.Params
	gatewayInfo, err := relaycommon.GenRelayInfo(gateway, kittypes.RelayFormatTask, nil, nil)
	require.NoError(t, err)
	gatewayInfo.UserId = 7
	require.Nil(t, ResolveOriginTask(gateway, gatewayInfo))
	assert.Equal(t, "sora", gateway.GetString("expected_task_plugin_key"))

	// The submission loop's setup of the locked channel.
	common.SetContextKey(c, constant.ContextKeyChannelId, origin.Id)
	common.SetContextKey(c, constant.ContextKeyChannelType, origin.Type)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "origin-key")
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
	c.Set("task_plugin_key", "sora")
	billing := &imageReservation{limit: 1 << 30}
	info.Billing = billing
	_, taskErr := RelayTaskSubmit(c, info)
	require.Nil(t, taskErr, "submission error: %+v", taskErr)
	assert.Equal(t, []string{"POST /v1/videos/video_upstream/remix"}, paths)
	assert.Equal(t, constant.TaskActionRemix, info.Action)
	assert.Equal(t, common.QuotaRound(8*0.1*common.QuotaPerUnit), billing.held, "8 seconds of the origin video at 0.1 per second")

	// Another node disabled the channel and this node's cache has not synced
	// yet: the remix still stops before reaching the upstream.
	require.NoError(t, database.AutoMigrate(&model.Ability{}))
	common.MemoryCacheEnabled = true
	model.InitChannelCache()
	require.NoError(t, database.Model(origin).Update("status", common.ChannelStatusManuallyDisabled).Error)
	cached, err := model.CacheGetChannel(origin.Id)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, cached.Status, "the cache is stale")
	stale, _ := gin.CreateTestContext(httptest.NewRecorder())
	stale.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/task_public/remix", strings.NewReader(`{"prompt":"make it blue"}`))
	stale.Params = c.Params
	staleInfo, err := relaycommon.GenRelayInfo(stale, kittypes.RelayFormatTask, nil, nil)
	require.NoError(t, err)
	staleInfo.UserId = 7
	taskErr = ResolveOriginTask(stale, staleInfo)
	require.NotNil(t, taskErr)
	assert.Equal(t, "task_channel_disable", taskErr.Code)
	assert.Nil(t, staleInfo.LockedChannel)

	// A channel another node just created is not in this node's cache yet;
	// its origin task can still be remixed.
	fresh := &model.Channel{Name: "sora-fresh", Type: constant.ChannelTypeSora, Key: "fresh-key", Status: common.ChannelStatusEnabled, BaseURL: &server.URL}
	require.NoError(t, database.Create(fresh).Error)
	require.NoError(t, database.Create(&model.Task{TaskID: "task_fresh", UserId: 7, ChannelId: fresh.Id, Platform: constant.TaskPlatform("sora"),
		Properties:  model.Properties{OriginModelName: "sora-2"},
		Data:        []byte(`{"id":"video_fresh","status":"completed","seconds":"8","size":"1280x720"}`),
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "video_fresh"}}).Error)
	_, err = model.CacheGetChannel(fresh.Id)
	require.Error(t, err, "the cache has not synced")
	freshCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	freshCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/task_fresh/remix", strings.NewReader(`{"prompt":"make it blue"}`))
	freshCtx.Params = gin.Params{{Key: "video_id", Value: "task_fresh"}}
	freshInfo, err := relaycommon.GenRelayInfo(freshCtx, kittypes.RelayFormatTask, nil, nil)
	require.NoError(t, err)
	freshInfo.UserId = 7
	require.Nil(t, ResolveOriginTask(freshCtx, freshInfo))
	locked, ok = freshInfo.LockedChannel.(*model.Channel)
	require.True(t, ok)
	assert.Equal(t, fresh.Id, locked.Id)
}

// An upstream New API gateway that lost its own upstream's reply answers with
// the outcome header; this gateway then keeps the submission off its other
// channels as well, since the task may already exist.
func TestRelayTaskSubmitKeepsUpstreamGatewayOutcomeUnknown(t *testing.T) {
	service.InitHttpClient()
	saveBillingConfig(t)
	savedPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices)) })
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"declared-model":0.01}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{}`,
		"billing_setting.billing_expr":    `{}`,
		"group_ratio_setting.group_ratio": `{"default":1}`,
	}))
	const source = `
export const meta = {apiVersion:1,key:"outcome",name:"outcome",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) { return {url: ctx.baseUrl+"/submit", method:"POST", body:{model: ctx.model}}; }
export function parseSubmitResponse(ctx, response) { return {taskId: response.body.id}; }
export function buildQueryRequest(ctx) { return {url: ctx.baseUrl+"/query"}; }
export function parseTaskResult() { return {status:"SUCCESS"}; }
`
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	for _, tc := range []struct {
		outcome    string
		status     int
		wantStatus int
		large      bool
	}{
		{outcome: "unknown", status: http.StatusInternalServerError, wantStatus: http.StatusInternalServerError},
		// A native error renderer may answer a failure with 200.
		{outcome: "unknown", status: http.StatusOK, wantStatus: http.StatusBadGateway},
		{outcome: "", status: http.StatusInternalServerError, wantStatus: http.StatusInternalServerError},
		// An oversized failure reply is read only up to the limit.
		{outcome: "unknown", status: http.StatusOK, wantStatus: http.StatusBadGateway, large: true},
	} {
		outcome := tc.outcome
		t.Run(fmt.Sprintf("outcome=%s/status=%d/large=%t", outcome, tc.status, tc.large), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if outcome != "" {
					w.Header().Set(service.TaskOutcomeHeader, outcome)
				}
				w.WriteHeader(tc.status)
				if tc.large {
					_, _ = w.Write([]byte(strings.Repeat("x", 2*taskErrorBodyLimit)))
					return
				}
				_, _ = w.Write([]byte(`{"code":"upstream_error","message":"failed"}`))
			}))
			defer server.Close()
			c, info := newTaskSubmitContext(t, "declared-model", "")
			c.Set("group", "default")
			c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
			info.UserGroup, info.UsingGroup, info.OriginModelName = "default", "default", "declared-model"
			info.Billing = &imageReservation{limit: 1 << 30}

			_, taskErr := RelayTaskSubmit(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, tc.wantStatus, taskErr.StatusCode)
			assert.Equal(t, outcome == "unknown", taskErr.SkipRetry)
			assert.Equal(t, outcome == "unknown", taskErr.Code == service.TaskOutcomeUnknownCode)
			assert.LessOrEqual(t, len(taskErr.Message), taskErrorBodyLimit)
		})
	}
}

// A remix on a plugin that builds some other action is rejected before any
// billing or upstream request, rather than submitted as a new generation.
func TestRemixRejectedByPluginWithoutRemix(t *testing.T) {
	const source = `
export const meta = {apiVersion:1,key:"noremix",name:"noremix",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) { return {url: ctx.baseUrl+"/generate", method:"POST", body:{model: ctx.model}, action:"text_to_video"}; }
export function parseSubmitResponse(ctx, response) { return {taskId: response.body.id}; }
export function buildQueryRequest(ctx) { return {url: ctx.baseUrl+"/query"}; }
export function parseTaskResult() { return {status:"SUCCESS"}; }
`
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	c, info := newTaskSubmitContext(t, "declared-model", "")
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
	info.OriginModelName, info.Action, info.OriginTaskID = "declared-model", constant.TaskActionRemix, "video_upstream"
	billing := &imageReservation{limit: 1 << 30}
	info.Billing = billing

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Contains(t, taskErr.Message, "does not support remix")
	assert.Zero(t, requests)
	assert.Zero(t, billing.held)
}
