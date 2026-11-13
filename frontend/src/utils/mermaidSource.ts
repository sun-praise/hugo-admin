export interface MermaidNodeLike {
  textContent: string | null;
  removeAttribute(name: string): void;
}

/**
 * mermaid.run() 会把 .mermaid 节点的内容替换成渲染后的 SVG（或错误块）。预览栏
 * 收起再展开、窄屏切页签时，渲染 effect 会在同一批节点上重跑；此时若直接读
 * textContent，就会把 SVG 文本当成图表定义重新解析。
 *
 * 按节点缓存首次见到的源码，重跑前还原。只依赖最小接口而非真实 DOM Element，
 * 这样这部分逻辑可以在 node 环境下单测。
 */
export class MermaidSourceCache {
  private readonly sources = new WeakMap<object, string>();

  /** 首次见到节点时登记源码；之后始终返回首次登记的值。 */
  source(node: MermaidNodeLike): string {
    let code = this.sources.get(node);
    if (code === undefined) {
      code = node.textContent ?? '';
      this.sources.set(node, code);
    }
    return code;
  }

  /** 还原节点为原始源码并清掉 mermaid 的处理标记，返回可用的图表定义。 */
  restore(node: MermaidNodeLike): string {
    const code = this.source(node);
    node.textContent = code;
    node.removeAttribute('data-processed');
    return code;
  }
}
