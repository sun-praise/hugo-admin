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
