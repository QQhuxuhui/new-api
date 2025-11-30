# Tasks: IO Performance Optimization

## Phase 1: 数据库N+1查询优化 (P0)

### 1.1 Tag模式查询优化
- [x] **T1.1.1** 在 `model/channel.go` 添加 `GetChannelsByTags(tags []string, idSort bool)` 批量查询函数
- [x] **T1.1.2** 修改 `controller/channel.go:GetAllChannels` 使用新的批量查询函数
- [x] **T1.1.3** 修改 `controller/channel.go:SearchChannels` 使用新的批量查询函数
- [ ] **T1.1.4** 添加单元测试验证查询结果一致性
- [ ] **T1.1.5** 性能基准测试对比

### 1.2 QuotaData批量保存优化
- [x] **T1.2.1** 在 `model/usedata.go` 添加 `SaveQuotaDataCacheBatch()` 批量保存函数
- [x] **T1.2.2** 修改 `SaveQuotaDataCache()` 调用新的批量函数
- [ ] **T1.2.3** 添加单元测试验证数据一致性
- [ ] **T1.2.4** 验证大批量数据保存性能

### 1.3 用户计划迁移优化
- [x] **T1.3.1** 修改 `model/plan_migration.go:MigrateUsersToPlans` 使用批量查询
- [ ] **T1.3.2** 添加测试验证迁移结果正确性

## Phase 2: Redis批量操作优化 (P0)

### 2.1 GetWindowStats Pipeline优化
- [x] **T2.1.1** 修改 `service/channel_health.go:GetWindowStats` 使用Pipeline
- [x] **T2.1.2** 添加 `GetBatchWindowStats(channelIDs []int)` 批量获取函数
- [x] **T2.1.3** 修改 `controller/channel.go:GetAllChannelsHealth` 使用批量函数
- [ ] **T2.1.4** 添加单元测试
- [ ] **T2.1.5** 性能基准测试对比

### 2.2 并发信息批量获取优化
- [x] **T2.2.1** 修改 `service/concurrency.go:GetBatchChannelsConcurrency` 使用Pipeline
- [ ] **T2.2.2** 添加单元测试
- [ ] **T2.2.3** 性能测试验证

### 2.3 健康状态批量操作
- [x] **T2.3.1** 添加 `GetBatchChannelHealth` 函数使用Pipeline批量获取
- [x] **T2.3.2** 修改 `controller/channel.go:GetAllChannelsHealth` 使用批量函数

## Phase 3: 缓存策略优化 (P1)

### 3.1 用户计划缓存优化
- [x] **T3.1.1** 在 `common/redis.go` 添加 `RedisSetNX` 分布式锁函数
- [x] **T3.1.2** 修改 `model/user_plan_cache.go` TTL从60秒延长到300秒
- [x] **T3.1.3** 修改 `CachedGetUserValidPlans` 添加分布式锁防止缓存击穿
- [x] **T3.1.4** 修改 `CachedGetUserCurrentPlan` 添加分布式锁防止缓存击穿
- [ ] **T3.1.5** 并发测试验证锁的正确性

### 3.2 缓存监控
- [ ] **T3.2.1** 添加缓存命中率监控指标
- [ ] **T3.2.2** 添加缓存操作延迟监控

## Phase 4: 性能监控与验证 (P2)

### 4.1 性能指标
- [ ] **T4.1.1** 添加数据库查询耗时监控
- [ ] **T4.1.2** 添加Redis操作耗时监控
- [ ] **T4.1.3** 添加API响应时间监控

### 4.2 集成测试
- [ ] **T4.2.1** 端到端性能测试
- [ ] **T4.2.2** 压力测试验证
- [ ] **T4.2.3** 回归测试确保功能正确

## Dependencies

```
T1.1.1 ──► T1.1.2 ──► T1.1.3 ──► T1.1.4 ──► T1.1.5
T1.2.1 ──► T1.2.2 ──► T1.2.3 ──► T1.2.4
T1.3.1 ──► T1.3.2

T2.1.1 ──► T2.1.2 ──► T2.1.3 ──► T2.1.4 ──► T2.1.5
T2.2.1 ──► T2.2.2 ──► T2.2.3
T2.3.1 ──► T2.3.2

T3.1.1 ──► T3.1.3 ──► T3.1.4 ──► T3.1.5
        └─► T3.1.2
T3.2.1 ──► T3.2.2

Phase 1 & 2 can run in parallel
Phase 3 depends on Phase 2 (Redis functions)
Phase 4 depends on all previous phases
```

## Parallelizable Work

以下任务可以并行执行：
- T1.1.x 和 T1.2.x 和 T1.3.x (不同模块的数据库优化)
- T2.1.x 和 T2.2.x 和 T2.3.x (不同模块的Redis优化)
- T3.1.x 和 T3.2.x (缓存优化和监控)

## Validation Criteria

### 功能验证
- [ ] 所有现有单元测试通过
- [ ] 新增单元测试覆盖率 > 80%
- [ ] API响应数据与优化前一致

### 性能验证
- [ ] Tag模式查询延迟降低 > 80%
- [ ] GetAllChannelsHealth延迟降低 > 90%
- [ ] QuotaData保存延迟降低 > 95%
- [ ] 整体API吞吐量提升 > 3倍
