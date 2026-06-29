# Hugo Admin Dockerfile

# ============ Stage 1: 前端构建 ============
FROM node:22-slim AS frontend-build
WORKDIR /build
RUN corepack enable && corepack prepare pnpm@9.15.0 --activate
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
# vite outDir 为 ../admin-ui（相对 frontend/），构建产物落到 /admin-ui
RUN pnpm run build && ls -la /admin-ui

# ============ Stage 2: 运行时 ============
FROM python:3.11-slim

# 设置工作目录
WORKDIR /app

# 设置环境变量
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

# 安装系统依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*

# 安装 Hugo Extended
# 升级到 0.163.3：Fried-Rice 等现代主题要求 hugo >= 0.157.0（hash.FNV32a / mod
# 等函数旧版不支持）。对齐本机 dev 环境（0.163.1），消除"本地能跑、demo 跑不动"的版本差。
ARG HUGO_VERSION=0.163.3
RUN curl -L "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-amd64.tar.gz" -o hugo.tar.gz \
    && tar -xzf hugo.tar.gz -C /usr/local/bin/ \
    && rm hugo.tar.gz \
    && chmod +x /usr/local/bin/hugo \
    && hugo version \
    && hugo version | grep -i extended

# 复制应用代码
COPY . .

# 用 Stage 1 构建的前端产物覆盖到 /app/admin-ui（避开博客 static 挂载点）
COPY --from=frontend-build /admin-ui /app/admin-ui

# 安装 Python 依赖（兼容移除 requirements.txt 后的依赖管理方式）
RUN pip install --no-cache-dir .

# 创建必要的目录
RUN mkdir -p /app/content /app/public /app/static /app/templates

# 暴露端口
EXPOSE 5050 1313

# 健康检查：/api/version 免认证，确保 healthcheck 不会因 401 永远 starting
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:5050/api/version || exit 1

# 清理 Docker 挂载可能自动创建的 config.toml 目录（Hugo 会误读为配置文件）
CMD ["sh", "-c", "test -d /app/config.toml && rm -rf /app/config.toml; python app.py"]
