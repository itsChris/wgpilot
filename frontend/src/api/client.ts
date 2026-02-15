import type { ApiError } from '@/types/api';

const TOKEN_KEY = 'wgpilot_token';

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export class ApiClientError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiClientError';
  }
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (response.status === 401) {
    // Don't redirect during setup wizard — let the component handle 401.
    if (!window.location.pathname.startsWith('/setup')) {
      clearToken();
      window.location.href = '/login';
    }
    throw new ApiClientError(401, 'UNAUTHORIZED', 'Session expired');
  }

  if (response.status === 403) {
    // Redirect to setup if the server says setup is required.
    const body = await response.json();
    const apiError = body as ApiError;
    if (apiError.error?.code === 'SETUP_REQUIRED') {
      window.location.href = '/setup';
      throw new ApiClientError(403, 'SETUP_REQUIRED', 'Setup not complete');
    }
    throw new ApiClientError(
      response.status,
      apiError.error?.code ?? 'FORBIDDEN',
      apiError.error?.message ?? 'Forbidden',
    );
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const body = await response.json();

  if (!response.ok) {
    const apiError = body as ApiError;
    throw new ApiClientError(
      response.status,
      apiError.error?.code ?? 'UNKNOWN',
      apiError.error?.message ?? 'An unexpected error occurred',
    );
  }

  return body as T;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  if (token) {
    return { Authorization: `Bearer ${token}` };
  }
  return {};
}

export async function apiGet<T>(path: string): Promise<T> {
  const response = await fetch(`/api${path}`, {
    headers: { ...authHeaders() },
  });
  return handleResponse<T>(response);
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  const response = await fetch(`/api${path}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...authHeaders(),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  return handleResponse<T>(response);
}

export async function apiPut<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`/api${path}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      ...authHeaders(),
    },
    body: JSON.stringify(body),
  });
  return handleResponse<T>(response);
}

export async function apiDelete<T = void>(path: string): Promise<T> {
  const response = await fetch(`/api${path}`, {
    method: 'DELETE',
    headers: { ...authHeaders() },
  });
  return handleResponse<T>(response);
}
