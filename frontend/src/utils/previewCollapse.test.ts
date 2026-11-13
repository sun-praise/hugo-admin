import { describe, expect, it, vi } from 'vitest';

import {
  getPaneDisplayClasses,
  PREVIEW_COLLAPSED_KEY,
  readPreviewCollapsed,
  writePreviewCollapsed,
  type StorageLike,
} from './previewCollapse';

class FakeStorage implements StorageLike {
  values = new Map<string, string>();
  getItem = vi.fn((key: string) => this.values.get(key) ?? null);
  setItem = vi.fn((key: string, value: string) => {
    this.values.set(key, value);
  });
}

const deniedStorage: StorageLike = {
  getItem: () => {
    throw new Error('storage denied');
  },
  setItem: () => {
    throw new Error('storage denied');
  },
};

describe('预览收起状态持久化', () => {
  it('读取：仅 "1" 视为收起', () => {
    const storage = new FakeStorage();
    expect(readPreviewCollapsed(storage)).toBe(false);
    storage.setItem(PREVIEW_COLLAPSED_KEY, '1');
    expect(readPreviewCollapsed(storage)).toBe(true);
    storage.setItem(PREVIEW_COLLAPSED_KEY, '0');
    expect(readPreviewCollapsed(storage)).toBe(false);
  });

  it('写入：展开回落为 "0"，不会一直保持收起', () => {
    const storage = new FakeStorage();
    writePreviewCollapsed(storage, true);
    expect(storage.getItem(PREVIEW_COLLAPSED_KEY)).toBe('1');
    writePreviewCollapsed(storage, false);
    expect(storage.getItem(PREVIEW_COLLAPSED_KEY)).toBe('0');
  });

  it('localStorage 不可用（null / 抛错）时安全降级、不抛异常', () => {
    expect(readPreviewCollapsed(null)).toBe(false);
    expect(readPreviewCollapsed(deniedStorage)).toBe(false);
    expect(() => writePreviewCollapsed(null, true)).not.toThrow();
    expect(() => writePreviewCollapsed(deniedStorage, true)).not.toThrow();
  });
});

describe('两栏 display 类', () => {
  it('收起：编辑器占满整行、预览隐藏（与窄屏页签状态无关）', () => {
    for (const pane of ['edit', 'preview'] as const) {
      expect(getPaneDisplayClasses(true, pane)).toEqual({ editor: 'flex', preview: 'hidden' });
    }
  });

  it('展开且在编辑页签：窄屏只看编辑器，lg+ 双栏', () => {
    expect(getPaneDisplayClasses(false, 'edit')).toEqual({
      editor: 'flex',
      preview: 'hidden lg:flex',
    });
  });

  it('展开且在预览页签：窄屏只看预览，lg+ 双栏', () => {
    expect(getPaneDisplayClasses(false, 'preview')).toEqual({
      editor: 'hidden lg:flex',
      preview: 'flex',
    });
  });
});
