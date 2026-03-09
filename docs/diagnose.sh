#!/bin/bash

# ============================================
# 性能诊断脚本
# ============================================
# 此脚本会检查：
# 1. 数据库表大小和索引情况
# 2. 慢查询统计
# 3. 数据库连接状态
# 4. 容器资源使用情况
# ============================================

set -e

echo "============================================"
echo "性能诊断报告"
echo "生成时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "============================================"
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 检查Docker是否运行
if ! docker ps > /dev/null 2>&1; then
    echo -e "${RED}错误：Docker未运行或无权限访问${NC}"
    exit 1
fi

# 检查PostgreSQL容器
if ! docker ps | grep -q postgres; then
    echo -e "${RED}错误：PostgreSQL容器未运行${NC}"
    exit 1
fi

echo -e "${BLUE}[1] 数据库表大小统计${NC}"
echo "----------------------------------------"
docker exec postgres psql -U root -d new-api -c "
SELECT
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS total_size,
    pg_size_pretty(pg_relation_size(schemaname||'.'||tablename)) AS table_size,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename) - pg_relation_size(schemaname||'.'||tablename)) AS indexes_size
FROM pg_tables
WHERE schemaname = 'public' AND tablename = 'logs'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
"
echo ""

echo -e "${BLUE}[2] 日志表记录数统计${NC}"
echo "----------------------------------------"
docker exec postgres psql -U root -d new-api -c "
SELECT
    COUNT(*) as total_logs,
    COUNT(*) FILTER (WHERE type = 2) as consume_logs,
    COUNT(*) FILTER (WHERE type = 5) as error_logs,
    COUNT(*) FILTER (WHERE created_at >= EXTRACT(EPOCH FROM NOW() - INTERVAL '7 days')) as logs_last_7days,
    COUNT(*) FILTER (WHERE created_at >= EXTRACT(EPOCH FROM NOW() - INTERVAL '30 days')) as logs_last_30days
FROM logs;
"
echo ""

echo -e "${BLUE}[3] 数据库索引情况${NC}"
echo "----------------------------------------"
docker exec postgres psql -U root -d new-api -c "
SELECT
    indexname,
    pg_size_pretty(pg_relation_size(indexrelid)) as index_size,
    idx_scan as times_used,
    CASE
        WHEN idx_scan = 0 THEN '⚠️  未使用'
        WHEN idx_scan < 100 THEN '⚠️  使用较少'
        ELSE '✓ 正常使用'
    END as usage_status
FROM pg_stat_user_indexes
WHERE tablename = 'logs'
ORDER BY idx_scan DESC;
"
echo ""

echo -e "${BLUE}[4] 数据库连接状态${NC}"
echo "----------------------------------------"
docker exec postgres psql -U root -d new-api -c "
SELECT
    COUNT(*) as total_connections,
    COUNT(*) FILTER (WHERE state = 'active') as active_connections,
    COUNT(*) FILTER (WHERE state = 'idle') as idle_connections,
    COUNT(*) FILTER (WHERE state = 'idle in transaction') as idle_in_transaction,
    (SELECT setting::int FROM pg_settings WHERE name = 'max_connections') as max_connections
FROM pg_stat_activity
WHERE datname = 'new-api';
"
echo ""

echo -e "${BLUE}[5] 最慢的5个查询（如果启用了pg_stat_statements）${NC}"
echo "----------------------------------------"
docker exec postgres psql -U root -d new-api -c "
SELECT
    LEFT(query, 100) as query_preview,
    calls,
    ROUND(total_exec_time::numeric, 2) as total_time_ms,
    ROUND(mean_exec_time::numeric, 2) as avg_time_ms,
    ROUND(max_exec_time::numeric, 2) as max_time_ms
FROM pg_stat_statements
WHERE query LIKE '%logs%'
ORDER BY mean_exec_time DESC
LIMIT 5;
" 2>/dev/null || echo "pg_stat_statements 扩展未启用（可选）"
echo ""

echo -e "${BLUE}[6] 当前活跃查询${NC}"
echo "----------------------------------------"
docker exec postgres psql -U root -d new-api -c "
SELECT
    pid,
    usename,
    application_name,
    state,
    EXTRACT(EPOCH FROM (NOW() - query_start))::int as duration_seconds,
    LEFT(query, 80) as query_preview
FROM pg_stat_activity
WHERE datname = 'new-api'
    AND state != 'idle'
    AND query NOT LIKE '%pg_stat_activity%'
ORDER BY query_start;
"
echo ""

echo -e "${BLUE}[7] Docker容器资源使用情况${NC}"
echo "----------------------------------------"
docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.NetIO}}\t{{.BlockIO}}"
echo ""

echo -e "${BLUE}[8] 数据库缓存命中率${NC}"
echo "----------------------------------------"
docker exec postgres psql -U root -d new-api -c "
SELECT
    'Buffer Cache Hit Rate' as metric,
    ROUND(100.0 * sum(blks_hit) / NULLIF(sum(blks_hit) + sum(blks_read), 0), 2) || '%' as value,
    CASE
        WHEN ROUND(100.0 * sum(blks_hit) / NULLIF(sum(blks_hit) + sum(blks_read), 0), 2) >= 99 THEN '✓ 优秀'
        WHEN ROUND(100.0 * sum(blks_hit) / NULLIF(sum(blks_hit) + sum(blks_read), 0), 2) >= 95 THEN '✓ 良好'
        WHEN ROUND(100.0 * sum(blks_hit) / NULLIF(sum(blks_hit) + sum(blks_read), 0), 2) >= 90 THEN '⚠️  一般'
        ELSE '⚠️  需要优化'
    END as status
FROM pg_stat_database
WHERE datname = 'new-api';
"
echo ""

echo -e "${BLUE}[9] 表膨胀检查${NC}"
echo "----------------------------------------"
docker exec postgres psql -U root -d new-api -c "
SELECT
    schemaname,
    tablename,
    n_live_tup as live_tuples,
    n_dead_tup as dead_tuples,
    CASE
        WHEN n_live_tup > 0 THEN ROUND(100.0 * n_dead_tup / n_live_tup, 2)
        ELSE 0
    END as dead_tuple_percent,
    last_vacuum,
    last_autovacuum
FROM pg_stat_user_tables
WHERE tablename = 'logs';
"
echo ""

echo "============================================"
echo -e "${GREEN}诊断完成${NC}"
echo "============================================"
echo ""
echo "分析建议："
echo ""
echo "1. 如果日志表大小超过10GB，建议定期归档历史数据"
echo "2. 如果有索引显示'未使用'，可以考虑删除以节省空间"
echo "3. 如果缓存命中率低于95%，建议增加shared_buffers"
echo "4. 如果dead_tuple_percent超过20%，建议执行VACUUM"
echo "5. 如果活跃连接数接近max_connections，建议优化连接池"
echo ""
echo "优化命令："
echo "  - 手动VACUUM: docker exec postgres psql -U root -d new-api -c 'VACUUM ANALYZE logs;'"
echo "  - 查看配置: docker exec postgres psql -U root -d new-api -c 'SHOW ALL;'"
echo "  - 重启数据库: docker-compose restart postgres"
echo ""
