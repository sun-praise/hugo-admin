import { useState, useCallback, type ReactNode } from 'react';
import { PageTitleContext } from '../hooks/usePageTitle';

export function PageTitleProvider({ children }: { children: ReactNode }) {
  const [title, setTitleState] = useState('');
  const setTitle = useCallback((t: string) => setTitleState(t), []);
  const resetTitle = useCallback(() => setTitleState(''), []);

  return (
    <PageTitleContext.Provider value={{ title, setTitle, resetTitle }}>
      {children}
    </PageTitleContext.Provider>
  );
}
