## Context

- 现有 `GitService` 已经提供 `get_status()` 与 `push()`，并在 `push()` 内部完成推送历史记录（成功/失败都会写入 `cache.db` 的 `git_push_history` 表）。
- `routes/publish_routes.py` 已经暴露 `GET /api/git/status`、`GET /api/git/commits`、`GET /api/git/pushes` 与 `POST /api/publish/system`（add + commit + push 完整链路），但没有"只 push"的独立接口。
- `frontend/src/pages/Git.tsx` 目前只有"提交记录"与"推送记录"两个 Tab，无法直接看到当前工作区是否干净，也无法发起一次独立 push。
- 业务场景：管理员修改了一些 Hugo 配置或修复了 CI 工作流，希望直接 `git push` 一个已存在的本地 commit，而不想再次 `add`/`commit`；或者远程连接断开后，希望在恢复后从 UI 端补推一次。

## Goals / Non-Goals

**Goals:**

- 让管理员在 Git 页面上一眼看到工作区状态（staged / unstaged / untracked 文件列表与是否有改动）。
- 让管理员通过 Git 页面上的"推送"按钮触发一次纯 `git push`，无需走完整发布流程。
- 推送成功后联动刷新状态与推送记录视图，避免手动刷新。
- 与现有推送历史记录机制完全一致：成功/失败都写入 `git_push_history`，UI 也能看到这条记录。

**Non-Goals:**

- 不修改 `GitService.push()` 内部行为；不引入新的推送历史字段或新表。
- 不在 Git 页面上提供 `add` / `commit` 控件 —— 那些仍然归 "系统发布" 流程负责（`POST /api/publish/system`）。
- 不做远程分支选择器或复杂 push 选项；UI 仅暴露"推送"按钮 + 可选切换是否 `set_upstream`。
- 不替换 `GET /api/git/status` 接口；状态视图直接复用现有响应字段。

## Decisions

1. **复用现有 `GitService.push()` 与 `GET /api/git/status`**
   - 后端只新增一个路由 `POST /api/git/push` 作为薄壳：解析 JSON body（可选 `remote`、`branch`、`set_upstream`），调用 `registry.git_service.push()`，并返回 `(success, message)`。
   - 状态接口保持不变，状态视图直接消费既有 `{ success, has_changes, staged, unstaged, untracked, message }`。
   - *理由*: 现有服务层已经具备完整能力，避免重复实现推送历史/错误记录逻辑；只在路由层补一个入口。

2. **Git 页面新增"状态"Tab，与提交/推送并列**
   - 三个 Tab 顺序：`状态` → `提交记录` → `推送记录`，状态放在最前面因为它反映当前工作区、最容易被日常使用。
   - 状态卡片内分三组渲染文件列表（staged 绿色、unstaged 黄色、untracked 灰色），空组不渲染标题。
   - 顶部增加一个"推送"按钮（带可选 `set_upstream` 切换），按钮在以下条件禁用：(a) `!is_git_repo`，(b) 仓库干净且无新 commit，禁用时通过 `title` 属性说明原因。

3. **推送成功后联动刷新状态与推送记录**
   - push 按钮调用 `POST /api/git/push`，返回成功时：刷新当前状态视图（如果有 commits view/pushes view 也刷新）；返回失败时仅展示错误消息，不刷新。
   - *理由*: 让管理员在一个动作里看到完整闭环 —— push 完状态变干净、推送记录多一行。

4. **`set_upstream` 通过 Modal 中的复选框暴露**
   - 第一次推送（upstream 缺失）默认勾选；后续默认不勾选。
   - UI 通过一个轻量的二级确认（点击"推送"时若无 upstream 自动勾选并提示）来避免误操作。

5. **前端类型补齐**
   - `frontend/src/utils/api.ts` 新增 `GitStatus`、`GitStatusResponse`、`PushResponse` 三个接口，并提供 `getGitStatus()` 与 `pushGit(remote?, branch?, setUpstream?)` 函数。

## Risks / Trade-offs

- **重复推送风险** → 按钮默认在"无 upstream"时强制 `set_upstream=true` 避免创建错误分支；UI 上禁用条件覆盖"工作区干净 + 无新 commit"，防止空推。
- **长 push 卡顿** → 按钮在按下后立即禁用并显示加载 spinner；后端 push 不做超时（沿用 `subprocess.run` 默认行为），但失败会被 `_record_push` 完整捕获并写入历史，不会留下悬挂状态。
- **状态接口在 push 期间短暂过期** → 推送成功后状态自动刷新；UI 不轮询，避免给后端增加负担。
- **与 `POST /api/publish/system` 的语义重叠** → 两个接口职责清晰：前者只 push，后者 push + add + commit。文档（spec）明确写出差异。

## Migration Plan

- 后端：纯增量改动，新增一个 Blueprint 路由；现有所有接口保持原状。
- 前端：在 Git 页面上叠加一个 Tab + 一个按钮；现有提交记录与推送记录 Tab 不变。
- 回滚策略：如出现严重问题，移除 `POST /api/git/push` 路由 + 还原 `Git.tsx` 即可，不影响 `cache.db` 与已有推送历史。

## Open Questions

- 是否需要在 push 之前展示"即将推送的 commit 列表"（类似 `git log origin/main..HEAD` 的预览）？当前设计默认不做，可在后续迭代中加；如需要则同步扩展 spec。
- push 失败时是否要区分"网络错误"与"rejected（非快进）"两类提示？当前仅返回后端 `stderr`，由 UI 直接展示。
