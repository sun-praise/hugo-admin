# coding: utf-8
"""
配置文件
"""

import os
from pathlib import Path


class Config:
    """基础配置"""

    # Flask 配置
    SECRET_KEY = os.environ.get("SECRET_KEY") or "dev-secret-key-change-in-production"
    DEBUG = True

    # 项目路径配置
    BASE_DIR = Path(__file__).parent.parent
    WEB_ADMIN_DIR = Path(__file__).parent
    HUGO_ROOT = BASE_DIR
    CONTENT_DIR = BASE_DIR / "content"
    PUBLIC_DIR = BASE_DIR / "public"

    # Hugo 配置
    HUGO_SERVER_HOST = "0.0.0.0"
    HUGO_SERVER_PORT = 1313
    HUGO_SERVER_BASE_URL = "http://192.168.2.14"

    # 文件上传配置
    MAX_CONTENT_LENGTH = 16 * 1024 * 1024  # 16MB
    ALLOWED_EXTENSIONS = {"md", "markdown", "jpg", "jpeg", "png", "gif", "webp"}

    # 安全配置
    # 只允许编辑这些目录下的文件
    ALLOWED_PATHS = [
        CONTENT_DIR / "post",
        CONTENT_DIR / "page",
    ]

    # WebSocket 配置
    SOCKETIO_ASYNC_MODE = "threading"

    # AI 助手配置
    AI_API_KEY = os.environ.get("AI_API_KEY") or ""
    AI_BASE_URL = os.environ.get("AI_BASE_URL") or "https://api.deepseek.com"
    AI_MODEL = os.environ.get("AI_MODEL") or "deepseek-chat"

    @staticmethod
    def init_app(app):
        """初始化应用配置"""
        pass


class DevelopmentConfig(Config):
    """开发环境配置"""

    DEBUG = True
    TESTING = False


class ProductionConfig(Config):
    """生产环境配置"""

    DEBUG = False
    TESTING = False
    # 生产环境应该从环境变量读取密钥
    SECRET_KEY = os.environ.get("SECRET_KEY") or "production-secret-key-please-change"


# 配置字典
config = {
    "development": DevelopmentConfig,
    "production": ProductionConfig,
    "default": DevelopmentConfig,
}
