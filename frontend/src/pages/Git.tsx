import { useState, useEffect, useCallback } from 'react';
import { GitCommit, GitBranch, RefreshCw, ChevronLeft, ChevronRight, CheckCircle2, XCircle } from 'lucide-react';
import { getCommits, getPushes } from '../utils/api';
import type { Commit, PushRecord } from '../utils/api';

type Tab = 'commits' | 'pushes';

const COMMIT_PAGE_SIZE = 20;
const PUSH_PAGE_SIZE = 20;

export default function Git() {
  const [tab, setTab] = useState<Tab>('commits');

  // commits state
  const [commits, setCommits] = useState<Commit[]>([]);
  const [commitsLoading, setCommitsLoading] = useState(false);
  const [commitsCount, setCommitsCount] = useState(COMMIT_PAGE_SIZE);
  const [commitsHasMore, setCommitsHasMore] = useState(true);

  // pushes state
  const [pushes, setPushes] = useState<PushRecord[]>([]);
  const [pushPage, setPushPage] = useState(1);
  const [pushPagination, setPushPagination] = useState({ total: 0, total_pages: 0, has_next: false, has_prev: false });
  const [pushesLoading, setPushesLoading] = useState(false);

  const loadCommits = useCallback(async (count: number) => {
    setCommitsLoading(true);
    try {
      const data = await getCommits(count);
      if (data.success) {
        setCommits(data.commits);
        // 后端钳制到 [1, 50]，若返回少于请求数量则没有更多
        setCommitsHasMore(data.commits.length >= count && count < 50);
      }
    } catch (e) {
      console.error('Failed to load commits:', e);
    } finally {
      setCommitsLoading(false);
    }
  }, []);

  const loadPushes = useCallback(async (page: number) => {
    setPushesLoading(true);
    try {
      const data = await getPushes(page, PUSH_PAGE_SIZE);
      if (data.success) {
        setPushes(data.pushes);
        setPushPage(data.page);
        setPushPagination({
          total: data.total,
          total_pages: data.total_pages,
          has_next: data.page < data.total_pages,
          has_prev: data.page > 1,
        });
      }
    } catch (e) {
      console.error('Failed to load pushes:', e);
    } finally {
      setPushesLoading(false);
    }
  }, []);

  // 仅在 tab 切换时拉取对应数据；loadCommits/loadPushes 为 useCallback([]) 稳定引用，
  // commitsCount/pushPage 在切 tab 时读取最新值即可，故依赖数组只含 [tab]。
  useEffect(() => {
    if (tab === 'commits') {
      loadCommits(commitsCount);
    } else {
      loadPushes(pushPage);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab]);

  function refresh() {
    if (tab === 'commits') loadCommits(commitsCount);
    else loadPushes(pushPage);
  }

  function loadMoreCommits() {
    const next = Math.min(commitsCount + COMMIT_PAGE_SIZE, 50);
    setCommitsCount(next);
    loadCommits(next);
  }

  function goPushPage(p: number) {
    const target = Math.max(1, Math.min(p, pushPagination.total_pages || 1));
    if (target !== pushPage) loadPushes(target);
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-stone-800">Git</h2>
        <button
          onClick={refresh}
          disabled={tab === 'commits' ? commitsLoading : pushesLoading}
          className="flex items-center gap-2 px-3 py-2 text-sm bg-white ring-1 ring-stone-900/5 rounded-lg hover:bg-stone-50 transition-colors disabled:opacity-50"
        >
          <RefreshCw className={`w-4 h-4 ${(tab === 'commits' ? commitsLoading : pushesLoading) ? 'animate-spin' : ''}`} />
          刷新
        </button>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 mb-6 bg-stone-100 p-1 rounded-lg w-fit">
        <button
          onClick={() => setTab('commits')}
          className={`flex items-center gap-2 px-4 py-2 rounded-md text-sm transition-colors ${
            tab === 'commits' ? 'bg-white text-stone-800 shadow-sm font-medium' : 'text-stone-500 hover:text-stone-700'
          }`}
        >
          <GitCommit className="w-4 h-4" />
          提交记录
        </button>
        <button
          onClick={() => setTab('pushes')}
          className={`flex items-center gap-2 px-4 py-2 rounded-md text-sm transition-colors ${
            tab === 'pushes' ? 'bg-white text-stone-800 shadow-sm font-medium' : 'text-stone-500 hover:text-stone-700'
          }`}
        >
          <GitBranch className="w-4 h-4" />
          推送记录
        </button>
      </div>

      {tab === 'commits' ? (
        <CommitsView
          commits={commits}
          loading={commitsLoading}
          hasMore={commitsHasMore}
          onLoadMore={loadMoreCommits}
        />
      ) : (
        <PushesView
          pushes={pushes}
          loading={pushesLoading}
          pagination={pushPagination}
          page={pushPage}
          onPage={goPushPage}
        />
      )}
    </div>
  );
}

function CommitsView({
  commits,
  loading,
  hasMore,
  onLoadMore,
}: {
  commits: Commit[];
  loading: boolean;
  hasMore: boolean;
  onLoadMore: () => void;
}) {
  if (loading && commits.length === 0) {
    return <EmptyState text="加载提交记录..." />;
  }
  if (commits.length === 0) {
    return <EmptyState text="暂无提交记录" />;
  }
  return (
    <div className="bg-white rounded-md ring-1 ring-stone-900/5 overflow-hidden">
      <ul className="divide-y divide-stone-100">
        {commits.map((c) => (
          <li key={c.hash} className="p-4 hover:bg-stone-50 transition-colors">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0 flex-1">
                <p className="font-medium text-stone-800 truncate">{c.message}</p>
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1 mt-1 text-sm text-stone-500">
                  <span className="font-mono text-xs text-stone-600">{c.hash.slice(0, 7)}</span>
                  <span>{c.author}</span>
                  <span title={c.date}>{relativeFromNow(c.date)}</span>
                  {c.refs && (
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 bg-blue-50 text-blue-700 rounded text-xs">
                      <GitBranch className="w-3 h-3" />
                      {c.refs}
                    </span>
                  )}
                </div>
              </div>
              <div className="flex items-center gap-3 text-xs font-mono whitespace-nowrap shrink-0">
                <span className="text-stone-500">{c.stats.files} 文件</span>
                <span className="text-green-600">+{c.stats.insertions}</span>
                <span className="text-red-600">-{c.stats.deletions}</span>
              </div>
            </div>
          </li>
        ))}
      </ul>
      {hasMore && (
        <div className="p-3 border-t border-stone-100 text-center">
          <button
            onClick={onLoadMore}
            disabled={loading}
            className="px-4 py-2 text-sm text-stone-600 hover:text-stone-900 hover:bg-stone-50 rounded-lg transition-colors disabled:opacity-50"
          >
            {loading ? '加载中...' : '加载更多'}
          </button>
        </div>
      )}
    </div>
  );
}

function PushesView({
  pushes,
  loading,
  pagination,
  page,
  onPage,
}: {
  pushes: PushRecord[];
  loading: boolean;
  pagination: { total: number; total_pages: number; has_next: boolean; has_prev: boolean };
  page: number;
  onPage: (p: number) => void;
}) {
  if (loading && pushes.length === 0) {
    return <EmptyState text="加载推送记录..." />;
  }
  if (pushes.length === 0) {
    return <EmptyState text="暂无推送记录" />;
  }
  return (
    <div className="bg-white rounded-md ring-1 ring-stone-900/5 overflow-hidden">
      <ul className="divide-y divide-stone-100">
        {pushes.map((p) => {
          const range = p.from_sha && p.to_sha ? `${p.from_sha.slice(0, 7)}..${p.to_sha.slice(0, 7)}` : p.to_sha ? `${p.to_sha.slice(0, 7)}` : '—';
          return (
            <li key={p.id} className="p-4 hover:bg-stone-50 transition-colors">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    {p.success ? (
                      <CheckCircle2 className="w-4 h-4 text-green-600 shrink-0" />
                    ) : (
                      <XCircle className="w-4 h-4 text-red-600 shrink-0" />
                    )}
                    <p className={`font-medium truncate ${p.success ? 'text-stone-800' : 'text-red-700'}`}>
                      {p.commit_message || (p.success ? '推送成功' : '推送失败')}
                    </p>
                  </div>
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1 mt-1 text-sm text-stone-500">
                    <span className="inline-flex items-center gap-1">
                      <GitBranch className="w-3 h-3" />
                      {p.remote}/{p.branch}
                    </span>
                    <span className="font-mono text-xs">{range}</span>
                    {p.success && p.commit_count > 0 && <span>{p.commit_count} 次提交</span>}
                    <span title={p.pushed_at_iso}>{relativeFromNow(p.pushed_at)}</span>
                  </div>
                  {!p.success && p.message && (
                    <p className="mt-1 text-xs text-red-600 truncate">{p.message}</p>
                  )}
                </div>
              </div>
            </li>
          );
        })}
      </ul>

      {pagination.total_pages > 1 && (
        <div className="flex items-center justify-between p-3 border-t border-stone-100">
          <button
            onClick={() => onPage(page - 1)}
            disabled={!pagination.has_prev || loading}
            className="flex items-center gap-1 px-3 py-1.5 text-sm text-stone-600 rounded hover:bg-stone-50 disabled:opacity-40 disabled:hover:bg-transparent"
          >
            <ChevronLeft className="w-4 h-4" />
            上一页
          </button>
          <span className="text-sm text-stone-500">
            第 {page} / {pagination.total_pages} 页（共 {pagination.total} 条）
          </span>
          <button
            onClick={() => onPage(page + 1)}
            disabled={!pagination.has_next || loading}
            className="flex items-center gap-1 px-3 py-1.5 text-sm text-stone-600 rounded hover:bg-stone-50 disabled:opacity-40 disabled:hover:bg-transparent"
          >
            下一页
            <ChevronRight className="w-4 h-4" />
          </button>
        </div>
      )}
    </div>
  );
}

function EmptyState({ text }: { text: string }) {
  return (
    <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-12 text-center text-stone-400">
      {text}
    </div>
  );
}

/** ISO 字符串或 epoch 秒 -> "x 分钟前" 风格的相对时间。 */
function relativeFromNow(value: number | string): string {
  const ms = typeof value === 'string' ? new Date(value).getTime() : value * 1000;
  if (Number.isNaN(ms)) return String(value);
  const diff = Date.now() - ms;
  const sec = Math.round(diff / 1000);
  if (sec < 60) return `${sec} 秒前`;
  const min = Math.round(sec / 60);
  if (min < 60) return `${min} 分钟前`;
  const hr = Math.round(min / 60);
  if (hr < 24) return `${hr} 小时前`;
  const day = Math.round(hr / 24);
  if (day < 30) return `${day} 天前`;
  return new Date(ms).toLocaleDateString('zh-CN');
}
