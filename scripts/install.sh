#!/usr/bin/env bash
###############################################################################
# go-claw 一键安装脚本 (Linux)
# 功能：自动检测系统、安装依赖(git/go/node)、拉取代码、编译、创建systemd服务
# 运行目录：/goclaw
# 用法：curl -fsSL <url>/install.sh | bash
#    或：bash install.sh [--repo <git-url>] [--branch <branch>]
###############################################################################
set -euo pipefail

# ======================== 配置项 ========================
REPO_URL="${REPO_URL:-https://github.com/nllihui6390/goclaw.git}"
BRANCH="${BRANCH:-master}"
INSTALL_DIR="/goclaw"
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
        # shellcheck source=/dev/null
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

    # 归类为发行版族
    case "$OS_NAME" in
        ubuntu|debian|linuxmint|pop|elementary|kali)
            OS_FAMILY="debian"
            ;;
        centos|rhel|rocky|almalinux|fedora|amzn|oracle|cloudlinux)
            OS_FAMILY="rhel"
            ;;
        alpine)
            OS_FAMILY="alpine"
            ;;
        arch|manjaro|endeavouros)
            OS_FAMILY="arch"
            ;;
        opensuse*|sles|suse)
            OS_FAMILY="suse"
            ;;
        *)
            # 尝试通过 ID_LIKE 判断
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
            --dir)    INSTALL_DIR="$2"; shift 2 ;;
            -h|--help)
                echo "用法: $0 [选项]"
                echo "  --repo <url>     Git 仓库地址 (默认: ${REPO_URL})"
                echo "  --branch <name>  分支名称   (默认: ${BRANCH})"
                echo "  --dir <path>     安装目录   (默认: ${INSTALL_DIR})"
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
        debian)
            apt-get install -y "${pkgs[@]}"
            ;;
        rhel)
            (yum install -y "${pkgs[@]}" 2>/dev/null || dnf install -y "${pkgs[@]}")
            ;;
        alpine)
            apk add --no-cache "${pkgs[@]}"
            ;;
        arch)
            pacman -S --noconfirm --needed "${pkgs[@]}"
            ;;
        suse)
            zypper install -y "${pkgs[@]}"
            ;;
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
        debian)
            pkg_install curl ca-certificates gnupg wget
            ;;
        rhel)
            pkg_install curl ca-certificates wget
            ;;
        alpine)
            pkg_install curl ca-certificates wget
            ;;
        arch)
            pkg_install curl ca-certificates wget
            ;;
        suse)
            pkg_install curl ca-certificates wget
            ;;
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

    # 移除旧版本
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"

    # 确保 PATH 包含 /usr/local/go/bin
    if ! grep -q '/usr/local/go/bin' /etc/profile; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    fi
    export PATH=$PATH:/usr/local/go/bin

    # 配置 Go 模块代理
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
            # 使用 NodeSource 仓库
            local codename
            codename=$(lsb_release -cs 2>/dev/null || echo "stable")
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
        rhel)
            curl -fsSL "https://rpm.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
            pkg_install nodejs
            ;;
        alpine)
            # Alpine 3.x 自带 nodejs 包，通常版本较新
            pkg_install nodejs npm
            ;;
        arch)
            pkg_install nodejs npm
            ;;
        suse)
            curl -fsSL "https://rpm.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
            pkg_install nodejs
            ;;
    esac

    ok "Node.js 安装完成: $(node -v), npm $(npm -v)"
}

# ======================== 代码拉取 ========================
clone_repo() {
    if [[ -d "${INSTALL_DIR}/.git" ]]; then
        info "代码已存在，执行 git pull ..."
        cd "${INSTALL_DIR}"
        git fetch origin
        git checkout "$BRANCH" || git checkout -b "$BRANCH" "origin/$BRANCH"
        git pull origin "$BRANCH"
        ok "代码更新完成"
        return
    fi

    info "从 ${REPO_URL} 拉取代码 (分支: ${BRANCH}) ..."

    # 目录已存在但不是 git 仓库：先备份再清空
    if [[ -d "$INSTALL_DIR" ]]; then
        warn "${INSTALL_DIR} 已存在但不是有效的 git 仓库"
        local backup="${INSTALL_DIR}.bak.$(date +%Y%m%d%H%M%S)"
        info "备份到 ${backup} ..."
        mv "$INSTALL_DIR" "$backup"
        ok "备份完成"
    fi

    mkdir -p "$INSTALL_DIR"
    git clone -b "$BRANCH" --depth 1 "$REPO_URL" "$INSTALL_DIR" || {
        # 浅克隆失败时尝试完整克隆
        warn "浅克隆失败，尝试完整克隆..."
        rm -rf "$INSTALL_DIR"
        mkdir -p "$INSTALL_DIR"
        git clone -b "$BRANCH" "$REPO_URL" "$INSTALL_DIR" || die "代码拉取失败"
    }
    ok "代码拉取完成"
}

# ======================== 编译 ========================
build_frontend() {
    info "编译前端..."
    cd "${INSTALL_DIR}/frontend"
    npm install
    npm run build
    ok "前端编译完成"
}

build_backend() {
    info "编译后端..."
    cd "$INSTALL_DIR"

    # 下载 Go 依赖
    export PATH=$PATH:/usr/local/go/bin
    export GOPATH=/root/go
    go mod download

    # 编译
    CGO_ENABLED=1 go build -tags server -o "${INSTALL_DIR}/go-claw-server" .
    ok "后端编译完成: $(ls -lh "${INSTALL_DIR}/go-claw-server" | awk '{print $5}')"
}

# ======================== 配置 ========================
setup_config() {
    info "初始化配置..."

    # 创建运行目录
    mkdir -p "${INSTALL_DIR}/clawdata/skills"
    mkdir -p "${INSTALL_DIR}/clawdata/workspaces"
    mkdir -p "${INSTALL_DIR}/logs"

    # 如果不存在 config.json，则复制示例
    if [[ ! -f "${INSTALL_DIR}/config.json" ]]; then
        if [[ -f "${INSTALL_DIR}/config.json.example" ]]; then
            cp "${INSTALL_DIR}/config.json.example" "${INSTALL_DIR}/config.json"
            info "已复制 config.json.example -> config.json"
        else
            warn "未找到 config.json 或 config.json.example，请手动创建 config.json"
        fi
    fi

    # 复制 .env.example
    if [[ -f "${INSTALL_DIR}/.env.example" && ! -f "${INSTALL_DIR}/.env" ]]; then
        cp "${INSTALL_DIR}/.env.example" "${INSTALL_DIR}/.env"
        info "已复制 .env.example -> .env"
    fi

    ok "配置初始化完成"
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
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/go-claw-server
Restart=on-failure
RestartSec=5
StandardOutput=append:${INSTALL_DIR}/logs/goclaw.log
StandardError=append:${INSTALL_DIR}/logs/goclaw-error.log

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

    # 重载并启用服务
    systemctl daemon-reload
    systemctl enable "${SERVICE_NAME}.service"

    ok "systemd 服务已创建: ${SERVICE_NAME}.service"
}

# ======================== 主流程 ========================
main() {
    parse_args "$@"

    # 切换到安全目录，避免操作安装目录时 CWD 被删除导致脚本崩溃
    cd /

    echo ""
    echo "╔══════════════════════════════════════════════════════╗"
    echo "║           go-claw 一键安装脚本                        ║"
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  仓库:    ${REPO_URL}"
    echo "║  分支:    ${BRANCH}"
    echo "║  安装目录: ${INSTALL_DIR}"
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
    info "[5/8] 拉取代码..."
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

    # 8. 配置与服务
    info "[8/8] 初始化配置与创建服务..."
    setup_config
    create_service
    ok "[8/8] 配置与服务创建完成"

    # ======================== 完成 ========================
    echo ""
    echo "╔══════════════════════════════════════════════════════╗"
    echo "║             安装完成！                                ║"
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  安装目录:  ${INSTALL_DIR}"
    echo "║  服务名称:  ${SERVICE_NAME}"
    echo "║  配置文件:  ${INSTALL_DIR}/config.json"
    echo "║  日志目录:  ${INSTALL_DIR}/logs/"
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  启动服务:  systemctl start ${SERVICE_NAME}"
    echo "║  停止服务:  systemctl stop ${SERVICE_NAME}"
    echo "║  查看状态:  systemctl status ${SERVICE_NAME}"
    echo "║  查看日志:  tail -f ${INSTALL_DIR}/logs/goclaw.log"
    echo "╚══════════════════════════════════════════════════════╝"
    echo ""

    # 询问是否立即启动服务
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
