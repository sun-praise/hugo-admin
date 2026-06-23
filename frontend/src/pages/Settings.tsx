import { useState, useEffect } from 'react';
import { get, put, initProject, getThemes, installTheme, activateTheme, previewTheme } from '../utils/api';
import type { Settings as SettingsType } from '../types';

export default function SettingsPage() {
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');
  const [apiKeyInput, setApiKeyInput] = useState('');
  const [apiKeySource, setApiKeySource] = useState('none');
  const [apiKeyHint, setApiKeyHint] = useState('');
  const [listmonkApiKeyInput, setListmonkApiKeyInput] = useState('');
  const [form, setForm] = useState<SettingsType>({
    hugo: { base_dir: '', server_url: '' },
    ai: { base_url: '', model: '', api_key_source: 'none', api_key_hint: '' },
    listmonk: { api_url: '', api_user: '', api_key: '', blog_list_id: 1 },
    theme: { name: '' },
  });

  // Project init state
  const [initPath, setInitPath] = useState('');
  const [initFormat, setInitFormat] = useState<'toml' | 'yaml'>('toml');
  const [initLoading, setInitLoading] = useState(false);

  // Themes state
  const [themes, setThemes] = useState<{ name: string; is_submodule: boolean }[]>([]);
  const [activeTheme, setActiveTheme] = useState<string | null>(null);
  const [previewThemeName, setPreviewThemeName] = useState<string | null>(null);
  const [themesLoading, setThemesLoading] = useState(false);
  const [installUrl, setInstallUrl] = useState('');
  const [installName, setInstallName] = useState('');
  const [installMode, setInstallMode] = useState<'submodule' | 'copy'>('submodule');
  const [installLoading, setInstallLoading] = useState(false);

  useEffect(() => {
    fetchSettings();
    fetchThemes();
  }, []);

  async function fetchSettings() {
    setLoading(true);
    setErrorMessage('');
    try {
      const data = await get<{ success: boolean; settings?: SettingsType; message?: string }>('/api/settings');
      if (!data.success) {
        throw new Error(data.message || '加载设置失败');
      }
      const settings = data.settings || { hugo: { base_dir: '', server_url: '' }, ai: { base_url: '', model: '' }, listmonk: { api_url: '', api_user: '', api_key: '', blog_list_id: 1 }, theme: { name: '' } };
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
        listmonk: {
          api_url: settings.listmonk?.api_url || '',
          api_user: settings.listmonk?.api_user || '',
          api_key: settings.listmonk?.api_key || '',
          blog_list_id: settings.listmonk?.blog_list_id || 1,
        },
        theme: {
          name: settings.theme?.name || '',
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

  async function fetchThemes() {
    setThemesLoading(true);
    try {
      const data = await getThemes();
      if (!data.success) {
        throw new Error(data.message || '加载主题失败');
      }
      setThemes(data.themes || []);
      setActiveTheme(data.active_theme || null);
    } catch (error) {
      setErrorMessage((error as Error).message);
    } finally {
      setThemesLoading(false);
    }
  }

  async function handleInitProject(e: React.FormEvent) {
    e.preventDefault();
    if (initLoading || !initPath.trim()) return;
    setInitLoading(true);
    setErrorMessage('');
    try {
      const data = await initProject({ path: initPath.trim(), config_format: initFormat });
      if (!data.success) {
        throw new Error(data.message || '初始化失败');
      }
      showNotification(`站点已创建: ${data.path}`, 'success');
      setInitPath('');
      await fetchSettings();
    } catch (error) {
      setErrorMessage((error as Error).message);
    } finally {
      setInitLoading(false);
    }
  }

  async function handleInstallTheme(e: React.FormEvent) {
    e.preventDefault();
    if (installLoading || !installUrl.trim() || !installName.trim()) return;
    setInstallLoading(true);
    setErrorMessage('');
    try {
      const data = await installTheme({
        repo_url: installUrl.trim(),
        name: installName.trim(),
        mode: installMode,
      });
      if (!data.success) {
        throw new Error(data.message || '安装失败');
      }
      showNotification(`主题 ${data.theme?.name} 安装成功`, 'success');
      setInstallUrl('');
      setInstallName('');
      await fetchThemes();
    } catch (error) {
      setErrorMessage((error as Error).message);
    } finally {
      setInstallLoading(false);
    }
  }

  async function handleActivateTheme(name: string) {
    setErrorMessage('');
    try {
      const data = await activateTheme(name);
      if (!data.success) {
        throw new Error(data.message || '激活失败');
      }
      setActiveTheme(name);
      setForm((prev) => ({ ...prev, theme: { name } }));
      showNotification(`已激活主题: ${name}`, 'success');
    } catch (error) {
      setErrorMessage((error as Error).message);
    }
  }

  async function handlePreviewTheme(name: string) {
    setErrorMessage('');
    try {
      const data = await previewTheme(name);
      if (!data.success) {
        throw new Error(data.message || '预览失败');
      }
      setPreviewThemeName(name);
      showNotification(`正在预览主题: ${name}`, 'success');
    } catch (error) {
      setErrorMessage((error as Error).message);
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
        listmonk: {
          api_url: form.listmonk.api_url,
          api_user: form.listmonk.api_user,
          blog_list_id: form.listmonk.blog_list_id,
        },
        theme: {
          name: form.theme.name,
        },
      };
      if (apiKeyInput.trim() !== '') {
        payload.ai.api_key = apiKeyInput.trim();
      }
      if (listmonkApiKeyInput.trim() !== '') {
        payload.listmonk.api_key = listmonkApiKeyInput.trim();
      }
      const data = await put<{ success: boolean; message?: string }>('/api/settings', payload);
      if (!data.success) {
        throw new Error(data.message || '保存设置失败');
      }
      showNotification('设置保存成功', 'success');
      setApiKeyInput('');
      setListmonkApiKeyInput('');
      await fetchSettings();
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
          <p className="mt-4 text-stone-600">加载中...</p>
        </div>
      )}

      {!loading && (
        <>
          <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-6">
            <h3 className="text-lg font-medium mb-4">Hugo Blog</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-2">博客根目录</label>
                <input
                  type="text"
                  value={form.hugo.base_dir}
                  onChange={(e) => setForm({ ...form, hugo: { ...form.hugo, base_dir: e.target.value } })}
                  placeholder="/path/to/hugo-blog"
                  className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 font-mono text-sm"
                />
                <p className="mt-2 text-xs text-stone-500">Hugo 项目的根目录，需包含 config.toml / config.yaml 等配置文件。</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-2">Hugo 服务器 URL</label>
                <input
                  type="text"
                  value={form.hugo.server_url}
                  onChange={(e) => setForm({ ...form, hugo: { ...form.hugo, server_url: e.target.value } })}
                  placeholder="http://0.0.0.0:1313"
                  className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 font-mono text-sm"
                />
                <p className="mt-2 text-xs text-stone-500">Hugo 预览服务器的基础 URL。留空则使用默认值。</p>
              </div>
            </div>
          </div>

          <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-6">
            <h3 className="text-lg font-medium mb-4">AI 助手</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-2">AI Base URL</label>
                <input
                  type="text"
                  value={form.ai.base_url}
                  onChange={(e) => setForm({ ...form, ai: { ...form.ai, base_url: e.target.value } })}
                  placeholder="https://api.deepseek.com"
                  className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 font-mono text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-2">AI Model</label>
                <input
                  type="text"
                  value={form.ai.model}
                  onChange={(e) => setForm({ ...form, ai: { ...form.ai, model: e.target.value } })}
                  placeholder="deepseek-chat"
                  className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 font-mono text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-2">AI API Key（可选）</label>
                <input
                  type="password"
                  value={apiKeyInput}
                  placeholder="留空则保持当前来源"
                  onChange={(e) => setApiKeyInput(e.target.value)}
                  className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 font-mono text-sm"
                />
                {apiKeySource === 'session' && (
                  <p className="mt-2 text-xs text-stone-500">
                    当前使用会话密钥（重启后失效）
                    {apiKeyHint && `（${apiKeyHint}）`}
                  </p>
                )}
                {apiKeySource === 'env' && <p className="mt-2 text-xs text-stone-500">当前使用环境变量中的密钥</p>}
                {apiKeySource === 'none' && <p className="mt-2 text-xs text-stone-500">当前未配置 AI API Key</p>}
              </div>
              <div className="bg-blue-50 border border-blue-200 rounded-lg p-3 text-sm text-blue-800">
                未保存密钥时会自动回退到环境变量（<code>DEEPSEEK_API_KEY</code> 或 <code>AI_API_KEY</code>）。
              </div>
            </div>
          </div>

          <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-6">
            <h3 className="text-lg font-medium mb-4">邮件推送 (Listmonk)</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-2">Listmonk API URL</label>
                <input
                  type="text"
                  value={form.listmonk.api_url}
                  onChange={(e) => setForm({ ...form, listmonk: { ...form.listmonk, api_url: e.target.value } })}
                  placeholder="http://localhost:9000/api"
                  className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 font-mono text-sm"
                />
                <p className="mt-2 text-xs text-stone-500">Listmonk 服务的 API 地址。</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-2">API User</label>
                <input
                  type="text"
                  value={form.listmonk.api_user}
                  onChange={(e) => setForm({ ...form, listmonk: { ...form.listmonk, api_user: e.target.value } })}
                  placeholder="admin"
                  className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 font-mono text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-2">API Key</label>
                <input
                  type="password"
                  value={listmonkApiKeyInput}
                  placeholder="留空则保持当前值"
                  onChange={(e) => setListmonkApiKeyInput(e.target.value)}
                  className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 font-mono text-sm"
                />
                {form.listmonk.api_key && (
                  <p className="mt-2 text-xs text-stone-500">当前密钥: {form.listmonk.api_key}</p>
                )}
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-2">订阅列表 ID</label>
                <input
                  type="number"
                  value={form.listmonk.blog_list_id}
                  onChange={(e) => { const v = parseInt(e.target.value); setForm({ ...form, listmonk: { ...form.listmonk, blog_list_id: isNaN(v) ? 1 : v } }); }}
                  className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 font-mono text-sm"
                />
                <p className="mt-2 text-xs text-stone-500">博客订阅者的 Listmonk 列表 ID。</p>
              </div>
            </div>
          </div>

          <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-6">
            <h3 className="text-lg font-medium mb-4">初始化项目</h3>
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

          <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-6">
            <h3 className="text-lg font-medium mb-4">主题管理</h3>
            {themesLoading ? (
              <p className="text-stone-500 text-sm">加载主题中...</p>
            ) : (
              <>
                {themes.length === 0 ? (
                  <p className="text-stone-500 text-sm mb-4">当前未安装任何主题。</p>
                ) : (
                  <ul className="divide-y divide-stone-200 mb-4">
                    {themes.map((theme) => (
                      <li key={theme.name} className="py-3 flex items-center justify-between">
                        <div>
                          <span className="font-medium text-stone-800">{theme.name}</span>
                          {theme.is_submodule && (
                            <span className="ml-2 text-xs px-2 py-0.5 bg-stone-100 text-stone-600 rounded">submodule</span>
                          )}
                          {activeTheme === theme.name && (
                            <span className="ml-2 text-xs px-2 py-0.5 bg-green-100 text-green-700 rounded">已启用</span>
                          )}
                          {previewThemeName === theme.name && (
                            <span className="ml-2 text-xs px-2 py-0.5 bg-blue-100 text-blue-700 rounded">预览中</span>
                          )}
                        </div>
                        <div className="flex gap-2">
                          <button
                            onClick={() => handlePreviewTheme(theme.name)}
                            className="px-3 py-1.5 text-sm bg-blue-50 text-blue-700 rounded hover:bg-blue-100 transition-colors"
                          >
                            预览
                          </button>
                          <button
                            onClick={() => handleActivateTheme(theme.name)}
                            disabled={activeTheme === theme.name}
                            className="px-3 py-1.5 text-sm bg-stone-100 text-stone-700 rounded hover:bg-stone-200 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                          >
                            启用
                          </button>
                        </div>
                      </li>
                    ))}
                  </ul>
                )}

                {previewThemeName && (
                  <div className="mb-4 p-3 bg-blue-50 border border-blue-200 rounded-lg text-sm text-blue-800">
                    当前预览主题: <strong>{previewThemeName}</strong>。停止并重新启动服务器后将恢复为已启用主题。
                  </div>
                )}

                <form onSubmit={handleInstallTheme} className="space-y-4 border-t border-stone-200 pt-4">
                  <h4 className="text-sm font-medium text-stone-700">安装主题</h4>
                  <div>
                    <label className="block text-sm text-stone-600 mb-1">Git 仓库地址</label>
                    <input
                      type="text"
                      value={installUrl}
                      onChange={(e) => setInstallUrl(e.target.value)}
                      placeholder="https://github.com/user/hugo-theme-example.git"
                      className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 font-mono text-sm"
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-stone-600 mb-1">主题名称</label>
                    <input
                      type="text"
                      value={installName}
                      onChange={(e) => setInstallName(e.target.value)}
                      placeholder="hugo-theme-example"
                      className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 text-sm"
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-stone-600 mb-1">安装模式</label>
                    <select
                      value={installMode}
                      onChange={(e) => setInstallMode(e.target.value as 'submodule' | 'copy')}
                      className="w-full px-3 py-2 border border-stone-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-stone-400 text-sm"
                    >
                      <option value="submodule">Git 子模块（推荐）</option>
                      <option value="copy">复制（不包含 .git）</option>
                    </select>
                  </div>
                  <div className="flex justify-end">
                    <button
                      type="submit"
                      disabled={installLoading || !installUrl.trim() || !installName.trim()}
                      className="px-6 py-2 bg-stone-800 text-white rounded-lg hover:bg-stone-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {installLoading ? '安装中...' : '安装主题'}
                    </button>
                  </div>
                </form>
              </>
            )}
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
