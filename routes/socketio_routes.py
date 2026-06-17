from flask import session
from flask_socketio import emit


def register_socketio_handlers(registry):
    """Register SocketIO event handlers.

    SocketIO events are not Flask routes, so this is a plain registration
    function rather than a Blueprint factory.
    """

    def handle_connect():
        """客户端连接 — 未登录则拒绝，防止实时通道绕过 API 守卫。"""
        if "username" not in session:
            emit("auth_error", {"message": "未登录或会话已过期"})
            return False  # 拒绝连接
        emit("connected", {"message": "已连接到服务器"})

    registry.socketio.on("connect")(handle_connect)

    def handle_disconnect():
        """客户端断开连接"""
        print("Client disconnected")

    registry.socketio.on("disconnect")(handle_disconnect)

    def handle_request_logs():
        """客户端请求日志"""
        logs = registry.hugo_manager.get_recent_logs()
        emit("server_log", {"logs": logs})

    registry.socketio.on("request_logs")(handle_request_logs)
