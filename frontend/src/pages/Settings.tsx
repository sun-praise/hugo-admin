import { useState, useEffect } from 'react';
import ConfigEditor from '../components/ConfigEditor';
import {
  get,
  put,
  initProject,
  getThemes,
  getAvailableThemes,
  installTheme,
  activateTheme,
  previewTheme,
  getActiveProject,
  resetActiveProject,
  cleanPlaceholderLayouts,
  listConfigs,
  getConfigFile,
  saveConfigFile,
} from '../utils/api';
import type { AvailableTheme, ConfigFileInfo } from '../utils/api';
import type { Settings as SettingsType, Theme } from '../types';
type TabKey = 'general' | 'project' | 'themes' | 'config';

export default function SettingsPage() {
  const [activeTab, setActiveTab] = useState<TabKey>('general');
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
  const [activeProjectPath, setActiveProjectPath] = useState('');
  const [initPath, setInitPath] = useState('');
  const [initFormat, setInitFormat] = useState<'toml' | 'yaml'>('toml');
  const [initLoading, setInitLoading] = useState(false);
  const [initResult, setInitResult] = useState<{
    path: string;
    config_format: string;
    default_theme?: {
      name: string;
      installed: boolean;
      activated: boolean;
      error: string | null;
    };
  } | null>(null);

  // Themes state
  const [themes, setThemes] = useState<Theme[]>([]);
  const [activeTheme, setActiveTheme] = useState<string | null>(null);
  const [previewThemeName, setPreviewThemeName] = useState<string | null>(null);
  const [themesLoading, setThemesLoading] = useState(false);
  const [availableThemes, setAvailableThemes] = useState<AvailableTheme[]>([]);
  const [installUrl, setInstallUrl] = useState('');
  const [installName, setInstallName] = useState('');
  const [installMode, setInstallMode] = useState<'submodule' | 'copy'>('submodule');
  const [installLoading, setInstallLoading] = useState(false);

  // Config editor state
  const [configFiles, setConfigFiles] = useState<ConfigFileInfo[]>([]);
  const [activeConfigFile, setActiveConfigFile] = useState<ConfigFileInfo | null>(null);
  const [configContent, setConfigContent] = useState('');
  const [configLoading, setConfigLoading] = useState(false);
  const [configSaving, setConfigSaving] = useState(false);

  useEffect(() => {
    fetchSettings();
    fetchThemes();
    fetchActiveProject();
  }, []);

  useEffect(() => {
    if (activeTab === 'config' && !configContent && !configLoading) {
      fetchConfig();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab]);

  async function fetchActiveProject() {
    try {
      const data = await getActiveProject();
      if (data.success) {
        setActiveProjectPath(data.path || '');
      }
    } catch {
      // 静默失败：active project 端点非关键
    }
  }

  async function handleResetActiveProject() {
    if (
      !confirm(
        '确认清除持久化的活跃项目？清除后下次启动会回退到默认 HUGO_ROOT。',
      )
    ) {
      return;
    }
    try {
      const res = await resetActiveProject();
      if (!res.success) {
        throw new Error(res.message || '清除失败');
      }
      showNotification('已清除持久化的活跃项目', 'success');
      await fetchActiveProject();
    } catch (error) {
      showNotification((error as Error).message, 'error');
    }
  }

  async function handleCleanLayouts() {
    if (
      !confirm(
        '确认清理活跃项目下的占位 layouts/？如果之前是旧版本 init 创建的"毛坯"站点，这一步会让已安装的主题接管渲染。',
      )
    ) {
      return;
    }
    try {
      const res = await cleanPlaceholderLayouts();
      if (!res.success) {
        throw new Error(res.message || '清理失败');
      }
      showNotification(res.message || '已清理占位 layouts', 'success');
    } catch (error) {
      showNotification((error as Error).message, 'error');
    }
  }

  async function fetchConfig() {
    setConfigLoading(true);
    try {
      const data = await listConfigs();
      if (data.success && data.files && data.files.length > 0) {
        setConfigFiles(data.files);
        if (!activeConfigFile) {
          await loadConfigFile(data.files[0]);
        }
      }
    } catch (error) {
      if ((error as Error).message !== '未找到 Hugo 配置文件') {
        showNotification((error as Error).message, 'error');
      }
    } finally {
      setConfigLoading(false);
    }
  }

  async function loadConfigFile(file: ConfigFileInfo) {
    setActiveConfigFile(file);
    setConfigLoading(true);
    try {
      const data = await getConfigFile(file.name);
      if (data.success && data.content !== undefined) {
        setConfigContent(data.content);
      }
    } catch (error) {
      showNotification((error as Error).message, 'error');
    } finally {
      setConfigLoading(false);
    }
  }

  async function handleSaveConfig() {
    if (!activeConfigFile) return;
    setConfigSaving(true);
    try {
      const data = await saveConfigFile(activeConfigFile.name, configContent);
      if (!data.success) {
        throw new Error(data.message || '保存失败');
      }
      showNotification(data.message || '配置已保存', 'success');
    } catch (error) {
      showNotification((error as Error).message, 'error');
    } finally {
      setConfigSaving(false);
    }
  }

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
      const [installed, available] = await Promise.all([getThemes(), getAvailableThemes()]);
      if (!installed.success) {
        throw new Error(installed.message || '加载主题失败');
      }
      setThemes(installed.themes || []);
      setActiveTheme(installed.active_theme || null);
      if (available.success) {
        setAvailableThemes(available.available_themes || []);
      }
    } catch (error) {
      setErrorMessage((error as Error).message);
    } finally {
      setThemesLoading(false);
    }
  }

  async function handleInstallDefaultTheme(theme: AvailableTheme) {
    if (installLoading) return;
    setInstallLoading(true);
    setErrorMessage('');
    try {
      const result = await installTheme({
        repo_url: theme.repo,
        name: theme.name,
        mode: 'copy',
      });
      if (!result.success) {
        throw new Error(result.message || '安装失败');
      }
      showNotification(`已安装默认主题: ${theme.name}`, 'success');
      await fetchThemes();
    } catch (error) {
      setErrorMessage((error as Error).message);
      showNotification((error as Error).message, 'error');
    } finally {
      setInstallLoading(false);
    }
  }

  async function handleInitProject(e: React.FormEvent) {
    e.preventDefault();
    if (initLoading || !initPath.trim()) return;
    setInitLoading(true);
    setErrorMessage('');
    setInitResult(null);
    try {
      const data = await initProject({ path: initPath.trim(), config_format: initFormat });
      if (!data.success) {
        throw new Error(data.message || '初始化失败');
      }
      showNotification(`站点已创建: ${data.path}`, 'success');
      setInitResult({
        path: data.path || '',
        config_format: data.config_format || '',
        default_theme: data.default_theme,
      });
      setInitPath('');
      setPreviewThemeName(null);
      await fetchSettings();
      await fetchThemes();
      await fetchActiveProject();
    } catch (error) {
      setErrorMessage((error as Error).message);
      showNotification((error as Error).message, 'error');
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

  const tabs: { key: TabKey; label: string }[] = [
    { key: 'general', label: '常规设置' },
    { key: 'themes', label: '主题管理' },
    { key: 'project', label: '初始化项目' },
    { key: 'config', label: '站点配置' },
  ];

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
          <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-1">
            <div className="flex space-x-1">
              {tabs.map((tab) => (
                <button
                  key={tab.key}
                  onClick={() => setActiveTab(tab.key)}
                  className={`flex-1 px-4 py-2 text-sm font-medium rounded-md transition-colors ${
                    activeTab === tab.key
                      ? 'bg-stone-100 text-stone-900'
                      : 'text-stone-500 hover:text-stone-700 hover:bg-stone-50'
                  }`}
                >
                  {tab.label}
                </button>
              ))}
            </div>
          </div>

          {activeTab === 'general' && (
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

          {activeTab === 'project' && (
            <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-6">
              <h3 className="text-lg font-medium mb-2">初始化 Hugo 项目</h3>
              <p className="text-sm text-stone-500 mb-6">
                在指定路径创建全新的 Hugo 站点，并将其设为当前活跃项目。该操作会写入文件系统，请谨慎使用。
              </p>

              {activeProjectPath && (
                <div className="mb-6 p-3 bg-stone-50 border border-stone-200 rounded-lg text-sm">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <p className="text-stone-600">当前活跃项目：</p>
                      <p className="font-mono text-stone-800 break-all">
                        {activeProjectPath}
                      </p>
                      <p className="mt-1 text-xs text-stone-500">
                        新建站点会自动覆盖此路径，且会被持久化到
                        <code className="mx-1">data/active_project.txt</code>
                        ，重启后仍生效。
                      </p>
                    </div>
                    <div className="flex flex-col gap-2 shrink-0">
                      <button
                        onClick={handleResetActiveProject}
                        className="px-3 py-1.5 text-xs bg-stone-100 text-stone-700 rounded hover:bg-stone-200 transition-colors"
                      >
                        清除持久化
                      </button>
                      <button
                        onClick={handleCleanLayouts}
                        className="px-3 py-1.5 text-xs bg-amber-50 text-amber-800 border border-amber-200 rounded hover:bg-amber-100 transition-colors"
                        title="删除站点根 layouts/，让 themes/ 接管（修复旧 init 留下的毛坯）"
                      >
                        清理占位 layouts
                      </button>
                    </div>
                  </div>
                </div>
              )}

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

                {initResult && (
                  <div className="p-3 bg-green-50 border border-green-200 rounded-lg text-sm text-green-800 space-y-1">
                    <p>站点已创建：<code className="font-mono">{initResult.path}</code></p>
                    <p>配置文件格式：{initResult.config_format}</p>
                    {initResult.default_theme && (
                      <p>
                        默认主题 <span className="font-mono">{initResult.default_theme.name}</span>：
                        {initResult.default_theme.activated
                          ? '已安装并激活'
                          : initResult.default_theme.installed
                          ? '已安装但未激活'
                          : initResult.default_theme.error
                          ? `处理失败（${initResult.default_theme.error}），可稍后在主题页手动安装`
                          : '未安装（无网络/离线环境），可稍后在主题页手动安装'}
                      </p>
                    )}
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
          )}

          {activeTab === 'themes' && (
            <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-6">
              <h3 className="text-lg font-medium mb-4">主题管理</h3>

              {availableThemes.length > 0 && (
                <section className="mb-6">
                  <h4 className="text-sm font-medium text-stone-700 mb-2">默认主题</h4>
                  <p className="text-xs text-stone-500 mb-3">
                    hugo-admin 维护的推荐主题，点击一键安装到 <code>themes/</code>。
                  </p>
                  <ul className="divide-y divide-stone-200 border border-stone-200 rounded-lg">
                    {availableThemes.map((t) => {
                      const installed = themes.some((x) => x.name === t.name);
                      const isActive = activeTheme === t.name;
                      return (
                        <li key={t.name} className="p-3 flex items-center justify-between gap-3">
                          <div className="min-w-0 flex-1">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="font-medium text-stone-800">{t.name}</span>
                              {installed && (
                                <span className="text-xs px-2 py-0.5 bg-green-50 text-green-700 rounded">
                                  已安装
                                </span>
                              )}
                              {isActive && (
                                <span className="text-xs px-2 py-0.5 bg-blue-50 text-blue-700 rounded">
                                  已启用
                                </span>
                              )}
                            </div>
                            <p className="text-xs text-stone-500 mt-1 break-all">
                              {t.description}
                            </p>
                            <p className="text-xs text-stone-400 mt-0.5 font-mono break-all">
                              {t.repo}
                            </p>
                          </div>
                          <div className="flex gap-2 shrink-0">
                            {installed ? (
                              <button
                                onClick={() => handleActivateTheme(t.name)}
                                disabled={isActive || installLoading}
                                className="px-3 py-1.5 text-sm bg-stone-100 text-stone-700 rounded hover:bg-stone-200 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                              >
                                {isActive ? '已启用' : '启用'}
                              </button>
                            ) : (
                              <button
                                onClick={() => handleInstallDefaultTheme(t)}
                                disabled={installLoading}
                                className="px-3 py-1.5 text-sm bg-stone-800 text-white rounded hover:bg-stone-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                              >
                                {installLoading ? '安装中...' : '一键安装'}
                              </button>
                            )}
                          </div>
                        </li>
                      );
                    })}
                  </ul>
                </section>
              )}

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
          )}

          {activeTab === 'config' && (
            <div className="bg-white rounded-md ring-1 ring-stone-900/5 overflow-hidden">
              <div className="flex" style={{ minHeight: 480 }}>
                {/* 左侧文件树 */}
                <div className="w-48 border-r border-stone-200 bg-stone-50 p-3 flex flex-col">
                  <p className="text-xs font-medium text-stone-500 uppercase tracking-wider mb-2">
                    配置文件
                  </p>
                  {configLoading && configFiles.length === 0 ? (
                    <p className="text-xs text-stone-400">加载中...</p>
                  ) : configFiles.length === 0 ? (
                    <p className="text-xs text-stone-400">未找到配置文件</p>
                  ) : (
                    <ul className="space-y-0.5">
                      {configFiles.map((f) => (
                        <li key={f.name}>
                          <button
                            onClick={() => loadConfigFile(f)}
                            className={`w-full text-left px-2 py-1.5 rounded text-sm font-mono transition-colors ${
                              activeConfigFile?.name === f.name
                                ? 'bg-stone-200 text-stone-900'
                                : 'text-stone-600 hover:bg-stone-100 hover:text-stone-800'
                            }`}
                          >
                            {f.name}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>

                {/* 右侧编辑器 */}
                <div className="flex-1 flex flex-col min-w-0">
                  {activeConfigFile ? (
                    <>
                      {/* 顶栏 */}
                      <div className="flex items-center justify-between px-4 py-2 border-b border-stone-200 bg-stone-50">
                        <div className="flex items-center gap-2 min-w-0">
                          <span className="text-sm font-medium text-stone-700 truncate">
                            {activeConfigFile.name}
                          </span>
                          <span className="text-xs text-stone-400 uppercase shrink-0">
                            {activeConfigFile.format}
                          </span>
                        </div>
                        <button
                          onClick={handleSaveConfig}
                          disabled={configSaving || !configContent.trim()}
                          className="px-4 py-1.5 text-sm bg-stone-800 text-white rounded-md hover:bg-stone-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed shrink-0"
                        >
                          {configSaving ? '保存中...' : '保存'}
                        </button>
                      </div>

                      {/* 编辑区：textarea + 高亮 overlay */}
                      <div className="relative flex-1 overflow-auto">
                        {configLoading ? (
                          <div className="p-4 text-stone-400 text-sm">加载中...</div>
                        ) : (
                          <ConfigEditor
                            value={configContent}
                            onChange={setConfigContent}
                            format={activeConfigFile.format}
                          />
                        )}
                      </div>
                    </>
                  ) : (
                    <div className="flex-1 flex items-center justify-center text-stone-400 text-sm">
                      选择左侧文件开始编辑
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
