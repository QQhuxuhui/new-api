#!/bin/bash

VERSION_FILE=".docker-version"

# 颜色
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 初始化版本文件
if [ ! -f "$VERSION_FILE" ]; then
    echo "0" > "$VERSION_FILE"
fi

case "$1" in
    show|current)
        VERSION=$(cat "$VERSION_FILE")
        echo -e "${GREEN}当前版本号: v${VERSION}${NC}"
        ;;
    next)
        CURRENT=$(cat "$VERSION_FILE")
        NEXT=$((CURRENT + 1))
        echo -e "${GREEN}下次构建版本号: v${NEXT}${NC}"
        ;;
    set)
        if [ -z "$2" ]; then
            echo "用法: $0 set <版本号>"
            exit 1
        fi
        echo "$2" > "$VERSION_FILE"
        echo -e "${GREEN}版本号已设置为: v${2}${NC}"
        ;;
    reset)
        echo "0" > "$VERSION_FILE"
        echo -e "${YELLOW}版本号已重置为: v0${NC}"
        ;;
    *)
        echo "用法: $0 {show|next|set <版本号>|reset}"
        echo ""
        echo "命令:"
        echo "  show   - 显示当前版本号"
        echo "  next   - 显示下次构建的版本号"
        echo "  set    - 设置版本号"
        echo "  reset  - 重置版本号为 0"
        exit 1
        ;;
esac
