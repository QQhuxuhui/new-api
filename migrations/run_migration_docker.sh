#!/bin/bash

# 🚀 Solution B 数据库迁移脚本 - Docker 环境
# 用于在 Docker Compose 环境下执行完整迁移流程

set -e  # 遇到错误立即退出

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置（请根据您的环境修改）
POSTGRES_CONTAINER="postgres"  # PostgreSQL 容器名称
DB_USER="root"                 # 数据库用户名
DB_NAME="new-api"              # 数据库名称
DB_PASSWORD=""                 # 数据库密码（如果有）

echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}   Solution B 数据库迁移工具${NC}"
echo -e "${BLUE}======================================${NC}"
echo ""

# 步骤 0: 检查 Docker 容器
echo -e "${YELLOW}[0/6] 检查 PostgreSQL 容器...${NC}"
if ! docker ps | grep -q "$POSTGRES_CONTAINER"; then
    echo -e "${RED}✗ 错误: 找不到运行中的 PostgreSQL 容器 '$POSTGRES_CONTAINER'${NC}"
    echo -e "${YELLOW}提示: 请运行 'docker ps' 查看容器名称，并修改脚本中的 POSTGRES_CONTAINER 变量${NC}"
    exit 1
fi
echo -e "${GREEN}✓ PostgreSQL 容器运行中${NC}"
echo ""

# 步骤 1: 备份数据库
echo -e "${YELLOW}[1/6] 备份数据库...${NC}"
BACKUP_FILE="backup_before_solution_b_$(date +%Y%m%d_%H%M%S).sql"

if [ -n "$DB_PASSWORD" ]; then
    docker exec -e PGPASSWORD="$DB_PASSWORD" -t "$POSTGRES_CONTAINER" \
        pg_dump -U "$DB_USER" -d "$DB_NAME" > "$BACKUP_FILE"
else
    docker exec -t "$POSTGRES_CONTAINER" \
        pg_dump -U "$DB_USER" -d "$DB_NAME" > "$BACKUP_FILE"
fi

if [ -s "$BACKUP_FILE" ]; then
    BACKUP_SIZE=$(du -h "$BACKUP_FILE" | cut -f1)
    echo -e "${GREEN}✓ 备份成功: $BACKUP_FILE (${BACKUP_SIZE})${NC}"
else
    echo -e "${RED}✗ 备份失败: 文件为空${NC}"
    exit 1
fi
echo ""

# 步骤 2: 检查迁移文件
echo -e "${YELLOW}[2/6] 检查迁移文件...${NC}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ ! -f "$SCRIPT_DIR/solution_b_complete.sql" ]; then
    echo -e "${RED}✗ 找不到迁移文件: solution_b_complete.sql${NC}"
    echo -e "${YELLOW}提示: 请确保在 migrations/ 目录下执行此脚本${NC}"
    exit 1
fi
echo -e "${GREEN}✓ 迁移文件存在${NC}"
echo ""

# 步骤 3: 执行数据库迁移
echo -e "${YELLOW}[3/6] 执行数据库迁移...${NC}"
echo -e "${BLUE}正在执行 solution_b_complete.sql...${NC}"

if [ -n "$DB_PASSWORD" ]; then
    docker exec -i -e PGPASSWORD="$DB_PASSWORD" "$POSTGRES_CONTAINER" \
        psql -U "$DB_USER" -d "$DB_NAME" < "$SCRIPT_DIR/solution_b_complete.sql"
else
    docker exec -i "$POSTGRES_CONTAINER" \
        psql -U "$DB_USER" -d "$DB_NAME" < "$SCRIPT_DIR/solution_b_complete.sql"
fi

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ 迁移执行成功${NC}"
else
    echo -e "${RED}✗ 迁移执行失败${NC}"
    echo -e "${YELLOW}提示: 请检查错误信息，可以使用备份文件恢复: $BACKUP_FILE${NC}"
    exit 1
fi
echo ""

# 步骤 4: 验证迁移结果
echo -e "${YELLOW}[4/6] 验证迁移结果...${NC}"

# 验证 1: plan_id 列允许 NULL
echo -e "${BLUE}验证 1: 检查 plan_id 列是否允许 NULL...${NC}"
if [ -n "$DB_PASSWORD" ]; then
    NULLABLE=$(docker exec -e PGPASSWORD="$DB_PASSWORD" "$POSTGRES_CONTAINER" \
        psql -U "$DB_USER" -d "$DB_NAME" -t -c \
        "SELECT is_nullable FROM information_schema.columns WHERE table_name = 'plan_orders' AND column_name = 'plan_id';")
else
    NULLABLE=$(docker exec "$POSTGRES_CONTAINER" \
        psql -U "$DB_USER" -d "$DB_NAME" -t -c \
        "SELECT is_nullable FROM information_schema.columns WHERE table_name = 'plan_orders' AND column_name = 'plan_id';")
fi

if echo "$NULLABLE" | grep -q "YES"; then
    echo -e "${GREEN}✓ plan_id 列已允许 NULL${NC}"
else
    echo -e "${RED}✗ plan_id 列仍为 NOT NULL${NC}"
    exit 1
fi

# 验证 2: 外键约束为 SET NULL
echo -e "${BLUE}验证 2: 检查外键约束...${NC}"
if [ -n "$DB_PASSWORD" ]; then
    DELETE_RULE=$(docker exec -e PGPASSWORD="$DB_PASSWORD" "$POSTGRES_CONTAINER" \
        psql -U "$DB_USER" -d "$DB_NAME" -t -c \
        "SELECT delete_rule FROM information_schema.referential_constraints WHERE constraint_name = 'fk_plan_orders_plan';")
else
    DELETE_RULE=$(docker exec "$POSTGRES_CONTAINER" \
        psql -U "$DB_USER" -d "$DB_NAME" -t -c \
        "SELECT delete_rule FROM information_schema.referential_constraints WHERE constraint_name = 'fk_plan_orders_plan';")
fi

if echo "$DELETE_RULE" | grep -q "SET NULL"; then
    echo -e "${GREEN}✓ 外键约束已更新为 ON DELETE SET NULL${NC}"
else
    echo -e "${RED}✗ 外键约束不是 SET NULL: $DELETE_RULE${NC}"
    exit 1
fi

# 验证 3: 快照字段存在
echo -e "${BLUE}验证 3: 检查快照字段...${NC}"
if [ -n "$DB_PASSWORD" ]; then
    SNAPSHOT_COUNT=$(docker exec -e PGPASSWORD="$DB_PASSWORD" "$POSTGRES_CONTAINER" \
        psql -U "$DB_USER" -d "$DB_NAME" -t -c \
        "SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'plan_orders' AND column_name IN ('plan_name', 'plan_display_name');")
else
    SNAPSHOT_COUNT=$(docker exec "$POSTGRES_CONTAINER" \
        psql -U "$DB_USER" -d "$DB_NAME" -t -c \
        "SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'plan_orders' AND column_name IN ('plan_name', 'plan_display_name');")
fi

if [ "$(echo $SNAPSHOT_COUNT | tr -d ' ')" = "2" ]; then
    echo -e "${GREEN}✓ 快照字段已添加${NC}"
else
    echo -e "${RED}✗ 快照字段未完全添加${NC}"
    exit 1
fi

echo ""

# 步骤 5: 显示统计信息
echo -e "${YELLOW}[5/6] 显示数据库统计...${NC}"
if [ -n "$DB_PASSWORD" ]; then
    docker exec -e PGPASSWORD="$DB_PASSWORD" "$POSTGRES_CONTAINER" \
        psql -U "$DB_USER" -d "$DB_NAME" -c \
        "SELECT COUNT(*) as total_orders, COUNT(plan_name) as orders_with_snapshot, COUNT(plan_id) as orders_with_plan_id FROM plan_orders;"
else
    docker exec "$POSTGRES_CONTAINER" \
        psql -U "$DB_USER" -d "$DB_NAME" -c \
        "SELECT COUNT(*) as total_orders, COUNT(plan_name) as orders_with_snapshot, COUNT(plan_id) as orders_with_plan_id FROM plan_orders;"
fi
echo ""

# 步骤 6: 完成
echo -e "${GREEN}======================================${NC}"
echo -e "${GREEN}   ✓ 迁移完成！${NC}"
echo -e "${GREEN}======================================${NC}"
echo ""
echo -e "${BLUE}下一步操作：${NC}"
echo -e "1. 重启应用服务（如果需要）"
echo -e "2. 测试删除只有已完成订单的套餐"
echo -e "3. 验证订单历史显示正常（使用快照数据）"
echo ""
echo -e "${YELLOW}备份文件位置: $BACKUP_FILE${NC}"
echo -e "${YELLOW}如需回滚，请运行:${NC}"
if [ -n "$DB_PASSWORD" ]; then
    echo -e "  docker exec -i -e PGPASSWORD=\"$DB_PASSWORD\" $POSTGRES_CONTAINER psql -U $DB_USER -d $DB_NAME < $BACKUP_FILE"
else
    echo -e "  docker exec -i $POSTGRES_CONTAINER psql -U $DB_USER -d $DB_NAME < $BACKUP_FILE"
fi
echo ""
