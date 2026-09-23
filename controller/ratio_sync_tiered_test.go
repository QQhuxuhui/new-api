package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type syncChannel = struct {
	name string
	data map[string]any
}

func TestBuildDifferencesTieredUpstream(t *testing.T) {
	local := map[string]any{
		"model_ratio":      map[string]float64{"gpt-6-sol": 5, "gpt-4o": 1.25},
		"completion_ratio": map[string]float64{"gpt-6-sol": 5, "gpt-4o": 4},
	}
	expr := `len <= 272000 ? tier("standard", p * 12 + c * 60) : tier("long_context", p * 24 + c * 90)`
	upstream := []syncChannel{{
		name: "basellm",
		data: map[string]any{
			"billing_mode": map[string]any{"gpt-6-sol": "tiered_expr", "gpt-4o": "tiered_expr"},
			"billing_expr": map[string]any{"gpt-6-sol": expr, "gpt-4o": `tier("standard", p * 2.5 + c * 10)`},
		},
	}}

	diffs := buildDifferences(local, upstream)
	sol := diffs["gpt-6-sol"]
	if assert.NotNil(t, sol) {
		// 上游是阶梯计费时只对比计费模式与表达式，不再展示倍率差异
		assert.NotContains(t, sol, "model_ratio")
		assert.NotContains(t, sol, "completion_ratio")
		assert.Equal(t, "ratio", sol["billing_mode"].Current)
		assert.Equal(t, "tiered_expr", sol["billing_mode"].Upstreams["basellm"])
		assert.Nil(t, sol["billing_expr"].Current)
		assert.Equal(t, expr, sol["billing_expr"].Upstreams["basellm"])
	}
}

func TestBuildDifferencesSameExpressionHasNoDiff(t *testing.T) {
	expr := `tier("standard", p * 2 + c * 8)`
	local := map[string]any{
		"billing_mode": map[string]string{"m": "tiered_expr"},
		"billing_expr": map[string]string{"m": expr},
	}
	upstream := []syncChannel{{
		name: "peer",
		data: map[string]any{
			"model_ratio":  map[string]any{"m": 1.0},
			"billing_mode": map[string]any{"m": "tiered_expr"},
			"billing_expr": map[string]any{"m": expr},
		},
	}}
	assert.Empty(t, buildDifferences(local, upstream))
}

func TestBuildDifferencesRatioOnlyUnchanged(t *testing.T) {
	local := map[string]any{"model_ratio": map[string]float64{"m": 1}}
	upstream := []syncChannel{{name: "peer", data: map[string]any{"model_ratio": map[string]any{"m": 2.0}}}}
	diffs := buildDifferences(local, upstream)
	if assert.Contains(t, diffs, "m") {
		assert.Contains(t, diffs["m"], "model_ratio")
		assert.NotContains(t, diffs["m"], "billing_mode")
	}
}

// 只提供 billing_expr、没有 billing_mode 的上游预设按阶梯计费处理
func TestBuildDifferencesExprOnlyUpstream(t *testing.T) {
	expr := `tier("standard", p * 2 + c * 10)`
	local := map[string]any{"model_ratio": map[string]float64{"m": 1}}
	upstream := []syncChannel{{name: "preset", data: map[string]any{
		"billing_expr": map[string]any{"m": expr, "new-model": expr},
	}}}
	diffs := buildDifferences(local, upstream)
	for _, name := range []string{"m", "new-model"} {
		if assert.Contains(t, diffs, name) {
			assert.Equal(t, expr, diffs[name]["billing_expr"].Upstreams["preset"])
			assert.Equal(t, "tiered_expr", diffs[name]["billing_mode"].Upstreams["preset"])
		}
	}
}

// 显式声明 ratio 的上游，其表达式未生效，不参与对比
func TestBuildDifferencesExplicitRatioIgnoresExpr(t *testing.T) {
	local := map[string]any{"model_ratio": map[string]float64{"m": 1}}
	upstream := []syncChannel{{name: "peer", data: map[string]any{
		"model_ratio":  map[string]any{"m": 1.0},
		"billing_mode": map[string]any{"m": "ratio"},
		"billing_expr": map[string]any{"m": `tier("x", p * 9)`},
	}}}
	assert.Empty(t, buildDifferences(local, upstream))
}

// 本地阶梯计费、上游改回倍率计费时，计费模式与倍率差异都应展示
func TestBuildDifferencesTieredLocalRatioUpstream(t *testing.T) {
	local := map[string]any{
		"billing_mode": map[string]string{"m": "tiered_expr"},
		"billing_expr": map[string]string{"m": `tier("x", p * 2)`},
	}
	upstream := []syncChannel{{name: "peer", data: map[string]any{
		"model_ratio":      map[string]any{"m": 1.5},
		"completion_ratio": map[string]any{"m": 4.0},
	}}}
	diffs := buildDifferences(local, upstream)
	if assert.Contains(t, diffs, "m") {
		assert.Equal(t, "tiered_expr", diffs["m"]["billing_mode"].Current)
		assert.Equal(t, "ratio", diffs["m"]["billing_mode"].Upstreams["peer"])
		assert.Equal(t, 1.5, diffs["m"]["model_ratio"].Upstreams["peer"])
		assert.Equal(t, 4.0, diffs["m"]["completion_ratio"].Upstreams["peer"])
	}
}
