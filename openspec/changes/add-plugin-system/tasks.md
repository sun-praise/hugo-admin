## 1. Protocol & SDK

- [ ] 1.1 Define `proto/plugin.proto` with `PluginService`, `ImageUploader` message types
- [ ] 1.2 Generate Python gRPC stubs (`grpcio-tools`)
- [ ] 1.3 Create Go gRPC SDK skeleton (to be used by `hugo-admin-plugin-cloudflare` repo)
- [ ] 1.4 Define `plugin.toml` manifest schema and write a TOML parser in Python

## 2. Plugin Manager (Python)

- [ ] 2.1 Create `services/plugin_manager.py`: plugin discovery from `~/.hugo-admin/plugins/`
- [ ] 2.2 Implement subprocess lifecycle: start with dynamic port, health-check handshake, graceful shutdown
- [ ] 2.3 Implement gRPC client wrapper: connect to plugin, call Info/HealthCheck/SetConfig
- [ ] 2.4 Implement capability route registration: proxy Flask routes to gRPC services
- [ ] 2.5 Implement plugin configuration persistence in `~/.hugo-admin/plugin-config.json`

## 3. REST API

- [ ] 3.1 Create `routes/plugin_routes.py` with endpoints: list, config-schema, config, enable, disable
- [ ] 3.2 Add market endpoint `GET /api/plugins/market` (fetch remote manifest, cache result)
- [ ] 3.3 Register plugin blueprint in `app.py`
- [ ] 3.4 Expose `plugin_manager` through `ServiceRegistry`

## 4. Frontend

- [ ] 4.1 Create `frontend/src/pages/Plugins.tsx` with Market / Installed tabs
- [ ] 4.2 Create `frontend/src/components/PluginCard.tsx` for plugin display (market & installed)
- [ ] 4.3 Auto-generate config forms from plugin config schema
- [ ] 4.4 Integrate Plugins section into Settings navigation (or standalone page)
- [ ] 4.5 Wire image upload in Editor to use active image plugin when available

## 5. Cloudflare Images Plugin (Go, separate repo)

- [ ] 5.1 Create `hugo-admin-plugin-cloudflare` Go repo
- [ ] 5.2 Implement `PluginService` gRPC server (Info, HealthCheck, GetConfigSchema, SetConfig)
- [ ] 5.3 Implement `ImageUploader` gRPC server (Upload, Delete via Cloudflare Images API)
- [ ] 5.4 Write `plugin.toml` manifest
- [ ] 5.5 Build script: cross-compile for linux-amd64, linux-arm64, darwin-amd64, darwin-arm64
- [ ] 5.6 Package as `.tar.gz` with binary + manifest

## 6. Integration & Testing

- [ ] 6.1 Add `grpcio`, `grpcio-tools` to `pyproject.toml`
- [ ] 6.2 Write unit tests for `plugin_manager.py` (subprocess mock, gRPC mock)
- [ ] 6.3 Write unit tests for `plugin_routes.py` (API endpoints)
- [ ] 6.4 Write integration test: start mock plugin, verify proxy route works end-to-end
- [ ] 6.5 Verify existing image upload still works (no regression)
