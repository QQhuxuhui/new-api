package types

import "errors"

// ClientInputError 标记明确源于客户端输入的转换错误（如 JSON 图片编辑里的
// 坏 base64、图片过多/超限、无效 mask）。relay 层据此返回 400 + SkipRetry；
// 未标记的转换错误（渠道能力不支持、配置问题、转换期上游临时故障，如
// replicate 上传图片失败、mistral 不支持图片请求）保持可故障转移语义，
// 允许切换到其他可用渠道。
type ClientInputError struct {
	Err error
}

func (e *ClientInputError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ClientInputError) Unwrap() error {
	return e.Err
}

// NewClientInputError 包装 err 为客户端输入错误；nil 原样返回
func NewClientInputError(err error) error {
	if err == nil {
		return nil
	}
	return &ClientInputError{Err: err}
}

// IsClientInputError 判断错误链上是否带客户端输入标记
func IsClientInputError(err error) bool {
	var target *ClientInputError
	return errors.As(err, &target)
}
