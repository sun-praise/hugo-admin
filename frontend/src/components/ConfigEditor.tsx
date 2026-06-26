import { useRef, useEffect, useCallback } from 'react';
import Prism from 'prismjs';
import 'prismjs/components/prism-toml';
import 'prismjs/components/prism-yaml';
import 'prismjs/components/prism-json';
import 'prismjs/themes/prism-solarizedlight.css';

// Prism 的 TOML 分词会给 table header 加 class "token table"，
// 与 Tailwind 的 .table (display: table) 工具类冲突，导致 [outputs] 被渲染成
// 三个堆叠的块。强制所有 token inline。
const TOKEN_OVERRIDE = `
.config-editor .token {
  display: inline !important;
}
`;

interface ConfigEditorProps {
  value: string;
  onChange: (value: string) => void;
  format: string;
  fontSize: number;
}

const LANG_MAP: Record<string, string> = {
  toml: 'toml',
  yaml: 'yaml',
  yml: 'yaml',
  json: 'json',
};

export default function ConfigEditor({
  value,
  onChange,
  format,
  fontSize,
}: ConfigEditorProps) {
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const preRef = useRef<HTMLPreElement>(null);
  const codeRef = useRef<HTMLElement>(null);

  const lang = LANG_MAP[format] || 'toml';

  // 两层（高亮 pre + 编辑 textarea）必须使用完全相同的排版度量，
  // 否则光标位置与高亮文字错位。inline style 优先级高于 Prism 注入的样式。
  const SHARED_STYLE: React.CSSProperties = {
    fontFamily:
      'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace',
    fontSize: `${fontSize}px`,
    lineHeight: 1.5,
    tabSize: 2,
    padding: '16px',
    whiteSpace: 'pre',
    wordBreak: 'keep-all',
    letterSpacing: 'normal',
  };

  const highlight = useCallback(() => {
    if (codeRef.current) {
      codeRef.current.textContent = value;
      Prism.highlightElement(codeRef.current);
    }
  }, [value]);

  useEffect(() => {
    highlight();
  }, [highlight]);

  const handleScroll = () => {
    if (textareaRef.current && preRef.current) {
      preRef.current.scrollTop = textareaRef.current.scrollTop;
      preRef.current.scrollLeft = textareaRef.current.scrollLeft;
    }
  };

  return (
    <div className="config-editor relative w-full h-full" style={{ minHeight: 400 }}>
      <style>{TOKEN_OVERRIDE}</style>
      {/* 高亮层（底层） */}
      <pre
        ref={preRef}
        aria-hidden="true"
        className="absolute inset-0 m-0 overflow-auto pointer-events-none"
        style={{
          ...SHARED_STYLE,
          margin: 0,
          background: 'transparent',
        }}
      >
        <code
          ref={codeRef}
          className={`language-${lang}`}
          style={{
            ...SHARED_STYLE,
            padding: 0,
            margin: 0,
            background: 'none',
            textShadow: 'none',
          }}
        >
          {value}
        </code>
      </pre>

      {/* 编辑层（顶层，透明文字 + 可见光标） */}
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onScroll={handleScroll}
        spellCheck={false}
        className="relative w-full h-full resize-none outline-none border-0 bg-transparent"
        style={{
          ...SHARED_STYLE,
          margin: 0,
          color: 'transparent',
          caretColor: '#073642',
          boxSizing: 'border-box',
          border: 'none',
        }}
        autoComplete="off"
        autoCorrect="off"
        autoCapitalize="off"
      />
    </div>
  );
}
