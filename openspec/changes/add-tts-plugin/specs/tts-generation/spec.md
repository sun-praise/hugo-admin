# Spec: TTS Speech Generation

## Requirements

### REQ-1: 插件协议支持 TTS 能力
The system SHALL allow plugins to declare a `tts_generation` capability by implementing the optional `TTSGenerator` gRPC service (`Generate` server-streaming + `Delete`), declared in the plugin manifest's `[capabilities]` section.

### REQ-2: 文章级语音生成
The system SHALL provide `POST /api/article/tts` that reads an article's body (stripping frontmatter), invokes a running plugin with `tts_generation`, and writes the resulting public audio URL into the article's frontmatter `audio` field.

### REQ-3: 异步进度上报
The system SHALL return immediately from `/api/article/tts` with an `event_scope`, and stream progress via Socket.IO events `tts.progress`, `tts.done`, `tts.failed`, and `tts.conflict`, each carrying the `scope` for client correlation.

### REQ-4: 乐观锁写回
The system SHALL write the `audio` field back using optimistic locking (`expected_mtime`); on mtime mismatch it SHALL emit `tts.conflict` instead of overwriting.

### REQ-5: 能力代理直通
The system SHALL provide `POST /api/plugins/<name>/tts/generate` and `DELETE /api/plugins/<name>/tts/<audio_id>` that proxy directly to a plugin's `TTSGenerator` service without touching article content.

### REQ-6: 无插件降级
When no running plugin provides `tts_generation`, the article-TTS endpoint SHALL return HTTP 400 with a clear message, and the frontend SHALL hide/disable the generate button.

### REQ-7: 删除语音
The system SHALL provide `DELETE /api/article/tts` that removes the frontmatter `audio` (and related) fields and, when an audio deletion id is available, asks the plugin to delete the hosted audio object.

## Scenarios

**Scenario: 生成语音播报**
- **GIVEN** 一个声明 `tts_generation` 的插件已启用、配置完整
- **WHEN** 用户在编辑器点击"生成语音播报"并提交
- **THEN** 系统返回 `event_scope`，前端收到 `tts.progress`，最终收到 `tts.done {url, duration_seconds}`，且文章 frontmatter 的 `audio` 字段被更新为该 URL

**Scenario: 无 TTS 插件**
- **GIVEN** 没有任何运行中的插件提供 `tts_generation`
- **WHEN** 用户尝试生成语音
- **THEN** 系统返回 400 "未找到支持 tts_generation 的插件"，前端按钮置灰/隐藏

**Scenario: 生成期间文章被修改**
- **GIVEN** TTS 后台任务进行中，文章被另一会话保存
- **WHEN** 任务尝试写回 frontmatter
- **THEN** 系统检测到 mtime 不匹配，emit `tts.conflict {current_mtime}`，不覆盖文件；前端提示冲突

**Scenario: 删除语音**
- **GIVEN** 文章 frontmatter 含 `audio` 与删除凭证
- **WHEN** 用户点击删除
- **THEN** 系统清除 frontmatter 的音频字段，并通知插件删除托管 object
