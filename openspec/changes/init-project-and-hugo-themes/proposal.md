## Why

Currently, hugo-admin assumes the target Hugo site already exists and is configured on disk before the application starts. Users must manually run `hugo new site` and install a theme via shell commands before they can use the admin UI. This raises the onboarding barrier and makes it harder to spin up new blogs or switch themes from within the tool. We want hugo-admin to support initializing a fresh Hugo project and configuring different Hugo themes directly from the web interface.

## What Changes

- Add a **project initialization** capability that creates a new Hugo site directory from the admin UI.
  - Validates the target directory (must not already be a Hugo site or overwrite existing content).
  - Runs `hugo new site <path>` on behalf of the user.
  - Creates a default `hugo.toml` / `hugo.yaml` configuration matching the chosen format.
- Add **Hugo theme management** to discover, install, switch, and preview themes.
  - Support installing themes as Git submodules or plain directory copies into `themes/`.
  - Persist the active theme in `.admin/settings.json` and pass `--theme` to `hugo server`.
  - Allow switching the active theme without reinstalling it.
  - Allow previewing any installed theme by restarting the Hugo dev server with that theme and returning the preview URL.
- Extend the settings UI with project init and theme configuration sections.
- Add backend services, routes, and frontend pages for the new capabilities.
- Add tests covering validation, subprocess calls, and theme persistence.

## Capabilities

### New Capabilities

- `project-initialization`: Create a brand-new Hugo site from the admin UI, including default config and an initial content directory.
- `hugo-theme-management`: Install Hugo themes (Git submodule or copy), set the active theme, and preview any installed theme.

### Modified Capabilities

- (none)

## Impact

- **Backend**: new `ProjectInitService`, `ThemeService`, and corresponding routes; updates to `HugoServerManager` to read the active theme from settings and apply `--theme`; a preview endpoint that restarts the dev server with the selected theme.
- **Frontend**: new or extended settings pages for project init and theme selection.
- **Persistence**: `.admin/settings.json` gains a `theme` section; existing settings remain compatible.
- **Security**: project init performs filesystem writes outside the configured `HUGO_ROOT`; strict path validation and admin-only access are required.
- **Dependencies**: relies on `hugo` CLI and `git` already being available in the environment.
