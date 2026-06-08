# Demo 服务器部署方案

## 目标

在自有服务器上部署 hugo-admin 公开 demo 实例，供他人试用。

## 架构

```
用户 → Nginx (反代) → Docker (hugo-admin:5050)
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

### 2. docker-compose.yml 改动

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

### 3. 环境变量

```bash
SECRET_KEY=<随机生成的密钥>
FLASK_ENV=production
HUGO_BLOG_DIR=/path/to/demo/hugo-blog
HUGO_THEME_DIR=/path/to/demo/hugo-theme
HUGO_SERVER_BASE_URL=http://<服务器IP>:1313
```

### 4. Nginx 反代

```nginx
server {
    listen 80;
    server_name demo.yourdomain.com;

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

### 5. 启动

```bash
docker compose up -d --build
```

## 注意事项

- demo 实例裸跑，无登录保护，仅用于试用
- 使用独立的示例博客目录，避免影响正式数据
- `FLASK_ENV=production`，关闭 DEBUG
- `SECRET_KEY` 必须设置为随机值
- loop 设备空间用尽时需要手动清理或重建
