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
