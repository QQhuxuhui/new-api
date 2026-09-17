package controller

import "testing"

func TestParseLinearBillingExpr(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want map[string]float64
		ok   bool
	}{
		{"standard", `tier("standard", p * 3 + cr * 0.3 + cc * 3.75 + cc1h * 6 + c * 15)`, map[string]float64{"p": 3, "cr": 0.3, "cc": 3.75, "cc1h": 6, "c": 15}, true},
		{"conditional first tier", `len <= 200000 ? tier("0_200k", p * 1.25 + cr * 0.125 + c * 10) : tier("200k_plus", p * 2.5 + cr * 0.25 + c * 15)`, map[string]float64{"p": 1.25, "cr": 0.125, "c": 10}, true},
		{"time of day first tier", `weekday("UTC") >= 1 && hour("UTC") < 4 ? tier("peak", p * 0.3 + c * 1.2) : tier("off_peak", p * 0.15 + c * 0.6)`, map[string]float64{"p": 0.3, "c": 1.2}, true},
		{"bare linear", `p * 2 + c * 8`, map[string]float64{"p": 2, "c": 8}, true},
		{"non linear", `tier("x", p * 3 * 2 + c)`, nil, false},
		{"conditional without tier", `len > 1 ? 3 : 4`, nil, false},
		{"empty", ``, nil, false},
	}
	for _, tc := range cases {
		got, ok := parseLinearBillingExpr(tc.expr)
		if ok != tc.ok {
			t.Fatalf("%s: ok=%v want %v", tc.name, ok, tc.ok)
		}
		if !ok {
			continue
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Fatalf("%s: %s=%v want %v", tc.name, k, got[k], v)
			}
		}
	}
}

func TestConvertBillingExprData(t *testing.T) {
	converted, skipped := convertBillingExprData(map[string]any{
		"claude-sonnet-4-6": `tier("standard", p * 3 + cr * 0.3 + cc * 3.75 + cc1h * 6 + c * 15)`,
		"gemini-2.5-pro":    `len <= 200000 ? tier("0_200k", p * 1.25 + cr * 0.125 + c * 10) : tier("200k_plus", p * 2.5 + cr * 0.25 + c * 15)`,
		"free-model":        `tier("standard", p * 0 + c * 0)`,
		"output-only":       `tier("standard", p * 0 + c * 5)`,
		"weird":             `p * x`,
		"not-string":        123,
	})

	mr := converted["model_ratio"].(map[string]any)
	cr := converted["completion_ratio"].(map[string]any)
	cc := converted["cache_ratio"].(map[string]any)

	if mr["claude-sonnet-4-6"] != 1.5 || cr["claude-sonnet-4-6"] != 5.0 || cc["claude-sonnet-4-6"] != 0.1 {
		t.Fatalf("sonnet: %v %v %v", mr["claude-sonnet-4-6"], cr["claude-sonnet-4-6"], cc["claude-sonnet-4-6"])
	}
	if mr["gemini-2.5-pro"] != 0.625 || cr["gemini-2.5-pro"] != 8.0 || cc["gemini-2.5-pro"] != 0.1 {
		t.Fatalf("gemini: %v %v %v", mr["gemini-2.5-pro"], cr["gemini-2.5-pro"], cc["gemini-2.5-pro"])
	}
	if mr["free-model"] != 0.0 {
		t.Fatalf("free-model ratio = %v", mr["free-model"])
	}
	if _, ok := cr["free-model"]; ok {
		t.Fatalf("free-model should not have completion_ratio")
	}
	if len(skipped) != 3 || skipped[0] != "not-string" || skipped[1] != "output-only" || skipped[2] != "weird" {
		t.Fatalf("skipped = %v", skipped)
	}
}

func TestConvertBillingExprDataEmpty(t *testing.T) {
	converted, skipped := convertBillingExprData(map[string]any{"a": "??"})
	if len(converted) != 0 || len(skipped) != 1 {
		t.Fatalf("converted=%v skipped=%v", converted, skipped)
	}
}
