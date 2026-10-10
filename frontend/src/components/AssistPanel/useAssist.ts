import { useCallback, useRef, useState } from 'react';

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

/** 缓存条目：同选区文本，但文章全文变化后评价结果会过期，需一并失效。 */
interface CacheEntry {
  states: TaskStates;
  fullText: string;
}

export type { TaskStates };

/**
 * 选区辅助的任务编排：三个任务并行、同选区（选区文本为键）LRU 缓存、
 * 选区切换中止旧请求、单任务重试与整体重新生成。
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

  const setEnabled = useCallback((config: AssistTaskConfig) => {
    enabledRef.current = config;
  }, []);

  /** 停用一个任务：中止在途请求并复位该任务状态（区块随配置移除）。 */
  const disableTask = useCallback((task: AssistTask) => {
    controllersRef.current[task]?.abort();
    delete controllersRef.current[task];
    setTasks((prev) => ({ ...prev, [task]: { ...idleState } }));
  }, []);

  const abortAll = useCallback(() => {
    for (const controller of Object.values(controllersRef.current)) {
      controller?.abort();
    }
    controllersRef.current = {};
  }, []);

  const writeCache = useCallback(
    (key: string, task: AssistTask, state: TaskState, fullText: string) => {
      const cache = cacheRef.current;
      let entry = cache.get(key);
      if (!entry || entry.fullText !== fullText) {
        if (!entry && cache.size >= CACHE_CAPACITY) {
          const oldest = cache.keys().next().value;
          if (oldest !== undefined) cache.delete(oldest);
        }
        entry = { states: allIdle(), fullText };
        cache.set(key, entry);
      }
      entry.states = { ...entry.states, [task]: state };
    },
    [],
  );

  const runTask = useCallback(
    (task: AssistTask, run: RunContext) => {
      controllersRef.current[task]?.abort();
      const controller = new AbortController();
      controllersRef.current[task] = controller;

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
            setTasks((prev) => ({
              ...prev,
              [task]: {
                ...prev[task],
                streamText: prev[task].streamText + text,
              },
            }));
          },
          onDone: (result: AssistResult) => {
            const done: TaskState = { ...idleState, status: 'done', result };
            setTasks((prev) => ({ ...prev, [task]: done }));
            writeCache(key, task, done, run.fullText);
          },
          onError: (message) => {
            setTasks((prev) => ({
              ...prev,
              [task]: { ...idleState, status: 'error', error: message },
            }));
          },
        },
        controller.signal,
      );
    },
    [writeCache],
  );

  const assist = useCallback(
    (anchor: AssistAnchor, fullText: string) => {
      abortAll();
      lastRunRef.current = { anchor, fullText };

      const cached = cacheRef.current.get(anchor.text);
      if (cached && cached.fullText === fullText) {
        // LRU touch：重新插入刷新顺序
        cacheRef.current.delete(anchor.text);
        cacheRef.current.set(anchor.text, cached);
        setTasks(cached.states);
        return;
      }
      if (cached) {
        // 同选区但文章全文已变化：评价结果过期，整条失效
        cacheRef.current.delete(anchor.text);
      }

      setTasks(allIdle());
      const run: RunContext = { anchor, fullText };
      ASSIST_TASKS.filter((task) => enabledRef.current[task]).forEach((task) => {
        void runTask(task, run);
      });
    },
    [abortAll, runTask],
  );

  /** 单任务重试：只重建该任务的请求，不影响其他任务。 */
  const retry = useCallback(
    (task: AssistTask) => {
      const last = lastRunRef.current;
      if (!last) return;
      void runTask(task, last);
    },
    [runTask],
  );

  /** 重新生成：清除该选区缓存并重跑全部任务。 */
  const regenerate = useCallback(() => {
    const last = lastRunRef.current;
    if (!last) return;
    cacheRef.current.delete(last.anchor.text);
    assist(last.anchor, last.fullText);
  }, [assist]);

  /** 选区清空/面板关闭：中止全部请求并复位状态。 */
  const reset = useCallback(() => {
    abortAll();
    lastRunRef.current = null;
    setTasks(allIdle());
  }, [abortAll]);

  return { tasks, assist, retry, regenerate, reset, setEnabled, disableTask };
}
