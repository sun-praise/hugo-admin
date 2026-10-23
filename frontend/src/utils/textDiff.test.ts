import { describe, expect, it } from 'vitest';

import { wordDiff } from './textDiff';

function kinds(segments: ReturnType<typeof wordDiff>) {
  return segments.map((s) => s.kind);
}

function joined(segments: ReturnType<typeof wordDiff>, kind: 'added' | 'removed') {
  return segments.filter((s) => s.kind === kind).map((s) => s.text).join('');
}

describe('wordDiff', () => {
  it('无差异时只有 same 段', () => {
    const segs = wordDiff('完全相同的一段话', '完全相同的一段话');
    expect(kinds(segs)).toEqual(['same']);
    expect(segs[0].text).toBe('完全相同的一段话');
  });

  it('纯新增：revised 相对 original 的增量被标为 added', () => {
    const segs = wordDiff('原文', '原文（书面化改写）');
    expect(kinds(segs)).toEqual(['same', 'added']);
    expect(joined(segs, 'added')).toBe('（书面化改写）');
    expect(joined(segs, 'removed')).toBe('');
  });

  it('纯删除：original 独有部分被标为 removed', () => {
    const segs = wordDiff('原文加上多余的话', '原文');
    expect(joined(segs, 'removed')).not.toBe('');
    expect(joined(segs, 'added')).toBe('');
    // 拼回 original：same + removed == original
    expect(segs.filter((s) => s.kind !== 'added').map((s) => s.text).join('')).toBe('原文加上多余的话');
  });

  it('混合增删：same/added/removed 拼接可分别还原原文与新文', () => {
    const original = '今天天气特别好使，所有人都会开心。';
    const revised = '今天天气表现出色，多数人会开心。';
    const segs = wordDiff(original, revised);
    expect(
      segs.filter((s) => s.kind !== 'added').map((s) => s.text).join(''),
    ).toBe(original);
    expect(
      segs.filter((s) => s.kind !== 'removed').map((s) => s.text).join(''),
    ).toBe(revised);
    expect(kinds(segs)).toContain('added');
    expect(kinds(segs)).toContain('removed');
  });
});
