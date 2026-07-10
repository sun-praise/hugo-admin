# Tasks: Add TTS Speech Generation Plugin Capability

## 1. Protocol

- [x] 1.1 在 `proto/plugin.proto` 新增 `TTSGenerator` 服务（`Generate` 服务端流式 + `Delete`）及 `TTSRequest`/`TTSResponse`(`oneof payload`)/`TTSProgress`/`TTSResult`/`TTSDeleteRequest`/`TTSDeleteResponse` 消息
- [x] 1.2 用 `grpcio-tools` 重新生成 `proto/plugin_pb2.py` 与 `proto/plugin_pb2_grpc.py`

## 2. Host — Plugin Manager

- [x] 2.1 `services/plugin_manager.py` 新增 `get_tts_generator_stub(name)`
- [x] 2.2 新增通用 `find_plugin_with_capability(capability)` 辅助方法
- [x] 2.3 `routes/image_routes.py` 的内联能力查找改为复用 `find_plugin_with_capability`

## 3. Host — Routes

- [x] 3.1 `routes/plugin_routes.py` 新增 `POST /api/plugins/<name>/tts/generate`（迭代服务端流，progress 经 Socket.IO 转发）
- [x] 3.2 `routes/plugin_routes.py` 新增 `DELETE /api/plugins/<name>/tts/<audio_id>`
- [x] 3.3 新建 `routes/tts_routes.py`：`POST /api/article/tts`（读正文 → 后台任务生成 → 乐观锁写回 frontmatter → Socket.IO 进度）
- [x] 3.4 新建 `DELETE /api/article/tts`（删 frontmatter `audio` 字段）
- [x] 3.5 `routes/__init__.py` 导出 `register_tts_routes`；`app.py` 注册蓝图（需 `socketio` 与 `plugin_manager` 注入）

## 4. Frontend

- [x] 4.1 `utils/api.ts` 新增 `generateArticleTTS` / `deleteArticleTTS` 类型化封装
- [x] 4.2 `Editor.tsx` 新增"生成语音播报"按钮 + 参数面板（音色/语速/模型/格式）
- [x] 4.3 监听 `tts.progress`/`tts.done`/`tts.failed`/`tts.conflict`（基于 `useSocket`），实时进度
- [x] 4.4 frontmatter 含 `audio` 时内联渲染 `<audio>` 播放器 + 重新生成 / 删除

## 5. Go Plugin（独立私有仓库 `tts-gen-plugin`）

- [x] 5.1 `git init` + `go mod init` + 复制 `plugin.proto` 生成 Go 桩
- [x] 5.2 `cmd/tts-gen-plugin/main.go`：解析 `--port`，起 gRPC server，注册 `PluginService` + `TTSGenerator`
- [x] 5.3 `internal/server`：`Info`/`HealthCheck`/`GetConfigSchema`/`SetConfig`
- [x] 5.4 `internal/mimo`：调用 MiMo `chat/completions`（`modalities:["audio"]`），解析 base64 音频
- [x] 5.5 `internal/r2`：Cloudflare R2（S3 兼容）上传 / 删除
- [x] 5.6 `Generate`：分片合成 → 上传 R2 → 流式返回进度与结果 URL；`Delete`：按 key 删 object
- [x] 5.7 `plugin.toml`（`tts_generation = true` + config_schema JSON Schema）
- [x] 5.8 `.github/workflows/release.yml`：tag 触发跨平台编译（linux/darwin × amd64/arm64）

## 6. Tests & 收尾

- [x] 6.1 `tests/test_tts_routes.py`：mock stub 流，断言进度事件 + frontmatter 写入 + 无插件 400
- [x] 6.2 `tests/test_plugin_manager.py`：补 `get_tts_generator_stub` / `find_plugin_with_capability` 用例
- [x] 6.3 `uv run pytest` + `uv run ruff check .` 通过
- [ ] 6.4 端到端自测：装插件 → 编辑器生成一篇语音播报（需 MiMo/R2 凭据，留待用户验证）
