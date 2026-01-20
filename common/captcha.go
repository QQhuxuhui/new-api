package common

import (
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/slide"
)

var captchaBuilder *slide.Captcha

// InitCaptcha 初始化验证码生成器
func InitCaptcha() error {
	builder, err := slide.NewBuilder(
		slide.WithRangeLen(option.RangeVal{Min: 4, Max: 6}),
		slide.WithRangeVerifyLen(option.RangeVal{Min: 2, Max: 4}),
	)
	if err != nil {
		return fmt.Errorf("failed to create captcha builder: %w", err)
	}

	captchaBuilder = builder
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
	if captchaBuilder == nil {
		return nil, fmt.Errorf("captcha builder not initialized")
	}

	// 生成验证码
	captchaData, err := captchaBuilder.Make()
	if err != nil {
		return nil, fmt.Errorf("failed to generate captcha: %w", err)
	}

	// 获取图片数据
	bgImage := captchaData.GetMasterImage()
	sliderImage := captchaData.GetTileImage()

	// 转换为 base64
	bgBase64 := base64.StdEncoding.EncodeToString(bgImage.ToBytes())
	sliderBase64 := base64.StdEncoding.EncodeToString(sliderImage.ToBytes())

	// 生成唯一 ID
	captchaID := uuid.New().String()

	// 获取正确的 X 坐标
	correctX := captchaData.GetThumbX()

	// 存储答案
	StoreCaptchaAnswer(captchaID, correctX)

	return &CaptchaResponse{
		CaptchaID:       captchaID,
		BackgroundImage: "data:image/png;base64," + bgBase64,
		SliderImage:     "data:image/png;base64," + sliderBase64,
		SliderY:         captchaData.GetThumbY(),
	}, nil
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
