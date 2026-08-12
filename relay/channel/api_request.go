package channel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	common2 "github.com/QuantumNous/new-api/common"
	constant2 "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// debugPrintHeaders 打印请求头（调试用）
func debugPrintHeaders(prefix string, headers http.Header) {
	if !common2.DebugEnabled {
		return
	}
	println(fmt.Sprintf("\n========== %s ==========", prefix))
	// 按字母顺序排序 header keys
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range headers[k] {
			// 对敏感信息脱敏
			displayValue := v
			lowerKey := strings.ToLower(k)
			if strings.Contains(lowerKey, "key") || strings.Contains(lowerKey, "authorization") || strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "token") {
				if len(v) > 12 {
					displayValue = v[:6] + "****" + v[len(v)-4:]
				} else if len(v) > 4 {
					displayValue = v[:2] + "****"
				} else {
					displayValue = "****"
				}
			}
			println(fmt.Sprintf("  %s: %s", k, displayValue))
		}
	}
	println("==========================================\n")
}

// debugPrintBody 打印请求体（调试用，限制长度）
func debugPrintBody(prefix string, body []byte) {
	if !common2.DebugEnabled {
		return
	}
	println(fmt.Sprintf("\n========== %s ==========", prefix))
	bodyStr := string(body)
	// 限制输出长度，避免打印过长的内容
	maxLen := 2000
	if len(bodyStr) > maxLen {
		println(bodyStr[:maxLen])
		println(fmt.Sprintf("... [truncated, total %d bytes]", len(bodyStr)))
	} else {
		println(bodyStr)
	}
	println("==========================================\n")
}

// debugPrintClientRequest 打印客户端请求信息
func debugPrintClientRequest(c *gin.Context) {
	if !common2.DebugEnabled {
		return
	}
	println("\n##################################################")
	println("########## [DEBUG] CLIENT REQUEST INFO ##########")
	println("##################################################")
	println(fmt.Sprintf("Method: %s", c.Request.Method))
	println(fmt.Sprintf("URL: %s", c.Request.URL.String()))
	println(fmt.Sprintf("RemoteAddr: %s", c.ClientIP()))
	debugPrintHeaders("CLIENT REQUEST HEADERS", c.Request.Header)
}

// debugPrintUpstreamRequest 打印上游请求信息
func debugPrintUpstreamRequest(req *http.Request, body []byte) {
	if !common2.DebugEnabled {
		return
	}
	println("\n##################################################")
	println("########## [DEBUG] UPSTREAM REQUEST INFO ##########")
	println("##################################################")
	println(fmt.Sprintf("Method: %s", req.Method))
	println(fmt.Sprintf("URL: %s", req.URL.String()))
	debugPrintHeaders("UPSTREAM REQUEST HEADERS", req.Header)
	if body != nil {
		debugPrintBody("UPSTREAM REQUEST BODY", body)
	}
}

// debugPrintUpstreamResponse 打印上游响应信息
func debugPrintUpstreamResponse(resp *http.Response, body []byte) {
	if !common2.DebugEnabled {
		return
	}
	println("\n##################################################")
	println("########## [DEBUG] UPSTREAM RESPONSE INFO ##########")
	println("##################################################")
	println(fmt.Sprintf("Status: %s", resp.Status))
	println(fmt.Sprintf("StatusCode: %d", resp.StatusCode))
	debugPrintHeaders("UPSTREAM RESPONSE HEADERS", resp.Header)
	if body != nil {
		debugPrintBody("UPSTREAM RESPONSE BODY", body)
	}
}

func SetupApiRequestHeader(info *common.RelayInfo, c *gin.Context, req *http.Header) {
	if info.RelayMode == constant.RelayModeAudioTranscription || info.RelayMode == constant.RelayModeAudioTranslation {
		// multipart/form-data
	} else if info.RelayMode == constant.RelayModeRealtime {
		// websocket
	} else {
		req.Set("Content-Type", c.Request.Header.Get("Content-Type"))
		req.Set("Accept", c.Request.Header.Get("Accept"))
		if info.IsStream && c.Request.Header.Get("Accept") == "" {
			req.Set("Accept", "text/event-stream")
		}
	}

	// Transparent proxy: forward the client's real User-Agent to upstream.
	// If the client omits it we leave Go's default (Go-http-client/1.1) so the
	// request clearly originates from new-api rather than spoofing an identity.
	if ua := c.Request.Header.Get("User-Agent"); ua != "" {
		req.Set("User-Agent", ua)
	}
}

const clientHeaderPlaceholderPrefix = "{client_header:"

const (
	headerPassthroughAllKey        = "*"
	headerPassthroughRegexPrefix   = "re:"
	headerPassthroughRegexPrefixV2 = "regex:"
)

// passthroughSkipHeaderNamesLower 列出永远不参与名字匹配透传的请求头。
var passthroughSkipHeaderNamesLower = map[string]struct{}{
	// RFC 7230 逐跳（hop-by-hop）请求头。
	"connection":          {},
	"proxy-connection":    {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},

	"cookie": {},

	// 由出站请求自行决定，透传会破坏上游连接。
	"host":            {},
	"content-length":  {},
	"accept-encoding": {},

	// 凭证绝不能被通配/正则规则带到上游。这里必须覆盖所有 adaptor 会用
	// info.ApiKey 填充的头，否则客户端可以用同名头顶掉渠道密钥：
	//   authorization  —— 绝大多数 adaptor
	//   api-key        —— Azure（openai/adaptor.go）
	//   appid          —— 百度千帆 v2
	//   x-api-key      —— Claude
	//   x-goog-api-key —— Gemini / PaLM
	//   x-goog-user-project —— Vertex 配额/计费项目
	"authorization":  {},
	"api-key":        {},
	"appid":          {},
	"x-api-key":      {},
	"x-goog-api-key": {},
	// Vertex 的配额/计费项目同样属于渠道身份，不能由客户端通配覆盖。
	"x-goog-user-project": {},

	// WebSocket 握手头由 dialer 生成；Sec-WebSocket-Protocol 在 OpenAI Realtime
	// 里承载 openai-insecure-api-key.<渠道密钥>，透传会顶掉鉴权子协议。
	"sec-websocket-key":        {},
	"sec-websocket-version":    {},
	"sec-websocket-extensions": {},
	"sec-websocket-protocol":   {},
}

// dynamicHopByHopHeaders 解析 Connection / Proxy-Connection 中声明的逐跳头。
// RFC 7230 §6.1 允许在这两个头里动态列出仅限本跳的字段名，转发时必须剥掉，
// 否则客户端可以借 "Connection: X-Hop" 把任意字段标成逐跳又照样送到上游。
func dynamicHopByHopHeaders(h http.Header) map[string]struct{} {
	if len(h) == 0 {
		return nil
	}
	var names map[string]struct{}
	for _, headerName := range []string{"Connection", "Proxy-Connection"} {
		for _, value := range h.Values(headerName) {
			for _, token := range strings.Split(value, ",") {
				token = strings.ToLower(strings.TrimSpace(token))
				// "close" / "keep-alive" 是连接指令而非字段名，忽略即可。
				if token == "" || token == "close" || token == "keep-alive" {
					continue
				}
				if names == nil {
					names = make(map[string]struct{})
				}
				names[token] = struct{}{}
			}
		}
	}
	return names
}

var headerPassthroughRegexCache sync.Map // map[string]*regexp.Regexp

func getHeaderPassthroughRegex(pattern string) (*regexp.Regexp, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, errors.New("empty regex pattern")
	}
	if v, ok := headerPassthroughRegexCache.Load(pattern); ok {
		if re, ok := v.(*regexp.Regexp); ok {
			return re, nil
		}
		headerPassthroughRegexCache.Delete(pattern)
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	actual, _ := headerPassthroughRegexCache.LoadOrStore(pattern, compiled)
	if re, ok := actual.(*regexp.Regexp); ok {
		return re, nil
	}
	return compiled, nil
}

// IsHeaderPassthroughRuleKey 判断 header_override 的某个 key 是否为透传规则而非普通覆盖。
func IsHeaderPassthroughRuleKey(key string) bool {
	return isHeaderPassthroughRuleKey(key)
}

func isHeaderPassthroughRuleKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if key == headerPassthroughAllKey {
		return true
	}
	lower := strings.ToLower(key)
	return strings.HasPrefix(lower, headerPassthroughRegexPrefix) || strings.HasPrefix(lower, headerPassthroughRegexPrefixV2)
}

func shouldSkipPassthroughHeader(name string, dynamicHop map[string]struct{}) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	lower := strings.ToLower(name)
	if _, ok := passthroughSkipHeaderNamesLower[lower]; ok {
		return true
	}
	_, ok := dynamicHop[lower]
	return ok
}

// applyHeaderOverridePlaceholders 解析单条 header_override 的值。
// 返回的 bool 表示该头是否应当写入出站请求（取不到客户端头/空值时为 false）。
func applyHeaderOverridePlaceholders(template string, c *gin.Context, apiKey string) (string, bool, error) {
	trimmed := strings.TrimSpace(template)
	if strings.HasPrefix(trimmed, clientHeaderPlaceholderPrefix) {
		afterPrefix := trimmed[len(clientHeaderPlaceholderPrefix):]
		end := strings.Index(afterPrefix, "}")
		if end < 0 || end != len(afterPrefix)-1 {
			return "", false, fmt.Errorf("client_header placeholder must be the full value: %q", template)
		}

		name := strings.TrimSpace(afterPrefix[:end])
		if name == "" {
			return "", false, fmt.Errorf("client_header placeholder name is empty: %q", template)
		}
		if c == nil || c.Request == nil {
			return "", false, fmt.Errorf("missing request context for client_header placeholder")
		}
		clientHeaderValue := c.Request.Header.Get(name)
		if strings.TrimSpace(clientHeaderValue) == "" {
			return "", false, nil
		}
		// 客户端可控内容里含 CR/LF 一律丢弃，不当成错误：这是客户端输入，
		// 报错会让正常请求失败，而带换行的头值只会被传输层拒绝。
		if strings.ContainsAny(clientHeaderValue, "\r\n") {
			return "", false, nil
		}
		// 客户端提供的内容里不做 {api_key} 插值，避免把渠道密钥回填进客户端可控字符串。
		return clientHeaderValue, true, nil
	}

	if strings.Contains(template, "{api_key}") {
		// 多 Key 渠道的密钥块以换行分隔，调用方应当只传本次使用的那一个；
		// 这里再 Trim 一次兜底，避免把换行带进请求头值。
		template = strings.ReplaceAll(template, "{api_key}", strings.TrimSpace(apiKey))
	}
	// 静态配置里出现 CR/LF 属于配置错误，明确报出来，
	// 否则只会在传输层得到一句难以定位的 invalid header field value。
	if strings.ContainsAny(template, "\r\n") {
		return "", false, fmt.Errorf("header override value must not contain CR/LF: %q", template)
	}
	// 显式空值是有意义的配置：{"Authorization": ""} 用来把 adaptor 设置的凭证
	// 覆盖成空，历史行为一直如此。只有 {client_header:...} 取不到值时才跳过，
	// 那种情况下写入空头等于凭空造了一个客户端没发的字段。
	return template, true, nil
}

// processHeaderOverride 处理请求头覆盖，支持变量替换与客户端请求头透传。
//
// 支持的变量：
//   - {api_key}：替换为渠道 API Key
//   - {client_header:<name>}：取客户端同名请求头的值
//
// 透传规则（只看 key，value 忽略）：
//   - "*"：按名字透传全部客户端请求头（跳过逐跳头与凭证头）
//   - "re:<regex>" / "regex:<regex>"：透传名字匹配该 Go 正则的请求头
//
// 透传先应用、普通覆盖后应用，因此显式覆盖优先级更高。
// 渠道测试没有真实客户端请求，透传与 {client_header:...} 一律跳过。
// 返回 http.Header 而非 map[string]string：同名多值的请求头（Accept、
// anthropic-beta 等）必须原样带过去，压成单值会丢信息。
func processHeaderOverride(info *common.RelayInfo, c *gin.Context) (http.Header, error) {
	headerOverride := make(http.Header)
	// ChannelMeta 是内嵌指针，未 InitChannelMeta 时访问 HeadersOverride 会 panic。
	if info == nil || info.ChannelMeta == nil {
		return headerOverride, nil
	}

	// 没有真实客户端请求时（渠道测试、拉取上游模型列表等带外调用），
	// 透传规则和 {client_header:...} 都无从取值，一律跳过；普通静态覆盖照常生效。
	hasClientRequest := c != nil && c.Request != nil
	skipClientSignals := !hasClientRequest || common2.GetContextKeyBool(c, constant2.ContextKeyChannelTest)

	// [DEBUG] 打印 HeadersOverride 配置
	if common2.DebugEnabled {
		println(fmt.Sprintf("[DEBUG] HeadersOverride count: %d", len(info.HeadersOverride)))
		for k, v := range info.HeadersOverride {
			println(fmt.Sprintf("[DEBUG] HeadersOverride[%s] = %v (type: %T)", k, v, v))
		}
	}

	passAll := false
	var passthroughRegex []*regexp.Regexp
	if !skipClientSignals {
		for k := range info.HeadersOverride {
			trimmedKey := strings.TrimSpace(k)
			if trimmedKey == "" {
				continue
			}
			lowerKey := strings.ToLower(trimmedKey)
			if lowerKey == headerPassthroughAllKey {
				passAll = true
				continue
			}

			// 只用小写形式识别前缀，正则本体保留原始大小写：
			// 请求头名字在 http.Header 里是规范化的（X-App），若把模式一起小写，
			// "re:^X-" 会变成 "^x-" 而永远匹配不上。
			var pattern string
			switch {
			case strings.HasPrefix(lowerKey, headerPassthroughRegexPrefix):
				pattern = strings.TrimSpace(trimmedKey[len(headerPassthroughRegexPrefix):])
			case strings.HasPrefix(lowerKey, headerPassthroughRegexPrefixV2):
				pattern = strings.TrimSpace(trimmedKey[len(headerPassthroughRegexPrefixV2):])
			default:
				continue
			}

			if pattern == "" {
				return nil, types.NewError(fmt.Errorf("header passthrough regex pattern is empty: %q", k), types.ErrorCodeChannelHeaderOverrideInvalid)
			}
			compiled, err := getHeaderPassthroughRegex(pattern)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeChannelHeaderOverrideInvalid)
			}
			passthroughRegex = append(passthroughRegex, compiled)
		}
	}

	if passAll || len(passthroughRegex) > 0 {
		dynamicHop := dynamicHopByHopHeaders(c.Request.Header)
		for name := range c.Request.Header {
			if shouldSkipPassthroughHeader(name, dynamicHop) {
				continue
			}
			if !passAll {
				matched := false
				lowerName := strings.ToLower(name)
				for _, re := range passthroughRegex {
					// 同时按规范名（X-App）和全小写名（x-app）匹配，
					// 两种写法的模式都能用，不必强制写 (?i)。
					if re.MatchString(name) || re.MatchString(lowerName) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			values := c.Request.Header.Values(name)
			kept := make([]string, 0, len(values))
			for _, v := range values {
				if strings.TrimSpace(v) != "" {
					kept = append(kept, v)
				}
			}
			if len(kept) == 0 {
				continue
			}
			headerOverride.Del(name)
			for _, v := range kept {
				headerOverride.Add(name, v)
			}
		}
	}

	for k, v := range info.HeadersOverride {
		if isHeaderPassthroughRuleKey(k) {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(k))
		if key == "" {
			continue
		}

		str, ok := v.(string)
		if !ok {
			return nil, types.NewError(nil, types.ErrorCodeChannelHeaderOverrideInvalid)
		}
		if skipClientSignals && strings.HasPrefix(strings.TrimSpace(str), clientHeaderPlaceholderPrefix) {
			continue
		}

		value, include, err := applyHeaderOverridePlaceholders(str, c, info.ApiKey)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeChannelHeaderOverrideInvalid)
		}
		if !include {
			continue
		}

		// Set 会替换透传阶段收集到的同名多值，保证「显式覆盖优先」
		headerOverride.Set(key, value)

		// [DEBUG] 打印应用的头覆盖
		if common2.DebugEnabled {
			displayValue := value
			if strings.Contains(key, "key") || strings.Contains(key, "authorization") || strings.Contains(key, "secret") || strings.Contains(key, "token") {
				if len(value) > 12 {
					displayValue = value[:6] + "****" + value[len(value)-4:]
				} else if len(value) > 4 {
					displayValue = value[:2] + "****"
				} else {
					displayValue = "****"
				}
			}
			println(fmt.Sprintf("[DEBUG] Applying header override: %s = %s", key, displayValue))
		}
	}
	return headerOverride, nil
}

// ResolveHeaderOverride 暴露给包外解析最终生效的 header_override。
func ResolveHeaderOverride(info *common.RelayInfo, c *gin.Context) (http.Header, error) {
	return processHeaderOverride(info, c)
}

// SignedHeaderGuard 由自行计算请求签名的适配器实现，声明参与 canonical 签名
// 计算的请求头名字。
//
// 这些字段不允许被 header_override（含客户端请求头透传）改写：签名在适配器的
// SetupRequestHeader / BuildRequestHeader 里就算好了，之后再改这些头会让请求与
// 签名对不上，上游只会返回鉴权失败——覆盖不仅没生效，还把渠道打挂了。
//
// 自建出站请求、能自己控制顺序的适配器（如即梦）不需要实现本接口：
// 在签名之前应用覆盖即可，那样覆盖真正生效且签名依然有效。
type SignedHeaderGuard interface {
	SignedHeaderNames() []string
}

// applyHeaderOverrideRespectingSignature 应用覆盖，但保护签名字段不被改写。
func applyHeaderOverrideRespectingSignature(adaptor any, req *http.Request, headerOverride http.Header) {
	guard, ok := adaptor.(SignedHeaderGuard)
	if !ok {
		applyHeaderOverrideToRequest(req, headerOverride)
		return
	}
	names := guard.SignedHeaderNames()
	if len(names) == 0 || req == nil {
		applyHeaderOverrideToRequest(req, headerOverride)
		return
	}

	saved := make(map[string][]string, len(names))
	protectsHost := false
	for _, n := range names {
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(n))
		if canonical == "" {
			continue
		}
		if strings.EqualFold(canonical, "Host") {
			protectsHost = true
		}
		if existing, exists := req.Header[canonical]; exists {
			saved[canonical] = append([]string(nil), existing...)
		} else {
			saved[canonical] = nil
		}
	}
	savedHost := req.Host

	applyHeaderOverrideToRequest(req, headerOverride)

	for canonical, values := range saved {
		if values == nil {
			req.Header.Del(canonical)
			continue
		}
		req.Header[canonical] = values
	}
	if protectsHost {
		req.Host = savedHost
	}
}

// ApplyHeaderOverrideToRequest 供包外（自建出站请求、绕过 DoApiRequest 的适配器）使用。
func ApplyHeaderOverrideToRequest(req *http.Request, headerOverride http.Header) {
	applyHeaderOverrideToRequest(req, headerOverride)
}

// applyHeaderOverrideToRequest 把解析后的覆盖写入出站请求；Host 需要同时写 req.Host 才会生效。
func applyHeaderOverrideToRequest(req *http.Request, headerOverride http.Header) {
	if req == nil {
		return
	}
	for key, values := range headerOverride {
		if len(values) == 0 {
			continue
		}
		req.Header.Del(key)
		for _, v := range values {
			req.Header.Add(key, v)
		}
		if strings.EqualFold(key, "Host") {
			req.Host = values[0]
		}
	}
}

func DoApiRequest(a Adaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	// [DEBUG] 打印客户端请求信息
	debugPrintClientRequest(c)

	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	if common2.DebugEnabled {
		println("[DEBUG] Upstream URL:", fullRequestURL)
	}

	// 读取请求体用于调试（需要重新包装）
	// 注意：仅在 DEBUG 模式下读取，避免无谓的内存拷贝/GC 压力
	var bodyBytes []byte
	if common2.DebugEnabled && requestBody != nil {
		bodyBytes, err = io.ReadAll(requestBody)
		if err != nil {
			return nil, fmt.Errorf("read request body failed: %w", err)
		}
		requestBody = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	headers := req.Header

	// Step 1: 适配器设置请求头
	err = a.SetupRequestHeader(c, &headers, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}

	// Step 2: 应用渠道的 header_override（最高优先级，可覆盖适配器设置的值）
	headerOverride, err := processHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	applyHeaderOverrideRespectingSignature(a, req, headerOverride)

	// [DEBUG] 打印上游请求信息（在设置完所有 header 之后）
	debugPrintUpstreamRequest(req, bodyBytes)

	resp, err := doRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}

	// [DEBUG] 打印上游响应头信息（响应体在流式处理中打印）
	if common2.DebugEnabled && resp != nil {
		debugPrintUpstreamResponse(resp, nil)
	}

	return resp, nil
}

func DoFormRequest(a Adaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	// [DEBUG] 打印客户端请求信息
	debugPrintClientRequest(c)

	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	if common2.DebugEnabled {
		println("[DEBUG] Upstream URL:", fullRequestURL)
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	// set form data
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	headers := req.Header

	// Step 1: 适配器设置请求头
	err = a.SetupRequestHeader(c, &headers, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}

	// Step 2: 应用渠道的 header_override（最高优先级，可覆盖适配器设置的值）
	headerOverride, err := processHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	applyHeaderOverrideRespectingSignature(a, req, headerOverride)

	// [DEBUG] 打印上游请求信息
	debugPrintUpstreamRequest(req, nil)

	resp, err := doRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}

	// [DEBUG] 打印上游响应头信息
	if common2.DebugEnabled && resp != nil {
		debugPrintUpstreamResponse(resp, nil)
	}

	return resp, nil
}

func DoWssRequest(a Adaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*websocket.Conn, error) {
	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	targetHeader := http.Header{}

	// Step 1: 适配器设置请求头
	err = a.SetupRequestHeader(c, &targetHeader, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}

	// Step 2: 应用渠道的 header_override（最高优先级，可覆盖适配器设置的值）
	headerOverride, err := processHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for key, values := range headerOverride {
		targetHeader.Del(key)
		for _, v := range values {
			targetHeader.Add(key, v)
		}
	}

	targetHeader.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	targetConn, _, err := websocket.DefaultDialer.Dial(fullRequestURL, targetHeader)
	if err != nil {
		return nil, fmt.Errorf("dial failed to %s: %w", fullRequestURL, err)
	}
	// send request body
	//all, err := io.ReadAll(requestBody)
	//err = service.WssString(c, targetConn, string(all))
	return targetConn, nil
}

func startPingKeepAlive(c *gin.Context, pingInterval time.Duration) context.CancelFunc {
	pingerCtx, stopPinger := context.WithCancel(context.Background())

	gopool.Go(func() {
		defer func() {
			// 增加panic恢复处理
			if r := recover(); r != nil {
				if common2.DebugEnabled {
					println("SSE ping goroutine panic recovered:", fmt.Sprintf("%v", r))
				}
			}
			if common2.DebugEnabled {
				println("SSE ping goroutine stopped.")
			}
		}()

		if pingInterval <= 0 {
			pingInterval = helper.DefaultPingInterval
		}

		ticker := time.NewTicker(pingInterval)
		// 确保在任何情况下都清理ticker
		defer func() {
			ticker.Stop()
			if common2.DebugEnabled {
				println("SSE ping ticker stopped")
			}
		}()

		var pingMutex sync.Mutex
		if common2.DebugEnabled {
			println("SSE ping goroutine started")
		}

		// 增加超时控制，防止goroutine长时间运行
		maxPingDuration := 120 * time.Minute // 最大ping持续时间
		pingTimeout := time.NewTimer(maxPingDuration)
		defer pingTimeout.Stop()

		for {
			select {
			// 发送 ping 数据
			case <-ticker.C:
				if err := sendPingData(c, &pingMutex); err != nil {
					if common2.DebugEnabled {
						println("SSE ping error, stopping goroutine:", err.Error())
					}
					return
				}
			// 收到退出信号
			case <-pingerCtx.Done():
				return
			// request 结束
			case <-c.Request.Context().Done():
				return
			// 超时保护，防止goroutine无限运行
			case <-pingTimeout.C:
				if common2.DebugEnabled {
					println("SSE ping goroutine timeout, stopping")
				}
				return
			}
		}
	})

	return stopPinger
}

func sendPingData(c *gin.Context, mutex *sync.Mutex) error {
	// 增加超时控制，防止锁死等待
	done := make(chan error, 1)
	go func() {
		mutex.Lock()
		defer mutex.Unlock()

		err := helper.PingData(c)
		if err != nil {
			logger.LogError(c, "SSE ping error: "+err.Error())
			done <- err
			return
		}

		if common2.DebugEnabled {
			println("SSE ping data sent.")
		}
		done <- nil
	}()

	// 设置发送ping数据的超时时间
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		return errors.New("SSE ping data send timeout")
	case <-c.Request.Context().Done():
		return errors.New("request context cancelled during ping")
	}
}

func DoRequest(c *gin.Context, req *http.Request, info *common.RelayInfo) (*http.Response, error) {
	return doRequest(c, req, info)
}
func doRequest(c *gin.Context, req *http.Request, info *common.RelayInfo) (*http.Response, error) {
	var client *http.Client
	var err error
	if info.ChannelSetting.Proxy != "" {
		client, err = service.NewProxyHttpClient(info.ChannelSetting.Proxy)
		if err != nil {
			return nil, fmt.Errorf("new proxy http client failed: %w", err)
		}
	} else {
		client = service.GetHttpClient()
	}

	// Bind upstream request lifecycle to downstream request context:
	// - client disconnect / reverse proxy timeout should cancel upstream ASAP
	// This is critical to avoid goroutine/connection buildup when upstream is slow.
	if c != nil && c.Request != nil {
		// 如果下游 context 已经取消（客户端断开），直接返回不可重试的错误，
		// 避免重试循环中所有请求因复用已取消的 context 而瞬间失败。
		if err := c.Request.Context().Err(); err != nil {
			return nil, types.NewError(
				fmt.Errorf("downstream context already canceled: %w", err),
				types.ErrorCodeContextCanceled,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithHideErrMsg("client disconnected"),
			)
		}
		req = req.WithContext(c.Request.Context())
	}

	var stopPinger context.CancelFunc
	if info.IsStream {
		helper.SetEventStreamHeaders(c)
		// 处理流式请求的 ping 保活
		generalSettings := operation_setting.GetGeneralSetting()
		if generalSettings.PingIntervalEnabled && !info.DisablePing {
			pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
			stopPinger = startPingKeepAlive(c, pingInterval)
			// 使用defer确保在任何情况下都能停止ping goroutine
			defer func() {
				if stopPinger != nil {
					stopPinger()
					if common2.DebugEnabled {
						println("SSE ping goroutine stopped by defer")
					}
				}
			}()
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		// 区分"客户端断开导致 context 取消"和"上游真正故障"：
		// 如果下游 context 已取消，说明是客户端断开（超时/主动取消），
		// 不应归咎于渠道，也不应记录为渠道失败。
		if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
			return nil, types.NewError(
				fmt.Errorf("request canceled due to client disconnect: %w", err),
				types.ErrorCodeContextCanceled,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithHideErrMsg("client disconnected"),
			)
		}
		logger.LogError(c, "do request failed: "+err.Error())
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithHideErrMsg("upstream error: do request failed"))
	}
	if resp == nil {
		return nil, errors.New("resp is nil")
	}

	_ = req.Body.Close()
	_ = c.Request.Body.Close()
	return resp, nil
}

func DoTaskApiRequest(a TaskAdaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.BuildRequestURL(info)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(requestBody), nil
	}

	err = a.BuildRequestHeader(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}

	// 与 DoApiRequest 保持一致：任务渠道（Suno/Jimeng/Doubao/Gemini 等）同样
	// 需要 header_override 的静态覆盖、占位符与客户端请求头透传。
	headerOverride, err := processHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	applyHeaderOverrideRespectingSignature(a, req, headerOverride)

	resp, err := doRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}
