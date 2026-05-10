package controller

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// 海报上传配置常量
const (
	posterMaxFileSize = 5 * 1024 * 1024 // 5 MB
	posterFormField   = "file"
	posterOssPrefix   = "posters/"
)

// posterAllowedMimes 是允许上传的图片 mime 类型白名单。
// 不接受 image/svg+xml(可能内嵌 script,XSS 风险)。
var posterAllowedMimes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// UploadPoster POST /api/option/poster/upload
//
// 管理员上传海报图片到阿里云 OSS。校验:
//   - 文件大小 ≤ 5 MB
//   - Content-Type 在白名单(image/jpeg / png / webp / gif)
//   - OSS 4 个配置项已设置
//
// 上传成功后返回 OSS public URL,**不**自动覆盖 PosterImageUrl
// (让管理员预览后手动保存)。
func UploadPoster(c *gin.Context) {
	fileHeader, err := c.FormFile(posterFormField)
	if err != nil {
		common.ApiErrorMsg(c, "未提供文件 (form field 'file')")
		return
	}

	if fileHeader.Size > posterMaxFileSize {
		common.ApiErrorMsg(c, fmt.Sprintf("文件 size 超出限制 5 MB(实际 %d bytes)", fileHeader.Size))
		return
	}

	contentType := fileHeader.Header.Get("Content-Type")
	ext, ok := posterAllowedMimes[strings.ToLower(contentType)]
	if !ok {
		common.ApiErrorMsg(c, fmt.Sprintf("不支持的图片 type %q,仅允许 image/jpeg/png/webp/gif", contentType))
		return
	}

	// 优先使用 mime 推断的扩展名,fallback 到原始文件名扩展
	if ext == "" {
		ext = filepath.Ext(fileHeader.Filename)
	}
	objectKey := posterOssPrefix + "poster_" + uuid.NewString() + ext

	src, err := fileHeader.Open()
	if err != nil {
		common.ApiError(c, fmt.Errorf("读取上传文件失败: %w", err))
		return
	}
	defer src.Close()

	publicURL, err := service.UploadFileToOSS(src, objectKey, contentType)
	if err != nil {
		// service 层已经带"OSS"前缀,直接传出
		common.ApiErrorMsg(c, err.Error())
		return
	}

	common.ApiSuccess(c, gin.H{"url": publicURL})
}

// GetPoster GET /api/poster
//
// 公开接口(任何用户/匿名都能拉),返回海报弹窗的 3 个字段。
// 前端 Home/index.jsx 在 mount 时调用,根据返回决定弹海报还是回退到现有公告。
func GetPoster(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(200, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":   common.EnablePoster,
			"image_url": common.PosterImageUrl,
			"click_url": common.PosterClickUrl,
		},
	})
}
