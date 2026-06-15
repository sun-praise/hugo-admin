import { useCallback, useEffect, useRef, useState } from 'react';

import { Popup } from './Popup';
import { Trigger } from './Trigger';

const MIN_SELECTION = 3;
const MAX_SELECTION = 5000;
const DEBOUNCE_MS = 300;
const CONTEXT_RADIUS = 600;
const TRIGGER_SIZE = 36;
const VIEWPORT_MARGIN = 8;

export interface InlineEditOverlayProps {
  /** The textarea that the editor is rendering. */
  textareaRef: React.RefObject<HTMLTextAreaElement | null>;
  /** Source of truth for the editor content (used to slice context). */
  content: string;
  /**
   * Called when the user accepts a rewrite and the selection has not drifted.
   * Receives the revised text and the original anchor range so the host can
   * perform the actual `setContent` + `setSelectionRange`.
   */
  onAccept: (revisedText: string, anchorStart: number, anchorEnd: number) => void;
  /** Called when the user accepts a rewrite but the selection has drifted. */
  onDrift?: () => void;
}

interface Anchor {
  start: number;
  end: number;
  text: string;
  contextBefore: string;
  contextAfter: string;
}

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));

/**
 * Selection-anchored AI rewrite surface for a `<textarea>`.
 *
 * Tracks the textarea's selection; shows a floating trigger near the end of a
 * non-empty selection; opens a popup on click; passes the rewrite result back
 * to the host on accept. Selection drift is caught at accept time by
 * re-reading `textarea.value` and comparing against the snapshotted
 * `selectedText`.
 */
export function InlineEditOverlay({
  textareaRef,
  onAccept,
  onDrift,
}: InlineEditOverlayProps) {
  const [anchor, setAnchor] = useState<Anchor | null>(null);
  const [open, setOpen] = useState(false);
  const aliveRef = useRef(true);

  useEffect(() => {
    return () => {
      aliveRef.current = false;
    };
  }, []);

  const closeAll = useCallback(() => {
    setOpen(false);
    setAnchor(null);
  }, []);

  const onSelectionChange = useCallback(() => {
    if (!aliveRef.current) return;
    const ta = textareaRef.current;
    if (!ta) return;
    if (open) return; // freeze anchor while popup is open
    const start = ta.selectionStart;
    const end = ta.selectionEnd;
    if (start === end) {
      setAnchor(null);
      return;
    }
    const text = ta.value.substring(start, end);
    if (text.length < MIN_SELECTION || text.length > MAX_SELECTION) {
      setAnchor(null);
      return;
    }
    setAnchor({
      start,
      end,
      text,
      contextBefore: ta.value.substring(Math.max(0, start - CONTEXT_RADIUS), start),
      contextAfter: ta.value.substring(end, Math.min(ta.value.length, end + CONTEXT_RADIUS)),
    });
  }, [textareaRef, open]);

  useEffect(() => {
    const ta = textareaRef.current;
    if (!ta) return;
    let timer: number | null = null;
    const schedule = () => {
      if (timer !== null) window.clearTimeout(timer);
      timer = window.setTimeout(() => {
        timer = null;
        onSelectionChange();
      }, DEBOUNCE_MS);
    };
    const events: (keyof HTMLElementEventMap)[] = ['select', 'keyup', 'mouseup', 'focus'];
    events.forEach((e) => ta.addEventListener(e, schedule));
    return () => {
      if (timer !== null) window.clearTimeout(timer);
      events.forEach((e) => ta.removeEventListener(e, schedule));
    };
  }, [textareaRef, onSelectionChange]);

  // Hide on scroll: position would go stale.
  useEffect(() => {
    const ta = textareaRef.current;
    if (!ta) return;
    const onScroll = () => closeAll();
    ta.addEventListener('scroll', onScroll, { passive: true });
    return () => ta.removeEventListener('scroll', onScroll);
  }, [textareaRef, closeAll]);

  // Click outside the popup → close.
  useEffect(() => {
    if (!open) return;
    const onMouseDown = (e: MouseEvent) => {
      const target = e.target as HTMLElement | null;
      if (!target) return;
      if (target.closest('[data-inline-edit-root="true"]')) return;
      closeAll();
    };
    document.addEventListener('mousedown', onMouseDown);
    return () => document.removeEventListener('mousedown', onMouseDown);
  }, [open, closeAll]);

  const handleAccept = useCallback(
    (revised: string) => {
      const ta = textareaRef.current;
      if (!ta || !anchor) {
        closeAll();
        return;
      }
      const current = ta.value.substring(anchor.start, anchor.end);
      if (current !== anchor.text) {
        onDrift?.();
        closeAll();
        return;
      }
      onAccept(revised, anchor.start, anchor.end);
      closeAll();
    },
    [textareaRef, anchor, onAccept, onDrift, closeAll],
  );

  if (!anchor) return null;
  const ta = textareaRef.current;
  if (!ta) return null;

  // Approximate the viewport position of the selection end.
  // We don't have line-level pixel coords for a textarea, so we use a
  // best-effort approach: number of newlines up to `end` × line-height,
  // plus a small horizontal offset. This is rough but stable enough for a
  // 36×36 button and matches what the user sees.
  const rect = ta.getBoundingClientRect();
  const cs = window.getComputedStyle(ta);
  const lineHeight = parseFloat(cs.lineHeight) || 22;
  const paddingTop = parseFloat(cs.paddingTop) || 0;
  const paddingLeft = parseFloat(cs.paddingLeft) || 0;
  const textBeforeEnd = ta.value.substring(0, anchor.end);
  const linesBefore = textBeforeEnd.split('\n').length - 1;
  const lastLine = textBeforeEnd.split('\n').pop() || '';
  // Rough character width for monospace 14px ≈ 8.4px. We don't have
  // canvas-measured char width, so this is a heuristic. The trigger clamps
  // itself to the viewport, so being slightly off is fine.
  const charWidth = 8.4;
  const approxLeft = rect.left + paddingLeft + Math.min(lastLine.length, 80) * charWidth + 6;
  const approxTop = rect.top + paddingTop + linesBefore * lineHeight + 2;

  if (open) {
    const popupWidth = 320;
    const left = clamp(
      approxLeft,
      VIEWPORT_MARGIN,
      Math.max(VIEWPORT_MARGIN, window.innerWidth - popupWidth - VIEWPORT_MARGIN),
    );
    const popupHeightEstimate = 280;
    const below = approxTop + 8;
    const top =
      below + popupHeightEstimate > window.innerHeight
        ? Math.max(VIEWPORT_MARGIN, approxTop - popupHeightEstimate - 8)
        : below;
    return (
      <div data-inline-edit-root="true">
        <Popup
          selectedText={anchor.text}
          contextBefore={anchor.contextBefore}
          contextAfter={anchor.contextAfter}
          top={top}
          left={left}
          onClose={closeAll}
          onAccept={handleAccept}
        />
      </div>
    );
  }

  // Trigger mode
  const triggerTop = clamp(
    approxTop - 2,
    VIEWPORT_MARGIN,
    Math.max(VIEWPORT_MARGIN, window.innerHeight - TRIGGER_SIZE - VIEWPORT_MARGIN),
  );
  const triggerLeft = clamp(
    approxLeft,
    VIEWPORT_MARGIN,
    Math.max(VIEWPORT_MARGIN, window.innerWidth - TRIGGER_SIZE - VIEWPORT_MARGIN),
  );
  return (
    <div data-inline-edit-root="true">
      <Trigger
        top={triggerTop}
        left={triggerLeft}
        onClick={() => setOpen(true)}
      />
    </div>
  );
}
