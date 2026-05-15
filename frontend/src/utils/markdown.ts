import { marked } from 'marked';
import hljs from 'highlight.js';

(marked as any).setOptions({
  breaks: true,
  gfm: true,
  highlight: function(code: string, lang?: string) {
    if (lang && hljs.getLanguage(lang)) {
      try {
        return hljs.highlight(code, { language: lang }).value;
      } catch (e) {
        // ignore
      }
    }
    return hljs.highlightAuto(code).value;
  },
});

export function renderMarkdown(content: string): string {
  const normalized = content.replace(/\\n/g, '\n');
  return marked.parse(normalized) as string;
}

export function escapeHtml(str: string): string {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
