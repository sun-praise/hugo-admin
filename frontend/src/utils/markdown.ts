import { Marked } from 'marked';
import { markedHighlight } from 'marked-highlight';
import hljs from 'highlight.js';
import DOMPurify from 'dompurify';

const marked = new Marked(
  markedHighlight({
    langPrefix: 'hljs language-',
    highlight(code, lang) {
      const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext';
      try {
        return hljs.highlight(code, { language }).value;
      } catch {
        return hljs.highlightAuto(code).value;
      }
    },
  }),
);

marked.setOptions({ breaks: true, gfm: true });

export function renderMarkdown(content: string): string {
  const normalized = content.replace(/\\n/g, '\n');
  const html = marked.parse(normalized) as string;
  return DOMPurify.sanitize(html, {
    ADD_TAGS: ['svg', 'path', 'g', 'circle', 'rect', 'line', 'polyline', 'polygon'],
    ADD_ATTR: ['target', 'rel', 'fill', 'stroke', 'stroke-width', 'stroke-linecap', 'stroke-linejoin', 'viewBox', 'd', 'cx', 'cy', 'r', 'x', 'y', 'width', 'height', 'class'],
  });
}

export function escapeHtml(str: string): string {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
