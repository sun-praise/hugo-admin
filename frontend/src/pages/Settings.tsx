import { useState, useEffect } from 'react';
import { get, put } from '../utils/api';
import type { Settings as SettingsType } from '../types';

export default function SettingsPage() {
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');
  const [apiKeyTouched, setApiKeyTouched] = useState(false);
  const [apiKeySource, setApiKeySource] = useState('none');
  const [apiKeyHint, setApiKeyHint] = useState('');
  const [form, setForm] = useState<SettingsType>({
    hugo: { base_dir: '', server_url: '' },
    ai: { base_url: '', model: '', api_key_source: 'none', api_key_hint: '' },
  });

  useEffect(() => {
    fetchSettings();
  }, []);

  async function fetchSettings() {
    setLoading(true);
    setErrorMessage('');
    try {
      const data = await get<{ success: boolean; settings?: SettingsType; message?: string }>('/api/settings');
      if (!data.success) {
        throw new Error(data.message || '加载设置失败');
      }
      const settings = data.settings || { hugo: { base_dir: '', server_url: '' }, ai: { base_url: '', model: '' } };
      setForm({
        hugo: {
          base_dir: settings.hugo?.base_dir || '',
          server_url: settings.hugo?.server_url || '',
        },
        ai: {
          base_url: settings.ai?.base_url || 'https://api.deepseek.com',
          model: settings.ai?.model || 'deepseek-chat',
          api_key_source: settings.ai?.api_key_source || 'none',
          api_key_hint: settings.ai?.api_key_hint || '',
        },
      });
      setApiKeySource(settings.ai?.api_key_source || 'none');
      setApiKeyHint(settings.ai?.api_key_hint || '');
    } catch (error) {
      setErrorMessage((error as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function saveSettings() {
    if (saving || loading) return;
    setSaving(true);
    setErrorMessage('');
    try {
      const payload: SettingsType = {
        hugo: {
          base_dir: form.hugo.base_dir,
          server_url: form.hugo.server_url,
        },
        ai: {
          base_url: form.ai.base_url,
          model: form.ai.model,
        },
      };
      if (apiKeyTouched) {
        payload.ai.api_key = '';
      }
      const data = await put<{ success: boolean; message?: string }>('/api/settings', payload);
      if (!data.success) {
        throw new Error(data.message || '保存设置失败');
      }
      showNotification('设置保存成功', 'success');
      setApiKeyTouched(false);
    } catch (error) {
      setErrorMessage((error as Error).message);
    } finally {
      setSaving(false);
    }
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

  return (
    <div className="max-w-3xl space-y-6">
      {loading && (
        <div className="p-12 text-center">
          <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600" />
          <p className="mt-4 text-gray-600">加载中...</p>
        </div>
      )}

      {!loading && (
        <>
          <div className="bg-white rounded-lg shadow p-6">
            <h3 className="text-lg font-semibold mb-4">Hugo Blog</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">博客根目录</label>
                <input
                  type="text"
                  value={form.hugo.base_dir}
                  onChange={(e) => setForm({ ...form, hugo: { ...form.hugo, base_dir: e.target.value } })}
                  placeholder="/path/to/hugo-blog"
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono text-sm"
                />
                <p className="mt-2 text-xs text-gray-500">Hugo 项目的根目录，需包含 config.toml / config.yaml 等配置文件。</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">Hugo 服务器 URL</label>
                <input
                  type="text"
                  value={form.hugo.server_url}
                  onChange={(e) => setForm({ ...form, hugo: { ...form.hugo, server_url: e.target.value } })}
                  placeholder="http://0.0.0.0:1313"
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono text-sm"
                />
                <p className="mt-2 text-xs text-gray-500">Hugo 预览服务器的基础 URL。留空则使用默认值。</p>
              </div>
            </div>
          </div>

          <div className="bg-white rounded-lg shadow p-6">
            <h3 className="text-lg font-semibold mb-4">AI 助手</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">AI Base URL</label>
                <input
                  type="text"
                  value={form.ai.base_url}
                  onChange={(e) => setForm({ ...form, ai: { ...form.ai, base_url: e.target.value } })}
                  placeholder="https://api.deepseek.com"
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">AI Model</label>
                <input
                  type="text"
                  value={form.ai.model}
                  onChange={(e) => setForm({ ...form, ai: { ...form.ai, model: e.target.value } })}
                  placeholder="deepseek-chat"
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">AI API Key（可选）</label>
                <input
                  type="password"
                  placeholder="留空则保持当前来源"
                  onChange={() => setApiKeyTouched(true)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono text-sm"
                />
                {apiKeySource === 'session' && (
                  <p className="mt-2 text-xs text-gray-500">
                    当前使用会话密钥（重启后失效）
                    {apiKeyHint && `（${apiKeyHint}）`}
                  </p>
                )}
                {apiKeySource === 'env' && <p className="mt-2 text-xs text-gray-500">当前使用环境变量中的密钥</p>}
                {apiKeySource === 'none' && <p className="mt-2 text-xs text-gray-500">当前未配置 AI API Key</p>}
              </div>
              <div className="bg-blue-50 border border-blue-200 rounded-lg p-3 text-sm text-blue-800">
                未保存密钥时会自动回退到环境变量（<code>DEEPSEEK_API_KEY</code> 或 <code>AI_API_KEY</code>）。
              </div>
            </div>
          </div>

          {errorMessage && <p className="text-sm text-red-600">{errorMessage}</p>}

          <div className="flex justify-end">
            <button
              onClick={saveSettings}
              disabled={saving || loading}
              className="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {saving ? '保存中...' : '保存设置'}
            </button>
          </div>
        </>
      )}
    </div>
  );
}
