package zhipu

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// https://open.bigmodel.cn/doc/api#chatglm_std
// chatglm_std, chatglm_lite
// https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_std/invoke
// https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_std/sse-invoke

var zhipuTokens sync.Map
var expSeconds int64 = 24 * 3600

func getZhipuToken(apikey string) string {
	data, ok := zhipuTokens.Load(apikey)
	if ok {
		tokenData := data.(zhipuTokenData)
		if time.Now().Before(tokenData.ExpiryTime) {
			return tokenData.Token
		}
	}

	split := strings.Split(apikey, ".")
	if len(split) != 2 {
		common.SysLog("invalid zhipu key: " + apikey)
		return ""
	}

	id := split[0]
	secret := split[1]

	expMillis := time.Now().Add(time.Duration(expSeconds)*time.Second).UnixNano() / 1e6
	expiryTime := time.Now().Add(time.Duration(expSeconds) * time.Second)

	timestamp := time.Now().UnixNano() / 1e6

	payload := jwt.MapClaims{
		"api_key":   id,
		"exp":       expMillis,
		"timestamp": timestamp,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)

	token.Header["alg"] = "HS256"
	token.Header["sign_type"] = "SIGN"

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return ""
	}

	zhipuTokens.Store(apikey, zhipuTokenData{
		Token:      tokenString,
		ExpiryTime: expiryTime,
	})

	return tokenString
}

func requestOpenAI2Zhipu(request dto.GeneralOpenAIRequest) *ZhipuRequest {
	messages := make([]ZhipuMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.Role == "system" {
			messages = append(messages, ZhipuMessage{
				Role:    "system",
				Content: message.StringContent(),
			})
			messages = append(messages, ZhipuMessage{
				Role:    "user",
				Content: "Okay",
			})
		} else {
			messages = append(messages, ZhipuMessage{
				Role:    message.Role,
				Content: message.StringContent(),
			})
		}
	}
	return &ZhipuRequest{
		Prompt:      messages,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Incremental: false,
	}
}

func responseZhipu2OpenAI(response *ZhipuResponse) *dto.OpenAITextResponse {
	fullTextResponse := dto.OpenAITextResponse{
		Id:      response.Data.TaskId,
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Choices: make([]dto.OpenAITextResponseChoice, 0, len(response.Data.Choices)),
		Usage:   response.Data.Usage,
	}
	for i, choice := range response.Data.Choices {
		openaiChoice := dto.OpenAITextResponseChoice{
			Index: i,
			Message: dto.Message{
				Role:    choice.Role,
				Content: strings.Trim(choice.Content, "\""),
			},
			FinishReason: "",
		}
		if i == len(response.Data.Choices)-1 {
			openaiChoice.FinishReason = "stop"
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, openaiChoice)
	}
	return &fullTextResponse
}

func streamResponseZhipu2OpenAI(zhipuResponse string) *dto.ChatCompletionsStreamResponse {
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString(zhipuResponse)
	response := dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "chatglm",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}

func streamMetaResponseZhipu2OpenAI(zhipuResponse *ZhipuStreamMetaResponse) (*dto.ChatCompletionsStreamResponse, *dto.Usage) {
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString("")
	choice.FinishReason = &constant.FinishReasonStop
	response := dto.ChatCompletionsStreamResponse{
		Id:      zhipuResponse.RequestId,
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "chatglm",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response, &zhipuResponse.Usage
}

func zhipuStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	var usage *dto.Usage
	helper.ApplyStreamIdleTimeout(c, resp, info) // 渠道级/全局流式空闲超时（原始扫描循环不经过 StreamScannerHandler）
	streamCtx, cancelStream := context.WithCancel(c.Request.Context())
	scanDone := make(chan struct{})
	defer func() {
		cancelStream()
		service.CloseResponseBodyGracefully(resp)
		<-scanDone
	}()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	type streamEvent struct {
		data   string
		isMeta bool
	}
	eventChan := make(chan streamEvent)
	go func() {
		defer close(scanDone)
		defer close(eventChan)
		sendEvent := func(event streamEvent) bool {
			select {
			case eventChan <- event:
				return true
			case <-streamCtx.Done():
				return false
			}
		}
		for scanner.Scan() {
			data := scanner.Text()
			lines := strings.Split(data, "\n")
			for i, line := range lines {
				if len(line) < 5 {
					continue
				}
				if line[:5] == "data:" {
					if !sendEvent(streamEvent{data: line[5:]}) {
						return
					}
					if i != len(lines)-1 {
						if !sendEvent(streamEvent{data: "\n"}) {
							return
						}
					}
				} else if line[:5] == "meta:" {
					if !sendEvent(streamEvent{data: line[5:], isMeta: true}) {
						return
					}
				}
			}
		}
	}()
	renderData := func(data string) bool {
		response := streamResponseZhipu2OpenAI(data)
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			common.SysLog("error marshalling stream response: " + err.Error())
			return true
		}
		helper.MarkPayloadWritten(c)
		c.Render(-1, &common.CustomEvent{Data: "data: " + string(jsonResponse)})
		return true
	}
	renderMeta := func(data string) bool {
		var zhipuResponse ZhipuStreamMetaResponse
		err := json.Unmarshal([]byte(data), &zhipuResponse)
		if err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			return true
		}
		response, zhipuUsage := streamMetaResponseZhipu2OpenAI(&zhipuResponse)
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			common.SysLog("error marshalling stream response: " + err.Error())
			return true
		}
		usage = zhipuUsage
		helper.MarkPayloadWritten(c)
		c.Render(-1, &common.CustomEvent{Data: "data: " + string(jsonResponse)})
		return true
	}

	// 提交响应头之前先等首个事件：c.Stream 每次回调后都会 Flush，一旦进入
	// 就等于提交了 200，首包超时后再返回 504 也无法切换渠道了
	var firstEvent streamEvent
	gotFirst := false
	sourceDone := false
	for !gotFirst && !sourceDone {
		select {
		case event, ok := <-eventChan:
			if !ok {
				sourceDone = true
				continue
			}
			if event.isMeta {
				var probe ZhipuStreamMetaResponse
				if err := json.Unmarshal([]byte(event.data), &probe); err != nil {
					common.SysLog("error unmarshalling stream response: " + err.Error())
					continue
				}
			}
			firstEvent, gotFirst = event, true
		case <-streamCtx.Done():
			return nil, helper.NewEmptyStreamError(helper.StreamExitClientGone)
		}
	}
	if !gotFirst {
		if timeoutErr := helper.RawStreamFirstByteTimeoutError(c); timeoutErr != nil {
			return nil, timeoutErr
		}
		// 上游未发任何事件即正常结束：保持既有收尾语义
		helper.SetEventStreamHeaders(c)
		c.Render(-1, &common.CustomEvent{Data: "data: [DONE]"})
		return usage, nil
	}

	helper.SetEventStreamHeaders(c)
	pendingFirst := true
	clientGone := c.Stream(func(w io.Writer) bool {
		if pendingFirst {
			pendingFirst = false
			if firstEvent.isMeta {
				return renderMeta(firstEvent.data)
			}
			return renderData(firstEvent.data)
		}
		select {
		case event, ok := <-eventChan:
			if !ok {
				if !helper.MidStreamTimeoutOccurred(c) {
					c.Render(-1, &common.CustomEvent{Data: "data: [DONE]"})
				}
				return false
			}
			if event.isMeta {
				return renderMeta(event.data)
			}
			return renderData(event.data)
		case <-streamCtx.Done():
			return false
		}
	})
	cancelStream()
	if clientGone || c.Request.Context().Err() != nil {
		return nil, helper.NewEmptyStreamError(helper.StreamExitClientGone)
	}
	// 首包超时按空流 504 报错（可切换渠道）
	if timeoutErr := helper.RawStreamFirstByteTimeoutError(c); timeoutErr != nil {
		return nil, timeoutErr
	}
	return usage, nil
}

func zhipuHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	var zhipuResponse ZhipuResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	err = json.Unmarshal(responseBody, &zhipuResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if !zhipuResponse.Success {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: zhipuResponse.Msg,
			Code:    zhipuResponse.Code,
		}, resp.StatusCode)
	}
	fullTextResponse := responseZhipu2OpenAI(&zhipuResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return &fullTextResponse.Usage, nil
}
