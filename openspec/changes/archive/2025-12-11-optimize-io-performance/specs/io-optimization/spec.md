# IO Performance Optimization

## Overview

优化系统中存在的大量IO性能问题，提升API吞吐量和响应速度。

## ADDED Requirements

### Requirement: Batch query for channels by tags

系统 **SHALL** 提供批量获取多个Tag对应的渠道列表的能力，避免N+1查询问题。

#### Scenario: Query channels for multiple tags in single request

**Given** 系统中有100个不同的Tag，每个Tag下有若干渠道
**When** 调用 `GetChannelsByTags(["tag1", "tag2", ..., "tag100"], false)`
**Then** 系统执行单次数据库查询
**And** 返回按Tag分组的渠道Map
**And** 查询延迟 < 50ms

#### Scenario: Query with empty tags list

**Given** 传入空的tags列表
**When** 调用 `GetChannelsByTags([], false)`
**Then** 返回空Map
**And** 不执行任何数据库查询

---

### Requirement: Batch save QuotaData with UPSERT pattern

系统 **SHALL** 使用批量查询+批量插入/更新模式保存QuotaData。

#### Scenario: Save 1000 quota data records efficiently

**Given** 缓存中有1000条QuotaData记录
**When** 调用 `SaveQuotaDataCacheBatch()`
**Then** 数据库查询次数 <= 3 (1次查询 + 最多2次批量操作)
**And** 所有数据正确保存或更新
**And** 总耗时 < 1秒

#### Scenario: Handle mixed insert and update records

**Given** 缓存中有500条新记录和500条需更新的记录
**When** 调用 `SaveQuotaDataCacheBatch()`
**Then** 新记录被批量插入
**And** 已存在记录的count/quota/token_used被正确累加
**And** 数据一致性得到保证

---

### Requirement: Redis Pipeline for window stats

系统 **SHALL** 使用Redis Pipeline批量获取渠道健康统计数据。

#### Scenario: Get window stats with Pipeline

**Given** 单个渠道需要查询6个bucket的total和failures
**When** 调用 `GetWindowStats(channelID)`
**Then** 使用单次Pipeline执行12个GET命令
**And** 网络往返次数 = 1
**And** 返回正确的统计数据

#### Scenario: Batch get window stats for multiple channels

**Given** 需要查询100个渠道的健康统计
**When** 调用 `GetBatchWindowStats(channelIDs)`
**Then** 使用单次Pipeline执行所有GET命令
**And** 网络往返次数 = 1
**And** 返回所有渠道的统计Map

---

### Requirement: Cache stampede prevention with distributed lock

系统 **MUST** 使用分布式锁防止缓存击穿。

#### Scenario: Concurrent cache miss with lock protection

**Given** 用户计划缓存失效
**And** 100个并发请求同时访问该用户的计划
**When** 所有请求尝试获取缓存
**Then** 只有1个请求获取到锁并查询数据库
**And** 其他99个请求等待缓存更新后获取
**And** 数据库只执行1次查询

#### Scenario: Lock timeout and fallback

**Given** 获取分布式锁等待超过50ms
**When** 锁仍未获取到
**Then** 直接查询数据库返回结果
**And** 不影响用户请求的正常响应

---

### Requirement: Extended cache TTL with early refresh

系统 **SHALL** 延长缓存TTL并支持提前刷新。

#### Scenario: Cache TTL extended to 5 minutes

**Given** 用户计划缓存设置
**When** 缓存写入
**Then** TTL设置为300秒 (5分钟)

#### Scenario: Early refresh at 4 minutes

**Given** 缓存已存在4分钟
**When** 请求访问该缓存
**Then** 返回当前缓存数据
**And** 异步触发缓存刷新
**And** 用户请求不受刷新影响

