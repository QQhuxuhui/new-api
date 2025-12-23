package channel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	common2 "github.com/QuantumNous/new-api/common"
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

	// Pass through User-Agent from client, or use default if not provided
	userAgent := c.Request.Header.Get("User-Agent")
	if userAgent == "" {
		userAgent = common2.DefaultUserAgent
	}
	if userAgent != "" {
		req.Set("User-Agent", userAgent)
	}
}

// processHeaderOverride 处理请求头覆盖，支持变量替换
// 支持的变量：{api_key}
func processHeaderOverride(info *common.RelayInfo) (map[string]string, error) {
	headerOverride := make(map[string]string)
	for k, v := range info.HeadersOverride {
		str, ok := v.(string)
		if !ok {
			return nil, types.NewError(nil, types.ErrorCodeChannelHeaderOverrideInvalid)
		}

		// 替换支持的变量
		if strings.Contains(str, "{api_key}") {
			str = strings.ReplaceAll(str, "{api_key}", info.ApiKey)
		}

		headerOverride[k] = str
	}
	return headerOverride, nil
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
	var bodyBytes []byte
	if requestBody != nil {
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
	headerOverride, err := processHeaderOverride(info)
	if err != nil {
		return nil, err
	}
	for key, value := range headerOverride {
		headers.Set(key, value)
	}
	err = a.SetupRequestHeader(c, &headers, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}

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
	headerOverride, err := processHeaderOverride(info)
	if err != nil {
		return nil, err
	}
	for key, value := range headerOverride {
		headers.Set(key, value)
	}
	err = a.SetupRequestHeader(c, &headers, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}

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
	headerOverride, err := processHeaderOverride(info)
	if err != nil {
		return nil, err
	}
	for key, value := range headerOverride {
		targetHeader.Set(key, value)
	}
	err = a.SetupRequestHeader(c, &targetHeader, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
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
	resp, err := doRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}
