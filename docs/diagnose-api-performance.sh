#!/bin/bash

# ============================================
# A​PI性能诊断脚本
# ============================================
# 此脚本会检查：
# 1. DNS解析速度
# 2. 到上游A​PI的网络延迟
# 3. TCP连接建立时间
# 4. TLS握手时间
# 5. 首字节响应时间
# ============================================

set -e

echo "============================================"
echo "A​PI性能诊断报告"
echo "生成时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "============================================"
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 创建curl格式化文件
cat > /tmp/curl-format.txt << 'EOF'
    DNS解析时间:     %{time_namelookup}s
    TCP连接时间:     %{time_connect}s
    TLS握手时间:     %{time_appconnect}s
    传输准备时间:    %{time_pretransfer}s
    首字节时间(TTFB): %{time_starttransfer}s
    总时间:          %{time_total}s
    HTTP状态码:      %{http_code}
EOF

# 测试的上游A​PI列表
APIS=(
    "https://api.openai.com/v1/models"
    "https://api.anthropic.com/v1/messages"
    "https://generativelanguage.googleapis.com/v1beta/models"
)

echo -e "${BLUE}[1] 测试DNS解析速度${NC}"
echo "----------------------------------------"
for api in "${APIS[@]}"; do
    domain=$(echo $api | sed -E 's|https?://([^/]+).*|\1|')
    echo -n "测试 $domain ... "

    start=$(date +%s%N)
    nslookup $domain > /dev/null 2>&1
    end=$(date +%s%N)

    duration=$(echo "scale=3; ($end - $start) / 1000000000" | bc)

    if (( $(echo "$duration < 0.1" | bc -l) )); then
        echo -e "${GREEN}✓ ${duration}s (优秀)${NC}"
    elif (( $(echo "$duration < 0.5" | bc -l) )); then
        echo -e "${GREEN}✓ ${duration}s (良好)${NC}"
    elif (( $(echo "$duration < 1.0" | bc -l) )); then
        echo -e "${YELLOW}⚠ ${duration}s (一般)${NC}"
    else
        echo -e "${RED}✗ ${duration}s (慢)${NC}"
    fi
done
echo ""

echo -e "${BLUE}[2] 测试到上游A​PI的网络延迟${NC}"
echo "----------------------------------------"
for api in "${APIS[@]}"; do
    domain=$(echo $api | sed -E 's|https?://([^/]+).*|\1|')
    echo "测试 $api"

    # 使用curl测试详细时间
    curl -w "@/tmp/curl-format.txt" -o /dev/null -s "$api" 2>/dev/null || echo "  ✗ 请求失败"
    echo ""
done

echo -e "${BLUE}[3] 测试本地A​PI性能${NC}"
echo "----------------------------------------"
echo "测试本地new-api服务..."

# 检查服务是否运行
if ! docker ps | grep -q new-api; then
    echo -e "${RED}✗ new-api容器未运行${NC}"
else
    echo -e "${GREEN}✓ new-api容器正在运行${NC}"

    # 测试健康检查端点
    echo ""
    echo "测试健康检查端点 (http://localhost:3000/api/status):"
    curl -w "@/tmp/curl-format.txt" -o /dev/null -s "http://localhost:3000/api/status" 2>/dev/null || echo "  ✗ 请求失败"
fi
echo ""

echo -e "${BLUE}[4] 检查网络连接状态${NC}"
echo "----------------------------------------"
echo "当前TCP连接统计:"
netstat -an | grep ESTABLISHED | wc -l | xargs echo "  ESTABLISHED连接数:"
netstat -an | grep TIME_WAIT | wc -l | xargs echo "  TIME_WAIT连接数:"
echo ""

echo -e "${BLUE}[5] 检查Docker容器资源使用${NC}"
echo "----------------------------------------"
docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}"
echo ""

echo -e "${BLUE}[6] 检查系统网络配置${NC}"
echo "----------------------------------------"
echo "当前DNS服务器:"
cat /etc/resolv.conf | grep nameserver
echo ""

echo "网络接口统计:"
ip -s link show | grep -E "^\d+:|RX:|TX:" | head -20
echo ""

echo -e "${BLUE}[7] 检查到常见A​PI的ping延迟${NC}"
echo "----------------------------------------"
for api in "${APIS[@]}"; do
    domain=$(echo $api | sed -E 's|https?://([^/]+).*|\1|')
    echo -n "Ping $domain ... "

    # 尝试ping 3次
    result=$(ping -c 3 -W 2 $domain 2>/dev/null | grep "avg" | awk -F'/' '{print $5}')

    if [ -z "$result" ]; then
        echo -e "${YELLOW}⚠ 无法ping (可能被防火墙阻止)${NC}"
    else
        if (( $(echo "$result < 50" | bc -l) )); then
            echo -e "${GREEN}✓ ${result}ms (优秀)${NC}"
        elif (( $(echo "$result < 150" | bc -l) )); then
            echo -e "${GREEN}✓ ${result}ms (良好)${NC}"
        elif (( $(echo "$result < 300" | bc -l) )); then
            echo -e "${YELLOW}⚠ ${result}ms (一般)${NC}"
        else
            echo -e "${RED}✗ ${result}ms (慢)${NC}"
        fi
    fi
done
echo ""

echo -e "${BLUE}[8] 检查出口带宽使用情况${NC}"
echo "----------------------------------------"
echo "最近1分钟的网络流量:"
if command -v vnstat &> /dev/null; then
    vnstat -i eth0 -l 1
else
    echo "  vnstat未安装，跳过带宽统计"
    echo "  可以安装: apt-get install vnstat"
fi
echo ""

echo "============================================"
echo -e "${GREEN}诊断完成${NC}"
echo "============================================"
echo ""
echo "分析建议："
echo ""
echo "1. DNS解析时间："
echo "   - 如果 > 0.5秒，建议配置更快的DNS服务器（8.8.8.8, 1.1.1.1）"
echo ""
echo "2. TCP连接时间："
echo "   - 如果 > 1秒，说明网络延迟较高"
echo "   - 考虑使用CDN或选择更近的上游A​PI"
echo ""
echo "3. TLS握手时间："
echo "   - 如果 > 2秒，说明TLS握手慢"
echo "   - 优化HTTP客户端连接池可以复用连接，避免重复握手"
echo ""
echo "4. 首字节时间(TTFB)："
echo "   - 如果 > 5秒，说明上游A​PI响应慢或网络问题"
echo "   - 如果 > 30秒，可能是连接池耗尽，需要优化代码"
echo ""
echo "5. TIME_WAIT连接数："
echo "   - 如果 > 10000，说明连接复用不足"
echo "   - 需要优化HTTP客户端的keep-alive配置"
echo ""
echo "优化建议："
echo "  - 已修改代码优化连接池，需要重新构建镜像"
echo "  - 已配置DNS服务器为8.8.8.8和1.1.1.1"
echo "  - 已配置RELAY_TIMEOUT=120和STREAMING_TIMEOUT=300"
echo ""
echo "下一步："
echo "  1. 重新构建镜像: docker-compose build"
echo "  2. 重启服务: docker-compose down && docker-compose up -d"
echo "  3. 再次运行此脚本验证效果"
echo ""

# 清理临时文件
rm -f /tmp/curl-format.txt
