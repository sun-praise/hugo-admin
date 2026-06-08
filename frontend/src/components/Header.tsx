import { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { get } from '../utils/api';
import { usePageTitle } from '../contexts/PageTitleContext';
import type { ServerStatus } from '../types';

const pageTitles: Record<string, string> = {
  '/': '仪表板',
  '/posts': '文章管理',
  '/editor': '文章编辑器',
  '/server': 'Hugo 服务器控制',
  '/settings': '设置',
};

function matchPageTitle(pathname: string): string | undefined {
  if (pageTitles[pathname]) return pageTitles[pathname];
  for (const [path, title] of Object.entries(pageTitles)) {
    if (pathname.startsWith(path + '/')) return title;
  }
  return undefined;
}

export default function Header() {
  const location = useLocation();
  const [status, setStatus] = useState<ServerStatus>({ running: false, pid: null });
  const { title: articleTitle } = usePageTitle();

  const matchedTitle = matchPageTitle(location.pathname);

  useEffect(() => {
    async function fetchStatus() {
      try {
        const data = await get<ServerStatus>('/api/server/status');
        setStatus(data);
      } catch (error) {
        console.error('Failed to fetch server status:', error);
      }
    }
    fetchStatus();
    const interval = setInterval(fetchStatus, 5000);
    return () => clearInterval(interval);
  }, []);

  return (
    <header className="bg-white shadow-sm border-b border-stone-200 px-6 py-4">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-semibold text-stone-800">{articleTitle || matchedTitle}</h2>
        <div className="flex items-center space-x-4">
          <div className="flex items-center space-x-2">
            <div
              className={`w-3 h-3 rounded-full animate-pulse ${
                status.running ? 'bg-green-500' : 'bg-stone-400'
              }`}
            />
            <span className="text-sm text-stone-600">
              {status.running ? 'Hugo 运行中' : 'Hugo 已停止'}
            </span>
          </div>
        </div>
      </div>
    </header>
  );
}
