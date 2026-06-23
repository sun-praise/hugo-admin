# coding: utf-8
"""
主题管理服务
负责发现、安装、激活和预览 Hugo 主题。
"""

import shutil
import subprocess
import tempfile
from pathlib import Path


class ThemeError(ValueError):
    """主题管理相关错误"""


class ThemeService:
    """Hugo 主题管理服务"""

    def __init__(self, hugo_root: Path | str, settings_service=None):
        """
        初始化主题服务。

        Args:
            hugo_root: Hugo 站点根目录。
            settings_service: 设置服务实例，用于读取/持久化活跃主题。
        """
        self.hugo_root = Path(hugo_root)
        self.settings_service = settings_service

    @property
    def themes_dir(self) -> Path:
        """主题目录路径"""
        return self.hugo_root / "themes"

    def list_themes(self) -> list[dict]:
        """
        列出已安装的主题。

        Returns:
            主题列表，每项包含 name 和 is_submodule。
        """
        if not self.themes_dir.exists():
            return []

        submodules = self._detect_submodules()
        themes = []
        for item in sorted(self.themes_dir.iterdir()):
            if item.is_dir() and not item.name.startswith("."):
                themes.append(
                    {
                        "name": item.name,
                        "is_submodule": item.name in submodules,
                    }
                )
        return themes

    def _detect_submodules(self) -> set[str]:
        """检测 themes/ 中哪些目录是 Git 子模块。"""
        gitmodules = self.hugo_root / ".gitmodules"
        if not gitmodules.exists():
            return set()

        submodules = set()
        try:
            with gitmodules.open("r", encoding="utf-8") as f:
                for line in f:
                    line = line.strip()
                    if line.startswith("path ="):
                        path = line.split("=", 1)[1].strip()
                        if path.startswith("themes/"):
                            submodules.add(path[len("themes/") :])
        except OSError:
            return set()
        return submodules

    def install_theme(self, repo_url: str, name: str, mode: str = "submodule") -> dict:
        """
        安装主题。

        Args:
            repo_url: Git 仓库地址。
            name: 主题名称（决定 themes/<name> 目录名）。
            mode: 安装模式，"submodule" 或 "copy"。

        Returns:
            {"name": <str>, "mode": <str>}

        Raises:
            ThemeError: 参数非法、目录冲突或安装失败。
        """
        if not repo_url or not isinstance(repo_url, str):
            raise ThemeError("主题仓库地址不能为空")
        if not name or not isinstance(name, str):
            raise ThemeError("主题名称不能为空")
        if name.startswith(".") or "/" in name or "\\" in name:
            raise ThemeError("主题名称不能包含路径分隔符或特殊字符")
        if mode not in {"submodule", "copy"}:
            raise ThemeError("安装模式仅支持 submodule 或 copy")

        target_dir = self.themes_dir / name
        if target_dir.exists():
            raise ThemeError(f"主题目录已存在: {name}")

        self.themes_dir.mkdir(parents=True, exist_ok=True)

        if mode == "submodule":
            self._install_submodule(repo_url, name)
        else:
            self._install_copy(repo_url, name)

        return {"name": name, "mode": mode}

    def _install_submodule(self, repo_url: str, name: str) -> None:
        """使用 git submodule add 安装主题。"""
        try:
            result = subprocess.run(
                ["git", "submodule", "add", repo_url, f"themes/{name}"],
                cwd=self.hugo_root,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                check=False,
            )
        except FileNotFoundError as exc:
            raise ThemeError("未找到 git 命令，请确保 Git 已安装") from exc

        if result.returncode != 0:
            raise ThemeError(
                f"子模块安装失败 (exit {result.returncode}): {result.stdout.strip()}"
            )

    def _install_copy(self, repo_url: str, name: str) -> None:
        """浅克隆到临时目录后复制到 themes/<name>。"""
        target_dir = self.themes_dir / name
        with tempfile.TemporaryDirectory(prefix="hugo-theme-") as tmp:
            tmp_path = Path(tmp)
            try:
                result = subprocess.run(
                    ["git", "clone", "--depth", "1", repo_url, str(tmp_path)],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.STDOUT,
                    text=True,
                    check=False,
                )
            except FileNotFoundError as exc:
                raise ThemeError("未找到 git 命令，请确保 Git 已安装") from exc

            if result.returncode != 0:
                raise ThemeError(
                    f"主题克隆失败 (exit {result.returncode}): {result.stdout.strip()}"
                )

            if not any(tmp_path.iterdir()):
                raise ThemeError("克隆的仓库为空")

            target_dir.mkdir(parents=True, exist_ok=True)
            for item in tmp_path.iterdir():
                if item.name == ".git":
                    continue
                dest = target_dir / item.name
                if item.is_dir():
                    shutil.copytree(item, dest, dirs_exist_ok=True)
                else:
                    shutil.copy2(item, dest)

    def activate_theme(self, name: str) -> dict:
        """
        激活主题。

        Args:
            name: 主题名称。

        Returns:
            {"name": <str>, "active": True}

        Raises:
            ThemeError: 主题不存在或持久化失败。
        """
        if not self.theme_exists(name):
            raise ThemeError(f"主题不存在: {name}")

        if self.settings_service is None:
            raise ThemeError("设置服务未配置，无法持久化活跃主题")

        try:
            self.settings_service.update_settings({"theme": {"name": name}})
        except Exception as exc:
            raise ThemeError(f"持久化活跃主题失败: {exc}") from exc

        return {"name": name, "active": True}

    def get_active_theme(self) -> str | None:
        """获取当前持久化的活跃主题名称。"""
        if self.settings_service is None:
            return None
        try:
            settings = self.settings_service.get_settings()
        except Exception:
            return None
        theme_settings = settings.get("theme", {})
        name = theme_settings.get("name", "")
        return name or None

    def theme_exists(self, name: str) -> bool:
        """检查主题目录是否存在。"""
        return (self.themes_dir / name).is_dir()
