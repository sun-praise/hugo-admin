#!/bin/bash
# Hugo Blog Web 管理界面启动脚本

echo "=========================================="
echo "Hugo Blog Web 管理界面"
echo "=========================================="
echo ""

# 检查是否在正确的目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 检查 Python
if ! command -v python3 &> /dev/null; then
    echo "错误: 未找到 python3"
    exit 1
fi

echo "1. 检查依赖..."
# 检查是否已安装依赖
if ! uv run python3 -c "import flask" 2>/dev/null; then
    echo "首次运行，正在安装依赖..."
    uv pip install -r requirements.txt
    if [ $? -ne 0 ]; then
        echo "错误: 依赖安装失败"
        exit 1
    fi
else
    echo "✓ 依赖已安装"
fi

echo ""
echo "2. 检查 Hugo..."
if ! command -v hugo &> /dev/null; then
    echo "警告: 未找到 hugo 命令"
    echo "Hugo 服务器控制功能将不可用"
else
    echo "✓ Hugo 已安装: $(hugo version)"
fi

echo ""
echo "=========================================="
echo "启动 Web 应用..."
echo "=========================================="
echo "访问地址: http://127.0.0.1:5000"
echo "按 Ctrl+C 停止服务器"
echo "=========================================="
echo ""

# 启动应用
uv run python3 app.py
