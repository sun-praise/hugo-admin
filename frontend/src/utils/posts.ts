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
  return value !== null && typeof value === 'object';
}

function numberOr(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function boolOr(value: unknown, fallback: boolean): boolean {
  return typeof value === 'boolean' ? value : fallback;
}

export function normalizePost(raw: RawPost): Post {
  return {
    ...raw,
    tags: toStringArray(raw.tags),
    categories: toStringArray(raw.categories),
  };
}

export function normalizePostsResponse(raw: RawPostsResponse): PostsResponse {
  const posts = Array.isArray(raw.posts)
    ? raw.posts.filter(isRawPost).map(normalizePost)
    : [];
  return {
    ...raw,
    posts,
    total: numberOr(raw.total, 0),
    page: numberOr(raw.page, 1),
    per_page: numberOr(raw.per_page, 20),
    total_pages: numberOr(raw.total_pages, 0),
    has_next: boolOr(raw.has_next, false),
    has_prev: boolOr(raw.has_prev, false),
  };
}
