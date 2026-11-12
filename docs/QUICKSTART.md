# 快速开始指南

## 一、安装依赖

- Go 1.24+（后端）
- Node >= 22 + pnpm 9+（前端构建）
- Hugo（可选，仅影响 Hugo 服务器控制功能）

## 二、启动

### 方法 1: 使用启动脚本(推荐)

```bash
chmod +x run.sh
./run.sh
```

脚本会自动构建前端（首次运行）、编译后端并启动服务。传 `--build` 可强制重建前端。

### 方法 2: 手动启动

```bash
# 构建前端（产物输出到 admin-ui/）
cd frontend && pnpm install --frozen-lockfile && pnpm run build && cd ..

# 启动后端
go run ./cmd/hugo-admin
```

## 三、配置

后端配置通过环境变量或仓库根 `.env` 提供（环境变量优先）：

```bash
cp .env.example .env
# 编辑 .env，把 HUGO_ROOT / CONTENT_DIR 指向你的 Hugo 站点目录
```

常用变量：

| 变量 | 默认 | 说明 |
|---|---|---|
| `PORT` | 5050 | 监听端口 |
| `HUGO_ROOT` | 仓库根 | Hugo 站点根（git 仓库） |
| `CONTENT_DIR` | `<HUGO_ROOT>/content` | 内容目录，**必须可写** |
| `SECRET_KEY` | 开发默认值 | 会话密钥，生产必须修改 |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | admin/admin | 首次启动引导账户 |
| `API_TOKEN` | 空 | 机器客户端 Bearer 鉴权 |
| `AI_API_KEY` 等 | 空 | AI 助手，不设置则禁用 |

完整字段语义见 `internal/config/config.go`。

## 四、访问应用

打开浏览器访问: **http://127.0.0.1:5050**

首次启动以 `admin/admin` 引导默认账户（可用 `ADMIN_USERNAME`/`ADMIN_PASSWORD` 覆盖），登录后请立即修改密码。

## 五、常见问题

### Q1: 提示 CONTENT_DIR 不可写

服务启动即写 `<CONTENT_DIR>/.admin/cache.db`。用 `.env` 或环境变量指向你的 Hugo 站点：

```bash
HUGO_ROOT=~/work/blog/<站点> CONTENT_DIR=~/work/blog/<站点>/content ./run.sh
```

### Q2: 启动 Hugo 服务器失败

**可能原因:**
1. Hugo 未安装
2. Hugo 版本过低

**解决:**
```bash
# 检查 Hugo 是否安装
hugo version

# 如果未安装，访问 https://gohugo.io/installation/ 安装
```

### Q3: 无法编辑文章

**解决:** 确保文件有写权限
```bash
chmod -R u+w <你的站点>/content/
```

### Q4: 日志不显示

**解决:**
1. 检查浏览器是否支持 SSE / WebSocket
2. 查看浏览器控制台是否有错误
3. 尝试刷新页面

### Q5: 端口 5050 被占用

在 `.env` 中修改 `PORT`，或启动时指定：
```bash
PORT=5051 ./run.sh
```

## 六、开发调试

前端热更开发（vite 会把 `/api` 代理到本地 5050，无需重建 admin-ui）：

```bash
go run ./cmd/hugo-admin &   # 后端 API 在 :5050
cd frontend && pnpm dev
```

### 测试 API

```bash
curl http://127.0.0.1:5050/api/version
```

## 七、安全建议

1. **仅本地使用**: 应用默认只监听 127.0.0.1，对外暴露请置于反向代理之后并设置强 `SECRET_KEY`
2. **文件权限**: 确保 content 目录权限正确
3. **定期备份**: 编辑文章前建议先备份

## 技术支持

如果遇到问题:
1. 查看 README.md 了解更多信息
2. 检查浏览器控制台错误
3. 查看终端日志输出
