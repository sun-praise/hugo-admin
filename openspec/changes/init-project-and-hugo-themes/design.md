## Context

hugo-admin is a Flask + React application that manages an existing Hugo site. The current configuration model assumes `HUGO_ROOT` points to a directory that already contains a Hugo site (`config.toml`/`hugo.toml`, `content/`, etc.). Settings are persisted in `<HUGO_ROOT>/.admin/settings.json` and include AI config, Hugo base directory, and server URL. The `HugoServerManager` already supports passing `--theme` via the `HUGO_THEME` environment variable, but there is no UI or persistent mechanism for theme selection.

## Goals / Non-Goals

**Goals:**
- Allow an authenticated admin to create a new Hugo site from the UI.
- Allow an authenticated admin to install Hugo themes into the current project.
- Allow an authenticated admin to select an active theme that is used by the dev server and future builds.
- Allow an authenticated admin to preview any installed theme by restarting the Hugo dev server with that theme.
- Keep the existing settings and server-management flows backward-compatible.

**Non-Goals:**
- Hosting or publishing themes (this is local project tooling only).
- Full Hugo module theme support in the first iteration (focus on `themes/` directory and Git submodules).
- Migrating existing content between themes automatically.
- Multi-site management beyond the current single active `HUGO_ROOT`.

## Decisions

### 1. Two new domain services
- **ProjectInitService** handles validation and execution of `hugo new site`. Keeping it separate from `SettingsService` avoids mixing site lifecycle concerns with setting persistence.
- **ThemeService** handles theme discovery (reading `themes/`), installation (Git submodule or plain clone/copy), and activation. It centralizes all theme-related logic.

Rationale: The existing `HugoServerManager` is already focused on process management; adding init/theme logic there would violate single responsibility. A dedicated service per capability keeps the code modular and testable.

### 2. Theme activation stored in `.admin/settings.json`
The active theme is stored under `theme.name` in the existing settings file. `HugoServerManager.start()` reads this value from the registry/settings service and appends `--theme <name>` to the `hugo server` command, removing the current reliance on the `HUGO_THEME` environment variable.

Rationale: Reuses the existing persistence layer and keeps the active theme visible in the settings UI. It also allows runtime theme changes without restarting the admin process.

### 3. Git submodule as the default install strategy
Theme installation supports two modes:
1. **submodule** (default): `git submodule add <repo-url> themes/<theme-name>` — keeps the theme under version control as a submodule.
2. **copy**: shallow clone into a temporary directory, then copy into `themes/<theme-name>` without `.git` — for users who do not want submodules.

Rationale: Submodule is the Hugo community convention and makes theme updates easy (`git submodule update --remote`). Copy mode is a fallback for environments where submodule workflow is undesirable.

### 4. Path validation for project initialization
Project init requires an absolute path that does not already contain a Hugo config file and is not inside the admin application directory. The operation is rejected if the directory is non-empty and contains conflicting files.

Rationale: Prevents accidental data loss and protects the admin installation itself.

### 5. Admin-only access and synchronous API
All init/theme endpoints require the existing admin session. Long-running operations (`hugo new site`, `git submodule add`) run synchronously but with a generous request timeout and progress streamed via the existing WebSocket channel if needed in a future iteration.

Rationale: Keeps the first implementation simple while reusing the existing auth guard.

### 6. Theme preview restarts the dev server with the selected theme
Preview is implemented as a dedicated endpoint that stops the running Hugo server (if any), starts it with `--theme <preview-theme>`, and returns the preview URL. The active theme in settings is not changed unless the user explicitly clicks "Activate".

Rationale: Preview is inherently tied to the dev server lifecycle. Reusing `HugoServerManager.start()`/`stop()` avoids duplicating Hugo process logic and gives the user immediate visual feedback via the existing preview URL.

## Risks / Trade-offs

- **[Risk] Subprocess failures or partial init leaves a dirty directory** → Mitigation: validate the target directory before running `hugo new site`; on failure, return the stderr and do not attempt automatic cleanup unless the directory was created empty.
- **[Risk] Theme repos are large or network is slow** → Mitigation: support `--depth 1` for copy mode; submodule installs can later be improved with shallow options. Document timeout limits.
- **[Risk] Theme activation without a matching config breaks Hugo server** → Mitigation: the server manager already surfaces Hugo logs via WebSocket; if the theme is missing, the user sees the error in real time. Optionally validate that `themes/<name>` exists before activation.
- **[Risk] Theme preview temporarily overrides the active theme and may confuse the user** → Mitigation: the UI clearly distinguishes "Preview" from "Activate"; preview does not persist `theme.name`; stopping and restarting the server reverts to the persisted active theme.
- **[Risk] Writing outside `HUGO_ROOT` violates existing path-traversal protections** → Mitigation: ProjectInitService performs its own validation; it is treated as an administrative setup action rather than a content edit, so the content-directory guard does not apply.

## Migration Plan

No database or file migration is required. Existing installations continue to work:
- If no active theme is configured, `HugoServerManager` falls back to no `--theme` argument (current behavior).
- If `HUGO_THEME` environment variable is set, it takes precedence over the persisted setting to avoid breaking existing Docker deployments.

## Open Questions

1. Should the UI include a curated list of popular themes, or only support arbitrary Git URLs?
2. Should theme update/update-all operations be included in the first iteration?
3. Should project init also scaffold an initial `content/post/hello.md` or leave the site empty?
