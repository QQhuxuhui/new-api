# Design: IO Performance Optimization

## Overview

本设计文档描述IO性能优化的技术实现方案，涵盖数据库查询优化、Redis批量操作、缓存策略改进三个主要领域。

## Architecture

### 当前架构问题

```
┌─────────────┐     循环查询      ┌──────────┐
│  Controller │ ──────────────► │ Database │
│             │   N+1 问题       │          │
└─────────────┘                  └──────────┘
       │
       │         循环操作        ┌──────────┐
       └─────────────────────► │  Redis   │
                 N+1 问题       │          │
                               └──────────┘
```

### 优化后架构

```
┌─────────────┐     批量查询      ┌──────────┐
│  Controller │ ──────────────► │ Database │
│             │   单次往返       │          │
└─────────────┘                  └──────────┘
       │
       │         Pipeline       ┌──────────┐
       └─────────────────────► │  Redis   │
                 批量操作       │          │
                               └──────────┘
```

## Detailed Design

### 1. 数据库N+1查询优化

#### 1.1 Tag模式查询优化

**当前实现** (`controller/channel.go:85-112`):
```go
for _, tag := range tags {
    tagChannels, err := model.GetChannelsByTag(*tag, idSort)
    // N次查询
}
```

**优化方案**:
```go
// model/channel.go - 新增函数
func GetChannelsByTags(tags []string, idSort bool) (map[string][]*Channel, error) {
    var channels []*Channel

    query := DB.Where("tag IN ?", tags)
    if idSort {
        query = query.Order("id desc")
    } else {
        query = query.Order("priority desc")
    }

    if err := query.Omit("key").Find(&channels).Error; err != nil {
        return nil, err
    }

    // 内存分组
    result := make(map[string][]*Channel)
    for _, ch := range channels {
        if ch.Tag != nil {
            result[*ch.Tag] = append(result[*ch.Tag], ch)
        }
    }

    return result, nil
}
```

#### 1.2 QuotaData批量保存优化

**当前实现** (`model/usedata.go:75-87`):
```go
for _, quotaData := range CacheQuotaData {
    DB.Where(...).First(quotaDataDB)  // N次SELECT
    if quotaDataDB.Id > 0 {
        increaseQuotaData(...)        // N次UPDATE
    } else {
        DB.Create(quotaData)          // N次INSERT
    }
}
```

**优化方案**:
```go
func SaveQuotaDataCacheBatch() {
    if len(CacheQuotaData) == 0 {
        return
    }

    // 1. 收集所有查询条件
    var conditions []map[string]interface{}
    for _, qd := range CacheQuotaData {
        conditions = append(conditions, map[string]interface{}{
            "user_id":    qd.UserID,
            "username":   qd.Username,
            "model_name": qd.ModelName,
            "created_at": qd.CreatedAt,
        })
    }

    // 2. 批量查询已存在记录
    var existing []QuotaData
    DB.Table("quota_data").Where(conditions).Find(&existing)

    existingMap := make(map[string]int64)
    for _, e := range existing {
        key := fmt.Sprintf("%d-%s-%s-%d", e.UserID, e.Username, e.ModelName, e.CreatedAt)
        existingMap[key] = e.Id
    }

    // 3. 分离插入和更新
    var toInsert []*QuotaData
    var toUpdate []*QuotaData

    for _, qd := range CacheQuotaData {
        key := fmt.Sprintf("%d-%s-%s-%d", qd.UserID, qd.Username, qd.ModelName, qd.CreatedAt)
        if _, exists := existingMap[key]; exists {
            toUpdate = append(toUpdate, qd)
        } else {
            toInsert = append(toInsert, qd)
        }
    }

    // 4. 批量插入
    if len(toInsert) > 0 {
        DB.Table("quota_data").CreateInBatches(toInsert, 100)
    }

    // 5. 批量更新 (使用事务)
    if len(toUpdate) > 0 {
        tx := DB.Begin()
        for _, qd := range toUpdate {
            tx.Table("quota_data").
                Where("user_id = ? AND username = ? AND model_name = ? AND created_at = ?",
                    qd.UserID, qd.Username, qd.ModelName, qd.CreatedAt).
                Updates(map[string]interface{}{
                    "count":      gorm.Expr("count + ?", qd.Count),
                    "quota":      gorm.Expr("quota + ?", qd.Quota),
                    "token_used": gorm.Expr("token_used + ?", qd.TokenUsed),
                })
        }
        tx.Commit()
    }
}
```

### 2. Redis批量操作优化

#### 2.1 GetWindowStats Pipeline优化

**当前实现** (`service/channel_health.go:88-112`):
```go
for i := int64(0); i < BucketCount; i++ {
    total, _ := rdb.Get(ctx, totalKey).Int64()    // 12次GET
    failures, _ := rdb.Get(ctx, failureKey).Int64()
}
```

**优化方案**:
```go
// GetWindowStats - 使用Pipeline优化
func GetWindowStats(channelID int) (totalCount, failureCount int64) {
    ctx := context.Background()
    rdb := common.RDB

    if rdb == nil {
        return 0, 0
    }

    currentBucket := time.Now().Unix() / int64(BucketSize.Seconds())

    // 使用Pipeline批量获取
    pipe := rdb.Pipeline()
    totalCmds := make([]*redis.StringCmd, BucketCount)
    failureCmds := make([]*redis.StringCmd, BucketCount)

    for i := int64(0); i < BucketCount; i++ {
        bucket := currentBucket - i
        totalKey := fmt.Sprintf(keyBucketTotal, channelID, bucket)
        failureKey := fmt.Sprintf(keyBucketFailures, channelID, bucket)

        totalCmds[i] = pipe.Get(ctx, totalKey)
        failureCmds[i] = pipe.Get(ctx, failureKey)
    }

    pipe.Exec(ctx)  // 单次网络往返

    // 解析结果
    for i := int64(0); i < BucketCount; i++ {
        if total, err := totalCmds[i].Int64(); err == nil {
            totalCount += total
        }
        if failures, err := failureCmds[i].Int64(); err == nil {
            failureCount += failures
        }
    }

    return totalCount, failureCount
}

// GetBatchWindowStats - 批量获取多个channel的统计
func GetBatchWindowStats(channelIDs []int) map[int]*WindowStats {
    ctx := context.Background()
    rdb := common.RDB

    if rdb == nil || len(channelIDs) == 0 {
        return nil
    }

    currentBucket := time.Now().Unix() / int64(BucketSize.Seconds())

    // 使用Pipeline批量获取所有channel的所有bucket
    pipe := rdb.Pipeline()
    cmdMap := make(map[int][]*redis.StringCmd)  // channelID -> [total0, fail0, total1, fail1, ...]

    for _, channelID := range channelIDs {
        cmds := make([]*redis.StringCmd, BucketCount*2)
        for i := int64(0); i < BucketCount; i++ {
            bucket := currentBucket - i
            cmds[i*2] = pipe.Get(ctx, fmt.Sprintf(keyBucketTotal, channelID, bucket))
            cmds[i*2+1] = pipe.Get(ctx, fmt.Sprintf(keyBucketFailures, channelID, bucket))
        }
        cmdMap[channelID] = cmds
    }

    pipe.Exec(ctx)  // 单次网络往返获取所有数据

    // 解析结果
    result := make(map[int]*WindowStats)
    for channelID, cmds := range cmdMap {
        var totalCount, failureCount int64
        for i := int64(0); i < BucketCount; i++ {
            if total, err := cmds[i*2].Int64(); err == nil {
                totalCount += total
            }
            if failures, err := cmds[i*2+1].Int64(); err == nil {
                failureCount += failures
            }
        }
        result[channelID] = &WindowStats{
            TotalCount:   totalCount,
            FailureCount: failureCount,
        }
    }

    return result
}
```

### 3. 缓存策略优化

#### 3.1 用户计划缓存优化

**当前问题**:
- TTL仅60秒，频繁失效
- 异步更新可能导致缓存击穿

**优化方案**:
```go
const (
    userPlanCacheTTLSec     = 300  // 5分钟
    userPlanCacheRefreshSec = 240  // 4分钟后开始异步刷新
)

// CachedGetUserValidPlans - 带分布式锁的缓存获取
func CachedGetUserValidPlans(userId int) ([]*UserPlan, error) {
    // 1. 尝试从缓存获取
    plans, err := cacheGetUserValidPlans(userId)
    if err == nil && len(plans) > 0 {
        return plans, nil
    }

    // 2. 缓存未命中，尝试获取分布式锁
    lockKey := fmt.Sprintf("lock:user_plans:%d", userId)
    acquired := common.RedisSetNX(lockKey, "1", 10*time.Second)

    if acquired {
        // 获取到锁，查询数据库
        defer common.RedisDel(lockKey)

        plans, err = GetUserValidPlans(userId)
        if err != nil {
            return nil, err
        }

        // 同步更新缓存
        if len(plans) > 0 {
            cacheSetUserValidPlans(userId, plans)
        }

        return plans, nil
    }

    // 未获取到锁，等待后重试缓存
    time.Sleep(50 * time.Millisecond)
    plans, err = cacheGetUserValidPlans(userId)
    if err == nil && len(plans) > 0 {
        return plans, nil
    }

    // 最后降级到数据库查询
    return GetUserValidPlans(userId)
}
```

## Performance Metrics

### 监控指标

```go
// 添加到 common/metrics.go
var (
    DBQueryDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "db_query_duration_seconds",
            Help: "Database query duration in seconds",
        },
        []string{"query_type"},
    )

    RedisOpDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "redis_op_duration_seconds",
            Help: "Redis operation duration in seconds",
        },
        []string{"operation"},
    )

    CacheHitRatio = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "cache_hit_ratio",
            Help: "Cache hit ratio",
        },
        []string{"cache_type"},
    )
)
```

## Testing Strategy

### 单元测试

1. **查询结果一致性测试**: 验证优化后查询结果与原查询一致
2. **并发安全测试**: 验证分布式锁在高并发下的正确性
3. **降级测试**: 验证Redis不可用时的降级行为

### 性能测试

1. **基准测试**: 对比优化前后的响应时间
2. **压力测试**: 验证高并发下的稳定性
3. **长时间运行测试**: 验证内存和连接池稳定性

## Migration Plan

1. **Phase 1**: 添加新的批量查询函数，保留原函数
2. **Phase 2**: 逐步切换到新函数，监控性能指标
3. **Phase 3**: 确认稳定后删除旧函数
