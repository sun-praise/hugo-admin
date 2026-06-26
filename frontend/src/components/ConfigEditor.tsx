import { useRef, useEffect, useCallback } from 'react';
import Prism from 'prismjs';
import 'prismjs/components/prism-toml';
import 'prismjs/components/prism-yaml';
import 'prismjs/components/prism-json';
import 'prismjs/themes/prism-solarizedlight.css';

interface ConfigEditorProps {
  value: string;
  onChange: (value: string) => void;
  format: string;
}

const LANG_MAP: Record<string, string> = {
  toml: 'toml',
  yaml: 'yaml',
  yml: 'yaml',
  json: 'json',
};

export default function ConfigEditor({ value, onChange, format }: ConfigEditorProps) {
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const preRef = useRef<HTMLPreElement>(null);
  const codeRef = useRef<HTMLElement>(null);

  const lang = LANG_MAP[format] || 'toml';

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
    <div className="relative w-full h-full" style={{ minHeight: 400 }}>
      {/* 高亮层（底层） */}
      <pre
        ref={preRef}
        aria-hidden="true"
        className="absolute inset-0 m-0 p-4 overflow-auto pointer-events-none font-mono text-sm leading-relaxed whitespace-pre"
        style={{ tabSize: 2 }}
      >
        <code ref={codeRef} className={`language-${lang}`}>
          {value}
        </code>
      </pre>

      {/* 编辑层（顶层，透明文字） */}
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onScroll={handleScroll}
        spellCheck={false}
        className="relative w-full h-full resize-none p-4 font-mono text-sm leading-relaxed bg-transparent text-transparent caret-stone-800 outline-none border-0 whitespace-pre"
        style={{ tabSize: 2, minHeight: 400 }}
      />
    </div>
  );
}
