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
