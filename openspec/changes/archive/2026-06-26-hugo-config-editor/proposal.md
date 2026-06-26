## Why

Hugo 站点的 `hugo.toml`（或 `hugo.yaml`）是站点核心配置：baseURL、标题、语言、主题、菜单、自定义参数等。目前用户只能通过 SSH 或文件管理器手动编辑，容易出错且没有即时反馈。管理员界面应提供配置编辑能力，降低运维门槛。

## What Changes

- **新增后端 API**：`GET /api/config` 读取当前 Hugo 站点配置，`PUT /api/config` 写入配置（支持 TOML/YAML 格式自动检测）
- **新增前端配置编辑器**：在 Settings 页面新增 "站点配置" tab，提供结构化表单（常用字段）+ 原始文本编辑器（高级字段），带语法校验和实时预览
- **配置热重载**：写入后自动通知 Hugo 服务器重新加载（如果正在运行）

## Capabilities

### New Capabilities
- `hugo-config-api`: 后端配置读写 API，解析/序列化 TOML/YAML，校验格式合法性
- `hugo-config-editor`: 前端配置编辑器 UI，结构化表单 + 原始编辑器

### Modified Capabilities

无。现有 Settings 页面的 general/project/themes tab 不受影响。
