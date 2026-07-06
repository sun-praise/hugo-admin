import { useMemo } from 'react';
import { AlertTriangle, Save, RotateCcw, X } from 'lucide-react';

interface ConflictModalProps {
  localContent: string;
  remoteContent: string;
  onSaveForce: () => void;
  onDiscard: () => void;
  onClose: () => void;
}

interface DiffLine {
  type: 'add' | 'del' | 'same';
  text: string;
  oldNum?: number;
  newNum?: number;
}

function computeDiff(oldText: string, newText: string): DiffLine[] {
  const oldLines = oldText.split('\n');
  const newLines = newText.split('\n');

  // Simple LCS-based diff
  const m = oldLines.length;
  const n = newLines.length;
  const dp: number[][] = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));

  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      dp[i][j] =
        oldLines[i - 1] === newLines[j - 1]
          ? dp[i - 1][j - 1] + 1
          : Math.max(dp[i - 1][j], dp[i][j - 1]);
    }
  }

  const result: DiffLine[] = [];
  let i = m,
    j = n;
  const buf: DiffLine[] = [];

  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && oldLines[i - 1] === newLines[j - 1]) {
      buf.push({ type: 'same', text: oldLines[i - 1], oldNum: i, newNum: j });
      i--;
      j--;
    } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
      buf.push({ type: 'add', text: newLines[j - 1], newNum: j });
      j--;
    } else {
      buf.push({ type: 'del', text: oldLines[i - 1], oldNum: i });
      i--;
    }
  }

  buf.reverse().forEach((line) => result.push(line));
  return result;
}

export function ConflictModal({
  localContent,
  remoteContent,
  onSaveForce,
  onDiscard,
  onClose,
}: ConflictModalProps) {
  const diffLines = useMemo(
    () => computeDiff(remoteContent, localContent),
    [remoteContent, localContent],
  );

  const stats = useMemo(() => {
    let adds = 0;
    let dels = 0;
    diffLines.forEach((l) => {
      if (l.type === 'add') adds++;
      if (l.type === 'del') dels++;
    });
    return { adds, dels };
  }, [diffLines]);

  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center bg-black/50">
      <div className="bg-white rounded-xl shadow-2xl w-full max-w-4xl mx-4 max-h-[85vh] flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b">
          <div className="flex items-center gap-3">
            <AlertTriangle className="w-6 h-6 text-amber-500" />
            <div>
              <h3 className="text-lg font-semibold text-stone-900">文件冲突</h3>
              <p className="text-sm text-stone-500">
                文件已被其他人修改。请查看差异后选择操作。
                <span className="ml-2 text-green-600">+{stats.adds}</span>
                <span className="ml-1 text-red-600">-{stats.dels}</span>
              </p>
            </div>
          </div>
          <button onClick={onClose} className="text-stone-400 hover:text-stone-600">
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Diff view */}
        <div className="flex-1 overflow-auto font-mono text-sm">
          <table className="w-full border-collapse">
            <tbody>
              {diffLines.map((line, idx) => {
                const bg =
                  line.type === 'add'
                    ? 'bg-green-50'
                    : line.type === 'del'
                      ? 'bg-red-50'
                      : '';
                const prefix =
                  line.type === 'add' ? '+' : line.type === 'del' ? '-' : ' ';
                const prefixColor =
                  line.type === 'add'
                    ? 'text-green-600'
                    : line.type === 'del'
                      ? 'text-red-600'
                      : 'text-stone-400';
                return (
                  <tr key={idx} className={bg}>
                    <td className="w-12 px-2 py-0.5 text-right text-stone-400 select-none border-r border-stone-200">
                      {line.oldNum ?? ''}
                    </td>
                    <td className="w-12 px-2 py-0.5 text-right text-stone-400 select-none border-r border-stone-200">
                      {line.newNum ?? ''}
                    </td>
                    <td className="w-6 px-1 py-0.5 text-center select-none">
                      <span className={prefixColor}>{prefix}</span>
                    </td>
                    <td className="px-2 py-0.5 whitespace-pre-wrap break-all">
                      {line.text || '\u00A0'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-6 py-4 border-t bg-stone-50">
          <p className="text-sm text-stone-500">
            左列为服务端最新版本，右列为你的本地修改
          </p>
          <div className="flex gap-3">
            <button
              onClick={onDiscard}
              className="flex items-center gap-2 px-4 py-2 border border-stone-300 rounded-lg text-stone-700 hover:bg-stone-100 transition-colors"
            >
              <RotateCcw className="w-4 h-4" />
              放弃我的修改
            </button>
            <button
              onClick={onSaveForce}
              className="flex items-center gap-2 px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors"
            >
              <Save className="w-4 h-4" />
              覆盖保存
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
