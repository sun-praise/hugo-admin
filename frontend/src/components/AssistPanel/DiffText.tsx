import { wordDiff } from '../../utils/textDiff';

/** 词级 diff 渲染：新增绿底、删除红底删除线，未变化部分原样。 */
export function DiffText({ original, revised }: { original: string; revised: string }) {
  const segments = wordDiff(original, revised);
  return (
    <p className="text-sm leading-relaxed whitespace-pre-wrap break-words">
      {segments.map((seg, i) => {
        if (seg.kind === 'added') {
          return (
            <mark key={i} className="bg-green-100 text-green-900 rounded px-0.5">
              {seg.text}
            </mark>
          );
        }
        if (seg.kind === 'removed') {
          return (
            <del key={i} className="bg-red-100 text-red-700 rounded px-0.5">
              {seg.text}
            </del>
          );
        }
        return <span key={i}>{seg.text}</span>;
      })}
    </p>
  );
}
