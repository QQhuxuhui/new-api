#!/bin/bash

# New API 快速启动脚本
# Quick Start Script for New API

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_section() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
}

# 检查命令是否存在
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# 检查环境
check_environment() {
    print_section "环境检查 (Environment Check)"

    # 检查 Go
    if command_exists go; then
        GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
        print_success "Go 已安装: $GO_VERSION"
    else
        print_error "Go 未安装，请先安装 Go 1.25.1+"
        echo "下载地址: https://go.dev/dl/"
        exit 1
    fi

    # 检查 Node.js
    if command_exists node; then
        NODE_VERSION=$(node --version)
        print_success "Node.js 已安装: $NODE_VERSION"
    else
        print_error "Node.js 未安装，请先安装 Node.js 18+"
        echo "下载地址: https://nodejs.org/"
        exit 1
    fi

    # 检查 Bun
    if command_exists bun; then
        BUN_VERSION=$(bun --version)
        print_success "Bun 已安装: $BUN_VERSION"
        USE_BUN=true
    else
        print_warning "Bun 未安装，将使用 npm（速度较慢）"
        print_info "安装 Bun: curl -fsSL https://bun.sh/install | bash"
        USE_BUN=false
    fi

    # 检查 Redis
    if command_exists redis-cli; then
        if redis-cli ping > /dev/null 2>&1; then
            print_success "Redis 运行正常"
        else
            print_warning "Redis 未运行，尝试启动..."
            if command_exists systemctl; then
                sudo systemctl start redis 2>/dev/null || true
            fi
            if redis-cli ping > /dev/null 2>&1; then
                print_success "Redis 已启动"
            else
                print_error "无法连接到 Redis，请手动启动"
                echo "启动命令: redis-server"
                exit 1
            fi
        fi
    else
        print_error "Redis 未安装"
        echo "安装命令 (Ubuntu): sudo apt-get install redis-server"
        echo "安装命令 (macOS): brew install redis"
        exit 1
    fi
}

# 检查配置文件
check_config() {
    print_section "配置检查 (Configuration Check)"

    if [ ! -f ".env" ]; then
        print_warning ".env 文件不存在"

        if [ -f ".env.local.example" ]; then
            print_info "从 .env.local.example 创建 .env"
            cp .env.local.example .env
            print_success ".env 文件已创建"
            print_warning "请编辑 .env 文件，填写正确的数据库密码"
            echo ""
            echo "编辑命令: vim .env"
            echo "需要修改: SQL_DSN 中的密码"
            echo ""
            read -p "是否现在编辑？(y/n) " -n 1 -r
            echo
            if [[ $REPLY =~ ^[Yy]$ ]]; then
                ${EDITOR:-vim} .env
            else
                print_warning "请稍后手动编辑 .env 文件"
            fi
        else
            print_error "配置模板文件不存在"
            exit 1
        fi
    else
        print_success ".env 配置文件已存在"

        # 检查是否包含默认密码
        if grep -q "123456" .env; then
            print_warning ".env 中可能使用默认密码，请确认是否正确"
        fi
    fi
}

# 安装 Go 依赖
install_go_deps() {
    print_section "安装 Go 依赖 (Installing Go Dependencies)"

    print_info "下载 Go 模块..."
    go mod download

    print_info "验证 Go 模块..."
    go mod verify

    print_success "Go 依赖安装完成"
}

# 安装前端依赖
install_web_deps() {
    print_section "安装前端依赖 (Installing Web Dependencies)"

    cd web

    if [ "$USE_BUN" = true ]; then
        print_info "使用 Bun 安装前端依赖..."
        bun install
    else
        print_info "使用 npm 安装前端依赖..."
        npm install
    fi

    cd ..

    print_success "前端依赖安装完成"
}

# 构建前端
build_web() {
    print_section "构建前端 (Building Web Frontend)"

    cd web

    # 检查是否需要重新构建
    if [ -f "dist/index.html" ] && [ -d "dist/assets" ]; then
        print_info "检测到已有构建产物"
        read -p "是否重新构建前端？(y/n) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            cd ..
            print_info "跳过前端构建"
            return
        fi
    fi

    print_info "构建前端资源..."

    if [ "$USE_BUN" = true ]; then
        bun run build
    else
        npm run build
    fi

    # 验证构建产物
    if [ ! -f "dist/index.html" ]; then
        print_error "前端构建失败：dist/index.html 不存在"
        cd ..
        exit 1
    fi

    cd ..

    print_success "前端构建完成"
}

# 构建后端
build_backend() {
    print_section "构建后端 (Building Backend)"

    print_info "编译 Go 程序..."

    go build -o new-api main.go

    if [ ! -f "new-api" ]; then
        print_error "后端编译失败"
        exit 1
    fi

    print_success "后端编译完成"
}

# 运行项目
run_project() {
    print_section "启动项目 (Starting Project)"

    print_info "正在启动 New API..."
    echo ""
    print_info "访问地址: http://localhost:3000"
    print_info "默认账号: root"
    print_info "默认密码: 123456"
    echo ""
    print_warning "按 Ctrl+C 停止服务"
    echo ""

    # 运行
    ./new-api
}

# 主函数
main() {
    echo ""
    echo "╔════════════════════════════════════════╗"
    echo "║   New API 快速启动脚本                 ║"
    echo "║   Quick Start Script                   ║"
    echo "╚════════════════════════════════════════╝"
    echo ""

    # 1. 检查环境
    check_environment

    # 2. 检查配置
    check_config

    # 3. 询问是否安装依赖
    echo ""
    read -p "是否安装/更新依赖？(y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        install_go_deps
        install_web_deps
    else
        print_info "跳过依赖安装"
    fi

    # 4. 构建前端
    build_web

    # 5. 构建后端
    build_backend

    # 6. 运行项目
    run_project
}

# 运行主函数
main
