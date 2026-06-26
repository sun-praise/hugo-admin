## ADDED Requirements

### Requirement: Standalone push endpoint
The system SHALL expose `POST /api/git/push` that performs only the `git push` step (no `git add` or `git commit`) using `GitService.push()` and returns its result without altering the working tree. The endpoint SHALL accept an optional JSON body with `remote` (string, default `"origin"`), `branch` (string, default = current branch), and `set_upstream` (bool, default `false`), and SHALL return `{ success, message, remote, branch }`.

#### Scenario: Default push uses origin and current branch
- **WHEN** a client sends `POST /api/git/push` with an empty body or no body
- **THEN** the system calls `GitService.push(remote="origin", branch=<current branch>, set_upstream=False)` and returns the result with `remote` and `branch` echoed in the response.

#### Scenario: Explicit remote and branch are honored
- **WHEN** a client sends `POST /api/git/push` with `{ "remote": "github", "branch": "main", "set_upstream": true }`
- **THEN** the system calls `GitService.push(remote="github", branch="main", set_upstream=True)` and returns the corresponding `remote` and `branch` values in the response.

#### Scenario: Non-git repository returns 400
- **WHEN** `HUGO_ROOT` is not a git repository
- **THEN** the endpoint returns HTTP 400 with `{ success: false, message: "当前目录不是有效的 git 仓库" }`, matching the existing status endpoint behavior.

#### Scenario: Push failure is reported with 400
- **WHEN** the underlying `git push` command exits non-zero (e.g. rejected non-fast-forward, missing remote)
- **THEN** the endpoint returns HTTP 400 with `{ success: false, message: <git stderr trimmed>, remote, branch }`, so the UI can show the user what went wrong.

#### Scenario: Push success is reported with 200
- **WHEN** the underlying `git push` command exits zero
- **THEN** the endpoint returns HTTP 200 with `{ success: true, message: "推送成功到 <remote>/<branch>", remote, branch }`.

#### Scenario: Push history is recorded
- **WHEN** `POST /api/git/push` executes a push (success OR failure)
- **THEN** a row is written to `git_push_history` via the existing `Database.record_push()` call inside `GitService.push()`, with `remote`, `branch`, the best-effort commit range, success flag, and message — matching the behavior of `POST /api/publish/system`'s push step.

#### Scenario: Recording failure does not break the response
- **WHEN** writing to `git_push_history` raises an exception
- **THEN** the exception is logged and swallowed by `GitService._record_push()` and the HTTP response still reflects the underlying push result.

### Requirement: Push endpoint requires no body fields and is safe to retry
The system SHALL treat `POST /api/git/push` as an idempotent-style action with respect to its public contract: a client may call it multiple times with the same parameters, and the system SHALL NOT introduce additional state beyond what `git push` itself and the push-history table already record.

#### Scenario: No body is acceptable
- **WHEN** a client sends `POST /api/git/push` with no body
- **THEN** the system processes the request normally and does not return a 4xx for the missing body.

#### Scenario: Empty JSON object is acceptable
- **WHEN** a client sends `POST /api/git/push` with `{}`
- **THEN** the system behaves identically to the no-body case.

#### Scenario: Unknown JSON fields are ignored
- **WHEN** a client sends extra fields alongside `remote` / `branch` / `set_upstream`
- **THEN** the system ignores them and uses only the recognised fields, so older clients can adopt the endpoint without coordination.
