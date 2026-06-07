from flask_socketio import emit


def register_socketio_handlers(socketio, hugo_manager):
    """Register SocketIO event handlers.

    SocketIO events are not Flask routes, so this is a plain registration
    function rather than a Blueprint factory.
    """

    def handle_connect():
        """客户端连接"""
        emit("connected", {"message": "已连接到服务器"})

    socketio.on("connect")(handle_connect)

    def handle_disconnect():
        """客户端断开连接"""
        print("Client disconnected")

    socketio.on("disconnect")(handle_disconnect)

    def handle_request_logs():
        """客户端请求日志"""
        logs = hugo_manager.get_recent_logs()
        emit("server_log", {"logs": logs})

    socketio.on("request_logs")(handle_request_logs)
