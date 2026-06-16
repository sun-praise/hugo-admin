## Why

The admin can trigger `git add → commit → push` ("系统发布") from the Dashboard, but once a push fires the user has no way to *see what happened*. Git commits are readable via the existing `GET /api/git/commits` endpoint, yet no frontend surfaces them; and git does not natively log push history, so past pushes are invisible — the user can only infer success from a toast. Authors need a Git page that shows recent commits and a persisted record of every push this admin performed (message, remote/branch, commit range, result, time).

## What Changes

- **Persist push history.** Extend the existing SQLite cache (`models/database.py`) with a `git_push_history` table, and record one row every time `GitService.push()` actually runs a push — capturing remote, branch, from/to commit range (best-effort), commit message, success, and a short error/summary. The single internal caller is `GitService.publish_system()`.
- **Push-history read API.** New `GET /api/git/pushes` (paginated, newest first) over the persisted records, alongside the existing `GET /api/git/commits` and `/api/git/status`.
- **Commit list enrichment (backend, minimal).** `GET /api/git/commits` already exists; augment its underlying `get_recent_commits()` output with `refs` (branches/tags pointing at the commit) and a `stats` summary (files/insertions/deletions) by switching its single `git log` invocation to emit `--numstat` inline — one subprocess, not one per commit. No change to the endpoint path or existing response shape — only added fields.
- **New "Git" page (frontend).** A `frontend/src/pages/Git.tsx` page with two tabs: **提交记录 (Commits)** — a paginated commit list with author, relative time, message, refs, and file-change stats; and **推送记录 (Pushes)** — the persisted push history showing each push's success/failure, commit range, message, and time. Both tabs fetch on mount and tab switch and have a manual refresh button; the page does **not** live-update when a publish runs from the Dashboard (it refetches next time it is visited). Add a "Git" nav entry to the Sidebar and a route in `App.tsx`.
- **Dashboard wiring.** The existing "系统发布" button already calls `/api/publish/system`; the recorded push appears in the Git page's Pushes tab the next time the page is visited or refreshed (no new button, no live cross-route update).

## Capabilities

### New Capabilities
- `git-history`: A frontend Git page plus the push-history persistence and read API that back it, letting an admin review recent commits and every push performed by this admin.

### Modified Capabilities
<!-- None. Commit reading already exists at the spec level (GET /api/git/commits); this change only enriches its payload (additive fields) and adds a UI for it. No existing requirement's behavior changes. -->

## Impact

- **Backend (new code)**: a `push_history` table + accessor methods in `models/database.py`; push-recording hook inside `services/git_service.py` (`push()` writes a row, success or failure); new `GET /api/git/pushes` route in `routes/publish_routes.py`; additive fields from `get_recent_commits()`. `GitService` gains an optional `database` dependency (injected via `ServiceRegistry`, mirrors how `post_service` is wired).
- **Backend (reused)**: existing `GitService._run_git_command`, `publish_system()`, `ServiceRegistry`, and the SQLite `Database` already used for post/chat caching. No new dependency; `git` subprocess is already the transport.
- **Config**: none. The DB path already lives at `content/.admin/cache.db`; pushes land in the same file. No migration of existing rows needed (additive `CREATE TABLE IF NOT EXISTS` + idempotent `_migrate_db`).
- **Frontend**: new `Git.tsx` page, a Sidebar nav item, an `App.tsx` route, and small typed helpers in `frontend/src/utils/api.ts` (`getCommits`, `getPushes`).
- **Tests**: `pytest` cases for the DB table, push recording on success/failure/skip, the `/api/git/pushes` route, and the enriched commit payload, following the repo's temp-repo/mock conventions. No frontend tests run in CI today (frontend has no test runner).
