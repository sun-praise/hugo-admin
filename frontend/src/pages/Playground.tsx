import { useEffect, useRef, useState } from 'react';
import { Loader2, Square } from 'lucide-react';

import { post } from '../utils/api';

// Playground —— poc/index.html 演示页的正式版:Go 重写的两条核心能力。
// 1. SSE 实时通道:连接状态 + 事件日志 + 模拟 TTS 任务
// 2. AI 流式改写:/api/ai/rewrite,带只读工具,帧协议同 /api/ai/chat

// SSE 广播事件(与 useEvents 订阅面一致,此处直接 EventSource 以便展示原始通道)
const WATCHED_EVENTS = [
  'server_log',
  'tts.progress',
  'tts.done',
  'tts.failed',
  'tts.conflict',
  'article_import.progress',
  'cover_done',
  'cover_failed',
];

interface LogEntry {
  id: number;
  name: string;
  data: string;
}

type PartKind = 'delta' | 'tool' | 'result' | 'error' | 'meta';

interface Part {
  kind: PartKind;
  text: string;
}

const MAX_LOG_ENTRIES = 200;
const DEFAULT_TEXT = '这个方案的性能很好，我们用了很多新技术，架构也很优雅。';

function formatPayload(payload: unknown): string {
  if (typeof payload === 'string') return payload;
  try {
    return JSON.stringify(payload);
  } catch {
    return String(payload);
  }
}

export default function Playground() {
  const [connState, setConnState] = useState<'connecting' | 'open' | 'retrying'>('connecting');
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [text, setText] = useState(DEFAULT_TEXT);
  const [parts, setParts] = useState<Part[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const logIdRef = useRef(0);
  const logBoxRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    const es = new EventSource('/api/events');
    const appendLog = (name: string, payload: unknown) => {
      setLogs((prev) => {
        const entry: LogEntry = { id: ++logIdRef.current, name, data: formatPayload(payload) };
        return [...prev.slice(-(MAX_LOG_ENTRIES - 1)), entry];
      });
    };
    es.addEventListener('connected', () => setConnState('open'));
    es.onerror = () => {
      // EventSource 断线后浏览器自动重连
      setConnState((prev) => (prev === 'open' ? 'retrying' : prev));
    };
    for (const name of WATCHED_EVENTS) {
      es.addEventListener(name, (ev) => {
        let payload: unknown = (ev as MessageEvent).data;
        try {
          payload = JSON.parse((ev as MessageEvent).data as string);
        } catch {
          /* 非 JSON 载荷原样记录 */
        }
        appendLog(name, payload);
      });
    }
    return () => es.close();
  }, []);

  useEffect(() => {
    const box = logBoxRef.current;
    if (box) box.scrollTop = box.scrollHeight;
  }, [logs]);

  async function runTTSDemo() {
    try {
      await post('/api/tts/demo');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'TTS 演示任务触发失败');
    }
  }

  function stopRewrite() {
    abortRef.current?.abort();
  }

  async function rewrite() {
    if (streaming || !text.trim()) return;
    setStreaming(true);
    setError(null);
    setParts([]);
    const abort = new AbortController();
    abortRef.current = abort;
    const append = (kind: PartKind, text: string) =>
      setParts((prev) => [...prev, { kind, text }]);

    try {
      const resp = await fetch('/api/ai/rewrite', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text }),
        signal: abort.signal,
      });
      if (!resp.ok) {
        const detail = await resp.text();
        throw new Error(`HTTP ${resp.status} — ${detail}`);
      }
      const reader = resp.body!.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const frames = buffer.split('\n\n');
        buffer = frames.pop()!;
        for (const frame of frames) {
          const data = frame
            .split('\n')
            .filter((l) => l.startsWith('data:'))
            .map((l) => l.slice(5).trim())
            .join('');
          if (!data) continue;
          if (data === '[DONE]') {
            append('meta', ' [完成]');
            continue;
          }
          try {
            const parsed = JSON.parse(data) as {
              type: string;
              tool?: string;
              result?: string;
              error?: string;
            };
            if (parsed.type === 'tool_call') {
              append('tool', `[调用工具 ${parsed.tool}] `);
            } else if (parsed.type === 'tool_result') {
              const result = (parsed.result || '').slice(0, 200);
              append('result', `[工具结果 ${result}${(parsed.result || '').length > 200 ? '…' : ''}] `);
            } else if (parsed.type === 'error') {
              append('error', `[错误] ${parsed.error || '未知错误'}`);
            }
          } catch {
            // 纯文本增量(\n 被转义为字面 \n,渲染前还原)
            append('delta', data.replace(/\\n/g, '\n'));
          }
        }
      }
    } catch (err) {
      if ((err as Error).name !== 'AbortError') {
        setError(err instanceof Error ? err.message : '改写失败');
      }
    } finally {
      setStreaming(false);
      abortRef.current = null;
    }
  }

  const connLabel =
    connState === 'open' ? '已连接（SSE）' : connState === 'retrying' ? '重连中…' : '连接中…';
  const connClass =
    connState === 'open' ? 'text-green-600' : connState === 'retrying' ? 'text-amber-600' : 'text-stone-400';

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <h1 className="text-xl font-bold text-stone-900">演示场</h1>
      <p className="text-sm text-stone-500">
        Go 重写核心能力的演示页,对应 poc/index.html:SSE 实时通道与 AI 流式改写。
      </p>

      <section className="rounded-lg border border-stone-200 bg-white p-4">
        <h2 className="mb-2 text-base font-medium text-stone-800">1. SSE 实时通道</h2>
        <div className="mb-2 flex items-center gap-3">
          <span className={`text-sm ${connClass}`}>{connLabel}</span>
          <button
            type="button"
            onClick={() => void runTTSDemo()}
            className="rounded-md bg-stone-900 px-3 py-1.5 text-xs text-white hover:bg-stone-700"
          >
            模拟 TTS 任务（广播进度）
          </button>
        </div>
        <div
          ref={logBoxRef}
          className="h-44 overflow-y-auto rounded-md bg-stone-50 p-2 font-mono text-xs leading-relaxed text-stone-600"
        >
          {logs.length === 0 && <span className="text-stone-400">等待事件…</span>}
          {logs.map((entry) => (
            <div key={entry.id}>
              <span className="text-stone-400">[{entry.name}]</span> {entry.data}
            </div>
          ))}
        </div>
      </section>

      <section className="rounded-lg border border-stone-200 bg-white p-4">
        <h2 className="mb-2 text-base font-medium text-stone-800">
          2. AI 流式改写（trpc-agent-go + 只读工具）
        </h2>
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          rows={4}
          disabled={streaming}
          className="w-full resize-none rounded-md border border-stone-200 px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-stone-400 disabled:opacity-60"
        />
        <div className="mt-2 flex items-center gap-2">
          {streaming ? (
            <button
              type="button"
              onClick={stopRewrite}
              className="inline-flex items-center gap-1 rounded-md bg-stone-900 px-3 py-1.5 text-xs text-white hover:bg-stone-700"
            >
              <Square className="h-3 w-3" />
              停止
            </button>
          ) : (
            <button
              type="button"
              onClick={() => void rewrite()}
              disabled={!text.trim()}
              className="inline-flex items-center gap-1 rounded-md bg-stone-900 px-3 py-1.5 text-xs text-white hover:bg-stone-700 disabled:opacity-60"
            >
              AI 润色
            </button>
          )}
          {streaming && <Loader2 className="h-3.5 w-3.5 animate-spin text-stone-400" />}
        </div>
        {error && <p className="mt-2 text-xs text-red-600">{error}</p>}
        <div className="mt-2 min-h-10 whitespace-pre-wrap rounded-md border border-stone-200 bg-stone-50 p-2 text-sm leading-relaxed">
          {parts.length === 0 && !streaming && (
            <span className="text-stone-400">结果将流式输出到这里…</span>
          )}
          {parts.map((part, i) => (
            <span
              key={i}
              className={
                part.kind === 'tool'
                  ? 'text-amber-700'
                  : part.kind === 'result'
                    ? 'text-xs text-stone-500'
                    : part.kind === 'error'
                      ? 'text-red-600'
                      : part.kind === 'meta'
                        ? 'text-stone-400'
                        : 'text-stone-800'
              }
            >
              {part.text}
            </span>
          ))}
        </div>
      </section>
    </div>
  );
}
