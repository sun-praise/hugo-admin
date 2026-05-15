import { useState, useEffect, useRef } from 'react';
import { Play, Bug, Square, ExternalLink, Trash2 } from 'lucide-react';
import { get, post } from '../utils/api';
import { useSocket } from '../hooks/useSocket';
import type { ServerStatus, LogEntry } from '../types';

export default function ServerPage() {
  const [status, setStatus] = useState<ServerStatus>({
    running: false,
    pid: null,
    uptime: undefined,
    cpu_percent: undefined,
    memory_mb: undefined,
  });
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const logContainerRef = useRef<HTMLDivElement>(null);
  const socketRef = useSocket();

  useEffect(() => {
    fetchStatus();
    const interval = setInterval(fetchStatus, 3000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    const socket = socketRef.current;
    if (!socket) return;

    const handleLog = (data: LogEntry) => {
      setLogs((prev) => [...prev, data]);
    };

    socket.on('server_log', handleLog);
    socket.emit('request_logs');

    return () => {
      socket.off('server_log', handleLog);
    };
  }, [socketRef]);

  useEffect(() => {
    if (logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight;
    }
  }, [logs]);

  async function fetchStatus() {
    try {
      const data = await get<ServerStatus>('/api/server/status');
      setStatus(data);
    } catch (error) {
      console.error('Failed to fetch status:', error);
    }
  }

  async function startServer(debug = false) {
    setLoading(true);
    try {
      const data = await post<{ success: boolean; status?: ServerStatus; message?: string }>('/api/server/start', { debug });
      if (data.success && data.status) {
        showNotification('服务器启动成功', 'success');
        setStatus(data.status);
      } else {
        showNotification('启动失败: ' + data.message, 'error');
      }
    } catch (error) {
      showNotification('启动失败', 'error');
    } finally {
      setLoading(false);
    }
  }

  async function stopServer() {
    setLoading(true);
    try {
      const data = await post<{ success: boolean; status?: ServerStatus; message?: string }>('/api/server/stop');
      if (data.success && data.status) {
        showNotification('服务器已停止', 'success');
        setStatus(data.status);
      } else {
        showNotification('停止失败: ' + data.message, 'error');
      }
    } catch (error) {
      showNotification('停止失败', 'error');
    } finally {
      setLoading(false);
    }
  }

  function clearLogs() {
    setLogs([]);
  }

  function showNotification(message: string, type: 'info' | 'success' | 'error' | 'warning' = 'info') {
    const notification = document.createElement('div');
    notification.className = `fixed top-4 right-4 px-6 py-3 rounded-lg shadow-lg z-50 fade-in ${
      type === 'success' ? 'bg-green-500' : type === 'error' ? 'bg-red-500' : type === 'warning' ? 'bg-yellow-500' : 'bg-blue-500'
    } text-white`;
    notification.textContent = message;
    document.body.appendChild(notification);
    setTimeout(() => notification.remove(), 3000);
  }

  const logLevelColors: Record<string, string> = {
    SUCCESS: 'text-green-400',
    ERROR: 'text-red-400',
    WARNING: 'text-yellow-400',
    INFO: 'text-gray-300',
  };

  return (
    <div>
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div>
            <h3 className="text-lg font-semibold mb-4">服务器状态</h3>
            <div className="space-y-3">
              <div className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                <span className="text-gray-700">运行状态:</span>
                <span className={`font-semibold flex items-center ${status.running ? 'text-green-600' : 'text-gray-500'}`}>
                  <span className={`w-2 h-2 rounded-full mr-2 ${status.running ? 'bg-green-500' : 'bg-gray-400'}`} />
                  {status.running ? '运行中' : '已停止'}
                </span>
              </div>
              {status.running && (
                <>
                  <div className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                    <span className="text-gray-700">进程 PID:</span>
                    <span className="font-mono">{status.pid}</span>
                  </div>
                  <div className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                    <span className="text-gray-700">运行时间:</span>
                    <span className="font-mono">{status.uptime || '-'}</span>
                  </div>
                  <div className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                    <span className="text-gray-700">CPU 使用率:</span>
                    <span className="font-mono">{status.cpu_percent ? status.cpu_percent + '%' : '-'}</span>
                  </div>
                  <div className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                    <span className="text-gray-700">内存使用:</span>
                    <span className="font-mono">{status.memory_mb ? status.memory_mb + ' MB' : '-'}</span>
                  </div>
                </>
              )}
            </div>
          </div>

          <div>
            <h3 className="text-lg font-semibold mb-4">操作控制</h3>
            <div className="space-y-3">
              <button
                onClick={() => startServer(false)}
                disabled={status.running || loading}
                className={`w-full px-6 py-3 text-white rounded-lg transition-colors flex items-center justify-center ${
                  status.running || loading ? 'bg-gray-400 cursor-not-allowed' : 'bg-green-600 hover:bg-green-700'
                }`}
              >
                <Play className="w-5 h-5 mr-2" />
                {loading ? '启动中...' : '启动服务器'}
              </button>
              <button
                onClick={() => startServer(true)}
                disabled={status.running || loading}
                className={`w-full px-6 py-3 text-white rounded-lg transition-colors flex items-center justify-center ${
                  status.running || loading ? 'bg-gray-400 cursor-not-allowed' : 'bg-blue-600 hover:bg-blue-700'
                }`}
              >
                <Bug className="w-5 h-5 mr-2" />
                {loading ? '启动中...' : '启动服务器 (草稿模式)'}
              </button>
              <button
                onClick={stopServer}
                disabled={!status.running || loading}
                className={`w-full px-6 py-3 text-white rounded-lg transition-colors flex items-center justify-center ${
                  !status.running || loading ? 'bg-gray-400 cursor-not-allowed' : 'bg-red-600 hover:bg-red-700'
                }`}
              >
                <Square className="w-5 h-5 mr-2" />
                {loading ? '停止中...' : '停止服务器'}
              </button>
              {status.running && (
                <a
                  href="http://192.168.2.14:1313"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="block w-full px-6 py-3 bg-purple-600 text-white text-center rounded-lg hover:bg-purple-700 transition-colors"
                >
                  <span className="flex items-center justify-center">
                    <ExternalLink className="w-5 h-5 mr-2" />
                    访问网站
                  </span>
                </a>
              )}
            </div>
          </div>
        </div>
      </div>

      <div className="bg-gray-900 rounded-lg shadow overflow-hidden">
        <div className="px-6 py-4 bg-gray-800 border-b border-gray-700 flex items-center justify-between">
          <h3 className="text-white font-semibold">服务器日志</h3>
          <button
            onClick={clearLogs}
            className="px-3 py-1 text-xs bg-gray-700 text-white rounded hover:bg-gray-600 transition-colors flex items-center"
          >
            <Trash2 className="w-3 h-3 mr-1" />
            清空日志
          </button>
        </div>
        <div ref={logContainerRef} className="p-4 h-96 overflow-y-auto font-mono text-sm text-gray-300 bg-gray-900">
          {logs.length === 0 && <p className="text-gray-500">暂无日志输出</p>}
          {logs.map((log, index) => (
            <div key={index} className="mb-1 hover:bg-gray-800 px-2 py-1 rounded">
              <span className="text-gray-500">[{log.timestamp}]</span>{' '}
              <span className={logLevelColors[log.level] || 'text-gray-300'}>{log.message}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
