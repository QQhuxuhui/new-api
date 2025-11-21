//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/go-redis/redis/v8"
)

// 清理所有泄漏的并发计数
// 使用方法：
//   REDIS_CONN_STRING=redis://host:port go run scripts/cleanup_concurrency.go

func main() {
	redisConnString := os.Getenv("REDIS_CONN_STRING")
	if redisConnString == "" {
		fmt.Println("错误: 请设置 REDIS_CONN_STRING 环境变量")
		fmt.Println("示例: REDIS_CONN_STRING=redis://sparkcode.top:6379 go run scripts/cleanup_concurrency.go")
		os.Exit(1)
	}

	opt, err := redis.ParseURL(redisConnString)
	if err != nil {
		fmt.Printf("解析 Redis 连接字符串失败: %v\n", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(opt)
	ctx := context.Background()

	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		fmt.Printf("Redis 连接失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Redis 连接成功")
	fmt.Println("==========================================")
	fmt.Println()

	// 扫描并清理所有并发计数
	cursor := uint64(0)
	deletedCount := 0
	totalCount := 0

	fmt.Println("开始清理泄漏的并发计数...")
	fmt.Println()

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

			totalCount++
			val, err := rdb.Get(ctx, key).Result()
			if err != nil {
				continue
			}

			count, _ := strconv.Atoi(val)

			// 删除主 key 和时间戳 key
			timestampKey := key + ":timestamp"
			err = rdb.Del(ctx, key, timestampKey).Err()
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
	fmt.Printf("清理完成！\n")
	fmt.Printf("- 扫描 key 总数: %d\n", totalCount)
	fmt.Printf("- 成功清理: %d\n", deletedCount)
	fmt.Println()
	fmt.Println("建议：")
	fmt.Println("1. 检查应用日志，查看是否有 Redis 超时或错误")
	fmt.Println("2. 监控并发计数，如果持续泄漏需要修复代码")
	fmt.Println("3. 考虑添加定时清理任务，防止并发计数长期堆积")
}
