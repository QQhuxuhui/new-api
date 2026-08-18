package relay

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
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
	c.Set(common.KeyRequestBody, rewritten)
	return true
}
