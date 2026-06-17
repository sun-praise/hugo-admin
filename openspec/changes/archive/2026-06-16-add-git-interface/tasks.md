## 1. Push-history persistence (SQLite)

- [x] 1.1 In `models/database.py` `_init_db`, add a `git_push_history` table (`id`, `remote`, `branch`, `from_sha`, `to_sha`, `commit_count`, `commit_message`, `success` (0/1), `message`, `pushed_at`) with `CREATE TABLE IF NOT EXISTS` and a `idx_pushed_at` index, matching the surrounding parameterized-SQL style. No change to `_migrate_db` needed (table is new).
- [x] 1.2 Add `Database.record_push(self, *, remote, branch, from_sha, to_sha, commit_count, commit_message, success, message)` that inserts one row with `pushed_at = time.time()` and returns the new row id; tolerate `from_sha`/`to_sha` being empty strings.
- [x] 1.3 Add `Database.list_pushes(self, limit=20, offset=0)` returning a dict `{success, pushes, total}` where each push is a dict (newest-first) with all columns plus an ISO-formatted `pushed_at_iso` derived from `pushed_at`; compute `total` via `SELECT COUNT(*)`.

## 2. Push recording inside GitService.push()

- [x] 2.1 Change `GitService.__init__(self, repo_path)` to `GitService.__init__(self, repo_path, database=None)`, storing `self.database`; keep it optional so existing tests without a DB stay green.
- [x] 2.2 In `push()`, before running `git push`, capture the current remote-tracking HEAD best-effort via `_run_git_command(["rev-parse", f"{remote}/{branch}"])` (empty string on failure / first push); after a successful push, capture `to_sha` the same way and `commit_count` via `git rev-list --count <from>..<to>` (fallback to `git rev-list --count <to_sha>` when `from_sha` is empty, else 0).
- [x] 2.3 After computing the push result (success tuple), if `self.database` is set, call `self.database.record_push(...)` inside a `try/except` that logs and swallows any error so recording never affects the returned push result; record on both success and failure with the corresponding `message` text.
- [x] 2.4 Leave `publish_system()` behavior unchanged beyond the now-recorded `push()` side effect — no new return fields.

## 3. Commit-list enrichment (single git log call)

- [x] 3.1 Rewrite `get_recent_commits(count=10)` to run a single `git log -<count> --pretty=format:%H|%an|%ae|%ad|%d|%s --date=iso --numstat` via `_run_git_command`; clamp `count` to `[1, 50]`.
- [x] 3.2 Parse the output in one pass: each block starts at a `%H|...` line, followed by zero-or-more tab-delimited `<ins>\t<del>\t<path>` numstat lines (treat `-` entries as binary: count toward `files` only) until the blank-line separator; accumulate per-commit `refs` (from `%d`, strip surrounding `()`/spaces) and `stats = {files, insertions, deletions}`.
- [x] 3.3 Keep the existing commit dict fields (`hash`, `author`, `email`, `date`, `message`) and add `refs` (string, possibly empty) and `stats` (object) to each entry; preserve the existing `{success, commits, message}` envelope.

## 4. Wiring (registry + app + settings rebuild)

- [x] 4.1 Add a `database` property (getter+setter) to `ServiceRegistry` mirroring `git_service`.
- [x] 4.2 In `app.py`, pass the existing `db` instance into `GitService(app.config["HUGO_ROOT"], db)` and set `registry.database = db` (alongside the existing `registry` construction).
- [x] 4.3 In `routes/settings_routes.py`, inside the `hugo_root`-change branch (~line 158-198), construct `new_db = Database(str(app.config["CONTENT_DIR"] / ".admin" / "cache.db"))` for the **new** content dir, set `registry.database = new_db`, and pass `new_db` into the rebuilt `GitService(new_root, new_db)`. The store is per-repo (`CONTENT_DIR = HUGO_ROOT/content`), so history is intentionally not carried across the switch — the new repo reads/writes its own `cache.db`. Do NOT retrofit the same fix onto `chat_history_service` in this change (pre-existing, out of scope).

## 5. Backend routes

- [x] 5.1 Add `GET /api/git/pushes` inside `register_publish_routes` (next to `/api/git/commits`): read `page` (default 1, min 1) and `per_page` (default 20, clamp `[1, 100]`) from `request.args`, compute `offset`, call `registry.database.list_pushes(limit=per_page, offset=offset)`, and return `{success, pushes, total, page, per_page, total_pages}`.
- [x] 5.2 Guard the new route for the case `registry.database` is missing (return 500 with a clear message) and wrap in `try/except` returning `{success:false, message}` on 500, matching the existing `/api/git/commits` handler style.
- [x] 5.3 No change to `GET /api/git/commits` route code beyond what it already passes through — the enriched payload from task 3 surfaces automatically; keep the existing `count` query param and 500-on-error handling.

## 6. Frontend

- [x] 6.1 Add typed helpers in `frontend/src/utils/api.ts`: `getCommits(count=20)` → `{success, commits: Commit[]}`, and `getPushes(page=1, perPage=20)` → `{success, pushes: PushRecord[], total, page, per_page, total_pages}`, plus the `Commit`/`PushRecord` types (`Commit` adds `refs` and `stats{files,insertions,deletions}` to the existing fields).
- [x] 6.2 Create `frontend/src/pages/Git.tsx` with two tabs — **提交记录** (commits list: short hash, author, relative time, message, refs badge, `±files/ins/del` summary, "加载更多" via increasing `count`) and **推送记录** (push history newest-first: success/fail icon, commit message, `<from>..<to>` short range or "—", remote/branch, timestamp; prev/next pagination matching `Posts.tsx`). Include a manual refresh button for the active tab and an empty-state ("暂无推送记录") for the pushes tab.
- [x] 6.3 Add the route `<Route path="git" element={<Git />} />` in `frontend/src/App.tsx` and import the page.
- [x] 6.4 Add a "Git" entry (`GitBranch` icon from `lucide-react`) to `navItems` in `frontend/src/components/Sidebar.tsx`.

## 7. Tests & lint

- [x] 7.1 Add DB tests in a new `tests/test_push_history_db.py` (or extend an existing db test file): `record_push` then `list_pushes` returns the row newest-first with correct `total`; empty `from_sha`/`to_sha` round-trip; `list_pushes(limit, offset)` paginates.
- [x] 7.2 Extend `tests/test_git_service.py`: `push()` records a successful push when a `Database` (temp file) is injected, records a failed push when the remote is unreachable, and is a no-op when `database=None`; `get_recent_commits` returns `refs` and non-negative `stats` (files/ins/del) from the single `--numstat` call, and clamps `count`.
- [x] 7.3 Add route tests (extend `tests/test_publish_api.py` or a new `tests/test_git_history_api.py`): `GET /api/git/pushes` default + paginated + clamped params + empty history; `GET /api/git/commits` still returns `hash/author/email/date/message` and now `refs`+`stats`. Use the repo's Flask test-client + temp-repo fixtures.
- [x] 7.4 Run `pytest` and `ruff check .` from the project root; ensure new files are ruff-clean and the full suite passes (the repo has pre-existing ruff errors in untouched files — do not "fix" those).
