package billing_setting

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// 移植自上游 new-api 的阶梯（表达式）计费配置。
// dev 未移植任务插件（jsplugin），因此去掉了 plugin_billing_expr 与 u() 任务用量相关逻辑。
const (
	BillingModeRatio      = "ratio"
	BillingModeTieredExpr = "tiered_expr"
	BillingModeField      = "billing_mode"
	BillingExprField      = "billing_expr"
	BillingModeOptionKey  = "billing_setting.billing_mode"
	BillingExprOptionKey  = "billing_setting.billing_expr"
)

// BillingSetting is managed by config.GlobalConfig.Register.
// DB keys: billing_setting.billing_mode, billing_setting.billing_expr
type BillingSetting struct {
	BillingMode AtomicStringMap `json:"billing_mode"`
	BillingExpr AtomicStringMap `json:"billing_expr"`
}

var billingSetting = BillingSetting{
	BillingMode: NewAtomicStringMap(),
	BillingExpr: NewAtomicStringMap(),
}

func init() {
	config.GlobalConfig.Register("billing_setting", &billingSetting)
}

// ---------------------------------------------------------------------------
// Read accessors (hot path, must be fast)
// ---------------------------------------------------------------------------

func GetBillingMode(model string) string {
	mode, ok := billingSetting.BillingMode.Get(model)
	if ok {
		return mode
	}
	if _, ok := builtinBillingExpr[model]; ok {
		// Existing administrator-configured legacy prices take precedence over
		// a newly introduced built-in expression unless a mode was explicit.
		if ratio_setting.HasConfiguredModelRatio(model) {
			return BillingModeRatio
		}
		if _, configured := ratio_setting.GetModelPrice(model, false); configured {
			return BillingModeRatio
		}
		return BillingModeTieredExpr
	}
	return BillingModeRatio
}

func GetBillingExpr(model string) (string, bool) {
	expr, ok := billingSetting.BillingExpr.Get(model)
	if ok {
		return expr, true
	}
	if GetBillingMode(model) == BillingModeTieredExpr {
		expr, ok := builtinBillingExpr[model]
		return expr, ok
	}
	return "", false
}

// IsTieredBilling 判断模型当前是否走阶梯计费且表达式可用。
func IsTieredBilling(model string) bool {
	if GetBillingMode(model) != BillingModeTieredExpr {
		return false
	}
	expr, ok := GetBillingExpr(model)
	return ok && strings.TrimSpace(expr) != ""
}

func GetBuiltinBillingExpr(model string) (string, bool) {
	expression, ok := builtinBillingExpr[model]
	return expression, ok
}

func GetBillingModeCopy() map[string]string {
	configured := billingSetting.BillingMode.Load()
	modes := make(map[string]string, len(configured)+len(builtinBillingExpr))
	for k, v := range configured {
		modes[k] = v
	}
	for model := range builtinBillingExpr {
		if _, configured := modes[model]; !configured && GetBillingMode(model) == BillingModeTieredExpr {
			modes[model] = BillingModeTieredExpr
		}
	}
	return modes
}

func GetBillingExprCopy() map[string]string {
	configuredExprs := billingSetting.BillingExpr.Load()
	expressions := make(map[string]string, len(configuredExprs)+len(builtinBillingExpr))
	for k, v := range configuredExprs {
		expressions[k] = v
	}
	for model := range builtinBillingExpr {
		if _, configured := expressions[model]; configured {
			continue
		}
		if expression, ok := GetBillingExpr(model); ok {
			expressions[model] = expression
		}
	}
	return expressions
}

// GetPricingSyncData 在 /api/ratio_config 暴露的数据中追加阶梯计费字段，供下游站点同步。
func GetPricingSyncData(base map[string]any) map[string]any {
	out := make(map[string]any, len(base)+2)
	for k, v := range base {
		out[k] = v
	}
	if modes := GetBillingModeCopy(); len(modes) > 0 {
		out[BillingModeField] = modes
	}
	if exprs := GetBillingExprCopy(); len(exprs) > 0 {
		out[BillingExprField] = exprs
	}
	return out
}

// ---------------------------------------------------------------------------
// Smoke test (called externally for validation before save)
// ---------------------------------------------------------------------------

func SmokeTestExpr(exprStr string) error {
	if _, err := billingexpr.CompileFromCache(exprStr); err != nil {
		return err
	}
	// dev 未移植任务用量计费，结算时 u() 恒为 nil；动态键（如 u(header("x"))）也无法静态校验，
	// 一律拒绝，避免保存后按 0 计费
	if billingexpr.UsedVars(exprStr)["u"] {
		return fmt.Errorf("u() task usage variables are not supported")
	}

	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
		{P: 300, C: 100, Len: 1000, CR: 100, Img: 400, ImgCR: 200},
		{P: 800, C: 50, Len: 1000, AI: 200, AO: 50},
		{Len: math.MaxInt32, ImgCR: math.MaxInt32},
	}

	for _, v := range vectors {
		for _, request := range billingExprSmokeRequests() {
			result, _, err := billingexpr.RunExprWithRequest(exprStr, v, request)
			if err != nil {
				return fmt.Errorf("vector {p=%g, c=%g}: run failed: %w", v.P, v.C, err)
			}
			if math.IsNaN(result) || math.IsInf(result, 0) || result < 0 {
				return fmt.Errorf("vector {p=%g, c=%g}: result must be finite and non-negative, got %f", v.P, v.C, result)
			}
		}
	}
	return nil
}

// ValidateBillingExprMap 在保存前校验每个模型的表达式。
func ValidateBillingExprMap(expressions map[string]string) error {
	models := make([]string, 0, len(expressions))
	for name := range expressions {
		models = append(models, name)
	}
	sort.Strings(models)
	for _, name := range models {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("模型名不能为空")
		}
		if err := SmokeTestExpr(expressions[name]); err != nil {
			return fmt.Errorf("模型 %s 的计费表达式无效: %w", name, err)
		}
	}
	return nil
}

// ValidateBillingModeMap 在保存前校验计费模式取值。
func ValidateBillingModeMap(modes map[string]string) error {
	for name, mode := range modes {
		if mode != BillingModeRatio && mode != BillingModeTieredExpr {
			return fmt.Errorf("模型 %s 的计费模式 %q 无效，只能是 %s 或 %s", name, mode, BillingModeRatio, BillingModeTieredExpr)
		}
	}
	return nil
}

func billingExprSmokeRequests() []billingexpr.RequestInput {
	return []billingexpr.RequestInput{
		{},
		{
			Headers: map[string]string{
				"anthropic-beta": "fast-mode-2026-02-01",
			},
			Body: []byte(`{"service_tier":"fast","stream_options":{"include_usage":true},"messages":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]}`),
		},
	}
}
