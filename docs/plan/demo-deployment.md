# Demo 服务器部署方案

## 目标

在自有服务器上部署 hugo-admin 公开 demo 实例，供他人试用。

## 架构

```
用户 → Nginx (反代, HTTPS) → Docker (hugo-admin:5050)
                            → Hugo 预览 (1313, 可选)
```

## 资源限制

| 资源 | 限制方式 | 上限 | 说明 |
|------|---------|------|------|
| CPU | Docker deploy limits | 1 核 | Flask + Hugo CLI 够用 |
| Memory | Docker deploy limits | 512MB | Python 常驻 ~100M，Hugo 构建峰值 ~300M |
| 磁盘（数据/缓存） | loop 设备 | 512MB | 硬上限，防止缓存无限膨胀 |
| 磁盘（构建产物） | tmpfs | 不占磁盘 | Hugo 构建产物放内存，重启可重建 |

## 部署步骤

### 1. 磁盘限制准备

创建 loop 设备限制数据目录：

```bash
# 创建 512MB 虚拟磁盘
dd if=/dev/zero of=/opt/hugo-admin-data.img bs=1M count=512
mkfs.ext4 /opt/hugo-admin-data.img
mkdir -p /opt/hugo-admin-data
mount -o loop /opt/hugo-admin-data.img /opt/hugo-admin-data

# 开机自动挂载
echo '/opt/hugo-admin-data.img /opt/hugo-admin-data ext4 loop 0 0' >> /etc/fstab
```

### 2. SECRET_KEY 生成

```bash
openssl rand -hex 32
```

### 3. docker-compose.yml 改动

在现有 `hugo-admin` 服务中添加：

```yaml
deploy:
  resources:
    limits:
      cpus: '1'
      memory: 512M
    reservations:
      memory: 128M
tmpfs:
  - /app/public
  - /tmp
```

数据目录 volume 从 `./data:/hugo_admin_data` 改为：
```yaml
- /opt/hugo-admin-data:/hugo_admin_data
```

<!-- Swarm 模式备用：如果使用 Docker Swarm 部署，将上述 deploy 配置放在服务定义下，
     并使用 `docker stack deploy -c docker-compose.yml hugo-admin` 替代 `docker compose up`。
     Swarm 模式下需要先 `docker swarm init`，且不支持 tmpfs 与 volume 混用，
     磁盘限制改用 Docker volume driver 或宿主机 loop 设备。 -->

### 4. 环境变量

```bash
SECRET_KEY=<上一步生成的密钥>
FLASK_ENV=production
HUGO_BLOG_DIR=/path/to/demo/hugo-blog
HUGO_THEME_DIR=/path/to/demo/hugo-theme
HUGO_SERVER_BASE_URL=https://<服务器IP或域名>:1313
```

### 5. Nginx 反代 (HTTPS)

```nginx
server {
    listen 80;
    server_name demo.yourdomain.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name demo.yourdomain.com;

    ssl_certificate     /etc/letsencrypt/live/demo.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/demo.yourdomain.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:5050;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /socket.io/ {
        proxy_pass http://127.0.0.1:5050;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

证书签发（首次）：
```bash
certbot --nginx -d demo.yourdomain.com
```

### 6. 启动

```bash
docker compose up -d --build
```

## 注意事项

- demo 实例裸跑，无登录保护，仅用于试用
- 使用独立的示例博客目录，避免影响正式数据
- `FLASK_ENV=production`，关闭 DEBUG
- `SECRET_KEY` 必须使用 `openssl rand -hex 32` 生成
- loop 设备空间用尽时需要手动清理或重建
