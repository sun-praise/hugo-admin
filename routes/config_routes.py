# coding: utf-8
"""
Hugo 站点配置文件读写路由
"""

import json
import logging
from pathlib import Path

from flask import Blueprint, jsonify, request

logger = logging.getLogger(__name__)

# Hugo 配置文件优先级（与 Hugo 自身一致）
_CONFIG_CANDIDATES = (
    "hugo.toml",
    "hugo.yaml",
    "config.toml",
    "config.yaml",
    "config.json",
)

# 扩展名 → 格式
_EXT_FORMAT = {
    ".toml": "toml",
    ".yaml": "yaml",
    ".yml": "yaml",
    ".json": "json",
}


def _detect_config_file(hugo_root: Path) -> tuple[Path | None, str]:
    """扫描站点根目录，返回 (文件路径, 格式) 或 (None, '')。"""
    for name in _CONFIG_CANDIDATES:
        p = hugo_root / name
        if p.is_file():
            ext = p.suffix.lower()
            fmt = _EXT_FORMAT.get(ext, "toml")
            return p, fmt
    return None, ""


def _validate_content(content: str, fmt: str) -> str | None:
    """校验配置文本的语法，返回错误信息或 None（合法）。"""
    if fmt == "toml":
        try:
            import tomllib

            tomllib.loads(content)
        except Exception as exc:
            return f"TOML 语法错误: {exc}"
    elif fmt in ("yaml", "yml"):
        try:
            import yaml

            yaml.safe_load(content)
        except Exception as exc:
            return f"YAML 语法错误: {exc}"
    elif fmt == "json":
        try:
            json.loads(content)
        except Exception as exc:
            return f"JSON 语法错误: {exc}"
    else:
        return f"不支持的配置格式: {fmt}"
    return None


bp = Blueprint("config", __name__)


def register_config_routes(app):
    """注册配置读写路由。"""

    @bp.route("/api/config", methods=["GET"])
    def get_config():
        """读取当前 Hugo 站点配置文件。"""
        hugo_root = Path(app.config.get("HUGO_ROOT", ""))
        if not hugo_root.is_dir():
            return jsonify({"success": False, "message": "Hugo 项目路径无效"}), 400

        config_path, fmt = _detect_config_file(hugo_root)
        if config_path is None:
            return (
                jsonify({"success": False, "message": "未找到 Hugo 配置文件"}),
                404,
            )

        content = config_path.read_text(encoding="utf-8")
        return jsonify(
            {
                "success": True,
                "format": fmt,
                "content": content,
                "path": str(config_path),
            }
        )

    @bp.route("/api/config", methods=["PUT"])
    def save_config():
        """写入 Hugo 站点配置文件。"""
        hugo_root = Path(app.config.get("HUGO_ROOT", ""))
        if not hugo_root.is_dir():
            return jsonify({"success": False, "message": "Hugo 项目路径无效"}), 400

        data = request.get_json(silent=True) or {}
        content = data.get("content", "")
        if not content:
            return (
                jsonify({"success": False, "message": "配置内容不能为空"}),
                400,
            )

        # 检测现有配置文件；不存在则按请求格式或默认 TOML 创建
        config_path, fmt = _detect_config_file(hugo_root)
        if config_path is None:
            fmt = data.get("format", "toml")
            config_path = hugo_root / f"hugo.{fmt}"

        # 语法校验
        error = _validate_content(content, fmt)
        if error:
            return jsonify({"success": False, "message": error}), 400

        config_path.write_text(content, encoding="utf-8")
        logger.info("Hugo 配置已保存: %s", config_path)
        return jsonify(
            {
                "success": True,
                "message": "配置已保存",
                "path": str(config_path),
            }
        )

    return bp
