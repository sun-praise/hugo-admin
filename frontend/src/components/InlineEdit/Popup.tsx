import { Loader2, X } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';

import { INLINE_EDIT_PRESETS } from './presets';
import { post } from '../../utils/api';

export interface PopupProps {
  selectedText: string;
  contextBefore: string;
  contextAfter: string;
  /** Viewport-relative top (px). */
  top: number;
  /** Viewport-relative left (px). */
  left: number;
  onClose: () => void;
  onAccept: (revisedText: string) => void;
}

interface InlineEditResponse {
  success: boolean;
  revised_text?: string;
  model?: string;
  message?: string;
}

export function Popup({
  selectedText,
  contextBefore,
  contextAfter,
  top,
  left,
  onClose,
  onAccept,
}: PopupProps) {
  const [instruction, setInstruction] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const aliveRef = useRef(true);

  useEffect(() => {
    return () => {
      aliveRef.current = false;
    };
  }, []);

  async function send(instructionText: string) {
    const trimmed = instructionText.trim();
    if (!trimmed || loading) return;
    setLoading(true);
    setError(null);
    setResult(null);
    try {
      const resp = await post<InlineEditResponse>('/api/ai/inline-edit', {
        selected_text: selectedText,
        instruction: trimmed,
        context_before: contextBefore,
        context_after: contextAfter,
      });
      if (!aliveRef.current) return;
      if (!resp.success || !resp.revised_text) {
        setError(resp.message || '改写失败');
      } else {
        setResult(resp.revised_text);
      }
    } catch (err) {
      if (!aliveRef.current) return;
      setError(err instanceof Error ? err.message : '改写失败');
    } finally {
      if (aliveRef.current) setLoading(false);
    }
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement | HTMLDivElement>) {
    if (e.key === 'Enter' && !e.shiftKey && e.currentTarget.tagName === 'TEXTAREA') {
      e.preventDefault();
      e.stopPropagation();
      void send(instruction);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      onClose();
    }
  }

  return (
    <div
      role="dialog"
      aria-label="AI 改写"
      onKeyDown={handleKeyDown}
      onMouseDown={(e) => e.stopPropagation()}
      className="fixed z-[60] flex w-80 flex-col gap-2 animate-rise rounded-[10px] border border-[#e4e4e7] bg-white p-3 shadow-2xl"
      style={{ top, left }}
    >
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-[#3f3f46]">快速改写</span>
        <button
          type="button"
          aria-label="关闭"
          onClick={onClose}
          className="rounded p-1 text-[#9ca3a8] hover:bg-[#f4f4f5] hover:text-zinc-900"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>

      {result === null ? (
        <>
          <textarea
            value={instruction}
            onChange={(e) => setInstruction(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="输入改写指令，或点击下方预设…"
            autoFocus
            rows={2}
            disabled={loading}
            className="resize-none rounded-md border border-[#e4e4e7] bg-white px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-[#d4d4d8] disabled:opacity-60"
          />
          <div className="flex flex-wrap gap-1.5">
            {INLINE_EDIT_PRESETS.map((preset) => (
              <button
                key={preset.id}
                type="button"
                disabled={loading}
                onClick={() => void send(preset.instruction)}
                className="h-6 rounded-md border border-[#e4e4e7] bg-[#f4f4f5] px-2 text-xs text-[#3f3f46] hover:bg-[#ebebeb] disabled:opacity-60"
              >
                {preset.label}
              </button>
            ))}
          </div>
          {error && <p className="text-xs text-red-600">{error}</p>}
          <div className="flex items-center justify-end gap-2 pt-1">
            <button
              type="button"
              onClick={onClose}
              disabled={loading}
              className="rounded-md px-2 py-1 text-xs text-[#52525b] hover:bg-[#f4f4f5] disabled:opacity-60"
            >
              取消
            </button>
            <button
              type="button"
              onClick={() => void send(instruction)}
              disabled={loading || instruction.trim().length === 0}
              className="rounded-lg bg-zinc-900 px-2 py-1 text-xs text-white hover:bg-[#2e2e33] disabled:opacity-60 inline-flex items-center gap-1"
            >
              {loading ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  改写中…
                </>
              ) : (
                '发送'
              )}
            </button>
          </div>
        </>
      ) : (
        <div className="flex flex-col gap-2">
          <div className="rounded-md border border-[#e4e4e7] bg-[#f4f4f5] p-2">
            <div className="mb-1 text-[11px] font-medium text-[#52525b]">原文</div>
            <pre className="max-h-24 overflow-auto whitespace-pre-wrap break-words text-xs leading-relaxed text-[#52525b]">
              {selectedText}
            </pre>
          </div>
          <div className="rounded-md border border-[#e4e4e7] bg-white p-2">
            <div className="mb-1 text-[11px] font-medium text-zinc-900">改写</div>
            <pre className="max-h-32 overflow-auto whitespace-pre-wrap break-words text-xs leading-relaxed">
              {result}
            </pre>
          </div>
          <div className="flex justify-end gap-2 pt-1">
            <button
              type="button"
              onClick={() => {
                setResult(null);
                setError(null);
              }}
              className="rounded-md px-2 py-1 text-xs text-[#52525b] hover:bg-[#f4f4f5]"
            >
              取消
            </button>
            <button
              type="button"
              onClick={() => onAccept(result)}
              className="rounded-lg bg-zinc-900 px-2 py-1 text-xs text-white hover:bg-[#2e2e33]"
            >
              接受
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
