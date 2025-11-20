package service

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

// ConcurrencyLeakThreshold 并发泄漏判断阈值（默认 2 分钟）
// 如果并发计数存在超过此时间，认为是泄漏
var ConcurrencyLeakThreshold = 2 * time.Minute

// loadLeakThresholdFromEnv 从环境变量加载阈值配置
// 必须在 .env 加载后调用，因此放在 StartConcurrencyCleanupTask 中而非 init()
func loadLeakThresholdFromEnv() {
	// 支持通过环境变量配置阈值（单位：秒）
	if thresholdStr := os.Getenv("CONCURRENCY_LEAK_THRESHOLD"); thresholdStr != "" {
		if seconds, err := strconv.Atoi(thresholdStr); err == nil && seconds > 0 {
			ConcurrencyLeakThreshold = time.Duration(seconds) * time.Second
		}
	}
}

// StartConcurrencyCleanupTask 启动定期清理任务，清理可能泄漏的并发计数
// 该任务会定期检查并发计数的最后活动时间，防止因服务崩溃导致的泄漏
func StartConcurrencyCleanupTask() {
	if !common.RedisEnabled {
		return
	}

	// 从环境变量加载阈值配置（在 .env 加载之后）
	loadLeakThresholdFromEnv()

	// 清理间隔设为泄漏阈值的一半，确保及时发现泄漏
	// 例如：阈值 2 分钟，则每 1 分钟扫描一次
	cleanupInterval := ConcurrencyLeakThreshold / 2
	if cleanupInterval < 30*time.Second {
		cleanupInterval = 30 * time.Second // 最小间隔 30 秒
	}

	ticker := time.NewTicker(cleanupInterval)
	go func() {
		// 启动时立即执行一次
		cleanupStaleConcurrency()

		for range ticker.C {
			cleanupStaleConcurrency()
		}
	}()

	common.SysLog(fmt.Sprintf("Concurrency cleanup task started (interval: %v, threshold: %v)", cleanupInterval, ConcurrencyLeakThreshold))
}

// cleanupStaleConcurrency 清理可疑的并发计数
// 策略：检查时间戳 key（最后活动时间），如果距今超过阈值，认为是泄漏并清理
func cleanupStaleConcurrency() {
	ctx := context.Background()

	cursor := uint64(0)
	cleanedCount := 0
	scannedCount := 0
	now := time.Now().Unix()

	// 移除整体超时限制，确保能扫描完所有 key
	for {
		// 单次 Scan 操作有合理的超时
		scanCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		keys, nextCursor, err := common.RDB.Scan(scanCtx, cursor, "channel:key:*:concurrent", 100).Result()
		cancel()

		if err != nil {
			if common.DebugEnabled {
				common.SysLog(fmt.Sprintf("Concurrency cleanup scan error: %v", err))
			}
			break
		}

		for _, key := range keys {
			scannedCount++

			// 跳过时间戳 key
			if len(key) > 10 && key[len(key)-10:] == ":timestamp" {
				continue
			}

			// 获取当前并发数
			getCtx, getCancel := context.WithTimeout(ctx, 1*time.Second)
			val, err := common.RDB.Get(getCtx, key).Result()
			getCancel()

			if err == redis.Nil {
				continue
			}
			if err != nil {
				continue
			}

			count, err := strconv.Atoi(val)
			if err != nil || count <= 0 {
				// 无效或为 0 的计数，删除主 key 和时间戳 key
				timestampKey := GetConcurrencyTimestampKey(key)
				common.RDB.Del(ctx, key, timestampKey)
				continue
			}

			// 获取时间戳
			timestampKey := GetConcurrencyTimestampKey(key)
			tsCtx, tsCancel := context.WithTimeout(ctx, 1*time.Second)
			timestampStr, err := common.RDB.Get(tsCtx, timestampKey).Result()
			tsCancel()

			if err == redis.Nil {
				// 时间戳不存在，这是旧版本遗留的 key 或异常情况
				// 由于无法判断最后活动时间，直接删除以保证数据一致性
				common.SysLog(fmt.Sprintf("Cleaned concurrency key without timestamp (legacy or corrupted): key=%s, count=%d", key, count))
				common.RDB.Del(ctx, key, timestampKey)
				cleanedCount++
				continue
			}
			if err != nil {
				continue
			}

			timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
			if err != nil {
				continue
			}

			// 检查是否超过泄漏阈值
			age := time.Duration(now-timestamp) * time.Second
			if age > ConcurrencyLeakThreshold {
				// 这是一个泄漏的并发计数，删除它和时间戳
				delCtx, delCancel := context.WithTimeout(ctx, 1*time.Second)
				err := common.RDB.Del(delCtx, key, timestampKey).Err()
				delCancel()

				if err == nil {
					cleanedCount++
					common.SysLog(fmt.Sprintf("Cleaned leaked concurrency: key=%s, count=%d, age=%v", key, count, age))
				}
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if cleanedCount > 0 {
		common.SysLog(fmt.Sprintf("Concurrency cleanup completed: scanned=%d, cleaned=%d", scannedCount, cleanedCount))
	} else if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Concurrency cleanup completed: scanned=%d, no leaked keys found", scannedCount))
	}
}
