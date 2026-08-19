package dto

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// 图片档位常量。口径与 sub2api 的 ClassifyImageBillingTier 完全一致，
// 两侧必须同步：new-api 用它做选路过滤，sub2api 用它做计费分档，
// 一侧口径漂移会让"选到的渠道"和"被计费的档位"对不上。
const (
	ImageSizeTier1K = "1K"
	ImageSizeTier2K = "2K"
	ImageSizeTier4K = "4K"

	// ImageResampleMaxDimension 是本地超分/尺寸规整允许的单边像素上限。
	// 上游仍负责校验自身支持的尺寸；这里只约束会进入本地 worker 的任务。
	ImageResampleMaxDimension = 4096
)

// DefaultImageSizeForModel returns the size applied later by image request
// validation. Routing must use the same default so omitted sizes do not bypass
// channel capability filtering.
func DefaultImageSizeForModel(model string) string {
	switch model {
	case "dall-e", "dall-e-2", "dall-e-3":
		return "1024x1024"
	default:
		return ""
	}
}

// imageSizeTierRatioTable 是 gpt-image-2 各宽高比 × 各档位的实测原生尺寸。
// 只保留尺寸（token 数是计费侧的事，选路用不到，抄过来只会随上游调价而腐烂）。
var imageSizeTierRatioTable = map[string]map[string]string{
	"1:1": {
		ImageSizeTier1K: "1024x1024",
		ImageSizeTier2K: "2048x2048",
		ImageSizeTier4K: "2880x2880",
	},
	"5:4": {
		ImageSizeTier1K: "1120x896",
		ImageSizeTier2K: "2240x1792",
		ImageSizeTier4K: "3200x2560",
	},
	"4:3": {
		ImageSizeTier1K: "1152x864",
		ImageSizeTier2K: "2304x1728",
		ImageSizeTier4K: "3264x2448",
	},
	"3:2": {
		ImageSizeTier1K: "1248x832",
		ImageSizeTier2K: "2496x1664",
		ImageSizeTier4K: "3504x2336",
	},
	"16:9": {
		ImageSizeTier1K: "1280x720",
		ImageSizeTier2K: "2560x1440",
		ImageSizeTier4K: "3840x2160",
	},
	"21:9": {
		ImageSizeTier1K: "1456x624",
		ImageSizeTier2K: "3024x1296",
		ImageSizeTier4K: "3696x1584",
	},
}

type imageSizeGeometry struct {
	Width  int
	Height int
	Ratio  string
	Tier   string
}

var imageSizeIndex = buildImageSizeIndex()

// buildImageSizeIndex 把比例表摊平成 "宽x高" → 档位 的查找表。
// 5:4 / 4:3 / 3:2 / 16:9 同时登记竖版（转置）尺寸：OpenAI 对这些比例
// 接受横竖两种写法，且两者属于同一档位。1:1 是正方形无需转置，
// 21:9 上游未提供竖版，登记了反而会把不存在的尺寸判成合法档位。
func buildImageSizeIndex() map[string]imageSizeGeometry {
	transposable := map[string]bool{"5:4": true, "4:3": true, "3:2": true, "16:9": true}
	index := make(map[string]imageSizeGeometry, 30)
	for ratio, tiers := range imageSizeTierRatioTable {
		for tier, size := range tiers {
			width, height, ok := parseImageSizeDimensions(size)
			if !ok {
				continue
			}
			index[imageSizeDimensionsKey(width, height)] = imageSizeGeometry{
				Width: width, Height: height, Ratio: ratio, Tier: tier,
			}
			if width != height && transposable[ratio] {
				index[imageSizeDimensionsKey(height, width)] = imageSizeGeometry{
					Width: height, Height: width, Ratio: ratio, Tier: tier,
				}
			}
		}
	}
	return index
}

func lookupImageSize(width, height int) (imageSizeGeometry, bool) {
	if width <= 0 || height <= 0 {
		return imageSizeGeometry{}, false
	}
	geometry, ok := imageSizeIndex[imageSizeDimensionsKey(width, height)]
	return geometry, ok
}

func imageSizeDimensionsKey(width, height int) string {
	return fmt.Sprintf("%dx%d", width, height)
}

func parseImageSizeDimensions(size string) (int, int, bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

// ImageResampleDimensionsAllowed 判断精确目标尺寸能否进入本地重采样链路。
func ImageResampleDimensionsAllowed(width, height int) bool {
	return width > 0 && height > 0 &&
		width <= ImageResampleMaxDimension && height <= ImageResampleMaxDimension
}

// ClassifyImageRoutingTier 是**选路**用的档位判定，按最长边。
//
// 与下面按面积的 ClassifyImageBillingTier 并存，是因为两者回答的问题不同：
// 计费问的是"这张图该按多少钱算"（面积语义，1280x768 计 1K）；
// 选路问的是"上游那张尺寸表够不够得着这条边"。上游 adobe2api 的判据
// （core/models/resolver.py:199-215 resolution_from_size）就是纯 max(w,h)：
// <=1024 → 1K，<=2048 → 2K，否则 4K。选路口径必须与它逐字对应，
// 否则过滤器预测不准上游会不会拒。
//
// 拿面积口径做选路的具体后果（实跑 515 个尺寸 + 14 天真实请求量）：
// 2560x1440 / 2304x1728 / 2496x1664 / 3024x1296 这些实测表里的原生 2K 尺寸，
// 面积口径判 2K 放行，上游按长边判 4K 照拒——渠道 190 的召回只有 45%。
// 换长边后召回 100% 且零误杀（"上游放行 ⟹ 长边<=2048" 在 gpt 族严格成立）。
//
// sub2api 早就把这两个口径拆开了，见 openai_images.go:552 openAIImageSizeRequiresHighRes
// 的注释："路由口径与计费档位解耦……路由仍按最长边判断"。
func ClassifyImageRoutingTier(size string) (string, bool) {
	trimmed := strings.TrimSpace(size)
	switch strings.ToLower(trimmed) {
	case "", "auto":
		return "", false
	case "1k":
		return ImageSizeTier1K, true
	case "2k":
		return ImageSizeTier2K, true
	case "4k":
		return ImageSizeTier4K, true
	}
	width, height, ok := parseImageSizeDimensions(trimmed)
	if !ok {
		return "", false
	}
	longest := width
	if height > longest {
		longest = height
	}
	switch {
	case longest <= 1024:
		return ImageSizeTier1K, true
	case longest <= 2048:
		return ImageSizeTier2K, true
	default:
		return ImageSizeTier4K, true
	}
}

// ImageQualityRequiresCapability 判断 quality 参数是否需要渠道显式支持高质量图片。
//
// 上游把 quality 单独映射成 output_resolution（adobe2api core/models/quality.py 的
// 别名表 + api/routes/generation.py:801），high/4k/ultra 会直接 400，
// **与 size 完全无关**：quality="high" 配 size="1024x1024" 一样被拒。
// 因此它使用独立渠道开关，绝不参与图片 size 档位计算。
func ImageQualityRequiresCapability(quality string) bool {
	switch strings.TrimSpace(strings.ToLower(quality)) {
	case "high", "4k", "ultra":
		return true
	default:
		return false
	}
}

// ClassifyImageBillingTier 把请求里的 size 参数归到 1K/2K/4K 档位。
// 第二个返回值为 false 表示"判不出档位"——调用方必须放行而不是拒绝：
// size 可以是 auto、比例写法（"1:1"）、上游新增的未知写法，
// 在选路阶段因为看不懂就拒绝渠道会把正常请求打成 503。
//
// 注意：这是**计费**口径（面积 + 实测表），与 sub2api 逐行对齐，是两边防漂移的锚点。
// 选路请用上面的 ClassifyImageRoutingTier，不要改这个函数去迁就选路。
func ClassifyImageBillingTier(size string) (string, bool) {
	trimmed := strings.TrimSpace(size)
	normalized := strings.ToLower(trimmed)
	switch normalized {
	case "", "auto":
		return "", false
	case "1k":
		return ImageSizeTier1K, true
	case "2k":
		return ImageSizeTier2K, true
	case "4k":
		return ImageSizeTier4K, true
	case "2048x2048", "2048x1152":
		return ImageSizeTier2K, true
	case "3840x2160", "2160x3840":
		return ImageSizeTier4K, true
	}

	width, height, ok := parseImageSizeDimensions(trimmed)
	if !ok {
		return "", false
	}
	// 实测表命中时以模型原生档位为准：非方形的 2K 档长边天然超过 2048
	// （如 2560x1440、2304x1728），按最长边会错判成 4K
	if geometry, ok := lookupImageSize(width, height); ok {
		return geometry.Tier, true
	}
	// 表外尺寸按像素总量兜底，与档位的面积语义一致（1K/2K/4K 各档不同
	// 宽高比的面积近似 1024²/2048²/2880² 网格）。用除法比较避免用户提供
	// 的超大尺寸在 width*height 时发生整数溢出并错误降档
	switch {
	case width <= 1024*1024/height:
		return ImageSizeTier1K, true
	case width <= 2048*2048/height:
		return ImageSizeTier2K, true
	default:
		return ImageSizeTier4K, true
	}
}

// NormalizeImageSizeTier 把用户/前端写入的档位名归一到常量形式。
func NormalizeImageSizeTier(tier string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(tier)) {
	case ImageSizeTier1K:
		return ImageSizeTier1K, true
	case ImageSizeTier2K:
		return ImageSizeTier2K, true
	case ImageSizeTier4K:
		return ImageSizeTier4K, true
	default:
		return "", false
	}
}

// AllImageSizeTiers 返回全部合法档位（升序），供前端下拉与错误提示使用。
func AllImageSizeTiers() []string {
	return []string{ImageSizeTier1K, ImageSizeTier2K, ImageSizeTier4K}
}

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

// ImageSizeCapability 声明渠道能承接哪些图片档位。
//
// 语义刻意做成"只有明确写了白名单才收紧"：
//   - 整个结构为 nil（存量渠道 setting 里没有 image_sizes）    → 全部档位放行
//   - Allowed 为空数组（管理员开了配置但一个都没勾）           → 全部档位放行
//
// 空数组不解释成"什么都不支持"是有意的：那种配置只可能是误操作，
// 而它的后果是整条渠道对图片请求彻底消失且没有任何报错线索。
type ImageSizeCapability struct {
	// Allowed 是档位白名单，元素取值 1K/2K/4K。
	Allowed []string `json:"allowed,omitempty"`
	// Upscale 声明超分规则，见 ImageUpscaleRule。nil = 无超分能力。
	Upscale *ImageUpscaleRule `json:"upscale,omitempty"`
	// Normalize 声明"尺寸规整"：上游实际出图尺寸与用户请求的精确 WxH 不一致时，
	// 回程经超分链路重采样到请求尺寸（放大走 ESRGAN，缩小纯 Lanczos）。
	// 与 Allowed/Upscale 正交：不要求配置档位白名单，也不参与选路派生。
	Normalize bool `json:"normalize,omitempty"`
}

// Allow 判断该渠道能否承接 tier 档位的请求。
// nil 接收者、空白名单、空 tier（分类失败）一律放行——选路过滤只做
// "明确不支持时提前排除"，任何不确定都必须 fail open。
func (c *ImageSizeCapability) Allow(tier string) bool {
	if c == nil || len(c.Allowed) == 0 {
		return true
	}
	normalizedTier, ok := NormalizeImageSizeTier(tier)
	if !ok {
		return true
	}
	// anyValid 区分"白名单里没有这一档"和"白名单整个写废了"。
	// ValidateSettings 只在走 API 的写入路径上把关，SQL/后台脚本直接改 setting
	// 绕得过去；全是垃圾项的白名单与空数组一样只可能是误操作，若按字面全拒，
	// 该渠道会对所有图片请求隐身，线上唯一线索只有一行 incapable 计数。
	anyValid := false
	for _, allowed := range c.Allowed {
		candidate, ok := NormalizeImageSizeTier(allowed)
		if !ok {
			continue
		}
		anyValid = true
		if candidate == normalizedTier {
			return true
		}
	}
	return !anyValid
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

// Validate 校验白名单里的档位名合法，避免管理员写错字符串后
// 变成一条永远匹配不上、却又静默排除所有图片请求的配置。
func (c *ImageSizeCapability) Validate() error {
	if c == nil {
		return nil
	}
	for _, allowed := range c.Allowed {
		if _, ok := NormalizeImageSizeTier(allowed); !ok {
			return fmt.Errorf("image_sizes.allowed 含非法档位 %q，仅支持 %s",
				allowed, strings.Join(AllImageSizeTiers(), "/"))
		}
	}
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
	return nil
}

// NormalizedAllowed 返回去重、归一并排序后的白名单，便于日志与提示展示。
func (c *ImageSizeCapability) NormalizedAllowed() []string {
	if c == nil {
		return nil
	}
	seen := make(map[string]bool, len(c.Allowed))
	out := make([]string, 0, len(c.Allowed))
	for _, allowed := range c.Allowed {
		tier, ok := NormalizeImageSizeTier(allowed)
		if !ok || seen[tier] {
			continue
		}
		seen[tier] = true
		out = append(out, tier)
	}
	sort.Strings(out)
	return out
}

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
	if !ok || !ImageResampleDimensionsAllowed(width, height) {
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

// NormalizeEnabled 判定该渠道是否开启"尺寸规整"。nil 接收者安全（未配置=关闭）。
func (c *ImageSizeCapability) NormalizeEnabled() bool {
	return c != nil && c.Normalize
}

// ParseImageSizeWH 把 "WxH" 写法解析为精确宽高（导出给 relay 的尺寸规整用）。
// 字面档位（1K/2K/4K）、auto、比例写法等没有精确像素语义，返回 false。
func ParseImageSizeWH(size string) (int, int, bool) {
	return parseImageSizeDimensions(size)
}
