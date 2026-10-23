# Agent 导入 API（机器客户端写入 Markdown）

面向自动化客户端（agent、脚本）的 Markdown 投递通道：无需接入文件系统、
无需浏览器会话，用 Bearer Token + JSON 即可把文章写入 Hugo 内容目录。

## 前置配置

设置环境变量 `API_TOKEN`（也支持写入 `.env` 文件）：

```bash
API_TOKEN=<足够长的随机串>   # 例如 openssl rand -hex 32 生成
```

- `API_TOKEN` **未配置时 Bearer 通道关闭**，所有请求仍走会话认证（行为与旧版一致）。
- 配置后，携带 `Authorization: Bearer <API_TOKEN>` 的请求等价于已登录管理员，
  可访问全部 `/api/*` 端点——请把 token 当作管理员凭据保管。
- 校验使用常数时间比较，不暴露通道是否开启。

## 导入文章：POST /api/article/import

`Content-Type: application/json` 时进入 JSON 模式（浏览器 UI 的 multipart
上传不受影响，两者共用同一路径与导入语义）。

### 请求

```bash
curl -X POST https://<host>/api/article/import \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "filename": "my-note.md",
    "content": "---\ntitle: 我的文章\ntags: [agent]\n---\n\n正文……",
    "draft": true
  }'
```

| 字段 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `content` | string | 必填 | Markdown 全文，可自带 YAML/TOML frontmatter |
| `filename` | string | `import.md` | 仅用于扩展名校验与标题派生兜底，须为 `.md`/`.markdown` |
| `title` | string | 自动派生 | 显式标题；缺省依次取 frontmatter `title` → 首个 `# ` 标题 → 文件名 |
| `draft` | bool | `true` | 草稿位；`false` 直接进入已发布状态 |
| `generate_frontmatter` | bool | `false` | AI 补全 description/tags/categories（需要已配置 AI Key） |
| `generate_cover` | bool | `false` | AI 生成封面（需要 `OPENROUTER_API_KEY`，后台异步，SSE 事件 `article_import.*`） |

> JSON 模式的 AI 开关默认**关闭**——与浏览器 multipart 模式（默认开启）不同，
> 让 agent 集成行为可预测、无隐性外部依赖。

### 响应

```json
{
  "success": true,
  "path": "post/2026-10-23-my-note/index.md",
  "title": "我的文章",
  "warnings": [],
  "cover_pending": false,
  "event_scope": ""
}
```

### 语义约定

- 文章写入 `content/post/<日期>-<slug>/index.md`（page bundle），目录唯一、
  **永不覆盖**既有文章；重名自动追加随机后缀。
- 导入会重建 frontmatter：已有字段（title/date/description/categories/tags/cover）
  保留，`date` 缺省为当前东八区时间，`categories`/`tags` 缺省补空数组。
- `draft: false` 导入即发布 Hugo 语义的已发布文章（`draft` 字段为 false）。

### 更新已有文章

导入只负责创建。更新请组合使用：

```bash
# 1. 读当前内容与 mtime
curl -X POST https://<host>/api/file/read-with-frontmatter \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path": "post/2026-10-23-my-note/index.md"}'

# 2. 带 expected_mtime 保存（乐观锁；冲突返回 409 与远端内容）
curl -X POST https://<host>/api/file/save \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path": "post/2026-10-23-my-note/index.md", "content": "……", "expected_mtime": 1730000000}'
```

## 典型 agent 工作流

```python
import requests

BASE = "https://hugo-admin.example.com"
HDRS = {"Authorization": "Bearer <API_TOKEN>"}

r = requests.post(f"{BASE}/api/article/import", headers=HDRS, json={
    "content": md_text,          # agent 生成的 Markdown
    "title": "每周汇总",
    "draft": True,               # 人工审核后再发布
})
path = r.json()["path"]          # 后续可用 /api/file/save 更新
```

## 安全注意事项

- Bearer token 与管理员会话等权，仅建议在可信内网/部署边界内使用；
  公网部署务必走 HTTPS。
- 泄露疑似时直接轮换 `API_TOKEN` 并重启服务即可，无需改任何数据文件。
