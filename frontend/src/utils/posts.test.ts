import { describe, expect, it } from 'vitest';
import { normalizePost, normalizePostsResponse, type RawPostsResponse } from './posts';

const base = {
  total: 1,
  page: 1,
  per_page: 20,
  total_pages: 1,
  has_next: false,
  has_prev: false,
};

const rawPost = {
  path: 'post/no-taxonomies.md',
  title: '无标签',
  date: '2025-01-01',
  excerpt: '正文',
};

describe('normalizePostsResponse', () => {
  it('把 null 的 tags / categories 归一化成空数组', () => {
    const data = normalizePostsResponse({
      ...base,
      posts: [{ ...rawPost, tags: null, categories: null }],
    });
    expect(data.posts[0].tags).toEqual([]);
    expect(data.posts[0].categories).toEqual([]);
  });

  it('字段缺失时也返回可安全解引用的空数组', () => {
    const data = normalizePostsResponse({ ...base, posts: [{ ...rawPost }] });
    expect(data.posts[0].tags).toEqual([]);
    expect(data.posts[0].categories).toEqual([]);
  });

  it('posts 为 null / 非数组时返回空列表，并保留分页元信息', () => {
    for (const posts of [null, undefined, 'oops'] as unknown[]) {
      const data = normalizePostsResponse({ ...base, posts } as RawPostsResponse);
      expect(data.posts).toEqual([]);
      expect(data.total).toBe(1);
      expect(data.total_pages).toBe(1);
    }
  });

  it('过滤数组里的非字符串项，正常标签保持原顺序', () => {
    const data = normalizePostsResponse({
      ...base,
      posts: [{
        ...rawPost,
        tags: ['Hugo', null, 42, 'Go'],
        categories: ['笔记', {}],
      }] as unknown as RawPostsResponse['posts'],
    });
    expect(data.posts[0].tags).toEqual(['Hugo', 'Go']);
    expect(data.posts[0].categories).toEqual(['笔记']);
  });

  it('posts 数组里的 null / 非对象元素被跳过，不拖垮整页', () => {
    const data = normalizePostsResponse({
      ...base,
      posts: [null, undefined, 'oops', { ...rawPost, tags: null }] as unknown as RawPostsResponse['posts'],
    });
    expect(data.posts).toHaveLength(1);
    expect(data.posts[0].title).toBe('无标签');
    expect(data.posts[0].tags).toEqual([]);
  });

  it('分页字段缺失 / 非数值时从已返回的 posts 推导，不出现「共 0 篇」却列出文章', () => {
    const data = normalizePostsResponse({
      posts: [{ ...rawPost, tags: null }],
      total: undefined,
      total_pages: NaN,
      has_next: 'yes' as unknown as boolean,
    });
    expect(data).toMatchObject({
      total: 1,
      page: 1,
      per_page: 1,
      total_pages: 1,
      has_next: false,
      has_prev: false,
    });
  });

  it('posts 数组里的数组元素被跳过，不会 spread 成空白行', () => {
    const data = normalizePostsResponse({
      ...base,
      posts: [[], [1, 2], { ...rawPost, tags: null }] as unknown as RawPostsResponse['posts'],
    });
    expect(data.posts).toHaveLength(1);
    expect(data.posts[0].title).toBe('无标签');
  });

  it('响应体本身为 null / undefined 时返回可用的空响应', () => {
    for (const raw of [null, undefined]) {
      expect(normalizePostsResponse(raw)).toMatchObject({
        posts: [],
        total: 0,
        page: 1,
        per_page: 20,
        total_pages: 0,
        has_next: false,
        has_prev: false,
      });
    }
  });

  it('page 缺失或非法时回落到请求页码，不把用户弹回第 1 页', () => {
    for (const page of [undefined, 0, NaN, '2']) {
      const data = normalizePostsResponse(
        { ...base, page: page as number, posts: [{ ...rawPost }] },
        3,
      );
      expect(data.page).toBe(3);
    }
    // 服务端明确回传合法页码时仍以服务端为准
    expect(normalizePostsResponse({ ...base, page: 2, posts: [] }, 3).page).toBe(2);
  });

  it('缺 total_pages / has_next 时按 total 与 per_page 推导出多页', () => {
    const data = normalizePostsResponse({
      posts: [{ ...rawPost }],
      total: 45,
      per_page: 20,
      page: 1,
    });
    expect(data.total_pages).toBe(3);
    expect(data.has_next).toBe(true);
    expect(data.has_prev).toBe(false);
  });

  it('第 2 页缺分页字段时派生总页数不小于当前页码', () => {
    const data = normalizePostsResponse(
      { posts: [{ ...rawPost }, { ...rawPost, path: 'post/b.md' }] },
      2,
    );
    expect(data).toMatchObject({
      page: 2,
      total: 2,
      total_pages: 2,
      has_prev: true,
      has_next: false,
    });
  });

  it('page 回落时重算 has_prev / has_next，不被陈旧的 false 锁死', () => {
    const data = normalizePostsResponse({
      posts: [{ ...rawPost }],
      total: 45,
      per_page: 20,
      total_pages: 3,
      has_prev: false,
      has_next: false,
    }, 2);
    expect(data.page).toBe(2);
    expect(data.has_prev).toBe(true);
    expect(data.has_next).toBe(true);
  });

  it('顶层数组响应体按空响应处理，不把数字键铺进返回值', () => {
    const data = normalizePostsResponse([{ ...rawPost }] as unknown as RawPostsResponse);
    expect(data.posts).toEqual([]);
    expect(data.total).toBe(0);
    expect(data.total_pages).toBe(0);
    expect(data).not.toHaveProperty('0');
  });

  it('正常响应保持字段不变', () => {
    const data = normalizePostsResponse({
      ...base,
      posts: [{ ...rawPost, tags: ['Hugo'], categories: ['笔记'], description: '摘要' }],
    });
    expect(data.posts[0]).toMatchObject({
      path: 'post/no-taxonomies.md',
      title: '无标签',
      description: '摘要',
      tags: ['Hugo'],
      categories: ['笔记'],
    });
  });
});

describe('normalizePost', () => {
  it('单独归一化一篇文章时不影响其它字段', () => {
    expect(normalizePost({ ...rawPost, tags: null })).toEqual({
      ...rawPost,
      tags: [],
      categories: [],
    });
  });
});
