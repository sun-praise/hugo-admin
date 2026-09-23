import { Loader2 } from 'lucide-react';

// Placeholder while a lazy route chunk is loading. Follows the app's existing
// loading convention (RequireAuth's centered 加载中…, Plugins' Loader2 spinner).
export default function Loading({ fullScreen = false }: { fullScreen?: boolean }) {
  return (
    <div
      role="status"
      aria-live="polite"
      className={`flex items-center justify-center gap-2 text-stone-500 ${
        fullScreen ? 'h-screen' : 'py-20'
      }`}
    >
      <Loader2 className="h-6 w-6 animate-spin" />
      <span className="text-sm">加载中…</span>
    </div>
  );
}
