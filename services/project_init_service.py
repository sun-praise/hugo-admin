# coding: utf-8
"""
项目初始化服务
负责验证目标路径、执行 `hugo new site` 并切换活跃项目。
"""

import subprocess
from pathlib import Path

from models.database import Database
from services.git_service import GitService
from services.hugo_service import HugoServerManager
from services.post_service import PostService
from services.reference_service import ReferenceService
from services.settings_service import (
    SettingsService,
    SettingsStorageError,
    SettingsValidationError,
)


class ProjectInitError(ValueError):
    """项目初始化参数或执行失败"""


class ProjectInitService:
    """Hugo 项目初始化服务"""

    HUGO_CONFIG_FILES = (
        "config.toml",
        "config.yaml",
        "config.json",
        "hugo.toml",
        "hugo.yaml",
    )

    def __init__(self, admin_root: Path | str):
        """
        初始化项目初始化服务。

        Args:
            admin_root: hugo-admin 安装目录的绝对路径，用于防止自覆盖。
        """
        self.admin_root = Path(admin_root).resolve()

    def validate_target_path(self, target_path: Path | str) -> Path:
        """
        校验目标路径是否可用于初始化新 Hugo 站点。

        Args:
            target_path: 用户提供的目录路径。

        Returns:
            解析后的绝对路径。

        Raises:
            ProjectInitError: 路径不合法或已存在 Hugo 站点。
        """
        path = Path(target_path)

        if not path.is_absolute():
            raise ProjectInitError("目标路径必须是绝对路径")

        path = path.resolve()

        # 防止写入 hugo-admin 自身目录
        try:
            path.relative_to(self.admin_root)
        except ValueError:
            pass
        else:
            raise ProjectInitError("目标路径不能位于 hugo-admin 安装目录内")

        # 若目录已存在，检查是否包含 Hugo 配置文件
        if path.exists():
            for config_name in self.HUGO_CONFIG_FILES:
                if (path / config_name).exists():
                    raise ProjectInitError(
                        f"目标路径已包含 Hugo 配置文件: {config_name}"
                    )
            if (path / "config" / "_default" / "config.toml").exists() or (
                path / "config" / "_default" / "config.yaml"
            ).exists():
                raise ProjectInitError("目标路径已包含 Hugo 站点配置")

        return path

    def create_site(self, target_path: Path | str, config_format: str = "toml") -> dict:
        """
        在指定路径创建新的 Hugo 站点。

        Args:
            target_path: 站点目标目录的绝对路径。
            config_format: 配置文件格式，仅支持 "toml" 或 "yaml"。

        Returns:
            {"path": <str>, "config_format": <str>}

        Raises:
            ProjectInitError: 参数非法或 `hugo new site` 执行失败。
        """
        if config_format not in {"toml", "yaml"}:
            raise ProjectInitError("配置文件格式仅支持 toml 或 yaml")

        path = self.validate_target_path(target_path)
        parent = path.parent

        if not parent.exists():
            raise ProjectInitError(f"目标路径的父目录不存在: {parent}")

        try:
            result = subprocess.run(
                ["hugo", "new", "site", str(path)],
                cwd=parent,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                check=False,
            )
        except FileNotFoundError as exc:
            raise ProjectInitError("未找到 hugo 命令，请确保 Hugo 已安装") from exc

        if result.returncode != 0:
            raise ProjectInitError(
                f"创建 Hugo 站点失败 (exit {result.returncode}): {result.stdout.strip()}"
            )

        self._write_default_config(path, config_format)
        self._write_default_layouts(path)

        return {"path": str(path), "config_format": config_format}

    def _write_default_config(self, site_root: Path, config_format: str) -> None:
        """在新站点目录写入默认 Hugo 配置文件。"""
        if config_format == "toml":
            config_file = site_root / "hugo.toml"
            config_content = (
                'baseURL = "https://example.org/"\n'
                'languageCode = "zh-CN"\n'
                'title = "My New Hugo Site"\n'
                'theme = ""\n'
            )
        else:
            config_file = site_root / "hugo.yaml"
            config_content = (
                "baseURL: https://example.org/\n"
                "languageCode: zh-CN\n"
                "title: My New Hugo Site\n"
                'theme: ""\n'
            )

        # hugo new site 默认会生成 hugo.toml；仅当格式不一致时才覆盖
        if not config_file.exists():
            config_file.write_text(config_content, encoding="utf-8")
        else:
            # 用户选择 yaml 时，删除默认的 hugo.toml 并写入 hugo.yaml
            default_toml = site_root / "hugo.toml"
            if default_toml.exists() and config_format == "yaml":
                default_toml.unlink()
            config_file.write_text(config_content, encoding="utf-8")

    @staticmethod
    def _write_default_layouts(site_root: Path) -> None:
        """在新站点目录写入基础 Hugo layouts，保证无主题时也能渲染内容。"""
        layouts_dir = site_root / "layouts"
        default_dir = layouts_dir / "_default"
        default_dir.mkdir(parents=True, exist_ok=True)

        baseof_html = (
            "<!DOCTYPE html>\n"
            '<html lang="zh-CN">\n'
            "<head>\n"
            '  <meta charset="UTF-8">\n'
            '  <meta name="viewport" content="width=device-width, initial-scale=1.0">\n'
            '  <title>{{ block "title" . }}{{ .Site.Title }}{{ end }}</title>\n'
            "</head>\n"
            "<body>\n"
            '  <main>{{ block "main" . }}{{ end }}</main>\n'
            "</body>\n"
            "</html>\n"
        )
        (layouts_dir / "_default" / "baseof.html").write_text(
            baseof_html, encoding="utf-8"
        )

        list_html = (
            '{{ define "title" }}{{ .Title }} - {{ .Site.Title }}{{ end }}\n'
            '{{ define "main" }}\n'
            "  <h1>{{ .Title }}</h1>\n"
            "  <ul>\n"
            "  {{ range .Pages }}\n"
            '    <li><a href="{{ .Permalink }}">{{ .Title }}</a></li>\n'
            "  {{ end }}\n"
            "  </ul>\n"
            "{{ end }}\n"
        )
        (default_dir / "list.html").write_text(list_html, encoding="utf-8")

        single_html = (
            '{{ define "title" }}{{ .Title }} - {{ .Site.Title }}{{ end }}\n'
            '{{ define "main" }}\n'
            "  <article>\n"
            "    <h1>{{ .Title }}</h1>\n"
            "    {{ .Content }}\n"
            "  </article>\n"
            "{{ end }}\n"
        )
        (default_dir / "single.html").write_text(single_html, encoding="utf-8")

        index_html = (
            '{{ define "title" }}{{ .Site.Title }}{{ end }}\n'
            '{{ define "main" }}\n'
            "  <h1>{{ .Site.Title }}</h1>\n"
            "  <ul>\n"
            "  {{ range .Site.RegularPages }}\n"
            '    <li><a href="{{ .Permalink }}">{{ .Title }}</a></li>\n'
            "  {{ end }}\n"
            "  </ul>\n"
            "{{ end }}\n"
        )
        (layouts_dir / "index.html").write_text(index_html, encoding="utf-8")

    @staticmethod
    def switch_active_project(app, registry, new_root: Path | str) -> None:
        """
        将活跃 Hugo 项目切换到新目录，并重新初始化所有依赖服务。

        Args:
            app: Flask 应用实例。
            registry: ServiceRegistry 实例。
            new_root: 新的 Hugo 站点根目录。
        """
        new_root = Path(new_root).resolve()

        # 停止当前运行的 Hugo 服务器，避免旧进程成为孤儿进程
        if registry.hugo_manager and registry.hugo_manager.is_running:
            registry.hugo_manager.stop()

        app.config["HUGO_ROOT"] = new_root
        app.config["CONTENT_DIR"] = new_root / "content"

        new_post_service = PostService(app.config["CONTENT_DIR"], use_cache=True)
        new_ref_service = ReferenceService(
            app.config["CONTENT_DIR"],
            (
                new_post_service.cache_service.db
                if new_post_service.cache_service
                else None
            ),
        )
        new_db_path = Path(app.config["CONTENT_DIR"]) / ".admin" / "cache.db"
        new_db = Database(str(new_db_path))
        new_git_service = GitService(new_root, database=new_db)

        server_url = registry.hugo_manager.server_url
        new_settings_service = SettingsService(
            new_root / ".admin" / "settings.json",
            legacy_settings_file=Path(app.config["CONTENT_DIR"])
            / ".admin"
            / "settings.json",
            defaults={
                "AI_BASE_URL": app.config.get(
                    "AI_BASE_URL", "https://api.deepseek.com"
                ),
                "AI_MODEL": app.config.get("AI_MODEL", "deepseek-chat"),
                "HUGO_BASE_DIR": str(new_root),
                "HUGO_SERVER_URL": server_url
                or app.config.get("HUGO_SERVER_BASE_URL", "http://0.0.0.0:1313"),
            },
        )
        new_hugo_manager = HugoServerManager(
            new_root,
            registry.socketio,
            server_url=server_url,
            settings_service=new_settings_service,
        )

        registry.post_service = new_post_service
        registry.ref_service = new_ref_service
        registry.git_service = new_git_service
        registry.hugo_manager = new_hugo_manager
        registry.database = new_db
        registry.settings_service = new_settings_service
        registry.ai_service = None

        # 初始化新站点的设置文件（创建 .admin/settings.json）
        try:
            new_settings_service.get_settings()
        except (SettingsStorageError, SettingsValidationError, OSError, ValueError):
            pass

        # 扫描新内容目录中的引用关系
        try:
            new_ref_service.scan_all()
        except (OSError, ValueError):
            pass
