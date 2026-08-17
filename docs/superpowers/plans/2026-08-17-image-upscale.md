# new-api 渠道级图片超分 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 渠道配置 `upscale: {from, to}` 后，new-api 自动把高档位图片请求降档发上游、回程调 RunPod Serverless（Real-ESRGAN）放大到目标尺寸再返回。

**Architecture:** dto 层扩展能力声明与宽松派生；distributor 注入超分资格；选路过滤用派生可达集；`ImageHelper` 出站降档 + `DoResponse` 前偷换响应体；独立 RunPod worker 经 presigned URL 走对象存储中转，worker 零凭据。

**Tech Stack:** Go（gjson/sjson、aws-sdk-go-v2 s3）、Python（runpod SDK、Real-ESRGAN x4plus）、RunPod Serverless、Cloudflare R2 / 阿里云 OSS（S3 兼容）。

**Spec:** `docs/superpowers/specs/2026-08-17-image-upscale-design.md`

## Global Constraints

- 仓库：`/usr/src/workspace/github/QQhuxuhui/new-api`（Go 部分）；worker 新仓库 `/usr/src/workspace/github/QQhuxuhui/runpod-upscale-worker`。
- **工作区已有他人未提交改动**（veo 等）：每次 commit 只 `git add` 本任务明确列出的文件，**禁止 `git add -A` / `git add .`**。
- `allowed` 语义保持纯原生，绝不把派生档位写进 `allowed`。
- 一切不确定 fail-open：规则解析失败/非法 → 视同无规则；超分失败 → 返回上游原图，绝不吞掉已付费的生成。
- 超分资格谓词与 sub2api `openAIImagesRequestSimulatable` / `isSimulatableOpenAIImagesModel` 逐字对齐（模型白名单：`gpt-image-2`、`gpt-image-2-2026-04-21`），注释必须双向指认。
- 端点范围仅 `/v1/images/generations`、`/v1/images/edits`（含 `/v1/edits` 别名）；stream / n>1 / mask / 非 png / 非 b64_json 一律不超分。
- 中间档宽松派生：规则 `from→to` 使 `(max(allowed), to]` 全部可达。
- 档位序：1K < 2K < 4K（`tierRank`：1K=1, 2K=2, 4K=3）。
- Go 测试统一 `cd /usr/src/workspace/github/QQhuxuhui/new-api && go test ./<pkg>/ -run <Test> -v`。
- 环境变量前缀 `IMAGE_UPSCALE_`，任一必需项缺失即模块自禁用（不 panic、不阻启动）。

---

### Task 1: dto — ImageUpscaleRule 结构与校验

**Files:**
- Modify: `dto/image_size_tier.go`（`ImageSizeCapability` 定义处，约 267 行）
- Test: `dto/image_size_tier_upscale_test.go`（新建）

**Interfaces:**
- Consumes: 现有 `NormalizeImageSizeTier`、`ImageSizeCapability.NormalizedAllowed()`、`AllImageSizeTiers()`
- Produces:
  - `type ImageUpscaleRule struct { From string; To string }`（json tag `from`/`to`）
  - `ImageSizeCapability.Upscale *ImageUpscaleRule`（json tag `upscale,omitempty`）
  - `func imageSizeTierRank(tier string) int`（1K=1/2K=2/4K=3，非法=0）
  - `func (c *ImageSizeCapability) NormalizedUpscale() *ImageUpscaleRule`（运行期 fail-open 访问器：规则缺失/非法/allowed 为空 → nil；合法 → 归一化 From/To）
  - `Validate()` 扩展（写入路径硬校验）

- [ ] **Step 1: 写失败测试**

```go
// dto/image_size_tier_upscale_test.go
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./dto/ -run 'TestImageUpscaleRuleValidate|TestNormalizedUpscaleFailOpen' -v`
Expected: 编译失败 `undefined: ImageUpscaleRule`

- [ ] **Step 3: 实现**

在 `dto/image_size_tier.go` 的 `ImageSizeCapability` 定义前后添加：

```go
// ImageUpscaleRule 声明渠道的超分能力：把 From 档原生出图放大到 To 档返回。
// 每渠道至多一条（结构上是单对象非数组）。选路据此派生可达档位（宽松：
// (max(allowed), To] 全部可达），出站请求降档为 From，回程在 relay 层超分。
type ImageUpscaleRule struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// imageSizeTierRank 给档位排序：1K=1 < 2K=2 < 4K=3，非法=0。
func imageSizeTierRank(tier string) int {
	switch tier {
	case ImageSizeTier1K:
		return 1
	case ImageSizeTier2K:
		return 2
	case ImageSizeTier4K:
		return 3
	default:
		return 0
	}
}

// maxAllowedTierRank 返回白名单里最高档位的 rank；无有效项返回 0。
func (c *ImageSizeCapability) maxAllowedTierRank() int {
	maxRank := 0
	for _, tier := range c.NormalizedAllowed() {
		if r := imageSizeTierRank(tier); r > maxRank {
			maxRank = r
		}
	}
	return maxRank
}

// NormalizedUpscale 返回归一化后的合法超分规则；任何不合法（allowed 为空、
// from∉allowed、to 非法或不高于 max(allowed)）一律返回 nil —— 与 Allow() 的
// anyValid 兜底同款：SQL/后台脚本绕过 Validate 直改 setting 时，写废的规则
// 必须静默退化为"无规则"，绝不让渠道行为进入未定义状态。
func (c *ImageSizeCapability) NormalizedUpscale() *ImageUpscaleRule {
	if c == nil || c.Upscale == nil {
		return nil
	}
	from, okF := NormalizeImageSizeTier(c.Upscale.From)
	to, okT := NormalizeImageSizeTier(c.Upscale.To)
	if !okF || !okT {
		return nil
	}
	maxRank := c.maxAllowedTierRank()
	if maxRank == 0 || imageSizeTierRank(to) <= maxRank {
		return nil
	}
	fromInAllowed := false
	for _, tier := range c.NormalizedAllowed() {
		if tier == from {
			fromInAllowed = true
			break
		}
	}
	if !fromInAllowed {
		return nil
	}
	return &ImageUpscaleRule{From: from, To: to}
}
```

`ImageSizeCapability` 结构体加字段（`Allowed` 之后）：

```go
	// Upscale 声明超分规则，见 ImageUpscaleRule。nil = 无超分能力。
	Upscale *ImageUpscaleRule `json:"upscale,omitempty"`
```

`Validate()` 末尾（现有 allowed 循环之后）追加：

```go
	if c.Upscale != nil {
		from, okF := NormalizeImageSizeTier(c.Upscale.From)
		if !okF {
			return fmt.Errorf("image_sizes.upscale.from 含非法档位 %q，仅支持 %s",
				c.Upscale.From, strings.Join(AllImageSizeTiers(), "/"))
		}
		to, okT := NormalizeImageSizeTier(c.Upscale.To)
		if !okT {
			return fmt.Errorf("image_sizes.upscale.to 含非法档位 %q，仅支持 %s",
				c.Upscale.To, strings.Join(AllImageSizeTiers(), "/"))
		}
		maxRank := c.maxAllowedTierRank()
		if maxRank == 0 {
			return fmt.Errorf("image_sizes.upscale 需要 allowed 声明原生档位：空白名单是全放行语义，叠加超分规则自相矛盾")
		}
		if imageSizeTierRank(to) <= maxRank {
			return fmt.Errorf("image_sizes.upscale.to=%s 必须严格高于 allowed 最高档，否则规则无意义", to)
		}
		fromInAllowed := false
		for _, tier := range c.NormalizedAllowed() {
			if tier == from {
				fromInAllowed = true
			}
		}
		if !fromInAllowed {
			return fmt.Errorf("image_sizes.upscale.from=%s 必须是 allowed 中的原生档位", from)
		}
	}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./dto/ -run 'TestImageUpscaleRuleValidate|TestNormalizedUpscaleFailOpen' -v` 以及回归 `go test ./dto/ -run TestImageSize -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add dto/image_size_tier.go dto/image_size_tier_upscale_test.go
git commit -m "feat(dto): image_sizes 增加 upscale 规则结构、校验与 fail-open 访问器"
```

---

### Task 2: dto — 宽松派生 AllowWithUpscale / UpscaleFromTier

**Files:**
- Modify: `dto/image_size_tier.go`
- Test: `dto/image_size_tier_upscale_test.go`

**Interfaces:**
- Consumes: Task 1 的 `NormalizedUpscale()`、`imageSizeTierRank`、现有 `Allow(tier)`
- Produces:
  - `func (c *ImageSizeCapability) AllowWithUpscale(tier string, upscaleEligible bool) bool` — 选路用
  - `func (c *ImageSizeCapability) UpscaleFromTier(tier string, upscaleEligible bool) (string, bool)` — relay 用：本请求需要超分时返回降档目标 From

- [ ] **Step 1: 写失败测试**

追加到 `dto/image_size_tier_upscale_test.go`：

```go
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./dto/ -run 'TestAllowWithUpscale|TestUpscaleFromTier' -v`
Expected: 编译失败 `undefined: AllowWithUpscale`

- [ ] **Step 3: 实现**

```go
// AllowWithUpscale 是选路口径的可达性判定：原生 Allow 之外，若本请求具备
// 超分资格（upscaleEligible，见 middleware 谓词）且渠道有合法超分规则，则
// (max(allowed), To] 区间的档位全部派生可达（宽松语义：能超到 4K 必然能超到
// 2K，同一张卡输出更小）。不具资格/无规则时与 Allow 完全一致。
func (c *ImageSizeCapability) AllowWithUpscale(tier string, upscaleEligible bool) bool {
	if c.Allow(tier) {
		return true
	}
	if !upscaleEligible {
		return false
	}
	rule := c.NormalizedUpscale()
	if rule == nil {
		return false
	}
	tierRank := imageSizeTierRank(tier)
	// Allow 已 false ⇒ tier 是合法档位且不在白名单（非法档位 Allow 会放行）
	return tierRank > c.maxAllowedTierRank() && tierRank <= imageSizeTierRank(rule.To)
}

// UpscaleFromTier 判定本请求是否走超分模式：tier 非原生但派生可达时返回
// (规则的 From 档, true)，relay 层据此降档出站并在回程放大。
func (c *ImageSizeCapability) UpscaleFromTier(tier string, upscaleEligible bool) (string, bool) {
	if c == nil || !upscaleEligible || c.Allow(tier) {
		return "", false
	}
	if !c.AllowWithUpscale(tier, upscaleEligible) {
		return "", false
	}
	return c.NormalizedUpscale().From, true
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./dto/ -v`
Expected: 全部 PASS（含存量测试回归）

- [ ] **Step 5: Commit**

```bash
git add dto/image_size_tier.go dto/image_size_tier_upscale_test.go
git commit -m "feat(dto): AllowWithUpscale 宽松派生与 UpscaleFromTier 超分判定"
```

---

### Task 3: dto — 尺寸映射 MapImageSizeForUpscale

**Files:**
- Modify: `dto/image_size_tier.go`
- Test: `dto/image_size_tier_upscale_test.go`

**Interfaces:**
- Consumes: `imageSizeTierRatioTable`、`parseImageSizeDimensions`、`NormalizeImageSizeTier`
- Produces: `func MapImageSizeForUpscale(requestSize string, fromTier string) (downgradedSize string, targetW int, targetH int, ok bool)`
  - `requestSize`：用户原始 size（"3840x2160" 或 "4K" 字面档位）
  - 返回：出站降档尺寸（如 "1280x720"）、超分目标精确宽高、是否可映射

- [ ] **Step 1: 写失败测试**

```go
func TestMapImageSizeForUpscale(t *testing.T) {
	cases := []struct {
		name        string
		requestSize string
		fromTier    string
		wantDown    string
		wantW       int
		wantH       int
		wantOK      bool
	}{
		{"16:9 4K → 1K", "3840x2160", "1K", "1280x720", 3840, 2160, true},
		{"16:9 竖版（转置）", "2160x3840", "1K", "720x1280", 2160, 3840, true},
		{"1:1 表内 4K", "2880x2880", "1K", "1024x1024", 2880, 2880, true},
		{"字面 4K（无比例→1:1）", "4K", "1K", "1024x1024", 2880, 2880, true},
		{"字面 2K", "2K", "1K", "1024x1024", 2048, 2048, true},
		{"3:2 2K → 1K", "2496x1664", "1K", "1248x832", 2496, 1664, true},
		{"表外比例就近（≈2:1 归 21:9）", "3600x1800", "1K", "1456x624", 3600, 1800, true},
		{"21:9 竖版回退 16:9 竖版", "1584x3696", "1K", "720x1280", 1584, 3696, true},
		{"from=2K 降档", "2880x2880", "2K", "2048x2048", 2880, 2880, true},
		{"解析不了", "auto", "1K", "", 0, 0, false},
		{"from 非法", "3840x2160", "8K", "", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			down, w, h, ok := MapImageSizeForUpscale(tc.requestSize, tc.fromTier)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if down != tc.wantDown || w != tc.wantW || h != tc.wantH {
				t.Fatalf("got (%s,%d,%d) want (%s,%d,%d)", down, w, h, tc.wantDown, tc.wantW, tc.wantH)
			}
		})
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./dto/ -run TestMapImageSizeForUpscale -v`
Expected: 编译失败 `undefined: MapImageSizeForUpscale`

- [ ] **Step 3: 实现**

```go
// upscaleRatioAspects 是比例表各行的横版宽高比数值（宽/高），用于就近匹配。
var upscaleRatioAspects = []struct {
	Ratio  string
	Aspect float64
}{
	{"1:1", 1.0}, {"5:4", 1.25}, {"4:3", 4.0 / 3.0},
	{"3:2", 1.5}, {"16:9", 16.0 / 9.0}, {"21:9", 21.0 / 9.0},
}

// MapImageSizeForUpscale 把用户请求的 size 翻译成【出站降档尺寸 + 超分目标宽高】。
//
//   - "WxH" 写法：目标 = 精确 WxH（超分能命中任意尺寸，比原生表更准）；
//     比例就近匹配表内横版行，竖版（H>W）取转置。21:9 上游无竖版
//     （见 buildImageSizeIndex 注释），竖版就近回退 16:9 转置。
//   - "4K"/"2K" 字面档位：无比例信息，按 1:1 处理，目标 = 表内 [1:1][档位]。
//   - 解析不了（auto/垃圾）或 fromTier 非法：ok=false，调用方放弃超分。
func MapImageSizeForUpscale(requestSize string, fromTier string) (string, int, int, bool) {
	from, okFrom := NormalizeImageSizeTier(fromTier)
	if !okFrom {
		return "", 0, 0, false
	}
	trimmed := strings.TrimSpace(requestSize)
	if tier, ok := NormalizeImageSizeTier(trimmed); ok {
		target := imageSizeTierRatioTable["1:1"][tier]
		tw, th, _ := parseImageSizeDimensions(target)
		down := imageSizeTierRatioTable["1:1"][from]
		return down, tw, th, true
	}
	width, height, ok := parseImageSizeDimensions(trimmed)
	if !ok {
		return "", 0, 0, false
	}
	portrait := height > width
	longer, shorter := float64(width), float64(height)
	if portrait {
		longer, shorter = float64(height), float64(width)
	}
	aspect := longer / shorter
	best, bestDiff := "1:1", math.MaxFloat64
	for _, cand := range upscaleRatioAspects {
		if diff := math.Abs(math.Log(aspect) - math.Log(cand.Aspect)); diff < bestDiff {
			best, bestDiff = cand.Ratio, diff
		}
	}
	if portrait && best == "21:9" {
		best = "16:9" // 21:9 无竖版原生尺寸，就近回退
	}
	dw, dh, _ := parseImageSizeDimensions(imageSizeTierRatioTable[best][from])
	if portrait {
		dw, dh = dh, dw
	}
	return fmt.Sprintf("%dx%d", dw, dh), width, height, true
}
```

`import` 增加 `"math"`（该文件现有 import 无 math）。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./dto/ -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add dto/image_size_tier.go dto/image_size_tier_upscale_test.go
git commit -m "feat(dto): MapImageSizeForUpscale 出站降档与超分目标尺寸映射"
```

---

### Task 4: middleware — 超分资格谓词与 context 注入

**Files:**
- Modify: `constant/context_key.go`（`ContextKeyImageHighQuality` 之后，约 75 行）
- Modify: `middleware/distributor.go`（`ModelRequest` 结构约 27-38 行；tier 注入块约 1091-1104 行）
- Test: `middleware/image_upscale_eligible_test.go`（新建）

**Interfaces:**
- Consumes: 现有 `imageStringValueFromRawJSON(raw, allowRepeatedFormValues)`、`common.SetContextKey`
- Produces:
  - `constant.ContextKeyImageUpscaleEligible ContextKey = "image_upscale_eligible"`
  - `ModelRequest` 新增字段：`N, ResponseFormat, Stream, Background, OutputFormat, OutputCompression, PartialImages, InputFidelity`（全部 `json.RawMessage`，沿用 Size/Quality 的"抄两行零额外读取"模式）
  - `func imageUpscaleEligible(c *gin.Context, m *ModelRequest, allowRepeatedFormValues bool) bool`

- [ ] **Step 1: 写失败测试**

```go
// middleware/image_upscale_eligible_test.go
package middleware

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func eligCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestImageUpscaleEligible(t *testing.T) {
	base := func() *ModelRequest { return &ModelRequest{Model: "gpt-image-2"} }
	cases := []struct {
		name string
		mut  func(m *ModelRequest)
		want bool
	}{
		{"最小合法请求", func(m *ModelRequest) {}, true},
		{"显式 n=1", func(m *ModelRequest) { m.N = raw(`1`) }, true},
		{"n=2 拒", func(m *ModelRequest) { m.N = raw(`2`) }, false},
		{"stream=true 拒", func(m *ModelRequest) { m.Stream = raw(`true`) }, false},
		{"stream=false 通", func(m *ModelRequest) { m.Stream = raw(`false`) }, true},
		{"partial_images 拒", func(m *ModelRequest) { m.PartialImages = raw(`2`) }, false},
		{"background=opaque 通", func(m *ModelRequest) { m.Background = raw(`"opaque"`) }, true},
		{"background=transparent 拒", func(m *ModelRequest) { m.Background = raw(`"transparent"`) }, false},
		{"output_format=png 通", func(m *ModelRequest) { m.OutputFormat = raw(`"png"`) }, true},
		{"output_format=webp 拒", func(m *ModelRequest) { m.OutputFormat = raw(`"webp"`) }, false},
		{"response_format=b64_json 通", func(m *ModelRequest) { m.ResponseFormat = raw(`"b64_json"`) }, true},
		{"response_format=url 拒", func(m *ModelRequest) { m.ResponseFormat = raw(`"url"`) }, false},
		{"output_compression 拒", func(m *ModelRequest) { m.OutputCompression = raw(`80`) }, false},
		{"input_fidelity 拒", func(m *ModelRequest) { m.InputFidelity = raw(`"high"`) }, false},
		{"模型不在白名单拒", func(m *ModelRequest) { m.Model = "dall-e-3" }, false},
		{"带日期版本模型通", func(m *ModelRequest) { m.Model = "gpt-image-2-2026-04-21" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base()
			tc.mut(m)
			if got := imageUpscaleEligible(eligCtx(), m, false); got != tc.want {
				t.Fatalf("eligible=%v want %v", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./middleware/ -run TestImageUpscaleEligible -v`
Expected: 编译失败（字段/函数未定义）

- [ ] **Step 3: 实现**

`constant/context_key.go` 在 `ContextKeyImageHighQuality` 行后加：

```go
	ContextKeyImageUpscaleEligible ContextKey = "image_upscale_eligible" // bool: 本请求形状具备超分资格（与 sub2api openAIImagesRequestSimulatable 对齐），选路可用派生档位、relay 可降档超分
```

`middleware/distributor.go` `ModelRequest` 结构在 `Quality` 字段后加（沿用现有 RawMessage + omitempty 风格）：

```go
	// 以下字段仅超分资格谓词使用（imageUpscaleEligible）：body 已解析过一遍，
	// 抄几行等于零额外读取。RawMessage 兼容 JSON 与表单两种来源。
	N                 json.RawMessage `json:"n,omitempty"`
	ResponseFormat    json.RawMessage `json:"response_format,omitempty"`
	Stream            json.RawMessage `json:"stream,omitempty"`
	Background        json.RawMessage `json:"background,omitempty"`
	OutputFormat      json.RawMessage `json:"output_format,omitempty"`
	OutputCompression json.RawMessage `json:"output_compression,omitempty"`
	PartialImages     json.RawMessage `json:"partial_images,omitempty"`
	InputFidelity     json.RawMessage `json:"input_fidelity,omitempty"`
```

新增谓词（放 `imageSizeForTierClassification` 附近）：

```go
// imageUpscaleEligible 判定本图片请求是否具备超分资格。
//
// 口径与 sub2api 的按实际像素计费门（openai_images_usage_simulation.go 的
// openAIImagesRequestSimulatable + isSimulatableOpenAIImagesModel）逐字对齐：
// 只有那条路径会解码实际像素计费，形状不满足时 sub2api 透传上游 usage（降档
// 后的量）——此时若仍超分，客户拿高清图按低档扣费，漏账。因此两侧必须同步：
// sub2api 放宽模拟资格可以随后放宽这里（方向安全）；反向收紧必须先收紧这里。
func imageUpscaleEligible(c *gin.Context, m *ModelRequest, allowRepeatedFormValues bool) bool {
	switch strings.ToLower(strings.TrimSpace(m.Model)) {
	case "gpt-image-2", "gpt-image-2-2026-04-21":
	default:
		return false
	}
	if len(m.PartialImages) > 0 || len(m.OutputCompression) > 0 || len(m.InputFidelity) > 0 {
		return false
	}
	if n := imageStringValueFromRawJSON(m.N, allowRepeatedFormValues); n != "" && n != "1" {
		return false
	}
	if s := strings.ToLower(imageStringValueFromRawJSON(m.Stream, allowRepeatedFormValues)); s != "" && s != "false" {
		return false
	}
	if b := strings.ToLower(imageStringValueFromRawJSON(m.Background, allowRepeatedFormValues)); b != "" && b != "opaque" {
		return false
	}
	if f := strings.ToLower(imageStringValueFromRawJSON(m.OutputFormat, allowRepeatedFormValues)); f != "" && f != "png" {
		return false
	}
	if f := strings.ToLower(imageStringValueFromRawJSON(m.ResponseFormat, allowRepeatedFormValues)); f != "" && f != "b64_json" {
		return false
	}
	// edits 带 mask 时 sub2api 不可模拟（HasMask）。multipart 表单里 mask 是文件字段。
	if c.Request != nil && strings.Contains(c.ContentType(), "multipart") {
		if _, _, err := c.Request.FormFile("mask"); err == nil {
			return false
		}
	}
	return true
}
```

（注：`imageStringValueFromRawJSON` 是现有函数，distributor.go:1101 已用于 Quality；如签名不同以现场为准适配。）

distributor 的 tier 注入块（`if tier, ok := dto.ClassifyImageRoutingTier(...)` 之后、quality 判断之前）加：

```go
		if imageUpscaleEligible(c, modelRequest, allowRepeatedFormValues) {
			common.SetContextKey(c, constant.ContextKeyImageUpscaleEligible, true)
		}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./middleware/ -run TestImageUpscaleEligible -v && go build ./...`
Expected: PASS，全仓可编译

- [ ] **Step 5: Commit**

```bash
git add constant/context_key.go middleware/distributor.go middleware/image_upscale_eligible_test.go
git commit -m "feat(middleware): 超分资格谓词（与 sub2api 模拟资格对齐）并注入 context"
```

---

### Task 5: model+service — 选路过滤接入派生可达集

**Files:**
- Modify: `model/channel_capability.go`（`ChannelSelectFilter` 结构 + `channelSatisfiesFilter` 约 96-110 行 + DB 模式 `filterChannelIdsByFilter` 中的同款判定）
- Modify: `service/channel_select.go`（约 22-31 行，filter 构造）
- Test: `model/channel_capability_upscale_test.go`（新建）

**Interfaces:**
- Consumes: Task 2 `AllowWithUpscale`、Task 4 `constant.ContextKeyImageUpscaleEligible`
- Produces: `ChannelSelectFilter.UpscaleEligible bool` 字段；`markImageSizeViaUpscale()` 观测方法（沿用 `markImageSizeRejected` 的计数模式，字段名 `imageSizeViaUpscale int`）

- [ ] **Step 1: 写失败测试**

```go
// model/channel_capability_upscale_test.go
package model

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func upscaleChannel(t *testing.T, setting string) *Channel {
	t.Helper()
	ch := &Channel{}
	ch.Setting = &setting
	return ch
}

func TestChannelSatisfiesFilterWithUpscale(t *testing.T) {
	setting := `{"image_sizes":{"allowed":["1K"],"upscale":{"from":"1K","to":"4K"}}}`
	ch := upscaleChannel(t, setting)

	f := &ChannelSelectFilter{ImageSizeTier: "4K", UpscaleEligible: true}
	if !channelSatisfiesFilter(ch, f) {
		t.Fatal("eligible 时 4K 应派生可达")
	}
	if f.imageSizeViaUpscale == 0 {
		t.Fatal("经派生通过应打 viaUpscale 计数")
	}

	f2 := &ChannelSelectFilter{ImageSizeTier: "4K", UpscaleEligible: false}
	if channelSatisfiesFilter(ch, f2) {
		t.Fatal("不具资格时 4K 应被拒")
	}

	f3 := &ChannelSelectFilter{ImageSizeTier: "1K", UpscaleEligible: false}
	if !channelSatisfiesFilter(ch, f3) {
		t.Fatal("原生 1K 不受资格影响")
	}
	if f3.imageSizeViaUpscale != 0 {
		t.Fatal("原生通过不应打 viaUpscale 计数")
	}
}
```

（`Channel.Setting`/`GetSettingReadonly` 的具体装配以现场结构为准；若 `Setting` 非 `*string` 字段或需其他初始化，适配测试帮助函数，断言不变。）

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./model/ -run TestChannelSatisfiesFilterWithUpscale -v`
Expected: 编译失败 `unknown field UpscaleEligible`

- [ ] **Step 3: 实现**

`ChannelSelectFilter` 加字段与观测（模式照抄现有 `markImageSizeRejected`）：

```go
	UpscaleEligible bool // 本请求具备超分资格（context 注入），派生可达集仅在 true 时生效

	imageSizeViaUpscale int // 观测：经超分派生（而非原生白名单）通过的次数
```

```go
func (f *ChannelSelectFilter) markImageSizeViaUpscale() {
	if f != nil {
		f.imageSizeViaUpscale++
	}
}
```

`channelSatisfiesFilter` 中 ImageSizeTier 判定改为：

```go
	if filter.ImageSizeTier != "" {
		if !setting.ImageSizes.AllowWithUpscale(filter.ImageSizeTier, filter.UpscaleEligible) {
			filter.markImageSizeRejected()
			satisfied = false
		} else if !setting.ImageSizes.Allow(filter.ImageSizeTier) {
			// 原生不通、派生通 ⇒ 这条渠道的该档位是超分出来的，运营侧要看得见
			filter.markImageSizeViaUpscale()
		}
	}
```

DB 模式 `filterChannelIdsByFilter`（同文件下方）里对 `ImageSizes.Allow(` 的调用点做同款替换（`grep -n "ImageSizes.Allow" model/channel_capability.go` 找齐所有调用点，一个不漏）。

`service/channel_select.go` 构造处：

```go
	tier := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier)
	highQuality := common.GetContextKeyBool(c, constant.ContextKeyImageHighQuality)
	upscaleEligible := common.GetContextKeyBool(c, constant.ContextKeyImageUpscaleEligible)
	if tier == "" && !highQuality {
		return nil
	}
	return &model.ChannelSelectFilter{ImageSizeTier: tier, ImageHighQuality: highQuality, UpscaleEligible: upscaleEligible}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./model/ -run TestChannelSatisfiesFilter -v && go build ./...`
Expected: PASS（含存量 filter 测试）

- [ ] **Step 5: Commit**

```bash
git add model/channel_capability.go model/channel_capability_upscale_test.go service/channel_select.go
git commit -m "feat(model): 选路过滤接入超分派生可达集与 via-upscale 观测"
```

---

### Task 6: service — 配置加载与 S3 兼容存储客户端

**Files:**
- Create: `service/image_upscale_config.go`
- Create: `service/image_upscale_storage.go`
- Test: `service/image_upscale_config_test.go`
- Modify: `go.mod`（`go get github.com/aws/aws-sdk-go-v2/service/s3 github.com/aws/aws-sdk-go-v2/config`）

**Interfaces:**
- Produces:
  - `type ImageUpscaleConfig struct { Endpoint, APIKey string; Timeout time.Duration; S3Endpoint, S3Region, S3Bucket, S3AccessKey, S3SecretKey string }`
  - `func LoadImageUpscaleConfig() *ImageUpscaleConfig`（读环境变量；任一必需项缺失或 `IMAGE_UPSCALE_ENABLED != "true"` → 返回 nil；`sync.Once` 缓存）
  - `type upscaleObjectStore interface { PutObject(ctx context.Context, key string, data []byte, contentType string) error; GetObject(ctx context.Context, key string) ([]byte, error); PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error); PresignPut(ctx context.Context, key string, contentType string, ttl time.Duration) (string, error) }`
  - `func newS3UpscaleStore(cfg *ImageUpscaleConfig) (upscaleObjectStore, error)`

- [ ] **Step 1: 拉依赖**

```bash
cd /usr/src/workspace/github/QQhuxuhui/new-api
go get github.com/aws/aws-sdk-go-v2/service/s3@latest github.com/aws/aws-sdk-go-v2/config@latest
```

- [ ] **Step 2: 写失败测试（配置加载）**

```go
// service/image_upscale_config_test.go
package service

import (
	"testing"
	"time"
)

func setAllUpscaleEnv(t *testing.T) {
	t.Helper()
	t.Setenv("IMAGE_UPSCALE_ENABLED", "true")
	t.Setenv("IMAGE_UPSCALE_RUNPOD_ENDPOINT", "https://api.runpod.ai/v2/test123")
	t.Setenv("IMAGE_UPSCALE_RUNPOD_API_KEY", "rpa_test")
	t.Setenv("IMAGE_UPSCALE_S3_ENDPOINT", "https://acc.r2.cloudflarestorage.com")
	t.Setenv("IMAGE_UPSCALE_S3_BUCKET", "upscale")
	t.Setenv("IMAGE_UPSCALE_S3_AK", "ak")
	t.Setenv("IMAGE_UPSCALE_S3_SK", "sk")
}

func TestLoadImageUpscaleConfig(t *testing.T) {
	setAllUpscaleEnv(t)
	cfg := loadImageUpscaleConfigFromEnv()
	if cfg == nil {
		t.Fatal("全量配置应加载成功")
	}
	if cfg.Timeout != 90*time.Second {
		t.Fatalf("默认超时应 90s, got %v", cfg.Timeout)
	}
	if cfg.S3Region != "auto" {
		t.Fatalf("默认 region 应 auto, got %v", cfg.S3Region)
	}

	t.Setenv("IMAGE_UPSCALE_TIMEOUT", "120s")
	if got := loadImageUpscaleConfigFromEnv(); got.Timeout != 120*time.Second {
		t.Fatalf("超时应可覆盖, got %v", got.Timeout)
	}

	t.Setenv("IMAGE_UPSCALE_ENABLED", "false")
	if loadImageUpscaleConfigFromEnv() != nil {
		t.Fatal("ENABLED=false 应禁用")
	}

	setAllUpscaleEnv(t)
	t.Setenv("IMAGE_UPSCALE_RUNPOD_API_KEY", "")
	if loadImageUpscaleConfigFromEnv() != nil {
		t.Fatal("必需项缺失应禁用而非 panic")
	}
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./service/ -run TestLoadImageUpscaleConfig -v`
Expected: 编译失败

- [ ] **Step 4: 实现配置与存储**

```go
// service/image_upscale_config.go
package service

import (
	"os"
	"strings"
	"sync"
	"time"
)

// ImageUpscaleConfig 超分模块配置，全部来自环境变量（前缀 IMAGE_UPSCALE_）。
type ImageUpscaleConfig struct {
	Endpoint    string        // RunPod endpoint 根地址，如 https://api.runpod.ai/v2/{id}
	APIKey      string        // RunPod API key
	Timeout     time.Duration // 单次超分总预算（含存储往返与轮询），默认 90s
	S3Endpoint  string        // S3 兼容 endpoint（R2 / 阿里云 OSS）
	S3Region    string        // 默认 auto（R2 约定；OSS 填对应 region）
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
}

// loadImageUpscaleConfigFromEnv 每次真实读 env（测试用）；生产入口是
// LoadImageUpscaleConfig 的 sync.Once 缓存。任一必需项缺失 → nil（模块自禁用，
// 不 panic 不阻启动——超分是增强能力，配置残缺时渠道退回纯原生行为）。
func loadImageUpscaleConfigFromEnv() *ImageUpscaleConfig {
	if strings.ToLower(os.Getenv("IMAGE_UPSCALE_ENABLED")) != "true" {
		return nil
	}
	cfg := &ImageUpscaleConfig{
		Endpoint:    strings.TrimRight(os.Getenv("IMAGE_UPSCALE_RUNPOD_ENDPOINT"), "/"),
		APIKey:      os.Getenv("IMAGE_UPSCALE_RUNPOD_API_KEY"),
		Timeout:     90 * time.Second,
		S3Endpoint:  os.Getenv("IMAGE_UPSCALE_S3_ENDPOINT"),
		S3Region:    os.Getenv("IMAGE_UPSCALE_S3_REGION"),
		S3Bucket:    os.Getenv("IMAGE_UPSCALE_S3_BUCKET"),
		S3AccessKey: os.Getenv("IMAGE_UPSCALE_S3_AK"),
		S3SecretKey: os.Getenv("IMAGE_UPSCALE_S3_SK"),
	}
	if cfg.S3Region == "" {
		cfg.S3Region = "auto"
	}
	if raw := os.Getenv("IMAGE_UPSCALE_TIMEOUT"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			cfg.Timeout = d
		}
	}
	if cfg.Endpoint == "" || cfg.APIKey == "" || cfg.S3Endpoint == "" ||
		cfg.S3Bucket == "" || cfg.S3AccessKey == "" || cfg.S3SecretKey == "" {
		return nil
	}
	return cfg
}

var (
	imageUpscaleConfigOnce sync.Once
	imageUpscaleConfig     *ImageUpscaleConfig
)

// LoadImageUpscaleConfig 生产入口；nil = 模块禁用。
func LoadImageUpscaleConfig() *ImageUpscaleConfig {
	imageUpscaleConfigOnce.Do(func() {
		imageUpscaleConfig = loadImageUpscaleConfigFromEnv()
	})
	return imageUpscaleConfig
}
```

```go
// service/image_upscale_storage.go
package service

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// upscaleObjectStore 抽掉具体存储：生产是 S3 兼容实现（R2/OSS 同一套），
// 测试用内存 mock。worker 侧零凭据——它只拿到 new-api 预签好的 GET/PUT URL。
type upscaleObjectStore interface {
	PutObject(ctx context.Context, key string, data []byte, contentType string) error
	GetObject(ctx context.Context, key string) ([]byte, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignPut(ctx context.Context, key string, contentType string, ttl time.Duration) (string, error)
}

type s3UpscaleStore struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func newS3UpscaleStore(cfg *ImageUpscaleConfig) (upscaleObjectStore, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.S3Endpoint)
		o.UsePathStyle = true // R2 与 OSS 的 S3 兼容层都接受 path-style，统一之
	})
	return &s3UpscaleStore{client: client, presign: s3.NewPresignClient(client), bucket: cfg.S3Bucket}, nil
}

func (s *s3UpscaleStore) PutObject(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
		Body: bytes.NewReader(data), ContentType: aws.String(contentType),
	})
	return err
}

func (s *s3UpscaleStore) GetObject(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (s *s3UpscaleStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx,
		&s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)},
		s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (s *s3UpscaleStore) PresignPut(ctx context.Context, key string, contentType string, ttl time.Duration) (string, error) {
	req, err := s.presign.PresignPutObject(ctx,
		&s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), ContentType: aws.String(contentType)},
		s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
```

- [ ] **Step 5: 跑测试确认通过 + Commit**

Run: `go test ./service/ -run TestLoadImageUpscaleConfig -v && go build ./...`
Expected: PASS

```bash
git add service/image_upscale_config.go service/image_upscale_storage.go service/image_upscale_config_test.go go.mod go.sum
git commit -m "feat(service): 超分配置加载与 S3 兼容存储客户端（R2/OSS 同一套）"
```

---

### Task 7: service — RunPod 客户端与 UpscaleImage 编排

**Files:**
- Create: `service/image_upscale.go`
- Test: `service/image_upscale_test.go`

**Interfaces:**
- Consumes: Task 6 `ImageUpscaleConfig`、`upscaleObjectStore`、`newS3UpscaleStore`
- Produces:
  - `type ImageUpscaler struct`（含 `store upscaleObjectStore`、`cfg *ImageUpscaleConfig`、`http *http.Client`、`keyFn func() string`）
  - `func GetImageUpscaler() *ImageUpscaler`（sync.Once 单例；配置禁用或存储初始化失败 → nil）
  - `func (u *ImageUpscaler) UpscaleImage(ctx context.Context, png []byte, targetW, targetH int) ([]byte, error)`

- [ ] **Step 1: 写失败测试**

用 `httptest` 起假 RunPod（/run 返回 job id，/status 第二次轮询返回 COMPLETED），存储用内存 mock；断言：src 被 PUT、worker 收到的 input 含 presigned URL 与目标尺寸、结果从 store 读回、输出尺寸校验失败报错。

```go
// service/image_upscale_test.go
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore { return &memStore{data: map[string][]byte{}} }

func (m *memStore) PutObject(_ context.Context, key string, data []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = data
	return nil
}
func (m *memStore) GetObject(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.data[key]
	if !ok {
		return nil, fmt.Errorf("no such key %s", key)
	}
	return d, nil
}
func (m *memStore) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://fake/presigned-get/" + key, nil
}
func (m *memStore) PresignPut(_ context.Context, key string, _ string, _ time.Duration) (string, error) {
	return "https://fake/presigned-put/" + key, nil
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUpscaleImageHappyPath(t *testing.T) {
	store := newMemStore()
	var gotInput map[string]any
	polls := 0
	rp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/run":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotInput, _ = req["input"].(map[string]any)
			// 模拟 worker：把结果 PNG 写进 out key
			_ = store.PutObject(r.Context(), gotInput["out_key"].(string), pngBytes(t, 128, 128), "image/png")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job-1", "status": "IN_QUEUE"})
		case r.URL.Path == "/status/job-1":
			polls++
			status := "IN_PROGRESS"
			if polls >= 2 {
				status = "COMPLETED"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job-1", "status": status})
		default:
			http.NotFound(w, r)
		}
	}))
	defer rp.Close()

	u := &ImageUpscaler{
		cfg:   &ImageUpscaleConfig{Endpoint: rp.URL, APIKey: "k", Timeout: 10 * time.Second},
		store: store,
		http:  rp.Client(),
		keyFn: func() string { return "upscale/test/req1" },
		pollInterval: 10 * time.Millisecond,
	}
	out, err := u.UpscaleImage(context.Background(), pngBytes(t, 32, 32), 128, 128)
	if err != nil {
		t.Fatalf("UpscaleImage: %v", err)
	}
	cfgImg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfgImg.Width != 128 || cfgImg.Height != 128 {
		t.Fatalf("输出应为 128x128 PNG, got %dx%d err=%v", cfgImg.Width, cfgImg.Height, err)
	}
	if gotInput["src_url"] == "" || gotInput["put_url"] == "" ||
		gotInput["target_w"].(float64) != 128 || gotInput["target_h"].(float64) != 128 {
		t.Fatalf("worker input 不完整: %+v", gotInput)
	}
	if _, err := store.GetObject(context.Background(), "upscale/test/req1/src.png"); err != nil {
		t.Fatal("源图应已上传到存储")
	}
}

func TestUpscaleImageDimensionMismatch(t *testing.T) {
	store := newMemStore()
	rp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/run" {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			in := req["input"].(map[string]any)
			_ = store.PutObject(r.Context(), in["out_key"].(string), pngBytes(t, 64, 64), "image/png") // 尺寸不对
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "j", "status": "COMPLETED"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "j", "status": "COMPLETED"})
	}))
	defer rp.Close()
	u := &ImageUpscaler{
		cfg: &ImageUpscaleConfig{Endpoint: rp.URL, APIKey: "k", Timeout: 5 * time.Second},
		store: store, http: rp.Client(),
		keyFn: func() string { return "upscale/test/req2" }, pollInterval: 10 * time.Millisecond,
	}
	if _, err := u.UpscaleImage(context.Background(), pngBytes(t, 32, 32), 128, 128); err == nil {
		t.Fatal("输出尺寸不符必须报错（由调用方降级）")
	}
}

func TestUpscaleImageRunpodFailed(t *testing.T) {
	rp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "j", "status": "FAILED", "error": "boom"})
	}))
	defer rp.Close()
	u := &ImageUpscaler{
		cfg: &ImageUpscaleConfig{Endpoint: rp.URL, APIKey: "k", Timeout: 5 * time.Second},
		store: newMemStore(), http: rp.Client(),
		keyFn: func() string { return "upscale/test/req3" }, pollInterval: 10 * time.Millisecond,
	}
	if _, err := u.UpscaleImage(context.Background(), pngBytes(t, 32, 32), 128, 128); err == nil {
		t.Fatal("FAILED 状态必须报错")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./service/ -run TestUpscaleImage -v`
Expected: 编译失败

- [ ] **Step 3: 实现**

```go
// service/image_upscale.go
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const upscalePresignTTL = 15 * time.Minute

// ImageUpscaler 编排一次超分：源图 → 对象存储 → RunPod Serverless（worker 经
// presigned URL 读写，零凭据）→ 取回结果并校验尺寸。所有失败向上抛错，由
// relay 层降级为返回原图——绝不在这里吞错。
type ImageUpscaler struct {
	cfg          *ImageUpscaleConfig
	store        upscaleObjectStore
	http         *http.Client
	keyFn        func() string // 对象 key 前缀（默认 upscale/{date}/{uuid}；测试注入固定值）
	pollInterval time.Duration
}

var (
	imageUpscalerOnce sync.Once
	imageUpscaler     *ImageUpscaler
)

// GetImageUpscaler 单例；nil = 模块禁用（配置缺失或存储初始化失败）。
func GetImageUpscaler() *ImageUpscaler {
	imageUpscalerOnce.Do(func() {
		cfg := LoadImageUpscaleConfig()
		if cfg == nil {
			return
		}
		store, err := newS3UpscaleStore(cfg)
		if err != nil {
			// 启动期打日志即可：超分是增强能力，存储配置错退回纯原生行为
			fmt.Printf("image_upscale: storage init failed, module disabled: %v\n", err)
			return
		}
		imageUpscaler = &ImageUpscaler{
			cfg: cfg, store: store,
			http:         &http.Client{Timeout: 30 * time.Second},
			keyFn:        func() string { return fmt.Sprintf("upscale/%s/%s", time.Now().UTC().Format("20060102"), uuid.NewString()) },
			pollInterval: time.Second,
		}
	})
	return imageUpscaler
}

// Timeout 暴露给 relay 层做 context.WithTimeout。
func (u *ImageUpscaler) Timeout() time.Duration { return u.cfg.Timeout }

func (u *ImageUpscaler) UpscaleImage(ctx context.Context, pngData []byte, targetW, targetH int) ([]byte, error) {
	prefix := u.keyFn()
	srcKey, outKey := prefix+"/src.png", prefix+"/out.png"

	if err := u.store.PutObject(ctx, srcKey, pngData, "image/png"); err != nil {
		return nil, fmt.Errorf("put src: %w", err)
	}
	srcURL, err := u.store.PresignGet(ctx, srcKey, upscalePresignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign src: %w", err)
	}
	putURL, err := u.store.PresignPut(ctx, outKey, "image/png", upscalePresignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign out: %w", err)
	}

	input := map[string]any{
		"src_url": srcURL, "put_url": putURL, "out_key": outKey,
		"target_w": targetW, "target_h": targetH,
	}
	status, jobID, err := u.runpodSubmit(ctx, input)
	if err != nil {
		return nil, err
	}
	for status != "COMPLETED" {
		switch status {
		case "FAILED", "CANCELLED", "TIMED_OUT":
			return nil, fmt.Errorf("runpod job %s: status=%s", jobID, status)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(u.pollInterval):
		}
		if status, err = u.runpodStatus(ctx, jobID); err != nil {
			return nil, err
		}
	}

	out, err := u.store.GetObject(ctx, outKey)
	if err != nil {
		return nil, fmt.Errorf("get out: %w", err)
	}
	cfgImg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("decode out: %w", err)
	}
	if cfgImg.Width != targetW || cfgImg.Height != targetH {
		return nil, fmt.Errorf("out size %dx%d != target %dx%d", cfgImg.Width, cfgImg.Height, targetW, targetH)
	}
	return out, nil
}

func (u *ImageUpscaler) runpodSubmit(ctx context.Context, input map[string]any) (status, jobID string, err error) {
	body, _ := json.Marshal(map[string]any{"input": input})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.cfg.Endpoint+"/run", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+u.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := u.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("runpod submit: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("runpod submit: HTTP %d: %s", resp.StatusCode, truncateForLog(raw))
	}
	return gjson.GetBytes(raw, "status").String(), gjson.GetBytes(raw, "id").String(), nil
}

func (u *ImageUpscaler) runpodStatus(ctx context.Context, jobID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.Endpoint+"/status/"+jobID, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+u.cfg.APIKey)
	resp, err := u.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("runpod status: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("runpod status: HTTP %d: %s", resp.StatusCode, truncateForLog(raw))
	}
	return gjson.GetBytes(raw, "status").String(), nil
}

func truncateForLog(b []byte) string {
	if len(b) > 200 {
		b = b[:200]
	}
	return string(b)
}
```

（`ImageUpscaler` 结构体字段测试里直接构造，字段需包内可见——测试同包，成立。`uuid` 已是 new-api 依赖，若 import 路径不同以 go.mod 为准。）

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./service/ -run TestUpscaleImage -v && go build ./...`
Expected: 3 个测试 PASS

- [ ] **Step 5: Commit**

```bash
git add service/image_upscale.go service/image_upscale_test.go
git commit -m "feat(service): ImageUpscaler——RunPod 提交/轮询/结果校验编排"
```

---

### Task 8: service — 响应改写 RewriteImageResponseWithUpscale

**Files:**
- Create: `service/image_upscale_rewrite.go`
- Test: `service/image_upscale_rewrite_test.go`

**Interfaces:**
- Consumes: Task 7 的 `ImageUpscaler`（经接口注入便于测试）
- Produces:
  - `type imageUpscaleFunc func(ctx context.Context, png []byte, targetW, targetH int) ([]byte, error)`
  - `func RewriteImageResponseWithUpscale(ctx context.Context, body []byte, targetW, targetH int, up imageUpscaleFunc) ([]byte, error)` — 成功返回改写后 body；任何失败返回 `(nil, err)`，调用方降级用原 body

- [ ] **Step 1: 写失败测试**

```go
// service/image_upscale_rewrite_test.go
package service

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/tidwall/gjson"
)

func fakeUp(out []byte, err error) imageUpscaleFunc {
	return func(_ context.Context, _ []byte, _, _ int) ([]byte, error) { return out, err }
}

func TestRewriteImageResponseWithUpscale(t *testing.T) {
	src := pngBytes(t, 32, 32)
	big := pngBytes(t, 128, 128)
	body := []byte(`{"created":1,"size":"32x32","data":[{"b64_json":"` +
		base64.StdEncoding.EncodeToString(src) + `","size":"32x32"}],"usage":{"total_tokens":10}}`)

	out, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128, fakeUp(big, nil))
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	gotB64 := gjson.GetBytes(out, "data.0.b64_json").String()
	decoded, _ := base64.StdEncoding.DecodeString(gotB64)
	if len(decoded) != len(big) {
		t.Fatal("b64_json 应替换为放大后的图")
	}
	if gjson.GetBytes(out, "size").String() != "128x128" {
		t.Fatal("根级 size 声明必须改写（sub2api 兜底路径读它）")
	}
	if gjson.GetBytes(out, "data.0.size").String() != "128x128" {
		t.Fatal("data 项级 size 声明必须改写")
	}
	if gjson.GetBytes(out, "usage.total_tokens").Int() != 10 {
		t.Fatal("usage 必须原样保留")
	}
}

func TestRewriteNoSizeFieldsStaysAbsent(t *testing.T) {
	src := pngBytes(t, 32, 32)
	body := []byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(src) + `"}]}`)
	out, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128, fakeUp(pngBytes(t, 128, 128), nil))
	if err != nil {
		t.Fatal(err)
	}
	if gjson.GetBytes(out, "size").Exists() {
		t.Fatal("原本没有的 size 字段不应凭空出现")
	}
}

func TestRewriteFailurePropagates(t *testing.T) {
	src := pngBytes(t, 32, 32)
	body := []byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(src) + `"}]}`)
	if _, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128, fakeUp(nil, errors.New("gpu down"))); err == nil {
		t.Fatal("超分失败必须报错（由调用方降级）")
	}
	if _, err := RewriteImageResponseWithUpscale(context.Background(), []byte(`{"data":[]}`), 128, 128, fakeUp(nil, nil)); err == nil {
		t.Fatal("空 data 必须报错")
	}
	if _, err := RewriteImageResponseWithUpscale(context.Background(), []byte(`{"data":[{"url":"http://x"}]}`), 128, 128, fakeUp(nil, nil)); err == nil {
		t.Fatal("无 b64_json（url 响应）必须报错——资格谓词已排除该形状，走到这说明上游违约")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./service/ -run TestRewrite -v`
Expected: 编译失败

- [ ] **Step 3: 实现**

```go
// service/image_upscale_rewrite.go
package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type imageUpscaleFunc func(ctx context.Context, png []byte, targetW, targetH int) ([]byte, error)

// RewriteImageResponseWithUpscale 把上游生图响应里的图放大并改写声明尺寸。
// 只处理 data[0]（资格谓词已保证 n=1）。改写声明 size 是硬要求：sub2api 的
// 非模拟兜底路径按响应【声明的】size 判计费档位（image_output_accounting.go
// addDataArray），不改写会把 4K 图判成 1K 档。原本不存在的字段不凭空创建。
// 任何失败返回 error——调用方（ImageHelper）降级为返回原 body，绝不吞掉
// 已付费的上游生成。
func RewriteImageResponseWithUpscale(ctx context.Context, body []byte, targetW, targetH int, up imageUpscaleFunc) ([]byte, error) {
	items := gjson.GetBytes(body, "data")
	if !items.IsArray() || len(items.Array()) == 0 {
		return nil, errors.New("image response has no data items")
	}
	b64 := gjson.GetBytes(body, "data.0.b64_json").String()
	if b64 == "" {
		return nil, errors.New("image response item has no b64_json")
	}
	src, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode b64_json: %w", err)
	}
	out, err := up(ctx, src, targetW, targetH)
	if err != nil {
		return nil, fmt.Errorf("upscale: %w", err)
	}
	sizeStr := fmt.Sprintf("%dx%d", targetW, targetH)
	newBody, err := sjson.SetBytes(body, "data.0.b64_json", base64.StdEncoding.EncodeToString(out))
	if err != nil {
		return nil, err
	}
	if gjson.GetBytes(newBody, "size").Exists() {
		if newBody, err = sjson.SetBytes(newBody, "size", sizeStr); err != nil {
			return nil, err
		}
	}
	if gjson.GetBytes(newBody, "data.0.size").Exists() {
		if newBody, err = sjson.SetBytes(newBody, "data.0.size", sizeStr); err != nil {
			return nil, err
		}
	}
	if gjson.GetBytes(newBody, "output_format").Exists() {
		if newBody, err = sjson.SetBytes(newBody, "output_format", "png"); err != nil {
			return nil, err
		}
	}
	return newBody, nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./service/ -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add service/image_upscale_rewrite.go service/image_upscale_rewrite_test.go
git commit -m "feat(service): 生图响应超分改写（b64 替换 + 声明 size 修正）"
```

---

### Task 9: relay — ImageHelper 接线（降档出站 + 回程超分 + 降级）

**Files:**
- Modify: `relay/image_handler.go`
- Create: `relay/image_upscale_plan.go`
- Test: `relay/image_upscale_plan_test.go`

**Interfaces:**
- Consumes: `dto.MapImageSizeForUpscale`、`ImageSizeCapability.UpscaleFromTier`、`constant.ContextKeyImageSizeTier` / `ContextKeyImageUpscaleEligible`、`service.GetImageUpscaler()`、`service.RewriteImageResponseWithUpscale`
- Produces: `type imageUpscalePlan struct { DowngradedSize string; TargetW, TargetH int; FromTier string }`；`func resolveImageUpscalePlan(c *gin.Context, info *relaycommon.RelayInfo, requestSize string) *imageUpscalePlan`

- [ ] **Step 1: 写失败测试（plan 解析）**

```go
// relay/image_upscale_plan_test.go
package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func planCtx(tier string, eligible bool) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	if tier != "" {
		common.SetContextKey(c, constant.ContextKeyImageSizeTier, tier)
	}
	if eligible {
		common.SetContextKey(c, constant.ContextKeyImageUpscaleEligible, true)
	}
	return c
}

func infoWithUpscale() *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{}
	info.ChannelSetting.ImageSizes = &dto.ImageSizeCapability{
		Allowed: []string{"1K"},
		Upscale: &dto.ImageUpscaleRule{From: "1K", To: "4K"},
	}
	return info
}

func TestResolveImageUpscalePlan(t *testing.T) {
	plan := resolveImageUpscalePlan(planCtx("4K", true), infoWithUpscale(), "3840x2160")
	if plan == nil {
		t.Fatal("4K+eligible+规则 应产出 plan")
	}
	if plan.DowngradedSize != "1280x720" || plan.TargetW != 3840 || plan.TargetH != 2160 || plan.FromTier != "1K" {
		t.Fatalf("plan 错误: %+v", plan)
	}
	if resolveImageUpscalePlan(planCtx("1K", true), infoWithUpscale(), "1024x1024") != nil {
		t.Fatal("原生档位不应产出 plan")
	}
	if resolveImageUpscalePlan(planCtx("4K", false), infoWithUpscale(), "3840x2160") != nil {
		t.Fatal("不具资格不应产出 plan")
	}
	if resolveImageUpscalePlan(planCtx("", true), infoWithUpscale(), "auto") != nil {
		t.Fatal("无档位不应产出 plan")
	}
	noRule := &relaycommon.RelayInfo{}
	noRule.ChannelSetting.ImageSizes = &dto.ImageSizeCapability{Allowed: []string{"1K"}}
	if resolveImageUpscalePlan(planCtx("4K", true), noRule, "3840x2160") != nil {
		t.Fatal("无规则渠道不应产出 plan")
	}
}
```

（`RelayInfo.ChannelSetting` 的字段路径以现场为准：`image_handler.go:49` 已用 `info.ChannelSetting.PassThroughBodyEnabled`，说明它是 `dto.ChannelSettings` 值/指针；测试装配按实际类型微调，断言不变。）

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./relay/ -run TestResolveImageUpscalePlan -v`
Expected: 编译失败

- [ ] **Step 3: 实现 plan 解析**

```go
// relay/image_upscale_plan.go
package relay

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// imageUpscalePlan 描述本次请求的超分执行计划：出站降什么档、回程放大到多大。
type imageUpscalePlan struct {
	DowngradedSize string
	TargetW        int
	TargetH        int
	FromTier       string
}

// resolveImageUpscalePlan 汇合三方信息决定是否走超分模式：
// distributor 注入的档位与资格 × 渠道 setting 的超分规则 × 尺寸可映射性。
// 任何一环不满足返回 nil（正常直通，零行为变化）。
func resolveImageUpscalePlan(c *gin.Context, info *relaycommon.RelayInfo, requestSize string) *imageUpscalePlan {
	tier := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier)
	eligible := common.GetContextKeyBool(c, constant.ContextKeyImageUpscaleEligible)
	if tier == "" || !eligible {
		return nil
	}
	caps := info.ChannelSetting.ImageSizes
	from, ok := caps.UpscaleFromTier(tier, eligible)
	if !ok {
		return nil
	}
	down, w, h, ok := dto.MapImageSizeForUpscale(requestSize, from)
	if !ok {
		return nil
	}
	return &imageUpscalePlan{DowngradedSize: down, TargetW: w, TargetH: h, FromTier: from}
}
```

- [ ] **Step 4: 接线 ImageHelper**

`relay/image_handler.go` 修改（对照现有行号）：

a. `helper.ModelMappedHelper` 成功后（39 行后）插：

```go
	var upscalePlan *imageUpscalePlan
	if service.GetImageUpscaler() != nil {
		upscalePlan = resolveImageUpscalePlan(c, info, request.Size)
	}
	if upscalePlan != nil {
		request.Size = upscalePlan.DowngradedSize
	}
```

b. passthrough 分支（49-54 行）改：

```go
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if upscalePlan != nil {
			if strings.Contains(c.ContentType(), "multipart") {
				// multipart 透传无法安全改写表单 size：放弃超分，纯原生直通。
				// （该组合 = edits + passthrough 渠道 + 超分规则，运营上应避免。）
				logger.LogWarn(c, "image_upscale: multipart passthrough, skip upscale")
				upscalePlan = nil
			} else if rewritten, err := sjson.SetBytes(body, "size", upscalePlan.DowngradedSize); err == nil {
				body = rewritten
			} else {
				logger.LogWarn(c, fmt.Sprintf("image_upscale: passthrough size rewrite failed, skip upscale: %v", err))
				upscalePlan = nil
			}
		}
		requestBody = bytes.NewBuffer(body)
	} else {
```

c. `adaptor.DoResponse`（108 行）之前插回程拦截：

```go
	if upscalePlan != nil && httpResp != nil && !info.IsStream {
		upstreamBody, readErr := io.ReadAll(httpResp.Body)
		_ = httpResp.Body.Close()
		if readErr != nil {
			return types.NewOpenAIError(readErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		}
		upscaler := service.GetImageUpscaler()
		upscaleCtx, cancel := context.WithTimeout(c.Request.Context(), upscaler.Timeout())
		newBody, upErr := service.RewriteImageResponseWithUpscale(
			upscaleCtx, upstreamBody, upscalePlan.TargetW, upscalePlan.TargetH, upscaler.UpscaleImage)
		cancel()
		if upErr != nil {
			// 降级：返回上游原图（降档尺寸）。sub2api 按实际像素计费 ⇒ 自动按低档收，
			// 不会多收；绝不因超分失败吞掉一次已付费的生成。
			logger.LogWarn(c, fmt.Sprintf("image_upscale_degraded: %v", upErr))
			newBody = upstreamBody
		} else {
			logger.LogInfo(c, fmt.Sprintf("image_upscale_done: %s→%dx%d",
				upscalePlan.FromTier, upscalePlan.TargetW, upscalePlan.TargetH))
		}
		httpResp.Body = io.NopCloser(bytes.NewReader(newBody))
		httpResp.ContentLength = int64(len(newBody))
		httpResp.Header.Del("Content-Length")
	}
```

d. logContent（129-131 行）改用**用户原始尺寸**并标注超分：

```go
	if len(imageReq.Size) > 0 {
		logContent = fmt.Sprintf("大小 %s, 品质 %s, 张数 %d", imageReq.Size, quality, request.N)
		if upscalePlan != nil {
			logContent += fmt.Sprintf("（%s 超分）", upscalePlan.FromTier)
		}
	}
```

e. import 增补：`context`、`github.com/tidwall/sjson`（`io`/`bytes`/`fmt`/`strings` 已有）。

- [ ] **Step 5: 编译 + 测试 + Commit**

Run: `go build ./... && go test ./relay/ ./service/ ./dto/ ./middleware/ ./model/ -count=1`
Expected: 全绿

```bash
git add relay/image_handler.go relay/image_upscale_plan.go relay/image_upscale_plan_test.go
git commit -m "feat(relay): ImageHelper 接入降档出站与回程超分（失败降级返回原图）"
```

---

### Task 10: RunPod worker 仓库

**Files:**
- Create: `/usr/src/workspace/github/QQhuxuhui/runpod-upscale-worker/Dockerfile`
- Create: `/usr/src/workspace/github/QQhuxuhui/runpod-upscale-worker/rp_handler.py`
- Create: `/usr/src/workspace/github/QQhuxuhui/runpod-upscale-worker/test_input.json`
- Create: `/usr/src/workspace/github/QQhuxuhui/runpod-upscale-worker/README.md`

**Interfaces:**
- Consumes: Task 7 定义的 job input：`{src_url, put_url, out_key, target_w, target_h}`（`out_key` worker 不用，仅供 new-api 侧读回，忽略即可）；本地自测走 `{src_b64, target_w, target_h}` 旁路
- Produces: worker 把结果 PNG PUT 到 `put_url`（Content-Type 必须 `image/png`，与预签名一致），返回 `{width, height, bytes}`；`src_b64` 旁路返回 `{out_b64, width, height}`

- [ ] **Step 1: 建仓库与 handler**

```bash
mkdir -p /usr/src/workspace/github/QQhuxuhui/runpod-upscale-worker
cd /usr/src/workspace/github/QQhuxuhui/runpod-upscale-worker && git init
```

```python
# rp_handler.py
"""RunPod Serverless worker: Real-ESRGAN x4plus 超分。

约定（与 new-api service/image_upscale.go 对齐）：
  input: {src_url, put_url, target_w, target_h}   # presigned URL, worker 零凭据
  流程 : GET src_url → ESRGAN 4x → Lanczos 精确缩放到 target → PUT put_url (image/png)
  本地自测旁路: {src_b64, target_w, target_h} → 返回 {out_b64}

FlashBoot 关键：模型加载与 CUDA 预热必须在模块顶层——快照只捕获缩容瞬间
已存在的进程状态，懒加载会让每次冷启动重付 10-30s。
"""
import base64
import io

import numpy as np
import requests
import runpod
import torch
from PIL import Image
from basicsr.archs.rrdbnet_arch import RRDBNet
from realesrgan import RealESRGANer

DEVICE = torch.device("cuda" if torch.cuda.is_available() else "cpu")
_model = RRDBNet(num_in_ch=3, num_out_ch=3, num_feat=64,
                 num_block=23, num_grow_ch=32, scale=4)
UPSAMPLER = RealESRGANer(
    scale=4,
    model_path="/models/RealESRGAN_x4plus.pth",
    model=_model,
    tile=512,          # 大图分块，防显存溢出
    tile_pad=32,
    half=(DEVICE.type == "cuda"),
    device=DEVICE,
)
# 预热：让 FlashBoot 快照到已初始化的 CUDA context 与首次编译产物
UPSAMPLER.enhance(np.zeros((64, 64, 3), dtype=np.uint8), outscale=4)


def _upscale_to(img: Image.Image, tw: int, th: int) -> bytes:
    out, _ = UPSAMPLER.enhance(np.array(img.convert("RGB")), outscale=4)
    pil = Image.fromarray(out)
    if pil.size != (tw, th):
        pil = pil.resize((tw, th), Image.LANCZOS)  # 4x 后精确缩放（4K 档非整倍）
    buf = io.BytesIO()
    pil.save(buf, format="PNG")
    return buf.getvalue()


def handler(job):
    inp = job["input"]
    tw, th = int(inp["target_w"]), int(inp["target_h"])

    if inp.get("src_b64"):  # 本地自测旁路
        img = Image.open(io.BytesIO(base64.b64decode(inp["src_b64"])))
        data = _upscale_to(img, tw, th)
        return {"out_b64": base64.b64encode(data).decode(), "width": tw, "height": th}

    src = requests.get(inp["src_url"], timeout=60)
    src.raise_for_status()
    data = _upscale_to(Image.open(io.BytesIO(src.content)), tw, th)
    put = requests.put(inp["put_url"], data=data,
                       headers={"Content-Type": "image/png"}, timeout=120)
    put.raise_for_status()
    return {"width": tw, "height": th, "bytes": len(data)}


runpod.serverless.start({"handler": handler})
```

```dockerfile
# Dockerfile
FROM runpod/pytorch:2.1.0-py3.10-cuda11.8.0-devel-ubuntu22.04

RUN pip install --no-cache-dir runpod realesrgan basicsr requests
# basicsr 与新 torchvision 的已知兼容问题（functional_tensor 被移除）
RUN sed -i 's/from torchvision.transforms.functional_tensor import rgb_to_grayscale/from torchvision.transforms.functional import rgb_to_grayscale/' \
    $(python3 -c "import basicsr, os; print(os.path.join(os.path.dirname(basicsr.__file__),'data','degradations.py'))") || true

RUN mkdir -p /models && wget -q -O /models/RealESRGAN_x4plus.pth \
    https://github.com/xinntao/Real-ESRGAN/releases/download/v0.1.0/RealESRGAN_x4plus.pth

COPY rp_handler.py /rp_handler.py
CMD ["python3", "-u", "/rp_handler.py"]
```

`test_input.json`（64x64 红色方块 PNG 的 b64，构建时生成）：

```bash
python3 - <<'EOF'
import base64, io, json
from PIL import Image
buf = io.BytesIO()
Image.new("RGB", (64, 64), (200, 30, 30)).save(buf, format="PNG")
json.dump({"input": {"src_b64": base64.b64encode(buf.getvalue()).decode(),
                     "target_w": 256, "target_h": 256}},
          open("test_input.json", "w"))
EOF
```

`README.md`：一段话说明镜像用途、input 约定、构建推送命令（`docker build -t <registry>/runpod-upscale-worker:v1 . && docker push ...`）、endpoint 推荐配置（16GB 档 / min 0 / max 3 / idle 5s / FlashBoot on）。

- [ ] **Step 2: 构建镜像**

```bash
cd /usr/src/workspace/github/QQhuxuhui/runpod-upscale-worker
docker build -t runpod-upscale-worker:dev .
```

Expected: 构建成功（权重下载 64MB）

- [ ] **Step 3: 容器内 CPU 自测**

```bash
docker run --rm -v $PWD/test_input.json:/test_input.json runpod-upscale-worker:dev \
  python3 -u /rp_handler.py --test_input "$(cat test_input.json)"
```

Expected: 日志输出 handler 返回 `{"out_b64": "...", "width": 256, "height": 256}`（CPU 上 64→256 数秒内完成）。用一段 python 解 out_b64 验证确为 256x256 PNG。

- [ ] **Step 4: Commit（worker 仓库）**

```bash
cd /usr/src/workspace/github/QQhuxuhui/runpod-upscale-worker
git add Dockerfile rp_handler.py test_input.json README.md
git commit -m "feat: Real-ESRGAN x4plus RunPod serverless worker（presigned URL 零凭据）"
```

---

### Task 11: 部署与灰度验收（运维 checklist）

**Files:**
- Modify: OVH 上 new-api 的 docker-compose 环境变量（生产操作，非本仓库文件）

此任务是操作清单，逐项人工执行并勾选；代码零改动。

- [ ] **Step 1: 对象存储就绪**

R2（推荐）：建 bucket `upscale`，生成 S3 API Token（限该 bucket 读写），配生命周期规则 1 天过期。OSS 替代：专用 RAM 子账号，权限仅该 bucket `oss:GetObject/PutObject`。

- [ ] **Step 2: worker 上线**

```bash
docker tag runpod-upscale-worker:dev <registry>/runpod-upscale-worker:v1
docker push <registry>/runpod-upscale-worker:v1
```

RunPod 控制台 → Serverless → New Endpoint：镜像 `<registry>/runpod-upscale-worker:v1`，GPU 16GB（A4000/A4500），workers min=0 / max=3，idle timeout 5s，FlashBoot on。记下 endpoint id 与 API key。

- [ ] **Step 3: 冒烟 worker**

用 R2 手工传一张 1024 PNG，预签 GET/PUT（`aws s3 presign` 或控制台），直接 `curl -X POST {endpoint}/run` 提交，确认 out.png 出现且尺寸正确；再看 worker 日志确认第二次调用冷启动 <1s（FlashBoot 快照命中；持续 >10s 说明预热回归）。

- [ ] **Step 4: new-api 上环境变量并部署**

compose 的 new-api 服务加：

```yaml
      IMAGE_UPSCALE_ENABLED: "true"
      IMAGE_UPSCALE_RUNPOD_ENDPOINT: "https://api.runpod.ai/v2/<endpoint_id>"
      IMAGE_UPSCALE_RUNPOD_API_KEY: "<rpa_...>"
      IMAGE_UPSCALE_TIMEOUT: "90s"
      IMAGE_UPSCALE_S3_ENDPOINT: "https://<acc>.r2.cloudflarestorage.com"
      IMAGE_UPSCALE_S3_BUCKET: "upscale"
      IMAGE_UPSCALE_S3_AK: "<ak>"
      IMAGE_UPSCALE_S3_SK: "<sk>"
```

构建镜像、部署（沿用现有 build-and-push 流程），`docker logs` 无 `storage init failed`。

- [ ] **Step 5: 灰度渠道验收（spec §13）**

1. 后台克隆一条现有 1K gpt-image-2 渠道，setting 配 `{"image_sizes":{"allowed":["1K"],"upscale":{"from":"1K","to":"4K"}}}`，`weight=0` 低优先级；
2. 用 sub2api 的 key 发 `size=3840x2160` 请求手动打到该渠道 → 响应图实测 3840x2160 PNG；
3. sub2api 日志确认该笔按 4K 档计费（`openai_images.usage_simulated` 且 output size 为 4K 尺寸）；
4. new-api 渠道日志出现 `大小 3840x2160 ...（1K 超分）` 与 `image_upscale_done`；
5. 发 `size=2560x1440`（宽松中间档）→ 得 2560x1440，验证派生；
6. 发 `stream` / `n=2` 形状请求 → 该渠道不被选中（原生渠道接走或 503），验证资格闸门；
7. 降级演练：临时把 `IMAGE_UPSCALE_RUNPOD_API_KEY` 改错重启 → 4K 请求得到 1K 原图 + `image_upscale_degraded` 日志，sub2api 按 1K 计费；改回。

- [ ] **Step 6: 收尾**

灰度观察 1-2 天（degraded 率、耗时分布、RunPod 账单），符合预期后按需提高渠道权重。告警：现阶段靠日志人工看，`image_upscale_degraded` 的 TG 告警接入排到后续（spec §11 注记）。

---

## Self-Review 记录

- **Spec 覆盖**：§4 配置模型→Task 1；§5 选路派生→Task 2/5；§6 资格谓词→Task 4；§7 出站降档→Task 3/9；§8 回程超分+存储+worker→Task 6/7/8/10；§9 降级→Task 7/8/9 的错误契约 + Task 11 演练；§10 计费一致性→Task 4 注释对齐 + Task 11 验收 3；§11 观测→Task 5 计数 + Task 9 日志 + Task 11 Step 6（TG 告警显式降级为后续项）；§12 成本→Task 11 Step 6 观察；§13 测试→各任务 TDD + Task 11；§14 分期→multipart passthrough 跳过（Task 9b）与 chat 路径不做一致。
- **偏差声明**（相对 spec，均为实施期合理化，不改变行为契约）：① spec §8"校验源图尺寸=预期降档尺寸"实现为不校验源图、只硬校验输出尺寸——源图尺寸偏差不影响正确性（4x 后按目标精确缩放），硬失败反而浪费已付费生成；② worker 采用 presigned PUT 使其零凭据，spec 的"凭据最小化"的更强形式。
- **类型一致性**：`UpscaleFromTier`/`AllowWithUpscale`/`MapImageSizeForUpscale`/`RewriteImageResponseWithUpscale`/`GetImageUpscaler().UpscaleImage` 签名在 Task 2/3/7/8/9 间已交叉核对一致；worker input 字段（`src_url/put_url/out_key/target_w/target_h`）Task 7 与 Task 10 一致。
