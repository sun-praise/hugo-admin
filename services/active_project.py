# coding: utf-8
"""
活跃 Hugo 项目路径持久化

`switch_active_project` 只把活跃项目写入 Flask 内存的 ``app.config["HUGO_ROOT"]``，
但 Flask 进程重启后会丢失，导致用户新建的博客"消失"。本服务把活跃项目路径
持久化到 ``data/active_project.txt``，``app.py`` 启动时优先读它，再回退到环境
变量或默认值。
"""

import logging
from pathlib import Path

logger = logging.getLogger(__name__)

DEFAULT_FILENAME = "active_project.txt"


class ActiveProjectRegistry:
    """
    持久化"当前活跃 Hugo 站点根目录"的小型注册器。

    文件格式：单行，写入绝对路径。读取时校验：
      - 文件存在
      - 路径非空
      - 路径是一个已存在的目录

    校验失败的路径会被忽略并视为"未持久化"，避免坏值导致启动崩溃。
    """

    def __init__(self, file_path: Path | str):
        self.file_path = Path(file_path)
        self.file_path.parent.mkdir(parents=True, exist_ok=True)

    def record_path(self, project_root: Path | str) -> None:
        """把活跃项目绝对路径写入持久化文件。"""
        try:
            path_str = str(Path(project_root).resolve())
        except OSError as exc:
            logger.warning("无法解析活跃项目路径 (%s): %s", project_root, exc)
            return

        try:
            self.file_path.write_text(path_str + "\n", encoding="utf-8")
        except OSError as exc:
            logger.warning("写入活跃项目文件失败 (%s): %s", self.file_path, exc)

    def load_path(self) -> Path | None:
        """
        读取持久化的活跃项目路径；不存在或无效时返回 ``None``。

        返回值是 ``Path`` 绝对路径，调用方可直接用于 ``HUGO_ROOT``。
        """
        if not self.file_path.exists():
            return None
        try:
            raw = self.file_path.read_text(encoding="utf-8").strip()
        except OSError as exc:
            logger.warning("读取活跃项目文件失败 (%s): %s", self.file_path, exc)
            return None
        if not raw:
            return None
        candidate = Path(raw)
        if not candidate.is_absolute():
            candidate = candidate.resolve()
        if not candidate.is_dir():
            logger.info("持久化的活跃项目路径无效（%s），已忽略", candidate)
            return None
        return candidate

    def clear(self) -> None:
        """清除持久化文件（用于恢复默认 HUGO_ROOT）。"""
        try:
            if self.file_path.exists():
                self.file_path.unlink()
        except OSError as exc:
            logger.warning("删除活跃项目文件失败 (%s): %s", self.file_path, exc)
