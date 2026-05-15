import { useState, useEffect, useCallback } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Search, RefreshCw, Plus, Upload, Tag, FolderOpen, Calendar, Clock, FileText } from 'lucide-react';
import { get, post } from '../utils/api';
import type { Post, PostsResponse, Tag as TagType, Category } from '../types';

export default function Posts() {
  const navigate = useNavigate();
  const [posts, setPosts] = useState<Post[]>([]);
  const [pagination, setPagination] = useState({ total: 0, page: 1, per_page: 20, total_pages: 0, has_next: false, has_prev: false });
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [tags, setTags] = useState<TagType[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [filters, setFilters] = useState({ query: '', category: '', tag: '' });
  const [selectedPosts, setSelectedPosts] = useState<Set<string>>(new Set());
  const [selectAll, setSelectAll] = useState(false);
  const [bulkPublishing, setBulkPublishing] = useState(false);

  const loadPosts = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('per_page', '20');
      if (filters.query) params.set('q', filters.query);
      if (filters.category) params.set('category', filters.category);
      if (filters.tag) params.set('tag', filters.tag);
      const data = await get<PostsResponse>(`/api/posts?${params.toString()}`);
      setPosts(data.posts);
      setPagination({
        total: data.total,
        page: data.page,
        per_page: data.per_page,
        total_pages: data.total_pages,
        has_next: data.has_next,
        has_prev: data.has_prev,
      });
    } catch (error) {
      console.error('Failed to load posts:', error);
    } finally {
      setLoading(false);
    }
  }, [filters]);

  async function loadFilters() {
    try {
      const [tagsData, catsData] = await Promise.all([
        get<{ tags: TagType[] }>('/api/posts/tags'),
        get<{ categories: Category[] }>('/api/posts/categories'),
      ]);
      setTags(tagsData.tags);
      setCategories(catsData.categories);
    } catch (error) {
      console.error('Failed to load filters:', error);
    }
  }

  useEffect(() => {
    loadPosts();
    loadFilters();
  }, [loadPosts]);

  async function refreshCache() {
    setRefreshing(true);
    try {
      await post('/api/cache/refresh');
      await loadPosts();
      showNotification('缓存刷新成功', 'success');
    } catch (error) {
      showNotification('刷新失败', 'error');
    } finally {
      setRefreshing(false);
    }
  }

  async function createNewPost() {
    const title = prompt('请输入文章标题:');
    if (!title) return;
    try {
      const data = await post<{ success: boolean; path?: string; message?: string }>('/api/post/create', { title });
      if (data.success && data.path) {
        showNotification('文章创建成功', 'success');
        setTimeout(() => navigate(`/editor/${data.path}`), 1000);
      } else {
        showNotification('创建失败: ' + (data.message || ''), 'error');
      }
    } catch (error) {
      showNotification('创建失败', 'error');
    }
  }

  function togglePostSelection(path: string) {
    setSelectedPosts((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }

  function toggleSelectAll() {
    if (selectAll) {
      setSelectedPosts(new Set());
    } else {
      setSelectedPosts(new Set(posts.map((p) => p.path)));
    }
    setSelectAll(!selectAll);
  }

  async function bulkPublish() {
    if (selectedPosts.size === 0) return;
    setBulkPublishing(true);
    try {
      const paths = Array.from(selectedPosts);
      const result = await post<{ success: boolean; results?: Array<{ path: string; success: boolean; message?: string }>; message?: string }>('/api/article/publish/bulk', { paths });
      if (result.success && result.results) {
        const successCount = result.results.filter((r) => r.success).length;
        showNotification(`批量发布完成：${successCount}/${paths.length} 成功`, 'success');
        await loadPosts();
        setSelectedPosts(new Set());
        setSelectAll(false);
      } else {
        showNotification('批量发布失败: ' + result.message, 'error');
      }
    } catch (error) {
      showNotification('批量发布失败', 'error');
    } finally {
      setBulkPublishing(false);
    }
  }

  function showNotification(message: string, type: 'info' | 'success' | 'error' | 'warning' = 'info') {
    const notification = document.createElement('div');
    notification.className = `fixed top-4 right-4 px-6 py-3 rounded-lg shadow-lg z-50 fade-in ${
      type === 'success' ? 'bg-green-500' : type === 'error' ? 'bg-red-500' : type === 'warning' ? 'bg-yellow-500' : 'bg-blue-500'
    } text-white`;
    notification.textContent = message;
    document.body.appendChild(notification);
    setTimeout(() => notification.remove(), 3000);
  }

  return (
    <div>
      {/* 搜索和筛选 */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div className="md:col-span-2">
            <label className="block text-sm font-medium text-gray-700 mb-2">搜索</label>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
              <input
                type="text"
                value={filters.query}
                onChange={(e) => setFilters({ ...filters, query: e.target.value })}
                placeholder="搜索标题、内容、标签..."
                className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              />
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">分类</label>
            <select
              value={filters.category}
              onChange={(e) => setFilters({ ...filters, category: e.target.value })}
              className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500"
            >
              <option value="">全部分类</option>
              {categories.map((cat) => (
                <option key={cat.name} value={cat.name}>
                  {cat.name} ({cat.count})
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">标签</label>
            <select
              value={filters.tag}
              onChange={(e) => setFilters({ ...filters, tag: e.target.value })}
              className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500"
            >
              <option value="">全部标签</option>
              {tags.map((tag) => (
                <option key={tag.name} value={tag.name}>
                  {tag.name} ({tag.count})
                </option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {/* 文章列表 */}
      <div className="bg-white rounded-lg shadow">
        {loading ? (
          <div className="p-12 text-center">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600" />
            <p className="mt-4 text-gray-600">加载中...</p>
          </div>
        ) : (
          <div>
            <div className="px-6 py-4 border-b border-gray-200">
              <div className="flex items-center justify-between flex-wrap gap-2">
                <h3 className="text-lg font-semibold">共 {pagination.total} 篇文章</h3>
                <div className="flex items-center gap-2">
                  <button
                    onClick={refreshCache}
                    disabled={refreshing}
                    className="px-4 py-2 border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50 transition-colors disabled:opacity-50 flex items-center"
                  >
                    <RefreshCw className={`w-4 h-4 mr-2 ${refreshing ? 'animate-spin' : ''}`} />
                    {refreshing ? '刷新中...' : '刷新缓存'}
                  </button>
                  <button
                    onClick={createNewPost}
                    className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors flex items-center"
                  >
                    <Plus className="w-4 h-4 mr-2" />
                    新建文章
                  </button>
                </div>
              </div>
            </div>

            <div className="flex items-center gap-4 px-6 py-3 border-b border-gray-200">
              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="select-all"
                  checked={selectAll}
                  onChange={toggleSelectAll}
                  disabled={posts.length === 0}
                  className="w-4 h-4 border-gray-300 rounded text-blue-600 focus:ring-2 focus:ring-blue-500"
                />
                <label htmlFor="select-all" className="text-sm font-medium text-gray-700 cursor-pointer">
                  全选 ({selectedPosts.size} / {posts.length})
                </label>
              </div>
              <button
                onClick={bulkPublish}
                disabled={selectedPosts.size === 0 || bulkPublishing}
                className="px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center"
              >
                <Upload className="w-4 h-4 mr-2" />
                {bulkPublishing ? '批量发布中...' : `批量发布 (${selectedPosts.size})`}
              </button>
            </div>

            <div className="divide-y divide-gray-200">
              {posts.length === 0 && (
                <div className="p-12 text-center text-gray-500">
                  <FileText className="w-16 h-16 mb-4 text-gray-400 mx-auto" />
                  <p>未找到文章</p>
                </div>
              )}
              {posts.map((post) => (
                <div key={post.path} className="p-6 hover:bg-gray-50 transition-colors">
                  <div className="flex items-start justify-between">
                    <div className="flex-shrink-0 mr-4 pt-1">
                      <input
                        type="checkbox"
                        checked={selectedPosts.has(post.path)}
                        onChange={() => togglePostSelection(post.path)}
                        className="w-4 h-4 border-gray-300 rounded text-blue-600 focus:ring-2 focus:ring-blue-500"
                      />
                    </div>
                    {post.cover_url && (
                      <Link to={`/editor/${post.path}`} className="flex-shrink-0 mr-4">
                        <img
                          src={post.cover_url}
                          alt={post.title}
                          className="w-24 h-16 object-cover rounded-lg bg-gray-100"
                          onError={(e) => (e.currentTarget.style.display = 'none')}
                        />
                      </Link>
                    )}
                    <div className="flex-1 min-w-0">
                      <Link to={`/editor/${post.path}`} className="block">
                        <h4 className="text-lg font-semibold text-gray-900 hover:text-blue-600 mb-2">{post.title}</h4>
                      </Link>
                      <p className="text-gray-600 text-sm mb-3 line-clamp-2">{post.description || post.excerpt}</p>
                      <div className="flex flex-wrap items-center gap-4 text-sm text-gray-500">
                        <span className="flex items-center">
                          <Calendar className="w-4 h-4 mr-1" />
                          {post.date}
                        </span>
                        {post.status && (
                          <span className="flex items-center">
                            <Upload className="w-4 h-4 mr-1" />
                            <span className={post.status.is_draft ? 'text-yellow-600' : 'text-green-600 font-medium'}>
                              {post.status.is_draft ? '草稿' : '已发布'}
                            </span>
                          </span>
                        )}
                        {post.categories.length > 0 && (
                          <span className="flex items-center">
                            <FolderOpen className="w-4 h-4 mr-1" />
                            {post.categories.join(', ')}
                          </span>
                        )}
                        <span className="flex items-center text-xs">
                          <Clock className="w-3 h-3 mr-1" />
                          更新于 {post.mod_time}
                        </span>
                      </div>
                      {post.tags.length > 0 && (
                        <div className="flex flex-wrap gap-1 mt-2">
                          {post.tags.map((tag) => (
                            <span key={tag} className="inline-flex items-center px-2 py-0.5 rounded text-xs bg-gray-100 text-gray-600">
                              <Tag className="w-3 h-3 mr-1" />
                              {tag}
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
