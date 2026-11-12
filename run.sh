#!/bin/bash
# hugo-admin (Go) 启动脚本
#
# 用法: ./run.sh [--build]
#   --build   强制重建前端（默认 admin-ui/ 缺失时才构建）
#
# 配置通过环境变量或仓库根目录 .env 提供（PORT/HUGO_ROOT/CONTENT_DIR/
# SECRET_KEY/AUTH_STORE/API_TOKEN/AI_* 等），语义见 internal/config/config.go。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

die() {
    echo "错误: $*" >&2
    exit 1
}

FORCE_BUILD=0
if [ "${1:-}" = "--build" ]; then
    FORCE_BUILD=1
fi

APP_ROOT="${HUGO_ADMIN_ROOT:-$SCRIPT_DIR}"

# --- Go 工具链 ---
command -v go >/dev/null 2>&1 || die "未找到 go 命令（需要 Go 1.24+）"

# --- 前端构建产物（Go 二进制直接托管 admin-ui/）---
if [ "$FORCE_BUILD" -eq 1 ] || [ ! -f admin-ui/index.html ]; then
    command -v pnpm >/dev/null 2>&1 || die "未找到 pnpm，无法构建前端（需要 Node >= 22 + pnpm 9+）"
    echo "==> 构建前端..."
    if [ -d frontend/node_modules ]; then
        (cd frontend && pnpm run build)
    else
        (cd frontend && pnpm install --frozen-lockfile && pnpm run build)
    fi
fi

# --- Hugo（可选，仅影响 Hugo 服务器控制功能）---
if command -v hugo >/dev/null 2>&1; then
    echo "==> Hugo: $(hugo version | head -n1)"
else
    echo "==> 警告: 未找到 hugo 命令，Hugo 服务器控制功能不可用"
fi

# --- CONTENT_DIR 必须可写（服务启动即写 <CONTENT_DIR>/.admin/cache.db）---
CONTENT_DIR_CHECK="${CONTENT_DIR:-$APP_ROOT/content}"
if [ ! -w "$CONTENT_DIR_CHECK" ]; then
    die "CONTENT_DIR 不可写: $CONTENT_DIR_CHECK
请用环境变量或 .env 指向你自己的 Hugo 站点目录，例如:
  HUGO_ROOT=~/work/blog/<站点> CONTENT_DIR=~/work/blog/<站点>/content ./run.sh"
fi

echo "==> 启动 hugo-admin (Go): http://127.0.0.1:${PORT:-5050}"
exec go run ./cmd/hugo-admin
