#!/usr/bin/env bash
###############################################################################
# go-claw 一键更新脚本 (Linux)
# 功能：拉取最新代码 → 备份旧版 → 重新编译 → 部署 → 重启服务 → 失败自动回滚
# 源码目录：/data/goclaw-src  (代码、编译工具)
# 运行目录：/data/goclaw      (二进制、配置、数据、日志)
# 用法：bash update.sh [--branch <branch>] [--data <data-dir>] [--restart|--no-restart]
###############################################################################
set -euo pipefail

# ======================== 配置项 ========================
REPO_URL="${REPO_URL:-https://gitee.com/nll/goClaw.git}"
BRANCH="${BRANCH:-master}"
DATA_DIR="/data"
SRC_DIR="${DATA_DIR}/goclaw-src"
RUN_DIR="${DATA_DIR}/goclaw"
SERVICE_NAME="goclaw"
BACKUP_BIN="${RUN_DIR}/go-claw-server.bak.$(date +%Y%m%d%H%M%S)"
LOG_PREFIX="[go-claw-update]"

# ======================== 颜色输出 ========================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}${LOG_PREFIX} $*${NC}"; }
ok()    { echo -e "${GREEN}${LOG_PREFIX} [OK] $*${NC}"; }
warn()  { echo -e "${YELLOW}${LOG_PREFIX} [WARN] $*${NC}"; }
error() { echo -e "${RED}${LOG_PREFIX} [ERROR] $*${NC}" >&2; }
die()   { error "$*"; exit 1; }

# ======================== 参数解析 ========================
AUTO_RESTART="yes"

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --branch)     BRANCH="$2";        shift 2 ;;
            --data)       DATA_DIR="$2"; SRC_DIR="${DATA_DIR}/goclaw-src"; RUN_DIR="${DATA_DIR}/goclaw"; shift 2 ;;
            --restart)    AUTO_RESTART="yes"; shift ;;
            --no-restart) AUTO_RESTART="no";  shift ;;
            -h|--help)
                echo "用法: $0 [选项]"
                echo "  --branch <name>  分支名称 (默认: ${BRANCH})"
                echo "  --data <path>    数据目录 (默认: ${DATA_DIR})"
                echo "  --restart        更新后自动重启服务 (默认)"
                echo "  --no-restart     更新后不重启服务"
                exit 0
                ;;
            *) die "未知参数: $1，使用 --help 查看帮助" ;;
        esac
    done
}

# ======================== 前置检查 ========================
check_installed() {
    [[ -d "${SRC_DIR}/.git" ]] || die "${SRC_DIR} 不存在或不是有效的 git 仓库，请先运行安装脚本"
    [[ -f "${RUN_DIR}/go-claw-server" ]] || die "${RUN_DIR}/go-claw-server 不存在，请先完成安装"

    export PATH=$PATH:/usr/local/go/bin
    export GOPATH=/root/go

    command -v git &>/dev/null    || die "git 未安装"
    command -v go  &>/dev/null    || die "Go 未安装，请安装后重试"
    command -v node &>/dev/null   || die "Node.js 未安装，请安装后重试"

    info "当前版本: $(cd "$SRC_DIR" && git log --oneline -1)"
}

# ======================== 拉取最新代码 ========================
pull_code() {
    cd "${SRC_DIR}"

    git checkout "$BRANCH" 2>/dev/null || git checkout -b "$BRANCH" "origin/$BRANCH"

    info "拉取最新代码 (分支: ${BRANCH}) ..."
    git fetch origin --quiet
    local before_commit
    before_commit=$(git rev-parse HEAD)

    git pull origin "$BRANCH" || {
        warn "git pull 失败，尝试 stash 后重试..."
        git stash --quiet
        git pull origin "$BRANCH" || die "代码更新失败"
    }

    local after_commit
    after_commit=$(git rev-parse HEAD)

    if [[ "$before_commit" == "$after_commit" ]]; then
        ok "代码已是最新，无需更新"
        return 1
    fi

    ok "代码已更新: $(git log --oneline -1)"
    return 0
}

# ======================== 备份旧版二进制 ========================
do_backup() {
    info "备份当前二进制文件..."
    cp -f "${RUN_DIR}/go-claw-server" "$BACKUP_BIN"
    if [[ -d "${RUN_DIR}/frontend/dist" ]]; then
        cp -a "${RUN_DIR}/frontend/dist" "${RUN_DIR}/frontend/dist.bak"
    fi
    ok "备份完成"
}

rollback() {
    warn "更新失败，正在回滚..."
    if [[ -f "$BACKUP_BIN" ]]; then
        cp -f "$BACKUP_BIN" "${RUN_DIR}/go-claw-server"
        rm -f "$BACKUP_BIN"
    fi
    if [[ -d "${RUN_DIR}/frontend/dist.bak" ]]; then
        rm -rf "${RUN_DIR}/frontend/dist"
        mv "${RUN_DIR}/frontend/dist.bak" "${RUN_DIR}/frontend/dist"
    fi
    # 恢复 git
    cd "${SRC_DIR}"
    git stash --quiet 2>/dev/null || true
    git reset --hard HEAD --quiet 2>/dev/null || true
    git checkout "$BRANCH" --quiet 2>/dev/null || true
    ok "已回滚到更新前状态"
}

# ======================== 编译 ========================
build_frontend() {
    info "编译前端..."
    cd "${SRC_DIR}/frontend"
    npm install --quiet
    npm run build
    ok "前端编译完成"
}

build_backend() {
    info "编译后端..."
    cd "$SRC_DIR"
    go mod download
    CGO_ENABLED=1 go build -tags server -o "${SRC_DIR}/go-claw-server" .
    ok "后端编译完成: $(ls -lh "${SRC_DIR}/go-claw-server" | awk '{print $5}')"
}

# ======================== 部署 ========================
deploy() {
    info "部署到运行目录 ${RUN_DIR} ..."
    mkdir -p "${RUN_DIR}/frontend"

    cp -f "${SRC_DIR}/go-claw-server" "${RUN_DIR}/go-claw-server"

    if [[ -d "${SRC_DIR}/frontend/dist" ]]; then
        rm -rf "${RUN_DIR}/frontend/dist"
        cp -a "${SRC_DIR}/frontend/dist" "${RUN_DIR}/frontend/dist"
    fi

    # 清理备份
    rm -f "$BACKUP_BIN"
    rm -rf "${RUN_DIR}/frontend/dist.bak" 2>/dev/null || true

    ok "部署完成"
}

# ======================== 重启服务 ========================
restart_service() {
    if [[ "$AUTO_RESTART" != "yes" ]]; then
        info "跳过服务重启 (--no-restart)"
        return
    fi

    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        info "重启服务 ${SERVICE_NAME} ..."
        systemctl restart "${SERVICE_NAME}"
        sleep 2
        if systemctl is-active --quiet "${SERVICE_NAME}"; then
            ok "服务重启成功"
        else
            warn "服务重启后状态异常，请检查: systemctl status ${SERVICE_NAME}"
        fi
    else
        info "服务 ${SERVICE_NAME} 未运行，跳过重启"
    fi
}

# ======================== 主流程 ========================
main() {
    parse_args "$@"
    cd /

    echo ""
    echo "╔══════════════════════════════════════════════════════╗"
    echo "║           go-claw 一键更新脚本                        ║"
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  源码目录: ${SRC_DIR}"
    echo "║  运行目录: ${RUN_DIR}"
    echo "║  分支:     ${BRANCH}"
    echo "║  自动重启: ${AUTO_RESTART}"
    echo "╚══════════════════════════════════════════════════════╝"
    echo ""

    check_installed

    info "开始更新..."
    echo ""

    # 1. 拉取最新代码
    info "[1/5] 拉取最新代码..."
    pulled=true
    pull_code || pulled=false
    if [[ "$pulled" != "true" ]]; then
        ok "[1/5] 无需更新"
        echo ""
        echo "╔══════════════════════════════════════════════════════╗"
        echo "║             已是最新版本，无需更新！                    ║"
        echo "╚══════════════════════════════════════════════════════╝"
        echo ""
        exit 0
    fi
    ok "[1/5] 代码拉取完成"

    # 2. 备份旧版
    info "[2/5] 备份旧版二进制..."
    do_backup
    ok "[2/5] 备份完成"

    # 3. 编译
    info "[3/5] 重新编译..."
    trap 'rollback; exit 1' ERR
    build_frontend
    build_backend
    ok "[3/5] 编译完成"

    # 4. 部署
    info "[4/5] 部署到运行目录..."
    deploy
    trap - ERR
    ok "[4/5] 部署完成"

    # 5. 重启服务
    info "[5/5] 重启服务..."
    restart_service
    ok "[5/5] 服务处理完成"

    # ======================== 完成 ========================
    echo ""
    echo "╔══════════════════════════════════════════════════════╗"
    echo "║             更新完成！                                ║"
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  当前版本:  $(cd "$SRC_DIR" && git log --oneline -1)"
    echo "║  日志目录:  ${RUN_DIR}/logs/"
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  查看状态:  systemctl status ${SERVICE_NAME}"
    echo "║  查看日志:  tail -f ${RUN_DIR}/logs/goclaw.log"
    echo "╚══════════════════════════════════════════════════════╝"
    echo ""
}

main "$@"
