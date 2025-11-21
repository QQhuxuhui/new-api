//go:build ignore

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

// getLeakThreshold 从环境变量读取泄漏阈值，与自动清理任务保持一致
func getLeakThreshold() time.Duration {
	threshold := 2 * time.Minute // 默认 2 分钟
	if thresholdStr := os.Getenv("CONCURRENCY_LEAK_THRESHOLD"); thresholdStr != "" {
		if seconds, err := strconv.Atoi(thresholdStr); err == nil && seconds > 0 {
			threshold = time.Duration(seconds) * time.Second
		}
	}
	return threshold
}

func main() {
	// 从环境变量或参数获取 Redis 连接字符串
	redisConnString := os.Getenv("REDIS_CONN_STRING")
	if redisConnString == "" {
		redisConnString = "redis://localhost:6379"
	}

	// 解析并连接 Redis
	opt, err := redis.ParseURL(redisConnString)
	if err != nil {
		fmt.Printf("解析 Redis 连接字符串失败: %v\n", err)
		return
	}

	rdb := redis.NewClient(opt)
	ctx := context.Background()

	// 测试连接
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		fmt.Printf("Redis 连接失败: %v\n", err)
		return
	}

	fmt.Println("✓ Redis 连接成功")
	fmt.Println("==========================================")
	fmt.Println()

	// 处理命令行参数
	if len(os.Args) > 1 {
		command := os.Args[1]
		switch command {
		case "clear":
			clearAllConcurrency(ctx, rdb)
			return
		case "detail":
			showDetailedInfo(ctx, rdb)
			return
		default:
			fmt.Printf("未知命令: %s\n\n", command)
			printUsage()
			return
		}
	}

	// 默认：扫描并显示概览
	scanConcurrencyOverview(ctx, rdb)
}

func printUsage() {
	fmt.Println("用法:")
	fmt.Println("  go run scripts/diagnose_concurrency.go           - 显示并发概览")
	fmt.Println("  go run scripts/diagnose_concurrency.go detail    - 显示详细信息（含时间戳）")
	fmt.Println("  go run scripts/diagnose_concurrency.go clear     - 清理所有并发计数")
	fmt.Println()
	fmt.Println("环境变量:")
	fmt.Println("  REDIS_CONN_STRING - Redis 连接字符串（默认: redis://localhost:6379）")
}

func scanConcurrencyOverview(ctx context.Context, rdb *redis.Client) {
	fmt.Println("扫描所有并发计数...")
	fmt.Println()

	cursor := uint64(0)
	totalKeys := 0
	keysWithConcurrency := 0

	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, "channel:key:*:concurrent", 100).Result()
		if err != nil {
			fmt.Printf("扫描失败: %v\n", err)
			break
		}

		for _, key := range keys {
			// 跳过时间戳 key
			if strings.HasSuffix(key, ":timestamp") {
				continue
			}

			totalKeys++
			val, err := rdb.Get(ctx, key).Result()
			if err != nil {
				continue
			}

			count, err := strconv.Atoi(val)
			if err != nil || count <= 0 {
				continue
			}

			keysWithConcurrency++
			// 获取 TTL
			ttl, _ := rdb.TTL(ctx, key).Result()

			fmt.Printf("Key: %s\n", key)
			fmt.Printf("  当前并发数: %d\n", count)
			fmt.Printf("  TTL: %v\n", ttl)
			fmt.Println()
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	fmt.Println("==========================================")
	fmt.Printf("总共扫描: %d 个并发 key\n", totalKeys)
	fmt.Printf("当前有并发: %d 个 key\n", keysWithConcurrency)
	fmt.Println()

	if keysWithConcurrency > 0 {
		fmt.Println("建议操作:")
		fmt.Println("1. 使用 'detail' 命令查看详细信息（含创建时间）:")
		fmt.Println("   go run scripts/diagnose_concurrency.go detail")
		fmt.Println("2. 如果确认这些并发数据是泄漏的，可以使用 'clear' 命令清理:")
		fmt.Println("   go run scripts/diagnose_concurrency.go clear")
	}
}

func showDetailedInfo(ctx context.Context, rdb *redis.Client) {
	fmt.Println("扫描并发计数（详细模式）...")
	fmt.Println()

	cursor := uint64(0)
	totalKeys := 0
	keysWithConcurrency := 0
	now := time.Now().Unix()
	leakThreshold := getLeakThreshold() // 从环境变量读取阈值

	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, "channel:key:*:concurrent", 100).Result()
		if err != nil {
			fmt.Printf("扫描失败: %v\n", err)
			break
		}

		for _, key := range keys {
			// 跳过时间戳 key
			if strings.HasSuffix(key, ":timestamp") {
				continue
			}

			totalKeys++
			val, err := rdb.Get(ctx, key).Result()
			if err != nil {
				continue
			}

			count, err := strconv.Atoi(val)
			if err != nil || count <= 0 {
				continue
			}

			keysWithConcurrency++

			// 获取时间戳
			timestampKey := key + ":timestamp"
			timestampStr, err := rdb.Get(ctx, timestampKey).Result()

			var ageStr string
			var leakWarning string
			if err == nil {
				timestamp, _ := strconv.ParseInt(timestampStr, 10, 64)
				age := time.Duration(now-timestamp) * time.Second
				ageStr = fmt.Sprintf("%v", age)

				// 使用配置的阈值判断是否可能泄漏
				if age > leakThreshold {
					leakWarning = " ⚠️  可能泄漏！"
				}
			} else {
				ageStr = "未知（无时间戳）"
				leakWarning = " ⚠️  旧版本 key"
			}

			fmt.Printf("Key: %s\n", key)
			fmt.Printf("  当前并发数: %d\n", count)
			fmt.Printf("  存在时长: %s%s\n", ageStr, leakWarning)
			fmt.Println()
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	fmt.Println("==========================================")
	fmt.Printf("总共扫描: %d 个并发 key\n", totalKeys)
	fmt.Printf("当前有并发: %d 个 key\n", keysWithConcurrency)
}

// 清理所有并发计数
func clearAllConcurrency(ctx context.Context, rdb *redis.Client) {
	fmt.Println("开始清理所有并发计数...")
	fmt.Println()

	cursor := uint64(0)
	deletedCount := 0

	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, "channel:key:*:concurrent", 100).Result()
		if err != nil {
			fmt.Printf("扫描失败: %v\n", err)
			break
		}

		for _, key := range keys {
			// 跳过时间戳 key（会随主 key 一起删除）
			if strings.HasSuffix(key, ":timestamp") {
				continue
			}

			val, _ := rdb.Get(ctx, key).Result()
			count, _ := strconv.Atoi(val)

			// 删除主 key 和时间戳 key
			timestampKey := key + ":timestamp"
			err := rdb.Del(ctx, key, timestampKey).Err()
			if err != nil {
				fmt.Printf("✗ 删除失败: %s (错误: %v)\n", key, err)
			} else {
				deletedCount++
				fmt.Printf("✓ 已清理: %s (并发数: %d)\n", key, count)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	fmt.Println()
	fmt.Println("==========================================")
	fmt.Printf("清理完成，共删除 %d 个并发计数\n", deletedCount)
}

// getConcurrencyKey 生成并发 key（与 service/concurrency.go 中的逻辑一致）
func getConcurrencyKey(apiKey string, channelType int) string {
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])
	return fmt.Sprintf("channel:key:%s:type:%d:concurrent", keyHash, channelType)
}
