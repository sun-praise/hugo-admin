import { diffWords } from 'diff';

export interface DiffSegment {
  text: string;
  kind: 'same' | 'added' | 'removed';
}

/** 词级 diff：用于润色候选与原文的对照渲染。 */
export function wordDiff(original: string, revised: string): DiffSegment[] {
  return diffWords(original, revised).map((part) => ({
    text: part.value,
    kind: part.added ? 'added' : part.removed ? 'removed' : 'same',
  }));
}
