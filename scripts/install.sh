#!/usr/bin/env bash
###############################################################################
# go-claw 一键安装脚本 (Linux)
# 功能：自动检测系统、安装依赖(git/go/node)、拉取代码、编译、创建systemd服务
# 源码目录：/data/goclaw-src  (代码、编译工具)
# 运行目录：/data/goclaw      (二进制、配置、数据、日志)
# 用法：curl -fsSL <url>/install.sh | bash
#    或：bash install.sh [--repo <git-url>] [--branch <branch>]
###############################################################################
set -euo pipefail

# ======================== 配置项 ========================
REPO_URL="${REPO_URL:-https://gitee.com/nll/goClaw.git}"
BRANCH="${BRANCH:-master}"
DATA_DIR="/data"
SRC_DIR="${DATA_DIR}/goclaw-src"
RUN_DIR="${DATA_DIR}/goclaw"
SERVICE_NAME="goclaw"
GO_VERSION="1.23.0"
NODE_MAJOR="20"
LOG_PREFIX="[go-claw-install]"

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

# ======================== 前置检查 ========================
check_root() {
    [[ $EUID -eq 0 ]] || die "请使用 root 用户或 sudo 运行此脚本"
}

check_os() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        OS_NAME="${ID,,}"
        OS_VERSION="${VERSION_ID:-}"
        OS_LIKE="${ID_LIKE:-}"
    elif [[ -f /etc/redhat-release ]]; then
        OS_NAME="centos"
        OS_VERSION=$(grep -oP '[0-9]+' /etc/redhat-release | head -1)
    else
        die "无法识别操作系统，仅支持 Linux"
    fi

    case "$OS_NAME" in
        ubuntu|debian|linuxmint|pop|elementary|kali) OS_FAMILY="debian" ;;
        centos|rhel|rocky|almalinux|fedora|amzn|oracle|cloudlinux) OS_FAMILY="rhel" ;;
        alpine) OS_FAMILY="alpine" ;;
        arch|manjaro|endeavouros) OS_FAMILY="arch" ;;
        opensuse*|sles|suse) OS_FAMILY="suse" ;;
        *)
            if echo "$OS_LIKE" | grep -qE 'debian|ubuntu'; then
                OS_FAMILY="debian"
            elif echo "$OS_LIKE" | grep -qE 'rhel|centos|fedora'; then
                OS_FAMILY="rhel"
            elif echo "$OS_LIKE" | grep -qE 'alpine'; then
                OS_FAMILY="alpine"
            elif echo "$OS_LIKE" | grep -qE 'arch'; then
                OS_FAMILY="arch"
            else
                die "不支持的操作系统: ${OS_NAME} ${OS_VERSION}，请手动安装依赖"
            fi
            ;;
    esac

    info "检测到系统: ${OS_NAME} ${OS_VERSION} (族: ${OS_FAMILY})"
}

# ======================== 参数解析 ========================
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --repo)   REPO_URL="$2"; shift 2 ;;
            --branch) BRANCH="$2";  shift 2 ;;
            --data)   DATA_DIR="$2"; SRC_DIR="${DATA_DIR}/goclaw-src"; RUN_DIR="${DATA_DIR}/goclaw"; shift 2 ;;
            -h|--help)
                echo "用法: $0 [选项]"
                echo "  --repo <url>     Git 仓库地址 (默认: ${REPO_URL})"
                echo "  --branch <name>  分支名称   (默认: ${BRANCH})"
                echo "  --data <path>    数据目录   (默认: ${DATA_DIR})"
                exit 0
                ;;
            *) die "未知参数: $1，使用 --help 查看帮助" ;;
        esac
    done
}

# ======================== 包管理 ========================
pkg_update() {
    info "更新软件包索引..."
    case "$OS_FAMILY" in
        debian) apt-get update -y ;;
        rhel)   yum makecache -y 2>/dev/null || dnf makecache -y 2>/dev/null ;;
        alpine) apk update ;;
        arch)   pacman -Sy --noconfirm ;;
        suse)   zypper refresh ;;
    esac
}

pkg_install() {
    local pkgs=("$@")
    info "安装: ${pkgs[*]}"
    case "$OS_FAMILY" in
        debian)  apt-get install -y "${pkgs[@]}" ;;
        rhel)    (yum install -y "${pkgs[@]}" 2>/dev/null || dnf install -y "${pkgs[@]}") ;;
        alpine)  apk add --no-cache "${pkgs[@]}" ;;
        arch)    pacman -S --noconfirm --needed "${pkgs[@]}" ;;
        suse)    zypper install -y "${pkgs[@]}" ;;
    esac
}

pkg_install_if_missing() {
    local cmd="$1" pkg="${2:-$1}"
    if command -v "$cmd" &>/dev/null; then
        ok "$cmd 已安装: $($cmd --version 2>/dev/null | head -1 || echo 'unknown')"
        return
    fi
    pkg_install "$pkg"
}

# ======================== 依赖安装 ========================
install_dependencies() {
    info "安装系统依赖..."
    pkg_update
    case "$OS_FAMILY" in
        debian) pkg_install curl ca-certificates gnupg wget ;;
        *)      pkg_install curl ca-certificates wget ;;
    esac
}

install_git() {
    pkg_install_if_missing git git
}

install_go() {
    if command -v go &>/dev/null; then
        local current_ver
        current_ver=$(go version | grep -oP 'go[0-9]+\.[0-9]+' | head -1)
        local major minor
        major=$(echo "$current_ver" | grep -oP '[0-9]+\.[0-9]+' | cut -d. -f1)
        minor=$(echo "$current_ver" | grep -oP '[0-9]+\.[0-9]+' | cut -d. -f2)
        if [[ "$major" -ge 1 && "$minor" -ge 23 ]]; then
            ok "Go 已安装: $current_ver"
            return
        fi
        warn "Go 版本过低: $current_ver，需要 >= 1.23，将安装 $GO_VERSION"
    fi

    info "安装 Go $GO_VERSION ..."
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)  arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        armv7l|armv7)  arch="armv6l" ;;
        *) die "不支持的架构: $arch" ;;
    esac

    local tarball="go${GO_VERSION}.linux-${arch}.tar.gz"
    local url="https://go.dev/dl/${tarball}"

    info "下载 ${url} ..."
    wget -q --show-progress -O "/tmp/${tarball}" "$url" || \
        die "Go 下载失败，请手动下载 ${url} 到 /tmp/${tarball} 后重新运行"

    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"

    if ! grep -q '/usr/local/go/bin' /etc/profile; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    fi
    export PATH=$PATH:/usr/local/go/bin

    export GOPATH=/root/go
    export GO111MODULE=on
    go env -w GOPROXY=https://goproxy.cn,direct 2>/dev/null || true

    ok "Go 安装完成: $(go version)"
}

install_node() {
    if command -v node &>/dev/null; then
        local current_ver
        current_ver=$(node -v | grep -oP '[0-9]+' | head -1)
        if [[ "$current_ver" -ge 18 ]]; then
            ok "Node.js 已安装: $(node -v)"
            return
        fi
        warn "Node.js 版本过低: $(node -v)，需要 >= 18，将安装 Node.js $NODE_MAJOR"
    fi

    info "安装 Node.js $NODE_MAJOR ..."

    case "$OS_FAMILY" in
        debian)
            mkdir -p /etc/apt/keyrings
            curl -fsSL "https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key" \
                | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg 2>/dev/null || \
                wget -qO- "https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key" \
                | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg
            echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
                > /etc/apt/sources.list.d/nodesource.list
            apt-get update -y
            pkg_install nodejs
            ;;
        rhel|suse)
            curl -fsSL "https://rpm.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
            pkg_install nodejs
            ;;
        alpine|arch)
            pkg_install nodejs npm
            ;;
    esac

    ok "Node.js 安装完成: $(node -v), npm $(npm -v)"
}

# ======================== 代码拉取 ========================
clone_repo() {
    if [[ -d "${SRC_DIR}/.git" ]]; then
        info "代码已存在，执行 git pull ..."
        cd "${SRC_DIR}"
        git fetch origin
        git checkout "$BRANCH" || git checkout -b "$BRANCH" "origin/$BRANCH"
        git pull origin "$BRANCH"
        ok "代码更新完成"
        return
    fi

    info "从 ${REPO_URL} 拉取代码到 ${SRC_DIR} (分支: ${BRANCH}) ..."
    mkdir -p "$SRC_DIR"
    git clone -b "$BRANCH" --depth 1 "$REPO_URL" "$SRC_DIR" || {
        warn "浅克隆失败，尝试完整克隆..."
        rm -rf "$SRC_DIR"
        mkdir -p "$SRC_DIR"
        git clone -b "$BRANCH" "$REPO_URL" "$SRC_DIR" || die "代码拉取失败"
    }
    ok "代码拉取完成"
}

# ======================== 编译 ========================
build_frontend() {
    info "编译前端..."
    cd "${SRC_DIR}/frontend"
    npm install
    npm run build
    ok "前端编译完成"
}

build_backend() {
    info "编译后端..."
    cd "$SRC_DIR"

    export PATH=$PATH:/usr/local/go/bin
    export GOPATH=/root/go
    go mod download

    CGO_ENABLED=1 go build -tags server -o "${SRC_DIR}/go-claw-server" .
    ok "后端编译完成: $(ls -lh "${SRC_DIR}/go-claw-server" | awk '{print $5}')"
}

# ======================== 部署到运行目录 ========================
deploy_to_run() {
    info "部署到运行目录 ${RUN_DIR} ..."

    mkdir -p "${RUN_DIR}/clawdata/skills"
    mkdir -p "${RUN_DIR}/clawdata/workspaces"
    mkdir -p "${RUN_DIR}/logs"
    mkdir -p "${RUN_DIR}/frontend"

    # 复制编译产物
    cp -f "${SRC_DIR}/go-claw-server" "${RUN_DIR}/go-claw-server"
    ok "二进制文件已复制"

    # 复制前端构建产物
    if [[ -d "${SRC_DIR}/frontend/dist" ]]; then
        rm -rf "${RUN_DIR}/frontend/dist"
        cp -a "${SRC_DIR}/frontend/dist" "${RUN_DIR}/frontend/dist"
        ok "前端文件已复制"
    fi

    # 复制 .env.example（不覆盖已存在的）
    if [[ -f "${SRC_DIR}/.env.example" && ! -f "${RUN_DIR}/.env" ]]; then
        cp "${SRC_DIR}/.env.example" "${RUN_DIR}/.env"
    fi

    ok "部署完成"
}

# ======================== 安装更新脚本 ========================
install_update_script() {
    info "安装更新脚本到 ${RUN_DIR}/update.sh ..."

    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

    cat > "${RUN_DIR}/update.sh" <<'UPDATE_SCRIPT_EOF'
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
UPDATE_SCRIPT_EOF

    chmod +x "${RUN_DIR}/update.sh"
    ok "更新脚本已安装: ${RUN_DIR}/update.sh"
}

# ======================== 系统服务 ========================
create_service() {
    info "创建 systemd 服务..."

    cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=Go Claw AI Agent Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=${RUN_DIR}
ExecStart=${RUN_DIR}/go-claw-server
Restart=on-failure
RestartSec=5
StandardOutput=append:${RUN_DIR}/logs/goclaw.log
StandardError=append:${RUN_DIR}/logs/goclaw-error.log

# 环境变量
Environment="TZ=Asia/Shanghai"
Environment="PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin"
Environment="GOPATH=/root/go"
Environment="GO111MODULE=on"

# 资源限制
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable "${SERVICE_NAME}.service"

    ok "systemd 服务已创建: ${SERVICE_NAME}.service"
}

# ======================== 主流程 ========================
main() {
    parse_args "$@"

    cd /

    echo ""
    echo "╔══════════════════════════════════════════════════════╗"
    echo "║           go-claw 一键安装脚本                        ║"
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  仓库:     ${REPO_URL}"
    echo "║  分支:     ${BRANCH}"
    echo "║  源码目录: ${SRC_DIR} (代码+编译)"
    echo "║  运行目录: ${RUN_DIR} (配置+数据)"
    echo "╚══════════════════════════════════════════════════════╝"
    echo ""

    check_root
    check_os

    echo ""
    info "开始安装..."
    echo ""

    # 1. 安装系统依赖
    info "[1/8] 安装系统依赖..."
    install_dependencies
    ok "[1/8] 系统依赖安装完成"

    # 2. 安装 git
    info "[2/8] 检查/安装 git..."
    install_git
    ok "[2/8] git 安装完成"

    # 3. 安装 Go
    info "[3/8] 检查/安装 Go..."
    install_go
    ok "[3/8] Go 安装完成"

    # 4. 安装 Node.js
    info "[4/8] 检查/安装 Node.js..."
    install_node
    ok "[4/8] Node.js 安装完成"

    # 5. 拉取代码
    info "[5/8] 拉取代码到 ${SRC_DIR} ..."
    clone_repo
    ok "[5/8] 代码拉取完成"

    # 6. 编译前端
    info "[6/8] 编译前端..."
    build_frontend
    ok "[6/8] 前端编译完成"

    # 7. 编译后端
    info "[7/8] 编译后端..."
    build_backend
    ok "[7/8] 后端编译完成"

    # 8. 部署 + 配置 + 服务
    info "[8/8] 部署到运行目录、创建服务..."
    deploy_to_run
    create_service
    install_update_script
    ok "[8/8] 部署与服务创建完成"

    # ======================== 完成 ========================
    echo ""
    echo "╔══════════════════════════════════════════════════════╗"
    echo "║             安装完成！                                ║"
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  源码目录:  ${SRC_DIR} (代码、编译产物)"
    echo "║  运行目录:  ${RUN_DIR} (二进制、配置、数据、日志)"
    echo "║  配置文件:  ${RUN_DIR}/config.json"
    echo "║  日志目录:  ${RUN_DIR}/logs/"
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  启动服务:  systemctl start ${SERVICE_NAME}"
    echo "║  停止服务:  systemctl stop ${SERVICE_NAME}"
    echo "║  查看状态:  systemctl status ${SERVICE_NAME}"
    echo "║  查看日志:  tail -f ${RUN_DIR}/logs/goclaw.log"
    echo "║  更新脚本:  ${RUN_DIR}/update.sh"
    echo "╚══════════════════════════════════════════════════════╝"
    echo ""

    read -rp "是否立即启动 ${SERVICE_NAME} 服务? [y/N]: " answer
    case "$answer" in
        [yY][eE][sS]|[yY])
            systemctl start "${SERVICE_NAME}"
            ok "服务已启动"
            sleep 2
            systemctl status "${SERVICE_NAME}" --no-pager || true
            ;;
        *)
            info "未启动服务，请手动运行: systemctl start ${SERVICE_NAME}"
            ;;
    esac
}

main "$@"
