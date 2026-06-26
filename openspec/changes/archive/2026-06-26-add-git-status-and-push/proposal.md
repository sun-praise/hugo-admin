## Why

Git 页面目前只展示历史 commit 与 push 记录，管理员无法直接看到当前仓库的工作区状态，也无法在不重新走完 `add + commit + push` 的情况下执行一次纯粹的 push。后端 `GitService.push()` 已经支持单独调用，前端却没有任何入口；二次推送（例如只想推一个已经存在的本地 commit，或远程连接恢复后补推）只能临时跑脚本或重启完整发布流程。本次变更在现有 Git 页面上新增"仓库状态"视图并暴露一个独立的 push 按钮，把这两个高频操作补齐。

## What Changes

- 新增 Git 页面 "状态" 视图：通过 `GET /api/git/status` 展示 staged / unstaged / untracked 三组文件列表，以及 `has_changes` 汇总徽标。
- 新增 Git 页面手动"推送"按钮：在状态视图与推送记录视图上提供一个独立的 push 控件，点击后调用新增的 `POST /api/git/push`，仅执行 `git push`，不触发 `add`/`commit`。
- 新增后端路由 `POST /api/git/push`：调用现有 `GitService.push()`，支持可选 `remote`、`branch`、`set_upstream` 形参；与现有 `POST /api/publish/system` 共用推送记录逻辑。
- 状态视图随推送结果自动刷新：push 成功并写完推送历史后，自动重新拉取状态（变干净）与推送记录列表，无需手动刷新。
- 推送按钮在仓库非 git、工作区干净（无未推送 commit 且 has_changes=false）时禁用，并在按钮旁展示禁用原因。

## Capabilities

### New Capabilities

- `git-status-view`: 在 Git 页面渲染仓库工作区状态（staged / unstaged / untracked 文件列表），并提供手动刷新。
- `git-push-action`: 暴露独立的 push 接口（不包含 add/commit）与前端按钮，用于"二次推送"等纯推送场景。

### Modified Capabilities

- `git-history`: 扩展 `Git page surfaces commits and pushes` 需求，新增状态视图与推送按钮场景；同时在 `Read push history via API` 能力之上新增"独立 push 接口"与"push 后联动刷新"的需求块。

## Impact

- **后端**:
  - `routes/publish_routes.py` 新增 `POST /api/git/push` 路由，复用 `registry.git_service.push()`。
  - `services/git_service.py` 无需改动，`push()` 已有完整的推送历史记录逻辑。
  - 现有 `GET /api/git/status` 路由已存在并返回 `{ success, has_changes, staged, unstaged, untracked, message }`，无需新增后端状态接口。
- **前端**:
  - `frontend/src/pages/Git.tsx` 新增 "状态" Tab、状态卡片视图和推送按钮。
  - `frontend/src/utils/api.ts` 新增 `getGitStatus()`、`pushGit(remote?, branch?, set_upstream?)` 调用。
- **数据**: 推送历史写入 `cache.db` 的 `git_push_history` 表，与现有逻辑一致，不引入新表。
- **兼容性**: 完全向后兼容；只在已有 Git 页面上叠加视图与按钮，不影响现有发布流程或推送记录。
