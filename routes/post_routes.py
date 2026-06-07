# coding: utf-8
"""
文章管理、缓存相关路由
"""

import logging

from flask import Blueprint, jsonify, request

logger = logging.getLogger(__name__)

bp = Blueprint("posts", __name__)


def register_post_routes(registry):
    """
    注册文章管理、缓存和引用关系路由

    Args:
        registry: ServiceRegistry 实例
    """

    @bp.route("/api/posts")
    def get_posts():
        """获取文章列表"""
        query = request.args.get("q", "")
        category = request.args.get("category", "")
        tag = request.args.get("tag", "")
        page = int(request.args.get("page", 1))
        per_page = int(request.args.get("per_page", 20))

        result = registry.post_service.get_posts(
            query=query, category=category, tag=tag, page=page, per_page=per_page
        )

        return jsonify(result)

    @bp.route("/api/posts/tags")
    def get_tags():
        """获取所有标签"""
        tags = registry.post_service.get_all_tags()
        return jsonify({"tags": tags})

    @bp.route("/api/posts/categories")
    def get_categories():
        """获取所有分类"""
        categories = registry.post_service.get_all_categories()
        return jsonify({"categories": categories})

    @bp.route("/api/cache/refresh", methods=["POST"])
    def refresh_cache():
        """刷新文章缓存"""
        if registry.post_service.cache_service:
            registry.post_service.cache_service.refresh()
            stats = registry.post_service.cache_service.get_stats()
            # 同步刷新引用索引
            try:
                registry.ref_service.scan_all()
            except Exception as e:
                logger.exception(e)
            return jsonify({"success": True, "message": "缓存刷新成功", "stats": stats})
        else:
            return jsonify({"success": False, "message": "缓存未启用"}), 400

    @bp.route("/api/cache/stats")
    def cache_stats():
        """获取缓存统计信息"""
        if registry.post_service.cache_service:
            stats = registry.post_service.cache_service.get_stats()
            return jsonify({"success": True, "stats": stats})
        else:
            return jsonify({"success": False, "message": "缓存未启用"}), 400

    return bp
