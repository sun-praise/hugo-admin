/**
 * 从不完整的 JSON 流中尽力抽取 translation 字符串片段。
 *
 * 后端 prompt 要求模型输出严格 JSON（{"translation": ..., "hard_words": ...}），
 * SSE loading 期直接渲染原始流会让用户先看到 JSON 原文；这里对半截流做
 * 容错解析：找到 "translation" 字段后按 JSON 字符串转义规则逐字符还原，
 * 未闭合（流还没到）就返回已到达的部分。
 */

export function extractPartialTranslation(raw: string): string | null {
  const key = raw.indexOf('"translation"');
  if (key === -1) return null;
  const colon = raw.indexOf(':', key);
  if (colon === -1) return null;
  const quote = raw.indexOf('"', colon + 1);
  if (quote === -1) return null;

  let out = '';
  for (let i = quote + 1; i < raw.length; i++) {
    const ch = raw[i];
    if (ch === '\\') {
      // 流尾孤立反斜杠：转义不完整，丢弃并等下一帧补全
      if (i + 1 >= raw.length) continue;
      const next = raw[++i];
      switch (next) {
        case 'n':
          out += '\n';
          break;
        case 't':
          out += '\t';
          break;
        case 'r':
          out += '\r';
          break;
        case 'b':
          out += '\b';
          break;
        case 'f':
          out += '\f';
          break;
        case 'u': {
          const hex = raw.slice(i + 1, i + 5);
          if (/^[0-9a-fA-F]{4}$/.test(hex)) {
            out += String.fromCharCode(parseInt(hex, 16));
            i += 4;
          } else {
            out += 'u';
          }
          break;
        }
        default:
          out += next; // \" \\ \/ 等
      }
      continue;
    }
    if (ch === '"') break; // 字符串闭合，抽取完成
    out += ch;
  }
  return out;
}
