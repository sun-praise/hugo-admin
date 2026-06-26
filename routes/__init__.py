# coding: utf-8
"""
路由模块
"""

# Keep existing
from .ai_routes import register_ai_routes
from .auth_routes import install_auth_guard, register_auth_routes
from .config_routes import register_config_routes
from .email_routes import register_email_routes
from .file_routes import register_file_routes
from .image_routes import register_image_routes
from .inline_edit_routes import register_inline_edit_routes
from .page_routes import register_page_routes
from .plugin_routes import register_plugin_routes
from .post_routes import register_post_routes
from .project_init_routes import register_project_init_routes
from .publish_routes import register_publish_routes
from .references_routes import register_references_routes
from .server_routes import register_server_routes
from .settings_routes import register_settings_routes
from .socketio_routes import register_socketio_handlers
from .theme_routes import register_theme_routes

__all__ = [
    "register_page_routes",
    "register_server_routes",
    "register_post_routes",
    "register_references_routes",
    "register_file_routes",
    "register_image_routes",
    "register_publish_routes",
    "register_email_routes",
    "register_settings_routes",
    "register_socketio_handlers",
    "register_ai_routes",
    "register_inline_edit_routes",
    "register_plugin_routes",
    "register_auth_routes",
    "install_auth_guard",
    "register_theme_routes",
    "register_config_routes",
]
