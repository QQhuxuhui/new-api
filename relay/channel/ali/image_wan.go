package ali

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type WanImageInput struct {
	Prompt         string   `json:"prompt"`                    // 必需：文本提示词，描述生成图像中期望包含的元素和视觉特点
	Images         []string `json:"images"`                    // 必需：图像URL数组，长度不超过2，支持HTTP/HTTPS URL或Base64编码
	NegativePrompt string   `json:"negative_prompt,omitempty"` // 可选：反向提示词，描述不希望在画面中看到的内容
}

type WanImageParameters struct {
	N         int     `json:"n,omitempty"`         // 生成图片数量，取值范围1-4，默认4
	Watermark *bool   `json:"watermark,omitempty"` // 是否添加水印标识，默认false
	Seed      int     `json:"seed,omitempty"`      // 随机数种子，取值范围[0, 2147483647]
	Strength  float64 `json:"strength,omitempty"`  // 修改幅度 0.0-1.0，默认0.5（部分模型支持）
}

func oaiFormEdit2WanxImageEdit(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (*AliImageRequest, error) {
	var err error
	var imageRequest AliImageRequest
	imageRequest.Model = request.Model
	imageRequest.ResponseFormat = request.ResponseFormat
	wanInput := WanImageInput{
		Prompt: request.Prompt,
	}

	if err := common.UnmarshalBodyReusable(c, &wanInput); err != nil {
		return nil, err
	}
	if wanInput.Images, err = getImageBase64sFromForm(c, "image"); err != nil {
		return nil, fmt.Errorf("get image base64s from form failed: %w", err)
	}
	imageRequest.Input = wanInput
	n := int(request.N)
	if n <= 0 {
		n = 1
	}
	imageRequest.Parameters = AliImageParameters{
		N: n,
	}
	info.PriceData.AddOtherRatio("n", float64(n))

	return &imageRequest, nil
}

func getImageBase64sFromForm(c *gin.Context, fieldName string) ([]string, error) {
	mf := c.Request.MultipartForm
	if mf == nil {
		if _, err := c.MultipartForm(); err != nil {
			return nil, fmt.Errorf("failed to parse image edit form request: %w", err)
		}
		mf = c.Request.MultipartForm
	}

	var imageFiles []*multipart.FileHeader
	var exists bool

	// First check for standard "image" field
	if imageFiles, exists = mf.File[fieldName]; !exists || len(imageFiles) == 0 {
		// If not found, check for "image[]" field
		if imageFiles, exists = mf.File[fieldName+"[]"]; !exists || len(imageFiles) == 0 {
			// If still not found, iterate through all fields to find any that start with "image["
			foundArrayImages := false
			for name, files := range mf.File {
				if strings.HasPrefix(name, fieldName+"[") && len(files) > 0 {
					foundArrayImages = true
					imageFiles = append(imageFiles, files...)
				}
			}

			// If no image fields found at all
			if !foundArrayImages && (len(imageFiles) == 0) {
				return nil, errors.New("image is required")
			}
		}
	}

	if len(imageFiles) == 0 {
		return nil, errors.New("image is required")
	}

	// 获取base64编码的图片
	var imageBase64s []string
	for _, file := range imageFiles {
		image, err := file.Open()
		if err != nil {
			return nil, errors.New("failed to open image file")
		}

		// 读取文件内容
		imageData, err := io.ReadAll(image)
		if err != nil {
			image.Close()
			return nil, errors.New("failed to read image file")
		}

		// 获取MIME类型
		mimeType := http.DetectContentType(imageData)

		// 编码为base64
		base64Data := base64.StdEncoding.EncodeToString(imageData)

		// 构造data URL格式
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)
		imageBase64s = append(imageBase64s, dataURL)
		image.Close()
	}
	return imageBase64s, nil
}

func isOldWanModel(modelName string) bool {
	return strings.Contains(modelName, "wan") &&
		!lo.SomeBy([]string{"wan2.6", "wan2.7"}, func(v string) bool { return strings.Contains(modelName, v) })
}

func isWanModel(modelName string) bool {
	return strings.Contains(modelName, "wan")
}
