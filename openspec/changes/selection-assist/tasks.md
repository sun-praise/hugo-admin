# Tasks: selection-assist

## 1. 后端：AIService 流式一次性调用

- [x] 1.1 在 `services/ai_service.py` 新增流式无工具的一次性调用方法（复用 `_build_quick_rewrite_options` 构造方式，开启流式，逐 delta 产出文本），保留超时与空结果错误语义
- [x] 1.2 pytest 覆盖新方法（流式产出、空结果、超时），沿用现有 ai_service 测试的构造方式

## 2. 后端：assist 服务层

- [x] 2.1 新建 `services/assist_service.py`：三类任务的 prompt 构建（自 selection-assist `server/prompts.ts` 移植，评价维度改为博客语境：断言过强/事实风险、标题一致、术语一致、口语化、结构可读性）
- [x] 2.2 实现输出规整：剥 code fence、提取首个 JSON 对象、按任务校验必需字段（translate: translation/hard_words；polish: 恰好 3 个 candidates；review: issues 数组），失败抛出可识别异常
- [x] 2.3 pytest 覆盖规整逻辑：fence 包裹、夹带说明文字、字段缺失、候选数不足、非法 JSON

## 3. 后端：assist 路由

- [x] 3.1 新建 `routes/assist_routes.py`（blueprint factory，`register_assist_routes(registry)`），实现 `POST /api/ai/assist/<task>`：参数校验（task 白名单 404、必填/上限 400、review 必须带 full_text、超限整体拒绝不截断）、SSE `start/delta/done/error` 事件、AI 未配置 503
- [x] 3.2 在 `app.py` 注册 assist 路由
- [x] 3.3 pytest 覆盖端点：三任务成功流（断言事件序列与 done 载荷契约）、未登录 401、超限 400、review 缺 full_text 400、非法 task 404、AI 未配置 503、规整失败走 error 事件

## 4. 前端：基础设施

- [x] 4.1 `frontend` 安装 `diff` 依赖；封装词级 diff 工具（基于 `diffWords`，输出 React 可渲染的片段结构）
- [x] 4.2 封装备选区替换工具函数：优先 `document.execCommand('insertText')`（保留原生 Ctrl+Z），失败降级 `setRangeText`，返回 `{ start, end, oldText, newText }` 供撤销栈使用
- [x] 4.3 新建 SSE 客户端工具（消费 assist 端点，解析四类事件，支持 AbortController 取消）

## 5. 前端：AssistPanel 组件

- [x] 5.1 新建 `frontend/src/components/AssistPanel/`：面板骨架（展开/折叠、折叠状态会话内记忆、加载/空/错误态）与三任务区块布局
- [x] 5.2 翻译区：流式渲染译文、难词下划线标注与悬停浮层（原文 + 释义）
- [x] 5.3 润色区：3 候选卡片、词级 diff 高亮、应用候选（选区替换 + 其余候选锁定）
- [x] 5.4 评价区：意见列表（类型 + 严重级别）、before/after 快速修复（逐字定位、定位失败显示“无法定位”）、已应用标记
- [x] 5.5 应用记录栈与“撤销”按钮（后进先出恢复原文），显示可撤销步数
- [x] 5.6 单任务失败显示重试（绕过缓存）；“重新生成”清除该选区缓存并重跑

## 6. 前端：Editor 接入与 InlineEdit 退役

- [x] 6.1 `Editor.tsx` 布局接入常驻可折叠面板；选区监听（mouseup/keyup/select + 500ms 防抖，≥2 字符触发，切换选区重置，折叠时收起）
- [x] 6.2 实现同选区缓存（选区文本哈希为键，LRU 20，内存态，位于 `useAssist.ts`）
- [x] 6.3 移除 InlineEdit 前端组件（Trigger/Popup/Overlay/presets）及其在 Editor 中的挂载点；确认 `/inline-edit` 相关后端测试仍通过

## 7. 验证与收尾

- [x] 7.1 后端全量 `pytest`（根目录，449 个）通过；`ruff check .` 仅剩 main 上已存在的存量错误（`routes/__init__.py` 的 `register_project_init_routes` 未列入 `__all__`，本变更未引入新错误）
- [x] 7.2 前端 `pnpm lint`、`pnpm build`（含 tsc）通过
- [ ] 7.3 手动端到端验证：启动服务，在编辑器选中一段中文与一段英文，核对三任务结果、难词悬停、diff、应用/撤销、缓存命中（无重复请求）、未配置 AI 时的错误提示（部分完成：服务启动 ✅、未登录 401 ✅、SPA+新 bundle 200 ✅、上游失败走空闲超时 ✅；真实 LLM 往返被环境阻塞——当前 DEEPSEEK_API_KEY 已失效，浏览器内交互未验证）
- [x] 7.4 按 openspec 流程校验变更（`openspec validate`），准备归档材料

## 8. 任务开关（评审后新增需求）

- [x] 8.1 types 增加 `AssistTaskConfig` 与默认值（翻译关、润色/评价开）；useAssist 增加 `setEnabled`/`disableTask`，assist 只运行已启用任务
- [x] 8.2 AssistPanel 头部增加任务配置弹层（三个复选框），停用任务不渲染区块；全部停用时给出提示
- [x] 8.3 Editor 持久化任务配置（localStorage `assist-task-config`），开关对当前选区即时生效（停用中止在途、启用补发 idle 任务）
- [x] 8.4 前端 tsc/eslint/build 通过；openspec 校验通过；提交推送
- [x] 8.5 移除「会话内记住折叠」机制（sessionStorage dismissed 标记）：旧遮罩版本点击编辑器会写入该标记且刷新不消失，导致面板不再弹出且原因不可见；改为刻意选中（含重新选中同一段）即展开、输入过程不弹出，spec/design 同步

## 9. 恢复 InlineEdit 快速编辑弹窗（用户决策）

- [x] 9.1 从 main（213b433）恢复 InlineEdit 四个组件（Trigger/Popup/Overlay/presets）原样
- [x] 9.2 Editor 重新挂载 InlineEditOverlay 与 applyInlineEdit/drift 处理，与面板并存
- [x] 9.3 spec 移除「InlineEdit 弹窗退役」requirement（补「重新生成」场景至缓存 requirement），proposal/design 同步为「并存」

## 10. v3 Go 移植（main 迁移至 Go 后）

- [x] 10.1 rebase 到 v3 main：Python 后端（app.py/routes/services 及 assist 相关 pytest）按删除解决，逻辑转 Go
- [x] 10.2 `internal/ai/assist.go`：三任务 prompt + AssistStream（无工具流式，AssistEvent 带 Err 透传上游错误）
- [x] 10.3 `internal/ai/assist_normalize.go`：ExtractJSON/NormalizeFor（fence、尾逗号、字段回退、severity 别名、polish 恰好 3、review 空 issues 合法）
- [x] 10.4 `internal/httpapi/assist_handlers.go` + server.go 路由注册：SSE start/delta/done/error，断连由 request context 取消
- [x] 10.5 Go 测试：assist_test.go（prompt/提取/规整）+ assist_http_test.go（mock provider 端到端：happy/不足 3/error/空 issues/404/400×4/503/401）
- [x] 10.6 前端融合 main 的 useEvents/toast 重构（showNotification → toast），tsc/eslint/build 通过；go build/go vet/go test ./... 全绿；openspec validate 通过

## 11. PR #172 第一轮评审修复

- [x] 11.1 goroutine 泄漏（CodeRabbit Major + rs quality/performance 共同发现）：AssistStream 的发送与事件循环均 select ctx.Done()——handler 停止读取或 runner 不关 events 都不泄漏
- [x] 11.2 handler 上下文守卫：超时/断连后不把半截输出规整成 done；断连不发帧，超时且客户端仍在发 error
- [x] 11.3 面板重置键改完整选区身份（start/end/text）；applyFix 选区范围优先定位 + expectedOld 漂移校验
- [x] 11.4 翻译 loading 增量抽取译文片段（不再裸渲染 JSON 流）；DiffText React.memo+useMemo；SSE delta 50ms 批处理
- [x] 11.5 retry/regenerate 接收当前正文（修过期 fullText）；缓存仅 review 依赖全文（translate/polish 不再陪跑重算）；sse abort 后丢弃陈旧帧 + decoder 尾部 flush
- [x] 11.6 Go 测试补缺：边界 5000/2000/30000 正例、非 JSON 输出 → error、断流上游 → error、delta 拼接还原、review issue 上限、AssistStream 取消回归
- [x] 11.7 文档澄清：proposal 移植描述 Go 化；design 标注 D1–D6 为 v2 遗留（45s 等为 v2 行为）
- [ ] 11.8 前端测试框架（vitest 从零接入）：排期确认后再做（rs 接受书面排期）
