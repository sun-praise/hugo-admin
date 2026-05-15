const API_BASE = '';

export async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${url}`, {
    headers: {
      'Content-Type': 'application/json',
    },
    ...options,
  });
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
