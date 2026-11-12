#!/bin/bash
# hugo-admin (Go) 启动脚本
#
# 用法: ./run.sh [--build]
#   --build   强制重建前端（默认前端产物缺失时才构建）
#
# 配置通过环境变量或仓库根 .env 提供（PORT/HUGO_ROOT/CONTENT_DIR/
# SECRET_KEY/ADMIN_UI_DIR/API_TOKEN/AI_* 等），字段语义见
# internal/config/config.go。本脚本按同一语义预读 .env：
# KEY=VALUE 行、# 注释、成对引号去除、已存在的环境变量优先。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

usage() {
    echo "用法: ./run.sh [--build]" >&2
    exit 1
}

die() {
    echo "错误: $*" >&2
    exit 1
}

# 与 internal/config/config.go 的 loadDotEnv 同语义的 .env 解析：
# 只填充 shell 中尚未设置的变量，服务进程与脚本看到的配置一致。
load_dotenv() {
    local env_file="$1" line key value
    [ -f "$env_file" ] || return 0
    while IFS= read -r line || [ -n "$line" ]; do
        line="${line%$'\r'}"
        line="${line#"${line%%[![:space:]]*}"}"
        line="${line%"${line##*[![:space:]]}"}"
        case "$line" in '' | '#'*) continue ;; esac
        case "$line" in *=*) ;; *) continue ;; esac
        key="${line%%=*}"
        key="${key#"${key%%[![:space:]]*}"}"
        key="${key%"${key##*[![:space:]]}"}"
        value="${line#*=}"
        value="${value#"${value%%[![:space:]]*}"}"
        value="${value%"${value##*[![:space:]]}"}"
        if [ ${#value} -ge 2 ]; then
            case "$value" in
                \"*\") value="${value#\"}"; value="${value%\"}" ;;
                \'*\') value="${value#\'}"; value="${value%\'}" ;;
            esac
        fi
        [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
        if [ -z "${!key+x}" ]; then
            export "$key=$value"
        fi
    done < "$env_file"
}

FORCE_BUILD=0
while [ $# -gt 0 ]; do
    case "$1" in
        --build) FORCE_BUILD=1; shift ;;
        *) usage ;;
    esac
done

APP_ROOT="${HUGO_ADMIN_ROOT:-$SCRIPT_DIR}"
load_dotenv "$APP_ROOT/.env"

# --- Go 工具链 ---
command -v go >/dev/null 2>&1 || die "未找到 go 命令（需要 Go 1.24+）"

# --- 前端构建产物（Go 二进制直接托管，目录由 ADMIN_UI_DIR 决定，默认 admin-ui/）---
ADMIN_UI="${ADMIN_UI_DIR:-$APP_ROOT/admin-ui}"
if [ "$FORCE_BUILD" -eq 1 ] || [ ! -f "$ADMIN_UI/index.html" ]; then
    command -v pnpm >/dev/null 2>&1 || die "未找到 pnpm，无法构建前端（需要 Node >= 22 + pnpm 9+）"
    echo "==> 构建前端..."
    if [ -d frontend/node_modules ]; then
        (cd frontend && pnpm run build)
    else
        (cd frontend && pnpm install --frozen-lockfile && pnpm run build)
    fi
    # vite 输出固定为 frontend/../admin-ui
    [ -f "$SCRIPT_DIR/admin-ui/index.html" ] || die "前端构建完成但未找到产物: admin-ui/index.html"
fi

# --- 编译后端并 exec（信号直达服务进程，退出时才可靠回收 Hugo 子进程）---
echo "==> 编译后端..."
mkdir -p bin
go build -o bin/hugo-admin ./cmd/hugo-admin

# --- Hugo（可选，仅影响 Hugo 服务器控制功能）---
if command -v hugo >/dev/null 2>&1; then
    echo "==> Hugo: $(hugo version 2>/dev/null | head -n1 || true)"
else
    echo "==> 警告: 未找到 hugo 命令，Hugo 服务器控制功能不可用"
fi

# --- CONTENT_DIR 守卫：服务启动即写 <CONTENT_DIR>/.admin/cache.db ---
# 对齐 db.Open 的 MkdirAll 语义：目录可不存在，但最近一个已存在的祖先目录必须可写
CONTENT_DIR_CHECK="${CONTENT_DIR:-$APP_ROOT/content}"
_content_parent="$CONTENT_DIR_CHECK"
while [ ! -d "$_content_parent" ]; do
    _content_parent="$(dirname "$_content_parent")"
done
if [ ! -w "$_content_parent" ]; then
    die "CONTENT_DIR 不可写: $CONTENT_DIR_CHECK
服务启动需要在 <CONTENT_DIR>/.admin/ 写缓存数据库。
请用环境变量或仓库根 .env 指向你的 Hugo 站点目录，例如:
  HUGO_ROOT=~/work/blog/<站点> CONTENT_DIR=~/work/blog/<站点>/content ./run.sh"
fi

echo "==> 启动 hugo-admin (Go): http://127.0.0.1:${PORT:-5050}"
exec ./bin/hugo-admin
