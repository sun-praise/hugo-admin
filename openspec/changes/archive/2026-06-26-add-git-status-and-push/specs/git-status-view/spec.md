## ADDED Requirements

### Requirement: Surface repository working-tree status
The admin UI SHALL render a "Status" view on the Git page that displays the current working-tree state returned by `GET /api/git/status`, including `has_changes`, the staged / unstaged / untracked file lists, and a manual refresh control. The view SHALL auto-fetch on mount and re-fetch on refresh.

#### Scenario: Status view loads working-tree data
- **WHEN** the user opens the Git page and the "Status" tab is active (or the page defaults to it)
- **THEN** the page issues `GET /api/git/status` and renders the `has_changes` flag and the staged / unstaged / untracked file lists grouped by their category.

#### Scenario: Non-git repository shows a clear message
- **WHEN** `GET /api/git/status` returns `success` false (e.g. directory is not a git repository)
- **THEN** the status view shows the server-supplied `message` in an empty/error state and does not render the push action.

#### Scenario: Clean working tree
- **WHEN** `GET /api/git/status` returns `has_changes` false and all three file lists are empty
- **THEN** the status view renders an empty-state message indicating the working tree is clean, and the push action is disabled with an explanation that there is nothing to push (unless new local commits exist that have not been pushed).

#### Scenario: Manual refresh re-fetches status
- **WHEN** the user clicks the refresh control on the status view
- **THEN** the page re-issues `GET /api/git/status` and replaces the rendered state with the new response.

#### Scenario: Empty file groups are hidden
- **WHEN** one or more of the staged / unstaged / untracked lists is empty
- **THEN** the status view omits the heading for that group rather than rendering an empty list, so the user only sees sections that contain files.

### Requirement: Push action is wired to the status view
The admin UI SHALL expose a "Push" action on the Git page that is available from the status view, calls the dedicated push endpoint, and reflects the result back into the visible status and push history without requiring a manual page reload.

#### Scenario: Push button reuses the existing push service
- **WHEN** the user clicks "Push" while on the status view
- **THEN** the page issues `POST /api/git/push` with the currently selected remote / branch (and `set_upstream` flag), shows a loading indicator while the request is in flight, and disables the button to prevent double-clicks.

#### Scenario: Successful push refreshes status and history
- **WHEN** `POST /api/git/push` returns `success` true
- **THEN** the page re-issues `GET /api/git/status` and `GET /api/git/pushes` (page 1) so the status view reflects the new working-tree state and the new push-history entry appears in the push list.

#### Scenario: Failed push surfaces the error
- **WHEN** `POST /api/git/push` returns `success` false
- **THEN** the page displays the server-supplied error message inline next to the push button, keeps the button enabled for a retry, and does not refresh status or push history.

#### Scenario: Push button disabled when there is nothing to push
- **WHEN** the status view is clean AND the local branch has no commits ahead of its upstream
- **THEN** the push action is rendered as disabled with a `title` attribute explaining that there is nothing to push; clicking it SHALL be a no-op.

#### Scenario: First push to a new branch sets upstream by default
- **WHEN** the current branch has no upstream configured
- **THEN** the push action is rendered with `set_upstream` pre-selected; a non-technical warning indicates this will create the remote tracking branch.

### Requirement: Status, commits, and push history live behind tabs
The admin UI Git page SHALL organise its features into three tabs — "状态" (status), "提交记录" (commits), "推送记录" (pushes) — that share a single header and a single manual refresh control, and SHALL remember the active tab in component state for the duration of the session.

#### Scenario: Tab navigation
- **WHEN** the user clicks any of the three tab buttons
- **THEN** only the corresponding view is rendered and the active tab is visually highlighted; the previously active view's pagination/load-more state SHALL be preserved in memory so returning to it does not re-fetch when the underlying data is still current.

#### Scenario: Switching tabs triggers a fresh fetch for the target view
- **WHEN** the user switches to a tab whose data has not been loaded yet (or whose underlying data may be stale)
- **THEN** the page issues the corresponding API call (`GET /api/git/status`, `GET /api/git/commits`, or `GET /api/git/pushes`) for the new view.

#### Scenario: Single shared refresh button
- **WHEN** the user clicks the header refresh control
- **THEN** only the currently active tab's data is re-fetched; the other tabs' data is not affected.
