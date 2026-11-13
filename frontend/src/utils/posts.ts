import type { Post, PostsResponse } from '../types';

/**
 * /api/posts 的原始响应形状。
 *
 * Go 侧 `[]string` 为 nil 时 encoding/json 会输出 `null`，老版本后端、
 * 代理缓存或手工构造的响应里都可能出现；组件侧则统一按 `Post` 使用，
 * 会在 `post.tags.length` / `post.tags.slice()` 上直接解引用。
 * 后端已在序列化边界保证空数组，这里是第二道防线：即便拿到旧数据，
 * 文章列表也不会整页崩掉。
 */
export interface RawPost extends Omit<Post, 'tags' | 'categories'> {
  tags?: string[] | null;
  categories?: string[] | null;
}

// 分页字段也允许缺失：旧后端 / 代理返回半截响应时，不能让页面显示
// 「共 undefined 篇文章」。缺省值与空列表的语义保持一致。
export interface RawPostsResponse extends Partial<Omit<PostsResponse, 'posts'>> {
  posts?: RawPost[] | null;
}

function toStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  // 只保留字符串：null / 对象既不满足 Post 的类型，也会成为不稳定的 React key。
  return value.filter((item): item is string => typeof item === 'string');
}

function isRawPost(value: unknown): value is RawPost {
  // 数组也是 object，放行会 spread 出一条所有字段都是 undefined 的空白文章。
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function intOr(value: unknown, fallback: number, min: number): number {
  if (typeof value === 'number' && Number.isInteger(value) && value >= min) return value;
  // fallback 也可能来自不可信的调用方（如页码参数），用同一套判据规范化，
  // 避免 normalizePostsResponse(raw, 0) 直接产出 page: 0。
  return typeof fallback === 'number' && Number.isInteger(fallback) && fallback >= min
    ? fallback
    : min;
}

export function normalizePost(raw: RawPost): Post {
  return {
    ...raw,
    tags: toStringArray(raw.tags),
    categories: toStringArray(raw.categories),
  };
}

/**
 * requestedPage 是当前请求的页码：后端不回 `page` 时用它回落，避免
 * Posts.tsx 的「服务端页码与请求不符就跟随纠正」把用户弹回第 1 页。
 */
export function normalizePostsResponse(
  raw: RawPostsResponse | null | undefined,
  requestedPage = 1,
): PostsResponse {
  // 整个响应体为 null / undefined 时（代理异常、旧后端返回 `null`），
  // 仍给出一份可用的空响应，而不是在 Dashboard 的 async effect 里抛错。
  // 与 isRawPost 保持一致：顶层数组同样是 object，展开会把数字键铺进返回值。
  const source: RawPostsResponse = raw && typeof raw === 'object' && !Array.isArray(raw) ? raw : {};
  const posts = Array.isArray(source.posts)
    ? source.posts.filter(isRawPost).map(normalizePost)
    : [];

  const total = intOr(source.total, posts.length, 0);
  const perPage = intOr(source.per_page, posts.length || 20, 1);
  // 服务端没给总页数时只能按本页条数估：总量为 0 派生 0，否则向上取整。
  const derivedTotalPages = total > 0 ? Math.ceil(total / perPage) : 0;
  const totalPages = intOr(source.total_pages, derivedTotalPages, 0);

  let page = intOr(source.page, requestedPage, 1);
  // 只要页码超过总页数就收敛到最后一页，与「服务端有没有回显 page」无关：
  // 服务端可能回显合法 page 却缺分页字段（派生出 total_pages = 1），也可能
  // 自相矛盾（Go 后端不 clamp page，page=5 / totalPages=3 时回空列表）。
  // 收敛后避免「第 2 / 1 页」「第 5 / 3 页」以及分页按钮被 clamp 锁死。
  if (totalPages > 0 && page > totalPages) {
    page = totalPages;
  }

  // has_next / has_prev 一律由页码推导：Go 后端用的也是
  // page < totalPages / page > 1，前后端同构；旧后端给陈旧的 false
  // 不会把「上一页 / 下一页」锁死。
  return {
    ...source,
    posts,
    total,
    page,
    per_page: perPage,
    total_pages: totalPages,
    has_next: page < totalPages,
    has_prev: page > 1,
  };
}
