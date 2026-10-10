import type { AssistResult, AssistTask } from './types';

export interface AssistRequestBody {
  selected_text: string;
  context_before?: string;
  context_after?: string;
  full_text?: string;
}

export interface AssistStreamHandlers {
  onStart?: (model: string) => void;
  onDelta?: (text: string) => void;
  onDone?: (result: AssistResult) => void;
  onError?: (message: string) => void;
}

/**
 * 消费 /api/ai/assist/<task> 的 SSE 流（start/delta/done/error）。
 * 结束方式：onDone 或 onError 恰好其一；请求中止（AbortError）静默返回。
 */
export async function streamAssist(
  task: AssistTask,
  body: AssistRequestBody,
  handlers: AssistStreamHandlers,
  signal?: AbortSignal,
): Promise<void> {
  let resp: Response;
  try {
    resp = await fetch(`/api/ai/assist/${task}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal,
    });
  } catch (err) {
    if ((err as Error).name === 'AbortError') return;
    handlers.onError?.('请求失败，请检查网络');
    return;
  }

  if (!resp.ok) {
    let message = `HTTP ${resp.status}`;
    try {
      const data = await resp.json();
      if (data && typeof data.message === 'string') message = data.message;
    } catch {
      // 非 JSON 错误体，保留 HTTP 状态信息
    }
    // abort 可能恰好发生在读取错误体时：静默返回，不污染新选区的状态
    if (signal?.aborted) return;
    handlers.onError?.(message);
    return;
  }

  if (!resp.body) {
    handlers.onError?.('响应不含数据流');
    return;
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let finished = false;

  const handleFrame = (frame: string) => {
    if (!frame.startsWith('data: ')) return;
    let payload: unknown;
    try {
      payload = JSON.parse(frame.slice(6));
    } catch {
      return;
    }
    if (!payload || typeof payload !== 'object') return;
    const event = payload as Record<string, unknown>;
    const type = event.type;
    if (type === 'start') {
      handlers.onStart?.(String(event.model ?? ''));
    } else if (type === 'delta') {
      handlers.onDelta?.(String(event.text ?? ''));
    } else if (type === 'done') {
      finished = true;
      handlers.onDone?.(event.result as AssistResult);
    } else if (type === 'error') {
      finished = true;
      handlers.onError?.(String(event.error ?? '生成失败'));
    }
  };

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let sep: number;
      while ((sep = buffer.indexOf('\n\n')) !== -1) {
        const frame = buffer.slice(0, sep);
        buffer = buffer.slice(sep + 2);
        handleFrame(frame);
        if (finished) return;
      }
    }
    if (!finished) handlers.onError?.('连接中断，未收到完整结果');
  } catch (err) {
    if ((err as Error).name === 'AbortError') return;
    handlers.onError?.('读取数据流失败');
  }
}
