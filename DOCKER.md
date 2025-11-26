# Hugo Admin Docker 部署指南

本文档介绍如何使用 Docker 部署 Hugo Admin 管理界面。

## 快速开始

### 使用 Docker Compose（推荐）

1. **启动服务**

```bash
docker-compose up -d
```

2. **查看日志**

```bash
docker-compose logs -f hugo-admin
```

3. **停止服务**

```bash
docker-compose down
```

### 使用 Docker 命令

1. **构建镜像**

```bash
docker build -t hugo-admin:latest .
```

2. **运行容器**

```bash
docker run -d \
  --name hugo-admin \
  -p 5050:5050 \
  -p 1313:1313 \
  -v $(pwd)/content:/app/content \
  -v $(pwd)/public:/app/public \
  -v $(pwd)/static:/app/static \
  -e SECRET_KEY=your-secret-key \
  hugo-admin:latest
```

3. **查看日志**

```bash
docker logs -f hugo-admin
```

4. **停止容器**

```bash
docker stop hugo-admin
docker rm hugo-admin
```

## 配置说明

### 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `SECRET_KEY` | Flask 密钥 | `dev-secret-key-change-in-production` |
| `FLASK_ENV` | Flask 环境 | `development` |
| `HUGO_ROOT` | Hugo 根目录 | `/app` |
| `CONTENT_DIR` | 内容目录 | `/app/content` |
| `PUBLIC_DIR` | 发布目录 | `/app/public` |

### 端口映射

- `5050`: Hugo Admin 管理界面
- `1313`: Hugo 预览服务器

### 数据卷

建议挂载以下目录以持久化数据：

- `./content:/app/content` - Hugo 博客内容
- `./public:/app/public` - Hugo 生成的静态文件
- `./static:/app/static` - 静态资源文件
- `./config.toml:/app/config.toml:ro` - Hugo 配置文件（只读）

### 自定义配置

如果需要使用自定义配置，可以创建 `config_local.py` 文件并挂载到容器：

```yaml
volumes:
  - ./config_local.py:/app/config_local.py:ro
```

## Hugo 版本

默认使用 Hugo Extended v0.121.1。如需使用其他版本，可以在构建时指定：

```bash
docker build --build-arg HUGO_VERSION=0.120.0 -t hugo-admin:latest .
```

或在 docker-compose.yml 中修改：

```yaml
build:
  args:
    HUGO_VERSION: "0.120.0"
```

## 健康检查

容器包含健康检查，每 30 秒检查一次应用状态：

```bash
docker inspect --format='{{json .State.Health}}' hugo-admin
```

## 故障排查

### 无法访问管理界面

1. 检查容器是否正在运行：
   ```bash
   docker ps | grep hugo-admin
   ```

2. 查看容器日志：
   ```bash
   docker logs hugo-admin
   ```

3. 检查端口映射：
   ```bash
   docker port hugo-admin
   ```

### Hugo 预览服务器无法启动

确保 content 目录已正确挂载，并且包含有效的 Hugo 内容。

### 权限问题

如果遇到文件权限问题，可能需要调整挂载目录的权限：

```bash
chmod -R 755 content public static
```

## 生产环境部署

在生产环境中部署时，请注意：

1. **修改密钥**：设置强密码的 `SECRET_KEY` 环境变量
2. **使用反向代理**：建议使用 Nginx 或 Traefik 作为反向代理
3. **配置 HTTPS**：使用 SSL/TLS 证书保护连接
4. **限制访问**：配置防火墙规则，仅允许必要的端口访问
5. **数据备份**：定期备份 content 目录

### 使用 Nginx 反向代理示例

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:5050;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket 支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

## 更新应用

1. **拉取最新代码**
   ```bash
   git pull
   ```

2. **重新构建镜像**
   ```bash
   docker-compose build
   ```

3. **重启服务**
   ```bash
   docker-compose up -d
   ```

## 清理

删除所有相关资源：

```bash
# 停止并删除容器
docker-compose down

# 删除镜像
docker rmi hugo-admin:latest

# 删除数据卷（注意：这会删除所有数据！）
docker-compose down -v
```

## 技术栈

- **基础镜像**: Python 3.11-slim
- **Web 框架**: Flask 3.0.0
- **WebSocket**: Flask-SocketIO 5.3.5
- **静态站点生成器**: Hugo Extended
- **WSGI 服务器**: Werkzeug（开发环境）

## 支持

如有问题，请提交 Issue 或查看项目文档。
