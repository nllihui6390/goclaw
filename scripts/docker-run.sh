#!/bin/bash
# Docker 运行脚本

set -e

CONTAINER_NAME="go-claw"

echo "=== 启动 go-claw 服务 ==="

# 检查容器是否已存在
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "容器 ${CONTAINER_NAME} 已存在，正在重启..."
    docker restart ${CONTAINER_NAME}
else
    echo "创建并启动新容器..."
    docker compose up -d
fi

echo ""
echo "=== 服务状态 ==="
docker ps --filter "name=${CONTAINER_NAME}"

echo ""
echo "=== 访问地址 ==="
echo "  Web 前端:   http://localhost:8080"
echo "  API 文档:   http://localhost:8080/api/v1"
echo "  健康检查:   http://localhost:8080/health"
echo ""
echo "=== 常用命令 ==="
echo "  查看日志:   docker logs -f ${CONTAINER_NAME}"
echo "  停止服务:   docker stop ${CONTAINER_NAME}"
echo "  重启服务:   docker restart ${CONTAINER_NAME}"
echo ""