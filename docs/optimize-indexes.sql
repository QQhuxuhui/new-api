-- ============================================
-- 日志表性能优化 - 索引优化脚本
-- ============================================
-- 说明：此脚本为PostgreSQL数据库添加复合索引以优化日志查询性能
-- 使用 CONCURRENTLY 关键字避免锁表，可以在生产环境在线执行
-- 执行时间：约5-15分钟（取决于数据量）
-- ============================================

-- 检查当前索引
SELECT
    schemaname,
    tablename,
    indexname,
    indexdef
FROM pg_indexes
WHERE tablename = 'logs'
ORDER BY indexname;

-- ============================================
-- 1. 优化按时间范围 + 类型查询（最常用场景）
-- 适用于：GetAllLogs, GetUserLogs 的时间范围过滤
-- ============================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_created_type_id
ON logs(created_at DESC, type, id DESC);

-- ============================================
-- 2. 优化按用户ID查询
-- 适用于：GetUserLogs 函数
-- ============================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_userid_created_type
ON logs(user_id, created_at DESC, type);

-- ============================================
-- 3. 优化按用户名查询
-- 适用于：管理员按用户名筛选日志
-- ============================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_username_created_type
ON logs(username, created_at DESC, type)
WHERE username != '';

-- ============================================
-- 4. 优化按模型名查询
-- 适用于：按模型筛选日志
-- ============================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_model_created_type
ON logs(model_name, created_at DESC, type)
WHERE model_name != '';

-- ============================================
-- 5. 优化按token名查询
-- 适用于：按token筛选日志
-- ============================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_token_created_type
ON logs(token_name, created_at DESC, type)
WHERE token_name != '';

-- ============================================
-- 6. 优化按渠道ID查询
-- 适用于：按渠道筛选日志
-- ============================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_channel_created_type
ON logs(channel_id, created_at DESC, type)
WHERE channel_id != 0;

-- ============================================
-- 7. 优化按用户套餐ID查询
-- 适用于：按套餐筛选日志
-- ============================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_userplan_created_type
ON logs(user_plan_id, created_at DESC, type)
WHERE user_plan_id != 0;

-- ============================================
-- 8. 优化统计查询（SumUsedQuota函数）
-- 适用于：GetLogsStat, GetLogsSelfStat
-- 这个索引覆盖了消费类型日志的所有常用查询条件
-- ============================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_consume_stats
ON logs(type, created_at DESC, username, token_name, model_name, channel_id)
WHERE type = 2;

-- ============================================
-- 9. 优化最近60秒的RPM/TPM统计查询
-- 适用于：SumUsedQuota 中的 rpm/tpm 计算
-- ============================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_recent_consume
ON logs(type, created_at DESC)
WHERE type = 2;

-- ============================================
-- 10. 优化按IP查询（如果启用了IP记录）
-- 适用于：按IP筛选日志
-- ============================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_ip_created
ON logs(ip, created_at DESC)
WHERE ip != '';

-- ============================================
-- 验证索引创建结果
-- ============================================
SELECT
    schemaname,
    tablename,
    indexname,
    pg_size_pretty(pg_relation_size(indexrelid)) as index_size,
    idx_scan as times_used,
    idx_tup_read as tuples_read,
    idx_tup_fetch as tuples_fetched
FROM pg_stat_user_indexes
WHERE tablename = 'logs'
ORDER BY indexname;

-- ============================================
-- 分析表以更新统计信息
-- ============================================
ANALYZE logs;

-- ============================================
-- 完成提示
-- ============================================
SELECT 'Index optimization completed! Please check the query performance.' as status;
