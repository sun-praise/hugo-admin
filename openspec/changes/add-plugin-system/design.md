## Context

hugo-admin is a Python/Flask + React SPA for managing Hugo blogs. It currently handles image uploads by saving files locally to the Hugo content directory. Users want to extend this with external hosting (Cloudflare Images, S3, etc.) and potentially other capabilities, without modifying core code.

Key constraints:
- Plugin code MUST be closed-source (compiled Go binaries, not Python)
- Plugins communicate with the Python host over localhost (no network exposure)
- The system is self-hosted by users; no central authentication
- First plugin is Cloudflare Images (separate repo `hugo-admin-plugin-cloudflare`)

## Goals / Non-Goals

### Goals
- Define a stable gRPC protocol that any plugin MUST implement
- Build a Plugin Manager that handles plugin lifecycle (discover, start, stop, health-check)
- Provide REST API routes that proxy requests to the correct plugin based on capability
- Create a frontend UI for plugin management and configuration
- Implement the first Cloudflare Images plugin as proof-of-concept

### Non-Goals
- Payment/licensing system (defer to future)
- Plugin auto-update mechanism (defer)
- Sandboxed execution (plugins are trusted binaries; security is the admin's responsibility)
- Multiple concurrent Hugo instances per plugin

## Decisions

### Decision 1: gRPC over HTTP

**Choice**: gRPC with protobuf-defined service contracts.

**Why**:
- Strong typing — both Go and Python get auto-generated client/server stubs from `.proto`
- Binary protocol — efficient for image upload (streaming support)
- Code generation eliminates serialization bugs between Python host and Go plugin
- Future-proof — adding new capabilities means adding new service definitions, not inventing new JSON schemas

**Alternatives**:
- HTTP JSON: simpler, but no code generation, manual schema negotiation, harder to evolve
- stdin/stdout pipe: not bidirectional, poor for streaming, single-connection bottleneck

### Decision 2: Plugin as a subprocess with gRPC server

**Choice**: Each plugin runs as an independent OS process, listening on a dynamic localhost port.

**Why**:
- Process isolation — a crashing plugin does not crash hugo-admin
- Language agnostic — any language with gRPC support can be a plugin (Go chosen for IP protection)
- Dynamic port allocation avoids conflicts when multiple plugins run simultaneously
- Plugin Manager assigns ports at startup; plugin reports readiness via gRPC handshake

**Plugin startup flow**:
1. Plugin Manager reads `plugin.toml` from plugin directory
2. Spawns plugin binary with `--port <assigned_port>` argument
3. Waits for gRPC `HealthCheck` to succeed (with timeout)
4. Calls `PluginService.Info()` to discover capabilities
5. Registers Flask proxy routes for discovered capabilities

### Decision 3: Plugin manifest format (plugin.toml)

```toml
[plugin]
name = "cloudflare-images"
version = "1.0.0"
author = "svtter"
description = "Upload images to Cloudflare Images"
entry = "./bin/cloudflare-images-plugin"

[capabilities]
image_upload = true

[config_schema]
# JSON Schema describing user-configurable settings
schema = '''
{
  "type": "object",
  "properties": {
    "api_token": { "type": "string", "label": "API Token" },
    "account_id": { "type": "string", "label": "Account ID" }
  },
  "required": ["api_token", "account_id"]
}
'''
```

**Why TOML**: Human-readable, standard in Go ecosystem (BurntSushi/toml), supports nested structures.

### Decision 4: Capability-based routing

Each plugin declares capabilities in its manifest. The Plugin Manager creates Flask routes scoped to the plugin:

| Capability | Route pattern | gRPC service |
|---|---|---|
| `image_upload` | `POST /api/plugins/<name>/image/upload` | `ImageUploader.Upload` |
| `image_upload` | `DELETE /api/plugins/<name>/image/<id>` | `ImageUploader.Delete` |

When multiple plugins provide the same capability, the user selects the active one in Settings.

### Decision 5: Plugin storage layout

```
~/.hugo-admin/
  plugins/
    cloudflare-images/
      plugin.toml
      bin/
        cloudflare-images-plugin   # compiled Go binary
    another-plugin/
      plugin.toml
      bin/
        another-plugin
  plugin-config.json               # persisted user config per plugin
```

**Why `~/.hugo-admin/`**: Separate from the project directory — plugins are user-level, not per-project. Same plugins work across multiple hugo-admin instances.

### Decision 6: gRPC Protocol Definition

```protobuf
syntax = "proto3";
package hugo_admin.plugin;

// Core service every plugin MUST implement
service PluginService {
  rpc Info(Empty) returns (PluginInfo);
  rpc HealthCheck(Empty) returns (HealthResponse);
  rpc GetConfigSchema(Empty) returns (ConfigSchemaResponse);
  rpc SetConfig(SetConfigRequest) returns (SetConfigResponse);
}

// Optional capability: Image Upload
service ImageUploader {
  rpc Upload(ImageUploadRequest) returns (ImageUploadResponse);
  rpc Delete(ImageDeleteRequest) returns (ImageDeleteResponse);
}
```

### Decision 7: Market manifest

A simple JSON file hosted at a configurable URL:

```json
{
  "version": 1,
  "plugins": [
    {
      "name": "cloudflare-images",
      "version": "1.0.0",
      "description": "Upload images to Cloudflare Images",
      "author": "svtter",
      "download_url": "https://releases.example.com/plugins/cloudflare-images/v1.0.0.tar.gz",
      "sha256": "abc123...",
      "capabilities": ["image_upload"],
      "config_schema": { ... }
    }
  ]
}
```

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Plugin process hangs on shutdown | Plugin Manager sends SIGTERM, waits 5s, then SIGKILL |
| Plugin binary incompatible with host OS/arch | Manifest includes `platform` and `arch` fields; Plugin Manager checks before install |
| gRPC port conflict | Dynamic port assignment via OS; plugin accepts `--port` flag |
| Malicious plugin | Plugins run as the same user as hugo-admin — admin trusts installed plugins. Future: code signing |
| Plugin breaks on hugo-admin upgrade | Protocol versioning in `PluginInfo`; Manager checks compatibility |

## Migration Plan

1. Add `grpcio` / `grpcio-tools` to `pyproject.toml` dependencies
2. Add `proto/` directory with `.proto` files
3. Add plugin infrastructure (`plugin_manager.py`, `plugin_routes.py`)
4. Add frontend plugin management page
5. Existing image upload flow remains unchanged — plugin upload is an additional option, not a replacement
6. No breaking changes to existing APIs

## Open Questions

- Should the plugin market URL be configurable in Settings, or hardcoded?
- Should we support plugin hot-reload (install without restart), or require hugo-admin restart?
- For the Cloudflare plugin repo — should it be public (marketing) or private?
