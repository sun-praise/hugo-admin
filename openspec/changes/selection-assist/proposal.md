# Proposal: selection-assist

## Why

编辑器目前的 AI 辅助只有「选区 → 浮动弹窗 → 单次改写」（InlineEdit，非流式、单结果、无对照），翻译/润色/评审要靠 AIChat 往返粘贴。selection-assist 项目（gitea.local/sun-praise/selection-assist）已验证「选中即并行得到 翻译/润色/评价 三类建议」的交互价值——把「想怎么改」变成「选哪个」。本变更将该能力以原生方式并入 hugo-admin（明确不做 gRPC 插件，理由见 design），模型调用统一走 ai_service，避免出现第二条 LLM 配置路径。

## What Changes

- 新增后端 assist API：`POST /api/ai/assist/translate|polish|review`，SSE 流式（`start/delta/done/error`），严格 JSON 输出 + 服务端规整，复用 `services/ai_service.py` 的模型配置；prompt 编排与 JSON 规整逻辑从 selection-assist 的 `server/prompts.ts`、`normalize.ts`、`json.ts` 移植为 Python
- 前端 Editor 页新增常驻选区辅助面板：选中文字自动触发（防抖），三类任务并行生成
  - 翻译：中英互译，LLM 判定的难词虚线下划线，悬停显示原文与释义
  - 润色：3 个候选（正式/精简/句式重构），词级 diff 高亮，一键替换选区
  - 评价：以全文为上下文，按博客语境维度（断言过强、标题与内容不符、术语不一致、口语化、结构可读性）给出意见，部分意见带 before→after 快速修复
- 应用/撤销栈；同一选区（文本哈希）结果缓存；选区切换时面板重置
- 任务开关：翻译默认停用（润色/评价默认启用），localStorage 持久化，启停对当前选区即时生效
- 前端新增依赖：`diff`（词级 diff）；不引入 CodeMirror、文件工作区、演示模式（hugo-admin 已有对应能力或不需要）
- InlineEdit 浮动弹窗（✨ 快速编辑：预设改写 + 自定义指令）保留，与选区辅助面板并存（按用户决策：显式入口与自定义指令仍有价值，接受双 AI 入口）；`/inline-edit` API 保留不动，无 BREAKING

## Capabilities

### New Capabilities

- `selection-assist-api`: 选区辅助后端 API——翻译/润色/评价三类任务的 SSE 端点、prompt 编排与严格 JSON 规整、全文上下文输入、鉴权与错误语义
- `selection-assist-panel`: 编辑器选区辅助面板——选区触发与防抖、三任务并行展示、难词标注、词级 diff、多候选、应用与撤销、同选区缓存

### Modified Capabilities

（无——InlineEdit 现有行为未被任何 spec 覆盖，`/inline-edit` API 行为保持不变）

## Impact

- 后端：新增 `routes/assist_routes.py` 与 assist 服务层（prompt/normalize/JSON 规整）；复用 `services/ai_service.py`；app 注册新 blueprint
- 前端：`frontend/src/pages/Editor.tsx` 接入选区监听与新面板；新增 `frontend/src/components/AssistPanel/`；`frontend/package.json` 新增 `diff` 依赖
- 测试：pytest 覆盖 assist API（SSE 事件序列、JSON 规整边界、鉴权）；前端 `pnpm lint && pnpm build`
- 不改数据库、不改插件协议、不加 gRPC 能力、无 BREAKING
