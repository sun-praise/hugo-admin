# coding: utf-8
"""
SocketIO connect 鉴权测试：未登录连接应被拒绝。
"""

import app as app_module


def test_socket_connect_rejects_anonymous(auth_store):
    """未登录的 SocketIO 连接应被 connect 守卫拒绝。"""
    tc = app_module.socketio.test_client(app_module.app)
    assert tc is not None
    assert not tc.is_connected()


def test_socket_connect_accepts_session(auth_store, login):
    """已登录（携带会话）的连接应被接受。"""
    http = app_module.app.test_client()
    login(http)
    tc = app_module.socketio.test_client(app_module.app, flask_test_client=http)
    assert tc.is_connected()
