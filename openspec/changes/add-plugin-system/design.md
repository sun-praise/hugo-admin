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
protocol_version = "1"
priority = 100

[build]
platform = "linux"       # target platform: linux, darwin, windows
arch = "amd64"           # target architecture: amd64, arm64

[capabilities]
image_upload = true

[config_schema]
# JSON Schema describing user-configurable settings
# Recursion depth is limited to 5 levels; no $ref/allOf/oneOf allowed
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

// Optional capability: Image Upload (client-streaming for large files)
service ImageUploader {
  rpc Upload(stream ImageUploadChunk) returns (ImageUploadResponse);
  rpc Delete(ImageDeleteRequest) returns (ImageDeleteResponse);
}

message ImageUploadChunk {
  bytes data = 1;         // file content chunk (64 KiB)
  string filename = 2;    // original filename
  string mime_type = 3;   // e.g. "image/png"
  string article_path = 4;// target article path for context
  bool is_last = 5;       // true on the final chunk
}
```

### Decision 7: Market manifest

A JSON file hosted at a hardcoded HTTPS URL. Third-party distribution is out of scope for v1 — the market is curated and operated by the hugo-admin maintainer.

**Security constraints**:
- Market URL MUST use HTTPS; SSRF prevention blocks non-HTTPS and private/internal IP ranges (RFC 1918, link-local)
- Each plugin package MUST be accompanied by a Minisign signature (`.tar.gz.minisig`)
- On install: verify SHA256 digest of the downloaded archive, then verify the Minisign signature against the embedded public key
- Market manifest cache: 5-minute TTL, stored alongside its SHA256; stale cache rejected if digest mismatches

```json
{
  "version": 1,
  "public_key": "RW... (minisign public key)",
  "plugins": [
    {
      "name": "cloudflare-images",
      "version": "1.0.0",
      "description": "Upload images to Cloudflare Images",
      "author": "svtter",
      "download_url": "https://releases.example.com/plugins/cloudflare-images/v1.0.0.tar.gz",
      "signature_url": "https://releases.example.com/plugins/cloudflare-images/v1.0.0.tar.gz.minisig",
      "sha256": "abc123...",
      "platform": "linux",
      "arch": "amd64",
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
| Plugin config credential leak | Fernet symmetric encryption; key derived from a machine-specific secret stored in `~/.hugo-admin/.secret_key` |
| Path traversal in plugin entry | Entry resolved to absolute path and validated to be within the plugin directory; symlinks dereferenced |
| Man-in-the-middle on plugin download | Minisign signature verification on every install; SHA256 digest double-check |
| gRPC 4 MiB message size limit | `ImageUploader.Upload` uses client-streaming RPC; Flask reads file in 64 KiB chunks, streams to plugin. Accepted constraint: max upload 16 MB (Flask `MAX_CONTENT_LENGTH`) |

### Decision 8: Plugin config encryption

**Choice**: Fernet symmetric encryption for sensitive plugin configuration values.

**Why**:
- API tokens, account IDs, and other credentials MUST NOT be stored in plaintext
- Fernet (via `cryptography` package) provides authenticated encryption — tampering is detectable
- Key derivation: on first use, generate a random 256-bit key and store it in `~/.hugo-admin/.secret_key` (file permissions `0600`)
- Non-sensitive config values (e.g., preferred theme) remain plaintext for readability

**Implementation**: The Plugin Manager encrypts values marked `"sensitive": true` in the plugin's `config_schema` before writing to `plugin-config.json`. On read, encrypted values are transparently decrypted before being forwarded to the plugin via gRPC `SetConfig`.

### Decision 9: Plugin binary path validation

**Choice**: Resolve and sandbox the `entry` path before passing to `subprocess.Popen`.

**Implementation**:
1. Join the plugin directory absolute path with the relative `entry` from `plugin.toml`
2. Call `os.path.realpath()` to dereference any symlinks
3. Verify the resolved path starts with the plugin directory (no `../` traversal)
4. Verify the file exists and has executable bit set
5. Only then pass the resolved absolute path to `subprocess.Popen`

### Decision 10: Plugin package signature verification

**Choice**: Minisign for plugin package integrity and authenticity.

**Why**:
- Minisign is lightweight, well-audited, and widely used in the Go ecosystem
- The market manifest embeds a trusted public key; each plugin archive ships with a `.tar.gz.minisign` signature file
- On install: download archive + signature, verify SHA256, then verify Minisign signature against the trusted public key
- A compromised mirror cannot serve malicious plugins without the signer's private key

## Migration Plan

1. Add `grpcio`, `grpcio-tools`, `cryptography`, `minisign` to `pyproject.toml` dependencies
2. Add `proto/` directory with `.proto` files
3. Add plugin infrastructure (`plugin_manager.py`, `plugin_routes.py`)
4. Add frontend plugin management page
5. Existing image upload flow remains unchanged — plugin upload is an additional option (parallel, not replacement)
6. No breaking changes to existing APIs

## Resolved Questions

- **Market URL**: Hardcoded to `https://plugins.hugo-admin.dev/manifest.json` (HTTPS-only, no user configuration in v1)
- **Hot-reload**: Not supported in v1. Installing a new plugin requires hugo-admin restart. This avoids runtime complexity (route registration/de-registration, in-process gRPC channel teardown)
- **Cloudflare plugin repo**: Public repository (marketing value); the Go source is readable but the distributed artifact is the compiled binary from CI releases
