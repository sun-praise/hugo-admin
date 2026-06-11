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

  // Extract <details>...</details> blocks before markdown parsing to prevent
  // marked from mangling them (marked doesn't recognise details as block-level).
  const placeholders: string[] = [];
  const stripped = normalized.replace(/<details[\s\S]*?<\/details>/g, (match) => {
    placeholders.push(match);
    return `\n<!--PLACEHOLDER_${placeholders.length - 1}-->\n`;
  });

  const html = marked.parse(stripped) as string;

  // Restore details blocks
  const restored = html.replace(/<!--PLACEHOLDER_(\d+)-->/g, (_, i) => placeholders[Number(i)]);

  return DOMPurify.sanitize(restored, {
    ADD_TAGS: ['svg', 'path', 'g', 'circle', 'rect', 'line', 'polyline', 'polygon', 'details', 'summary'],
    ADD_ATTR: ['target', 'rel', 'fill', 'stroke', 'stroke-width', 'stroke-linecap', 'stroke-linejoin', 'viewBox', 'd', 'cx', 'cy', 'r', 'x', 'y', 'width', 'height', 'class', 'open'],
  });
}

export function escapeHtml(str: string): string {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
