import { Check, Loader2, Lock } from 'lucide-react';

import { DiffText } from '../DiffText';
import {
  POLISH_STYLE_LABELS,
  type PolishResult,
  type TaskState,
} from '../types';

function isPolishResult(result: unknown): result is PolishResult {
  return typeof result === 'object' && result !== null && 'candidates' in result;
}

interface Props {
  state: TaskState;
  selectedText: string;
  /** 已应用的候选下标；null 表示尚未应用。 */
  appliedIndex: number | null;
  onApply: (index: number) => void;
  onRetry?: () => void;
}

export function PolishSection({ state, selectedText, appliedIndex, onApply, onRetry }: Props) {
  if (state.status === 'loading') {
    return (
      <div className="flex items-center gap-2 text-sm text-stone-400">
        <Loader2 className="w-4 h-4 animate-spin" />
        正在生成 3 个候选...
      </div>
    );
  }
  if (state.status === 'error') {
    return (
      <div className="text-sm text-red-600 space-y-2">
        <div>{state.error}</div>
        {onRetry && (
          <button
            type="button"
            onClick={onRetry}
            className="px-2 py-1 text-xs rounded-md border border-stone-300 text-stone-700 hover:bg-stone-50 transition-colors"
          >
            重试
          </button>
        )}
      </div>
    );
  }
  if (state.status === 'idle' || !state.result || !isPolishResult(state.result)) {
    return <div className="text-sm text-stone-400">等待选区</div>;
  }

  return (
    <div className="space-y-3">
      {state.result.candidates.map((candidate, i) => {
        const locked = appliedIndex !== null && appliedIndex !== i;
        const applied = appliedIndex === i;
        return (
          <div
            key={i}
            className={`rounded-lg border p-3 ${
              applied ? 'border-blue-300 bg-blue-50/50' : 'border-stone-200'
            } ${locked ? 'opacity-50' : ''}`}
          >
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs font-medium text-stone-500">
                {POLISH_STYLE_LABELS[candidate.style] || candidate.style}
                {candidate.note ? ` · ${candidate.note}` : ''}
              </span>
              <button
                type="button"
                disabled={locked || applied}
                onClick={() => onApply(i)}
                className="flex items-center gap-1 px-2 py-1 text-xs rounded-md border border-stone-300 text-stone-700 hover:bg-stone-50 transition-colors disabled:cursor-not-allowed disabled:hover:bg-transparent"
              >
                {applied ? (
                  <>
                    <Check className="w-3.5 h-3.5 text-blue-600" /> 已应用
                  </>
                ) : locked ? (
                  <>
                    <Lock className="w-3.5 h-3.5" /> 已锁定
                  </>
                ) : (
                  '应用'
                )}
              </button>
            </div>
            <DiffText original={selectedText} revised={candidate.text} />
          </div>
        );
      })}
    </div>
  );
}
