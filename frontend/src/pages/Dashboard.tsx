import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  ChevronRight,
  FileText,
  FolderOpen,
  Mail,
  Pen,
  Search,
  Send,
  Server,
  Tag,
  Upload,
  type LucideIcon,
} from 'lucide-react';

import { get } from '../utils/api';
import { createNewPost, publishSystem, pushLatestEmail, pushSpecificEmail } from '../utils/actions';
import { useMe } from '../hooks/useMe';
import type { GitStatus } from '../utils/api';
import type { Category, Post, PostsResponse, ServerStatus, Tag as TagType } from '../types';

const WEEK_MS = 7 * 24 * 3600 * 1000;

function mondayOf(d: Date): number {
  const day = new Date(d.getFullYear(), d.getMonth(), d.getDate());
  day.setDate(day.getDate() - ((day.getDay() + 6) % 7));
  return day.getTime();
}

function formatMonthDay(date: string): string {
  return date.length >= 10 ? date.slice(5, 10) : date;
}

// 仪表板:对齐 docs/design/admin-redesign-preview.html,数据全部真实。
export default function Dashboard() {
  const navigate = useNavigate();
  const me = useMe();
  const [posts, setPosts] = useState<Post[]>([]);
  const [tags, setTags] = useState<TagType[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [serverStatus, setServerStatus] = useState<ServerStatus>({ running: false, pid: null });
  const [gitStatus, setGitStatus] = useState<GitStatus | null>(null);
  const [version, setVersion] = useState('');

  useEffect(() => {
    let alive = true;
    (async () => {
      // 每个请求独立失败兜底:可选项挂了不影响文章/状态等主数据
      const safe = async <T,>(p: Promise<T>, fallback: T): Promise<T> => {
        try {
          return await p;
        } catch {
          return fallback;
        }
      };
      const [all, tagData, catData, st, gs, ver] = await Promise.all([
        safe(get<PostsResponse>('/api/posts?per_page=1000'), {
          posts: [], total: 0, page: 1, per_page: 0, total_pages: 0, has_next: false, has_prev: false,
        }),
        safe(get<{ tags: TagType[] }>('/api/posts/tags'), { tags: [] }),
        safe(get<{ categories: Category[] }>('/api/posts/categories'), { categories: [] }),
        safe(get<ServerStatus>('/api/server/status'), { running: false, pid: null }),
        safe(get<GitStatus>('/api/git/status'), { success: false } as GitStatus),
        safe(get<{ version: string }>('/api/version'), { version: '' }),
      ]);
      if (!alive) return;
      setPosts(all.posts || []);
      setTags(tagData.tags || []);
      setCategories(catData.categories || []);
      setServerStatus(st);
      setGitStatus(gs);
      setVersion(ver.version || '');
    })();
    return () => {
      alive = false;
    };
  }, []);

  // 页面加载时刻(问候语/本周统计共用),挂载时取一次
  const [now] = useState(() => new Date());
  const thisMonday = mondayOf(now);

  const stats = useMemo(() => {
    const nowMs = now.getTime();
    const drafts = posts.filter((p) => p.status?.is_draft).length;
    const thisWeek = posts.filter((p) => {
      const t = new Date(p.date).getTime();
      return !Number.isNaN(t) && t >= thisMonday && t <= nowMs;
    }).length;
    return { total: posts.length, drafts, thisWeek };
  }, [posts, thisMonday, now]);

  // 近 12 周发布柱状图(原型为 0/1 二值柱)
  const weeklyBars = useMemo(() => {
    const counts = new Array<number>(12).fill(0);
    for (const p of posts) {
      const t = new Date(p.date).getTime();
      if (Number.isNaN(t)) continue;
      const weekIdx = Math.floor((thisMonday - mondayOf(new Date(t))) / WEEK_MS);
      if (weekIdx >= 0 && weekIdx < 12) counts[11 - weekIdx] += 1;
    }
    return counts;
  }, [posts, thisMonday]);

  const recentPosts = posts.slice(0, 15);
  const topTags = [...tags].sort((a, b) => b.count - a.count).slice(0, 3).map((t) => t.name);
  const topCategories = [...categories].sort((a, b) => b.count - a.count).slice(0, 3).map((c) => c.name);
  // git 状态失败时(如非 git 目录)返回体可能缺少数组字段,做防御
  const dirtyCount = gitStatus?.success
    ? (gitStatus.staged?.length ?? 0) + (gitStatus.unstaged?.length ?? 0) + (gitStatus.untracked?.length ?? 0)
    : 0;

  const h = now.getHours();
  const part = h < 6 ? '夜深了' : h < 12 ? '上午好' : h < 18 ? '下午好' : '晚上好';
  const greeting = `${part},${me?.username || '朋友'} · ${now.getMonth() + 1} 月 ${now.getDate()} 日,继续写点东西吧`;

  interface QuickAction {
    icon: LucideIcon;
    label: string;
    hint?: string;
    run: () => void | Promise<void>;
  }
  const quickActions: QuickAction[] = [
    { icon: Pen, label: '新建文章', hint: 'N', run: () => createNewPost(navigate) },
    { icon: Search, label: '浏览文章', hint: `${stats.total} 篇`, run: () => navigate('/posts') },
    { icon: Upload, label: '系统发布', hint: dirtyCount > 0 ? `${dirtyCount} 处待提交` : undefined, run: publishSystem },
    { icon: Mail, label: '推送最新文章', run: pushLatestEmail },
    { icon: Send, label: '推送指定文章', run: pushSpecificEmail },
    { icon: Server, label: '服务器控制', hint: serverStatus.running ? '运行中' : '已停止', run: () => navigate('/server') },
  ];

  return (
    <div>
      <div className="page-head">
        <h1 className="m-0 text-[22px] font-semibold tracking-[-0.02em]">仪表板</h1>
        <p className="mb-0 mt-1 text-[13.5px] text-[#52525b]">{greeting}</p>
      </div>

      {/* 统计卡片 */}
      <section aria-label="统计" className="mb-4 mt-6 grid grid-cols-4 gap-4 max-[1080px]:grid-cols-2">
        {[
          {
            label: '总文章数',
            icon: FileText,
            value: String(stats.total),
            sub: (
              <>
                {stats.thisWeek > 0 && <span className="font-medium text-green-600">本周 +{stats.thisWeek}</span>}
                {stats.thisWeek > 0 && ' · '}草稿 {stats.drafts} 篇
              </>
            ),
          },
          {
            label: '标签',
            icon: Tag,
            value: String(tags.length),
            sub: topTags.length > 0 ? `最常用:${topTags.join(' · ')}` : '—',
          },
          {
            label: '分类',
            icon: FolderOpen,
            value: String(categories.length),
            sub: topCategories.length > 0 ? `${topCategories.join(' · ')} …` : '—',
          },
        ].map((card, i) => (
          <div
            key={card.label}
            className="min-w-0 animate-rise rounded-[10px] border border-[#e4e4e7] bg-white px-[18px] pb-3.5 pt-4"
            style={{ animationDelay: `${i * 0.04}s` }}
          >
            <div className="flex items-center justify-between gap-2">
              <span className="text-[13px] text-[#52525b]">{card.label}</span>
              <span className="grid h-[30px] w-[30px] flex-none place-items-center rounded-lg bg-[#f4f4f5] text-[#52525b]">
                <card.icon className="h-[15px] w-[15px]" />
              </span>
            </div>
            <div className="mt-2.5 flex text-[27px] font-semibold tabular-nums tracking-[-0.02em]">{card.value}</div>
            <div className="mt-0.5 truncate text-[12.5px] tabular-nums text-[#9ca3a8]">{card.sub}</div>
          </div>
        ))}
        <div className="min-w-0 animate-rise rounded-[10px] border border-[#e4e4e7] bg-white px-[18px] pb-3.5 pt-4" style={{ animationDelay: '0.12s' }}>
          <div className="flex items-center justify-between gap-2">
            <span className="text-[13px] text-[#52525b]">Hugo 服务器</span>
            <span className="grid h-[30px] w-[30px] flex-none place-items-center rounded-lg bg-[#f4f4f5] text-[#52525b]">
              <Server className="h-[15px] w-[15px]" />
            </span>
          </div>
          <div
            className={`mt-2.5 flex items-center gap-2 text-[21px] font-semibold tracking-[-0.02em] ${
              serverStatus.running ? 'text-green-600' : 'text-[#9ca3a8]'
            }`}
          >
            <span className={`dot-pulse ${serverStatus.running ? '' : 'is-off'}`} aria-hidden="true" />
            {serverStatus.running ? '运行中' : '已停止'}
          </div>
          <div className="mt-0.5 text-[12.5px] tabular-nums text-[#9ca3a8]">
            {serverStatus.pid != null ? `PID ${serverStatus.pid}` : '—'}
            {serverStatus.uptime ? ` · ${serverStatus.uptime}` : ''}
          </div>
        </div>
      </section>

      <div className="animate-rise grid grid-cols-[minmax(0,1fr)_360px] items-start gap-4 [animation-delay:0.16s] max-[1080px]:grid-cols-1 min-[1440px]:grid-cols-[minmax(0,1fr)_400px]">
        {/* 最近文章 */}
        <section aria-label="最近文章" className="min-w-0 rounded-[10px] border border-[#e4e4e7] bg-white">
          <div className="flex items-center justify-between gap-3 border-b border-[#e4e4e7] py-[13px] pl-5 pr-4">
            <span className="text-[14px] font-semibold tracking-[-0.01em]">最近文章</span>
            <Link to="/posts" className="inline-flex items-center gap-0.5 text-[13px] font-medium text-blue-600 hover:underline">
              查看全部
              <ChevronRight className="h-3.5 w-3.5" />
            </Link>
          </div>
          <div className="grid gap-0.5 p-2">
            {recentPosts.length === 0 && <p className="py-10 text-center text-[13px] text-[#9ca3a8]">暂无文章</p>}
            {recentPosts.map((post) => (
              <button
                key={post.path}
                onClick={() => navigate(`/editor/${post.path}`)}
                className="block w-full min-w-0 cursor-pointer rounded-lg border-0 bg-transparent px-3 py-[11px] text-left transition-colors hover:bg-[#f4f4f5]"
              >
                <div className="flex min-w-0 items-center gap-2.5">
                  <span className="min-w-0 flex-1 truncate text-[14px] font-[550]">{post.title}</span>
                  {post.status?.is_draft && (
                    <span className="flex-none rounded-full bg-amber-50 px-2 py-[1.5px] text-[11.5px] font-medium text-amber-700">
                      草稿
                    </span>
                  )}
                  <span className="flex-none text-[12.5px] tabular-nums text-[#9ca3a8]">{formatMonthDay(post.date)}</span>
                </div>
                <div className="mt-[3px] flex min-w-0 items-center gap-3">
                  <span className="min-w-0 flex-1 truncate text-[13px] text-[#52525b]">{post.excerpt}</span>
                  <span className="flex flex-none gap-1.5">
                    {post.tags.slice(0, 2).map((t) => (
                      <span key={t} className="whitespace-nowrap rounded-full bg-[#f4f4f5] px-2 py-[1.5px] text-[11.5px] text-[#52525b]">
                        {t}
                      </span>
                    ))}
                  </span>
                </div>
              </button>
            ))}
          </div>
        </section>

        {/* 右侧栏 */}
        <div className="grid min-w-0 gap-4">
          <section aria-label="快捷操作" className="rounded-[10px] border border-[#e4e4e7] bg-white">
            <div className="flex items-center justify-between gap-3 border-b border-[#e4e4e7] py-[13px] pl-5 pr-4">
              <span className="text-[14px] font-semibold tracking-[-0.01em]">快捷操作</span>
            </div>
            <div className="grid gap-0.5 p-2">
              {quickActions.map((action) => (
                <button
                  key={action.label}
                  onClick={() => void action.run()}
                  className="flex h-[34px] cursor-pointer items-center gap-2.5 rounded-md border-0 bg-transparent px-2.5 text-left text-[13.5px] font-medium text-zinc-900 transition-colors hover:bg-[#f4f4f5]"
                >
                  <action.icon className="h-[15px] w-[15px] flex-none text-[#9ca3a8]" />
                  {action.label}
                  {action.hint && <span className="ml-auto text-[12px] tabular-nums text-[#9ca3a8]">{action.hint}</span>}
                </button>
              ))}
            </div>
          </section>

          <section aria-label="发布动态" className="rounded-[10px] border border-[#e4e4e7] bg-white">
            <div className="flex items-center justify-between gap-3 border-b border-[#e4e4e7] py-[13px] pl-5 pr-4">
              <span className="text-[14px] font-semibold tracking-[-0.01em]">发布动态</span>
            </div>
            <div className="px-5 pb-3.5 pt-4">
              <div className="flex h-[72px] items-end gap-1.5" role="img" aria-label="近 12 周发布柱状图">
                {weeklyBars.map((count, i) => (
                  <div
                    key={i}
                    className={`min-h-[3px] flex-1 rounded-[3px_3px_2px_2px] ${count > 0 ? 'bg-zinc-900' : 'bg-[#ebebeb]'}`}
                    style={{ height: `${Math.max(3, count * 26)}px` }}
                    title={`${i === 11 ? '本周' : `${11 - i} 周前`}:${count} 篇`}
                  />
                ))}
              </div>
              <div className="mt-2.5 flex justify-between border-t border-dashed border-[#e4e4e7] pt-2.5 text-[12px] tabular-nums text-[#9ca3a8]">
                <span>
                  近 12 周 · 共 <strong className="font-semibold text-zinc-900">{weeklyBars.reduce((a, b) => a + b, 0)}</strong> 篇
                </span>
                <span>
                  本周 <strong className="font-semibold text-zinc-900">{weeklyBars[11]}</strong> 篇
                </span>
              </div>
            </div>
          </section>

          <section aria-label="系统概况" className="rounded-[10px] border border-[#e4e4e7] bg-white">
            <div className="flex items-center justify-between gap-3 border-b border-[#e4e4e7] py-[13px] pl-5 pr-4">
              <span className="text-[14px] font-semibold tracking-[-0.01em]">系统概况</span>
            </div>
            <div className="grid gap-0.5 p-2">
              <div className="flex h-8 items-center justify-between gap-3 rounded-md px-3 text-[13px] text-[#52525b]">
                <span>管理版本</span>
                <strong className="font-medium tabular-nums text-zinc-900">{version ? `v${version}` : '—'}</strong>
              </div>
              <div className="flex h-8 items-center justify-between gap-3 rounded-md px-3 text-[13px] text-[#52525b]">
                <span>待提交改动</span>
                <strong className={`font-medium tabular-nums ${dirtyCount > 0 ? 'text-amber-700' : 'text-zinc-900'}`}>
                  {dirtyCount} 个文件
                </strong>
              </div>
              <div className="flex h-8 items-center justify-between gap-3 rounded-md px-3 text-[13px] text-[#52525b]">
                <span>Hugo 服务器</span>
                <strong className="font-medium tabular-nums text-zinc-900">
                  {serverStatus.running ? '运行中' : '已停止'}
                </strong>
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}
