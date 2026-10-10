import { useEffect, useMemo, useRef, useState } from 'react';
import { RefreshCw, SlidersHorizontal, Undo2, X } from 'lucide-react';

import { PolishSection } from './sections/PolishSection';
import { ReviewSection } from './sections/ReviewSection';
import { TranslateSection } from './sections/TranslateSection';
import type { AssistAnchor } from './useAssist';
import type { TaskStates } from './useAssist';
import { ASSIST_TASKS, TASK_LABELS, type AssistTask, type AssistTaskConfig } from './types';
import type { ReplaceRecord } from '../../utils/selectionReplace';

/** Editor 传入的选区：useAssist 的锚点 + textarea 内的位置区间。 */
export interface EditorSelection extends AssistAnchor {
  start: number;
  end: number;
}

interface AssistPanelProps {
  open: boolean;
  selection: EditorSelection | null;
  tasks: TaskStates;
  /** 当前文章全文（评价修复定位用，随编辑实时更新）。 */
  content: string;
  onRetry: (task: AssistTask) => void;
  onRegenerate: () => void;
  onClose: () => void;
  /** 执行替换（含漂移检查与撤销栈登记），失败返回 null。 */
  onApplyReplacement: (
    start: number,
    end: number,
    newText: string,
    expectedOld?: string,
  ) => ReplaceRecord | null;
  undoCount: number;
  /** 撤销最近一次应用并返回被撤销的记录；无可撤销时返回 null。 */
  onUndo: () => ReplaceRecord | null;
  /** 任务启停配置：停用的任务不渲染区块。 */
  taskConfig: AssistTaskConfig;
  onToggleTask: (task: AssistTask, enabled: boolean) => void;
}

function SectionShell({
  title,
  hint,
  children,
}: {
  title: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <section className="border-t border-stone-100 pt-3">
      <div className="flex items-center gap-2 mb-2">
        <h4 className="text-sm font-semibold text-stone-700">{title}</h4>
        {hint && <span className="text-[11px] text-stone-400">{hint}</span>}
      </div>
      {children}
    </section>
  );
}

export function AssistPanel({
  open,
  selection,
  tasks,
  content,
  onRetry,
  onRegenerate,
  onClose,
  onApplyReplacement,
  undoCount,
  onUndo,
  taskConfig,
  onToggleTask,
}: AssistPanelProps) {
  const [appliedIndex, setAppliedIndex] = useState<number | null>(null);
  const [appliedFixes, setAppliedFixes] = useState<Set<number>>(new Set());
  const [configOpen, setConfigOpen] = useState(false);
  // 应用时返回的替换记录，用于撤销时精确匹配该清除哪个锁定/已应用标记
  const polishRecordRef = useRef<ReplaceRecord | null>(null);
  const fixRecordsRef = useRef(new Map<number, ReplaceRecord>());

  // 选区变化时重置应用状态（render 期重置模式，见 react.dev derive-state-from-props）
  const [lastSelectionText, setLastSelectionText] = useState<string | null>(null);
  const selectionText = selection?.text ?? null;
  if (selectionText !== lastSelectionText) {
    setLastSelectionText(selectionText);
    setAppliedIndex(null);
    setAppliedFixes(new Set());
  }

  // ref 不允许在 render 期写入，记录的重置放到 effect 里
  useEffect(() => {
    polishRecordRef.current = null;
    fixRecordsRef.current = new Map();
  }, [selectionText]);

  // 修复项的可定位性随正文变化实时计算。
  const unlocatable = useMemo(() => {
    const result = new Set<number>();
    const review = tasks.review;
    if (review.status === 'done' && review.result && 'issues' in review.result) {
      review.result.issues.forEach((issue, i) => {
        if (issue.before && !content.includes(issue.before)) {
          result.add(i);
        }
      });
    }
    return result;
  }, [tasks.review, content]);

  if (!open) return null;

  const applyCandidate = (index: number) => {
    if (!selection) return;
    const result = tasks.polish.result;
    if (!result || !('candidates' in result)) return;
    const candidate = result.candidates[index];
    if (!candidate) return;
    const record = onApplyReplacement(
      selection.start,
      selection.end,
      candidate.text,
      selection.text,
    );
    if (record) {
      polishRecordRef.current = record;
      setAppliedIndex(index);
    }
  };

  const applyFix = (index: number, issue: { before: string | null; after: string | null }) => {
    if (!issue.before || !issue.after) return;
    const idx = content.indexOf(issue.before);
    if (idx === -1) return;
    const record = onApplyReplacement(idx, idx + issue.before.length, issue.after);
    if (record) {
      fixRecordsRef.current.set(index, record);
      setAppliedFixes((prev) => new Set(prev).add(index));
    }
  };

  // 撤销后按被撤销记录的身份精确清除对应的锁定/已应用标记；
  // 撤销的是别的替换（如某条评价修复）时，润色锁定不受影响。
  const handleUndo = () => {
    const undone = onUndo();
    if (!undone) return;
    if (polishRecordRef.current === undone) {
      polishRecordRef.current = null;
      setAppliedIndex(null);
    }
    for (const [index, record] of fixRecordsRef.current) {
      if (record === undone) {
        fixRecordsRef.current.delete(index);
        setAppliedFixes((prev) => {
          const next = new Set(prev);
          next.delete(index);
          return next;
        });
        break;
      }
    }
  };

  return (
    // 非模态抽屉：不加全屏遮罩，用户可以在面板开着时继续在编辑器里选字
    <div className="fixed top-0 right-0 w-[400px] max-w-[90vw] h-screen bg-white shadow-xl z-[60] flex flex-col border-l border-stone-200">
      <div className="flex items-center justify-between p-4 border-b">
        <div className="min-w-0">
          <h3 className="text-lg font-medium text-stone-900">选区辅助</h3>
          {selection && (
            <p className="text-xs text-stone-400 truncate mt-0.5" title={selection.text}>
              「{selection.text.slice(0, 60)}
              {selection.text.length > 60 ? '…' : ''}」
            </p>
          )}
        </div>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={handleUndo}
            disabled={undoCount === 0}
            title={undoCount > 0 ? `撤销上一次应用（还可撤销 ${undoCount} 步）` : '无可撤销的应用'}
            className="relative h-8 w-8 flex items-center justify-center rounded-md border border-stone-300 text-stone-600 hover:bg-stone-50 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Undo2 className="w-3.5 h-3.5" />
            {undoCount > 0 && (
              <span className="absolute -top-1 -right-1 bg-blue-100 text-blue-800 text-[10px] font-semibold min-w-4 h-4 px-1 flex items-center justify-center rounded-full">
                {undoCount}
              </span>
            )}
          </button>
          <button
            type="button"
            onClick={onRegenerate}
            disabled={!selection}
            title="清除缓存并重新生成全部结果"
            className="h-8 w-8 flex items-center justify-center rounded-md border border-stone-300 text-stone-600 hover:bg-stone-50 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
          <div className="relative">
            <button
              type="button"
              onClick={() => setConfigOpen((v) => !v)}
              title="配置任务（翻译/润色/评价）"
              className={`h-8 w-8 flex items-center justify-center rounded-md border transition-colors ${
                configOpen
                  ? 'border-blue-400 text-blue-600'
                  : 'border-stone-300 text-stone-600 hover:bg-stone-50'
              }`}
            >
              <SlidersHorizontal className="w-3.5 h-3.5" />
            </button>
            {configOpen && (
              <div className="absolute right-0 top-full mt-1 z-10 w-44 rounded-lg border border-stone-200 bg-white p-1.5 shadow-lg">
                {ASSIST_TASKS.map((task) => (
                  <label
                    key={task}
                    className="flex items-center gap-2 px-2 py-1.5 text-sm text-stone-700 hover:bg-stone-50 cursor-pointer rounded"
                  >
                    <input
                      type="checkbox"
                      checked={taskConfig[task]}
                      onChange={(e) => onToggleTask(task, e.target.checked)}
                    />
                    {TASK_LABELS[task]}
                  </label>
                ))}
              </div>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            className="h-8 w-8 ml-1 flex items-center justify-center rounded-md text-stone-400 hover:text-stone-600 hover:bg-stone-50 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>
      </div>

        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          {!selection && (
            <p className="text-sm text-stone-400 text-center py-8">在编辑器中选中文字后自动生成</p>
          )}
          {selection && (
            <>
              {taskConfig.translate && (
                <SectionShell title="翻译" hint="难词悬停查看释义">
                  <TranslateSection state={tasks.translate} onRetry={() => onRetry('translate')} />
                </SectionShell>
              )}
              {taskConfig.polish && (
                <SectionShell title="润色" hint="3 个候选，词级对照">
                  <PolishSection
                    state={tasks.polish}
                    selectedText={selection.text}
                    appliedIndex={appliedIndex}
                    onApply={applyCandidate}
                    onRetry={() => onRetry('polish')}
                  />
                </SectionShell>
              )}
              {taskConfig.review && (
                <SectionShell title="评价" hint="结合全文">
                  <ReviewSection
                    state={tasks.review}
                    appliedFixes={appliedFixes}
                    unlocatable={unlocatable}
                    onApplyFix={applyFix}
                    onRetry={() => onRetry('review')}
                  />
                </SectionShell>
              )}
              {!ASSIST_TASKS.some((task) => taskConfig[task]) && (
                <p className="text-sm text-stone-400 text-center py-6">
                  所有任务已停用，点击上方配置按钮开启
                </p>
              )}
            </>
          )}
        </div>
      </div>
  );
}
