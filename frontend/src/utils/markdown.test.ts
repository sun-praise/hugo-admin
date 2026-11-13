import { beforeEach, describe, expect, it, vi } from 'vitest';

// DOMPurify needs a real DOM, which the node test environment does not have.
// These tests pin the contract between renderMarkdown and the sanitiser:
// which HTML is handed over, and which allowlist is applied. URI/attribute
// filtering itself is DOMPurify's own responsibility (and is not relaxed
// here: srcdoc, event handlers and script/data URLs stay blocked).
vi.mock('dompurify', () => ({
  default: { sanitize: vi.fn((html: string) => html) },
}));

import DOMPurify from 'dompurify';
import { renderMarkdown } from './markdown';

const sanitize = vi.mocked(DOMPurify.sanitize);

interface SanitizeConfig {
  ADD_TAGS: string[];
  ADD_ATTR: string[];
}

function lastConfig(): SanitizeConfig {
  const call = sanitize.mock.calls.at(-1);
  if (!call) throw new Error('DOMPurify.sanitize was not called');
  return call[1] as SanitizeConfig;
}

const iframe =
  '<iframe src="//player.bilibili.com/player.html?bvid=BV1xx&page=1" scrolling="no" frameborder="no" framespacing="0" allowfullscreen="true"></iframe>';

describe('renderMarkdown / embeds', () => {
  beforeEach(() => {
    sanitize.mockClear();
  });

  it('默认不放行 iframe，AIChat 等调用方行为不变', () => {
    renderMarkdown(iframe);
    expect(lastConfig().ADD_TAGS).not.toContain('iframe');
    expect(lastConfig().ADD_ATTR).not.toContain('allowfullscreen');
  });

  it('embeds: true 保留原生 iframe 并放行嵌入所需属性', () => {
    const html = renderMarkdown(`前文\n\n${iframe}\n\n后文`, { embeds: true });
    const config = lastConfig();

    expect(config.ADD_TAGS).toContain('iframe');
    for (const attr of [
      'allow',
      'allowfullscreen',
      'border',
      'frameborder',
      'framespacing',
      'referrerpolicy',
      'sandbox',
      'scrolling',
    ]) {
      expect(config.ADD_ATTR).toContain(attr);
    }
    expect(config.ADD_ATTR).not.toContain('srcdoc');
    expect(config.ADD_TAGS).not.toContain('script');

    expect(html).toContain('<iframe src="//player.bilibili.com/player.html?bvid=BV1xx&page=1"');
    expect(html).toContain('allowfullscreen="true"');
    expect(html).toContain('</iframe>');
  });

  it('多行书写的 iframe 属性不被 markdown 解析破坏', () => {
    const multiline =
      '<iframe src="//player.bilibili.com/player.html?bvid=BV1xx"\n' +
      '  scrolling="no" frameborder="no"\n' +
      '  allowfullscreen="true"></iframe>';
    const html = renderMarkdown(`前文\n\n${multiline}\n\n后文`, { embeds: true });

    expect(html).toContain('scrolling="no"');
    expect(html).toContain('allowfullscreen="true"');
    expect(html).toContain('</iframe>');
  });

  it('代码块中的 iframe 仍按代码显示，不会被当作嵌入', () => {
    const html = renderMarkdown('```html\n<iframe src="x"></iframe>\n```', { embeds: true });

    // highlight.js 把转义后的标签再包进 <span>，所以只断言「已转义 + 仍是代码块」
    expect(html).toContain('class="hljs language-html"');
    expect(html).toContain('&lt;');
    expect(html).not.toContain('<iframe');
  });
});
