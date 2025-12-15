# coding: utf-8
"""
Hugo Blog Web 管理界面
简单轻量的 Flask 应用，用于管理 Hugo 博客
"""
import os
import sys
from pathlib import Path
from datetime import datetime
from flask import Flask, render_template, jsonify, request, send_from_directory
from flask_socketio import SocketIO, emit

from services.hugo_service import HugoServerManager
from services.post_service import PostService
from services.git_service import GitService

# 初始化 Flask 应用
app = Flask(__name__)

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
app.config['HUGO_ROOT'] = app.config.get('HUGO_ROOT', Path(__file__).parent.parent)
app.config['CONTENT_DIR'] = app.config.get('CONTENT_DIR', app.config['HUGO_ROOT'] / 'content')

# 初始化 SocketIO
socketio = SocketIO(app, cors_allowed_origins="*")

# 初始化服务
hugo_manager = HugoServerManager(app.config['HUGO_ROOT'], socketio)
post_service = PostService(app.config['CONTENT_DIR'], use_cache=True)
git_service = GitService(app.config['HUGO_ROOT'])

# 在应用启动时初始化缓存
print("正在初始化文章缓存...")
if post_service.cache_service:
    post_service.cache_service.initialize()
print("缓存初始化完成")


# ============ 页面路由 ============

@app.route('/')
def index():
    """首页 - 仪表板"""
    return render_template('index.html')


@app.route('/posts')
def posts_page():
    """文章列表页面"""
    return render_template('posts.html')


@app.route('/editor')
@app.route('/editor/<path:file_path>')
def editor_page(file_path=None):
    """文章编辑器页面"""
    return render_template('editor.html', file_path=file_path)


@app.route('/server')
def server_page():
    """Hugo 服务器控制页面"""
    return render_template('server.html')


@app.route('/test')
def test_page():
    """测试页面"""
    return send_from_directory(app.root_path, 'test_editor.html')


@app.route('/content/<path:filename>')
def serve_content_files(filename):
    """提供 content 目录下的静态文件（如图片）"""
    content_dir = app.config['CONTENT_DIR']
    return send_from_directory(content_dir, filename)


# ============ API 路由 ============

# --- Hugo 服务器管理 API ---

@app.route('/api/server/status')
def server_status():
    """获取服务器状态"""
    status = hugo_manager.get_status()
    return jsonify(status)


@app.route('/api/server/start', methods=['POST'])
def server_start():
    """启动 Hugo 服务器"""
    data = request.get_json() or {}
    debug = data.get('debug', False)

    success, message = hugo_manager.start(debug=debug)
    return jsonify({
        'success': success,
        'message': message,
        'status': hugo_manager.get_status()
    })


@app.route('/api/server/stop', methods=['POST'])
def server_stop():
    """停止 Hugo 服务器"""
    success, message = hugo_manager.stop()
    return jsonify({
        'success': success,
        'message': message,
        'status': hugo_manager.get_status()
    })


# --- 文章管理 API ---

@app.route('/api/posts')
def get_posts():
    """获取文章列表"""
    query = request.args.get('q', '')
    category = request.args.get('category', '')
    tag = request.args.get('tag', '')
    page = int(request.args.get('page', 1))
    per_page = int(request.args.get('per_page', 20))

    result = post_service.get_posts(
        query=query,
        category=category,
        tag=tag,
        page=page,
        per_page=per_page
    )

    return jsonify(result)


@app.route('/api/posts/tags')
def get_tags():
    """获取所有标签"""
    tags = post_service.get_all_tags()
    return jsonify({'tags': tags})


@app.route('/api/posts/categories')
def get_categories():
    """获取所有分类"""
    categories = post_service.get_all_categories()
    return jsonify({'categories': categories})


@app.route('/api/cache/refresh', methods=['POST'])
def refresh_cache():
    """刷新文章缓存"""
    if post_service.cache_service:
        post_service.cache_service.refresh()
        stats = post_service.cache_service.get_stats()
        return jsonify({
            'success': True,
            'message': '缓存刷新成功',
            'stats': stats
        })
    else:
        return jsonify({
            'success': False,
            'message': '缓存未启用'
        }), 400


@app.route('/api/cache/stats')
def cache_stats():
    """获取缓存统计信息"""
    if post_service.cache_service:
        stats = post_service.cache_service.get_stats()
        return jsonify({
            'success': True,
            'stats': stats
        })
    else:
        return jsonify({
            'success': False,
            'message': '缓存未启用'
        }), 400


# --- 文件操作 API ---

@app.route('/api/file/read', methods=['POST'])
def read_file():
    """读取文件内容"""
    data = request.get_json()
    file_path = data.get('path')

    if not file_path:
        return jsonify({'success': False, 'message': '缺少文件路径'}), 400

    success, content = post_service.read_file(file_path)

    if success:
        return jsonify({
            'success': True,
            'content': content,
            'path': file_path
        })
    else:
        return jsonify({
            'success': False,
            'message': content
        }), 404


@app.route('/api/file/save', methods=['POST'])
def save_file():
    """保存文件内容"""
    data = request.get_json()
    file_path = data.get('path')
    content = data.get('content')

    if not file_path or content is None:
        return jsonify({'success': False, 'message': '缺少必要参数'}), 400

    success, message = post_service.save_file(file_path, content)

    return jsonify({
        'success': success,
        'message': message
    }), 200 if success else 500


@app.route('/api/post/create', methods=['POST'])
def create_post():
    """创建新文章"""
    data = request.get_json()
    title = data.get('title')

    if not title:
        return jsonify({'success': False, 'message': '缺少文章标题'}), 400

    success, result = post_service.create_post(title)

    if success:
        return jsonify({
            'success': True,
            'path': result,
            'message': '文章创建成功'
        })
    else:
        return jsonify({
            'success': False,
            'message': result
        }), 500


@app.route('/api/image/upload', methods=['POST'])
def upload_image():
    """上传图片到文章目录"""
    if 'file' not in request.files:
        return jsonify({'success': False, 'message': '没有文件'}), 400

    file = request.files['file']
    article_path = request.form.get('article_path')

    if not article_path:
        return jsonify({'success': False, 'message': '缺少文章路径'}), 400

    if file.filename == '':
        return jsonify({'success': False, 'message': '文件名为空'}), 400

    # 检查文件类型
    allowed_extensions = {'png', 'jpg', 'jpeg', 'gif', 'svg', 'webp'}
    ext = file.filename.rsplit('.', 1)[1].lower() if '.' in file.filename else ''
    if ext not in allowed_extensions:
        return jsonify({'success': False, 'message': f'不支持的文件类型: {ext}'}), 400

    success, result = post_service.save_image(article_path, file)

    if success:
        return jsonify({
            'success': True,
            'url': result,
            'message': '图片上传成功'
        })
    else:
        return jsonify({
            'success': False,
            'message': result
        }), 500


@app.route('/api/image/list', methods=['POST'])
def list_images():
    """列出文章目录下的所有图片"""
    data = request.get_json()
    article_path = data.get('article_path')

    if not article_path:
        return jsonify({'success': False, 'message': '缺少文章路径'}), 400

    success, result = post_service.list_images(article_path)

    if success:
        return jsonify({
            'success': True,
            'images': result
        })
    else:
        return jsonify({
            'success': False,
            'message': result
        }), 500


# --- 文章发布 API ---

@app.route('/api/article/publish', methods=['POST'])
def publish_article():
    """发布单个文章"""
    data = request.get_json()

    if not data or 'file_path' not in data:
        return jsonify({
            'success': False,
            'error': '缺少 file_path 参数',
            'error_code': 'MISSING_PARAMETER'
        }), 400

    file_path = data['file_path']
    success, message, operation_id = post_service.publish_article(file_path)

    if success:
        return jsonify({
            'success': True,
            'message': message,
            'operation_id': operation_id,
            'article_path': file_path,
            'draft_status_changed': True,
            'published_at': datetime.now().isoformat() + 'Z'
        })
    else:
        # 根据错误消息返回适当的 HTTP 状态码
        if "不存在" in message:
            status_code = 404
        elif "已经发布" in message or "访问被拒绝" in message:
            status_code = 409
        else:
            status_code = 400

        return jsonify({
            'success': False,
            'error': message,
            'error_code': 'PUBLISH_FAILED'
        }), status_code


@app.route('/api/article/status')
def get_article_status():
    """获取文章发布状态"""
    file_path = request.args.get('file_path')

    if not file_path:
        return jsonify({
            'success': False,
            'error': '缺少 file_path 参数',
            'error_code': 'MISSING_PARAMETER'
        }), 400

    status = post_service.get_publish_status(file_path)

    if 'error' in status:
        status_code = 404 if "不存在" in status['error'] else 400
        return jsonify({
            'success': False,
            'error': status['error'],
            'error_code': 'STATUS_CHECK_FAILED'
        }), status_code

    return jsonify({
        'success': True,
        'status': status
    })


@app.route('/api/article/status/bulk', methods=['POST'])
def get_bulk_article_status():
    """批量获取文章发布状态"""
    data = request.get_json()

    if not data or 'file_paths' not in data:
        return jsonify({
            'success': False,
            'error': '缺少 file_paths 参数',
            'error_code': 'MISSING_PARAMETER'
        }), 400

    file_paths = data['file_paths']
    results = []

    for file_path in file_paths:
        status = post_service.get_publish_status(file_path)
        results.append({
            'file_path': file_path,
            'status': status
        })

    return jsonify({
        'success': True,
        'results': results,
        'count': len(results)
    })


@app.route('/api/article/publish/bulk', methods=['POST'])
def bulk_publish_articles():
    """批量发布文章"""
    data = request.get_json()

    if not data or 'file_paths' not in data:
        return jsonify({
            'success': False,
            'error': '缺少 file_paths 参数',
            'error_code': 'MISSING_PARAMETER'
        }), 400

    file_paths = data['file_paths']
    stop_on_error = data.get('stop_on_first_error', False)

    try:
        result = post_service.bulk_publish_articles(file_paths)

        # 根据结果返回适当的 HTTP 状态码
        if result['success'] or not result['failed_count']:
            status_code = 200
        elif result['failed_count'] > 0 and result['published_count'] > 0:
            status_code = 207  # Multi-Status
        else:
            status_code = 400

        return jsonify(result), status_code
    except Exception as e:
        return jsonify({
            'success': False,
            'error': f'批量发布失败: {str(e)}',
            'error_code': 'BULK_PUBLISH_FAILED'
        }), 500


# ============ Git / 系统发布相关 API ============

@app.route('/api/git/status')
def git_status():
    """获取 Git 仓库状态"""
    try:
        status = git_service.get_status()
        return jsonify(status)
    except Exception as e:
        return jsonify({
            'success': False,
            'message': f'获取 Git 状态失败: {str(e)}'
        }), 500


@app.route('/api/git/commits')
def git_commits():
    """获取最近的提交记录"""
    try:
        count = request.args.get('count', 10, type=int)
        result = git_service.get_recent_commits(count)
        return jsonify(result)
    except Exception as e:
        return jsonify({
            'success': False,
            'message': f'获取提交记录失败: {str(e)}'
        }), 500


@app.route('/api/publish/system', methods=['POST'])
def publish_system():
    """系统发布 - 执行 git add, commit, push 完整流程"""
    try:
        data = request.get_json() or {}
        commit_message = data.get('message')

        result = git_service.publish_system(commit_message)

        if result['success']:
            return jsonify(result), 200
        else:
            return jsonify(result), 400

    except Exception as e:
        return jsonify({
            'success': False,
            'message': f'系统发布失败: {str(e)}',
            'steps': {}
        }), 500


# ============ WebSocket 事件 ============

@socketio.on('connect')
def handle_connect():
    """客户端连接"""
    emit('connected', {'message': '已连接到服务器'})


@socketio.on('disconnect')
def handle_disconnect():
    """客户端断开连接"""
    print('Client disconnected')


@socketio.on('request_logs')
def handle_request_logs():
    """客户端请求日志"""
    logs = hugo_manager.get_recent_logs()
    emit('server_log', {'logs': logs})


# ============ 错误处理 ============

@app.errorhandler(404)
def not_found(e):
    """404 错误处理"""
    if request.path.startswith('/api/'):
        return jsonify({'success': False, 'message': '接口不存在'}), 404
    return render_template('404.html'), 404


@app.errorhandler(500)
def server_error(e):
    """500 错误处理"""
    if request.path.startswith('/api/'):
        return jsonify({'success': False, 'message': '服务器内部错误'}), 500
    return render_template('500.html'), 500


# ============ 主程序入口 ============

if __name__ == '__main__':
    print("=" * 50)
    print("Hugo Blog Web 管理界面")
    print("=" * 50)
    print(f"Hugo 根目录: {app.config['HUGO_ROOT']}")
    print(f"内容目录: {app.config['CONTENT_DIR']}")

    host = '0.0.0.0'
    port = app.config.get('PORT', 5050)  # 从配置中读取端口，默认为5050
    print(f"访问地址: http://{host}:{port}")
    print("=" * 50)

    # 运行应用
    # allow_unsafe_werkzeug=True 允许使用 Werkzeug 开发服务器(仅用于开发环境)
    socketio.run(app, host=host, port=port, debug=True, allow_unsafe_werkzeug=True)
