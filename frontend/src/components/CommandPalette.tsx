import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  FileText,
  GitBranch,
  Home,
  Layers,
  Mail,
  Pen,
  Search,
  Send,
  Server,
  Settings,
  Upload,
  type LucideIcon,
} from 'lucide-react';

import { get } from '../utils/api';
import { createNewPost, publishSystem, pushLatestEmail, pushSpecificEmail } from '../utils/actions';
import type { PalettePost } from './PaletteBridgeContext';

interface Command {
  group: string;
  icon: LucideIcon;
  label: string;
  hint?: string;
  run: () => void | Promise<void>;
}

interface SearchHit {
  path: string;
  title: string;
}

// ⌘K 命令面板:页面跳转 + 快捷操作 + 文章搜索(对齐重设计原型,功能真实)。
// 由父组件条件挂载,关闭即卸载,状态天然重置。
//
// 编辑器通过 PaletteBridgeContext 注册插入回调后,onInsertPost 非空,
// 面板进入"插入引用"模式:文章条目排在最前、回车把引用插到光标处而非跳转,
// 空查询时先展示最近文章,省一次输入。
export default function CommandPalette({
  onClose,
  onInsertPost,
}: {
  onClose: () => void;
  onInsertPost?: (post: PalettePost) => void;
}) {
  const navigate = useNavigate();
  const insertMode = !!onInsertPost;
  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  // hits 与产出它的查询绑定,渲染时按当前查询过滤,天然过期即失效
  const [search, setSearch] = useState<{ q: string; items: SearchHit[] }>({ q: '', items: [] });
  const searchSeq = useRef(0);
  // 插入模式下打开面板(空查询)时先展示的最近文章
  const [recent, setRecent] = useState<SearchHit[]>([]);

  const commands = useMemo<Command[]>(
    () => [
      { group: '操作', icon: Pen, label: '新建文章', hint: 'N', run: () => createNewPost(navigate) },
      { group: '操作', icon: Upload, label: '系统发布（提交并触发构建）', run: publishSystem },
      { group: '操作', icon: Mail, label: '推送最新文章到订阅者', run: pushLatestEmail },
      { group: '操作', icon: Send, label: '推送指定文章…', run: pushSpecificEmail },
      { group: '页面', icon: Home, label: '仪表板', run: () => navigate('/') },
      { group: '页面', icon: FileText, label: '文章管理', run: () => navigate('/posts') },
      { group: '页面', icon: Pen, label: '编辑器', run: () => navigate('/editor') },
      { group: '页面', icon: Server, label: '服务器控制', run: () => navigate('/server') },
      { group: '页面', icon: GitBranch, label: 'Git', run: () => navigate('/git') },
      { group: '页面', icon: Layers, label: '插件', run: () => navigate('/plugins') },
      { group: '页面', icon: Settings, label: '设置', run: () => navigate('/settings') },
    ],
    [navigate],
  );

  const trimmed = query.trim();
  const filtered = useMemo(() => {
    const q = trimmed.toLowerCase();
    if (!q) return commands;
    return commands.filter((c) => c.label.toLowerCase().includes(q));
  }, [commands, trimmed]);

  // 仅展示由当前查询产出的结果(清空/改词后旧结果自动失效)
  const hits = useMemo(() => (search.q === trimmed ? search.items : []), [search, trimmed]);

  // 插入模式:面板刚打开(空查询)时拉一次最近文章,按日期倒序
  useEffect(() => {
    if (!insertMode) return;
    let cancelled = false;
    get<{ posts?: Array<{ path: string; title: string }> }>('/api/posts?page=1&per_page=15')
      .then((d) => {
        if (!cancelled) setRecent((d.posts || []).map((p) => ({ path: p.path, title: p.title })));
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [insertMode]);

  const articleCommands = useMemo<Command[]>(() => {
    const showRecent = insertMode && !trimmed;
    const list = showRecent ? recent : hits;
    return list.slice(0, 30).map((h) => ({
      group: showRecent ? '最近文章' : '文章',
      icon: FileText,
      label: h.title || h.path,
      hint: h.path,
      run: insertMode && onInsertPost ? () => onInsertPost(h) : () => navigate(`/editor/${h.path}`),
    }));
  }, [insertMode, trimmed, recent, hits, navigate, onInsertPost]);

  // 插入模式文章优先(当前意图是引用);浏览模式维持命令在前
  const items = useMemo(
    () => (insertMode ? [...articleCommands, ...filtered] : [...filtered, ...articleCommands]),
    [filtered, articleCommands, insertMode],
  );

  // 文章搜索(防抖 250ms);结果携带其查询串,竞态响应自然过期
  useEffect(() => {
    const q = trimmed;
    if (!q) return;
    const seq = ++searchSeq.current;
    const timer = setTimeout(() => {
      get<{ success: boolean; posts?: SearchHit[] }>('/api/posts/search?q=' + encodeURIComponent(q))
        .then((d) => {
          if (seq !== searchSeq.current) return;
          setSearch({ q, items: d.posts || [] });
          setActiveIndex(0);
        })
        .catch(() => {
          if (seq !== searchSeq.current) return;
          setSearch({ q, items: [] });
          setActiveIndex(0);
        });
    }, 250);
    return () => clearTimeout(timer);
  }, [trimmed]);

  // 结果集变化后下标可能越界,渲染时收敛(带下界)
  const clampedIndex = Math.max(0, Math.min(activeIndex, items.length - 1));

  const runActive = useCallback(() => {
    const item = items[clampedIndex];
    if (!item) return;
    onClose();
    void item.run();
  }, [items, clampedIndex, onClose]);

  let lastGroup: string | null = null;

  return (
    <div className="fixed inset-0 z-50">
      <div className="absolute inset-0 bg-zinc-900/40 backdrop-blur-[2px]" onClick={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        aria-label="搜索与命令"
        className="relative mx-auto mt-[14vh] w-[min(580px,calc(100vw-32px))] overflow-hidden rounded-xl bg-white shadow-[0_24px_48px_-16px_rgb(24_24_24/0.22),0_0_0_1px_rgb(24_24_24/0.04)]"
      >
        <div className="flex h-[52px] items-center gap-2.5 border-b border-[#e4e4e7] px-4">
          <Search className="h-4 w-4 flex-none text-[#9ca3a8]" />
          <input
            autoFocus
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setActiveIndex(0);
            }}
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown') {
                e.preventDefault();
                setActiveIndex((i) => Math.min(i + 1, items.length - 1));
              } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                setActiveIndex((i) => Math.max(i - 1, 0));
              } else if (e.key === 'Enter') {
                e.preventDefault();
                runActive();
              } else if (e.key === 'Escape') {
                e.preventDefault();
                onClose();
              }
            }}
            placeholder={insertMode ? '搜索文章，回车插入引用…' : '搜索文章、跳转页面、执行操作…'}
            autoComplete="off"
            spellCheck={false}
            className="h-full min-w-0 flex-1 border-0 bg-transparent text-[15px] text-zinc-900 outline-none placeholder:text-[#9ca3a8]"
          />
          <kbd className="kbd">esc</kbd>
        </div>

        <div className="max-h-[336px] overflow-y-auto p-2">
          {items.length === 0 && <div className="py-7 text-center text-[13px] text-[#9ca3a8]">没有匹配的结果</div>}
          {items.map((item, i) => {
            const groupHeader = item.group !== lastGroup ? item.group : null;
            lastGroup = item.group;
            return (
              <div key={`${item.group}-${i}`}>
                {groupHeader && (
                  <div className="px-2.5 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-[0.06em] text-[#9ca3a8]">
                    {groupHeader}
                  </div>
                )}
                <div
                  onMouseMove={() => setActiveIndex(i)}
                  onClick={() => {
                    onClose();
                    void item.run();
                  }}
                  className={`flex h-[38px] cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-[13.5px] text-zinc-900 ${
                    i === clampedIndex ? 'bg-[#f4f4f5]' : ''
                  }`}
                >
                  <item.icon className="h-[15px] w-[15px] flex-none text-[#9ca3a8]" />
                  <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{item.label}</span>
                  {item.hint && <span className="ml-auto flex-none text-[12px] text-[#9ca3a8]">{item.hint}</span>}
                </div>
              </div>
            );
          })}
        </div>

        <div className="flex items-center gap-3.5 border-t border-[#e4e4e7] bg-white px-4 py-2 text-[12px] text-[#9ca3a8]">
          <span>
            <kbd className="kbd mr-1">↑</kbd>
            <kbd className="kbd">↓</kbd> 导航
          </span>
          <span>
            <kbd className="kbd mr-1">↵</kbd>
            {insertMode ? '插入 / 执行' : '选择'}
          </span>
          <span>
            <kbd className="kbd mr-1">esc</kbd>关闭
          </span>
        </div>
      </div>
    </div>
  );
}
