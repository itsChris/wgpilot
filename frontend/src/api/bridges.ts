import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiDelete, apiGet, apiPost } from './client';
import type { Bridge, CreateBridgeRequest } from '@/types/api';

export const bridgeKeys = {
  all: ['bridges'] as const,
  detail: (id: number) => ['bridges', id] as const,
};

export function useBridges() {
  return useQuery({
    queryKey: bridgeKeys.all,
    queryFn: () => apiGet<Bridge[]>('/bridges'),
  });
}

export function useBridge(id: number) {
  return useQuery({
    queryKey: bridgeKeys.detail(id),
    queryFn: () => apiGet<Bridge>(`/bridges/${id}`),
    enabled: id > 0,
  });
}

export function useCreateBridge() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateBridgeRequest) =>
      apiPost<Bridge>('/bridges', data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: bridgeKeys.all });
    },
  });
}

export function useDeleteBridge() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => apiDelete(`/bridges/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: bridgeKeys.all });
    },
  });
}
