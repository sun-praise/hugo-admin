# Design: selection-assist

## Context

- hugo-admin 编辑器（`frontend/src/pages/Editor.tsx`）基于原生 `<textarea>`，已有 InlineEdit（`frontend/src/components/InlineEdit/`：选区 → 浮动 ✨ → Popup 预设 → `POST /inline-edit` 非流式单结果替换）。
- 后端 AI 调用集中在 `services/ai_service.py`（Anthropic 兼容 base_url + claude_agent_sdk；`chat` 流式带工具、`quick_rewrite` 一次性非流式无工具），路由统一 blueprint factory（`register_*_routes(registry)`）+ `install_auth_guard(app)` 全局鉴权。
- selection-assist（React 18 + CodeMirror + Hono）验证了目标交互，但其技术栈不可直接复用：CodeMirror/文件工作区/演示模式与 hugo-admin 重复，Hono LLM 转发会形成第二条模型配置路径。

## Goals / Non-Goals

**Goals:**

- 选中即得：翻译（难词标注）/ 润色（3 候选 + 词级 diff + 一键替换）/ 评价（全文上下文 + 博客语境维度 + 快速修复）三类并行建议
- 模型配置与调用统一走 `AIService`，SSE 流式（`start/delta/done/error`）
- 应用替换保留编辑器撤销能力；面板提供多次撤销
- 同选区缓存、防抖，控制调用成本

**Non-Goals:**

- 不做 gRPC 插件化（不新增 proto 能力）
- 不引入 CodeMirror / react-resizable-panels / i18n 框架 / 演示模式
- 第一版不做：面板宽度拖拽、惰性生成（视口内才请求）、自定义任务（custom）、编辑器内 inline diff 高亮（diff 只在面板内渲染）
- 不改 `/inline-edit` API 行为

## Decisions

### D1: 原生实现，不走插件协议

现有插件协议（`proto/plugin.proto`）是「host→子进程数据进出」的后端能力模型（ImageUploader/TTSGenerator），无 UI 扩展点；selection-assist 的核心价值在前端交互，gRPC 插件触达不到。且 AIChat、inline-edit、frontmatter 生成都直连 `AIService`，单独让 assist 走插件会形成双份 LLM 配置（插件有自己的 GetConfigSchema 体系）。备选「新增 TextAssist gRPC 能力」被否：当前没有可替换实现的需求，收益不抵双配置与调试成本。若未来需要策略可替换，再以独立变更扩展协议。

### D2: 后端形态——`routes/assist_routes.py` + `services/assist_service.py`

- `services/assist_service.py`：三类任务的 prompt 构建（移植 selection-assist `server/prompts.ts`）、LLM 输出 JSON 规整（移植 `normalize.ts`/`json.ts`：剥 code fence、提取首个 JSON 对象、字段校验与降级）。
- `AIService` 新增一次性**流式**无工具方法（复用 `_build_quick_rewrite_options` 的构造方式，stream 开启、逐 delta 产出文本），assist 层在其上包装 SSE。不新建第二条 SDK 通道。
- 端点：`POST /api/ai/assist/<task>`，task ∈ `translate|polish|review`。body 为 JSON：`selected_text`（必填）、`context_before`/`context_after`（润色/翻译用，各 ≤2000 字符）、`full_text`（评价必填，≤30000 字符）。SSE 事件：`start`（任务与模型名）、`delta`（增量文本）、`done`（规整后结构化结果）、`error`。
- 结构化结果契约：
  - translate: `{ "translation": str, "hard_words": [{ "word": str, "source": str, "gloss": str }] }`
  - polish: `{ "candidates": [{ "style": "formal"|"concise"|"restructure", "note": str, "text": str }] }`（恰好 3 候选；规整后不足 3 个视为规整失败，走 error 事件供客户端重试）
  - review: `{ "issues": [{ "type": str, "severity": "info"|"warn"|"high", "comment": str, "before": str|null, "after": str|null }] }`，维度按博客语境：断言过强/事实风险、标题与内容一致、术语不一致、口语化、结构可读性；before/after 必须成对出现否则视为无可修复项；`issues` 为空数组是合法结果（选区没有问题）。

### D3: 输入上限与鉴权沿用现有边界

assist 端点自动处于 `install_auth_guard` 之后（同 AIChat/inline-edit）。上限沿用 inline-edit 的防御尺度：选区 ≤5000、前后文各 ≤2000、全文 ≤30000，超限 400。评价任务全文超长直接拒绝（不做截断——半篇上下文得出的评价意见有误导性，宁可不给）。

### D4: 前端——常驻侧面板组件，InlineEdit 弹窗退役

- 新增 `frontend/src/components/AssistPanel/`（PanelShell + 三个 Section + hooks）。`Editor.tsx` 布局改为「编辑器 + 可折叠右侧面板」，面板收起时完全还原现状。
- 触发：textarea `selectionchange`/`mouseup`/`keyup` 监听 + 500ms 防抖；选区 <2 字符不触发；切换选区重置面板；同一选区（选区文本哈希）命中内存缓存（LRU 20）不重发请求。
- 应用替换用 `document.execCommand('insertText')`：这是 textarea 上唯一能保留原生 Ctrl+Z 撤销栈的替换方式（`setRangeText` 会破坏 undo）。面板内另维护「应用记录栈」支持多步撤销（恢复上次替换的旧文本）。execCommand 已标记 deprecated 但全浏览器仍支持——风险见下。
- 词级 diff 用 `diff`（jsdiff）的 `diffWords`，仅在面板内渲染（编辑器本体不动）。
- InlineEdit 前端组件（Trigger/Popup/Overlay/presets）曾在本变更中移除，后按用户决策恢复并与面板并存：✨ 弹窗提供显式入口与自定义指令（面板第一版不做），两套入口各自独立（弹窗走非流式 `/inline-edit`，面板走流式 assist 端点）；`/inline-edit` API 全程未动。

### D5: 并行三任务，不做惰性生成

第一版选区稳定后对已启用任务并行发请求（实现简单、首屏体验好）；惰性生成（润色/评价进入视口再请求）留待后续按成本观察决定。同选区缓存已覆盖主要重复成本。

### D6: 任务开关——翻译默认停用，localStorage 持久化

中文博客写作里翻译的使用频率显著低于润色/评价，默认关掉翻译可以省掉每次选区的一笔请求。开关只做任务级启停（不做自定义指令/自由任务，留待后续独立变更）：停用的任务不发请求、不渲染区块；开关对当前选区即时生效（停用即中止在途请求，启用即补发）。配置存浏览器 localStorage（单机自用足够，不进服务端 Settings 体系）。面板不做"会话内记住折叠"——只有刻意选中文字才触发弹出，输入过程不会打扰；关闭后下次选中自然重现，避免出现"面板不弹出但原因不可见"的隐式状态。

## Risks / Trade-offs

- [LLM 输出非严格 JSON] → 服务端规整（fence 剥离 + 首个 `{...}` 提取 + 字段级校验）；仍失败则发 `error` 事件，前端可一键重试并绕过缓存。
- [`execCommand` 已废弃，未来浏览器移除] → 封装在单一工具函数中，检测失败时降级 `setRangeText` + 面板撤销栈兜底；替换路径收敛于一处，切换成本低。
- [评价任务全文上下文的 token 成本] → 30000 字符硬上限 + 同选区缓存；单篇多次评价靠缓存吸收。
- [移除 InlineEdit 弹窗是行为变化] → 交互由面板一对一承接（选中即触发，比 ✨ 更省一步）；`/inline-edit` API 与其测试保留，回滚只需还原前端组件。
- [三任务并行放大单次选区的调用成本] → 防抖 + 缓存 + 三个请求各自独立失败互不影响；模型沿用 AIService 现有配置，无新增计费路径。

## Migration Plan

纯增量 + 前端入口替换，无数据迁移。部署即生效；回滚 = revert 单个合并提交，`/inline-edit` API 全程未动。

## Open Questions

- 自定义指令（custom task）与面板宽度拖拽是否进第二版——倾向独立小变更。
- assist 是否需要独立的模型选择（如评价用更强模型）——第一版复用全局配置，观察后再定。
