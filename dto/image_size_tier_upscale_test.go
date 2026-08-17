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
