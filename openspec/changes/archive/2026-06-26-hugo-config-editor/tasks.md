## 1. 后端 API

- [x] 1.1 在 `routes/` 下新增 `config_routes.py`，实现 `GET /api/config` 和 `PUT /api/config`
- [x] 1.2 配置文件检测逻辑：按优先级扫描 `hugo.toml` → `hugo.yaml` → `config.toml` → `config.yaml` → `config.json`
- [x] 1.3 PUT 写入前做 TOML/YAML 语法校验（tomllib / pyyaml）
- [x] 1.4 在 `app.py` 中注册 config_routes Blueprint

## 2. 前端 API 层

- [x] 2.1 在 `frontend/src/utils/api.ts` 中新增 `getConfig()` 和 `saveConfig(content)` 函数

## 3. 前端配置编辑器

- [x] 3.1 在 Settings.tsx 新增 "站点配置" tab（`TabKey` 扩展）
- [x] 3.2 实现配置编辑器主体：monospace textarea，加载/显示配置内容
- [x] 3.3 实现快捷编辑表单（baseURL、title、languageCode、theme），修改同步到 textarea
- [x] 3.4 实现保存逻辑：前端基础校验 → PUT 请求 → 成功/失败通知

## 4. 测试

- [x] 4.1 后端测试：`GET /api/config` 读取现有配置
- [x] 4.2 后端测试：`PUT /api/config` 写入合法配置
- [x] 4.3 后端测试：`PUT /api/config` 拒绝非法格式
- [x] 4.4 后端测试：无配置文件时返回 404

## 5. 收尾

- [x] 5.1 前端 build + lint 通过
- [x] 5.2 后端 black + flake8 + pytest 全部通过
