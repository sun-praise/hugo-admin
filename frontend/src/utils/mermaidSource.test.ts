import { describe, expect, it } from 'vitest';

import { MermaidSourceCache, type MermaidNodeLike } from './mermaidSource';

class FakeNode implements MermaidNodeLike {
  textContent: string | null;
  removed: string[] = [];

  constructor(source: string | null) {
    this.textContent = source;
  }

  removeAttribute(name: string): void {
    this.removed.push(name);
  }
}

describe('MermaidSourceCache', () => {
  it('首次登记源码，节点被 SVG 覆盖后仍返回原始源码', () => {
    const node = new FakeNode('graph TD\nA-->B');
    const cache = new MermaidSourceCache();

    expect(cache.source(node)).toBe('graph TD\nA-->B');
    node.textContent = '<svg><text>graph TD</text></svg>';
    expect(cache.source(node)).toBe('graph TD\nA-->B');
  });

  it('restore 把节点还原成源码并清掉 data-processed', () => {
    const node = new FakeNode('sequenceDiagram\nA->>B: hi');
    const cache = new MermaidSourceCache();
    cache.source(node); // 首次渲染前登记
    node.textContent = '<svg>rendered</svg>'; // mermaid 改写了节点

    const code = cache.restore(node);
    expect(code).toBe('sequenceDiagram\nA->>B: hi');
    expect(node.textContent).toBe(code);
    expect(node.removed).toEqual(['data-processed']);
  });

  it('未见过节点也能 restore：现场读取源码', () => {
    const node = new FakeNode('pie title Pets\n"Dogs" : 386');
    const cache = new MermaidSourceCache();

    expect(cache.restore(node)).toBe('pie title Pets\n"Dogs" : 386');
  });

  it('空内容安全降级为空串，节点之间互不串源码', () => {
    const cache = new MermaidSourceCache();
    const empty = new FakeNode(null);
    const other = new FakeNode('graph LR\nA-->B');

    expect(cache.source(empty)).toBe('');
    expect(cache.source(other)).toBe('graph LR\nA-->B');
    expect(cache.source(empty)).toBe('');
  });
});
