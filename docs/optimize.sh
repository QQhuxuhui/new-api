#!/bin/bash

# ============================================
# 日志页面性能优化 - 一键执行脚本
# ============================================
# 此脚本会自动执行以下优化：
# 1. 备份当前配置
# 2. 添加数据库索引
# 3. 优化docker-compose配置
# 4. 重启服务
# ============================================

set -e  # 遇到错误立即退出

echo "============================================"
echo "日志页面性能优化脚本"
echo "============================================"
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查是否在项目根目录
if [ ! -f "docker-compose.yml" ]; then
    echo -e "${RED}错误：请在项目根目录执行此脚本${NC}"
    exit 1
fi

# 检查Docker是否运行
if ! docker ps > /dev/null 2>&1; then
    echo -e "${RED}错误：Docker未运行或无权限访问${NC}"
    exit 1
fi

echo -e "${YELLOW}步骤 1/5: 备份当前配置${NC}"
BACKUP_DIR="backup_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$BACKUP_DIR"
cp docker-compose.yml "$BACKUP_DIR/docker-compose.yml.backup"
echo -e "${GREEN}✓ 配置已备份到: $BACKUP_DIR${NC}"
echo ""

echo -e "${YELLOW}步骤 2/5: 检查数据库连接${NC}"
if docker ps | grep -q postgres; then
    echo -e "${GREEN}✓ PostgreSQL容器正在运行${NC}"
else
    echo -e "${RED}错误：PostgreSQL容器未运行${NC}"
    echo "请先启动服务: docker-compose up -d"
    exit 1
fi
echo ""

echo -e "${YELLOW}步骤 3/5: 添加数据库索引（这可能需要5-15分钟）${NC}"
echo "正在执行索引优化..."

# 执行索引优化SQL
docker exec -i postgres psql -U root -d new-api <<'EOF'
-- 显示当前时间
SELECT NOW() as start_time, '开始创建索引...' as status;

-- 1. 优化按时间范围 + 类型查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_created_type_id
ON logs(created_at DESC, type, id DESC);
SELECT '✓ 索引 1/10 创建完成' as status;

-- 2. 优化按用户ID查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_userid_created_type
ON logs(user_id, created_at DESC, type);
SELECT '✓ 索引 2/10 创建完成' as status;

-- 3. 优化按用户名查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_username_created_type
ON logs(username, created_at DESC, type)
WHERE username != '';
SELECT '✓ 索引 3/10 创建完成' as status;

-- 4. 优化按模型名查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_model_created_type
ON logs(model_name, created_at DESC, type)
WHERE model_name != '';
SELECT '✓ 索引 4/10 创建完成' as status;

-- 5. 优化按token名查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_token_created_type
ON logs(token_name, created_at DESC, type)
WHERE token_name != '';
SELECT '✓ 索引 5/10 创建完成' as status;

-- 6. 优化按渠道ID查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_channel_created_type
ON logs(channel_id, created_at DESC, type)
WHERE channel_id != 0;
SELECT '✓ 索引 6/10 创建完成' as status;

-- 7. 优化按用户套餐ID查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_userplan_created_type
ON logs(user_plan_id, created_at DESC, type)
WHERE user_plan_id != 0;
SELECT '✓ 索引 7/10 创建完成' as status;

-- 8. 优化统计查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_consume_stats
ON logs(type, created_at DESC, username, token_name, model_name, channel_id)
WHERE type = 2;
SELECT '✓ 索引 8/10 创建完成' as status;

-- 9. 优化最近60秒的RPM/TPM统计
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_recent_consume
ON logs(type, created_at DESC)
WHERE type = 2;
SELECT '✓ 索引 9/10 创建完成' as status;

-- 10. 优化按IP查询
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_ip_created
ON logs(ip, created_at DESC)
WHERE ip != '';
SELECT '✓ 索引 10/10 创建完成' as status;

-- 分析表
ANALYZE logs;
SELECT '✓ 表分析完成' as status;

-- 显示完成时间
SELECT NOW() as end_time, '所有索引创建完成！' as status;

-- 显示索引信息
SELECT
    indexname,
    pg_size_pretty(pg_relation_size(indexrelid)) as index_size
FROM pg_stat_user_indexes
WHERE tablename = 'logs'
ORDER BY indexname;
EOF

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ 数据库索引优化完成${NC}"
else
    echo -e "${RED}✗ 索引创建失败，请检查错误信息${NC}"
    exit 1
fi
echo ""

echo -e "${YELLOW}步骤 4/5: 更新docker-compose配置${NC}"
echo "是否要更新docker-compose.yml以优化数据库连接池和PostgreSQL配置？"
echo -e "${YELLOW}注意：这将需要重启服务（约1-2分钟停机时间）${NC}"
read -p "继续? (y/n): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    # 检查优化配置文件是否存在
    if [ -f "docs/docker-compose-optimized.yml" ]; then
        cp docker-compose.yml "$BACKUP_DIR/docker-compose.yml.before_optimization"
        cp docs/docker-compose-optimized.yml docker-compose.yml
        echo -e "${GREEN}✓ docker-compose.yml 已更新${NC}"

        echo -e "${YELLOW}步骤 5/5: 重启服务${NC}"
        echo "正在重启服务..."
        docker-compose down
        docker-compose up -d

        echo ""
        echo "等待服务启动..."
        sleep 10

        # 检查服务状态
        if docker-compose ps | grep -q "Up"; then
            echo -e "${GREEN}✓ 服务重启成功${NC}"
        else
            echo -e "${RED}✗ 服务启动失败，请检查日志${NC}"
            echo "查看日志: docker-compose logs -f"
            exit 1
        fi
    else
        echo -e "${RED}错误：找不到优化配置文件 docs/docker-compose-optimized.yml${NC}"
        exit 1
    fi
else
    echo -e "${YELLOW}跳过docker-compose配置更新${NC}"
    echo -e "${YELLOW}你可以稍后手动更新配置并重启服务${NC}"
fi
echo ""

echo "============================================"
echo -e "${GREEN}优化完成！${NC}"
echo "============================================"
echo ""
echo "优化摘要："
echo "  ✓ 已添加10个数据库索引"
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "  ✓ 已优化数据库连接池配置"
    echo "  ✓ 已优化PostgreSQL性能参数"
    echo "  ✓ 服务已重启"
fi
echo ""
echo "备份位置: $BACKUP_DIR"
echo ""
echo "预期效果："
echo "  - API响应时间从 30秒+ 降低到 1-3秒"
echo "  - 高峰期性能显著提升"
echo ""
echo "监控建议："
echo "  1. 查看应用日志: docker-compose logs -f new-api"
echo "  2. 查看数据库日志: docker-compose logs -f postgres"
echo "  3. 监控资源使用: docker stats"
echo "  4. 检查慢查询: docker exec postgres psql -U root -d new-api -c \"SELECT * FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 10;\""
echo ""
echo "如果遇到问题，可以恢复备份："
echo "  cp $BACKUP_DIR/docker-compose.yml.backup docker-compose.yml"
echo "  docker-compose down && docker-compose up -d"
echo ""
echo -e "${GREEN}祝使用愉快！${NC}"
