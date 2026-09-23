package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testQuotaPerUnit = 500_000.0

func tieredRelayInfo(expr string, groupRatio float64) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:  billing_setting.BillingModeTieredExpr,
			ExprString:   expr,
			ExprHash:     billingexpr.ExprHashString(expr),
			GroupRatio:   groupRatio,
			QuotaPerUnit: testQuotaPerUnit,
		},
		PriceData: types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: groupRatio}},
	}
}

func testGinContext(channelRatio, channelModelRatio float64) *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	if channelRatio != 0 {
		common.SetContextKey(ctx, constant.ContextKeyChannelRatio, channelRatio)
	}
	if channelModelRatio != 0 {
		common.SetContextKey(ctx, constant.ContextKeyChannelModelRatio, channelModelRatio)
	}
	return ctx
}

// gpt-6-astra 用例与上游 builtin_billing_test.go 保持一致
func TestGPT6AstraBuiltinTieredSettle(t *testing.T) {
	expression, ok := billing_setting.GetBuiltinBillingExpr("gpt-6-astra")
	require.True(t, ok)

	for _, tc := range []struct {
		name                           string
		input, output, cached, written int
		quota                          int
	}{
		{"standard", 1000, 100, 0, 0, 7500},
		{"cache at context boundary", 272000, 1000, 200000, 20000, 510000},
		{"whole request above boundary", 272001, 1000, 200000, 20000, 1007510},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := &dto.Usage{
				PromptTokens: tc.input, CompletionTokens: tc.output,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: tc.cached, CachedCreationTokens: tc.written},
			}
			ok, quota, result := TryTieredSettle(testGinContext(0, 0), tieredRelayInfo(expression, 1), TieredUsageFromDTO(usage))
			require.True(t, ok)
			require.NotNil(t, result)
			assert.Equal(t, tc.quota, quota)
		})
	}
}

func TestTryTieredSettleAppliesGroupAndChannelRatios(t *testing.T) {
	expr := `tier("base", p * 2 + c * 10)`
	usage := &dto.Usage{PromptTokens: 1000, CompletionTokens: 100}
	// (1000*2 + 100*10) / 1e6 * 500000 = 1500；× 分组 2 × 渠道 1.5 × 渠道模型 0.5 = 2250
	ok, quota, _ := TryTieredSettle(testGinContext(1.5, 0.5), tieredRelayInfo(expr, 2), TieredUsageFromDTO(usage))
	require.True(t, ok)
	assert.Equal(t, 2250, quota)
}

func TestTryTieredSettleSkipsRatioBilling(t *testing.T) {
	ok, _, _ := TryTieredSettle(testGinContext(0, 0), &relaycommon.RelayInfo{}, TieredUsage{Prompt: 10})
	assert.False(t, ok)
}

func TestTryTieredSettleFallsBackToPreConsumeOnError(t *testing.T) {
	info := tieredRelayInfo(`tier("x", p / 0 * 0 + header("x"))`, 1)
	info.FinalPreConsumedQuota = 321
	ok, quota, result := TryTieredSettle(testGinContext(0, 0), info, TieredUsage{Prompt: 10, PromptIncludesCache: true})
	require.True(t, ok)
	assert.Nil(t, result)
	assert.Equal(t, 321, quota)
}

func TestBuildTieredTokenParams(t *testing.T) {
	cacheVars := map[string]bool{"p": true, "c": true, "cr": true, "cc": true, "cc1h": true}

	t.Run("openai semantics subtracts separately priced parts", func(t *testing.T) {
		params := BuildTieredTokenParams(TieredUsage{
			Prompt: 1000, Completion: 50, CacheRead: 300, CacheCreation: 200, CacheCreation1h: 50,
			PromptIncludesCache: true,
		}, cacheVars)
		assert.Equal(t, 1000.0, params.Len)
		assert.Equal(t, 500.0, params.P)
		assert.Equal(t, 300.0, params.CR)
		assert.Equal(t, 150.0, params.CC)
		assert.Equal(t, 50.0, params.CC1h)
	})

	t.Run("openai semantics keeps unpriced parts in p", func(t *testing.T) {
		params := BuildTieredTokenParams(TieredUsage{
			Prompt: 1000, CacheRead: 300, PromptIncludesCache: true,
		}, map[string]bool{"p": true, "c": true})
		assert.Equal(t, 1000.0, params.P)
	})

	t.Run("anthropic semantics adds cache to len", func(t *testing.T) {
		params := BuildTieredTokenParams(TieredUsage{
			Prompt: 100, CacheRead: 300, CacheCreation: 200, CacheCreation1h: 50,
		}, cacheVars)
		assert.Equal(t, 600.0, params.Len)
		assert.Equal(t, 100.0, params.P)
	})

	t.Run("anthropic semantics merges unpriced cache into p", func(t *testing.T) {
		params := BuildTieredTokenParams(TieredUsage{
			Prompt: 100, CacheRead: 300, CacheCreation: 200,
		}, map[string]bool{"p": true, "c": true})
		assert.Equal(t, 600.0, params.P)
	})

	t.Run("negative remainder clamps to zero", func(t *testing.T) {
		params := BuildTieredTokenParams(TieredUsage{
			Prompt: 100, CacheRead: 80, CacheCreation: 80, PromptIncludesCache: true,
		}, cacheVars)
		assert.Equal(t, 0.0, params.P)
	})
}

func TestInjectTieredBillingInfo(t *testing.T) {
	expr := `len <= 100 ? tier("short", p * 1) : tier("long", p * 2)`
	info := tieredRelayInfo(expr, 1)
	info.TieredBillingSnapshot.EstimatedTier = "short"
	ok, _, result := TryTieredSettle(testGinContext(0, 0), info, TieredUsage{Prompt: 200, PromptIncludesCache: true})
	require.True(t, ok)

	other := map[string]interface{}{}
	InjectTieredBillingInfo(other, info, result)
	assert.Equal(t, billing_setting.BillingModeTieredExpr, other["billing_mode"])
	assert.Equal(t, "long", other["matched_tier"])
	assert.Equal(t, "short", other["estimated_tier"])
	assert.NotEmpty(t, other["expr_b64"])
}

// 按次计价与 token 数无关：零 token 用量仍要扣费，结算入口据此跳过「0 token 视为出错」的清零
func TestFixedPriceTieredSettleWithZeroTokens(t *testing.T) {
	info := tieredRelayInfo(`param("quality") == "hd" ? tier("hd", fixed(0.08)) : tier("sd", fixed(0.04))`, 1)
	info.BillingRequestInput = &billingexpr.RequestInput{Body: []byte(`{"quality":"hd"}`)}
	ok, quota, result := TryTieredSettle(testGinContext(0, 0), info, TieredUsage{PromptIncludesCache: true})
	require.True(t, ok)
	assert.Equal(t, 40000, quota) // 0.08 * 500000
	assert.True(t, IsFixedPriceTieredSettlement(info, result))

	tokenInfo := tieredRelayInfo(`tier("base", p * 2)`, 1)
	_, _, tokenResult := TryTieredSettle(testGinContext(0, 0), tokenInfo, TieredUsage{PromptIncludesCache: true})
	assert.False(t, IsFixedPriceTieredSettlement(tokenInfo, tokenResult))

	// 求值失败时沿用预扣阶段的计价单位
	info.TieredBillingSnapshot.EstimatedBillingUnit = billingexpr.BillingUnitRequest
	assert.True(t, IsFixedPriceTieredSettlement(info, nil))
}
