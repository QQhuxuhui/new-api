# Proposal: Optimize IO Performance

## Summary

优化系统中存在的大量IO性能问题，包括N+1数据库查询、N+1 Redis查询、缓存策略不合理等问题，预计可提升整体API吞吐量3-5倍。

## Problem Statement

经过代码审查，发现以下严重IO性能问题：

### P0 - 严重问题

1. **controller/channel.go:85-112** - Tag模式下的N+1数据库查询
   - 100个tags = 101次数据库查询
   - 延迟增加 500-2000ms

2. **model/usedata.go:75-87** - QuotaData批量保存的N+1问题
   - 1000条数据 = 2000次数据库操作
   - 严重影响数据导出功能性能

3. **controller/channel.go:1701-1706** - GetAllChannelsHealth的N+1 Redis问题
   - 100个channels = ~1700次Redis操作
   - 每次调用GetWindowStats产生12次Redis GET

### P1 - 中等问题

4. **model/user_plan_cache.go** - 缓存TTL过短(60秒)
   - 高并发时可能产生缓存击穿

5. **model/plan_migration.go:90-95** - 用户迁移的N+1查询
   - 10000用户 = 10000次数据库查询

6. **service/concurrency.go:314-320** - 批量获取并发信息时循环Redis操作

## Proposed Solution

### 数据库查询优化

1. **Tag模式查询优化**: 使用单次查询 + 内存分组替代循环查询
2. **QuotaData保存优化**: 使用批量查询 + UPSERT操作
3. **迁移脚本优化**: 批量查询已存在记录

### Redis操作优化

1. **GetWindowStats**: 使用Redis Pipeline批量获取
2. **并发信息获取**: 使用Pipeline批量查询
3. **健康状态重置**: Pipeline批量删除

### 缓存策略优化

1. **延长TTL**: 从60秒延长到300秒
2. **添加分布式锁**: 防止缓存击穿
3. **异步刷新**: 提前刷新即将过期的缓存

## Impact Analysis

| 场景 | 优化前 | 优化后 | 提升 |
|------|-------|-------|------|
| Tag模式列表 (100 tags) | 101次DB查询 | 2次DB查询 | **50倍** |
| 全部渠道健康状态 (100 channels) | ~1700次Redis | ~100次Redis | **17倍** |
| QuotaData保存 (1000条) | 2000次DB | ~10次DB | **200倍** |
| API整体吞吐 | ~500 QPS | ~2000 QPS | **4倍** |

## Scope

### In Scope
- 数据库N+1查询优化
- Redis批量操作优化
- 缓存策略改进
- 性能监控指标添加

### Out of Scope
- 数据库读写分离
- 消息队列异步处理
- 分布式缓存架构

## Dependencies

- Redis Pipeline支持 (已有)
- GORM批量操作支持 (已有)
- 分布式锁实现 (需新增)

## Risks

1. **缓存一致性**: 延长TTL可能导致数据短暂不一致
   - 缓解: 关键操作主动失效缓存

2. **迁移兼容性**: 查询优化需要保证结果一致
   - 缓解: 添加单元测试验证

3. **Redis依赖**: Pipeline操作需要Redis可用
   - 缓解: 保留降级逻辑
