import { useState, useEffect, useCallback, useRef } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Search, RefreshCw, Plus, Upload, FileUp, Tag, FolderOpen, Calendar, Clock, FileText, ChevronLeft, ChevronRight } from 'lucide-react';
import { get, post, uploadMarkdown } from '../utils/api';
import { useSocket } from '../hooks/useSocket';
import type { Post, PostsResponse, Tag as TagType, Category } from '../types';

export default function Posts() {
  const navigate = useNavigate();
  const [posts, setPosts] = useState<Post[]>([]);
  const [pagination, setPagination] = useState({ total: 0, page: 1, per_page: 20, total_pages: 0, has_next: false, has_prev: false });
  const [currentPage, setCurrentPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [tags, setTags] = useState<TagType[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [filters, setFilters] = useState({ query: '', category: '', tag: '' });
  const [selectedPosts, setSelectedPosts] = useState<Set<string>>(new Set());
  const [selectAll, setSelectAll] = useState(false);
  const [bulkPublishing, setBulkPublishing] = useState(false);

  // loadPosts takes the page + filters explicitly so callers can pass the
  // next values without waiting for a ref-flush cycle. Defaults fall back
  // to the current state when called with no args (e.g. on initial mount).
  const loadPosts = useCallback(
    async (page: number = currentPage, f: typeof filters = filters) => {
      setLoading(true);
      try {
        const params = new URLSearchParams();
        params.set('per_page', '20');
        params.set('page', String(page));
        if (f.query) params.set('q', f.query);
        if (f.category) params.set('category', f.category);
        if (f.tag) params.set('tag', f.tag);
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
        // 如果服务端回传的页码与请求不符（例如筛选后实际数据变少导致越界），
        // 跟随服务端纠正到合法页码，避免 UI 一直停留在不存在的页。
        if (data.page !== page) {
          setCurrentPage(data.page);
        }
      } catch (error) {
        console.error('Failed to load posts:', error);
      } finally {
        setLoading(false);
      }
    },
    [currentPage, filters],
  );

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

  // 仅在挂载时拉取一次数据；筛选/翻页/刷新都在对应的事件处理函数中显式触发。
  // `set-state-in-effect` is too strict for the standard mount-time
  // data-fetching pattern (no cascade — the effect deps are `[]`).
  useEffect(() => {
    // The rule is too strict for the standard mount-time data-fetching
    // pattern (no cascade because deps are `[]`).
    /* eslint-disable react-hooks/set-state-in-effect */
    loadPosts();
    loadFilters();
    /* eslint-enable react-hooks/set-state-in-effect */
    // loadPosts / loadFilters are stable references; safe to omit from deps.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 切换筛选条件时回到第 1 页，并显式触发一次加载（使用最新的 filter
  // 值）。这样既避免了 effect 内 setState 的级联渲染，也确保
  // loadPosts 不会拿到陈旧的闭包值。
  function updateFilter<K extends keyof typeof filters>(key: K, value: (typeof filters)[K]) {
    if (filters[key] === value) return;
    const next = { ...filters, [key]: value };
    setFilters(next);
    setCurrentPage(1);
    loadPosts(1, next);
  }

  function goToPage(page: number) {
    const total = pagination.total_pages || 1;
    const next = Math.max(1, Math.min(page, total));
    if (next !== currentPage) {
      setCurrentPage(next);
      loadPosts(next, filters);
    }
  }

  function prevPage() {
    goToPage(currentPage - 1);
  }

  function nextPage() {
    goToPage(currentPage + 1);
  }

  async function refreshCache() {
    setRefreshing(true);
    try {
      await post('/api/cache/refresh');
      await loadPosts();
      showNotification('缓存刷新成功', 'success');
    } catch {
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
    } catch {
      showNotification('创建失败', 'error');
    }
  }

  const socketRef = useSocket();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [importing, setImporting] = useState(false);

  function handleUploadClick() {
    fileInputRef.current?.click();
  }

  // 订阅本次导入的封面后台进度。导航到编辑器后组件卸载，订阅随之失效；
  // 若封面在停留期间完成则给出即时反馈。
  function subscribeCoverProgress(scope?: string) {
    const socket = socketRef.current;
    if (!socket) return;
    const onDone = (payload: { scope?: string; url?: string }) => {
      if (scope && payload.scope && payload.scope !== scope) return;
      showNotification('封面已生成', 'success');
      socket.off('article_import.cover_done', onDone);
      socket.off('article_import.cover_failed', onFail);
    };
    const onFail = (payload: { scope?: string; message?: string }) => {
      if (scope && payload.scope && payload.scope !== scope) return;
      showNotification('封面生成失败：' + (payload.message || ''), 'warning');
      socket.off('article_import.cover_done', onDone);
      socket.off('article_import.cover_failed', onFail);
    };
    socket.on('article_import.cover_done', onDone);
    socket.on('article_import.cover_failed', onFail);
  }

  async function handleFilePicked(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    // 重置 value，便于再次选择同一文件
    e.target.value = '';
    if (!file) return;

    if (!/\.(md|markdown)$/i.test(file.name)) {
      showNotification('请选择 .md / .markdown 文件', 'error');
      return;
    }

    setImporting(true);
    try {
      const data = await uploadMarkdown(file, { generate_cover: true });
      if (data.success && data.path) {
        await loadPosts();
        const hasWarnings = (data.warnings?.length ?? 0) > 0;
        if (hasWarnings) {
          showNotification('导入完成（部分步骤跳过）：' + (data.warnings ?? []).join('；'), 'warning');
        }
        if (data.cover_pending) {
          showNotification('文章已导入，封面后台生成中…', 'info');
          subscribeCoverProgress(data.event_scope);
        } else if (!hasWarnings) {
          showNotification('导入成功', 'success');
        }
        const path = data.path;
        setTimeout(() => navigate(`/editor/${path}`), 1200);
      } else {
        showNotification('导入失败: ' + (data.message || ''), 'error');
      }
    } catch {
      showNotification('导入失败', 'error');
    } finally {
      setImporting(false);
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
      const file_paths = Array.from(selectedPosts);
      const result = await post<{ success: boolean; results?: Array<{ path: string; success: boolean; message?: string }>; message?: string }>('/api/article/publish/bulk', { file_paths });
      if (result.success && result.results) {
        const successCount = result.results.filter((r) => r.success).length;
        showNotification(`批量发布完成：${successCount}/${file_paths.length} 成功`, 'success');
        await loadPosts();
        setSelectedPosts(new Set());
        setSelectAll(false);
      } else {
        showNotification('批量发布失败: ' + result.message, 'error');
      }
    } catch {
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
      <input
        type="file"
        accept=".md,.markdown"
        ref={fileInputRef}
        onChange={handleFilePicked}
        className="hidden"
      />
      {/* 搜索和筛选 */}
      <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-6 mb-6">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div className="md:col-span-2">
            <label className="block text-sm font-medium text-stone-700 mb-2">搜索</label>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-stone-400" />
              <input
                type="text"
                value={filters.query}
                onChange={(e) => updateFilter('query', e.target.value)}
                placeholder="搜索标题、内容、标签..."
                className="w-full pl-10 pr-4 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-stone-400 focus:border-transparent"
              />
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium text-stone-700 mb-2">分类</label>
            <select
              value={filters.category}
              onChange={(e) => updateFilter('category', e.target.value)}
              className="w-full px-4 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-stone-400"
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
            <label className="block text-sm font-medium text-stone-700 mb-2">标签</label>
            <select
              value={filters.tag}
              onChange={(e) => updateFilter('tag', e.target.value)}
              className="w-full px-4 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-stone-400"
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
      <div className="bg-white rounded-md ring-1 ring-stone-900/5">
        {loading ? (
          <div className="p-12 text-center">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600" />
            <p className="mt-4 text-stone-600">加载中...</p>
          </div>
        ) : (
          <div>
            <div className="px-6 py-4 border-b border-stone-200">
              <div className="flex items-center justify-between flex-wrap gap-2">
                <h3 className="text-lg font-medium">共 {pagination.total} 篇文章</h3>
                <div className="flex items-center gap-2">
                  <button
                    onClick={refreshCache}
                    disabled={refreshing}
                    className="px-4 py-2 border border-stone-300 text-stone-700 rounded-lg hover:bg-stone-50 transition-colors disabled:opacity-50 flex items-center"
                  >
                    <RefreshCw className={`w-4 h-4 mr-2 ${refreshing ? 'animate-spin' : ''}`} />
                    {refreshing ? '刷新中...' : '刷新缓存'}
                  </button>
                  <button
                    onClick={handleUploadClick}
                    disabled={importing}
                    className="px-4 py-2 border border-stone-300 text-stone-700 rounded-lg hover:bg-stone-50 transition-colors disabled:opacity-50 flex items-center"
                  >
                    <FileUp className={`w-4 h-4 mr-2 ${importing ? 'animate-spin' : ''}`} />
                    {importing ? '导入中...' : '上传 Markdown'}
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

            <div className="flex items-center gap-4 px-6 py-3 border-b border-stone-200">
              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="select-all"
                  checked={selectAll}
                  onChange={toggleSelectAll}
                  disabled={posts.length === 0}
                  className="w-4 h-4 border-stone-300 rounded text-blue-600 focus:ring-2 focus:ring-stone-400"
                />
                <label htmlFor="select-all" className="text-sm font-medium text-stone-700 cursor-pointer">
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

            <div className="divide-y divide-stone-200">
              {posts.length === 0 && (
                <div className="p-12 text-center text-stone-500">
                  <FileText className="w-16 h-16 mb-4 text-stone-400 mx-auto" />
                  <p>未找到文章</p>
                </div>
              )}
              {posts.map((post) => (
                <div key={post.path} className="p-6 hover:bg-stone-50 transition-colors">
                  <div className="flex items-start justify-between">
                    <div className="flex-shrink-0 mr-4 pt-1">
                      <input
                        type="checkbox"
                        checked={selectedPosts.has(post.path)}
                        onChange={() => togglePostSelection(post.path)}
                        className="w-4 h-4 border-stone-300 rounded text-blue-600 focus:ring-2 focus:ring-stone-400"
                      />
                    </div>
                    {post.cover_url && (
                      <Link to={`/editor/${post.path}`} className="flex-shrink-0 mr-4">
                        <img
                          src={post.cover_url}
                          alt={post.title}
                          className="w-24 h-16 object-cover rounded-lg bg-stone-100"
                          onError={(e) => (e.currentTarget.style.display = 'none')}
                        />
                      </Link>
                    )}
                    <div className="flex-1 min-w-0">
                      <Link to={`/editor/${post.path}`} className="block">
                        <h4 className="text-lg font-medium text-stone-900 hover:text-blue-600 mb-2">{post.title}</h4>
                      </Link>
                      <p className="text-stone-600 text-sm mb-3 line-clamp-2">{post.description || post.excerpt}</p>
                      <div className="flex flex-wrap items-center gap-4 text-sm text-stone-500">
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
                            <span key={tag} className="inline-flex items-center px-2 py-0.5 rounded text-xs bg-stone-100 text-stone-600">
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

      {/*
        控件本身的可见性：当没有任何文章时隐藏分页栏（避免显示"第 1 页 / 共 0 页"）。
        加载态下保留分页栏位，但禁用按钮，避免布局跳动。
      */}
      {pagination.total > 0 && (
        <nav
          aria-label="分页"
          className="bg-white rounded-md ring-1 ring-stone-900/5 px-6 py-3 mt-6 flex items-center justify-between flex-wrap gap-3"
        >
          <div className="text-sm text-stone-500">
            第 <span className="font-medium text-stone-700">{pagination.page}</span> / {pagination.total_pages} 页
            <span className="mx-2 text-stone-300">·</span>
            共 {pagination.total} 篇
          </div>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={prevPage}
              disabled={!pagination.has_prev || loading}
              className="px-3 py-1.5 border border-stone-300 text-stone-700 rounded-lg hover:bg-stone-50 transition-colors disabled:opacity-40 disabled:cursor-not-allowed flex items-center"
              aria-label="上一页"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            {getPageNumbers(pagination.page, pagination.total_pages).map((item, idx) =>
              item === '…' ? (
                <span
                  key={`ellipsis-${idx}`}
                  className="px-2 text-stone-400 select-none"
                  aria-hidden="true"
                >
                  …
                </span>
              ) : (
                <button
                  key={item}
                  type="button"
                  onClick={() => goToPage(item)}
                  disabled={loading}
                  aria-current={item === pagination.page ? 'page' : undefined}
                  className={
                    item === pagination.page
                      ? 'px-3 py-1.5 rounded-lg bg-blue-600 text-white font-medium cursor-default'
                      : 'px-3 py-1.5 border border-stone-300 text-stone-700 rounded-lg hover:bg-stone-50 transition-colors disabled:opacity-50'
                  }
                >
                  {item}
                </button>
              ),
            )}
            <button
              type="button"
              onClick={nextPage}
              disabled={!pagination.has_next || loading}
              className="px-3 py-1.5 border border-stone-300 text-stone-700 rounded-lg hover:bg-stone-50 transition-colors disabled:opacity-40 disabled:cursor-not-allowed flex items-center"
              aria-label="下一页"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </nav>
      )}
    </div>
  );
}

function getPageNumbers(current: number, total: number): (number | '…')[] {
  // 页数较少时直接展开；否则用 1 … current-1 current current+1 … last 的折叠形态。
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1);
  }
  const pages: (number | '…')[] = [1];
  const left = Math.max(2, current - 1);
  const right = Math.min(total - 1, current + 1);
  if (left > 2) pages.push('…');
  for (let p = left; p <= right; p++) pages.push(p);
  if (right < total - 1) pages.push('…');
  pages.push(total);
  return pages;
}
