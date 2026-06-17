# coding: utf-8
"""
基于密码验证的认证路由与会话守卫。

- ``register_auth_routes(registry)``：登录/登出/当前用户/改密 API。
- ``install_auth_guard(app)``：全局 ``before_request`` 守卫，未登录访问
  ``/api/*`` 一律返回 401（公开白名单除外）。
- ``login_required``：装饰器形式的同款校验，供需要逐路由保护时使用。
"""

from functools import wraps

from flask import Blueprint, jsonify, request, session

# 公开端点白名单：守卫对这些路径放行（即便未登录）。
PUBLIC_API_PATHS = {"/api/auth/login", "/api/auth/me", "/api/version"}

bp = Blueprint("auth", __name__)


def _is_logged_in() -> bool:
    return "username" in session


def _unauthorized(message: str = "未登录或会话已过期"):
    return (
        jsonify({"success": False, "message": message}),
        401,
    )


def login_required(func):
    """要求已登录的装饰器（返回 401 JSON）。"""

    @wraps(func)
    def wrapper(*args, **kwargs):
        if not _is_logged_in():
            return _unauthorized()
        return func(*args, **kwargs)

    return wrapper


def install_auth_guard(app):
    """注册全局守卫：未登录时拒绝除白名单外的所有 /api/* 请求。"""

    @app.before_request
    def _auth_guard():
        path = request.path
        if not path.startswith("/api/"):
            return None  # SPA 页面与静态资源放行
        if path in PUBLIC_API_PATHS:
            return None
        if not _is_logged_in():
            return _unauthorized()
        return None


def register_auth_routes(registry):
    """注册认证相关路由。

    :param registry: ServiceRegistry 实例
    :return: Blueprint
    """

    @bp.route("/api/auth/login", methods=["POST"])
    def login():
        data = request.get_json(silent=True) or {}
        username = data.get("username")
        password = data.get("password")
        if not username or not password:
            return (
                jsonify({"success": False, "message": "缺少用户名或密码"}),
                400,
            )

        auth_service = getattr(registry, "auth_service", None)
        if auth_service is None or not auth_service.verify(username, password):
            return (
                jsonify({"success": False, "message": "用户名或密码错误"}),
                401,
            )

        session.clear()
        session["username"] = username
        session.permanent = True
        return jsonify({"success": True, "user": {"username": username}})

    @bp.route("/api/auth/me", methods=["GET"])
    def me():
        username = session.get("username")
        if not username:
            return (
                jsonify({"success": False, "message": "未登录"}),
                401,
            )
        return jsonify({"success": True, "user": {"username": username}})

    @bp.route("/api/auth/logout", methods=["POST"])
    def logout():
        session.clear()
        return jsonify({"success": True})

    @bp.route("/api/auth/password", methods=["POST"])
    @login_required
    def change_password():
        data = request.get_json(silent=True) or {}
        current_password = data.get("current_password")
        new_password = data.get("new_password")
        if not current_password or not new_password:
            return (
                jsonify({"success": False, "message": "缺少当前密码或新密码"}),
                400,
            )

        username = session.get("username")
        auth_service = getattr(registry, "auth_service", None)
        if auth_service is None or not auth_service.verify(username, current_password):
            return (
                jsonify({"success": False, "message": "当前密码错误"}),
                401,
            )

        try:
            auth_service.set_password(username, new_password)
        except ValueError as e:
            return (
                jsonify({"success": False, "message": str(e)}),
                400,
            )

        return jsonify({"success": True})

    return bp
