export interface ReplaceRecord {
  /** 替换后的区间（start 不变，end 指向新文本末尾）。 */
  start: number;
  end: number;
  oldText: string;
  newText: string;
}

/**
 * 替换 textarea 中 [start, end) 的文本。
 *
 * 优先用 document.execCommand('insertText')：这是 textarea 上唯一能把
 * 替换并入原生 Ctrl+Z 撤销栈的方式（setRangeText 会绕过撤销栈）。
 * execCommand 已标记废弃，故保留 setRangeText 兜底路径——两条路径都
 * 只改 DOM value，调用方负责把新文本同步回 React 受控 state。
 */
export function replaceTextareaRange(
  textarea: HTMLTextAreaElement,
  start: number,
  end: number,
  newText: string,
): ReplaceRecord | null {
  const value = textarea.value;
  if (start < 0 || end > value.length || start > end) return null;
  const oldText = value.substring(start, end);

  textarea.focus();
  textarea.setSelectionRange(start, end);

  let ok: boolean;
  try {
    ok = document.execCommand('insertText', false, newText);
  } catch {
    ok = false;
  }
  if (!ok) {
    textarea.setRangeText(newText, start, end, 'end');
  }

  return { start, end: start + newText.length, oldText, newText };
}

/** 撤销一次替换：把 record 区间内的文本换回 oldText（同样走撤销栈）。 */
export function undoTextareaReplace(
  textarea: HTMLTextAreaElement,
  record: ReplaceRecord,
): ReplaceRecord | null {
  return replaceTextareaRange(textarea, record.start, record.end, record.oldText);
}

/** 校验区间当前文本是否仍与预期一致（撤销前的漂移检查）。 */
export function rangeMatches(
  textarea: HTMLTextAreaElement,
  start: number,
  end: number,
  expected: string,
): boolean {
  return textarea.value.substring(start, end) === expected;
}
