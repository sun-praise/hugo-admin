# coding: utf-8
"""
文章管理、缓存、引用关系相关路由
"""

import logging

from flask import Blueprint, jsonify, request

logger = logging.getLogger(__name__)

bp = Blueprint("posts", __name__)


def register_post_routes(post_service, ref_service):
    """
    注册文章管理、缓存和引用关系路由

    Args:
        post_service: PostService 实例
        ref_service: ReferenceService 实例
    """

    @bp.route("/api/posts")
    def get_posts():
        """获取文章列表"""
        query = request.args.get("q", "")
        category = request.args.get("category", "")
        tag = request.args.get("tag", "")
        page = int(request.args.get("page", 1))
        per_page = int(request.args.get("per_page", 20))

        result = post_service.get_posts(
            query=query, category=category, tag=tag, page=page, per_page=per_page
        )

        return jsonify(result)

    @bp.route("/api/posts/tags")
    def get_tags():
        """获取所有标签"""
        tags = post_service.get_all_tags()
        return jsonify({"tags": tags})

    @bp.route("/api/posts/categories")
    def get_categories():
        """获取所有分类"""
        categories = post_service.get_all_categories()
        return jsonify({"categories": categories})

    @bp.route("/api/cache/refresh", methods=["POST"])
    def refresh_cache():
        """刷新文章缓存"""
        if post_service.cache_service:
            post_service.cache_service.refresh()
            stats = post_service.cache_service.get_stats()
            # 同步刷新引用索引
            try:
                ref_service.scan_all()
            except Exception as e:
                logger.exception(e)
            return jsonify({"success": True, "message": "缓存刷新成功", "stats": stats})
        else:
            return jsonify({"success": False, "message": "缓存未启用"}), 400

    @bp.route("/api/cache/stats")
    def cache_stats():
        """获取缓存统计信息"""
        if post_service.cache_service:
            stats = post_service.cache_service.get_stats()
            return jsonify({"success": True, "stats": stats})
        else:
            return jsonify({"success": False, "message": "缓存未启用"}), 400

    @bp.route("/api/references/scan", methods=["POST"])
    def scan_references():
        """扫描所有文章的引用关系"""
        try:
            ref_service.scan_all()
            refs = ref_service.db.get_all_references()
            return jsonify({"success": True, "references": refs})
        except Exception as e:
            return jsonify({"success": False, "message": str(e)}), 500

    @bp.route("/api/references/backlinks")
    def get_backlinks():
        """获取反向链接"""
        file_path = request.args.get("path")
        if not file_path:
            return jsonify({"success": False, "message": "缺少 path 参数"}), 400

        backlinks = ref_service.get_backlinks(file_path)
        return jsonify({"success": True, "backlinks": backlinks})

    @bp.route("/api/posts/search")
    def search_posts():
        """文章搜索（自动补全用）"""
        query = request.args.get("q", "")
        if not query:
            return jsonify({"success": True, "posts": []})

        posts = ref_service.search_posts(query)
        return jsonify({"success": True, "posts": posts})

    return bp
