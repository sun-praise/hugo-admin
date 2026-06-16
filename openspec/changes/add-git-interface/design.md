## Context

hugo-admin drives a Hugo blog that is itself a git repo at `app.config["HUGO_ROOT"]`. `services/git_service.py`
wraps `git` via `subprocess` and exposes: `is_git_repo`, `get_status`, `add_all`, `commit`, `push`,
`publish_system` (add→commit→push), and `get_recent_commits(count)`. `app.py` constructs a single
`GitService(app.config["HUGO_ROOT"])` and stores it on `ServiceRegistry.git_service`; `settings_routes`
may rebuild it when `hugo_root` changes.

Routes today (`routes/publish_routes.py`, registered as `register_publish_routes(registry)`):
- `GET /api/git/status` → `git_service.get_status()`
- `GET /api/git/commits?count=N` → `git_service.get_recent_commits(count)`
- `POST /api/publish/system` → `git_service.publish_system(message)` (called by the Dashboard "系统发布" button)

The commit endpoint already works. What's missing: (1) no UI surfaces commits, and the commit payload
is bare (hash/author/email/date/message — no refs, no diffstat); (2) **pushes are not recorded anywhere** —
`push()` returns a success tuple and the only evidence of a push is a transient toast. Git has no native
push log, so the app must own this record.

State already lives in SQLite at `content/.admin/cache.db` via `models/database.py` (`Database` class),
used for post cache, chat sessions, and post references. It uses `_init_db` (`CREATE TABLE IF NOT EXISTS`)
plus an idempotent `_migrate_db` for additive column changes. `app.py` constructs `db = Database(...)`,
stores `chat_history_service` on it, and registers it on `registry`.

Frontend (`frontend/src`) is React + react-router + Tailwind. Pages live in `src/pages/`, nav in
`src/components/Sidebar.tsx` (`navItems` array), routes in `src/App.tsx`. `Dashboard.tsx` already calls
`/api/git/status` and `/api/publish/system`. API helpers use `get/post` from `src/utils/api.ts`.
There is no frontend test runner.

## Goals / Non-Goals

**Goals:**
- A dedicated **Git page** that shows recent commits (enriched with refs + diffstat) and the persisted
  push history, with pagination and a manual refresh.
- **Persist every push** this admin performs — remote, branch, commit range, message, result, timestamp —
  so pushes are auditable after the toast disappears. Failures are recorded too.
- Additive only: no change to existing endpoint paths or response shapes; the commit payload only gains
  fields. No DB migration of existing rows.
- Reuse existing infra: the SQLite `Database`, `GitService._run_git_command`, `ServiceRegistry`, and the
  existing publish flow. No new dependency, no background worker.

**Non-Goals:**
- Pushing from the Git page (the publish action stays on the Dashboard; the Git page is read-only).
- Per-file diff viewer / file-content diffs — only aggregate diffstat (files/insertions/deletions) in
  this change.
- Recording pushes performed outside this admin (e.g. `git push` on the server CLI). Only pushes that go
  through `GitService.push()` are recorded — that is the authoritative set for this tool.
- Pull/fetch/merge/rebase history or remote-management UI.
- Real-time push streaming. The page refreshes on demand and after a Dashboard publish; no Socket.IO
  channel is added for history.

## Decisions

### 1. Persist push history in the existing SQLite cache, written from `GitService.push()`
Add a `git_push_history` table and a `Database.record_push(...)` / `Database.list_pushes(limit, offset)`
pair. `GitService` takes an optional `database` in its constructor; `push()` captures the **before** HEAD
of the remote-tracking branch, runs the push, captures the **after** HEAD (best-effort), and writes one
row regardless of success. **Why SQLite over a log file / reflog parsing:** the cache DB already exists and
is transactional; reflog is local-only and doesn't reflect remote state; a log file gives no querying.
**Alternative considered:** a standalone `pushes.db` — rejected to avoid a second store and a second
migration path. **Dependency injection:** `GitService(repo_path, database=None)` keeps `push()` testable
without a DB (no-op when `database is None`, matching today's tests).

### 2. Commit enrichment is additive, in a single `git log` call
Replace the current `git log` invocation with one call that emits stats inline:
`git log -<count> --pretty=format:%H|%an|%ae|%ad|%d|%s --date=iso --numstat`. `%d` adds `refs`; `--numstat`
prints one `<insertions>\t<deletions>\t<path>` line per changed file after each commit (binary files show
`-`/`-`). Parse the output in one pass: commit-rows start with the `%H|...` marker; subsequent tab-delimited
rows accumulate into that commit's `stats = {files, insertions, deletions}` until the blank-line separator.
The `GET /api/git/commits` response keeps its current fields and gains `refs` and `stats`. **Why one call:**
O(1) subprocess (vs. N `git show` calls), matches the existing `_run_git_command` pattern, and `--numstat`
gives machine-parseable per-file numbers (unlike `--shortstat` which needs a second regex). **Cap:** `count`
is clamped to `[1, 50]` server-side to bound output size. **Alternative considered:** a separate
`/api/git/commits/detail` endpoint — rejected; it doubles round-trips and is unnecessary when one `git log`
already carries everything.

### 3. New read-only route `GET /api/git/pushes` inside `register_publish_routes`
`?page=1&per_page=20` → `{success, pushes, total, page, per_page, total_pages}`, newest first. Lives next to
`/api/git/commits` and `/api/git/status` since it's the same domain. No write endpoint on this blueprint —
pushes are recorded as a side effect of the existing `/api/publish/system`. **Alternative considered:** a
new `git_routes.py` blueprint — rejected; one more read endpoint doesn't justify a new registration line,
and `publish_routes.py` already owns all git reads today.

### 4. Wiring: `GitService` gets `database` via `ServiceRegistry` (per-repo)
`ServiceRegistry` gains a `database` property (mirrors `git_service`). `app.py` passes the existing `db`
into `GitService(app.config["HUGO_ROOT"], db)` and sets `registry.database = db`. The `pushes` route reads
`registry.database.list_pushes(...)`. **Per-repo, not global:** the DB lives at
`CONTENT_DIR/.admin/cache.db` where `CONTENT_DIR = HUGO_ROOT/content` (`app.py:83-84,125`), so push history
is scoped to the configured repo — the same convention as the post cache and chat history that share this
file. History is **not** carried across a repo switch, by design.

**Hugo-root reconfigure must rebuild the Database, not re-pass the stale handle.** `settings_routes.py:158-198`
rebuilds `git_service`/`post_service`/`ref_service`/`hugo_manager` on a `hugo_root` change but today never
recreates the module-level `db` (a latent quirk that already leaves `chat_history_service` pointed at the
old repo). For this change, the reconfigure branch SHALL additionally construct
`new_db = Database(str(new_content / ".admin" / "cache.db"))`, assign `registry.database = new_db`, and pass
`new_db` into the rebuilt `GitService`. This keeps push history consistent with whichever repo is active
(each repo sees its own `cache.db`). **Why not move the store to a `HUGO_ROOT`-independent path (e.g. a
web-admin data dir):** that would give cross-repo history but diverges from the post-cache/chat-history
convention and widens scope; it is explicitly a non-goal. (Note: this change does not retrofit the same
stale-db fix onto `chat_history_service` — that is an independent pre-existing issue.)

### 5. Push-record shape and best-effort commit range
Row schema:
```
git_push_history(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  remote TEXT NOT NULL,            -- e.g. 'origin'
  branch TEXT NOT NULL,            -- e.g. 'main'
  from_sha TEXT,                   -- remote-tracking HEAD before push (may be '' on first push)
  to_sha TEXT,                     -- remote-tracking HEAD after push (best-effort)
  commit_count INTEGER DEFAULT 0,  -- commits pushed (best-effort, git rev-list --count)
  commit_message TEXT,             -- the commit message used (for publish_system flow)
  success INTEGER NOT NULL,        -- 0/1
  message TEXT,                    -- human-readable result/error text
  pushed_at REAL NOT NULL          -- time.time() at push time
)
```
Commit range is **best-effort**: captured via `git rev-parse <remote>/<branch>` before and after; if the
remote-tracking ref doesn't exist (first push) `from_sha` is empty and `commit_count` falls back to
`git rev-list --count <to_sha>` or 0. Failures to capture the range never block the push itself.

### 6. Frontend: one new page, two tabs, polling-free
`frontend/src/pages/Git.tsx` with tabs **提交记录** and **推送记录**. Commits tab fetches
`GET /api/git/commits?count=20` (+ "load more"); pushes tab fetches `GET /api/git/pushes` with simple
prev/next pagination matching `Posts.tsx`. A manual refresh button re-fetches the active tab. After a
successful Dashboard publish, `publishSystem()` in `Dashboard.tsx` does **not** navigate away (the Git page
may not be mounted); instead the Git page refetches on mount and on tab switch, and the Dashboard toast
already closes the loop. **No Socket.IO**: push history is low-frequency; polling/streaming is overkill.

### 7. Validation & safety
The Git page and its endpoints are read-only, so the only mutations remain `publish_system()` (unchanged
behavior, now with a recorded side effect) and the new DB writes inside `push()`. DB writes use the existing
parameterized-SQL pattern from `database.py`. `count`/`per_page` are clamped server-side. No untrusted
path input reaches these endpoints. Push recording failures are caught and logged — they must never cause
a publish to appear failed.

## Risks / Trade-offs

- **History starts empty / pre-existing pushes invisible** → inherent to "record on push". Accepted; the
  page shows "暂无推送记录" until the next push. Back-filling from reflog is explicitly a non-goal (reflog
  is local and misleading for remote pushes).
- **Commit-list cost** → one `git log` call with `--numstat` (O(1) subprocess), `count` clamped to ≤50; a
  page load is one subprocess, not one-per-commit, so the Git page stays fast under repeated refreshes.
- **Push fails before/after SHA capture** → commit range fields are best-effort and may be empty; `success=0`
  and the error text are always recorded, so failures are never silent.
- **`database is None` in tests/old call sites** → push recording is a no-op; existing `test_git_service.py`
  stays green without changes (except new tests that opt in).
- **DB path under `content/.admin/`** → already gitignored by the existing cache setup; push history lives
  alongside post/chat cache and is not itself committed.
- **Concurrent pushes** → SQLite serializes writes; two simultaneous publishes are already an anti-pattern
  in this single-admin tool, and a locked-write retry is out of scope.
- **No migration needed** — `CREATE TABLE IF NOT EXISTS git_push_history` runs in `_init_db`; `_migrate_db`
  stays idempotent. Rollback = drop the table + revert code; existing data is untouched.
