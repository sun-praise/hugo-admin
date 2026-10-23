import { describe, expect, it, vi } from 'vitest';

import {
  rangeMatches,
  replaceTextareaRange,
  undoTextareaReplace,
  type ReplaceRecord,
} from './selectionReplace';

/** 最小 textarea 桩：只实现 selectionReplace 依赖的表面。 */
function fakeTextarea(initial: string, execCommandOk = true) {
  const state = { value: initial, selectionStart: 0, selectionEnd: 0 };
  const ta = {
    get value() {
      return state.value;
    },
    focus: vi.fn(),
    setSelectionRange: vi.fn((start: number, end: number) => {
      state.selectionStart = start;
      state.selectionEnd = end;
    }),
    setRangeText: vi.fn((text: string, start: number, end: number) => {
      state.value = state.value.slice(0, start) + text + state.value.slice(end);
      state.selectionStart = state.selectionEnd = start + text.length;
    }),
  };
  // 模拟浏览器 insertText：按当前选区替换并推进光标（真实浏览器行为）
  const execCommand = vi.fn((cmd: string, _ui: boolean, text?: string) => {
    if (execCommandOk && cmd === 'insertText' && typeof text === 'string') {
      state.value =
        state.value.slice(0, state.selectionStart) +
        text +
        state.value.slice(state.selectionEnd);
      state.selectionStart = state.selectionEnd = state.selectionStart + text.length;
      return true;
    }
    return false;
  });
  vi.stubGlobal('document', {
    execCommand,
    get activeElement() {
      return ta;
    },
  });
  return { ta: ta as unknown as HTMLTextAreaElement, state, execCommand };
}

describe('replaceTextareaRange', () => {
  it('execCommand 可用时走 insertText 且更新 value', () => {
    const { ta, state, execCommand } = fakeTextarea('Hello 世界');
    const record = replaceTextareaRange(ta, 6, 8, 'World');
    expect(execCommand).toHaveBeenCalledWith('insertText', false, 'World');
    expect(state.value).toBe('Hello World');
    expect(record).toEqual({ start: 6, end: 11, oldText: '世界', newText: 'World' });
  });

  it('execCommand 失败时降级 setRangeText', () => {
    const { ta, state, execCommand } = fakeTextarea('Hello 世界', false);
    const record = replaceTextareaRange(ta, 6, 8, 'World');
    expect(execCommand).toHaveBeenCalled();
    expect(state.value).toBe('Hello World');
    expect(record).not.toBeNull();
  });

  it('区间越界返回 null 且不改内容', () => {
    const { ta, state } = fakeTextarea('abc');
    expect(replaceTextareaRange(ta, -1, 2, 'X')).toBeNull();
    expect(replaceTextareaRange(ta, 0, 9, 'X')).toBeNull();
    expect(replaceTextareaRange(ta, 2, 1, 'X')).toBeNull();
    expect(state.value).toBe('abc');
  });
});

describe('undoTextareaReplace / rangeMatches', () => {
  it('撤销按记录恢复原文并返回撤销记录', () => {
    const { ta, state } = fakeTextarea('Hello 世界');
    const record = replaceTextareaRange(ta, 6, 8, 'World') as ReplaceRecord;
    expect(state.value).toBe('Hello World');

    const undone = undoTextareaReplace(ta, record);
    expect(state.value).toBe('Hello 世界');
    // 返回的是逆操作记录（old/new 互换），再次应用即重做
    expect(undone).toEqual({ start: 6, end: 8, oldText: 'World', newText: '世界' });
  });

  it('漂移检查：区间内容与预期不符时 rangeMatches 为 false', () => {
    const { ta } = fakeTextarea('Hello 世界');
    expect(rangeMatches(ta, 6, 8, '世界')).toBe(true);
    expect(rangeMatches(ta, 6, 8, '别的')).toBe(false);
  });
});
