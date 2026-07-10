# Change: Add TTS Speech Generation Plugin Capability

## Why

hugo-admin 已有基于 gRPC 子进程的插件系统，但目前只接了 `image_upload` 一种能力。用户希望给自己的博客文章生成语音播报（text-to-speech），便于读者"听文章"。小米 MiMo 提供了高质量的 TTS 接口（OpenAI 兼容），适合作为语音来源；生成的音频需要托管到公网（Cloudflare R2）并通过 frontmatter 的 `audio` 字段引用。

本次新增一种插件能力 `tts_generation`，沿用既有插件机制：host 端（开源）只扩展协议、能力路由与文章集成层；实际调用 MiMo 与 R2 托管的逻辑放在闭源的 Go 插件（独立私有仓库 `../tts-gen-plugin`）中。

## What Changes

- **Plugin Protocol**: 在 `proto/plugin.proto` 新增可选能力服务 `TTSGenerator`（`Generate` 服务端流式 + `Delete`），与 `ImageUploader` 并列，不破坏既有协议。
- **Plugin Manager**: `services/plugin_manager.py` 新增 `get_tts_generator_stub()` 与 `find_plugin_with_capability()` 通用辅助方法（顺带把 `image_routes.py` 里内联的"按能力找运行中插件"循环抽出来复用）。
- **Capability Proxy Routes**: `routes/plugin_routes.py` 新增 `POST /api/plugins/<name>/tts/generate`（流式进度经 Socket.IO 转发）与 `DELETE /api/plugins/<name>/tts/<audio_id>`。
- **Article Integration**: 新建 `routes/tts_routes.py`，`POST /api/article/tts` —— 读正文、调用带 `tts_generation` 能力的插件、后台任务经 Socket.IO 上报进度、成功后用乐观锁把 `audio` 写回 frontmatter。复刻 `article_import_service.py` 的后台任务 + 进度事件模式。
- **Frontend**: `Editor.tsx` 新增"生成语音播报"按钮（参数面板：音色/语速/模型/格式），进度经 Socket.IO 实时显示，完成后内联渲染 `<audio>` 播放器；`utils/api.ts` 新增类型化封装。
- **Go Plugin**: 独立私有仓库 `tts-gen-plugin`，实现 `PluginService` + `TTSGenerator`，调用 MiMo TTS 并上传 Cloudflare R2，返回公网 URL。

## Impact

- Affected specs: 新增 spec `tts-generation`
- Affected code:
  - Modified: `proto/plugin.proto`（新增 TTS 服务与消息）、`proto/plugin_pb2.py` / `proto/plugin_pb2_grpc.py`（重新生成）
  - Modified: `services/plugin_manager.py`（stub 获取器 + 能力查找辅助）
  - Modified: `routes/plugin_routes.py`（TTS 能力代理路由）、`routes/image_routes.py`（复用能力查找）
  - New: `routes/tts_routes.py`、`app.py`（注册新蓝图）、`routes/__init__.py`（导出）
  - New: `tests/test_tts_routes.py`、`tests/test_plugin_manager.py`（补 stub 用例）
  - Modified: `frontend/src/pages/Editor.tsx`、`frontend/src/utils/api.ts`
- External: 新私有 Go 仓库 `tts-gen-plugin`（独立仓库，不进 hugo-admin）
- New dependencies: 无 Python 新依赖（已有 `grpcio`/`grpcio-tools`）；Go 侧 `google.golang.org/grpc`、`aws-sdk-go-v2`（R2 走 S3 兼容 API）
