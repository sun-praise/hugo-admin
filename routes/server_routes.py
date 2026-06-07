# coding: utf-8
"""
Hugo 服务器管理路由
"""

from flask import Blueprint, jsonify, request

# 创建 Blueprint
server_bp = Blueprint("server", __name__, url_prefix="/api/server")


def register_server_routes(hugo_manager):
    """
    注册 Hugo 服务器管理路由。

    Args:
        hugo_manager: HugoServerManager 实例
    """

    @server_bp.route("/status")
    def server_status():
        """获取服务器状态"""
        status = hugo_manager.get_status()
        return jsonify(status)

    @server_bp.route("/start", methods=["POST"])
    def server_start():
        """启动 Hugo 服务器"""
        data = request.get_json() or {}
        debug = data.get("debug", False)

        success, message = hugo_manager.start(debug=debug)
        return jsonify(
            {
                "success": success,
                "message": message,
                "status": hugo_manager.get_status(),
            }
        )

    @server_bp.route("/stop", methods=["POST"])
    def server_stop():
        """停止 Hugo 服务器"""
        success, message = hugo_manager.stop()
        return jsonify(
            {
                "success": success,
                "message": message,
                "status": hugo_manager.get_status(),
            }
        )

    return server_bp
