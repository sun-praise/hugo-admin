import { Check, Loader2, SearchX } from 'lucide-react';

import {
  REVIEW_TYPE_LABELS,
  type ReviewIssue,
  type ReviewResult,
  type TaskState,
} from '../types';

function isReviewResult(result: unknown): result is ReviewResult {
  return typeof result === 'object' && result !== null && 'issues' in result;
}

interface Props {
  state: TaskState;
  /** 已应用修复的 issue 下标集合。 */
  appliedFixes: Set<number>;
  /** 基于当前正文计算的不可定位 issue 下标集合。 */
  unlocatable: Set<number>;
  onApplyFix: (index: number, issue: ReviewIssue) => void;
  onRetry?: () => void;
}

const SEVERITY_STYLES: Record<string, string> = {
  high: 'bg-red-100 text-red-700',
  warn: 'bg-amber-100 text-amber-700',
  info: 'bg-blue-100 text-blue-700',
};

const SEVERITY_LABELS: Record<string, string> = {
  high: '高',
  warn: '建议',
  info: '提示',
};

function IssueItem({
  issue,
  index,
  applied,
  unlocatable,
  onApplyFix,
}: {
  issue: ReviewIssue;
  index: number;
  applied: boolean;
  unlocatable: boolean;
  onApplyFix: Props['onApplyFix'];
}) {
  const fixable = Boolean(issue.before && issue.after);
  return (
    <div className="rounded-lg border border-stone-200 p-3 space-y-2">
      <div className="flex items-center gap-2 flex-wrap">
        <span className={`px-1.5 py-0.5 rounded text-[11px] font-medium ${SEVERITY_STYLES[issue.severity] || SEVERITY_STYLES.info}`}>
          {SEVERITY_LABELS[issue.severity] || issue.severity}
        </span>
        <span className="text-[11px] text-stone-400">{REVIEW_TYPE_LABELS[issue.type] || issue.type}</span>
        {applied && (
          <span className="flex items-center gap-1 text-[11px] text-blue-600">
            <Check className="w-3 h-3" /> 已应用
          </span>
        )}
      </div>
      <p className="text-sm text-stone-800 leading-relaxed">{issue.comment}</p>
      {fixable && (
        <div className="rounded-md bg-stone-50 p-2 space-y-1">
          <div className="text-xs text-red-600 line-through break-all">{issue.before}</div>
          <div className="text-xs text-green-700 break-all">{issue.after}</div>
          {!applied &&
            (unlocatable ? (
              <span className="flex items-center gap-1 text-[11px] text-stone-400">
                <SearchX className="w-3 h-3" /> 无法定位
              </span>
            ) : (
              <button
                type="button"
                onClick={() => onApplyFix(index, issue)}
                className="px-2 py-1 text-xs rounded-md border border-stone-300 text-stone-700 hover:bg-white transition-colors"
              >
                应用修复
              </button>
            ))}
        </div>
      )}
    </div>
  );
}

export function ReviewSection({ state, appliedFixes, unlocatable, onApplyFix, onRetry }: Props) {
  if (state.status === 'loading') {
    return (
      <div className="flex items-center gap-2 text-sm text-stone-400">
        <Loader2 className="w-4 h-4 animate-spin" />
        正在结合全文审校...
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
  if (state.status === 'idle' || !state.result || !isReviewResult(state.result)) {
    return <div className="text-sm text-stone-400">等待选区</div>;
  }

  if (state.result.issues.length === 0) {
    return <div className="text-sm text-stone-500">没有发现问题</div>;
  }

  return (
    <div className="space-y-3">
      {state.result.issues.map((issue, i) => (
        <IssueItem
          key={i}
          issue={issue}
          index={i}
          applied={appliedFixes.has(i)}
          unlocatable={unlocatable.has(i)}
          onApplyFix={onApplyFix}
        />
      ))}
    </div>
  );
}
