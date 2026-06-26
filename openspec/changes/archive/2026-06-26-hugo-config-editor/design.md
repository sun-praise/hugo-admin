## Context

Hugo 站点配置存储在站点根目录的 `hugo.toml`（或 `hugo.yaml`、`config.toml` 等）。当前 hugo-admin 只在 `create_site` 时写入一次配置，之后无法通过 UI 修改。用户必须 SSH 进服务器手动编辑。

现有架构：
- `ProjectInitService` 创建站点时写入默认 `hugo.toml`
- `HugoService` 启动 `hugo server` 时读取配置
- `SettingsService` 管理 hugo-admin 自身的设置（非 Hugo 站点配置）
- 前端 Settings 页有 3 个 tab：general / project / themes

## Goals / Non-Goals

**Goals:**
- 提供 API 读取/写入 Hugo 站点配置文件
- 前端提供配置编辑器：常用字段结构化表单 + 高级字段原始 TOML/YAML 编辑
- 写入后 Hugo 服务器自动重载（livereload 已有机制）

**Non-Goals:**
- 不做配置模板/预设管理
- 不做配置版本历史/回滚
- 不做多站点配置管理（单站点架构）
- 不支持 `config/` 目录下的多环境配置拆分（只处理根目录单文件）

## Decisions

### 1. 配置文件检测策略

读取时按优先级扫描：`hugo.toml` → `hugo.yaml` → `config.toml` → `config.yaml` → `config.json`。写入时保持原格式（如果文件存在），新建默认用 TOML。

### 2. API 设计

```
GET  /api/config          → { format, content, path }
PUT  /api/config          → { success, message }
     body: { content: string }  （原始文本，完整替换）
```

采用原始文本方式而非结构化 JSON，原因：
- Hugo 配置字段极多且可扩展（params、menu、markup 等全是用户自定义）
- 结构化 API 需要覆盖所有可能字段，维护成本高
- 原始文本 + TOML/YAML 语法校验已足够

### 3. 前端编辑器

使用 `<textarea>` + monospace 字体作为原始编辑器（不引入 CodeMirror 等重型依赖）。在编辑器上方提供常用字段的快捷表单（baseURL、title、languageCode、theme），修改后同步更新到下方的原始文本。

### 4. 写入校验

后端对 PUT 内容做两层校验：
1. TOML/YAML 语法解析（格式合法性）
2. 必填字段检查（baseURL 不能为空）

### 5. Hugo 服务器重载

Hugo 的 livereload 已经监听文件变更。写入 `hugo.toml` 后 Hugo 会自动重载，无需额外通知。

## Risks / Trade-offs

- **原始文本编辑** vs **结构化表单**：选择了原始文本 + 快捷表单的混合方案。结构化表单覆盖不全，纯文本对新手不友好，混合方案平衡两者。
- **配置文件格式检测**：如果用户同时有 `hugo.toml` 和 `config.toml`，只读第一个。这是 Hugo 自身的行为，保持一致。
- **写入安全性**：完整替换而非部分更新，避免 TOML 合并的复杂性。用户看到的是完整文件，所见即所得。
