#!/bin/bash

# 健康跟踪功能测试脚本
# Test script for health tracking functionality

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 测试配置
API_BASE="http://localhost:3000"
ADMIN_TOKEN="" # 需要从环境或配置获取

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[⚠]${NC} $1"
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
}

log_section() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

# 检查依赖
check_dependencies() {
    log_section "检查依赖"

    for cmd in curl redis-cli jq; do
        if command -v $cmd &> /dev/null; then
            log_success "$cmd 已安装"
        else
            log_error "$cmd 未安装"
            exit 1
        fi
    done
}

# 检查Redis连接
check_redis() {
    log_section "检查Redis连接"

    if redis-cli ping | grep -q PONG; then
        log_success "Redis连接正常"
    else
        log_error "Redis连接失败"
        exit 1
    fi
}

# 检查应用是否运行
check_app() {
    log_section "检查应用状态"

    if curl -s -f "$API_BASE/api/status" > /dev/null 2>&1; then
        log_success "应用运行正常"
        return 0
    else
        log_warning "应用未运行，尝试启动..."
        return 1
    fi
}

# 启动应用
start_app() {
    log_section "启动应用"

    # 检查二进制文件
    if [ ! -f "./new-api" ]; then
        log_error "new-api 二进制文件不存在，请先编译"
        exit 1
    fi

    # 启动应用（后台）
    log_info "启动 new-api（后台模式）..."
    nohup ./new-api > /tmp/new-api.log 2>&1 &
    APP_PID=$!
    echo $APP_PID > /tmp/new-api.pid

    # 等待应用启动
    log_info "等待应用启动（最多30秒）..."
    for i in {1..30}; do
        if curl -s -f "$API_BASE/api/status" > /dev/null 2>&1; then
            log_success "应用已启动 (PID: $APP_PID)"
            return 0
        fi
        sleep 1
    done

    log_error "应用启动超时"
    cat /tmp/new-api.log
    exit 1
}

# 停止应用
stop_app() {
    log_section "停止应用"

    if [ -f /tmp/new-api.pid ]; then
        PID=$(cat /tmp/new-api.pid)
        if kill -0 $PID 2>/dev/null; then
            log_info "停止应用 (PID: $PID)"
            kill $PID
            rm /tmp/new-api.pid
            log_success "应用已停止"
        fi
    fi
}

# 测试1: API端点测试
test_api_endpoints() {
    log_section "测试1: API端点"

    # 测试获取所有通道健康状态
    log_info "测试: GET /api/channel/health"
    response=$(curl -s -w "\n%{http_code}" "$API_BASE/api/channel/health")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [ "$http_code" = "200" ]; then
        log_success "API返回200"
        if echo "$body" | jq -e '.success' > /dev/null 2>&1; then
            log_success "响应格式正确"
            channel_count=$(echo "$body" | jq '.data | length')
            log_info "返回 $channel_count 个通道的健康状态"
        else
            log_warning "响应格式不正确: $body"
        fi
    else
        log_error "API返回 $http_code"
        echo "$body"
    fi

    # 测试获取单个通道健康状态（假设通道ID为1）
    log_info "测试: GET /api/channel/1/health"
    response=$(curl -s -w "\n%{http_code}" "$API_BASE/api/channel/1/health")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [ "$http_code" = "200" ]; then
        log_success "API返回200"
        if echo "$body" | jq -e '.success' > /dev/null 2>&1; then
            log_success "响应格式正确"
            if echo "$body" | jq -e '.data' > /dev/null 2>&1; then
                log_info "通道1健康数据:"
                echo "$body" | jq '.data'
            else
                log_info "通道1无健康数据（正常情况）"
            fi
        fi
    else
        log_error "API返回 $http_code"
    fi
}

# 测试2: Redis键结构测试
test_redis_keys() {
    log_section "测试2: Redis键结构"

    # 查找所有健康相关的键
    log_info "查找所有 channel:health:* 键"
    keys=$(redis-cli --scan --pattern "channel:health:*" | head -20)

    if [ -z "$keys" ]; then
        log_info "当前没有健康跟踪数据（需要有请求才会产生）"
    else
        key_count=$(echo "$keys" | wc -l)
        log_success "找到 $key_count 个健康相关键"

        # 显示键的示例
        log_info "键示例（前10个）:"
        echo "$keys" | head -10 | while read key; do
            ttl=$(redis-cli ttl "$key")
            type=$(redis-cli type "$key")
            echo "  - $key (type: $type, ttl: $ttl)"
        done
    fi
}

# 测试3: 模拟健康状态记录
test_health_recording() {
    log_section "测试3: 模拟健康状态记录"

    log_info "这个测试需要实际的API请求来触发健康记录"
    log_info "您可以手动发送一些请求到通道，然后观察Redis中的数据变化"
    log_info "建议命令:"
    echo "  redis-cli --scan --pattern 'channel:health:*'"
    echo "  redis-cli get 'channel:health:1:consecutive_failures'"
    echo "  redis-cli get 'channel:health:1:suspended'"
}

# 测试4: 前端列配置测试
test_frontend_column() {
    log_section "测试4: 前端列配置检查"

    log_info "检查前端代码中的列配置..."

    # 检查COLUMN_KEYS
    if grep -q "HEALTH: 'health'" web/src/hooks/channels/useChannelsData.jsx; then
        log_success "COLUMN_KEYS 包含 HEALTH 键"
    else
        log_error "COLUMN_KEYS 缺少 HEALTH 键"
    fi

    # 检查默认可见列
    if grep -q "\[COLUMN_KEYS.HEALTH\]: true" web/src/hooks/channels/useChannelsData.jsx; then
        log_success "默认可见列包含 HEALTH"
    else
        log_error "默认可见列缺少 HEALTH"
    fi

    # 检查ChannelsTable数据传递
    if grep -q "healthInfo," web/src/components/table/channels/ChannelsTable.jsx; then
        log_success "ChannelsTable 正确传递 healthInfo"
    else
        log_error "ChannelsTable 缺少 healthInfo 传递"
    fi
}

# 测试5: 优先级故障转移逻辑检查
test_priority_failover() {
    log_section "测试5: 优先级故障转移逻辑"

    log_info "检查故障转移代码..."

    # 检查model/channel_cache.go
    if grep -q "return nil, nil" model/channel_cache.go; then
        log_success "channel_cache.go 返回 nil, nil 允许重试"
    else
        log_warning "channel_cache.go 可能仍返回错误"
    fi

    # 检查relay.go重试逻辑
    if grep -q "IsSkipRetryError" controller/relay.go; then
        log_success "relay.go 正确检查 SkipRetry 错误"
    else
        log_error "relay.go 缺少 SkipRetry 检查"
    fi

    if grep -q "continue" controller/relay.go | grep -A2 "IsSkipRetryError"; then
        log_success "relay.go 在非SkipRetry错误时继续重试"
    else
        log_warning "relay.go 可能没有正确的继续逻辑"
    fi
}

# 生成测试报告
generate_report() {
    log_section "测试报告"

    echo ""
    log_info "✓ 测试完成"
    log_info ""
    log_info "后续手动测试建议:"
    echo "  1. 访问 http://localhost:3000 查看前端"
    echo "  2. 进入通道管理页面，检查'健康状态'列是否显示"
    echo "  3. 点击健康状态标签，检查详情弹窗是否打开"
    echo "  4. 发送一些API请求，观察健康状态变化"
    echo "  5. 使用 Redis CLI 监控健康数据:"
    echo "     redis-cli monitor | grep 'channel:health'"
    echo ""
    log_info "应用日志位置: /tmp/new-api.log"
    log_info "应用PID文件: /tmp/new-api.pid"
}

# 主函数
main() {
    echo ""
    log_section "🧪 健康跟踪功能测试套件"

    # 切换到项目目录
    cd /usr/src/workspace/github/QQhuxuhui/new-api

    # 检查依赖
    check_dependencies

    # 检查Redis
    check_redis

    # 检查应用状态
    if ! check_app; then
        start_app
    fi

    # 运行测试
    test_api_endpoints
    test_redis_keys
    test_health_recording
    test_frontend_column
    test_priority_failover

    # 生成报告
    generate_report

    echo ""
    log_info "提示: 使用 'tail -f /tmp/new-api.log' 查看实时日志"
    log_info "提示: 使用 'kill \$(cat /tmp/new-api.pid)' 停止应用"
}

# 清理函数（Ctrl+C时）
cleanup() {
    echo ""
    log_warning "收到中断信号，清理中..."
    # 不自动停止应用，让用户决定
    exit 0
}

trap cleanup INT TERM

# 运行主函数
main "$@"
