package openai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// imagesEditsInboundContentTypeKey 缓存首次进入转换时的入站 Content-Type。
// 转换会把请求头改写成出站 multipart，重试时必须依据该缓存值判断入站格式。
const imagesEditsInboundContentTypeKey = "images_edits_inbound_content_type"

const (
	// 上游 gpt-image-1 系列单请求最多 16 张图，超出必然被拒，提前挡下避免无谓解码
	maxImageSourcesPerRequest = 16
	// 累计解码预算倍数：JSON body、RawMessage、解码字节和 multipart buffer 会同时
	// 驻留内存，全部图片+mask 解码总量限制在 N 张满额以内，防止放大成 OOM
	maxTotalDecodedImageMultiple = 4
)

// maxDecodedImageMB 单张图片解码后的 MB 上限，与 URL 下载共用
// MAX_FILE_DOWNLOAD_MB（默认 20MB），保持两条入口限制一致；
// 未经 common/init 初始化时（如单测）回退到同一默认值
func maxDecodedImageMB() int64 {
	if constant.MaxFileDownloadMB > 0 {
		return int64(constant.MaxFileDownloadMB)
	}
	return 20
}

func maxDecodedImageBytes() int64 {
	return maxDecodedImageMB() * 1024 * 1024
}

// MaxImagesEditsJSONBodyBytes JSON 版 images/edits 允许的最大请求体：
// 解码预算的 base64 体积（4/3 倍）加少量 JSON 结构余量。
// 中间件在第一次读取请求体之前就以此为上限（http.MaxBytesReader），
// 超大请求在进入内存前即被 413 拒绝；转换器内的同值检查作为第二道防线。
func MaxImagesEditsJSONBodyBytes() int64 {
	return maxDecodedImageBytes()*maxTotalDecodedImageMultiple*4/3 + (8 << 20)
}

// writeEditsFormFromJSON 支持客户端用 application/json 调 /v1/images/edits。
// 上游只接受 multipart/form-data，这里把 JSON 体里的 image/mask
// （data URL、裸 base64 或 http(s) 链接）还原成文件字段后再转发。
// 调用方已写入 model 字段，本函数负责其余所有字段。
func writeEditsFormFromJSON(c *gin.Context, request dto.ImageRequest, writer *multipart.Writer) error {
	body, err := common.GetRequestBody(c)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// 以下均为确定性的客户端输入校验，失败带 ClientInputError 标记
	// （relay 层 400 + SkipRetry，不跨渠道重试）。
	// 第二道防线（第一道是中间件的 http.MaxBytesReader）：超过解码预算对应
	// 体积的请求直接拒绝，不进入 RawMessage/解码字节/multipart buffer 的转换流程
	if maxBody := MaxImagesEditsJSONBodyBytes(); int64(len(body)) > maxBody {
		return types.NewClientInputError(fmt.Errorf("request body too large for images/edits JSON conversion: %d bytes exceeds %d", len(body), maxBody))
	}

	var raw map[string]json.RawMessage
	if err := common.Unmarshal(body, &raw); err != nil {
		return types.NewClientInputError(fmt.Errorf("images/edits requires multipart/form-data or a JSON body: %w", err))
	}

	images, err := collectJSONImageSources(raw)
	if err != nil {
		return types.NewClientInputError(err)
	}
	if len(images) == 0 {
		return types.NewClientInputError(errors.New("image is required"))
	}
	if len(images) > maxImageSourcesPerRequest {
		return types.NewClientInputError(fmt.Errorf("too many images: got %d, at most %d are allowed", len(images), maxImageSourcesPerRequest))
	}

	// 非文件字段原样透传，model 由调用方写入，图片字段单独处理
	skipped := map[string]bool{"model": true, "mask": true}
	for _, key := range imageSourceKeys {
		skipped[key] = true
	}
	for key, value := range raw {
		if skipped[key] {
			continue
		}
		field, ok := jsonValueToFormField(value)
		if !ok {
			continue
		}
		if err := writer.WriteField(key, field); err != nil {
			return fmt.Errorf("write form field %q failed: %w", key, err)
		}
	}
	// prompt 走模型映射后可能只存在于 request 上
	if _, ok := raw["prompt"]; !ok && request.Prompt != "" {
		if err := writer.WriteField("prompt", request.Prompt); err != nil {
			return fmt.Errorf("write form field \"prompt\" failed: %w", err)
		}
	}

	// 全部图片 + mask 共享的累计解码预算
	remainingBudget := maxDecodedImageBytes() * maxTotalDecodedImageMultiple

	fieldName := "image"
	if len(images) > 1 {
		fieldName = "image[]"
	}
	for i, source := range images {
		if err := writeImagePartFromSource(writer, fieldName, fmt.Sprintf("image_%d", i), source, &remainingBudget); err != nil {
			return fmt.Errorf("failed to attach image %d: %w", i, err)
		}
	}

	// mask 非空但无法识别时立即报错，静默丢弃会让编辑范围不符合用户预期
	if maskRaw, ok := raw["mask"]; ok && !isEmptyJSONValue(maskRaw) {
		masks, err := extractImageSources(maskRaw, 0)
		if err != nil {
			return types.NewClientInputError(fmt.Errorf("invalid \"mask\" value: %w", err))
		}
		if len(masks) == 0 {
			return types.NewClientInputError(fmt.Errorf("unsupported \"mask\" value: %s; expect a base64/data URL/http(s) string, or an array/object containing one",
				sanitizeJSONPreview(maskRaw)))
		}
		if len(masks) > 1 {
			return types.NewClientInputError(fmt.Errorf("at most one mask is supported, got %d", len(masks)))
		}
		if err := writeImagePartFromSource(writer, "mask", "mask", masks[0], &remainingBudget); err != nil {
			return fmt.Errorf("failed to attach mask: %w", err)
		}
	}

	return nil
}

// 图片可能出现的字段名，按优先级依次尝试
var imageSourceKeys = []string{"image", "image[]", "images", "image_url", "image_urls", "input_image", "img"}

// 对象里承载图片的键，按优先级；覆盖 {url}、{b64_json}、
// {type:image_url,image_url:{url}} 等常见客户端写法
var imageSourceObjectKeys = []string{"url", "image_url", "b64_json", "base64", "data", "image", "b64", "content", "file", "value"}

const maxImageSourceDepth = 6

// collectJSONImageSources 从 JSON 体里挖出图片来源。值可以是字符串、
// 字符串数组、对象、对象数组——各家客户端写法不一，这里都接；
// 但首个非空的候选字段必须整体有效，无效项立即报错而不是静默丢弃。
func collectJSONImageSources(raw map[string]json.RawMessage) ([]string, error) {
	for _, key := range imageSourceKeys {
		value, ok := raw[key]
		if !ok || isEmptyJSONValue(value) {
			continue
		}
		sources, err := extractImageSources(value, 0)
		if err != nil {
			return nil, fmt.Errorf("invalid %q value: %w", key, err)
		}
		if len(sources) == 0 {
			// 报错带上脱敏后的实际结构，避免再靠猜
			return nil, fmt.Errorf("unsupported %q value: %s; expect a base64/data URL/http(s) string, or an array/object containing one",
				key, sanitizeJSONPreview(value))
		}
		return sources, nil
	}
	return nil, nil
}

// extractImageSources 递归抽取图片来源字符串。
// 空值（null/""/[]/{}）跳过；非空但无法识别的数组元素直接报错，
// 静默丢弃会让实际提交的图片数量不符合用户预期。
func extractImageSources(value json.RawMessage, depth int) ([]string, error) {
	if depth > maxImageSourceDepth {
		return nil, fmt.Errorf("image structure nested deeper than %d levels", maxImageSourceDepth)
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}

	switch trimmed[0] {
	case '"':
		source, ok := jsonStringValue(trimmed)
		if !ok {
			return nil, fmt.Errorf("invalid image string: %s", sanitizeJSONPreview(trimmed))
		}
		if strings.TrimSpace(source) == "" {
			return nil, nil
		}
		return []string{source}, nil
	case '[':
		var items []json.RawMessage
		if err := common.Unmarshal(trimmed, &items); err != nil {
			return nil, fmt.Errorf("invalid image array: %s", sanitizeJSONPreview(trimmed))
		}
		var sources []string
		for i, item := range items {
			got, err := extractImageSources(item, depth+1)
			if err != nil {
				return nil, fmt.Errorf("image item %d: %w", i, err)
			}
			if len(got) == 0 {
				if isEmptyJSONValue(item) {
					continue
				}
				return nil, fmt.Errorf("unsupported image item %d: %s", i, sanitizeJSONPreview(item))
			}
			sources = append(sources, got...)
		}
		return sources, nil
	case '{':
		var object map[string]json.RawMessage
		if err := common.Unmarshal(trimmed, &object); err != nil {
			return nil, fmt.Errorf("invalid image object: %s", sanitizeJSONPreview(trimmed))
		}
		for _, key := range imageSourceObjectKeys {
			nested, ok := object[key]
			if !ok || isEmptyJSONValue(nested) {
				continue
			}
			got, err := extractImageSources(nested, depth+1)
			if err != nil {
				return nil, err
			}
			if len(got) > 0 {
				return got, nil
			}
		}
		// 对象没有可识别的图片键：返回空，由上层带完整结构预览报错
		return nil, nil
	default:
		// 数字/布尔，不是图片：返回空，由上层报错
		return nil, nil
	}
}

func isEmptyJSONValue(value json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(value))
	switch trimmed {
	case "", "null", `""`, "[]", "{}":
		return true
	}
	return false
}

// sanitizeJSONPreview 输出结构预览：保留键名和层级，长字符串（base64）
// 折叠成长度，既能看清形状又不会把图片数据打进日志
func sanitizeJSONPreview(value json.RawMessage) string {
	var parsed any
	if err := common.Unmarshal(value, &parsed); err != nil {
		return "<unparseable json>"
	}
	preview, err := common.Marshal(foldLongStrings(parsed, 0))
	if err != nil {
		return "<unprintable json>"
	}
	if len(preview) > 400 {
		return string(preview[:400]) + "...(truncated)"
	}
	return string(preview)
}

func foldLongStrings(value any, depth int) any {
	if depth > maxImageSourceDepth {
		return "<...>"
	}
	switch typed := value.(type) {
	case string:
		if len(typed) > 48 {
			return fmt.Sprintf("(string, %d chars)", len(typed))
		}
		return typed
	case []any:
		folded := make([]any, 0, len(typed))
		for _, item := range typed {
			folded = append(folded, foldLongStrings(item, depth+1))
		}
		return folded
	case map[string]any:
		folded := make(map[string]any, len(typed))
		for key, item := range typed {
			folded[key] = foldLongStrings(item, depth+1)
		}
		return folded
	default:
		return typed
	}
}

func writeImagePartFromSource(writer *multipart.Writer, fieldName, baseName, source string, remainingBudget *int64) error {
	perImageLimit := maxDecodedImageBytes()
	// base64 体积先行预检：resolveImageSource 对裸 base64 会解码嗅探 mime，
	// 必须在它之前挡下超大载荷，避免为其分配解码内存（64 字节为 data URL 前缀余量）
	maxEncodedLen := (perImageLimit/3+1)*4 + 64
	if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") &&
		int64(len(source)) > maxEncodedLen {
		return types.NewClientInputError(fmt.Errorf("image %q exceeds the maximum allowed size of %d MB", baseName, maxDecodedImageMB()))
	}

	mimeType, b64Data, err := resolveImageSource(source)
	if err != nil {
		return err
	}
	if int64(len(b64Data)) > maxEncodedLen {
		return types.NewClientInputError(fmt.Errorf("image %q exceeds the maximum allowed size of %d MB", baseName, maxDecodedImageMB()))
	}
	data, err := decodeBase64Payload(b64Data)
	if err != nil {
		return err
	}
	if int64(len(data)) > perImageLimit {
		return types.NewClientInputError(fmt.Errorf("image %q exceeds the maximum allowed size of %d MB", baseName, maxDecodedImageMB()))
	}
	*remainingBudget -= int64(len(data))
	if *remainingBudget < 0 {
		return types.NewClientInputError(fmt.Errorf("total decoded image size exceeds the maximum allowed %d MB",
			maxDecodedImageMB()*maxTotalDecodedImageMultiple))
	}

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
		fieldName, baseName+imageExtFromMimeType(mimeType)))
	h.Set("Content-Type", mimeType)

	part, err := writer.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = part.Write(data)
	return err
}

// resolveImageSource 返回 (mimeType, base64 数据)。http(s) 链接走带 SSRF 防护的下载器。
// URL 下载失败（DNS/超时/远端错误）可能是临时故障，不打客户端标记，
// 保留跨渠道重试；base64 解析失败是确定性的客户端输入错误
func resolveImageSource(source string) (string, string, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		mimeType, b64Data, err := service.GetImageFromUrl(source)
		if err != nil && !types.IsClientInputError(err) {
			return "", "", types.NewNoRecordChannelHealthError(err)
		}
		return mimeType, b64Data, err
	}
	mimeType, b64Data, err := service.DecodeBase64FileData(source)
	if err != nil {
		return "", "", types.NewClientInputError(err)
	}
	return mimeType, b64Data, nil
}

// decodeBase64Payload 容忍客户端在 base64 里夹换行/空格，以及省略 padding
func decodeBase64Payload(b64Data string) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, b64Data)

	data, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		if raw, rawErr := base64.RawStdEncoding.DecodeString(cleaned); rawErr == nil {
			return raw, nil
		}
		return nil, types.NewClientInputError(fmt.Errorf("failed to decode image data: %w", err))
	}
	return data, nil
}

func imageExtFromMimeType(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func jsonStringValue(value json.RawMessage) (string, bool) {
	var s string
	if err := common.Unmarshal(value, &s); err != nil {
		return "", false
	}
	return s, true
}

// jsonValueToFormField 把标量 JSON 值转成表单字符串；嵌套结构上游 multipart 不接受，直接跳过
func jsonValueToFormField(value json.RawMessage) (string, bool) {
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || trimmed == "null" {
		return "", false
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return "", false
	}
	if s, ok := jsonStringValue(value); ok {
		return s, true
	}
	return trimmed, true
}
