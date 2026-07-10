## Context

hugo-admin 已有一套成熟的 gRPC 子进程插件系统（`add-plugin-system` 变更），但目前只接了 `image_upload` 一种能力。本变更新增 `tts_generation` 能力，让用户能给博客文章生成语音播报。

- 语音来源：小米 MiMo TTS（OpenAI 兼容 `chat/completions`，`modalities:["audio"]`，响应 `data[0].data` 为 base64 音频）。
- 音频托管：Cloudflare R2（S3 兼容），frontmatter `audio:` 字段引用公网 URL。
- 触发位置：编辑器内按钮 + 内联 `<audio>` 播放器（与"生成封面"对称）。
- host 端开源扩展协议/路由/前端；MiMo 与 R2 的实际调用放在闭源 Go 插件（独立私有仓库）。

## Goals / Non-Goals

### Goals
- 在既有插件协议上**新增**一种能力，不破坏 `image_upload`
- host 端不感知 MiMo / Cloudflare 细节，只负责读正文、转发进度、写回 frontmatter
- 长任务（TTS 合成 + 上传）走后台任务 + Socket.IO 进度，HTTP 立即返回
- frontmatter 写回用乐观锁，冲突可被前端处理（与编辑器既有 ConflictModal 一致）

### Non-Goals
- 自动下载/签名校验安装（host 当前本就是手动安装，保持一致）
- 前台 Hugo 主题的 `<audio>` 渲染（那是博客主题侧，hugo-admin 只约定 `audio:` 字段）
- 音色设计/复刻 UI（V1 用预设音色 + 语速；V2 再扩）

## Decisions

### Decision 1: 能力解耦对称 —— 插件"生成+托管"一体返回 URL

TTS 插件对外只返回公网 URL（+ audio_id 供删除），与 `ImageUploader.Upload` 返回 `url` 完全对称。host 不接触音频二进制、不感知 R2 / MiMo —— 这些都是插件的实现细节。好处：换语音供应商 / 换托管商都不用动 host；插件闭源也不会泄漏 host 侧。

### Decision 2: 协议向后兼容 —— 新增可选服务

`TTSGenerator` 作为新的可选 gRPC 服务加入 `plugin.proto`，与 `ImageUploader` 并列。`protocol_version` 维持 `"1"`。没有 TTS 插件时，编辑器按 `list_plugins()` 是否存在该能力决定按钮显隐。

### Decision 3: 服务端流式 + Socket.IO 转发

`Generate` 用 gRPC **服务端流式**（`returns (stream TTSResponse)`）：插件边合成边发 `TTSProgress`，最后一条 `TTSResult`。host 迭代流，把 progress 包经 `socketio.emit("tts.progress", {scope, stage, percent})` 转发给前端。复刻 `article_import_service.py` 的后台任务模式：HTTP 立即返回 `event_scope`，长任务在 `socketio.start_background_task` 中跑。

### Decision 4: 两层路由 —— 能力代理 + 文章集成

- **能力代理**（`plugin_routes.py`）：`/api/plugins/<name>/tts/...` 直通插件，不碰文章；适合任意 TTS 调用方。
- **文章集成**（`tts_routes.py`）：`/api/article/tts` 读正文 → 调能力代理选中的插件 → 写回 frontmatter；面向编辑器。

这与 `image_upload` 的两层（`plugin_routes` 代理 + `image_routes.generate_cover` 集成）结构一致。

### Decision 5: frontmatter 写回用乐观锁

后台任务拿到 URL 后，用 `post_service.save_file(article_path, body, frontmatter | {audio: url}, expected_mtime)` 写回。`expected_mtime` 取自任务启动时读文件的 mtime。冲突则 emit `tts.conflict` 事件，前端引导重试或放弃（与编辑器既有 ConflictModal 的语义一致，不自动覆盖）。

### Decision 6: 音频元信息随 frontmatter 落地

除 `audio: <url>` 外，同时写 `audio_duration_seconds` 与 `audio_format`（可选），方便 Hugo 主题渲染。插件在 `TTSResult` 里返回这些值。

### Decision 7: 删除语义

`DELETE /api/article/tts` 既调插件 `Delete(audio_id)` 删 R2 object，又清空 frontmatter 的 `audio*` 字段。`audio_id` 暂存在内存不可行（后台任务结束即丢），故约定：frontmatter 额外存一个**非公开**字段 `_tts_audio_id` 作为删除凭证。若缺失则只清 frontmatter、不调插件 Delete（降级）。

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| MiMo 单次合成有长度上限 | 插件侧分片合成 + 拼接，host 侧不感知；进度按分片上报 |
| 后台任务期间文章被改 | 乐观锁 + `tts.conflict` 事件，前端引导处理 |
| R2 凭据泄漏 | 走插件 config_schema，host 用 Fernet 加密落盘（既有机制），明文仅在 `SetConfig` 时下发 |
| 旧插件不含 `TTSGenerator` 服务 | 它是可选服务；`get_tts_generator_stub` 仅对声明该能力的插件调用 |
| Go 1.21 与 aws-sdk-go-v2 | 插件 go.mod 锁定兼容版本；host 不关心插件 Go 版本 |
| 音频重复生成产生 R2 垃圾 | 重新生成时先 Delete 旧 audio_id（若 frontmatter 有凭证） |

## Migration Plan

1. 扩展 `proto/plugin.proto`，重新生成 Python 桩（host）
2. host：plugin_manager stub → plugin_routes 代理 → tts_routes 文章集成 → app 注册
3. 前端：api 封装 → Editor 按钮/播放器/进度
4. Go 插件独立仓库：骨架 → MiMo → R2 → plugin.toml → CI
5. 测试：host 单测覆盖；端到端自测留给用户（需 MiMo/R2 凭据）

无破坏性变更：既有 `image_upload` 流程与 API 不受影响。
