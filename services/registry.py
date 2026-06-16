# coding: utf-8
"""
服务注册表 — 管理可变服务实例的集中容器。

当 settings 更新导致 hugo_root 变更时，settings_routes 会重建
post_service、git_service 等实例。通过 registry 间接访问可确保
所有 Blueprint 在重建后自动使用新实例，避免闭包捕获旧引用。
"""


class ServiceRegistry:
    """持有可变服务引用的轻量容器。"""

    def __init__(self, **services):
        self._services = services

    # ---- 便捷属性 ----

    @property
    def post_service(self):
        return self._services["post_service"]

    @post_service.setter
    def post_service(self, value):
        self._services["post_service"] = value

    @property
    def ref_service(self):
        return self._services["ref_service"]

    @ref_service.setter
    def ref_service(self, value):
        self._services["ref_service"] = value

    @property
    def git_service(self):
        return self._services["git_service"]

    @git_service.setter
    def git_service(self, value):
        self._services["git_service"] = value

    @property
    def database(self):
        return self._services["database"]

    @database.setter
    def database(self, value):
        self._services["database"] = value

    @property
    def hugo_manager(self):
        return self._services["hugo_manager"]

    @hugo_manager.setter
    def hugo_manager(self, value):
        self._services["hugo_manager"] = value

    @property
    def settings_service(self):
        return self._services["settings_service"]

    @settings_service.setter
    def settings_service(self, value):
        self._services["settings_service"] = value

    @property
    def ai_service(self):
        return self._services["ai_service"]

    @ai_service.setter
    def ai_service(self, value):
        self._services["ai_service"] = value

    @property
    def session_api_key(self):
        return self._services["session_api_key"]

    @session_api_key.setter
    def session_api_key(self, value):
        self._services["session_api_key"] = value

    @property
    def env_api_key(self):
        return self._services["env_api_key"]

    @env_api_key.setter
    def env_api_key(self, value):
        self._services["env_api_key"] = value

    @property
    def socketio(self):
        return self._services["socketio"]

    @socketio.setter
    def socketio(self, value):
        self._services["socketio"] = value

    @property
    def plugin_manager(self):
        return self._services.get("plugin_manager")

    @plugin_manager.setter
    def plugin_manager(self, value):
        self._services["plugin_manager"] = value
