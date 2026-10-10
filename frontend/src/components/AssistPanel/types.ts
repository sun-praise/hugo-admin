export type AssistTask = 'translate' | 'polish' | 'review';

export const ASSIST_TASKS: AssistTask[] = ['translate', 'polish', 'review'];

export interface HardWord {
  word: string;
  source: string;
  gloss: string;
}

export interface TranslateResult {
  translation: string;
  hard_words: HardWord[];
}

export interface PolishCandidate {
  style: string;
  note: string;
  text: string;
}

export interface PolishResult {
  candidates: PolishCandidate[];
}

export interface ReviewIssue {
  type: string;
  severity: 'high' | 'warn' | 'info';
  comment: string;
  before: string | null;
  after: string | null;
}

export interface ReviewResult {
  issues: ReviewIssue[];
}

export type AssistResult = TranslateResult | PolishResult | ReviewResult;

export type TaskStatus = 'idle' | 'loading' | 'done' | 'error';

export interface TaskState {
  status: TaskStatus;
  /** 流式过程中累积的原始文本（渲染预览用）。 */
  streamText: string;
  result: AssistResult | null;
  error: string | null;
}

export const POLISH_STYLE_LABELS: Record<string, string> = {
  formal: '正式书面',
  concise: '精简',
  restructure: '句式重构',
};

export const TASK_LABELS: Record<AssistTask, string> = {
  translate: '翻译',
  polish: '润色',
  review: '评价',
};

/** 任务启停配置：停用的任务不发请求、不渲染区块。翻译默认停用。 */
export type AssistTaskConfig = Record<AssistTask, boolean>;

export const DEFAULT_TASK_CONFIG: AssistTaskConfig = {
  translate: false,
  polish: true,
  review: true,
};

export const REVIEW_TYPE_LABELS: Record<string, string> = {
  overclaim: '断言过强',
  title: '标题一致',
  terminology: '术语一致',
  colloquial: '口语化',
  structure: '结构可读',
  other: '其他',
};
