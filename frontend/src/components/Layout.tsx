import { lazy, Suspense, useState } from 'react';
import { Outlet } from 'react-router-dom';
import Sidebar from './Sidebar';
import Loading from './Loading';
import Header from './Header';

// AIChat pulls in the markdown renderer (highlight.js) — load it as a lazy
// chunk so it stays out of the entry bundle; the floating widget mounts a
// beat after the shell without blocking first paint.
const AIChat = lazy(() => import('./AIChat'));

import { PageTitleProvider } from '../contexts/PageTitleContext';

export default function Layout() {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  return (
    <PageTitleProvider>
      <div className="flex h-screen overflow-hidden">
        <Sidebar collapsed={sidebarCollapsed} onToggle={() => setSidebarCollapsed(!sidebarCollapsed)} />
        <main className="flex-1 overflow-auto bg-stone-50">
          <Header />
          <div className="p-6">
            <Suspense fallback={<Loading />}>
              <Outlet />
            </Suspense>
          </div>
        </main>
        <Suspense fallback={null}>
          <AIChat />
        </Suspense>
      </div>
    </PageTitleProvider>
  );
}
