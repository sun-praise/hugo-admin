## 1. 后端：新增独立 push 接口

- [x] 1.1 在 `routes/publish_routes.py` 中新增 `POST /api/git/push` 路由：解析可选 JSON body（`remote`/`branch`/`set_upstream`），调用 `registry.git_service.push(...)`，统一返回 `{ success, message, remote, branch }`，并按 spec 在非 git 仓库 / push 失败时返回 400、成功时返回 200。
- [x] 1.2 不改动 `services/git_service.py`，确认现有 `push()` 与 `_record_push()` 已经覆盖推送历史与异常吞咽逻辑。
- [x] 1.3 为新路由补一个最小化的 pytest 覆盖（mock `git_service.push` 验证三种响应：成功、失败、非 git 仓库）。

## 2. 前端 API 封装

- [x] 2.1 在 `frontend/src/utils/api.ts` 中新增 `GitStatus` / `GitStatusResponse` / `PushResponse` 三个类型。
- [x] 2.2 新增 `getGitStatus()` 与 `pushGit(remote?, branch?, setUpstream?)` 函数，复用现有 `get` / `post` 工具。

## 3. 前端：Git 页面状态 Tab

- [x] 3.1 在 `frontend/src/pages/Git.tsx` 中将 Tab 类型扩展为 `'status' | 'commits' | 'pushes'`，并把默认 Tab 改为 `status`。
- [x] 3.2 新增 `loadStatus()` 调用 `getGitStatus()`，缓存到 `status` 状态；手动刷新按钮统一调用当前 Tab 的 loader。
- [x] 3.3 实现 `StatusView` 组件：按 `staged` / `unstaged` / `untracked` 分组渲染文件列表（空组隐藏标题），`has_changes=false` 时展示 "工作区干净" 空态，非 git 仓库时展示错误消息。
- [x] 3.4 在 `StatusView` 顶部增加 "推送" 按钮 + `set_upstream` 复选框：调用 `pushGit()`，按下时禁用并显示 spinner；成功时同时刷新 status 与 pushes（page 1）；失败时展示错误消息。
- [x] 3.5 当 `has_changes=false` 且当前分支没有领先 upstream 的 commit 时，把按钮禁用并通过 `title` 说明原因；当前分支缺少 upstream 时默认勾选 `set_upstream` 并展示一次提示。

## 4. 前端：Tab 切换与刷新一致性

- [x] 4.1 调整现有 useEffect，确保切换到 status / commits / pushes 三个 Tab 时分别只加载对应数据；保持 `commitsCount` / `pushPage` 的 ref 模式以避免手动翻页时的重复请求。
- [x] 4.2 将 `PushesView` / `CommitsView` 现有手动刷新逻辑统一收敛到 `refresh()`，由 `refresh` 读取当前 `tab` 决定调用哪个 loader。

## 5. 验证

- [x] 5.1 启动后端，使用 `curl` 直接验证：`GET /api/git/status`、`POST /api/git/push`（带 body / 空 body / 失败场景）。
- [x] 5.2 启动前端开发服务器，浏览器打开 Git 页面，依次验证：状态 Tab 渲染、推送按钮触发、推送后状态与推送记录自动刷新、错误提示。
- [x] 5.3 运行 `pytest` 与前端 `npm run build` / `pnpm build`，确保没有回归。
