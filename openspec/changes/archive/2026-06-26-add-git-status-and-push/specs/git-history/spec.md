## RENAMED Requirements

- FROM: `### Requirement: Git page surfaces commits and pushes`
  TO: `### Requirement: Git page surfaces commits, pushes, and working-tree status`

## MODIFIED Requirements

### Requirement: Git page surfaces commits, pushes, and working-tree status
The admin UI SHALL provide a "Git" page with three views — working-tree status, recent commits, and push history — each readable, paginated where applicable, and manually refreshable. The page SHALL also expose a dedicated push action that performs a standalone `git push` (without add/commit) and is wired into the status view and the push-history view.

#### Scenario: Commits view
- **WHEN** the user opens the Git page
- **THEN** the commits view shows recent commits with author, relative time, message, refs, and file-change stats, fetched from `GET /api/git/commits`.

#### Scenario: Pushes view
- **WHEN** the user switches to the push-history view
- **THEN** the page shows recorded pushes newest-first with success/failure status, commit message, commit range, remote/branch, and time, fetched from `GET /api/git/pushes`, with prev/next pagination.

#### Scenario: Status view
- **WHEN** the user opens the status tab
- **THEN** the page shows the current working-tree state with `has_changes`, the staged / unstaged / untracked file lists, and a manual refresh control, fetched from `GET /api/git/status`.

#### Scenario: Manual refresh
- **WHEN** the user clicks refresh on any view
- **THEN** the active view re-fetches its data from the backend.

#### Scenario: Navigation entry
- **WHEN** the sidebar is rendered
- **THEN** it contains a "Git" entry that navigates to the Git page.

#### Scenario: Empty push history message
- **WHEN** the pushes view loads and no pushes have been recorded
- **THEN** the page shows an empty-state message indicating there is no push history yet.

#### Scenario: Dedicated push button on the status view
- **WHEN** the user is on the status view
- **THEN** a "推送" button is visible and invokes `POST /api/git/push`; on success the page refreshes the status view and the push-history view, and on failure it shows the server error inline.

#### Scenario: Push button disabled on a clean tree with no ahead commits
- **WHEN** the status view reports `has_changes` false and the current branch has no commits ahead of its upstream
- **THEN** the push button is disabled with a tooltip explaining there is nothing to push.
