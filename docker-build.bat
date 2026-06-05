@echo off
REM Docker 构建脚本 (Windows)

set IMAGE_NAME=go-claw
set IMAGE_TAG=latest
set CONTAINER_NAME=go-claw

echo === 构建 Docker 镜像 ===
docker build -t %IMAGE_NAME%:%IMAGE_TAG% .

if %errorlevel% neq 0 (
    echo 构建失败！
    exit /b 1
)

echo.
echo === 镜像信息 ===
docker images %IMAGE_NAME%:%IMAGE_TAG%

echo.
echo 构建完成！
echo.
echo 运行方式：
echo   1. 使用 docker-compose:  docker compose up -d
echo   2. 直接运行:             docker run -d -p 8080:8080 -p 8081:8081 --name %CONTAINER_NAME% %IMAGE_NAME%:%IMAGE_TAG%
echo.