#!/bin/bash
# Docker 构建脚本

set -e

IMAGE_NAME="go-claw"
IMAGE_TAG="latest"
CONTAINER_NAME="go-claw"

echo "=== 构建 Docker 镜像 ==="
docker build -t ${IMAGE_NAME}:${IMAGE_TAG} .

echo "=== 镜像信息 ==="
docker images ${IMAGE_NAME}:${IMAGE_TAG}

echo ""
echo "构建完成！"
echo ""
echo "运行方式："
echo "  1. 使用 docker-compose:  docker compose up -d"
echo "  2. 直接运行:             docker run -d -p 8080:8080 -p 8081:8081 --name ${CONTAINER_NAME} ${IMAGE_NAME}:${IMAGE_TAG}"
echo ""