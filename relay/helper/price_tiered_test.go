package helper

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withBillingSetting(t *testing.T, modes, exprs string) {
	t.Helper()
	cfg := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	saved, err := config.ConfigToMap(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(cfg, saved)) })
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{"billing_mode": modes, "billing_expr": exprs}))
}

func TestModelPriceHelperTieredPreConsume(t *testing.T) {
	withBillingSetting(t,
		`{"gpt-6-sol":"tiered_expr"}`,
		`{"gpt-6-sol":"len <= 1000 ? tier(\"short\", p * 10 + c * 50) : tier(\"long\", p * 20 + c * 75)"}`)

	savedPreConsumed := common.PreConsumedQuota
	common.PreConsumedQuota = 0
	t.Cleanup(func() { common.PreConsumedQuota = savedPreConsumed })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-6-sol"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("Authorization", "Bearer sk-secret")
	common.SetContextKey(ctx, constant.ContextKeyChannelRatio, 2.0)

	info := &relaycommon.RelayInfo{OriginModelName: "gpt-6-sol", UsingGroup: "default"}
	priceData, err := ModelPriceHelper(ctx, info, 2000, &types.TokenCountMeta{MaxTokens: 100})
	require.NoError(t, err)

	snap := info.TieredBillingSnapshot
	require.NotNil(t, snap)
	// len=2000 命中 long 档；预扣按 (2000+100) 个输入 token 估算：2100*20/1e6*500000 = 21000，× 渠道倍率 2
	assert.Equal(t, "long", snap.EstimatedTier)
	assert.Equal(t, 42000, priceData.QuotaToPreConsume)
	assert.False(t, priceData.UsePrice)
	assert.NotContains(t, info.BillingRequestInput.Headers, "Authorization")
	assert.Equal(t, `{"model":"gpt-6-sol"}`, string(info.BillingRequestInput.Body))
	assert.True(t, ContainPriceOrRatio("gpt-6-sol"))
}

func TestModelPriceHelperRatioClearsTieredSnapshot(t *testing.T) {
	withBillingSetting(t, `{}`, `{}`)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-4o", UsingGroup: "default"}
	info.UserSetting.AcceptUnsetRatioModel = true
	// 模拟上一次定价（如故障转移前）留下的阶梯快照
	info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: billing_setting.BillingModeTieredExpr}
	_, err := ModelPriceHelper(ctx, info, 10, &types.TokenCountMeta{})
	require.NoError(t, err)
	assert.Nil(t, info.TieredBillingSnapshot)
}

func TestBuiltinExprYieldsToConfiguredRatio(t *testing.T) {
	withBillingSetting(t, `{}`, `{}`)
	// gpt-6-astra 没有倍率配置时走内置阶梯表达式
	assert.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("gpt-6-astra"))
	// 管理员显式指定 ratio 时以配置为准
	withBillingSetting(t, `{"gpt-6-astra":"ratio"}`, `{}`)
	assert.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode("gpt-6-astra"))
}

func TestBillingSettingMapReplaceDropsRemovedModels(t *testing.T) {
	withBillingSetting(t, `{"a":"tiered_expr","b":"tiered_expr"}`, `{}`)
	withBillingSetting(t, `{"a":"tiered_expr"}`, `{}`)
	assert.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode("b"))
}

// 渠道测试构造的请求设置了 JSON Content-Type，但定价时 Body 仍为 nil，不能 panic
func TestModelPriceHelperTieredNilBody(t *testing.T) {
	withBillingSetting(t, `{"gpt-6-sol":"tiered_expr"}`, `{"gpt-6-sol":"tier(\"base\", p * 2 + c * 10)"}`)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = &http.Request{Method: http.MethodPost, Header: make(http.Header)}
	ctx.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{OriginModelName: "gpt-6-sol", UsingGroup: "default"}
	require.NotPanics(t, func() {
		_, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})
		require.NoError(t, err)
	})
	require.NotNil(t, info.TieredBillingSnapshot)
	assert.Empty(t, info.BillingRequestInput.Body)
}
