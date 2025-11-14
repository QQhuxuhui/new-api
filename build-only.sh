#!/bin/bash
set -e

# 配置变量
REGISTRY="registry.cn-shanghai.aliyuncs.com"
NAMESPACE="hxh_ai"
IMAGE_NAME="new-api"
VERSION_FILE=".docker-version"

# 检查版本文件
if [ ! -f "$VERSION_FILE" ]; then
    echo "0" > "$VERSION_FILE"
fi

# 读取并递增版本号
CURRENT_VERSION=$(cat "$VERSION_FILE")
NEW_VERSION=$((CURRENT_VERSION + 1))

# 完整镜像名
FULL_IMAGE="${REGISTRY}/${NAMESPACE}/${IMAGE_NAME}"

echo "================================"
echo "构建 Docker 镜像（仅构建）"
echo "================================"
echo "镜像: ${FULL_IMAGE}:v${NEW_VERSION}"
echo "================================"

# 更新 VERSION 文件
echo "v${NEW_VERSION}" > VERSION

# 构建镜像
docker build \
    -t ${FULL_IMAGE}:v${NEW_VERSION} \
    -t ${FULL_IMAGE}:latest \
    .

# 保存新版本号
echo "$NEW_VERSION" > "$VERSION_FILE"

echo ""
echo "================================"
echo "✓ 构建完成！"
echo "================================"
echo "版本号: v${NEW_VERSION}"
echo "镜像标签:"
echo "  - ${FULL_IMAGE}:v${NEW_VERSION}"
echo "  - ${FULL_IMAGE}:latest"
echo ""
echo "推送镜像命令:"
echo "  docker push ${FULL_IMAGE}:v${NEW_VERSION}"
echo "  docker push ${FULL_IMAGE}:latest"
echo "================================"
