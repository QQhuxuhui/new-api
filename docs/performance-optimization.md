# 日志页面性能优化方案

## 问题描述
高峰期访问日志页面时，API请求首字节时间（TTFB）超过30秒，用户体验极差。

## 根本原因
1. 日志表数据量大，缺少针对查询场景的复合索引
2. 查询执行了多次数据库操作（COUNT + SELECT + JOIN）
3. 高峰期数据库连接池可能不足

## 优化方案

### 方案1：数据库索引优化（立即见效）⭐⭐⭐⭐⭐

#### 1.1 添加复合索引

连接到PostgreSQL数据库执行以下SQL：

```sql
-- 优化按时间范围 + 类型查询（最常用场景）
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_created_type_id
ON logs(created_at DESC, type, id DESC);

-- 优化按用户查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_userid_created_type
ON logs(user_id, created_at DESC, type);

-- 优化按用户名查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_username_created_type
ON logs(username, created_at DESC, type)
WHERE username != '';

-- 优化按模型名查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_model_created_type
ON logs(model_name, created_at DESC, type)
WHERE model_name != '';

-- 优化按token名查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_token_created_type
ON logs(token_name, created_at DESC, type)
WHERE token_name != '';

-- 优化统计查询（SumUsedQuota函数）
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_consume_stats
ON logs(type, created_at, username, token_name, model_name, channel_id)
WHERE type = 2;
```

**说明：**
- 使用 `CONCURRENTLY` 避免锁表，可以在线执行
- 这些索引针对 `GetAllLogs` 和 `GetUserLogs` 的查询模式优化
- 预计可将查询时间从30秒降低到1-3秒

#### 1.2 执行方式

```bash
# 方式1：通过docker exec进入postgres容器
docker exec -it postgres psql -U root -d new-api

# 然后执行上面的SQL语句

# 方式2：从宿主机直接执行
docker exec -i postgres psql -U root -d new-api < optimization.sql
```

### 方案2：数据库连接池优化（中等效果）⭐⭐⭐⭐

修改 `docker-compose.yml`，增加数据库连接池配置：

```yaml
environment:
  - SQL_DSN=postgresql://root:123456@postgres:5432/new-api
  - SQL_MAX_IDLE_CONNS=200      # 增加空闲连接数
  - SQL_MAX_OPEN_CONNS=500      # 降低最大连接数（避免数据库过载）
  - SQL_MAX_LIFETIME=300        # 增加连接生命周期到5分钟
```

### 方案3：分页查询优化（代码层面）⭐⭐⭐

当前查询会先 COUNT 再 SELECT，可以优化为：
- 使用游标分页（cursor-based pagination）代替 offset
- 或者异步执行 COUNT 查询，先返回数据

### 方案4：添加缓存层（长期方案）⭐⭐⭐⭐⭐

对于统计数据（GetLogsStat），可以使用Redis缓存：
- 缓存时间：30-60秒
- 高峰期直接返回缓存数据
- 减少数据库压力

### 方案5：数据归档（长期维护）⭐⭐⭐⭐

定期归档历史日志数据：
```bash
# 保留最近3个月的数据，归档更早的数据到历史表
# 可以通过定时任务执行 DeleteHistoryLogs API
```

### 方案6：PostgreSQL数据库优化⭐⭐⭐

优化PostgreSQL配置（需要重启）：

```yaml
# 在 docker-compose.yml 的 postgres 服务中添加
postgres:
  image: postgres:15
  command:
    - "postgres"
    - "-c"
    - "shared_buffers=256MB"           # 增加共享缓冲区
    - "-c"
    - "effective_cache_size=1GB"       # 设置有效缓存大小
    - "-c"
    - "work_mem=16MB"                  # 增加工作内存
    - "-c"
    - "maintenance_work_mem=128MB"     # 增加维护工作内存
    - "-c"
    - "max_connections=200"            # 限制最大连接数
    - "-c"
    - "random_page_cost=1.1"           # SSD优化
```

## 推荐执行顺序

1. **立即执行**：方案1（添加索引）- 5分钟，效果最明显
2. **立即执行**：方案2（连接池优化）- 2分钟，重启容器即可
3. **短期规划**：方案6（PostgreSQL优化）- 10分钟，需要重启
4. **中期规划**：方案4（添加缓存）- 需要开发
5. **长期维护**：方案5（数据归档）- 定期执行

## 预期效果

- **方案1+2**：TTFB从30秒降低到 **1-3秒**
- **方案1+2+6**：TTFB降低到 **0.5-1秒**
- **全部方案**：TTFB降低到 **0.2-0.5秒**

## 监控建议

执行优化后，建议监控：
1. 数据库慢查询日志
2. API响应时间
3. 数据库连接池使用率
4. 服务器资源使用情况（CPU、内存、磁盘IO）
