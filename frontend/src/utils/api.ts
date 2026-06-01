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
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
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
