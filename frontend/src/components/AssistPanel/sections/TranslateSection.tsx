import type { HardWord, TaskState, TranslateResult } from '../types';

function isTranslateResult(result: unknown): result is TranslateResult {
  return typeof result === 'object' && result !== null && 'translation' in result;
}

function RetryButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="px-2 py-1 text-xs rounded-md border border-stone-300 text-stone-700 hover:bg-stone-50 transition-colors"
    >
      重试
    </button>
  );
}


interface Segment {
  text: string;
  word?: HardWord;
}

/** 计算难词在译文中的标注区间：长词优先、每个词只标首处、区间不重叠。 */
function buildSegments(translation: string, words: HardWord[]): Segment[] {
  const taken: Array<[number, number]> = [];
  const ranges: Array<{ start: number; end: number; word: HardWord }> = [];

  for (const word of [...words].sort((a, b) => b.word.length - a.word.length)) {
    if (!word.word) continue;
    let from = 0;
    for (;;) {
      const idx = translation.indexOf(word.word, from);
      if (idx === -1) break;
      const end = idx + word.word.length;
      const overlaps = taken.some(([s, e]) => idx < e && end > s);
      if (!overlaps) {
        taken.push([idx, end]);
        ranges.push({ start: idx, end, word });
        break;
      }
      from = idx + 1;
    }
  }

  ranges.sort((a, b) => a.start - b.start);

  const segments: Segment[] = [];
  let cursor = 0;
  for (const range of ranges) {
    if (range.start < cursor) continue;
    if (range.start > cursor) segments.push({ text: translation.slice(cursor, range.start) });
    segments.push({ text: translation.slice(range.start, range.end), word: range.word });
    cursor = range.end;
  }
  if (cursor < translation.length) segments.push({ text: translation.slice(cursor) });
  return segments;
}

export function TranslateSection({
  state,
  onRetry,
}: {
  state: TaskState;
  onRetry?: () => void;
}) {
  if (state.status === 'loading') {
    return (
      <div className="text-sm text-stone-400 break-all whitespace-pre-wrap max-h-40 overflow-y-auto">
        {state.streamText || '生成中...'}
      </div>
    );
  }
  if (state.status === 'error') {
    return (
      <div className="text-sm text-red-600 space-y-2">
        <div>{state.error}</div>
        {onRetry && <RetryButton onClick={onRetry} />}
      </div>
    );
  }
  const result = state.status === 'done' ? state.result : null;
  if (!result || !isTranslateResult(result)) {
    return <div className="text-sm text-stone-400">等待选区</div>;
  }

  const segments = buildSegments(result.translation, result.hard_words);
  return (
    <p className="text-sm leading-relaxed whitespace-pre-wrap break-words text-stone-800">
      {segments.map((seg, i) =>
        seg.word ? (
          <span key={i} className="group relative border-b border-dotted border-blue-400 cursor-help">
            {seg.text}
            <span className="absolute left-0 bottom-full mb-1 hidden group-hover:block z-10 w-max max-w-[280px] px-2 py-1 rounded bg-stone-900 text-white text-xs leading-relaxed shadow-lg whitespace-normal">
              <span className="text-stone-400">{seg.word.source || seg.word.word}</span>
              {seg.word.gloss ? ` · ${seg.word.gloss}` : ''}
            </span>
          </span>
        ) : (
          <span key={i}>{seg.text}</span>
        ),
      )}
    </p>
  );
}
