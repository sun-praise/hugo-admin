## 1. Backend: Project Initialization Service

- [x] 1.1 Create `services/project_init_service.py` with `ProjectInitService`
- [x] 1.2 Implement target path validation (absolute path, not inside admin dir, no existing Hugo config)
- [x] 1.3 Implement `hugo new site <path>` execution and error handling
- [x] 1.4 Implement default `hugo.toml` / `hugo.yaml` generation based on selected format
- [x] 1.5 Implement active project switch logic (reinitialize registry services for new `HUGO_ROOT`)

## 2. Backend: Theme Management Service

- [x] 2.1 Create `services/theme_service.py` with `ThemeService`
- [x] 2.2 Implement installed theme discovery in `themes/` with submodule detection
- [x] 2.3 Implement Git submodule install mode (`git submodule add <url> themes/<name>`)
- [x] 2.4 Implement copy install mode (shallow clone to temp dir, copy without `.git`)
- [x] 2.5 Implement active theme persistence in `.admin/settings.json` under `theme.name`
- [x] 2.6 Implement validation to prevent overwriting existing theme directories

## 3. Backend: Routes and Server Integration

- [x] 3.1 Create `routes/project_init_routes.py` with endpoints for creating a site
- [x] 3.2 Create `routes/theme_routes.py` with endpoints for list/install/activate/preview themes
- [x] 3.3 Register new blueprints in `app.py` behind existing auth guard
- [x] 3.4 Update `HugoServerManager.start()` to read active theme from settings and append `--theme <name>`
- [x] 3.5 Ensure `HUGO_THEME` environment variable still overrides persisted active theme
- [x] 3.6 Update `SettingsService` to support the `theme` section in defaults and updates
- [x] 3.7 Add preview endpoint that stops/starts `HugoServerManager` with the selected theme without persisting it

## 4. Frontend

- [x] 4.1 Add "Initialize Project" section to the settings or dashboard page
- [x] 4.2 Add "Themes" section to manage installed and active themes
- [x] 4.3 Implement API client functions for project init and theme endpoints
- [x] 4.4 Display installed themes list with active theme indicator and install/activate/preview actions
- [x] 4.5 Add form for installing a theme from a Git URL with install-mode selection
- [x] 4.6 Show preview URL and distinguish preview state from active theme state

## 5. Tests and Quality Assurance

- [x] 5.1 Add unit tests for `ProjectInitService` path validation using temporary directories
- [x] 5.2 Add unit tests for `ThemeService` discovery and active-theme logic
- [x] 5.3 Add API tests for project init and theme endpoints (authenticated and unauthenticated)
- [x] 5.4 Add tests verifying `HugoServerManager` command includes `--theme` when active theme is set
- [x] 5.5 Add tests for theme preview endpoint (server restart, no persistence, missing theme error)
- [x] 5.6 Run the full test suite (`pytest`) and fix any regressions
