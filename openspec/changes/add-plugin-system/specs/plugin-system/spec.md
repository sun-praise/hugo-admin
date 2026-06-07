## ADDED Requirements

### Requirement: Plugin Protocol Definition
The system SHALL define a gRPC-based plugin protocol (`plugin.proto`) that specifies:
- A `PluginService` with `Info`, `HealthCheck`, `GetConfigSchema`, and `SetConfig` RPCs
- An `ImageUploader` service with `Upload` and `Delete` RPCs
- Standard message types for plugin metadata, image data, and configuration

#### Scenario: Plugin binary starts and responds to handshake
- **WHEN** Plugin Manager spawns a plugin binary with `--port <port>`
- **THEN** the plugin SHALL start a gRPC server on the specified port
- **AND** respond to `PluginService.HealthCheck` within 10 seconds

#### Scenario: Plugin declares its capabilities
- **WHEN** Plugin Manager calls `PluginService.Info` on a running plugin
- **THEN** the plugin SHALL return its name, version, description, and list of supported capabilities (e.g., `image_upload`)

### Requirement: Plugin Manifest Format
The system SHALL require each plugin to provide a `plugin.toml` manifest containing:
- Plugin identity (`name`, `version`, `author`, `description`)
- Binary entry point (`entry` path relative to plugin directory)
- Declared capabilities (e.g., `image_upload = true`)
- Configuration schema (JSON Schema describing user-configurable settings)

#### Scenario: Plugin Manager reads manifest
- **WHEN** Plugin Manager scans `~/.hugo-admin/plugins/` directory
- **THEN** it SHALL parse each subdirectory's `plugin.toml`
- **AND** skip directories that do not contain a valid `plugin.toml`

### Requirement: Plugin Manager Lifecycle
The system SHALL provide a `PluginManager` service that:
- Discovers installed plugins from `~/.hugo-admin/plugins/` on startup
- Assigns a dynamic localhost port to each plugin
- Starts plugin binaries as subprocess processes
- Performs gRPC handshake (HealthCheck + Info) within a configurable timeout
- Stops plugin processes gracefully (SIGTERM → wait → SIGKILL) on shutdown
- Persists per-plugin user configuration in `~/.hugo-admin/plugin-config.json`

#### Scenario: All healthy plugins start on hugo-admin boot
- **WHEN** hugo-admin starts and `~/.hugo-admin/plugins/` contains valid plugin directories
- **THEN** Plugin Manager SHALL start each plugin binary
- **AND** register Flask API routes for each discovered capability
- **AND** skip plugins that fail health check, logging a warning

#### Scenario: Plugin process is terminated on shutdown
- **WHEN** hugo-admin shuts down
- **THEN** Plugin Manager SHALL send SIGTERM to all running plugin processes
- **AND** wait up to 5 seconds
- **AND** send SIGKILL to any remaining processes

### Requirement: Plugin Capability Routing
The system SHALL automatically register Flask API routes for each plugin capability:
- `image_upload` capability → `POST /api/plugins/<name>/image/upload` and `DELETE /api/plugins/<name>/image/<image_id>`
- Routes SHALL proxy HTTP requests to the corresponding gRPC service on the plugin process

#### Scenario: User uploads image via plugin
- **WHEN** frontend sends `POST /api/plugins/cloudflare-images/image/upload` with a multipart file
- **THEN** the Flask proxy route SHALL forward the image data via gRPC `ImageUploader.Upload` to the plugin
- **AND** return the plugin's response (including the hosted image URL) to the frontend

### Requirement: Plugin REST API
The system SHALL provide REST endpoints for plugin management:
- `GET /api/plugins` — list installed plugins with status, capabilities, and configuration state
- `GET /api/plugins/<name>/config-schema` — return the plugin's configuration schema
- `PUT /api/plugins/<name>/config` — update plugin configuration
- `POST /api/plugins/<name>/enable` — enable a plugin
- `POST /api/plugins/<name>/disable` — disable a plugin (stop the process, keep configuration)

#### Scenario: User configures Cloudflare plugin
- **WHEN** user sends `PUT /api/plugins/cloudflare-images/config` with `{"api_token": "xxx", "account_id": "yyy"}`
- **THEN** the system SHALL persist the configuration
- **AND** forward it to the running plugin via gRPC `SetConfig`

### Requirement: Plugin Market API
The system SHALL provide an endpoint to browse available plugins:
- `GET /api/plugins/market` — fetch the remote plugin manifest and return the catalog

#### Scenario: User browses plugin market
- **WHEN** frontend requests `GET /api/plugins/market`
- **THEN** the system SHALL fetch the remote manifest JSON from the configured market URL
- **AND** return the list of available plugins with name, version, description, and capabilities

### Requirement: Frontend Plugin Management Page
The system SHALL provide a "Plugins" section in the Settings page that includes:
- A "Market" tab showing available plugins from the remote catalog
- An "Installed" tab showing installed plugins with enable/disable toggles
- Per-plugin configuration forms auto-generated from the plugin's config schema
- Status indicators (running/stopped/error) for each installed plugin

#### Scenario: User installs a plugin from market
- **WHEN** user clicks "Install" on a market plugin card
- **THEN** the system SHALL download the plugin package from the market URL
- **AND** extract it to `~/.hugo-admin/plugins/<name>/`
- **AND** start the plugin if it passes health check

#### Scenario: User views installed plugins
- **WHEN** user navigates to the "Installed" tab
- **THEN** the system SHALL display each installed plugin with its name, version, status, and a "Configure" button

### Requirement: Cloudflare Images Plugin
The Cloudflare Images plugin (separate Go repo) SHALL implement:
- The `PluginService` gRPC interface (Info, HealthCheck, GetConfigSchema, SetConfig)
- The `ImageUploader` gRPC interface (Upload, Delete)
- Upload images to Cloudflare Images via the Cloudflare API using user-provided API Token and Account ID
- Return the public CDN URL of the uploaded image

#### Scenario: Plugin uploads image to Cloudflare
- **WHEN** Plugin Manager proxies an image upload request to the Cloudflare plugin
- **THEN** the plugin SHALL call the Cloudflare Images API with the user's configured API Token
- **AND** return `{ success: true, url: "https://...cloudflare.com/..." }`

#### Scenario: Plugin requires valid configuration
- **WHEN** a user tries to upload without configuring API Token and Account ID
- **THEN** the plugin SHALL return `{ success: false, message: "API Token and Account ID are required" }`
