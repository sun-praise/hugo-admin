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

export interface RenderMarkdownOptions {
  // When true, ```mermaid fenced blocks are emitted as <div class="mermaid">
  // placeholders for the client-side mermaid runtime instead of highlighted
  // code. The caller is responsible for invoking mermaid.run() afterwards.
  mermaid?: boolean;
}

export function renderMarkdown(content: string, options: RenderMarkdownOptions = {}): string {
  const { mermaid: enableMermaid = false } = options;
  const normalized = content.replace(/\\n/g, '\n');

  // Extract <details>...</details> blocks before markdown parsing to prevent
  // marked from mangling them (marked doesn't recognise details as block-level).
  const placeholders: string[] = [];
  let stripped = normalized.replace(/<details[\s\S]*?<\/details>/g, (match) => {
    placeholders.push(match);
    return `\n<!--PLACEHOLDER_${placeholders.length - 1}-->\n`;
  });

  // Extract ```mermaid fenced code blocks before parsing so highlight.js
  // doesn't treat them as plain code. They are restored as
  // <div class="mermaid"> containers for the client-side mermaid runtime.
  //
  // v1 limit: the fence must be exactly `mermaid` with only optional trailing
  // spaces/tabs. Pandoc/Hugo info strings (e.g. ```mermaid {theme: forest}```
  // or ```mermaid theme=forest```) are intentionally NOT matched and fall
  // through to hljs; add handling here only if those variants are needed.
  const mermaidBlocks: string[] = [];
  if (enableMermaid) {
    stripped = stripped.replace(/```mermaid[ \t]*\r?\n([\s\S]*?)```/g, (_m, code: string) => {
      mermaidBlocks.push(code.replace(/\r?\n$/, ''));
      return `\n<!--MERMAID_${mermaidBlocks.length - 1}-->\n`;
    });
  }

  const html = marked.parse(stripped) as string;

  // Restore details and mermaid blocks
  let restored = html.replace(/<!--PLACEHOLDER_(\d+)-->/g, (_, i) => placeholders[Number(i)]);
  if (enableMermaid) {
    restored = restored.replace(/<!--MERMAID_(\d+)-->/g, (_, i) => {
      const code = escapeHtml(mermaidBlocks[Number(i)]);
      return `<div class="mermaid">${code}</div>`;
    });
  }

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
