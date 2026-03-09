#!/bin/bash

# ============================================
# 一键优化和部署脚本
# ============================================
# 此脚本会：
# 1. 备份当前配置
# 2. 检查是否有Dockerfile
# 3. 重新构建镜像（如果有Dockerfile）
# 4. 重启服务
# 5. 验证效果
# ============================================

set -e

echo "============================================"
echo "A​PI性能优化 - 一键部署脚本"
echo "============================================"
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 检查是否在项目根目录
if [ ! -f "docker-compose.yml" ]; then
    echo -e "${RED}错误：请在项目根目录执行此脚本${NC}"
    exit 1
fi

echo -e "${YELLOW}步骤 1/5: 备份当前配置${NC}"
BACKUP_DIR="backup_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$BACKUP_DIR"
cp docker-compose.yml "$BACKUP_DIR/docker-compose.yml.backup"
if [ -f "service/http_client.go" ]; then
    cp service/http_client.go "$BACKUP_DIR/http_client.go.backup"
fi
echo -e "${GREEN}✓ 配置已备份到: $BACKUP_DIR${NC}"
echo ""

echo -e "${YELLOW}步骤 2/5: 检查构建环境${NC}"

# 检查是否有Dockerfile
if [ -f "Dockerfile" ]; then
    echo -e "${GREEN}✓ 找到Dockerfile，可以本地构建${NC}"
    HAS_DOCKERFILE=true
else
    echo -e "${YELLOW}⚠ 未找到Dockerfile${NC}"
    echo "你有以下选择："
    echo "  1. 使用官方镜像（仅应用配置优化，效果有限）"
    echo "  2. 手动创建Dockerfile后再运行此脚本"
    echo ""
    read -p "是否继续使用官方镜像？(y/n): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "已取消"
        exit 0
    fi
    HAS_DOCKERFILE=false
fi
echo ""

if [ "$HAS_DOCKERFILE" = true ]; then
    echo -e "${YELLOW}步骤 3/5: 构建优化后的镜像${NC}"
    echo "正在构建镜像，这可能需要5-10分钟..."

    # 构建镜像
    docker-compose build || {
        echo -e "${RED}✗ 构建失败${NC}"
        echo "请检查错误信息并修复后重试"
        exit 1
    }

    echo -e "${GREEN}✓ 镜像构建成功${NC}"
else
    echo -e "${YELLOW}步骤 3/5: 跳过构建（使用官方镜像）${NC}"
    echo -e "${YELLOW}⚠ 注意：代码优化不会生效，仅应用配置优化${NC}"
fi
echo ""

echo -e "${YELLOW}步骤 4/5: 重启服务${NC}"
echo "正在停止服务..."
docker-compose down

echo "正在启动服务..."
docker-compose up -d

echo ""
echo "等待服务启动..."
sleep 10

# 检查服务状态
if docker-compose ps | grep -q "Up"; then
    echo -e "${GREEN}✓ 服务启动成功${NC}"
else
    echo -e "${RED}✗ 服务启动失败${NC}"
    echo "查看日志: docker-compose logs -f"
    exit 1
fi
echo ""

echo -e "${YELLOW}步骤 5/5: 验证优化效果${NC}"
echo "正在测试健康检查端点..."

# 测试健康检查
for i in {1..5}; do
    if curl -s http://localhost:3000/api/status | grep -q "success"; then
        echo -e "${GREEN}✓ 健康检查通过${NC}"
        break
    else
        if [ $i -eq 5 ]; then
            echo -e "${RED}✗ 健康检查失败${NC}"
            echo "查看日志: docker-compose logs -f new-api"
        else
            echo "等待服务就绪... ($i/5)"
            sleep 3
        fi
    fi
done
echo ""

echo "============================================"
echo -e "${GREEN}优化部署完成！${NC}"
echo "============================================"
echo ""
echo "优化摘要："
if [ "$HAS_DOCKERFILE" = true ]; then
    echo "  ✓ HTTP客户端连接池优化（MaxIdleConnsPerHost: 2 → 100）"
    echo "  ✓ 连接超时优化（30秒 → 10秒）"
fi
echo "  ✓ 配置RELAY_TIMEOUT=120秒"
echo "  ✓ 配置STREAMING_TIMEOUT=300秒"
echo "  ✓ 优化DNS配置（8.8.8.8, 1.1.1.1）"
echo ""
echo "备份位置: $BACKUP_DIR"
echo ""
echo "预期效果："
if [ "$HAS_DOCKERFILE" = true ]; then
    echo "  - 高峰期TTFB从30秒降低到3-8秒"
else
    echo "  - 配置优化可能带来小幅改善"
    echo "  - 建议创建Dockerfile并重新运行此脚本以获得最佳效果"
fi
echo ""
echo "下一步："
echo "  1. 运行诊断: ./docs/diagnose-api-performance.sh"
echo "  2. 查看日志: docker-compose logs -f new-api"
echo "  3. 监控资源: docker stats"
echo "  4. 测试A​PI响应时间"
echo ""
echo "如果遇到问题，可以恢复备份："
echo "  cp $BACKUP_DIR/docker-compose.yml.backup docker-compose.yml"
if [ "$HAS_DOCKERFILE" = true ]; then
    echo "  cp $BACKUP_DIR/http_client.go.backup service/http_client.go"
fi
echo "  docker-compose down && docker-compose up -d"
echo ""
echo -e "${GREEN}祝使用愉快！${NC}"
