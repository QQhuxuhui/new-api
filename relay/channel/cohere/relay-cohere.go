package cohere

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func requestOpenAI2Cohere(textRequest dto.GeneralOpenAIRequest) *CohereRequest {
	cohereReq := CohereRequest{
		Model:       textRequest.Model,
		ChatHistory: []ChatHistory{},
		Message:     "",
		Stream:      textRequest.Stream,
		MaxTokens:   textRequest.GetMaxTokens(),
	}
	if common.CohereSafetySetting != "NONE" {
		cohereReq.SafetyMode = common.CohereSafetySetting
	}
	if cohereReq.MaxTokens == 0 {
		cohereReq.MaxTokens = 4000
	}
	for _, msg := range textRequest.Messages {
		if msg.Role == "user" {
			cohereReq.Message = msg.StringContent()
		} else {
			var role string
			if msg.Role == "assistant" {
				role = "CHATBOT"
			} else if msg.Role == "system" {
				role = "SYSTEM"
			} else {
				role = "USER"
			}
			cohereReq.ChatHistory = append(cohereReq.ChatHistory, ChatHistory{
				Role:    role,
				Message: msg.StringContent(),
			})
		}
	}

	return &cohereReq
}

func requestConvertRerank2Cohere(rerankRequest dto.RerankRequest) *CohereRerankRequest {
	if rerankRequest.TopN == 0 {
		rerankRequest.TopN = 1
	}
	cohereReq := CohereRerankRequest{
		Query:           rerankRequest.Query,
		Documents:       rerankRequest.Documents,
		Model:           rerankRequest.Model,
		TopN:            rerankRequest.TopN,
		ReturnDocuments: true,
	}
	return &cohereReq
}

func stopReasonCohere2OpenAI(reason string) string {
	switch reason {
	case "COMPLETE":
		return "stop"
	case "MAX_TOKENS":
		return "max_tokens"
	default:
		return reason
	}
}

func cohereStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	responseId := helper.GetResponseID(c)
	createdTime := common.GetTimestamp()
	usage := &dto.Usage{}
	responseText := ""
	helper.ApplyStreamIdleTimeout(c, resp, info) // 渠道级/全局流式空闲超时（原始扫描循环不经过 StreamScannerHandler）
	streamCtx, cancelStream := context.WithCancel(c.Request.Context())
	scanDone := make(chan struct{})
	defer func() {
		cancelStream()
		service.CloseResponseBodyGracefully(resp)
		<-scanDone
	}()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := strings.Index(string(data), "\n"); i >= 0 {
			return i + 1, data[0:i], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	dataChan := make(chan string)
	go func() {
		defer close(scanDone)
		defer close(dataChan)
		for scanner.Scan() {
			data := scanner.Text()
			select {
			case dataChan <- data:
			case <-streamCtx.Done():
				return
			}
		}
	}()
	isFirst := true
	renderData := func(data string) bool {
		if isFirst {
			isFirst = false
			info.FirstResponseTime = time.Now()
		}
		data = strings.TrimSuffix(data, "\r")
		var cohereResp CohereResponse
		err := json.Unmarshal([]byte(data), &cohereResp)
		if err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			return true
		}
		var openaiResp dto.ChatCompletionsStreamResponse
		openaiResp.Id = responseId
		openaiResp.Created = createdTime
		openaiResp.Object = "chat.completion.chunk"
		openaiResp.Model = info.UpstreamModelName
		if cohereResp.IsFinished {
			finishReason := stopReasonCohere2OpenAI(cohereResp.FinishReason)
			openaiResp.Choices = []dto.ChatCompletionsStreamResponseChoice{
				{
					Delta:        dto.ChatCompletionsStreamResponseChoiceDelta{},
					Index:        0,
					FinishReason: &finishReason,
				},
			}
			if cohereResp.Response != nil {
				usage.PromptTokens = cohereResp.Response.Meta.BilledUnits.InputTokens
				usage.CompletionTokens = cohereResp.Response.Meta.BilledUnits.OutputTokens
			}
		} else {
			openaiResp.Choices = []dto.ChatCompletionsStreamResponseChoice{
				{
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						Role:    "assistant",
						Content: &cohereResp.Text,
					},
					Index: 0,
				},
			}
			responseText += cohereResp.Text
		}
		jsonStr, err := json.Marshal(openaiResp)
		if err != nil {
			common.SysLog("error marshalling stream response: " + err.Error())
			return true
		}
		helper.MarkPayloadWritten(c)
		c.Render(-1, &common.CustomEvent{Data: "data: " + string(jsonStr)})
		return true
	}

	// 提交响应头之前先等首个事件：c.Stream 每次回调后都会 Flush，一旦进入
	// 就等于提交了 200，首包超时后再返回 504 也无法切换渠道了。
	// 此处尚未写出任何字节，可以安全地返回错误交给外层重试。
	var firstData string
	gotFirst := false
	sourceDone := false
	for !gotFirst && !sourceDone {
		select {
		case data, ok := <-dataChan:
			if !ok {
				sourceDone = true
				continue
			}
			data = strings.TrimSuffix(data, "\r")
			var probe CohereResponse
			if err := json.Unmarshal([]byte(data), &probe); err != nil {
				common.SysLog("error unmarshalling stream response: " + err.Error())
				continue
			}
			firstData, gotFirst = data, true
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
		if usage.PromptTokens == 0 {
			usage = service.ResponseText2Usage(responseText, info.UpstreamModelName, info.PromptTokens)
		}
		return usage, nil
	}

	helper.SetEventStreamHeaders(c)
	pendingFirst := true
	clientGone := c.Stream(func(w io.Writer) bool {
		if pendingFirst {
			pendingFirst = false
			return renderData(firstData)
		}
		select {
		case data, ok := <-dataChan:
			if !ok {
				if !helper.MidStreamTimeoutOccurred(c) {
					c.Render(-1, &common.CustomEvent{Data: "data: [DONE]"})
				}
				return false
			}
			return renderData(data)
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
	if usage.PromptTokens == 0 {
		usage = service.ResponseText2Usage(responseText, info.UpstreamModelName, info.PromptTokens)
	}
	return usage, nil
}

func cohereHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	createdTime := common.GetTimestamp()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.CloseResponseBodyGracefully(resp)
	var cohereResp CohereResponseResult
	err = json.Unmarshal(responseBody, &cohereResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	usage := dto.Usage{}
	usage.PromptTokens = cohereResp.Meta.BilledUnits.InputTokens
	usage.CompletionTokens = cohereResp.Meta.BilledUnits.OutputTokens
	usage.TotalTokens = cohereResp.Meta.BilledUnits.InputTokens + cohereResp.Meta.BilledUnits.OutputTokens

	var openaiResp dto.TextResponse
	openaiResp.Id = cohereResp.ResponseId
	openaiResp.Created = createdTime
	openaiResp.Object = "chat.completion"
	openaiResp.Model = info.UpstreamModelName
	openaiResp.Usage = usage

	openaiResp.Choices = []dto.OpenAITextResponseChoice{
		{
			Index:        0,
			Message:      dto.Message{Content: cohereResp.Text, Role: "assistant"},
			FinishReason: stopReasonCohere2OpenAI(cohereResp.FinishReason),
		},
	}

	jsonResponse, err := json.Marshal(openaiResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)
	return &usage, nil
}

func cohereRerankHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.CloseResponseBodyGracefully(resp)
	var cohereResp CohereRerankResponseResult
	err = json.Unmarshal(responseBody, &cohereResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	usage := dto.Usage{}
	if cohereResp.Meta.BilledUnits.InputTokens == 0 {
		usage.PromptTokens = info.PromptTokens
		usage.CompletionTokens = 0
		usage.TotalTokens = info.PromptTokens
	} else {
		usage.PromptTokens = cohereResp.Meta.BilledUnits.InputTokens
		usage.CompletionTokens = cohereResp.Meta.BilledUnits.OutputTokens
		usage.TotalTokens = cohereResp.Meta.BilledUnits.InputTokens + cohereResp.Meta.BilledUnits.OutputTokens
	}

	var rerankResp dto.RerankResponse
	rerankResp.Results = cohereResp.Results
	rerankResp.Usage = usage

	jsonResponse, err := json.Marshal(rerankResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return &usage, nil
}
