import { useCallback, useMemo, useSyncExternalStore } from 'react';
import { apiGet, apiPost, clearToken, getToken, setToken } from '@/api/client';
import type { AuthLoginRequest, AuthLoginResponse, AuthUser } from '@/types/api';

let listeners: Array<() => void> = [];

function notifyListeners() {
  listeners.forEach((l) => l());
}

function subscribe(listener: () => void) {
  listeners = [...listeners, listener];
  return () => {
    listeners = listeners.filter((l) => l !== listener);
  };
}

function getSnapshot(): string | null {
  return getToken();
}

export function useAuth() {
  const token = useSyncExternalStore(subscribe, getSnapshot);
  const isAuthenticated = token !== null;

  const login = useCallback(
    async (credentials: AuthLoginRequest): Promise<AuthUser> => {
      const res = await apiPost<AuthLoginResponse>(
        '/auth/login',
        credentials,
      );
      setToken(res.token);
      notifyListeners();
      return res.user;
    },
    [],
  );

  const logout = useCallback(async () => {
    try {
      await apiPost('/auth/logout');
    } finally {
      clearToken();
      notifyListeners();
    }
  }, []);

  const getUser = useCallback(async (): Promise<AuthUser | null> => {
    if (!getToken()) return null;
    try {
      return await apiGet<AuthUser>('/auth/me');
    } catch {
      return null;
    }
  }, []);

  return useMemo(
    () => ({ isAuthenticated, login, logout, getUser }),
    [isAuthenticated, login, logout, getUser],
  );
}
