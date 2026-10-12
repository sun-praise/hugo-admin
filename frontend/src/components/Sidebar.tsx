import { useEffect, useState } from 'react';
import { NavLink, useNavigate } from 'react-router-dom';
import {
  Home,
  FileText,
  Edit3,
  Server,
  GitBranch,
  Puzzle,
  Settings,
  FlaskConical,
  LogOut,
} from 'lucide-react';
import { get, logout } from '../utils/api';
import { useMe } from '../hooks/useMe';

interface NavItem {
  to: string;
  icon: typeof Home;
  label: string;
  end?: boolean;
}

const NAV_GROUPS: { label: string; items: NavItem[] }[] = [
  {
    label: '工作台',
    items: [
      { to: '/', icon: Home, label: '仪表板', end: true },
      { to: '/posts', icon: FileText, label: '文章管理' },
      { to: '/editor', icon: Edit3, label: '编辑器' },
    ],
  },
  {
    label: '系统',
    items: [
      { to: '/server', icon: Server, label: '服务器控制' },
      { to: '/git', icon: GitBranch, label: 'Git' },
      { to: '/plugins', icon: Puzzle, label: '插件' },
      { to: '/settings', icon: Settings, label: '设置' },
      { to: '/playground', icon: FlaskConical, label: '演示场' },
    ],
  },
];

// 对齐重设计原型:浅色侧边栏,232px;窄屏(<860px)自动折叠为图标栏。
export default function Sidebar() {
  const navigate = useNavigate();
  const me = useMe();
  const [postCount, setPostCount] = useState<number | null>(null);

  useEffect(() => {
    let alive = true;
    get<{ total: number }>('/api/posts?per_page=1')
      .then((d) => {
        if (alive) setPostCount(d.total);
      })
      .catch(() => {
        /* 计数失败不阻塞导航 */
      });
    return () => {
      alive = false;
    };
  }, []);

  async function handleLogout() {
    try {
      await logout();
    } catch {
      // 忽略：无论如何都跳转登录页
    }
    navigate('/login', { replace: true });
  }

  const initial = (me?.username || 'U').slice(0, 1).toUpperCase();

  return (
    <aside className="sticky top-0 flex h-screen w-[232px] flex-col border-r border-[#e4e4e7] bg-white px-3 pb-3 pt-4 max-[860px]:w-[60px] max-[860px]:px-2.5">
      <div className="flex items-center gap-2.5 px-2 pb-[18px] max-[860px]:justify-center max-[860px]:px-0">
        <div className="grid h-7 w-7 flex-none place-items-center rounded-lg bg-zinc-900 text-[14px] font-bold text-white">
          H
        </div>
        <div className="min-w-0 leading-[1.3] max-[860px]:hidden">
          <strong className="block text-[14px] tracking-[-0.01em]">Hugo Blog</strong>
          <span className="block text-[12px] text-[#9ca3a8]">管理控制台</span>
        </div>
      </div>

      <nav aria-label="主导航" className="flex flex-col gap-0.5">
        {NAV_GROUPS.map((group) => (
          <div key={group.label}>
            <div className="px-2.5 pb-1.5 pt-3 text-[11px] font-semibold uppercase tracking-[0.06em] text-[#9ca3a8] max-[860px]:hidden">
              {group.label}
            </div>
            {group.items.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                title={item.label}
                className={({ isActive }) =>
                  `flex h-[34px] items-center gap-2.5 rounded-md px-2.5 text-[13.5px] font-medium transition-colors max-[860px]:justify-center max-[860px]:px-0 ${
                    isActive
                      ? 'bg-[#ebebeb] text-zinc-900 [&_svg]:text-zinc-900'
                      : 'text-[#52525b] hover:bg-[#f4f4f5] hover:text-zinc-900'
                  }`
                }
              >
                <item.icon className="h-4 w-4 flex-none text-[#9ca3a8]" />
                <span className="max-[860px]:hidden">{item.label}</span>
                {item.to === '/posts' && postCount !== null && (
                  <span className="ml-auto font-mono text-[11px] font-medium text-[#9ca3a8] max-[860px]:hidden">
                    {postCount}
                  </span>
                )}
              </NavLink>
            ))}
          </div>
        ))}
      </nav>

      <div className="mt-auto border-t border-[#e4e4e7] pt-3">
        <div className="flex items-center gap-2.5 px-2 py-1.5 max-[860px]:justify-center max-[860px]:px-0">
          <div className="grid h-7 w-7 flex-none place-items-center rounded-full bg-gradient-to-br from-blue-600 to-violet-600 text-[12px] font-semibold text-white">
            {initial}
          </div>
          <div className="min-w-0 leading-[1.3] max-[860px]:hidden">
            <strong className="block text-[13px]">{me?.username || '…'}</strong>
            <span className="block text-[11.5px] text-[#9ca3a8]">管理员</span>
          </div>
          <button
            onClick={handleLogout}
            title="退出登录"
            aria-label="退出登录"
            className="ml-auto grid h-[30px] w-[30px] cursor-pointer place-items-center rounded-md border-0 bg-transparent text-[#9ca3a8] hover:bg-[#f4f4f5] hover:text-zinc-900 max-[860px]:hidden"
          >
            <LogOut className="h-[15px] w-[15px]" />
          </button>
        </div>
      </div>
    </aside>
  );
}
