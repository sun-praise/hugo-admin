## ADDED Requirements

### Requirement: Admin can install a theme from a Git repository
The system SHALL allow an authenticated admin to install a Hugo theme into `themes/<theme-name>` from a Git repository URL.

#### Scenario: Install theme as Git submodule
- **WHEN** the admin provides a Git URL and selects the submodule install mode
- **THEN** the system runs `git submodule add <url> themes/<theme-name>` inside the current `HUGO_ROOT`
- **AND** the system returns the installed theme name and success status

#### Scenario: Install theme as a copy
- **WHEN** the admin provides a Git URL and selects the copy install mode
- **THEN** the system clones the repository with depth 1 into a temporary directory
- **AND** the system copies the contents into `themes/<theme-name>` excluding the `.git` directory
- **AND** the system returns the installed theme name and success status

#### Scenario: Theme name conflicts with existing directory
- **WHEN** the admin attempts to install a theme whose target directory already exists in `themes/`
- **THEN** the system rejects the request with a clear error message

### Requirement: Admin can list installed themes
The system SHALL allow an authenticated admin to view all themes currently present in the project's `themes/` directory.

#### Scenario: List installed themes
- **WHEN** the admin requests the theme list
- **THEN** the system returns each theme name and whether it is a Git submodule

### Requirement: Admin can set the active theme
The system SHALL allow an authenticated admin to select one installed theme as the active theme for Hugo server and build commands.

#### Scenario: Activate existing theme
- **WHEN** the admin selects a theme name that exists in `themes/`
- **THEN** the system persists the active theme in `.admin/settings.json` under `theme.name`
- **AND** the system returns the updated settings

#### Scenario: Activate missing theme
- **WHEN** the admin selects a theme name that does not exist in `themes/`
- **THEN** the system rejects the request with a clear error message

### Requirement: Admin can preview an installed theme
The system SHALL allow an authenticated admin to preview any installed theme by running the Hugo dev server with that theme without changing the persisted active theme.

#### Scenario: Preview existing theme
- **WHEN** the admin selects an installed theme for preview
- **THEN** the system stops the running Hugo dev server if it is running
- **AND** the system starts the Hugo dev server with `--theme <selected-theme>`
- **AND** the system returns the preview URL
- **AND** the persisted active theme remains unchanged

#### Scenario: Preview missing theme
- **WHEN** the admin selects a theme name that does not exist in `themes/`
- **THEN** the system rejects the request with a clear error message

### Requirement: Hugo server uses the active theme
The system SHALL pass the active theme to the Hugo server command when starting the development server.

#### Scenario: Start server with active theme
- **WHEN** the admin starts the Hugo dev server and an active theme is configured
- **THEN** `HugoServerManager` appends `--theme <active-theme>` to the `hugo server` command

#### Scenario: Environment theme overrides persisted theme
- **WHEN** the `HUGO_THEME` environment variable is set
- **THEN** the system uses the environment variable value and ignores the persisted active theme
