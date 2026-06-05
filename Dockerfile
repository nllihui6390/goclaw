# Stage 1: Frontend Builder
FROM node:20-alpine AS frontend-builder

WORKDIR /frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install

COPY frontend/ ./
RUN npm run build

# Stage 2: Backend Builder
FROM golang:1.23-alpine AS backend-builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -tags server -o /go-claw-server .

# Stage 3: Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

# 设置时区为中国
ENV TZ=Asia/Shanghai

WORKDIR /app

# 复制后端程序
COPY --from=backend-builder /go-claw-server .

# 复制前端构建产物
COPY --from=frontend-builder /frontend/dist ./frontend/dist

# 复制默认配置
COPY config.json .

# 创建数据目录
RUN mkdir -p /app/clawdata/skills /app/clawdata/workspaces /app/logs

# 暴露端口：8080 (HTTP API + 前端), 8081 (WebSocket)
EXPOSE 8080 8081

# 数据卷：配置文件、数据目录、日志
VOLUME ["/app/config.json", "/app/clawdata", "/app/logs"]

ENTRYPOINT ["/go-claw-server"]