import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Plus, Menu, Search } from 'lucide-react';

import { get } from '../utils/api';
import { createNewPost } from '../utils/actions';
import type { ServerStatus } from '../types';

// 顶栏:⌘K 搜索入口 + Hugo 状态胶囊 + 新建文章(对齐重设计原型)。
// 窄屏(<860px):汉堡按钮唤出侧边抽屉,搜索收缩为图标钮。
export default function Topbar({
  onOpenPalette,
  onOpenSidebar,
}: {
  onOpenPalette: () => void;
  onOpenSidebar: () => void;
}) {
  const navigate = useNavigate();
  const [status, setStatus] = useState<ServerStatus>({ running: false, pid: null });
  const isMac = typeof navigator !== 'undefined' && /mac/i.test(navigator.platform);

  useEffect(() => {
    let alive = true;
    async function fetchStatus() {
      try {
        const data = await get<ServerStatus>('/api/server/status');
        if (alive) setStatus(data);
      } catch {
        /* 状态拉取失败保持旧值 */
      }
    }
    fetchStatus();
    const interval = setInterval(fetchStatus, 5000);
    return () => {
      alive = false;
      clearInterval(interval);
    };
  }, []);

  return (
    <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b border-[#e4e4e7] bg-[#fafafa]/85 px-7 backdrop-blur max-[860px]:gap-2 max-[860px]:px-4">
      <button
        onClick={onOpenSidebar}
        aria-label="打开导航菜单"
        className="hidden h-9 w-9 flex-none cursor-pointer items-center justify-center rounded-lg border border-[#e4e4e7] bg-white text-[#52525b] transition-colors hover:bg-[#f4f4f5] max-[860px]:flex"
      >
        <Menu className="h-[17px] w-[17px]" />
      </button>

      <button
        onClick={onOpenPalette}
        aria-label="打开搜索"
        className="flex h-8 w-[clamp(240px,30vw,420px)] cursor-pointer items-center gap-2 rounded-lg border border-[#e4e4e7] bg-white pl-2.5 pr-2 text-[13px] text-[#9ca3a8] transition-colors hover:border-[#d4d4d8] max-[860px]:h-9 max-[860px]:w-9 max-[860px]:flex-none max-[860px]:justify-center max-[860px]:px-0"
      >
        <Search className="h-[15px] w-[15px] flex-none" />
        <span className="overflow-hidden text-ellipsis whitespace-nowrap max-[860px]:hidden">
          搜索文章、跳转页面、执行操作…
        </span>
        <kbd className="kbd ml-auto max-[860px]:hidden">{isMac ? '⌘' : 'Ctrl'}K</kbd>
      </button>

      <div className="ml-auto flex items-center gap-2.5 max-[860px]:gap-2">
        <div
          className="inline-flex h-[30px] items-center gap-2 rounded-full border border-[#e4e4e7] bg-white px-3 text-[12.5px] font-medium text-[#52525b] max-[860px]:px-2.5"
          title="Hugo 开发服务器状态"
        >
          <span className={`dot-pulse ${status.running ? '' : 'is-off'}`} aria-hidden="true" />
          <span className="max-[430px]:hidden">{status.running ? 'Hugo 运行中' : 'Hugo 已停止'}</span>
          {status.pid != null && (
            <span className="font-mono text-[11px] font-normal text-[#9ca3a8] max-[860px]:hidden">
              PID {status.pid}
            </span>
          )}
        </div>
        <button
          onClick={() => void createNewPost(navigate)}
          className="inline-flex h-8 cursor-pointer items-center gap-[7px] rounded-lg border-0 bg-zinc-900 pl-3 pr-3 text-[13px] font-medium text-white transition-colors hover:bg-[#2e2e33] max-[860px]:h-9 max-[860px]:pl-2.5 max-[860px]:pr-2.5"
        >
          <Plus className="h-[15px] w-[15px]" />
          新建文章
          <kbd className="kbd !border-white/25 !bg-white/[0.08] !text-white/[0.72] max-[860px]:hidden">N</kbd>
        </button>
      </div>
    </header>
  );
}
