package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize = 64 << 10 // 64KB (64*1024)
	MaxScannerBufferSize     = 10 << 20 // 10MB (10*1024*1024)
	DefaultPingInterval      = 10 * time.Second
	WriteTimeout             = 10 * time.Second
)

// writeTask 表示一个写任务
type writeTask struct {
	taskType   int    // 0: ping, 1: data
	data       string // 仅 data 任务使用
	resultChan chan writeResult
}

// writeResult 表示写任务的结果
type writeResult struct {
	success bool
	err     error
}

func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string) bool) {

	if resp == nil || dataHandler == nil {
		return
	}

	// 确保响应体总是被关闭
	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second

	var (
		stopChan   = make(chan bool, 3)      // 增加缓冲区避免阻塞
		scanner    = bufio.NewScanner(resp.Body)
		ticker     = time.NewTicker(streamingTimeout)
		pingTicker *time.Ticker
		taskChan   = make(chan writeTask, 10) // 任务队列
		producerWg sync.WaitGroup             // 生产者（ping + scanner）等待组
		writerWg   sync.WaitGroup             // 消费者（writer）等待组
	)

	generalSettings := operation_setting.GetGeneralSetting()
	pingEnabled := generalSettings.PingIntervalEnabled && !info.DisablePing
	pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
	if pingInterval <= 0 {
		pingInterval = DefaultPingInterval
	}

	if pingEnabled {
		pingTicker = time.NewTicker(pingInterval)
	}

	if common.DebugEnabled {
		// print timeout and ping interval for debugging
		println("relay timeout seconds:", common.RelayTimeout)
		println("streaming timeout seconds:", int64(streamingTimeout.Seconds()))
		println("ping interval seconds:", int64(pingInterval.Seconds()))
	}

	// 获取 ResponseController 用于设置写超时
	rc := http.NewResponseController(c.Writer)

	// 改进资源清理，确保所有 goroutine 正确退出
	defer func() {
		// 1. 通知所有 goroutine 停止
		common.SafeSendBool(stopChan, true)

		ticker.Stop()
		if pingTicker != nil {
			pingTicker.Stop()
		}

		// 2. 设置立即超时，强制中断任何阻塞的写操作
		_ = rc.SetWriteDeadline(time.Now())

		// 3. 等待所有生产者退出（确保不会再向 taskChan 发送）
		producerDone := make(chan struct{})
		go func() {
			producerWg.Wait()
			close(producerDone)
		}()

		select {
		case <-producerDone:
			// 生产者已全部退出
		case <-time.After(5 * time.Second):
			logger.LogError(c, "timeout waiting for producers to exit")
		}

		// 4. 关闭任务通道（此时没有生产者会发送了）
		close(taskChan)

		// 5. 等待 writer 退出
		writerDone := make(chan struct{})
		go func() {
			writerWg.Wait()
			close(writerDone)
		}()

		select {
		case <-writerDone:
			// writer 已退出
		case <-time.After(5 * time.Second):
			logger.LogError(c, "timeout waiting for writer to exit")
		}

		close(stopChan)
	}()

	scanner.Buffer(make([]byte, InitialScannerBufferSize), MaxScannerBufferSize)
	scanner.Split(bufio.ScanLines)
	SetEventStreamHeaders(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = context.WithValue(ctx, "stop_chan", stopChan)

	// Worker goroutine: 处理所有写操作
	writerWg.Add(1)
	gopool.Go(func() {
		defer func() {
			writerWg.Done()
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("writer goroutine panic: %v", r))
				common.SafeSendBool(stopChan, true)
			}
			if common.DebugEnabled {
				println("writer goroutine exited")
			}
		}()

		for {
			select {
			case task, ok := <-taskChan:
				if !ok {
					// taskChan 已关闭，退出
					return
				}

				// 为每个写操作设置超时
				deadline := time.Now().Add(WriteTimeout)
				if err := rc.SetWriteDeadline(deadline); err != nil {
					// 如果设置失败，记录但继续执行
					if common.DebugEnabled {
						println("failed to set write deadline:", err.Error())
					}
				}

				var result writeResult
				if task.taskType == 0 {
					// ping 任务
					result.err = PingData(c)
					result.success = result.err == nil
				} else {
					// data 任务
					result.success = dataHandler(task.data)
				}

				// 清除写超时（设置为零值表示无限制）
				_ = rc.SetWriteDeadline(time.Time{})

				// 非阻塞发送结果
				select {
				case task.resultChan <- result:
				default:
					// 如果没有接收者（调用方已超时），忽略
				}

			case <-stopChan:
				// 收到停止信号，排空剩余任务后退出
				for {
					select {
					case task, ok := <-taskChan:
						if !ok {
							return
						}
						// 对剩余任务返回失败
						select {
						case task.resultChan <- writeResult{success: false}:
						default:
						}
					default:
						return
					}
				}
			}
		}
	})

	// Handle ping data sending
	if pingEnabled && pingTicker != nil {
		producerWg.Add(1)
		gopool.Go(func() {
			defer func() {
				producerWg.Done()
				if r := recover(); r != nil {
					logger.LogError(c, fmt.Sprintf("ping goroutine panic: %v", r))
					common.SafeSendBool(stopChan, true)
				}
				if common.DebugEnabled {
					println("ping goroutine exited")
				}
			}()

			// 添加超时保护，防止 goroutine 无限运行
			maxPingDuration := 30 * time.Minute
			pingTimeout := time.NewTimer(maxPingDuration)
			defer pingTimeout.Stop()

			for {
				select {
				case <-pingTicker.C:
					resultChan := make(chan writeResult, 1)
					task := writeTask{
						taskType:   0,
						resultChan: resultChan,
					}

					// 发送任务（带取消检查）
					select {
					case taskChan <- task:
						// 任务发送成功
					case <-ctx.Done():
						return
					case <-stopChan:
						return
					case <-c.Request.Context().Done():
						return
					}

					// 等待结果或超时
					select {
					case result := <-resultChan:
						if result.err != nil {
							logger.LogError(c, "ping data error: "+result.err.Error())
							return
						}
						if common.DebugEnabled {
							println("ping data sent")
						}
					case <-time.After(WriteTimeout):
						logger.LogError(c, "ping data send timeout")
						return
					case <-ctx.Done():
						return
					case <-stopChan:
						return
					case <-c.Request.Context().Done():
						return
					}

				case <-ctx.Done():
					return
				case <-stopChan:
					return
				case <-c.Request.Context().Done():
					return
				case <-pingTimeout.C:
					logger.LogError(c, "ping goroutine max duration reached")
					return
				}
			}
		})
	}

	// Scanner goroutine
	producerWg.Add(1)
	common.RelayCtxGo(ctx, func() {
		defer func() {
			producerWg.Done()
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("scanner goroutine panic: %v", r))
			}
			common.SafeSendBool(stopChan, true)
			if common.DebugEnabled {
				println("scanner goroutine exited")
			}
		}()

		for scanner.Scan() {
			// 检查是否需要停止
			select {
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			case <-c.Request.Context().Done():
				return
			default:
			}

			ticker.Reset(streamingTimeout)
			data := scanner.Text()
			if common.DebugEnabled {
				println(data)
			}

			if len(data) < 5 {
				continue
			}

			// 先检查是否是裸 [DONE]（无 data: 前缀）
			trimmedData := strings.TrimSuffix(data, "\r")
			if trimmedData == "[DONE]" {
				if common.DebugEnabled {
					println("received bare [DONE], stopping scanner")
				}
				return
			}

			// 检查 data: 前缀
			if !strings.HasPrefix(data, "data:") {
				continue
			}

			// 移除 "data:" 前缀并处理
			data = data[5:]
			data = strings.TrimLeft(data, " ")
			data = strings.TrimSuffix(data, "\r")

			// 检查是否是 data: [DONE]
			if data == "[DONE]" || strings.HasPrefix(data, "[DONE]") {
				if common.DebugEnabled {
					println("received [DONE], stopping scanner")
				}
				return
			}

			info.SetFirstResponseTime()

			// 创建结果通道并发送任务
			resultChan := make(chan writeResult, 1)
			task := writeTask{
				taskType:   1,
				data:       data,
				resultChan: resultChan,
			}

			// 发送任务到 worker（带取消检查）
			select {
			case taskChan <- task:
				// 任务发送成功
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			case <-c.Request.Context().Done():
				return
			}

			// 等待结果或超时
			select {
			case result := <-resultChan:
				if !result.success {
					return
				}
			case <-time.After(WriteTimeout):
				logger.LogError(c, "data handler timeout")
				return
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			case <-c.Request.Context().Done():
				return
			}
		}

		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				logger.LogError(c, "scanner error: "+err.Error())
			}
		}
	})

	// 主循环等待完成或超时
	select {
	case <-ticker.C:
		// 超时处理逻辑
		logger.LogError(c, "streaming timeout")
	case <-stopChan:
		// 正常结束
		logger.LogInfo(c, "streaming finished")
	case <-c.Request.Context().Done():
		// 客户端断开连接
		logger.LogInfo(c, "client disconnected")
	}
}
