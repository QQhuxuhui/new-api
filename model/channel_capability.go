package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

// ChannelSelectFilter 承载"选路阶段就能判定"的渠道能力约束。
//
// 之所以在选渠道时过滤而不是打上游试错：图片请求的档位（1K/2K/4K）从
// 请求参数就能算出来，渠道支持哪些档位是静态配置。让不支持的渠道进入
// 重试链只会白白消耗重试次数、拖长首字延迟，还会给上游打一发注定失败的请求。
//
// 零值 / nil 表示不过滤——所有非图片路径都走这条路，开销为零。
type ChannelSelectFilter struct {
	// ImageSizeTier 为空表示本次请求判不出档位（auto、比例写法、解析失败），
	// 此时一律不过滤：选路过滤只做"明确不支持时提前排除"，不确定必须 fail open。
	ImageSizeTier string
	// ImageHighQuality 表示 quality 为 high/4k/ultra，需要渠道独立声明支持。
	ImageHighQuality bool
	// UpscaleEligible 本请求具备超分资格（context 注入），派生可达集仅在 true 时生效
	UpscaleEligible bool

	// 拒绝原因分开记录，避免把 quality 开关拒绝误报成 size 档位不支持。
	rejected             bool
	imageSizeRejected    bool
	imageQualityRejected bool
	imageSizeViaUpscale  int // 观测：经超分派生（而非原生白名单）通过的次数
}

// MarkRejected 由各条选路路径在真的排除掉渠道时调用。
//
// 没有这个信号的话，"无可用渠道"的报错只能靠"本次请求有没有能力约束"来决定文案，
// 而 channel==nil 是所有失败原因的共同出口——渠道全挂、模型没配到这个分组、
// 套餐把组滤空都会走到那里。那样最常见的故障会被伪装成一个几乎不存在的
// 白名单配置问题，把运维的排查方向带偏，比泛化文案更糟。
//
// 选路在单个请求内始终串行（plan_groups 循环、优先级循环、并发限流重试
// 都在同一 goroutine），因此不需要原子操作。
func (f *ChannelSelectFilter) MarkRejected() {
	if f != nil {
		f.rejected = true
	}
}

// Rejected 供 service 层在选路返回后读取。
func (f *ChannelSelectFilter) Rejected() bool {
	return f != nil && f.rejected
}

func (f *ChannelSelectFilter) markImageSizeRejected() {
	if f != nil {
		f.rejected = true
		f.imageSizeRejected = true
	}
}

func (f *ChannelSelectFilter) markImageQualityRejected() {
	if f != nil {
		f.rejected = true
		f.imageQualityRejected = true
	}
}

func (f *ChannelSelectFilter) markImageSizeViaUpscale() {
	if f != nil {
		f.imageSizeViaUpscale++
	}
}

func (f *ChannelSelectFilter) ImageSizeRejected() bool {
	return f != nil && f.imageSizeRejected
}

func (f *ChannelSelectFilter) ImageQualityRejected() bool {
	return f != nil && f.imageQualityRejected
}

// Active 表示本次选路真的需要过滤。非图片请求恒为 false，
// 用它把所有额外开销（DB 批量查询、逐渠道 setting 解析）挡在门外。
func (f *ChannelSelectFilter) Active() bool {
	return f != nil && (f.ImageSizeTier != "" || f.ImageHighQuality)
}

// Describe 供无可用渠道时的错误文案与日志使用。
func (f *ChannelSelectFilter) Describe() string {
	if !f.Active() {
		return ""
	}
	switch {
	case f.ImageSizeTier != "" && f.ImageHighQuality:
		return fmt.Sprintf("图片档位 %s 与高质量图片能力", f.ImageSizeTier)
	case f.ImageSizeTier != "":
		return fmt.Sprintf("图片档位 %s", f.ImageSizeTier)
	default:
		return "高质量图片能力"
	}
}

// channelSatisfiesFilter 判定单个渠道是否满足本次选路约束。
// 用 GetSettingReadonly：内存缓存模式下 channelsIDM 里的 *Channel 是共享对象，
// GetSetting 解析失败会就地清空字段并写库，绝不能在选路热路径上触发。
func channelSatisfiesFilter(channel *Channel, filter *ChannelSelectFilter) bool {
	if !filter.Active() || channel == nil {
		return true
	}
	setting := channel.GetSettingReadonly()
	satisfied := true
	if filter.ImageSizeTier != "" {
		if !setting.ImageSizes.AllowWithUpscale(filter.ImageSizeTier, filter.UpscaleEligible) {
			filter.markImageSizeRejected()
			satisfied = false
		} else if !setting.ImageSizes.Allow(filter.ImageSizeTier) {
			// 原生不通、派生通 ⇒ 这条渠道的该档位是超分出来的，运营侧要看得见
			filter.markImageSizeViaUpscale()
		}
	}
	if filter.ImageHighQuality && setting.ImageQualityEnabled != nil && !*setting.ImageQualityEnabled {
		filter.markImageQualityRejected()
		satisfied = false
	}
	return satisfied
}

// channelSettingRow 只取选路过滤需要的两列，避免把 key 等大字段拉进内存。
type channelSettingRow struct {
	Id      int     `gorm:"column:id"`
	Setting *string `gorm:"column:setting"`
}

// filterChannelIdsByFilter 批量判定一组渠道 ID 是否满足约束（DB 模式用）。
// 一次 IN 查询搞定，而不是逐候选 GetChannelById——后者在候选多时会把
// 一次选路放大成 N 次往返。
//
// 查询出错、渠道行缺失一律视为放行：过滤器是"锦上添花"的优化，
// 任何自身故障都不能把可用渠道判死。
func filterChannelIdsByFilter(channelIds []int, filter *ChannelSelectFilter) map[int]bool {
	if !filter.Active() || len(channelIds) == 0 {
		return nil
	}
	unique := make([]int, 0, len(channelIds))
	seen := make(map[int]bool, len(channelIds))
	for _, id := range channelIds {
		if seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}

	var rows []channelSettingRow
	if err := DB.Model(&Channel{}).Select("id", "setting").Where("id IN ?", unique).Find(&rows).Error; err != nil {
		common.SysLog(fmt.Sprintf("channel capability filter query failed, failing open: %v", err))
		return nil
	}

	rejected := make(map[int]bool)
	for _, row := range rows {
		channel := Channel{Id: row.Id, Setting: row.Setting}
		if !channelSatisfiesFilter(&channel, filter) {
			rejected[row.Id] = true
		}
	}
	if len(rejected) == 0 {
		return nil
	}
	return rejected
}
