# 复盘：编辑器预览吞掉内嵌 iframe（#185 / PR #186）

日期：2026-11-13

本文把修复的根因复盘落在仓库里，避免只存在于 PR 描述中、后续无法核验。

## 1. 怎么发生

作者在 Markdown 正文里粘贴 B 站 / YouTube 的嵌入代码。`<iframe ...>` 是原生 HTML 块，`marked`（`gfm: true`）原样透传，实测带前后段落、多行属性的写法都能完整保留；真正丢内容的是 `renderMarkdown` 末尾的 `DOMPurify.sanitize()`：默认白名单不含 `iframe`，`allowfullscreen` / `frameborder` / `scrolling` 等展示属性也不在 `ALLOWED_ATTR` 里，于是整段被剥掉，前后段落直接相接，界面上表现为「预览吞内容」且没有任何报错。

## 2. 在哪引入

`frontend/src/utils/markdown.ts` 的 sanitize 配置。此前为 mermaid 图表、内联 SVG、`<details>` 补 `ADD_TAGS` / `ADD_ATTR` 时定下了「默认白名单 + 少量附加标签」的模型，但没有覆盖嵌入类标签；`frontend/src/pages/Editor.tsx` 的 `updatePreview()` 调用 `renderMarkdown(src, { mermaid: true })` 也没有对应开关。不是编辑器、后端或 Hugo 渲染的问题。

## 3. 为什么以前没拦住

- 改动前的预览按「纯 Markdown 正文」验收，手测文章里没有原生 HTML 块，净化器静默删标签不会报错、也不会留下占位符。
- 前端单测只有 `translationPreview` / `selectionReplace` / `textDiff`，没有任何用例钉住 `renderMarkdown` 与 DOMPurify 白名单之间的契约，因此白名单少一个标签既无测试失败也无可见异常。
- 「预览能显示」与「净化器放行」被当成同一件事，没有对最终 HTML 做断言。

## 4. 是否沉淀

- `frontend/src/utils/markdown.test.ts`：钉住四条边界——默认不放行 iframe（AIChat 行为不变）、`embeds: true` 放行展示属性但不放行 `srcdoc`、多行属性不被 markdown 解析破坏、代码块里的 iframe 仍转义显示。
- `FORBID_TAGS` / `FORBID_ATTR` 显式声明 `script` / `style` / `srcdoc`，把安全承诺从「依赖上游默认值」改成代码里的显式约束；URI 策略（拦截 `javascript:` / `data:`）仍由 DOMPurify 负责。
- `embeds` 做成 opt-in：只有文章编辑器开启，渲染模型输出的 AIChat 不放行第三方 frame。
- `frontend/src/utils/previewCollapse.ts` + 单测：把收起状态持久化与两栏 display 类抽成纯函数，补上回归护栏。
- 本次评审又发现一个同类盲区并一并修掉：预览收起（`display:none`）时 mermaid 的 `getBBox` 拿不到尺寸，且渲染 effect 只依赖 `[preview]`，再次展开不会重跑。现在隐藏时跳过渲染、可见时重跑，并把「按节点缓存 / 还原原始图表源码」抽成 `MermaidSourceCache`（内部 WeakMap，配 4 例单测），避免把已渲染的 SVG 文本当成图表定义重新解析。窄屏切页签的同类隐患同时覆盖。

## 边界

管理端预览放行 iframe，不等于 Hugo 发布页一定渲染：站点侧 goldmark 默认 `unsafe = false`，正文原生 HTML 仍可能被过滤，需要 `[markup.goldmark.renderer] unsafe = true`，属站点配置。
