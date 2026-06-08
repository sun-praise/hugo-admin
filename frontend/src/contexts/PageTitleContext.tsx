import { createContext, useContext, useState, useCallback, type ReactNode } from 'react';

const PageTitleContext = createContext<{
  title: string;
  setTitle: (title: string) => void;
  resetTitle: () => void;
}>({ title: '', setTitle: () => {}, resetTitle: () => {} });

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

export function usePageTitle() {
  return useContext(PageTitleContext);
}
