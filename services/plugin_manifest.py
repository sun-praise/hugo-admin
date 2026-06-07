# coding: utf-8
"""
Plugin manifest parser — reads and validates plugin.toml files.

Each plugin directory under ~/.hugo-admin/plugins/ MUST contain a
plugin.toml with the following required fields:
  [plugin] section: name, version, entry
  [capabilities] section: at least one capability

Optional fields: author, description, protocol_version, priority,
[build] section with platform/arch, [config_schema] section.
"""

import logging
import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

try:
    import tomllib  # Python 3.11+
except ModuleNotFoundError:
    import tomli as tomllib  # type: ignore[no-redef]

logger = logging.getLogger(__name__)

# Required keys in [plugin] section
_REQUIRED_PLUGIN_KEYS = {"name", "version", "entry"}

# Allowed top-level sections
_ALLOWED_SECTIONS = {"plugin", "capabilities", "build", "config_schema"}


@dataclass
class PluginManifest:
    """Parsed and validated plugin manifest."""

    # [plugin]
    name: str
    version: str
    entry: str  # relative path from plugin directory
    author: str = ""
    description: str = ""
    protocol_version: str = "1"
    priority: int = 0

    # [build]
    platform: str = ""
    arch: str = ""

    # [capabilities]
    capabilities: list[str] = field(default_factory=list)

    # [config_schema]
    config_schema: dict[str, Any] = field(default_factory=dict)

    # Derived at parse time
    plugin_dir: Path = field(default_factory=lambda: Path("."), repr=False)


class ManifestError(Exception):
    """Raised when a plugin.toml is invalid or missing required fields."""


def parse_manifest(plugin_dir: Path) -> PluginManifest:
    """Parse and validate a plugin.toml in the given directory.

    Args:
        plugin_dir: Absolute path to the plugin directory.

    Returns:
        PluginManifest with validated fields.

    Raises:
        ManifestError: If the file is missing, malformed, or lacks required fields.
    """
    toml_path = plugin_dir / "plugin.toml"

    if not toml_path.is_file():
        raise ManifestError(f"No plugin.toml found in {plugin_dir}")

    try:
        raw = tomllib.loads(toml_path.read_text(encoding="utf-8"))
    except Exception as e:
        raise ManifestError(f"Malformed TOML in {toml_path}: {e}") from e

    # Validate [plugin] section
    plugin_section = raw.get("plugin")
    if not isinstance(plugin_section, dict):
        raise ManifestError(f"Missing [plugin] section in {toml_path}")

    missing = _REQUIRED_PLUGIN_KEYS - set(plugin_section.keys())
    if missing:
        raise ManifestError(
            f"Missing required fields in [plugin]: {', '.join(sorted(missing))} "
            f"in {toml_path}"
        )

    # Validate entry path safety (Decision 9)
    entry_raw = plugin_section["entry"]
    entry_resolved = (plugin_dir / entry_raw).resolve()
    plugin_dir_resolved = plugin_dir.resolve()
    try:
        entry_resolved.relative_to(plugin_dir_resolved)
    except ValueError:
        raise ManifestError(
            f"Entry path '{entry_raw}' escapes plugin directory in {toml_path}"
        )

    # Parse capabilities
    caps_section = raw.get("capabilities", {})
    capabilities = [k for k, v in caps_section.items() if v is True]

    if not capabilities:
        raise ManifestError(f"No capabilities declared in {toml_path}")

    # Parse build info
    build_section = raw.get("build", {})

    # Parse config schema
    config_schema: dict[str, Any] = {}
    cs_section = raw.get("config_schema", {})
    schema_str = cs_section.get("schema", "")
    if schema_str:
        import json

        try:
            config_schema = json.loads(schema_str)
        except json.JSONDecodeError as e:
            raise ManifestError(
                f"Invalid JSON in config_schema.schema in {toml_path}: {e}"
            ) from e

    return PluginManifest(
        name=plugin_section["name"],
        version=plugin_section["version"],
        entry=entry_raw,
        author=plugin_section.get("author", ""),
        description=plugin_section.get("description", ""),
        protocol_version=plugin_section.get("protocol_version", "1"),
        priority=plugin_section.get("priority", 0),
        platform=build_section.get("platform", ""),
        arch=build_section.get("arch", ""),
        capabilities=capabilities,
        config_schema=config_schema,
        plugin_dir=plugin_dir_resolved,
    )


def resolve_entry_path(manifest: PluginManifest) -> Path:
    """Resolve and validate the plugin binary entry path.

    Implements Decision 9: resolve realpath, dereference symlinks,
    verify path stays within plugin directory, verify executable bit.

    Returns:
        Absolute path to the plugin binary.

    Raises:
        ManifestError: If the path is invalid or not executable.
    """
    entry_path = (manifest.plugin_dir / manifest.entry).resolve()

    # Dereference symlinks
    entry_path = Path(os.path.realpath(entry_path))

    # Verify stays within plugin dir
    plugin_dir_real = Path(os.path.realpath(manifest.plugin_dir))
    try:
        entry_path.relative_to(plugin_dir_real)
    except ValueError:
        raise ManifestError(
            f"Entry '{manifest.entry}' resolves outside plugin directory "
            f"after symlink dereference"
        )

    # Verify exists and is executable
    if not entry_path.is_file():
        raise ManifestError(f"Entry binary not found: {entry_path}")

    if not os.access(entry_path, os.X_OK):
        raise ManifestError(f"Entry binary is not executable: {entry_path}")

    return entry_path
