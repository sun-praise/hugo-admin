import { useEffect, useState } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { getMe } from '../utils/api';

type State = 'loading' | 'ok' | 'out';

/**
 * 保护应用路由：挂载时校验会话，未登录则跳转 /login。
 * 会话过期的在途请求由 utils/api.ts 的 401 拦截器统一处理。
 */
export default function RequireAuth() {
  const [state, setState] = useState<State>('loading');
  const location = useLocation();

  useEffect(() => {
    let alive = true;
    getMe()
      .then((r) => {
        if (!alive) return;
        setState(r.success && r.user ? 'ok' : 'out');
      })
      .catch(() => alive && setState('out'));
    return () => {
      alive = false;
    };
  }, []);

  if (state === 'loading') {
    return (
      <div className="flex h-screen items-center justify-center text-stone-500">
        加载中…
      </div>
    );
  }

  if (state === 'out') {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return <Outlet />;
}
