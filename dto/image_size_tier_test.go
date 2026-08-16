package dto

import (
	"encoding/json"
	"testing"
)

func TestClassifyImageBillingTier(t *testing.T) {
	cases := []struct {
		name     string
		size     string
		wantTier string
		wantOK   bool
	}{
		// 早期直接命中的显式写法
		{"explicit-1k", "1K", ImageSizeTier1K, true},
		{"explicit-2k-lowercase", "2k", ImageSizeTier2K, true},
		{"explicit-4k-padded", "  4K  ", ImageSizeTier4K, true},
		{"square-2048", "2048x2048", ImageSizeTier2K, true},
		{"legacy-2048x1152", "2048x1152", ImageSizeTier2K, true},
		{"landscape-3840x2160", "3840x2160", ImageSizeTier4K, true},
		{"portrait-2160x3840", "2160x3840", ImageSizeTier4K, true},

		// 实测表命中
		{"table-1024x1024", "1024x1024", ImageSizeTier1K, true},
		{"table-2560x1440-not-4k-by-long-edge", "2560x1440", ImageSizeTier2K, true},
		{"table-2304x1728-not-4k-by-long-edge", "2304x1728", ImageSizeTier2K, true},
		{"table-transposed-2448x3264", "2448x3264", ImageSizeTier4K, true},
		{"table-transposed-720x1280", "720x1280", ImageSizeTier1K, true},
		{"table-21x9-not-transposed-fallback", "1584x3696", ImageSizeTier4K, true},

		// 表外按面积兜底
		{"area-fallback-3504x1168", "3504x1168", ImageSizeTier2K, true},
		{"area-fallback-tiny", "512x512", ImageSizeTier1K, true},
		{"area-fallback-huge", "8000x8000", ImageSizeTier4K, true},
		{"area-fallback-exact-1k-boundary", "1024x1024", ImageSizeTier1K, true},
		{"uppercase-separator", "1024X1024", ImageSizeTier1K, true},

		// 判不出档位：必须 fail open
		{"empty", "", "", false},
		{"auto", "auto", "", false},
		{"auto-mixed-case", "AUTO", "", false},
		{"ratio-notation", "1:1", "", false},
		{"garbage", "abc", "", false},
		{"missing-height", "1024x", "", false},
		{"missing-width", "x1024", "", false},
		{"fullwidth-separator", "2048×2048", "", false},
		{"negative", "-1024x1024", "", false},
		{"zero", "0x0", "", false},
		{"three-parts", "1024x1024x1024", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, ok := ClassifyImageBillingTier(tc.size)
			if ok != tc.wantOK || tier != tc.wantTier {
				t.Fatalf("ClassifyImageBillingTier(%q) = (%q, %v), want (%q, %v)",
					tc.size, tier, ok, tc.wantTier, tc.wantOK)
			}
		})
	}
}

// 表里每一格自身必须能被分类回同一档位，否则表和分类逻辑已经脱节
func TestClassifyImageBillingTier_TableSelfConsistent(t *testing.T) {
	for ratio, tiers := range imageSizeTierRatioTable {
		for tier, size := range tiers {
			got, ok := ClassifyImageBillingTier(size)
			if !ok || got != tier {
				t.Fatalf("ratio %s tier %s size %s classified as (%q, %v)", ratio, tier, size, got, ok)
			}
		}
	}
}

func TestBuildImageSizeIndex_TransposesOnlyIntendedRatios(t *testing.T) {
	// 4:3 4K 的竖版必须在表内
	if _, ok := lookupImageSize(2448, 3264); !ok {
		t.Fatal("expected transposed 4:3 4K (2448x3264) in index")
	}
	// 21:9 不转置：竖版不应被登记为表内尺寸
	if _, ok := lookupImageSize(1584, 3696); ok {
		t.Fatal("21:9 should not be transposed into the index")
	}
	// 1:1 是正方形，索引里只应有一条
	if len(imageSizeIndex) != 30 {
		t.Fatalf("index size = %d, want 30 (18 base + 12 transposed)", len(imageSizeIndex))
	}
}

func TestImageSizeCapability_Allow(t *testing.T) {
	cases := []struct {
		name string
		cap  *ImageSizeCapability
		tier string
		want bool
	}{
		{"nil-capability-allows-all", nil, ImageSizeTier4K, true},
		{"empty-allowlist-allows-all", &ImageSizeCapability{}, ImageSizeTier4K, true},
		{"empty-slice-allows-all", &ImageSizeCapability{Allowed: []string{}}, ImageSizeTier4K, true},
		{"listed-tier-allowed", &ImageSizeCapability{Allowed: []string{"1K", "2K"}}, ImageSizeTier2K, true},
		{"unlisted-tier-rejected", &ImageSizeCapability{Allowed: []string{"1K", "2K"}}, ImageSizeTier4K, false},
		{"case-insensitive-config", &ImageSizeCapability{Allowed: []string{"4k"}}, ImageSizeTier4K, true},
		{"padded-config", &ImageSizeCapability{Allowed: []string{" 2K "}}, ImageSizeTier2K, true},
		{"unknown-tier-fails-open", &ImageSizeCapability{Allowed: []string{"1K"}}, "", true},
		{"unparsable-tier-fails-open", &ImageSizeCapability{Allowed: []string{"1K"}}, "8K", true},
		// 整个白名单都写废时与空数组同策：那种配置只可能是误操作，按字面全拒
		// 会让渠道对所有图片请求隐身，线上只留一行 incapable 计数。ValidateSettings
		// 只在走 API 的写入路径上把关，SQL/后台脚本改 setting 绕得过去。
		{"all-garbage-config-fails-open", &ImageSizeCapability{Allowed: []string{"bogus"}}, ImageSizeTier1K, true},
		{"all-garbage-multi-fails-open", &ImageSizeCapability{Allowed: []string{"bogus", "8K", ""}}, ImageSizeTier4K, true},
		{"garbage-plus-valid-entry", &ImageSizeCapability{Allowed: []string{"bogus", "1K"}}, ImageSizeTier1K, true},
		// 但只要有一项合法，白名单就照常收紧——不能因为混了垃圾项就整体失效
		{"garbage-plus-valid-still-restricts", &ImageSizeCapability{Allowed: []string{"bogus", "1K"}}, ImageSizeTier4K, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cap.Allow(tc.tier); got != tc.want {
				t.Fatalf("Allow(%q) = %v, want %v", tc.tier, got, tc.want)
			}
		})
	}
}

// 选路口径必须与上游 adobe2api core/models/resolver.py:199-215 resolution_from_size
// 逐字对应（纯 max(w,h)），否则过滤器预测不准上游会不会拒。
// 标了「分歧」的用例是它与计费口径故意不同的地方——那正是这个函数存在的理由。
func TestClassifyImageRoutingTier(t *testing.T) {
	cases := []struct {
		size     string
		wantTier string
		wantOK   bool
	}{
		{"", "", false},
		{"auto", "", false},
		{"AUTO", "", false},
		{"1:1", "", false},
		{"abc", "", false},
		{"1024x", "", false},
		{"0x0", "", false},
		{"-1024x1024", "", false},
		{"1k", ImageSizeTier1K, true},
		{"4K", ImageSizeTier4K, true},
		{"1024x1024", ImageSizeTier1K, true},
		{"512x768", ImageSizeTier1K, true},
		{"2048x2048", ImageSizeTier2K, true},
		{"1536x1024", ImageSizeTier2K, true},
		{"2048x1152", ImageSizeTier2K, true},
		{"3840x2160", ImageSizeTier4K, true},
		{"2160x3840", ImageSizeTier4K, true},
		// 分歧：计费按面积判 1K（1280*720 < 1024²），选路按长边判 2K
		{"1280x720", ImageSizeTier2K, true},
		{"1456x624", ImageSizeTier2K, true},
		// 分歧：计费命中实测表判 2K，选路按长边判 4K —— 上游正是按长边拒的
		{"2560x1440", ImageSizeTier4K, true},
		{"2304x1728", ImageSizeTier4K, true},
		{"2496x1664", ImageSizeTier4K, true},
		{"3024x1296", ImageSizeTier4K, true},
		{"2049x1024", ImageSizeTier4K, true},
		{"2048x4096", ImageSizeTier4K, true},
	}
	for _, tc := range cases {
		t.Run(tc.size, func(t *testing.T) {
			tier, ok := ClassifyImageRoutingTier(tc.size)
			if tier != tc.wantTier || ok != tc.wantOK {
				t.Fatalf("ClassifyImageRoutingTier(%q) = (%q, %v), want (%q, %v)", tc.size, tier, ok, tc.wantTier, tc.wantOK)
			}
		})
	}
}

// 计费口径不能被选路的改动带偏：它与 sub2api 逐行对齐，是两边防漂移的锚点。
func TestBillingTierUnchangedAtDivergencePoints(t *testing.T) {
	cases := map[string]string{
		"1280x720":  ImageSizeTier1K,
		"2560x1440": ImageSizeTier2K,
		"2304x1728": ImageSizeTier2K,
		"2048x2048": ImageSizeTier2K,
		"3840x2160": ImageSizeTier4K,
	}
	for size, want := range cases {
		t.Run(size, func(t *testing.T) {
			if tier, ok := ClassifyImageBillingTier(size); !ok || tier != want {
				t.Fatalf("ClassifyImageBillingTier(%q) = (%q, %v), want (%q, true)", size, tier, ok, want)
			}
		})
	}
}

func TestImageQualityForcesHighestTier(t *testing.T) {
	for _, q := range []string{"high", "HIGH", " 4k ", "ultra", "4K"} {
		if !ImageQualityForcesHighestTier(q) {
			t.Fatalf("quality %q should force 4K", q)
		}
	}
	for _, q := range []string{"", "auto", "medium", "low", "standard", "hd", "2k", "1k", "garbage"} {
		if ImageQualityForcesHighestTier(q) {
			t.Fatalf("quality %q must not force 4K", q)
		}
	}
}

func TestImageSizeCapability_Validate(t *testing.T) {
	if err := (*ImageSizeCapability)(nil).Validate(); err != nil {
		t.Fatalf("nil capability should validate, got %v", err)
	}
	if err := (&ImageSizeCapability{Allowed: []string{"1K", "2k", " 4K "}}).Validate(); err != nil {
		t.Fatalf("valid capability rejected: %v", err)
	}
	if err := (&ImageSizeCapability{Allowed: []string{"1K", "8K"}}).Validate(); err == nil {
		t.Fatal("expected validation error for unknown tier 8K")
	}
}

// 存量渠道的 setting 反序列化后再序列化，不能凭空多出 image_sizes
func TestChannelSettings_ImageSizesOmittedWhenAbsent(t *testing.T) {
	var settings ChannelSettings
	if err := json.Unmarshal([]byte(`{"proxy":"","always_healthy":true}`), &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if settings.ImageSizes != nil {
		t.Fatal("ImageSizes should stay nil when absent")
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("unmarshal round trip: %v", err)
	}
	if _, ok := roundTrip["image_sizes"]; ok {
		t.Fatalf("image_sizes must not be emitted for legacy settings, got %s", encoded)
	}
}

func TestChannelSettings_ImageSizesRoundTrip(t *testing.T) {
	var settings ChannelSettings
	if err := json.Unmarshal([]byte(`{"image_sizes":{"allowed":["1K","2K"]}}`), &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if settings.ImageSizes == nil {
		t.Fatal("ImageSizes should be parsed")
	}
	if settings.ImageSizes.Allow(ImageSizeTier4K) {
		t.Fatal("4K should be rejected by the parsed allowlist")
	}
	if !settings.ImageSizes.Allow(ImageSizeTier1K) {
		t.Fatal("1K should be allowed by the parsed allowlist")
	}
}
