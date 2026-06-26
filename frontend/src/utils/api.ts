import type { Theme } from '../types';

const API_BASE = '';

export async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {};
  // Only set Content-Type for non-FormData bodies
  if (!options?.body || !(options.body instanceof FormData)) {
    headers['Content-Type'] = 'application/json';
  }

  const response = await fetch(`${API_BASE}${url}`, {
    ...options,
    headers: { ...headers, ...(options?.headers as Record<string, string>) },
  });

  // 会话过期：除登录接口外，统一跳转登录页（硬跳转以清空前端状态）
  if (response.status === 401 && url !== '/api/auth/login') {
    window.location.href = '/login';
    throw new Error('未登录或会话已过期');
  }

  if (!response.ok) {
    try {
      const error = await response.json();
      throw new Error(error.message || `HTTP ${response.status}`);
    } catch (e) {
      if (e instanceof Error && e.message !== `HTTP ${response.status}`) throw e;
      throw new Error(`HTTP ${response.status}: ${response.statusText}`, { cause: e });
    }
  }

  return response.json() as Promise<T>;
}

export async function get<T>(url: string): Promise<T> {
  return request<T>(url, { method: 'GET' });
}

export async function post<T>(url: string, body?: unknown): Promise<T> {
  return request<T>(url, {
    method: 'POST',
    body: body ? JSON.stringify(body) : undefined,
  });
}

export async function put<T>(url: string, body?: unknown): Promise<T> {
  return request<T>(url, {
    method: 'PUT',
    body: body ? JSON.stringify(body) : undefined,
  });
}

export async function del<T>(url: string): Promise<T> {
  return request<T>(url, { method: 'DELETE' });
}

export interface ImportResult {
  success: boolean;
  path?: string;
  title?: string;
  warnings?: string[];
  cover_pending?: boolean;
  event_scope?: string;
  message?: string;
}

/**
 * 上传一个 Markdown 文件，后端通过 AI 自动补全 frontmatter 与封面后导入为草稿。
 */
export async function uploadMarkdown(
  file: File,
  opts?: { title?: string; generate_frontmatter?: boolean; generate_cover?: boolean },
): Promise<ImportResult> {
  const form = new FormData();
  form.append('file', file);
  if (opts?.title) form.append('title', opts.title);
  if (opts?.generate_frontmatter !== undefined) {
    form.append('generate_frontmatter', String(opts.generate_frontmatter));
  }
  if (opts?.generate_cover !== undefined) {
    form.append('generate_cover', String(opts.generate_cover));
  }
  return request<ImportResult>('/api/article/import', { method: 'POST', body: form });
}

// ============ Git 历史 ============

export interface CommitStats {
  files: number;
  insertions: number;
  deletions: number;
}

export interface Commit {
  hash: string;
  author: string;
  email: string;
  date: string;
  refs: string;
  message: string;
  stats: CommitStats;
}

export interface CommitsResponse {
  success: boolean;
  commits: Commit[];
  message?: string;
}

export interface PushRecord {
  id: number;
  remote: string;
  branch: string;
  from_sha: string;
  to_sha: string;
  commit_count: number;
  commit_message: string;
  success: boolean;
  message: string;
  pushed_at: number;
  pushed_at_iso: string;
}

export interface PushesResponse {
  success: boolean;
  pushes: PushRecord[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
  message?: string;
}

/** 获取最近的提交记录（含 refs 与 diffstat）。count 会被后端钳制到 [1, 50]。 */
export async function getCommits(count = 20): Promise<CommitsResponse> {
  return get<CommitsResponse>(`/api/git/commits?count=${count}`);
}

/** 获取推送历史（分页，倒序）。 */
export async function getPushes(page = 1, perPage = 20): Promise<PushesResponse> {
  return get<PushesResponse>(
    `/api/git/pushes?page=${page}&per_page=${perPage}`,
  );
}

// ============ Git 工作区状态与独立 push ============

export interface GitStatus {
  success: boolean;
  has_changes: boolean;
  staged: string[];
  unstaged: string[];
  untracked: string[];
  message?: string;
}

export interface PushResponse {
  success: boolean;
  message: string;
  remote: string;
  branch: string;
}

/** 获取仓库工作区状态（staged / unstaged / untracked）。 */
export async function getGitStatus(): Promise<GitStatus> {
  return get<GitStatus>('/api/git/status');
}

/** 独立 push：仅执行 ``git push``，不触发 add / commit。
 *  - ``remote``：默认 ``origin``
 *  - ``branch``：默认当前分支（由后端解析）
 *  - ``setUpstream``：是否 ``-u``，首次推送到新分支时为 true
 */
export async function pushGit(
  remote?: string,
  branch?: string,
  setUpstream = false,
): Promise<PushResponse> {
  const body: Record<string, unknown> = { set_upstream: setUpstream };
  if (remote) body.remote = remote;
  if (branch) body.branch = branch;
  return post<PushResponse>('/api/git/push', body);
}

// ============ 认证 ============

export interface AuthUser {
  username: string;
}

export interface AuthResponse {
  success: boolean;
  user?: AuthUser;
  message?: string;
}

/** 登录（失败时由调用方捕获 message 展示）。 */
export async function login(
  username: string,
  password: string,
): Promise<AuthResponse> {
  return post<AuthResponse>('/api/auth/login', { username, password });
}

/** 登出，清除服务端会话。 */
export async function logout(): Promise<{ success: boolean }> {
  return post<{ success: boolean }>('/api/auth/logout');
}

/** 获取当前登录用户；未登录返回 success:false（401 会被全局拦截器跳转）。 */
export async function getMe(): Promise<AuthResponse> {
  return get<AuthResponse>('/api/auth/me');
}

/** 修改当前用户密码。 */
export async function changePassword(
  currentPassword: string,
  newPassword: string,
): Promise<{ success: boolean; message?: string }> {
  return post<{ success: boolean; message?: string }>('/api/auth/password', {
    current_password: currentPassword,
    new_password: newPassword,
  });
}

// ============ 项目初始化 ============

export interface InitProjectRequest {
  path: string;
  config_format: 'toml' | 'yaml';
}

export interface InitProjectResponse {
  success: boolean;
  path?: string;
  config_format?: 'toml' | 'yaml';
  message?: string;
}

/** 创建新的 Hugo 站点并设为活跃项目。 */
export async function initProject(payload: InitProjectRequest): Promise<InitProjectResponse> {
  return post<InitProjectResponse>('/api/project/init', payload);
}

// ============ 主题管理 ============

export interface ThemeListResponse {
  success: boolean;
  themes: Theme[];
  active_theme: string | null;
  message?: string;
}

export interface ThemeInstallRequest {
  repo_url: string;
  name: string;
  mode: 'submodule' | 'copy';
}

export interface ThemeInstallResponse {
  success: boolean;
  theme?: { name: string; mode: 'submodule' | 'copy' };
  message?: string;
}

export interface ThemeActivateResponse {
  success: boolean;
  theme?: { name: string; active: boolean };
  message?: string;
}

export interface ThemePreviewResponse {
  success: boolean;
  preview_theme?: string;
  server_url?: string;
  message?: string;
}

/** 获取已安装主题列表。 */
export async function getThemes(): Promise<ThemeListResponse> {
  return get<ThemeListResponse>('/api/themes');
}

/** 安装主题。 */
export async function installTheme(payload: ThemeInstallRequest): Promise<ThemeInstallResponse> {
  return post<ThemeInstallResponse>('/api/themes/install', payload);
}

/** 激活主题。 */
export async function activateTheme(name: string): Promise<ThemeActivateResponse> {
  return post<ThemeActivateResponse>('/api/themes/activate', { name });
}

/** 预览主题（不持久化）。 */
export async function previewTheme(name: string): Promise<ThemePreviewResponse> {
  return post<ThemePreviewResponse>('/api/themes/preview', { name });
}
