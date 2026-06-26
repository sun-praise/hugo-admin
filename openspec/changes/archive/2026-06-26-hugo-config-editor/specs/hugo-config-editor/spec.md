## ADDED Requirements

### Requirement: Config editor tab

前端 Settings 页 SHALL 新增 "站点配置" tab（第 4 个 tab），包含配置编辑器。

- 页面加载时调用 `GET /api/config` 获取当前配置内容
- 编辑器主体为 monospace 字体的 `<textarea>`，显示完整的配置文件文本
- 底部有 "保存" 按钮，调用 `PUT /api/config` 写入

### Requirement: Quick edit fields

编辑器上方 SHALL 提供常用字段的快捷编辑表单：

- `baseURL`：文本输入
- `title`：文本输入
- `languageCode`：文本输入（默认 `zh-CN`）
- `theme`：文本输入（与当前已安装主题联动）

修改快捷字段后，自动同步更新到下方的原始文本区域。用户也可以直接编辑原始文本。

### Requirement: Syntax validation

前端 SHALL 在保存前对配置内容做基础校验：

- TOML 格式：检查括号/引号匹配，基本语法合法
- 校验失败时显示错误提示，不发送请求
- 校验通过后才发送 PUT 请求

### Requirement: Save feedback

保存操作 SHALL 提供即时反馈：

- 保存中显示 loading 状态
- 成功显示绿色通知："配置已保存"
- 失败显示红色通知，包含后端返回的错误信息
- 保存成功后重新加载配置内容（确保编辑器与文件同步）
