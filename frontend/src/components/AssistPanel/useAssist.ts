import { useCallback, useEffect, useRef, useState } from 'react';

import { streamAssist } from './sse';
import {
  ASSIST_TASKS,
  DEFAULT_TASK_CONFIG,
  type AssistResult,
  type AssistTask,
  type AssistTaskConfig,
  type TaskState,
} from './types';

export interface AssistAnchor {
  text: string;
  contextBefore: string;
  contextAfter: string;
}

const CACHE_CAPACITY = 20;
/** SSE delta 攒批窗口：避免每帧一次 setState 引发编辑器大树高频重渲染。 */
const DELTA_BATCH_MS = 50;

const idleState: TaskState = { status: 'idle', streamText: '', result: null, error: null };

type TaskStates = Record<AssistTask, TaskState>;

const allIdle = (): TaskStates => ({
  translate: { ...idleState },
  polish: { ...idleState },
  review: { ...idleState },
});

interface RunContext {
  anchor: AssistAnchor;
  fullText: string;
}

/**
 * 缓存条目：以选区文本为键。translate/polish 不依赖文章全文、始终可复用；
 * 只有 review 的结果依赖全文——写入时记录当时的 fullText，命中时全文变了
 * 则只失效 review 并补跑，不拖累另两项。
 */
interface CacheEntry {
  states: TaskStates;
  reviewFullText: string | null;
}

export type { TaskStates };

/**
 * 选区辅助的任务编排：已启用任务并行、同选区 LRU 缓存、选区切换中止
 * 旧请求、单任务重试与整体重新生成。
 *
 * 每个任务持有独立 AbortController：重试单个任务不会误伤仍在流式传输
 * 的其他任务；只有选区切换（assist/reset）才全量中止。
 */
export function useAssist() {
  const [tasks, setTasks] = useState<TaskStates>(allIdle);
  const cacheRef = useRef(new Map<string, CacheEntry>());
  const controllersRef = useRef<Partial<Record<AssistTask, AbortController>>>({});
  const lastRunRef = useRef<RunContext | null>(null);
  const enabledRef = useRef<AssistTaskConfig>(DEFAULT_TASK_CONFIG);
  const pendingDeltaRef = useRef<Partial<Record<AssistTask, string>>>({});
  const batchTimerRef = useRef<number | null>(null);

  const setEnabled = useCallback((config: AssistTaskConfig) => {
    enabledRef.current = config;
  }, []);

  /** 停用一个任务：中止在途请求并复位该任务状态（区块随配置移除）。 */
  const disableTask = useCallback((task: AssistTask) => {
    controllersRef.current[task]?.abort();
    delete controllersRef.current[task];
    delete pendingDeltaRef.current[task];
    setTasks((prev) => ({ ...prev, [task]: { ...idleState } }));
  }, []);

  const cancelBatch = useCallback(() => {
    if (batchTimerRef.current !== null) {
      window.clearTimeout(batchTimerRef.current);
      batchTimerRef.current = null;
    }
    pendingDeltaRef.current = {};
  }, []);

  // 卸载兜底：攒批定时器不再触发（React 18+ 卸载后 setState 静默，但应清理）
  useEffect(() => () => cancelBatch(), [cancelBatch]);

  const flushDeltas = useCallback(() => {
    batchTimerRef.current = null;
    const pending = pendingDeltaRef.current;
    pendingDeltaRef.current = {};
    if (Object.keys(pending).length === 0) return;
    setTasks((prev) => {
      const next = { ...prev };
      for (const task of ASSIST_TASKS) {
        const text = pending[task];
        if (text) {
          next[task] = { ...next[task], streamText: next[task].streamText + text };
        }
      }
      return next;
    });
  }, []);

  const writeCache = useCallback(
    (key: string, task: AssistTask, state: TaskState, fullText: string) => {
      const cache = cacheRef.current;
      let entry = cache.get(key);
      if (!entry) {
        if (cache.size >= CACHE_CAPACITY) {
          const oldest = cache.keys().next().value;
          if (oldest !== undefined) cache.delete(oldest);
        }
        entry = { states: allIdle(), reviewFullText: null };
        cache.set(key, entry);
      }
      entry.states = { ...entry.states, [task]: state };
      if (task === 'review') {
        entry.reviewFullText = state.status === 'done' ? fullText : null;
      }
    },
    [],
  );

  const runTask = useCallback(
    (task: AssistTask, run: RunContext) => {
      controllersRef.current[task]?.abort();
      const controller = new AbortController();
      controllersRef.current[task] = controller;
      delete pendingDeltaRef.current[task];

      const body =
        task === 'review'
          ? { selected_text: run.anchor.text, full_text: run.fullText }
          : {
              selected_text: run.anchor.text,
              context_before: run.anchor.contextBefore,
              context_after: run.anchor.contextAfter,
            };

      setTasks((prev) => ({
        ...prev,
        [task]: { ...idleState, status: 'loading' },
      }));

      const key = run.anchor.text;
      return streamAssist(
        task,
        body,
        {
          onDelta: (text) => {
            pendingDeltaRef.current[task] = (pendingDeltaRef.current[task] ?? '') + text;
            if (batchTimerRef.current === null) {
              batchTimerRef.current = window.setTimeout(flushDeltas, DELTA_BATCH_MS);
            }
          },
          onDone: (result: AssistResult) => {
            flushDeltas();
            const done: TaskState = { ...idleState, status: 'done', result };
            setTasks((prev) => ({ ...prev, [task]: done }));
            writeCache(key, task, done, run.fullText);
          },
          onError: (message) => {
            flushDeltas();
            setTasks((prev) => ({
              ...prev,
              [task]: { ...idleState, status: 'error', error: message },
            }));
          },
        },
        controller.signal,
      );
    },
    [flushDeltas, writeCache],
  );

  const assist = useCallback(
    (anchor: AssistAnchor, fullText: string) => {
      cancelBatch();
      Object.values(controllersRef.current).forEach((c) => c?.abort());
      controllersRef.current = {};
      lastRunRef.current = { anchor, fullText };

      const cached = cacheRef.current.get(anchor.text);
      if (cached) {
        const reviewStale =
          cached.states.review.status === 'done' && cached.reviewFullText !== fullText;
        if (!reviewStale) {
          // LRU touch：重新插入刷新顺序
          cacheRef.current.delete(anchor.text);
          cacheRef.current.set(anchor.text, cached);
          setTasks(cached.states);
          return;
        }
        // translate/polish 直接命中；只有 review 因全文变化失效并补跑
        const states: TaskStates = {
          ...cached.states,
          review: { ...idleState },
        };
        cacheRef.current.delete(anchor.text);
        cacheRef.current.set(anchor.text, { states, reviewFullText: null });
        setTasks(states);
        if (enabledRef.current.review) {
          void runTask('review', { anchor, fullText });
        }
        return;
      }

      setTasks(allIdle());
      const run: RunContext = { anchor, fullText };
      ASSIST_TASKS.filter((task) => enabledRef.current[task]).forEach((task) => {
        void runTask(task, run);
      });
    },
    [cancelBatch, runTask],
  );

  /** 单任务重试：只重建该任务的请求；fullText 传当前正文避免 review 用过期全文。 */
  const retry = useCallback(
    (task: AssistTask, fullText?: string) => {
      const last = lastRunRef.current;
      if (!last) return;
      const run: RunContext = {
        anchor: last.anchor,
        fullText: fullText ?? last.fullText,
      };
      lastRunRef.current = run;
      void runTask(task, run);
    },
    [runTask],
  );

  /** 重新生成：清除该选区缓存并重跑全部已启用任务（fullText 传当前正文）。 */
  const regenerate = useCallback(
    (fullText?: string) => {
      const last = lastRunRef.current;
      if (!last) return;
      const run: RunContext = {
        anchor: last.anchor,
        fullText: fullText ?? last.fullText,
      };
      lastRunRef.current = run;
      cacheRef.current.delete(run.anchor.text);
      assist(run.anchor, run.fullText);
    },
    [assist],
  );

  /** 选区清空/面板关闭：中止全部请求并复位状态。 */
  const reset = useCallback(() => {
    cancelBatch();
    Object.values(controllersRef.current).forEach((c) => c?.abort());
    controllersRef.current = {};
    lastRunRef.current = null;
    setTasks(allIdle());
  }, [cancelBatch]);

  return { tasks, assist, retry, regenerate, reset, setEnabled, disableTask };
}
