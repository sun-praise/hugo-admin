# coding: utf-8
"""
Hugo 站点配置文件读写路由

支持两种配置结构：
- 根目录单文件：hugo.toml / config.toml 等
- config/_default/ 多文件：config.toml + languages.toml + menu.toml + …
"""

import json
import logging
from pathlib import Path

from flask import Blueprint, jsonify, request

logger = logging.getLogger(__name__)

# Hugo 根目录配置文件候选（优先级从高到低）
_ROOT_CANDIDATES = (
    "hugo.toml",
    "hugo.yaml",
    "config.toml",
    "config.yaml",
    "config.json",
)

# config/_default/ 下的主配置文件候选
_DIR_CANDIDATES = (
    "config/_default/config.toml",
    "config/_default/config.yaml",
    "config/_default/config.json",
)

# 扩展名 → 格式
_EXT_FORMAT = {
    ".toml": "toml",
    ".yaml": "yaml",
    ".yml": "yaml",
    ".json": "json",
}


def _detect_config_file(hugo_root: Path) -> tuple[Path | None, str]:
    """扫描站点根目录和 config/_default/，返回 (文件路径, 格式) 或 (None, '')。"""
    for name in _ROOT_CANDIDATES:
        p = hugo_root / name
        if p.is_file():
            ext = p.suffix.lower()
            fmt = _EXT_FORMAT.get(ext, "toml")
            return p, fmt
    for name in _DIR_CANDIDATES:
        p = hugo_root / name
        if p.is_file():
            ext = p.suffix.lower()
            fmt = _EXT_FORMAT.get(ext, "toml")
            return p, fmt
    return None, ""


def _detect_config_mode(hugo_root: Path) -> str:
    """检测配置结构模式：'root' 或 'dir' 或 'none'。"""
    for name in _ROOT_CANDIDATES:
        if (hugo_root / name).is_file():
            return "root"
    config_dir = hugo_root / "config" / "_default"
    if config_dir.is_dir() and any(config_dir.glob("*.toml")):
        return "dir"
    return "none"


def _list_config_files(hugo_root: Path) -> list[dict]:
    """列出所有可编辑的配置文件。"""
    mode = _detect_config_mode(hugo_root)
    files = []
    if mode == "root":
        for name in _ROOT_CANDIDATES:
            p = hugo_root / name
            if p.is_file():
                ext = p.suffix.lower()
                files.append(
                    {
                        "name": name,
                        "path": str(p),
                        "format": _EXT_FORMAT.get(ext, "toml"),
                    }
                )
                break  # 只取优先级最高的一个
    elif mode == "dir":
        config_dir = hugo_root / "config" / "_default"
        for p in sorted(config_dir.iterdir()):
            if p.is_file() and p.suffix.lower() in _EXT_FORMAT:
                ext = p.suffix.lower()
                files.append(
                    {
                        "name": p.name,
                        "path": str(p),
                        "format": _EXT_FORMAT.get(ext, "toml"),
                    }
                )
    return files


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


def _resolve_config_path(hugo_root: Path, filename: str) -> Path | None:
    """根据文件名解析安全路径，防止路径穿越。"""
    # 根目录单文件
    root_file = hugo_root / filename
    if root_file.is_file():
        return root_file
    # config/_default/ 下的文件
    dir_file = hugo_root / "config" / "_default" / filename
    try:
        dir_file.resolve().relative_to((hugo_root / "config" / "_default").resolve())
    except ValueError:
        return None
    if dir_file.is_file():
        return dir_file
    return None


bp = Blueprint("config", __name__)


def register_config_routes(app):
    """注册配置读写路由。"""

    @bp.route("/api/config", methods=["GET"])
    def list_configs():
        """列出所有可编辑的配置文件。"""
        hugo_root = Path(app.config.get("HUGO_ROOT", ""))
        if not hugo_root.is_dir():
            return jsonify({"success": False, "message": "Hugo 项目路径无效"}), 400

        files = _list_config_files(hugo_root)
        if not files:
            return (
                jsonify({"success": False, "message": "未找到 Hugo 配置文件"}),
                404,
            )

        return jsonify(
            {
                "success": True,
                "mode": _detect_config_mode(hugo_root),
                "files": files,
            }
        )

    @bp.route("/api/config/<filename>", methods=["GET"])
    def get_config(filename):
        """读取指定配置文件内容。"""
        hugo_root = Path(app.config.get("HUGO_ROOT", ""))
        if not hugo_root.is_dir():
            return jsonify({"success": False, "message": "Hugo 项目路径无效"}), 400

        config_path = _resolve_config_path(hugo_root, filename)
        if config_path is None:
            return (
                jsonify({"success": False, "message": f"配置文件不存在: {filename}"}),
                404,
            )

        ext = config_path.suffix.lower()
        content = config_path.read_text(encoding="utf-8")
        return jsonify(
            {
                "success": True,
                "filename": config_path.name,
                "format": _EXT_FORMAT.get(ext, "toml"),
                "content": content,
                "path": str(config_path),
            }
        )

    @bp.route("/api/config/<filename>", methods=["PUT"])
    def save_config(filename):
        """写入指定配置文件。"""
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

        # 解析路径：已有文件直接写，新文件默认创建到 config/_default/
        config_path = _resolve_config_path(hugo_root, filename)
        if config_path is None:
            # 新文件：如果站点用 dir 模式，放到 config/_default/；否则根目录
            mode = _detect_config_mode(hugo_root)
            if mode == "dir":
                config_path = hugo_root / "config" / "_default" / filename
            else:
                config_path = hugo_root / filename

        ext = config_path.suffix.lower()
        fmt = _EXT_FORMAT.get(ext, "toml")

        # 语法校验
        error = _validate_content(content, fmt)
        if error:
            return jsonify({"success": False, "message": error}), 400

        config_path.parent.mkdir(parents=True, exist_ok=True)
        config_path.write_text(content, encoding="utf-8")
        logger.info("Hugo 配置已保存: %s", config_path)
        return jsonify(
            {
                "success": True,
                "message": f"配置已保存: {config_path.name}",
                "path": str(config_path),
            }
        )

    return bp
