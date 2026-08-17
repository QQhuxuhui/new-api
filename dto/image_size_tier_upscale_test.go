package dto

import "testing"

func capWith(allowed []string, up *ImageUpscaleRule) *ImageSizeCapability {
	return &ImageSizeCapability{Allowed: allowed, Upscale: up}
}

func TestImageUpscaleRuleValidate(t *testing.T) {
	cases := []struct {
		name    string
		cap     *ImageSizeCapability
		wantErr bool
	}{
		{"合法 1K→4K", capWith([]string{"1K"}, &ImageUpscaleRule{From: "1K", To: "4K"}), false},
		{"合法 2K→4K 且 allowed 含两档", capWith([]string{"1K", "2K"}, &ImageUpscaleRule{From: "2K", To: "4K"}), false},
		{"小写归一 from", capWith([]string{"1K"}, &ImageUpscaleRule{From: "1k", To: "2K"}), false},
		{"allowed 为空 + 规则", capWith(nil, &ImageUpscaleRule{From: "1K", To: "4K"}), true},
		{"from 不在 allowed", capWith([]string{"2K"}, &ImageUpscaleRule{From: "1K", To: "4K"}), true},
		{"to 非法档位", capWith([]string{"1K"}, &ImageUpscaleRule{From: "1K", To: "8K"}), true},
		{"to 不高于 max(allowed)", capWith([]string{"1K", "2K"}, &ImageUpscaleRule{From: "1K", To: "2K"}), true},
		{"to 等于 from", capWith([]string{"1K"}, &ImageUpscaleRule{From: "1K", To: "1K"}), true},
		{"无规则照旧合法", capWith([]string{"1K"}, nil), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cap.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestNormalizedUpscaleFailOpen(t *testing.T) {
	// SQL 直改绕过 Validate 的兜底：非法规则运行期视同不存在
	if capWith(nil, &ImageUpscaleRule{From: "1K", To: "4K"}).NormalizedUpscale() != nil {
		t.Fatal("allowed 为空时规则应视同不存在")
	}
	if capWith([]string{"1K"}, &ImageUpscaleRule{From: "2K", To: "4K"}).NormalizedUpscale() != nil {
		t.Fatal("from∉allowed 时规则应视同不存在")
	}
	if capWith([]string{"1K"}, &ImageUpscaleRule{From: "垃圾", To: "4K"}).NormalizedUpscale() != nil {
		t.Fatal("垃圾 from 应视同不存在")
	}
	got := capWith([]string{"1K"}, &ImageUpscaleRule{From: "1k", To: "4k"}).NormalizedUpscale()
	if got == nil || got.From != "1K" || got.To != "4K" {
		t.Fatalf("合法规则应归一化返回, got=%+v", got)
	}
	var nilCap *ImageSizeCapability
	if nilCap.NormalizedUpscale() != nil {
		t.Fatal("nil 接收者应返回 nil")
	}
}

func TestAllowWithUpscaleDerivation(t *testing.T) {
	rule := &ImageUpscaleRule{From: "1K", To: "4K"}
	c := capWith([]string{"1K"}, rule)
	cases := []struct {
		name     string
		tier     string
		eligible bool
		want     bool
	}{
		{"原生 1K 直通", "1K", true, true},
		{"派生 2K（宽松中间档）", "2K", true, true},
		{"派生 4K（规则目标）", "4K", true, true},
		{"不具资格时 4K 拒", "4K", false, false},
		{"不具资格时原生 1K 仍通", "1K", false, true},
		{"判不出档位 fail-open", "", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.AllowWithUpscale(tc.tier, tc.eligible); got != tc.want {
				t.Fatalf("AllowWithUpscale(%q,%v)=%v want %v", tc.tier, tc.eligible, got, tc.want)
			}
		})
	}
	// to=2K 时 4K 不可达
	c2 := capWith([]string{"1K"}, &ImageUpscaleRule{From: "1K", To: "2K"})
	if c2.AllowWithUpscale("4K", true) {
		t.Fatal("超出 To 的档位不应可达")
	}
	// 无规则 = 与 Allow 完全一致
	c3 := capWith([]string{"1K"}, nil)
	if c3.AllowWithUpscale("4K", true) {
		t.Fatal("无规则时不应派生")
	}
	// nil 接收者 fail-open
	var nilCap *ImageSizeCapability
	if !nilCap.AllowWithUpscale("4K", true) {
		t.Fatal("nil 接收者应放行（与 Allow 一致）")
	}
}

func TestUpscaleFromTier(t *testing.T) {
	c := capWith([]string{"1K"}, &ImageUpscaleRule{From: "1K", To: "4K"})
	if from, ok := c.UpscaleFromTier("4K", true); !ok || from != "1K" {
		t.Fatalf("4K 应触发超分 from=1K, got %q/%v", from, ok)
	}
	if from, ok := c.UpscaleFromTier("2K", true); !ok || from != "1K" {
		t.Fatalf("宽松中间档 2K 应触发超分, got %q/%v", from, ok)
	}
	if _, ok := c.UpscaleFromTier("1K", true); ok {
		t.Fatal("原生档位不应触发超分")
	}
	if _, ok := c.UpscaleFromTier("4K", false); ok {
		t.Fatal("不具资格不应触发超分")
	}
	if _, ok := c.UpscaleFromTier("", true); ok {
		t.Fatal("判不出档位不应触发超分")
	}
	c2 := capWith([]string{"1K", "2K"}, &ImageUpscaleRule{From: "2K", To: "4K"})
	if from, ok := c2.UpscaleFromTier("4K", true); !ok || from != "2K" {
		t.Fatalf("from 应取规则声明的 2K, got %q/%v", from, ok)
	}
}
