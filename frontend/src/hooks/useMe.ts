import { useEffect, useState } from 'react';

import { get } from '../utils/api';

/** 当前登录用户(/api/auth/me),未登录或失败时为 null。 */
export function useMe(): { username: string } | null {
  const [me, setMe] = useState<{ username: string } | null>(null);
  useEffect(() => {
    let alive = true;
    get<{ success: boolean; user?: { username: string } }>('/api/auth/me')
      .then((d) => {
        if (alive && d.user) setMe(d.user);
      })
      .catch(() => {
        /* 未登录时静默 */
      });
    return () => {
      alive = false;
    };
  }, []);
  return me;
}
