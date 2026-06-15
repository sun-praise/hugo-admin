export interface InlineEditPreset {
  id: string;
  /** Chinese chip label shown in the popup. */
  label: string;
  /** The instruction text sent to the LLM. */
  instruction: string;
}

export const INLINE_EDIT_PRESETS: InlineEditPreset[] = [
  {
    id: 'polish',
    label: '润色',
    instruction: '请润色这段文字，保留原意，让表达更流畅、更专业。',
  },
  {
    id: 'translate-en',
    label: '翻译为英文',
    instruction: '请将这段文字翻译为英文，保持原意、语气和格式。',
  },
  {
    id: 'shorten',
    label: '精简',
    instruction: '请精简这段文字，保留核心信息，让表达更紧凑。',
  },
  {
    id: 'expand',
    label: '详细化',
    instruction: '请扩写这段文字，补充更多细节和说明。',
  },
];
