## Requirements

### Requirement: Read Hugo config

系统 SHALL 提供 `GET /api/config` 端点，读取当前活跃 Hugo 站点的配置文件。

- 按优先级检测配置文件：`hugo.toml` → `hugo.yaml` → `config.toml` → `config.yaml` → `config.json`
- 响应格式：`{ "success": true, "format": "toml", "content": "...", "path": "/abs/path/hugo.toml" }`
- 如果不存在任何配置文件，返回 404：`{ "success": false, "message": "未找到 Hugo 配置文件" }`
- 需要认证

### Requirement: Write Hugo config

系统 SHALL 提供 `PUT /api/config` 端点，写入 Hugo 站点配置。

- 请求体：`{ "content": "<完整的配置文件文本>" }`
- 写入时保持原文件格式（通过文件扩展名判断）
- 写入前校验 TOML/YAML 语法，解析失败返回 400：`{ "success": false, "message": "配置格式错误: ..." }`
- 写入成功返回：`{ "success": true, "message": "配置已保存", "path": "..." }`
- 需要认证

### Requirement: Config format detection

系统 SHALL 自动检测 Hugo 配置文件格式（TOML / YAML / JSON），并根据格式选择对应的解析器进行校验。

- 检测依据：文件扩展名
- TOML 使用 `tomllib`（Python 3.11+ 内置）
- YAML 使用 `pyyaml`
- JSON 使用 `json`（内置）
