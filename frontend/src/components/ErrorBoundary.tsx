import { Component, type ErrorInfo, type ReactNode } from 'react';

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

// Root-level boundary. The realistic trigger: after a redeploy, a stale
// index.html still references old hashed chunks and a React.lazy import
// rejects on navigation — Suspense handles pending imports, not rejected
// ones, and without a boundary React unmounts the whole tree (blank page).
// A reload fetches the new index.html and recovers.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('UI crashed:', error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex h-screen flex-col items-center justify-center gap-4 p-6 text-center">
          <h1 className="text-lg font-medium text-stone-800">页面出错了</h1>
          <p className="max-w-md text-sm break-all text-stone-500">
            {this.state.error.message || String(this.state.error)}
          </p>
          <button
            type="button"
            onClick={() => window.location.reload()}
            className="rounded-lg bg-stone-800 px-4 py-2 text-sm text-white hover:bg-stone-700"
          >
            重新加载
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
