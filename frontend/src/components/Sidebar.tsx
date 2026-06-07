import { NavLink } from 'react-router-dom';
import {
  Home,
  FileText,
  Edit3,
  Server,
  Settings,
  Puzzle,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react';

const navItems = [
  { to: '/', icon: Home, label: '仪表板' },
  { to: '/posts', icon: FileText, label: '文章管理' },
  { to: '/editor', icon: Edit3, label: '编辑器' },
  { to: '/server', icon: Server, label: '服务器控制' },
  { to: '/plugins', icon: Puzzle, label: '插件' },
  { to: '/settings', icon: Settings, label: '设置' },
];

interface SidebarProps {
  collapsed: boolean;
  onToggle: () => void;
}

export default function Sidebar({ collapsed, onToggle }: SidebarProps) {
  return (
    <aside
      className={`${
        collapsed ? 'w-16' : 'w-64'
      } bg-stone-900 text-stone-300 flex flex-col flex-shrink-0 min-h-screen transition-all duration-200`}
    >
      <div className={`border-b border-stone-700 ${collapsed ? 'p-3' : 'p-6'}`}>
        <div className="flex items-center justify-between">
          {collapsed ? (
            <h1 className="text-lg font-bold text-stone-300">H</h1>
          ) : (
            <div>
              <h1 className="text-2xl font-bold text-stone-300">Hugo Blog</h1>
              <p className="text-stone-500 text-sm">管理界面</p>
            </div>
          )}
          <button
            onClick={onToggle}
            className="p-1.5 rounded-lg text-stone-500 hover:text-stone-200 hover:bg-stone-800 transition-colors"
            title={collapsed ? '展开侧边栏' : '收起侧边栏'}
          >
            {collapsed ? <ChevronRight className="w-4 h-4" /> : <ChevronLeft className="w-4 h-4" />}
          </button>
        </div>
      </div>

      <nav className="flex-1 p-2 space-y-1">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            title={collapsed ? item.label : undefined}
            className={({ isActive }) =>
              `flex items-center gap-3 px-3 py-2.5 rounded-lg transition-all text-[15px] ${
                isActive
                  ? 'bg-stone-800 text-stone-100'
                  : 'text-stone-400 hover:bg-stone-800 hover:text-stone-200'
              }`
            }
          >
            <item.icon className="w-5 h-5 flex-shrink-0" />
            {!collapsed && <span>{item.label}</span>}
          </NavLink>
        ))}
      </nav>

      <div className="p-2 border-t border-stone-700">
        <div className={`flex items-center ${collapsed ? 'justify-center' : 'justify-between'} px-1 py-1`}>
          {!collapsed && <p className="text-xs text-stone-500">Hugo Blog Admin</p>}
          <a
            href="https://github.com/Svtter/hugo-admin"
            target="_blank"
            rel="noopener noreferrer"
            className="text-stone-400 hover:text-stone-200 transition-colors"
          >
            <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
              <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" />
            </svg>
          </a>
        </div>
      </div>
    </aside>
  );
}
