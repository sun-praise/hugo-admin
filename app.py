# coding: utf-8
"""
Hugo Blog Web 管理界面
简单轻量的 Flask 应用，用于管理 Hugo 博客
"""

import logging
from logging.handlers import RotatingFileHandler
from pathlib import Path

from dotenv import load_dotenv
from flask import Flask, current_app, jsonify, request, send_file
from flask_socketio import SocketIO

from models.database import Database
from routes import (
    register_ai_routes,
    register_email_routes,
    register_file_routes,
    register_image_routes,
    register_page_routes,
    register_post_routes,
    register_publish_routes,
    register_references_routes,
    register_server_routes,
    register_settings_routes,
    register_socketio_handlers,
)
from routes.plugin_routes import register_plugin_routes
from routes.settings_routes import _ensure_server_url_has_port
from services.chat_history_service import ChatHistoryService
from services.git_service import GitService
from services.hugo_service import HugoServerManager
from services.plugin_manager import PluginManager
from services.post_service import PostService
from services.reference_service import ReferenceService
from services.registry import ServiceRegistry
from services.settings_service import (
    SettingsService,
    SettingsStorageError,
    SettingsValidationError,
)

# 初始化 Flask 应用
load_dotenv()
app = Flask(__name__)


# React SPA 的 index.html 路径
REACT_INDEX = Path(__file__).parent / "static" / "dist" / "index.html"

# 配置日志 - 写入 app.log，带轮转
logger = logging.getLogger(__name__)
log_file = Path(__file__).parent / "app.log"
file_handler = RotatingFileHandler(
    log_file, maxBytes=5 * 1024 * 1024, backupCount=5, encoding="utf-8"
)
file_handler.setLevel(logging.DEBUG)
file_handler.setFormatter(
    logging.Formatter("%(asctime)s [%(levelname)s] %(name)s: %(message)s")
)
logging.root.addHandler(file_handler)
logging.root.setLevel(logging.INFO)
logging.getLogger("werkzeug").setLevel(logging.INFO)


# 加载配置
try:
    from config_local import LocalConfig

    app.config.from_object(LocalConfig)
    print("✓ 已加载 config_local.py 配置")
except ImportError:
    from config import DevelopmentConfig

    app.config.from_object(DevelopmentConfig)
    print("✓ 已加载默认配置 (config.py)")


# 向后兼容的配置
app.config["HUGO_ROOT"] = app.config.get("HUGO_ROOT", Path(__file__).parent.parent)
app.config["CONTENT_DIR"] = app.config.get(
    "CONTENT_DIR", app.config["HUGO_ROOT"] / "content"
)
ENV_AI_API_KEY = app.config.get("AI_API_KEY", "")

# 初始化可持久化设置
settings_service = SettingsService(
    app.config["HUGO_ROOT"] / ".admin" / "settings.json",
    defaults={
        "AI_BASE_URL": app.config.get("AI_BASE_URL"),
        "AI_MODEL": app.config.get("AI_MODEL"),
        "HUGO_BASE_DIR": str(app.config.get("HUGO_ROOT", "")),
        "HUGO_SERVER_URL": app.config.get("HUGO_SERVER_BASE_URL"),
    },
)

try:
    persisted_settings = settings_service.get_settings()
    _hugo_server_url = persisted_settings.get("hugo", {}).get("server_url", "")
    _hugo_server_url = _ensure_server_url_has_port(app, _hugo_server_url)
    app.config["AI_BASE_URL"] = persisted_settings["ai"]["base_url"]
    app.config["AI_MODEL"] = persisted_settings["ai"]["model"]
    app.config["AI_API_KEY"] = ENV_AI_API_KEY
except (SettingsValidationError, SettingsStorageError) as e:
    print(f"⚠ 设置文件读取失败，继续使用默认配置: {e}")
    _hugo_server_url = ""

# 初始化 SocketIO
socketio = SocketIO(app, cors_allowed_origins="*")

# 初始化服务
hugo_manager = HugoServerManager(
    app.config["HUGO_ROOT"], socketio, server_url=_hugo_server_url or None
)
post_service = PostService(app.config["CONTENT_DIR"], use_cache=True)
ref_service = ReferenceService(
    app.config["CONTENT_DIR"],
    post_service.cache_service.db if post_service.cache_service else None,
)
ref_service.scan_all()
git_service = GitService(app.config["HUGO_ROOT"])

db_path = Path(app.config["CONTENT_DIR"]) / ".admin" / "cache.db"
db = Database(str(db_path))
chat_history_service = ChatHistoryService(db)
app.chat_history_service = chat_history_service

# Lazy AI service init to avoid import errors when API key is missing in tests
ai_service = None


registry = ServiceRegistry(
    post_service=post_service,
    git_service=git_service,
    hugo_manager=hugo_manager,
    settings_service=settings_service,
    ref_service=ref_service,
    ai_service=ai_service,
    session_api_key="",
    env_api_key=ENV_AI_API_KEY,
    socketio=socketio,
)

# ============ Plugin 系统 ============

plugin_manager = PluginManager()
try:
    plugin_manager.start_all()
    print(
        f"✓ 已加载 {len([p for p in plugin_manager.list_plugins() if p['enabled']])} 个插件"
    )
    registry.plugin_manager = plugin_manager
except Exception as e:
    print(f"⚠ 插件系统初始化失败: {e}")

# ============ AI 服务懒加载 ============


class _DisabledAIService:
    """Mock AI service for when API key is not configured."""

    def __init__(self):
        self.enabled = False
        self.deps = None
        self.mcp_server = None
        self.options = None
        self.model_name = None

    async def chat(self, message, history=None):
        raise RuntimeError("AI service is disabled")


def get_ai_service():
    """Get or lazily initialize AI service."""
    ai = registry.ai_service
    if ai is None:
        from services.ai_service import AIService

        api_key = app.config.get("AI_API_KEY", "")
        if not api_key:
            print("⚠ AI service disabled: AI_API_KEY not configured")
            ai = _DisabledAIService()
        else:
            print("✓ Initializing AI service...")
            try:
                ai = AIService(
                    api_key=api_key,
                    base_url=app.config.get("AI_BASE_URL", "https://api.deepseek.com"),
                    model_name=app.config.get("AI_MODEL", "deepseek-chat"),
                    post_service=registry.post_service,
                    git_service=registry.git_service,
                    hugo_manager=registry.hugo_manager,
                )
            except Exception as e:
                print(f"⚠ AI service initialization failed: {e}")
                ai = _DisabledAIService()
        registry.ai_service = ai
    return ai


# ============ 注册 Blueprint ============

app.config["REACT_INDEX"] = REACT_INDEX
app.register_blueprint(register_page_routes())
app.register_blueprint(register_server_routes(registry))
app.register_blueprint(register_post_routes(registry))
app.register_blueprint(register_references_routes(registry))
app.register_blueprint(register_file_routes(registry))
app.register_blueprint(register_image_routes(registry))
app.register_blueprint(register_publish_routes(registry))
app.register_blueprint(register_email_routes())
app.register_blueprint(register_settings_routes(app, registry))
ai_main_bp, fm_bp = register_ai_routes(get_ai_service)
app.register_blueprint(ai_main_bp)
app.register_blueprint(fm_bp)
app.register_blueprint(register_plugin_routes(plugin_manager))

# ============ 注册 SocketIO 事件 ============

register_socketio_handlers(registry)

# ============ 错误处理 ============


@app.errorhandler(404)
def not_found(e):
    """404 错误处理 - SPA fallback"""
    if request.path.startswith("/api/"):
        return jsonify({"success": False, "message": "接口不存在"}), 404
    if request.path.startswith("/static/dist/"):
        return jsonify({"success": False, "message": "静态文件不存在"}), 404
    return send_file(current_app.config["REACT_INDEX"])


@app.errorhandler(500)
def server_error(e):
    """500 错误处理"""
    logger.exception(e)
    if request.path.startswith("/api/"):
        return jsonify({"success": False, "message": "服务器内部错误"}), 500
    return (
        '<html><body><h1>500 - 服务器内部错误</h1><p><a href="/">返回首页</a></p></body></html>',
        500,
    )


# ============ 主程序入口 ============

if __name__ == "__main__":
    print("=" * 50)
    print("Hugo Blog Web 管理界面")
    print("=" * 50)
    print(f"Hugo 根目录: {app.config['HUGO_ROOT']}")
    print(f"内容目录: {app.config['CONTENT_DIR']}")

    host = "0.0.0.0"
    port = app.config.get("PORT", 5050)

    import socket

    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("8.8.8.8", 80))
        inet_ip = s.getsockname()[0]
        s.close()
    except Exception:
        inet_ip = "127.0.0.1"

    print(f"本地访问: http://127.0.0.1:{port}")
    print(f"局域网访问: http://{inet_ip}:{port}")
    print("=" * 50)

    socketio.run(app, host=host, port=port, debug=False, allow_unsafe_werkzeug=True)
