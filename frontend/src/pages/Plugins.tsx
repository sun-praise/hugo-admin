import { useState, useEffect, useCallback } from 'react';
import { get, put, post } from '../utils/api';
import {
  Puzzle,
  Store,
  Download,
  Settings2,
  Power,
  PowerOff,
  RefreshCw,
  Check,
  Loader2,
} from 'lucide-react';

// ---------- types ----------

interface PluginInfo {
  name: string;
  version: string;
  description: string;
  author: string;
  capabilities: string[];
  status: string;
  enabled: boolean;
  has_config: boolean;
}

interface MarketPlugin {
  name: string;
  version: string;
  description: string;
  author: string;
  capabilities: string[];
  platform: string;
  arch: string;
}

interface MarketCatalog {
  version: number;
  plugins: MarketPlugin[];
}

// ---------- main component ----------

export default function PluginsPage() {
  const [tab, setTab] = useState<'installed' | 'market'>('installed');
  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [market, setMarket] = useState<MarketPlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    if (tab === 'installed') fetchPlugins();
    else fetchMarket();
  }, [tab]);

  async function fetchPlugins() {
    setLoading(true);
    setError('');
    try {
      const res = await get<{ success: boolean; plugins: PluginInfo[] }>('/api/plugins');
      setPlugins(res.plugins);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function fetchMarket() {
    setLoading(true);
    setError('');
    try {
      const res = await get<MarketCatalog>('/api/plugins/market');
      setMarket(res.plugins || []);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="max-w-4xl space-y-6">
      {/* header */}
      <div className="page-head">
        <h1 className="m-0 text-[22px] font-semibold tracking-[-0.02em]">插件</h1>
        <p className="mb-0 mt-1 text-[13.5px] text-[#52525b]">管理和浏览 hugo-admin 插件</p>
      </div>

      {/* tabs */}
      <div className="flex gap-1 border-b border-[#e4e4e7]">
        <TabBtn active={tab === 'installed'} onClick={() => setTab('installed')}>
          已安装
        </TabBtn>
        <TabBtn active={tab === 'market'} onClick={() => setTab('market')}>
          <Store className="w-4 h-4" />
          市场
        </TabBtn>
      </div>

      {/* error */}
      {error && (
        <div className="bg-red-50 text-red-700 px-4 py-3 rounded-md text-sm">{error}</div>
      )}

      {/* content */}
      {loading ? (
        <div className="flex items-center justify-center py-20 text-[#9ca3a8]">
          <Loader2 className="w-6 h-6 animate-spin" />
          <span className="ml-2">加载中...</span>
        </div>
      ) : tab === 'installed' ? (
        <InstalledTab plugins={plugins} onRefresh={fetchPlugins} />
      ) : (
        <MarketTab plugins={market} installed={plugins} />
      )}
    </div>
  );
}

// ---------- tab button ----------

function TabBtn({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${
        active
          ? 'border-zinc-900 text-zinc-900'
          : 'border-transparent text-[#52525b] hover:text-zinc-900'
      }`}
    >
      {children}
    </button>
  );
}

// ---------- installed tab ----------

function InstalledTab({
  plugins,
  onRefresh,
}: {
  plugins: PluginInfo[];
  onRefresh: () => void;
}) {
  if (plugins.length === 0) {
    return (
      <div className="text-center py-20 text-[#9ca3a8]">
        <Puzzle className="w-12 h-12 mx-auto mb-3 opacity-30" />
        <p>暂无已安装的插件</p>
        <p className="text-sm mt-1">从市场安装插件以扩展功能</p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <button
          onClick={onRefresh}
          className="flex items-center gap-1.5 text-sm text-[#52525b] hover:text-zinc-900 transition-colors"
        >
          <RefreshCw className="w-3.5 h-3.5" />
          刷新
        </button>
      </div>
      {plugins.map((p) => (
        <PluginCard key={p.name} plugin={p} onRefresh={onRefresh} />
      ))}
    </div>
  );
}

// ---------- plugin card ----------

function PluginCard({
  plugin,
  onRefresh,
}: {
  plugin: PluginInfo;
  onRefresh: () => void;
}) {
  const [configuring, setConfiguring] = useState(false);
  const [toggling, setToggling] = useState(false);

  async function toggleEnable() {
    setToggling(true);
    try {
      const endpoint = plugin.enabled ? 'disable' : 'enable';
      await post(`/api/plugins/${plugin.name}/${endpoint}`);
      onRefresh();
    } catch {
      // handled by parent refresh
    } finally {
      setToggling(false);
    }
  }

  return (
    <div className="rounded-[10px] border border-[#e4e4e7] bg-white p-5">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-base font-medium text-zinc-900">{plugin.name}</h3>
            <span className="font-mono text-xs text-[#52525b]">v{plugin.version}</span>
            <StatusBadge status={plugin.status} enabled={plugin.enabled} />
          </div>
          <p className="text-sm text-[#52525b] mt-1">{plugin.description}</p>
          <div className="flex items-center gap-3 mt-2">
            <span className="text-xs text-[#9ca3a8]">by {plugin.author}</span>
            <div className="flex gap-1">
              {plugin.capabilities.map((c) => (
                <span
                  key={c}
                  className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-[#f4f4f5] text-[#52525b]"
                >
                  {c}
                </span>
              ))}
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2 flex-shrink-0">
          {plugin.has_config && (
            <button
              onClick={() => setConfiguring(!configuring)}
              className="inline-flex items-center gap-1 px-3 py-1.5 text-sm border border-[#e4e4e7] text-[#3f3f46] rounded-lg hover:bg-[#f4f4f5] transition-colors"
            >
              <Settings2 className="w-3.5 h-3.5" />
              配置
            </button>
          )}
          <button
            onClick={toggleEnable}
            disabled={toggling}
            className={`inline-flex items-center gap-1 px-3 py-1.5 text-sm rounded-lg transition-colors ${
              plugin.enabled
                ? 'border border-[#e4e4e7] text-[#3f3f46] hover:bg-[#f4f4f5]'
                : 'bg-zinc-900 text-white hover:bg-[#2e2e33]'
            }`}
          >
            {toggling ? (
              <Loader2 className="w-3.5 h-3.5 animate-spin" />
            ) : plugin.enabled ? (
              <PowerOff className="w-3.5 h-3.5" />
            ) : (
              <Power className="w-3.5 h-3.5" />
            )}
            {plugin.enabled ? '禁用' : '启用'}
          </button>
        </div>
      </div>

      {/* config panel */}
      {configuring && <ConfigPanel name={plugin.name} />}
    </div>
  );
}

// ---------- status badge ----------

function StatusBadge({ status, enabled }: { status: string; enabled: boolean }) {
  const colors: Record<string, string> = {
    running: 'bg-emerald-50 text-emerald-700',
    stopped: 'bg-[#f4f4f5] text-[#52525b]',
    error: 'bg-red-50 text-red-700',
  };
  const labels: Record<string, string> = {
    running: '运行中',
    stopped: '已停止',
    error: '错误',
  };
  const dotColors: Record<string, string> = {
    running: 'dot-pulse',
    stopped: 'dot-pulse is-off',
    error: 'w-1.5 h-1.5 rounded-full bg-red-500',
  };

  const s = enabled ? status : 'stopped';

  return (
    <span
      className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium ${colors[s] || colors.stopped}`}
    >
      <span aria-hidden="true" className={dotColors[s] || dotColors.stopped} />
      {labels[s] || s}
    </span>
  );
}

// ---------- config panel ----------

function ConfigPanel({ name }: { name: string }) {
  const [schema, setSchema] = useState<Record<string, unknown> | null>(null);
  const [config, setConfig] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState('');

  const loadConfig = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const [schemaRes, configRes] = await Promise.all([
        get<{ success: boolean; schema: Record<string, unknown> }>(
          `/api/plugins/${name}/config-schema`
        ),
        get<{ success: boolean; config: Record<string, string> }>(
          `/api/plugins/${name}/config`
        ),
      ]);
      setSchema(schemaRes.schema);
      setConfig(configRes.config || {});
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [name]);

  useEffect(() => {
    (async () => {
      await loadConfig();
    })();
  }, [loadConfig]);

  async function saveConfig() {
    setSaving(true);
    setSaved(false);
    setError('');
    try {
      await put(`/api/plugins/${name}/config`, config);
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <div className="mt-4 pt-4 border-t border-[#e4e4e7] flex items-center gap-2 text-sm text-[#9ca3a8]">
        <Loader2 className="w-4 h-4 animate-spin" />
        加载配置...
      </div>
    );
  }

  const properties =
    (schema?.properties as Record<string, { type: string; label?: string }>) || {};

  return (
    <div className="mt-4 pt-4 border-t border-[#e4e4e7] space-y-3">
      <h4 className="text-sm font-medium text-[#3f3f46]">插件配置</h4>

      {error && (
        <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{error}</div>
      )}

      <div className="space-y-3">
        {Object.entries(properties).map(([key, def]) => (
          <div key={key}>
            <label className="block text-sm text-[#52525b] mb-1">
              {def.label || key}
            </label>
            <input
              type={key.toLowerCase().includes('token') || key.toLowerCase().includes('key') ? 'password' : 'text'}
              value={config[key] || ''}
              onChange={(e) => setConfig({ ...config, [key]: e.target.value })}
              placeholder={def.label || key}
              className="w-full px-3 py-2 border border-[#e4e4e7] rounded-lg text-sm focus:ring-2 focus:ring-[#d4d4d8] focus:border-[#d4d4d8] outline-none"
            />
          </div>
        ))}
      </div>

      <div className="flex items-center gap-2">
        <button
          onClick={saveConfig}
          disabled={saving}
          className="inline-flex items-center gap-1.5 px-4 py-2 text-sm bg-zinc-900 text-white rounded-lg hover:bg-[#2e2e33] transition-colors"
        >
          {saving ? (
            <Loader2 className="w-3.5 h-3.5 animate-spin" />
          ) : saved ? (
            <Check className="w-3.5 h-3.5" />
          ) : null}
          {saving ? '保存中...' : saved ? '已保存' : '保存配置'}
        </button>
      </div>
    </div>
  );
}

// ---------- market tab ----------

function MarketTab({
  plugins,
  installed,
}: {
  plugins: MarketPlugin[];
  installed: PluginInfo[];
}) {
  const installedNames = new Set(installed.map((p) => p.name));

  if (plugins.length === 0) {
    return (
      <div className="text-center py-20 text-[#9ca3a8]">
        <Store className="w-12 h-12 mx-auto mb-3 opacity-30" />
        <p>插件市场暂无可用插件</p>
        <p className="text-sm mt-1">敬请期待</p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {plugins.map((p) => (
        <div
          key={p.name}
          className="rounded-[10px] border border-[#e4e4e7] bg-white p-5"
        >
          <div className="flex items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-2">
                <h3 className="text-base font-medium text-zinc-900">{p.name}</h3>
                <span className="font-mono text-xs text-[#52525b]">v{p.version}</span>
              </div>
              <p className="text-sm text-[#52525b] mt-1">{p.description}</p>
              <div className="flex items-center gap-3 mt-2">
                <span className="text-xs text-[#9ca3a8]">by {p.author}</span>
                <div className="flex gap-1">
                  {p.capabilities.map((c) => (
                    <span
                      key={c}
                      className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-[#f4f4f5] text-[#52525b]"
                    >
                      {c}
                    </span>
                  ))}
                </div>
              </div>
            </div>

            {installedNames.has(p.name) ? (
              <span className="inline-flex items-center gap-1 px-3 py-1.5 text-sm text-emerald-700 bg-emerald-50 rounded-md">
                <Check className="w-3.5 h-3.5" />
                已安装
              </span>
            ) : (
              <span className="inline-flex items-center gap-1 px-3 py-1.5 text-sm border border-[#e4e4e7] text-[#52525b] rounded-lg cursor-not-allowed opacity-50">
                <Download className="w-3.5 h-3.5" />
                安装（需手动）
              </span>
            )}
          </div>
        </div>
      ))}

      <p className="text-xs text-[#9ca3a8] text-center pt-4">
        v1 版本需手动下载并解压到 ~/.hugo-admin/plugins/ 目录
      </p>
    </div>
  );
}
