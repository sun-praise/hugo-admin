# coding: utf-8
"""
Plugin management REST API routes.
"""
import logging

from flask import Blueprint, jsonify, request

from proto import plugin_pb2
from services.plugin_manager import PluginManager

logger = logging.getLogger(__name__)

bp = Blueprint("plugins", __name__)


def register_plugin_routes(plugin_manager: PluginManager):
    """Register all plugin REST endpoints and capability proxy routes."""

    # ---- Plugin Management ----

    @bp.route("/api/plugins", methods=["GET"])
    def list_plugins():
        """List all installed plugins with status."""
        plugins = plugin_manager.list_plugins()
        return jsonify({"success": True, "plugins": plugins})

    @bp.route("/api/plugins/<name>/config-schema", methods=["GET"])
    def get_config_schema(name):
        """Return the plugin's configuration schema."""
        schema = plugin_manager.get_config_schema(name)
        if not schema:
            return jsonify({"success": False, "message": "Plugin not found"}), 404
        return jsonify({"success": True, "schema": schema})

    @bp.route("/api/plugins/<name>/config", methods=["GET"])
    def get_config(name):
        """Get current plugin configuration (decrypted)."""
        state = plugin_manager.get_plugin(name)
        if state is None:
            return jsonify({"success": False, "message": "Plugin not found"}), 404

        config = plugin_manager.get_plugin_config(name)
        return jsonify({"success": True, "config": config})

    @bp.route("/api/plugins/<name>/config", methods=["PUT"])
    def set_config(name):
        """Update plugin configuration."""
        state = plugin_manager.get_plugin(name)
        if state is None:
            return jsonify({"success": False, "message": "Plugin not found"}), 404

        config = request.get_json(silent=True) or {}
        success = plugin_manager.set_plugin_config(name, config)
        if success:
            return jsonify({"success": True, "message": "Configuration saved"})
        else:
            return (
                jsonify(
                    {
                        "success": False,
                        "message": "Configuration saved but failed to push to plugin",
                    }
                ),
                207,
            )

    @bp.route("/api/plugins/<name>/enable", methods=["POST"])
    def enable_plugin(name):
        """Enable a stopped plugin."""
        success = plugin_manager.enable_plugin(name)
        if success:
            return jsonify({"success": True, "message": f"Plugin {name} enabled"})
        return jsonify({"success": False, "message": f"Failed to enable {name}"}), 500

    @bp.route("/api/plugins/<name>/disable", methods=["POST"])
    def disable_plugin(name):
        """Disable a running plugin (stop process, keep config)."""
        success = plugin_manager.disable_plugin(name)
        if success:
            return jsonify({"success": True, "message": f"Plugin {name} disabled"})
        return jsonify({"success": False, "message": f"Plugin {name} not found"}), 404

    # ---- Market ----

    @bp.route("/api/plugins/market", methods=["GET"])
    def get_market():
        """Fetch and return the remote plugin market catalog."""
        catalog = plugin_manager.fetch_market()
        return jsonify({"success": True, **catalog})

    # ---- Capability Proxy: Image Upload ----

    @bp.route("/api/plugins/<name>/image/upload", methods=["POST"])
    def upload_image_via_plugin(name):
        """Proxy image upload to a plugin's ImageUploader gRPC service."""
        if "file" not in request.files:
            return jsonify({"success": False, "message": "No file provided"}), 400

        file = request.files["file"]
        article_path = request.form.get("article_path", "")

        if file.filename == "":
            return jsonify({"success": False, "message": "Empty filename"}), 400

        stub = plugin_manager.get_image_uploader_stub(name)
        if stub is None:
            return (
                jsonify(
                    {
                        "success": False,
                        "message": (
                            f"Plugin {name} not running"
                            " or does not support image_upload"
                        ),
                    }
                ),
                404,
            )

        filename = file.filename or "upload"
        mime_type = file.mimetype or "application/octet-stream"

        # Stream file in 64 KiB chunks via gRPC client-streaming
        chunk_size = 64 * 1024

        def _chunk_generator():
            first = True
            while True:
                data = file.read(chunk_size)
                if not data:
                    break
                chunk = plugin_pb2.ImageUploadChunk(
                    data=data,
                    filename=filename if first else "",
                    mime_type=mime_type if first else "",
                    article_path=article_path if first else "",
                    is_last=False,
                )
                first = False
                yield chunk
            # Send final empty chunk with is_last=True
            yield plugin_pb2.ImageUploadChunk(is_last=True)

        try:
            resp = stub.Upload(_chunk_generator(), timeout=60)
            return jsonify(
                {
                    "success": resp.success,
                    "url": resp.url,
                    "image_id": resp.image_id,
                    "message": resp.message,
                }
            )
        except Exception as e:
            logger.error("Image upload via plugin %s failed: %s", name, e)
            return jsonify({"success": False, "message": f"Upload failed: {e}"}), 500

    @bp.route("/api/plugins/<name>/image/<image_id>", methods=["DELETE"])
    def delete_image_via_plugin(name, image_id):
        """Proxy image deletion to a plugin's ImageUploader gRPC service."""
        stub = plugin_manager.get_image_uploader_stub(name)
        if stub is None:
            return (
                jsonify({"success": False, "message": f"Plugin {name} not running"}),
                404,
            )

        try:
            resp = stub.Delete(
                plugin_pb2.ImageDeleteRequest(image_id=image_id), timeout=30
            )
            return jsonify({"success": resp.success, "message": resp.message})
        except Exception as e:
            logger.error("Image delete via plugin %s failed: %s", name, e)
            return jsonify({"success": False, "message": f"Delete failed: {e}"}), 500

    return bp
