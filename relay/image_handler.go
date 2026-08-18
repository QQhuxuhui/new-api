package relay

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

func ImageHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	imageReq, ok := info.Request.(*dto.ImageRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.ImageRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(imageReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ImageRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError)
	}

	// 上一轮（同一请求的前一个渠道）若做过 edits 降档，改的是跨重试共享的
	// multipart 表单 / KeyRequestBody 缓存体，controller 的重试回卷不认识它们。
	// 这里无条件先恢复到原始尺寸，再决定本渠道要不要重新降档——否则重试到无
	// 超分规则的渠道时上游会收到陈旧的降档尺寸（4K 计费拿 1K 图）。
	restoreEditsRequestSize(c)

	upscaler := service.GetImageUpscaler()
	var upscalePlan *imageUpscalePlan
	if upscaler != nil {
		upscalePlan = resolveImageUpscalePlan(c, info, request.Size)
	}
	if upscalePlan != nil {
		// edits 的两条出站转换路径都不读 request.Size，须改写各自的真实数据源
		// （multipart 表单 / JSON 缓存体）。改写不了就整体放弃超分：此时
		// request.Size 也保持原样，绝不会出现"降档发出、回程不放大"的少给。
		if downgradeEditsRequestSize(c, info, upscalePlan) {
			request.Size = upscalePlan.DowngradedSize
		} else {
			upscalePlan = nil
		}
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	var requestBody io.Reader

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
		convertedRequest, err := adaptor.ConvertImageRequest(c, info, *request)
		if err != nil {
			return newImageConversionError(err)
		}

		switch convertedRequest.(type) {
		case *bytes.Buffer:
			requestBody = convertedRequest.(io.Reader)
		default:
			jsonData, err := common.Marshal(convertedRequest)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			// apply param override
			if len(info.ParamOverride) > 0 {
				jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride)
				if err != nil {
					return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid)
				}
			}

			if common.DebugEnabled {
				logger.LogDebug(c, fmt.Sprintf("image request body: %s", string(jsonData)))
			}
			requestBody = bytes.NewBuffer(jsonData)
		}
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			if httpResp.StatusCode == http.StatusCreated && info.ApiType == constant.APITypeReplicate {
				// replicate channel returns 201 Created when using Prefer: wait, treat it as success.
				httpResp.StatusCode = http.StatusOK
			} else {
				newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
				// reset status code 重置状态码
				service.ResetStatusCode(newAPIError, statusCodeMappingStr)
				return newAPIError
			}
		}
	}

	// 超分实际降级（RunPod 失败/超时/结果校验失败，返回的是降档原图）标记，
	// 落库日志据此写"超分降级"而非"超分"，避免对账时把 1K 图读成 4K。
	upscaleDegraded := false

	if upscalePlan != nil && httpResp != nil && !info.IsStream {
		upstreamBody, readErr := io.ReadAll(httpResp.Body)
		_ = httpResp.Body.Close()
		if readErr != nil {
			return types.NewOpenAIError(readErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		}
		// c.Request.Context() 已取消时超分立即失败并走降级返回原图，属预期行为。
		upscaleCtx, cancel := context.WithTimeout(c.Request.Context(), upscaler.Timeout())
		newBody, upErr := service.RewriteImageResponseWithUpscale(
			upscaleCtx, upstreamBody, upscalePlan.TargetW, upscalePlan.TargetH, upscaler.UpscaleImage)
		cancel()
		if upErr != nil {
			// 降级：返回上游原图（降档尺寸）。sub2api 按实际像素计费 ⇒ 自动按低档收，
			// 不会多收；绝不因超分失败吞掉一次已付费的生成。
			logger.LogWarn(c, fmt.Sprintf("image_upscale_degraded: %v", upErr))
			newBody = upstreamBody
			upscaleDegraded = true
		} else {
			logger.LogInfo(c, fmt.Sprintf("image_upscale_done: %s→%dx%d",
				upscalePlan.FromTier, upscalePlan.TargetW, upscalePlan.TargetH))
		}
		httpResp.Body = io.NopCloser(bytes.NewReader(newBody))
		httpResp.ContentLength = int64(len(newBody))
		httpResp.Header.Del("Content-Length")
	} else if upscaler != nil && httpResp != nil && !info.IsStream {
		// 尺寸规整：无超分 plan 时，若渠道开了 normalize 且用户请求精确 WxH，
		// 上游实际出图尺寸不符则经同一条重采样链调整到请求尺寸（缩小纯 Lanczos，
		// 放大走 ESRGAN）。尺寸一致时零额外调用；失败降级返回原图。
		if tw, th, ok := resolveImageNormalizeTarget(c, info, imageReq.Size); ok {
			upstreamBody, readErr := io.ReadAll(httpResp.Body)
			_ = httpResp.Body.Close()
			if readErr != nil {
				return types.NewOpenAIError(readErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
			}
			normCtx, cancel := context.WithTimeout(c.Request.Context(), upscaler.Timeout())
			newBody, changed, normErr := service.NormalizeImageResponseSize(
				normCtx, upstreamBody, tw, th, upscaler.UpscaleImage)
			cancel()
			if normErr != nil {
				logger.LogWarn(c, fmt.Sprintf("image_normalize_degraded: %v", normErr))
				newBody = upstreamBody
			} else if changed {
				logger.LogInfo(c, fmt.Sprintf("image_normalize_done: →%dx%d", tw, th))
			}
			httpResp.Body = io.NopCloser(bytes.NewReader(newBody))
			httpResp.ContentLength = int64(len(newBody))
			httpResp.Header.Del("Content-Length")
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	if usage.(*dto.Usage).TotalTokens == 0 {
		usage.(*dto.Usage).TotalTokens = int(request.N)
	}
	if usage.(*dto.Usage).PromptTokens == 0 {
		usage.(*dto.Usage).PromptTokens = int(request.N)
	}

	quality := "standard"
	if request.Quality == "hd" {
		quality = "hd"
	}

	var logContent string

	if len(imageReq.Size) > 0 {
		logContent = fmt.Sprintf("大小 %s, 品质 %s, 张数 %d", imageReq.Size, quality, request.N)
		if upscalePlan != nil {
			if upscaleDegraded {
				// 降级后客户拿到的是降档尺寸的原图，日志必须如实标注。
				logContent += fmt.Sprintf("（%s 超分降级）", upscalePlan.FromTier)
			} else {
				logContent += fmt.Sprintf("（%s 超分）", upscalePlan.FromTier)
			}
		}
	}

	postConsumeQuota(c, info, usage.(*dto.Usage), logContent)
	return nil
}

func newImageConversionError(err error) *types.NewAPIError {
	if types.IsClientInputError(err) {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	options := make([]types.NewAPIErrorOptions, 0, 1)
	if types.IsNoRecordChannelHealthError(err) {
		options = append(options, types.ErrOptionWithNoRecordChannelHealth())
	}
	return types.NewError(err, types.ErrorCodeConvertRequestFailed, options...)
}
