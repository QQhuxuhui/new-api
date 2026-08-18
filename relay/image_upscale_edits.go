package relay

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

// downgradeEditsRequestSize 把降档尺寸写进 edits 出站转换真正读取的数据源。
//
// generations 的 ConvertImageRequest 读结构体，改 request.Size 即可；edits 的
// 两条转换路径都不看 request.Size：
//   - 入站 multipart：adaptor 把 c.Request.MultipartForm.Value 逐字复制进出站表单，
//     故原位改写该 map 的 size 即可，公共转换代码零改动。
//   - 入站 JSON：writeEditsFormFromJSON 从 common.GetRequestBody(c) 的缓存体复制
//     非文件字段，故用 sjson 改写后回填 common.KeyRequestBody。
//
// 返回 false 表示改写失败（数据源不可用），调用方应放弃本次超分、原样直通，
// 绝不能让上游收到未降档的 4K 请求。passthrough 组合不在此处处理：那条分支
// 自带各自的改写（JSON）/ 跳过（multipart）逻辑。
//
// 【重试安全】这两处数据源都是跨重试共享的可变状态，而 controller 的
// rewindRequestForRetry 只回卷 Body/Content-Type，不认识它们。所以改写前必须先
// 通过 saveEditsOriginal* 记下原值，由 restoreEditsRequestSize 在每次进入
// ImageHelper 时先行恢复——否则渠道 A 降档失败后重试到无超分规则的渠道 B，
// B 会收到陈旧的降档尺寸，客户按 4K 付费却拿 1K 图。
func downgradeEditsRequestSize(c *gin.Context, info *relaycommon.RelayInfo, plan *imageUpscalePlan) bool {
	if plan == nil || info == nil {
		return false
	}
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		return true
	}
	if info.RelayMode != relayconstant.RelayModeImagesEdits {
		// generations 等路径由 request.Size 承载降档，无需改写原始请求体。
		return true
	}

	if strings.Contains(c.ContentType(), "multipart") {
		if c.Request.MultipartForm == nil {
			// 正常流程里校验阶段已解析过；这里解析失败说明后续转换也必失败。
			if _, err := c.MultipartForm(); err != nil || c.Request.MultipartForm == nil {
				logger.LogWarn(c, "image_upscale: edits multipart parse failed, skip upscale")
				return false
			}
		}
		if c.Request.MultipartForm.Value == nil {
			c.Request.MultipartForm.Value = make(map[string][]string, 1)
		}
		saveEditsOriginalFormSize(c)
		c.Request.MultipartForm.Value["size"] = []string{plan.DowngradedSize}
		return true
	}

	body, err := common.GetRequestBody(c)
	if err != nil {
		logger.LogWarn(c, fmt.Sprintf("image_upscale: edits json body read failed, skip upscale: %v", err))
		return false
	}
	rewritten, err := sjson.SetBytes(body, "size", plan.DowngradedSize)
	if err != nil {
		logger.LogWarn(c, fmt.Sprintf("image_upscale: edits json size rewrite failed, skip upscale: %v", err))
		return false
	}
	saveEditsOriginalBody(c, body)
	c.Set(common.KeyRequestBody, rewritten)
	return true
}

// saveEditsOriginalFormSize 在首次降档前记下 MultipartForm.Value["size"] 的原值。
// 字段原本不存在时存 nil 切片，恢复阶段据此把字段整个删掉，而不是补一个空值。
// 只在第一次记录：后续重试恢复出来的就是原值，重复记录反而会把原值覆盖成降档值。
func saveEditsOriginalFormSize(c *gin.Context) {
	if _, saved := common.GetContextKey(c, constant.ContextKeyImageEditsOriginalFormSize); saved {
		return
	}
	orig, exists := c.Request.MultipartForm.Value["size"]
	if !exists {
		common.SetContextKey(c, constant.ContextKeyImageEditsOriginalFormSize, []string(nil))
		return
	}
	common.SetContextKey(c, constant.ContextKeyImageEditsOriginalFormSize, append([]string(nil), orig...))
}

// saveEditsOriginalBody 在首次降档前记下 KeyRequestBody 缓存体原文（拷贝一份，
// 避免与后续 sjson 结果共享底层数组）。同样只记第一次。
func saveEditsOriginalBody(c *gin.Context, body []byte) {
	if _, saved := common.GetContextKey(c, constant.ContextKeyImageEditsOriginalBody); saved {
		return
	}
	common.SetContextKey(c, constant.ContextKeyImageEditsOriginalBody, append([]byte(nil), body...))
}

// restoreEditsRequestSize 把上一轮降档改写过的共享数据源恢复成原值。
//
// 必须在 ImageHelper 每次进入时（含跨渠道重试重进）无条件调用，且要在
// resolveImageUpscalePlan / 出站转换之前：渠道 A 降档后失败，重试到没有超分
// 规则的渠道 B 时 downgradeEditsRequestSize 根本不会被调用，只有这里能把
// 上游看到的尺寸拉回原始值。
//
// 恢复后【保留】原值不清除：同一请求可能再重试到另一个有超分规则的渠道，
// 那次降档要能继续以原值为基准（saveEditsOriginal* 的"只记第一次"与之配套）。
func restoreEditsRequestSize(c *gin.Context) {
	if c == nil {
		return
	}
	if v, ok := common.GetContextKey(c, constant.ContextKeyImageEditsOriginalBody); ok {
		if body, isBytes := v.([]byte); isBytes {
			c.Set(common.KeyRequestBody, append([]byte(nil), body...))
		}
	}
	if v, ok := common.GetContextKey(c, constant.ContextKeyImageEditsOriginalFormSize); ok {
		if c.Request == nil || c.Request.MultipartForm == nil || c.Request.MultipartForm.Value == nil {
			return
		}
		orig, isSlice := v.([]string)
		if !isSlice || orig == nil {
			// 原本没有 size 字段：删掉降档时补进去的那个，别留一个凭空的值。
			delete(c.Request.MultipartForm.Value, "size")
			return
		}
		c.Request.MultipartForm.Value["size"] = append([]string(nil), orig...)
	}
}
