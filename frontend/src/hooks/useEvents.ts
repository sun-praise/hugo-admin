import { useEffect, useRef } from 'react';
import { io, type Socket } from 'socket.io-client';

// 内部存储类型：never 参数使任意具体类型的 handler 都可赋值（逆变）
type Handler = (data: never) => void;

export interface EventsClient {
  on<T>(name: string, cb: (data: T) => void): void;
  off<T>(name: string, cb: (data: T) => void): void;
  emit(name: string, ...args: unknown[]): void;
}

/**
 * 实时事件抽象：优先 SSE（/api/events，Go 服务），失败自动降级
 * socket.io（Python 服务）。灰度期前后端任意组合可用。
 *
 * - server_log 等推送事件在两种模式下事件名一致
 * - request_logs 在 SSE 模式降级为 GET /api/server/logs，
 *   响应中的 logs 数组逐条派发为 server_log（与 socket 模式的
 *   单条推送形状一致）
 */
class SSEWithFallback implements EventsClient {
  private es: EventSource | null = null;
  private socket: Socket | null = null;
  private handlers = new Map<string, Set<Handler>>();
  private sseListeners = new Map<string, (ev: MessageEvent) => void>();
  private sseReady = false;
  private closed = false;

  constructor() {
    this.connectSSE();
  }

  private connectSSE() {
    const es = new EventSource('/api/events');
    this.es = es;
    es.addEventListener('connected', () => {
      this.sseReady = true;
    });
    es.onerror = () => {
      if (this.sseReady || this.closed) return; // 已连上后交给浏览器自动重连
      // 连接失败（如 Python 服务无此端点）→ 降级 socket.io
      es.close();
      this.es = null;
      this.sseListeners.clear();
      this.connectSocketIO();
    };
    // 续订已注册的事件
    for (const name of this.handlers.keys()) this.subscribeSSE(name);
  }

  private subscribeSSE(name: string) {
    if (!this.es) return;
    const listener = (ev: MessageEvent) => {
      let data: unknown = ev.data as unknown;
      try {
        data = JSON.parse(ev.data) as unknown;
      } catch {
        /* 非 JSON 载荷原样传递 */
      }
      this.dispatch(name, data);
    };
    this.sseListeners.set(name, listener);
    this.es.addEventListener(name, listener as EventListener);
  }

  private connectSocketIO() {
    const socket = io(window.location.origin);
    this.socket = socket;
    socket.on('connect', () => {
      for (const name of this.handlers.keys()) {
        for (const cb of this.handlers.get(name)!) {
          socket.on(name, cb as (...args: unknown[]) => void);
        }
      }
    });
  }

  private dispatch(name: string, data: unknown) {
    const set = this.handlers.get(name);
    if (!set) return;
    for (const cb of set) (cb as (d: unknown) => void)(data);
  }

  on<T>(name: string, cb: (data: T) => void): void {
    if (!this.handlers.has(name)) this.handlers.set(name, new Set());
    this.handlers.get(name)!.add(cb as Handler);
    if (this.es) this.subscribeSSE(name);
    if (this.socket) this.socket.on(name, cb as never);
  }

  off<T>(name: string, cb: (data: T) => void): void {
    const listener = this.sseListeners.get(name);
    if (this.es && listener) this.es.removeEventListener(name, listener as EventListener);
    this.sseListeners.delete(name);
    this.handlers.get(name)?.delete(cb as Handler);
    if (this.socket) this.socket.off(name, cb as never);
  }

  emit(name: string, ...args: unknown[]): void {
    if (name === 'request_logs' && this.sseReady) {
      // SSE 模式：request_logs 降级为 REST 拉取，逐条派发
      fetch('/api/server/logs')
        .then((r) => r.json())
        .then((d) => {
          if (d && Array.isArray(d.logs)) {
            for (const entry of d.logs) this.dispatch('server_log', entry);
          }
        })
        .catch(() => {
          /* 拉取失败静默 */
        });
      return;
    }
    if (this.socket) this.socket.emit(name, ...args);
  }

  close() {
    this.closed = true;
    this.es?.close();
    this.socket?.disconnect();
  }
}

export function useEvents() {
  const ref = useRef<EventsClient | null>(null);
  useEffect(() => {
    const client = new SSEWithFallback();
    ref.current = client;
    return () => {
      client.close();
      ref.current = null;
    };
  }, []);
  return ref;
}
