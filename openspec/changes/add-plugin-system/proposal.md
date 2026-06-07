# Change: Add Plugin System with gRPC-based Extension Architecture

## Why

hugo-admin needs a plugin mechanism to support private, closed-source extensions (e.g., Cloudflare Images hosting) without modifying the core codebase. Plugins are distributed as pre-compiled Go binaries, communicating with the Python host via gRPC. This protects plugin source code while enabling a marketplace-driven ecosystem.

## What Changes

- **Plugin Protocol**: Define a gRPC service contract (`PluginService`) that all plugins MUST implement. Includes metadata discovery, health checks, and typed capability interfaces (e.g., `ImageUploader`).
- **Plugin Manager**: A Python service (`services/plugin_manager.py`) that discovers installed plugins, starts/stops plugin processes, manages gRPC connections, and registers proxy API routes into Flask.
- **Plugin Registry API**: REST endpoints for listing installed plugins, configuring plugin settings, and managing plugin lifecycle (install/enable/disable/uninstall).
- **Plugin Market API**: REST endpoint serving a curated plugin catalog from a remote manifest JSON, supporting browsing and download.
- **Frontend Plugin UI**: New "Plugins" page in Settings for browsing the market, managing installed plugins, and per-plugin configuration forms (auto-generated from plugin-declared config schema).
- **Cloudflare Images Plugin**: First reference plugin (separate Go repo `hugo-admin-plugin-cloudflare`) implementing `ImageUploader` capability. Users provide API Token + Account ID; plugin uploads images to Cloudflare and returns public URLs.

## Impact

- Affected specs: `ai-tools-integration` (image upload flow gains plugin-aware routing), new spec `plugin-system`
- Affected code:
  - New: `services/plugin_manager.py`, `services/plugin_service.py`, `routes/plugin_routes.py`, `proto/plugin.proto`
  - New: `frontend/src/pages/Plugins.tsx`, `frontend/src/components/PluginCard.tsx`
  - New: `tests/test_plugin_manager.py`, `tests/test_plugin_routes.py`, `tests/test_plugin_integration.py`
  - Modified: `app.py` (register plugin routes, init plugin manager), `services/registry.py` (expose plugin manager)
  - External: new Go repo `hugo-admin-plugin-cloudflare` (independent repository)
- New dependencies: `grpcio`, `grpcio-tools`, `cryptography`, `minisign` (Python); `google.golang.org/grpc` (Go plugins)
