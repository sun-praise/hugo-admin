import { useState } from 'react';
import { initProject } from '../utils/api';

export default function ProjectInitPage() {
  const [initPath, setInitPath] = useState('');
  const [initFormat, setInitFormat] = useState<'toml' | 'yaml'>('toml');
  const [initLoading, setInitLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');
  const [lastResult, setLastResult] = useState<{ path: string; config_format: string } | null>(null);

  function showNotification(message: string, type: 'info' | 'success' | 'error' | 'warning' = 'info') {
    const notification = document.createElement('div');
    notification.className = `fixed top-4 right-4 px-6 py-3 rounded-lg shadow-lg z-50 fade-in ${
      type === 'success' ? 'bg-green-500' : type === 'error' ? 'bg-red-500' : type === 'warning' ? 'bg-yellow-500' : 'bg-blue-500'
    } text-white`;
    notification.textContent = message;
    document.body.appendChild(notification);
    setTimeout(() => notification.remove(), 3000);
  }

  async function handleInitProject(e: React.FormEvent) {
    e.preventDefault();
    if (initLoading || !initPath.trim()) return;
    setInitLoading(true);
    setErrorMessage('');
    setLastResult(null);
    try {
      const data = await initProject({ path: initPath.trim(), config_format: initFormat });
      if (!data.success) {
        throw new Error(data.message || '初始化失败');
      }
      showNotification(`站点已创建: ${data.path}`, 'success');
      setLastResult({ path: data.path || '', config_format: data.config_format || '' });
      setInitPath('');
    } catch (error) {
      setErrorMessage((error as Error).message);
      showNotification((error as Error).message, 'error');
    } finally {
      setInitLoading(false);
    }
  }

  return (
    <div className="max-w-3xl space-y-6">
      <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-6">
        <h3 className="text-lg font-medium mb-2">初始化 Hugo 项目</h3>
        <p className="text-sm text-stone-500 mb-6">
          在指定路径创建全新的 Hugo 站点，并将其设为当前活跃项目。该操作会写入文件系统，请谨慎使用。
        </p>

        <form onSubmit={handleInitProject} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-stone-700 mb-2">目标路径</label>
            <input
              type="text"
              value={initPath}
              onChange={(e) => setInitPath(e.target.value)}
              placeholder="/path/to/new-hugo-site"
              className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 font-mono text-sm"
            />
            <p className="mt-2 text-xs text-stone-500">新 Hugo 站点的绝对路径，父目录必须已存在。</p>
          </div>
          <div>
            <label className="block text-sm font-medium text-stone-700 mb-2">配置文件格式</label>
            <select
              value={initFormat}
              onChange={(e) => setInitFormat(e.target.value as 'toml' | 'yaml')}
              className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 text-sm"
            >
              <option value="toml">TOML</option>
              <option value="yaml">YAML</option>
            </select>
          </div>

          {errorMessage && <p className="text-sm text-red-600">{errorMessage}</p>}

          {lastResult && (
            <div className="p-3 bg-green-50 border border-green-200 rounded-lg text-sm text-green-800">
              <p>站点已创建：<code className="font-mono">{lastResult.path}</code></p>
              <p>配置文件格式：{lastResult.config_format}</p>
            </div>
          )}

          <div className="flex justify-end">
            <button
              type="submit"
              disabled={initLoading || !initPath.trim()}
              className="px-6 py-2 bg-stone-800 text-white rounded-lg hover:bg-stone-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {initLoading ? '初始化中...' : '创建站点'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
