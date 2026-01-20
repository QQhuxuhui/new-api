package common

import (
	"fmt"
	"sync"

	"github.com/admpub/go-captcha-assets/resources/images"
	"github.com/admpub/go-captcha-assets/resources/tiles"
	"github.com/google/uuid"
	"github.com/wenlng/go-captcha/v2/slide"
)

var captchaBuilder slide.Captcha
var captchaInitMu sync.Mutex

// InitCaptcha 初始化验证码生成器
func InitCaptcha() error {
	// 获取背景图片
	bgImages, err := images.GetImages()
	if err != nil {
		return fmt.Errorf("failed to load background images: %w", err)
	}

	// 获取滑块图片
	tileGraphs, err := tiles.GetTiles()
	if err != nil {
		return fmt.Errorf("failed to load tile images: %w", err)
	}

	// 转换为 slide.GraphImage 格式
	var graphs []*slide.GraphImage
	for _, graph := range tileGraphs {
		graphs = append(graphs, &slide.GraphImage{
			OverlayImage: graph.OverlayImage,
			MaskImage:    graph.MaskImage,
			ShadowImage:  graph.ShadowImage,
		})
	}

	// 创建 builder 并设置资源
	builder := slide.NewBuilder()
	builder.SetResources(
		slide.WithBackgrounds(bgImages),
		slide.WithGraphImages(graphs),
	)

	// 创建 captcha 实例
	captchaBuilder = builder.Make()
	return nil
}

// CaptchaResponse 验证码响应数据
type CaptchaResponse struct {
	CaptchaID       string `json:"captcha_id"`
	BackgroundImage string `json:"background_image"`
	SliderImage     string `json:"slider_image"`
	SliderY         int    `json:"slider_y"`
}

// GenerateCaptcha 生成滑动验证码
func GenerateCaptcha() (*CaptchaResponse, error) {
	if !CaptchaEnabled {
		return nil, fmt.Errorf("captcha is disabled")
	}
	if err := ensureCaptchaInitialized(); err != nil {
		return nil, err
	}
	// 生成验证码
	captchaData, err := captchaBuilder.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate captcha: %w", err)
	}

	// 获取图片数据
	bgImage := captchaData.GetMasterImage()
	sliderImage := captchaData.GetTileImage()

	// 转换为 base64
	bgBase64, err := bgImage.ToBase64()
	if err != nil {
		return nil, fmt.Errorf("failed to encode background image: %w", err)
	}
	sliderBase64, err := sliderImage.ToBase64()
	if err != nil {
		return nil, fmt.Errorf("failed to encode slider image: %w", err)
	}

	// 生成唯一 ID
	captchaID := uuid.New().String()

	// 获取验证数据
	blockData := captchaData.GetData()
	if blockData == nil {
		return nil, fmt.Errorf("failed to get captcha data")
	}

	// 存储答案
	StoreCaptchaAnswer(captchaID, blockData.X)

	return &CaptchaResponse{
		CaptchaID:       captchaID,
		BackgroundImage: bgBase64,
		SliderImage:     sliderBase64,
		SliderY:         blockData.Y,
	}, nil
}

func ensureCaptchaInitialized() error {
	if captchaBuilder != nil {
		return nil
	}
	captchaInitMu.Lock()
	defer captchaInitMu.Unlock()
	if captchaBuilder != nil {
		return nil
	}
	return InitCaptcha()
}

// VerifyCaptcha 验证滑动验证码
func VerifyCaptcha(captchaID string, userX int) (bool, error) {
	// 获取正确答案
	correctX, exists := GetCaptchaAnswer(captchaID)
	if !exists {
		return false, fmt.Errorf("captcha not found or expired")
	}

	// 删除答案（防止重复验证）
	DeleteCaptchaAnswer(captchaID)

	// 验证坐标（允许 ±5px 误差）
	tolerance := 5
	if userX >= correctX-tolerance && userX <= correctX+tolerance {
		return true, nil
	}

	return false, nil
}
