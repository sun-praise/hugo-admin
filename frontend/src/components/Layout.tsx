import { lazy, Suspense, useCallback, useEffect, useState } from 'react';
import { Outlet, useNavigate } from 'react-router-dom';
import Sidebar from './Sidebar';
import Topbar from './Topbar';
import CommandPalette from './CommandPalette';
import Loading from './Loading';

// AIChat pulls in the markdown renderer (highlight.js) — load it as a lazy
// chunk so it stays out of the entry bundle; the floating widget mounts a
// beat after the shell without blocking first paint.
const AIChat = lazy(() => import('./AIChat'));

import { createNewPost } from '../utils/actions';

export default function Layout() {
  const navigate = useNavigate();
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);

  const openPalette = useCallback(() => setPaletteOpen(true), []);
  const closePalette = useCallback(() => setPaletteOpen(false), []);
  const closeSidebar = useCallback(() => setSidebarOpen(false), []);

  // 全局快捷键:⌘K/Ctrl+K 开关面板;N 新建文章(原型键盘约定);Esc 收起移动端抽屉
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      } else if (
        !e.metaKey &&
        !e.ctrlKey &&
        !e.altKey &&
        e.key.toLowerCase() === 'n' &&
        document.activeElement === document.body
      ) {
        void createNewPost(navigate);
      } else if (e.key === 'Escape') {
        setSidebarOpen(false);
      }
    }
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [navigate]);

  // 抽屉打开时锁住背景滚动,避免下层页面跟着滑动
  useEffect(() => {
    if (!sidebarOpen) return;
    const prev = document.documentElement.style.overflow;
    document.documentElement.style.overflow = 'hidden';
    return () => {
      document.documentElement.style.overflow = prev;
    };
  }, [sidebarOpen]);

  return (
    <div className="grid min-h-screen grid-cols-[232px_minmax(0,1fr)] max-[860px]:block">
      <Sidebar open={sidebarOpen} onClose={closeSidebar} />
      {sidebarOpen && (
        <div
          className="hidden fixed inset-0 z-40 bg-zinc-900/40 max-[860px]:block"
          onClick={closeSidebar}
          aria-hidden="true"
        />
      )}
      <div className="flex min-w-0 flex-col">
        <Topbar onOpenPalette={openPalette} onOpenSidebar={() => setSidebarOpen(true)} />
        <main className="w-full max-w-[1760px] flex-1 px-8 pb-14 pt-7 max-[860px]:px-4 max-[860px]:pb-10 max-[860px]:pt-4">
          <Suspense fallback={<Loading />}>
            <Outlet />
          </Suspense>
        </main>
      </div>
      <Suspense fallback={null}>
        <AIChat />
      </Suspense>
      <CommandPaletteWrapper open={paletteOpen} onClose={closePalette} />
    </div>
  );
}

// 条件挂载:关闭即卸载,面板状态天然重置
function CommandPaletteWrapper({ open, onClose }: { open: boolean; onClose: () => void }) {
  return open ? <CommandPalette onClose={onClose} /> : null;
}
