import { Sparkles } from 'lucide-react';

export interface TriggerProps {
  /** Viewport-relative top (px). */
  top: number;
  /** Viewport-relative left (px). */
  left: number;
  onClick: () => void;
}

/**
 * Small floating ✨ button shown at the user's text selection.
 * Position is viewport-relative, so we use position: fixed.
 */
export function Trigger({ top, left, onClick }: TriggerProps) {
  return (
    <button
      type="button"
      aria-label="AI 改写"
      title="AI 改写"
      onClick={onClick}
      onMouseDown={(e) => e.preventDefault()}
      style={{ top, left }}
      className="fixed z-50 h-9 w-9 rounded-full p-0 shadow-lg bg-stone-900 text-white hover:bg-stone-700 transition-transform hover:scale-105 flex items-center justify-center"
    >
      <Sparkles className="h-5 w-5" />
    </button>
  );
}
