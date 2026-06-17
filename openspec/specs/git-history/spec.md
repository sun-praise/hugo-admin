# git-history Specification

## Purpose
TBD - created by archiving change add-git-interface. Update Purpose after archive.
## Requirements
### Requirement: Persist push history
The system SHALL record one push-history entry every time `GitService.push()` executes a `git push`, capturing the remote, branch, commit message used, best-effort commit range, success/failure, result or error text, and timestamp, regardless of whether the push succeeds.

#### Scenario: Successful push is recorded
- **WHEN** `publish_system()` runs and the underlying `push()` to `origin/<branch>` succeeds
- **THEN** a push-history entry is stored with `success` true, the remote, branch, commit message, a best-effort `from_sha`/`to_sha` range, a commit count, and the push timestamp.

#### Scenario: Failed push is recorded
- **WHEN** `push()` is attempted but `git push` fails
- **THEN** a push-history entry is stored with `success` false, the remote, branch, the failure text as `message`, and the push timestamp, so the failure is not silently lost.

#### Scenario: Skipped push is not recorded
- **WHEN** `publish_system()` stops before reaching the push step (no changes, or add/commit fails)
- **THEN** no push-history entry is created, because no `git push` was executed.

#### Scenario: Recording failure never breaks the publish flow
- **WHEN** storing the push-history entry itself raises an exception
- **THEN** the exception is logged and swallowed, and the push result already returned to the caller is unaffected.

### Requirement: Read push history via API
The system SHALL expose `GET /api/git/pushes` returning push-history entries newest-first with pagination, each entry including remote, branch, commit message, commit range, commit count, success flag, result/error text, and timestamp.

#### Scenario: Paginated newest-first list
- **WHEN** a client requests `GET /api/git/pushes?page=2&per_page=10`
- **THEN** the system returns up to 10 entries ordered by `pushed_at` descending, skipping the newest 10, together with `total`, `page`, `per_page`, and `total_pages`.

#### Scenario: Default pagination
- **WHEN** a client requests `GET /api/git/pushes` with no pagination parameters
- **THEN** the system returns the first page with a default `per_page` of 20.

#### Scenario: Pagination parameters clamped
- **WHEN** a client requests an out-of-range `page` or `per_page`
- **THEN** `page` is clamped to a minimum of 1 and `per_page` is clamped to `[1, 100]`, and the response reflects the clamped values.

#### Scenario: Empty history
- **WHEN** no pushes have been recorded yet
- **THEN** the system returns `success` true with an empty `pushes` list and `total` 0.

### Requirement: Enriched commit list via API
The system SHALL return recent commits via `GET /api/git/commits` where each commit carries its hash, author, email, date, message, the refs (branches/tags) pointing at it, and an aggregate diffstat (files changed, insertions, deletions), produced by a single `git log` invocation with `--numstat`.

#### Scenario: Commit includes refs and stats
- **WHEN** a client requests `GET /api/git/commits?count=10`
- **THEN** each returned commit object includes `refs` and a `stats` object with non-negative `files`, `insertions`, and `deletions` counts derived from the `--numstat` block for that commit.

#### Scenario: Existing commit fields preserved
- **WHEN** a client requests `GET /api/git/commits`
- **THEN** each commit object still includes the pre-existing `hash`, `author`, `email`, `date`, and `message` fields with the same values and shapes as before this change.

#### Scenario: Count is clamped
- **WHEN** a client requests `GET /api/git/commits?count=999` or a non-positive count
- **THEN** the system clamps `count` to `[1, 50]` and returns at most 50 commits.

#### Scenario: Single subprocess invocation
- **WHEN** the system builds the commit list
- **THEN** it issues exactly one `git log` subprocess (with `--numstat`), not one subprocess per commit.

#### Scenario: Binary file changes counted as files only
- **WHEN** a commit changes a binary file (numstat reports `-` for insertions/deletions)
- **THEN** the commit's `stats.files` counts the file while `stats.insertions`/`stats.deletions` are unaffected by the binary entry.

#### Scenario: Non-git repository
- **WHEN** the configured `HUGO_ROOT` is not a git repository
- **THEN** the system returns `success` false with a message indicating the directory is not a valid git repository, matching the existing error behavior.

### Requirement: Git page surfaces commits and pushes
The admin UI SHALL provide a "Git" page with two views — recent commits and push history — each readable, paginated, and manually refreshable.

#### Scenario: Commits view
- **WHEN** the user opens the Git page
- **THEN** the commits view shows recent commits with author, relative time, message, refs, and file-change stats, fetched from `GET /api/git/commits`.

#### Scenario: Pushes view
- **WHEN** the user switches to the push-history view
- **THEN** the page shows recorded pushes newest-first with success/failure status, commit message, commit range, remote/branch, and time, fetched from `GET /api/git/pushes`, with prev/next pagination.

#### Scenario: Manual refresh
- **WHEN** the user clicks refresh on either view
- **THEN** the active view re-fetches its data from the backend.

#### Scenario: Navigation entry
- **WHEN** the sidebar is rendered
- **THEN** it contains a "Git" entry that navigates to the Git page.

#### Scenario: Empty push history message
- **WHEN** the pushes view loads and no pushes have been recorded
- **THEN** the page shows an empty-state message indicating there is no push history yet.

### Requirement: Push history is scoped to the configured repository
The system SHALL store push history in the per-repo SQLite cache at `CONTENT_DIR/.admin/cache.db` (where `CONTENT_DIR = HUGO_ROOT/content`), the same store used by the post cache and chat history. It SHALL NOT carry push history across a `HUGO_ROOT` change, and on such a change it SHALL rebuild the `Database` for the new content directory so the active repo reads and writes its own history.

#### Scenario: History is scoped to the active repo
- **WHEN** pushes are recorded for repo A and the admin then switches `HUGO_ROOT` to repo B
- **THEN** `GET /api/git/pushes` returns repo B's history (empty until B has its own pushes), not repo A's, because the store is rebuilt against the new content directory.

#### Scenario: Each repo keeps its own history on switch back
- **WHEN** the admin switches `HUGO_ROOT` back to repo A after recording pushes in both repos
- **THEN** `GET /api/git/pushes` returns repo A's previously recorded pushes, because each repo's `cache.db` holds its own `git_push_history` rows.
