## ADDED Requirements

### Requirement: Admin can create a new Hugo site
The system SHALL allow an authenticated admin to initialize a new Hugo site at a user-specified absolute path.

#### Scenario: Successful initialization
- **WHEN** the admin provides an absolute target path and a config format (toml or yaml)
- **THEN** the system creates the directory structure by running `hugo new site <path>`
- **AND** the system writes a default `hugo.<format>` configuration file
- **AND** the system returns the path and success status

#### Scenario: Target path already contains a Hugo site
- **WHEN** the admin provides a path that already contains a Hugo config file (`config.toml`, `config.yaml`, `hugo.toml`, or `hugo.yaml`)
- **THEN** the system rejects the request with a clear error message

#### Scenario: Target path is inside the admin application directory
- **WHEN** the admin provides a path that resolves inside the hugo-admin installation directory
- **THEN** the system rejects the request to prevent self-overwrite

### Requirement: Newly initialized site becomes the active project
The system SHALL update the active Hugo project to point to the newly initialized site after successful creation.

#### Scenario: Active project updates after init
- **WHEN** a new Hugo site is successfully initialized
- **THEN** the system sets `HUGO_ROOT`, `CONTENT_DIR`, and persisted `hugo.base_dir` to the new site path
- **AND** the system reinitializes `PostService`, `GitService`, and `HugoServerManager` for the new path
