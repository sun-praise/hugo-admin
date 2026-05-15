import { useState, useEffect } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { FileText, Tag, FolderOpen, Server, Search, Plus, Settings, Upload, Mail, Send } from 'lucide-react';
import { get, post } from '../utils/api';
import type { Post, PostsResponse, Tag as TagType, Category, ServerStatus } from '../types';

export default function Dashboard() {
  const navigate = useNavigate();
  const [stats, setStats] = useState({ total_posts: 0, total_tags: 0, total_categories: 0 });
  const [serverStatus, setServerStatus] = useState<ServerStatus>({ running: false, pid: null });
  const [recentPosts, setRecentPosts] = useState<Post[]>([]);
  const [publishing, setPublishing] = useState(false);
  const [pushingEmail, setPushingEmail] = useState(false);

  useEffect(() => {
    loadStats();
    loadRecentPosts();
    loadServerStatus();
  }, []);

  async function loadStats() {
    try {
      const posts = await get<PostsResponse>('/api/posts?per_page=1000');
      const tags = await get<{ tags: TagType[] }>('/api/posts/tags');
      const categories = await get<{ categories: Category[] }>('/api/posts/categories');
      setStats({
        total_posts: posts.total,
        total_tags: tags.tags.length,
        total_categories: categories.categories.length,
      });
    } catch (error) {
      console.error('Failed to load stats:', error);
    }
  }

  async function loadRecentPosts() {
    try {
      const data = await get<PostsResponse>('/api/posts?per_page=5');
      setRecentPosts(data.posts);
    } catch (error) {
      console.error('Failed to load recent posts:', error);
    }
  }

  async function loadServerStatus() {
    try {
      const data = await get<ServerStatus>('/api/server/status');
      setServerStatus(data);
    } catch (error) {
      console.error('Failed to load server status:', error);
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

  async function publishSystem() {
    setPublishing(true);
    try {
      const status = await get<{ success: boolean; has_changes?: boolean; message?: string }>('/api/git/status');
      if (!status.success) {
        showNotification('Git 状态检查失败: ' + status.message, 'error');
        return;
      }
      if (!status.has_changes) {
        showNotification('没有需要发布的改动', 'info');
        return;
      }
      const message = prompt('请输入提交消息 (可选，留空使用默认消息):');
      if (message === null) return;
      const result = await post<{ success: boolean; message?: string }>('/api/publish/system', { message: message || null });
      if (result.success) {
        showNotification('系统发布成功！GitHub Actions 将自动构建站点', 'success');
      } else {
        showNotification('发布失败: ' + result.message, 'error');
      }
    } catch (error) {
      showNotification('系统发布失败', 'error');
    } finally {
      setPublishing(false);
    }
  }

  async function pushLatestEmail() {
    setPushingEmail(true);
    try {
      const preview = await get<{ success: boolean; data?: { is_already_sent: boolean; post: { title: string; link: string }; subject: string }; message?: string }>('/api/email/preview-latest');
      if (!preview.success) {
        showNotification('获取预览失败: ' + preview.message, 'error');
        return;
      }
      const p = preview.data!;
      const alreadySent = p.is_already_sent ? '（该文章已推送过）' : '';
      const confirmed = confirm(
        `确定要推送最新文章给订阅者吗？${alreadySent}\n\n标题：${p.post.title}\n链接：${p.post.link}\n\n邮件主题：${p.subject}`,
      );
      if (!confirmed) return;
      const force = p.is_already_sent ? confirm('该文章已推送过，是否强制再次推送？') : false;
      const result = await post<{ success: boolean; message?: string }>('/api/email/push-latest', { force, debug_mode: false });
      if (result.success) {
        showNotification('邮件推送成功！', 'success');
      } else {
        showNotification('推送失败: ' + result.message, 'error');
      }
    } catch (error) {
      showNotification('邮件推送失败', 'error');
    } finally {
      setPushingEmail(false);
    }
  }

  async function pushSpecificEmail() {
    const url = prompt('请输入要推送的文章 URL 或路径：\n\n例如：\n- https://svtter.cn/post/xxx/\n- /post/xxx/');
    if (!url) return;
    setPushingEmail(true);
    try {
      const preview = await get<{ success: boolean; data?: { is_already_sent: boolean; post: { title: string; link: string }; subject: string }; message?: string }>(
        '/api/email/preview-article?url=' + encodeURIComponent(url),
      );
      if (!preview.success) {
        showNotification('获取预览失败: ' + preview.message, 'error');
        return;
      }
      const p = preview.data!;
      const alreadySent = p.is_already_sent ? '（该文章已推送过）' : '';
      const confirmed = confirm(
        `确定要推送这篇文章给订阅者吗？${alreadySent}\n\n标题：${p.post.title}\n链接：${p.post.link}\n\n邮件主题：${p.subject}`,
      );
      if (!confirmed) return;
      const force = p.is_already_sent ? confirm('该文章已推送过，是否强制再次推送？') : false;
      const result = await post<{ success: boolean; message?: string }>('/api/email/push-article', { url, force, debug_mode: false });
      if (result.success) {
        showNotification('邮件推送成功！', 'success');
      } else {
        showNotification('推送失败: ' + result.message, 'error');
      }
    } catch (error) {
      showNotification('邮件推送失败', 'error');
    } finally {
      setPushingEmail(false);
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

  const statCards = [
    { label: '总文章数', value: stats.total_posts, icon: FileText, color: 'blue' },
    { label: '标签数', value: stats.total_tags, icon: Tag, color: 'green' },
    { label: '分类数', value: stats.total_categories, icon: FolderOpen, color: 'purple' },
    {
      label: '服务器状态',
      value: serverStatus.running ? '运行中' : '已停止',
      icon: Server,
      color: serverStatus.running ? 'green' : 'gray',
      isStatus: true,
    },
  ];

  const colorMap: Record<string, { bg: string; text: string }> = {
    blue: { bg: 'bg-blue-100', text: 'text-blue-600' },
    green: { bg: 'bg-green-100', text: 'text-green-600' },
    purple: { bg: 'bg-purple-100', text: 'text-purple-600' },
    gray: { bg: 'bg-gray-100', text: 'text-gray-400' },
  };

  return (
    <div>
      {/* 统计卡片 */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
        {statCards.map((card) => (
          <div key={card.label} className="bg-white rounded-lg shadow p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-gray-500 text-sm">{card.label}</p>
                <p className={`text-3xl font-bold mt-2 ${card.isStatus && serverStatus.running ? 'text-green-600' : card.isStatus ? 'text-gray-400' : ''}`}>
                  {card.value}
                </p>
              </div>
              <div className={`p-3 rounded-full ${colorMap[card.color].bg}`}>
                <card.icon className={`w-8 h-8 ${colorMap[card.color].text}`} />
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* 快速操作 */}
      <div className="bg-white rounded-lg shadow p-6 mb-8">
        <h3 className="text-lg font-semibold mb-4">快速操作</h3>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          <Link to="/posts" className="flex items-center justify-center p-4 border-2 border-gray-200 rounded-lg hover:border-blue-500 hover:bg-blue-50 transition-colors">
            <Search className="w-6 h-6 text-blue-600 mr-3" />
            <span className="font-medium">浏览文章</span>
          </Link>
          <button onClick={createNewPost} className="flex items-center justify-center p-4 border-2 border-gray-200 rounded-lg hover:border-green-500 hover:bg-green-50 transition-colors">
            <Plus className="w-6 h-6 text-green-600 mr-3" />
            <span className="font-medium">新建文章</span>
          </button>
          <Link to="/server" className="flex items-center justify-center p-4 border-2 border-gray-200 rounded-lg hover:border-purple-500 hover:bg-purple-50 transition-colors">
            <Settings className="w-6 h-6 text-purple-600 mr-3" />
            <span className="font-medium">服务器控制</span>
          </Link>
          <button onClick={publishSystem} disabled={publishing} className="flex items-center justify-center p-4 border-2 border-gray-200 rounded-lg hover:border-orange-500 hover:bg-orange-50 transition-colors disabled:opacity-50">
            <Upload className="w-6 h-6 text-orange-600 mr-3" />
            <span className="font-medium">{publishing ? '发布中...' : '系统发布'}</span>
          </button>
          <button onClick={pushLatestEmail} disabled={pushingEmail} className="flex items-center justify-center p-4 border-2 border-gray-200 rounded-lg hover:border-indigo-500 hover:bg-indigo-50 transition-colors disabled:opacity-50">
            <Mail className="w-6 h-6 text-indigo-600 mr-3" />
            <span className="font-medium">{pushingEmail ? '推送中...' : '推送最新文章'}</span>
          </button>
          <button onClick={pushSpecificEmail} disabled={pushingEmail} className="flex items-center justify-center p-4 border-2 border-gray-200 rounded-lg hover:border-teal-500 hover:bg-teal-50 transition-colors disabled:opacity-50">
            <Send className="w-6 h-6 text-teal-600 mr-3" />
            <span className="font-medium">推送指定文章</span>
          </button>
        </div>
      </div>

      {/* 最近文章 */}
      <div className="bg-white rounded-lg shadow p-6">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold">最近文章</h3>
          <Link to="/posts" className="text-blue-600 hover:text-blue-800 text-sm">查看全部 →</Link>
        </div>
        <div className="space-y-3">
          {recentPosts.length === 0 && <p className="text-gray-500 text-center py-8">暂无文章</p>}
          {recentPosts.map((post) => (
            <div key={post.path} className="border-b border-gray-200 pb-3 last:border-b-0">
              <Link to={`/editor/${post.path}`} className="flex hover:bg-gray-50 rounded p-2 -m-2">
                {post.cover_url && (
                  <img
                    src={post.cover_url}
                    alt={post.title}
                    className="w-16 h-12 object-cover rounded mr-3 flex-shrink-0 bg-gray-100"
                    onError={(e) => (e.currentTarget.style.display = 'none')}
                  />
                )}
                <div className="flex-1 min-w-0">
                  <h4 className="font-medium text-gray-900 mb-1 truncate">{post.title}</h4>
                  <p className="text-sm text-gray-600 mb-1 line-clamp-1">{post.excerpt}</p>
                  <div className="flex items-center space-x-4 text-xs text-gray-500">
                    <span>{post.date}</span>
                    <span className="flex items-center space-x-1">
                      <Tag className="w-4 h-4" />
                      <span>{post.tags.join(', ')}</span>
                    </span>
                  </div>
                </div>
              </Link>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
