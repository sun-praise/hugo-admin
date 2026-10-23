import { describe, expect, it } from 'vitest';

import { extractPartialTranslation } from './translationPreview';

describe('extractPartialTranslation', () => {
  const full = '{"translation": "你好，世界。", "hard_words": [{"word": "世界"}]}';

  it('完整 JSON 抽取 translation', () => {
    expect(extractPartialTranslation(full)).toBe('你好，世界。');
  });

  it('半截流（未闭合字符串）抽取已到达部分', () => {
    expect(extractPartialTranslation('{"translation": "你好，世')).toBe('你好，世');
    expect(extractPartialTranslation('{"translation": "')).toBe('');
  });

  it('流还没到 translation 字段时返回 null', () => {
    expect(extractPartialTranslation('{"translat')).toBeNull();
    expect(extractPartialTranslation('{"hard_words": []}')).toBeNull();
    expect(extractPartialTranslation('')).toBeNull();
  });

  it('转义序列还原', () => {
    expect(extractPartialTranslation('{"translation": "a\\nb"}')).toBe('a\nb');
    expect(extractPartialTranslation('{"translation": "a\\tb"}')).toBe('a\tb');
    expect(extractPartialTranslation('{"translation": "a\\rb"}')).toBe('a\rb');
    expect(extractPartialTranslation('{"translation": "a\\bb"}')).toBe('a\bb');
    expect(extractPartialTranslation('{"translation": "a\\fb"}')).toBe('a\fb');
    expect(extractPartialTranslation('{"translation": "a\\"b\\\\c\\/d"}')).toBe('a"b\\c/d');
  });

  it('\\uXXXX 转义还原', () => {
    expect(extractPartialTranslation('{"translation": "\\u4f60\\u597d"}')).toBe('你好');
  });

  it('半截 \\u 转义不吞字符', () => {
    // \u4f 只到了一半：4f 先按字面保留，等下一帧补全
    expect(extractPartialTranslation('{"translation": "\\u4f')).toBe('u4f');
  });

  it('截断在反斜杠处：转义不完整时保留已见字符', () => {
    // 流尾正好是 "\\"：无法判断转义类型，丢弃并等下一帧
    expect(extractPartialTranslation('{"translation": "abc\\')).toBe('abc');
  });

  it('字符串闭合后忽略后续字段', () => {
    expect(extractPartialTranslation(full)).toBe('你好，世界。');
    expect(extractPartialTranslation('{"translation": "", "hard_words": []}')).toBe('');
  });
});
