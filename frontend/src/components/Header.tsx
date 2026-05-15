import { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { get } from '../utils/api';
import type { ServerStatus } from '../types';

const pageTitles: Record<string, string> = {
  '/': '仪表板',
  '/posts': '文章管理',
  '/editor': '文章编辑器',
  '/server': 'Hugo 服务器控制',
  '/settings': '设置',
};

export default function Header() {
  const location = useLocation();
  const [status, setStatus] = useState<ServerStatus>({ running: false, pid: null });

  const pageTitle = pageTitles[location.pathname] || 'Hugo Blog 管理';

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
    <header className="bg-white shadow-sm border-b border-gray-200 px-6 py-4">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-semibold text-gray-800">{pageTitle}</h2>
        <div className="flex items-center space-x-4">
          <div className="flex items-center space-x-2">
            <div
              className={`w-3 h-3 rounded-full animate-pulse ${
                status.running ? 'bg-green-500' : 'bg-gray-400'
              }`}
            />
            <span className="text-sm text-gray-600">
              {status.running ? 'Hugo 运行中' : 'Hugo 已停止'}
            </span>
          </div>
        </div>
      </div>
    </header>
  );
}
