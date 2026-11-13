export const PREVIEW_COLLAPSED_KEY = 'editor-preview-collapsed';

export type MobilePane = 'edit' | 'preview';

export interface StorageLike {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

/** 隐私模式 / 沙箱 iframe 下访问 window.localStorage 本身就可能抛错。 */
export function getLocalStorage(): StorageLike | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

export function readPreviewCollapsed(storage: StorageLike | null): boolean {
  try {
    return storage?.getItem(PREVIEW_COLLAPSED_KEY) === '1';
  } catch {
    return false;
  }
}

export function writePreviewCollapsed(storage: StorageLike | null, collapsed: boolean): void {
  try {
    storage?.setItem(PREVIEW_COLLAPSED_KEY, collapsed ? '1' : '0');
  } catch {
    // localStorage 不可用时仅本次会话生效
  }
}

/**
 * 两栏的 display 类。收起时预览用 `hidden` 而不是卸载 DOM——mermaid 直接改写
 * 预览里的节点，卸载会丢掉已经画好的图表，且内容不变时渲染 effect 不会重跑。
 * 窄屏（<lg）靠页签切换，未选中预览页签时隐藏预览；lg+ 双栏始终都显示。
 *
 * 这些类名必须写成字面量，Tailwind 靠源码扫描生成 CSS。
 */
export function getPaneDisplayClasses(collapsed: boolean, mobilePane: MobilePane): {
  editor: string;
  preview: string;
} {
  return {
    editor: collapsed || mobilePane === 'edit' ? 'flex' : 'hidden lg:flex',
    preview: collapsed ? 'hidden' : mobilePane === 'preview' ? 'flex' : 'hidden lg:flex',
  };
}
