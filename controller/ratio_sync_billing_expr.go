package controller

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 兼容上游 new-api 的 tiered_expr 计费表达式（如 basellm 官方倍率预设自 2026-09 起只输出 billing_expr）。
// dev 分支仍使用倍率计费，这里把线性表达式换算回 model_ratio / completion_ratio / cache_ratio：
//   - 表达式系数是 $/1M tokens 的真实价格，model_ratio = p / 2（new-api 约定 ratio 1 = $2/1M）
//   - completion_ratio = c / p，cache_ratio = cr / p
//   - 条件表达式（len/时段分档）只取第一个 tier(...)，即基础档价格
//   - 含 cc/cc1h/ai/ao 等 dev 不支持的子项直接忽略；解析失败的模型跳过
const (
	billingExprField      = "billing_expr"
	billingModeField      = "billing_mode"
	usdPerMillionPerRatio = 2.0
)

var (
	billingExprTierRe = regexp.MustCompile(`tier\(\s*"[^"]*"\s*,\s*([^()]*?)\s*\)`)
	billingExprTermRe = regexp.MustCompile(`^([a-z][a-z0-9_]*)\s*\*\s*([0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)$`)
)

// parseLinearBillingExpr 提取表达式（或其第一个 tier）中的线性系数，返回 变量名 -> $/1M。
func parseLinearBillingExpr(expr string) (map[string]float64, bool) {
	body := strings.TrimSpace(expr)
	if body == "" {
		return nil, false
	}
	if m := billingExprTierRe.FindStringSubmatch(body); m != nil {
		body = strings.TrimSpace(m[1])
	} else if strings.ContainsAny(body, "?():") {
		return nil, false
	}
	coef := make(map[string]float64)
	for _, term := range strings.Split(body, "+") {
		term = strings.TrimSpace(term)
		m := billingExprTermRe.FindStringSubmatch(term)
		if m == nil {
			return nil, false
		}
		v, err := strconv.ParseFloat(m[2], 64)
		if err != nil || v < 0 {
			return nil, false
		}
		coef[m[1]] += v
	}
	if len(coef) == 0 {
		return nil, false
	}
	return coef, true
}

func roundRatio(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

// convertBillingExprData 将 billing_expr 映射转换为 type1 结构；返回转换结果和被跳过的模型名（已排序）。
func convertBillingExprData(exprs map[string]any) (map[string]any, []string) {
	modelRatio := make(map[string]any)
	completionRatio := make(map[string]any)
	cacheRatio := make(map[string]any)
	var skipped []string

	for name, raw := range exprs {
		expr, _ := raw.(string)
		coef, ok := parseLinearBillingExpr(expr)
		if !ok {
			skipped = append(skipped, name)
			continue
		}
		p := coef["p"]
		c, hasC := coef["c"]
		if p <= 0 {
			// 免费模型：只有当输出也免费时才能用倍率表达
			if hasC && c > 0 {
				skipped = append(skipped, name)
				continue
			}
			modelRatio[name] = float64(0)
			continue
		}
		modelRatio[name] = roundRatio(p / usdPerMillionPerRatio)
		if hasC {
			completionRatio[name] = roundRatio(c / p)
		}
		if cr, ok := coef["cr"]; ok {
			cacheRatio[name] = roundRatio(cr / p)
		}
	}

	converted := make(map[string]any)
	if len(modelRatio) > 0 {
		converted["model_ratio"] = modelRatio
	}
	if len(completionRatio) > 0 {
		converted["completion_ratio"] = completionRatio
	}
	if len(cacheRatio) > 0 {
		converted["cache_ratio"] = cacheRatio
	}
	sort.Strings(skipped)
	return converted, skipped
}
