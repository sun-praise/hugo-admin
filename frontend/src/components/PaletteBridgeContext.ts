import { createContext } from 'react';

/** 面板文章条目的最小字段(与 /api/posts/search 返回一致) */
export interface PalettePost {
  path: string;
  title: string;
}

/** 用对象包一层,避免函数直接作为 setState 参数被当成 updater */
export interface PaletteInsertHandler {
  insert: (post: PalettePost) => void;
}

/**
 * Layout(⌘K 面板属主)与编辑器之间的桥:
 * 编辑器挂载后注册插入回调,面板即进入"插入引用"模式——
 * 回车不再跳转编辑页,而是把文章引用插到光标处;
 * open() 供编辑器工具栏"文章引用"按钮复用同一个面板。
 */
export interface PaletteBridge {
  setInsertHandler: (handler: PaletteInsertHandler | null) => void;
  open: () => void;
}

export const PaletteBridgeContext = createContext<PaletteBridge>({
  setInsertHandler: () => {},
  open: () => {},
});
