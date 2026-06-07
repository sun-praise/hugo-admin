# coding: utf-8
"""
引用关系、反向链接、文章搜索相关路由
"""

from flask import Blueprint, jsonify, request

bp = Blueprint("references", __name__)


def register_references_routes(registry):
    """
    注册引用关系相关路由

    Args:
        registry: ServiceRegistry 实例
    """

    @bp.route("/api/references/scan", methods=["POST"])
    def scan_references():
        """扫描所有文章的引用关系"""
        try:
            registry.ref_service.scan_all()
            refs = registry.ref_service.db.get_all_references()
            return jsonify({"success": True, "references": refs})
        except Exception as e:
            return jsonify({"success": False, "message": str(e)}), 500

    @bp.route("/api/references/backlinks")
    def get_backlinks():
        """获取反向链接"""
        file_path = request.args.get("path")
        if not file_path:
            return jsonify({"success": False, "message": "缺少 path 参数"}), 400

        backlinks = registry.ref_service.get_backlinks(file_path)
        return jsonify({"success": True, "backlinks": backlinks})

    @bp.route("/api/posts/search")
    def search_posts():
        """文章搜索（自动补全用）"""
        query = request.args.get("q", "")
        if not query:
            return jsonify({"success": True, "posts": []})

        posts = registry.ref_service.search_posts(query)
        return jsonify({"success": True, "posts": posts})

    return bp
