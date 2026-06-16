import { createContext, useContext } from 'react';

export const PageTitleContext = createContext<{
  title: string;
  setTitle: (title: string) => void;
  resetTitle: () => void;
}>({ title: '', setTitle: () => {}, resetTitle: () => {} });

export function usePageTitle() {
  return useContext(PageTitleContext);
}
