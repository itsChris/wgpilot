import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiDelete, apiGet, apiPost, apiPut } from './client';
import type {
  CreateNetworkRequest,
  Network,
  TransferStats,
  UpdateNetworkRequest,
} from '@/types/api';

export const networkKeys = {
  all: ['networks'] as const,
  detail: (id: number) => ['networks', id] as const,
  stats: (id: number, from?: number, to?: number) =>
    ['networks', id, 'stats', { from, to }] as const,
};

export function useNetworks() {
  return useQuery({
    queryKey: networkKeys.all,
    queryFn: () => apiGet<Network[]>('/networks'),
  });
}

export function useNetwork(id: number) {
  return useQuery({
    queryKey: networkKeys.detail(id),
    queryFn: () => apiGet<Network>(`/networks/${id}`),
    enabled: id > 0,
  });
}

export function useNetworkStats(
  id: number,
  from?: number,
  to?: number,
) {
  return useQuery({
    queryKey: networkKeys.stats(id, from, to),
    queryFn: () => {
      const params = new URLSearchParams();
      if (from) params.set('from', String(from));
      if (to) params.set('to', String(to));
      const qs = params.toString();
      return apiGet<TransferStats[]>(
        `/networks/${id}/stats${qs ? `?${qs}` : ''}`,
      );
    },
    enabled: id > 0,
  });
}

export function useCreateNetwork() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateNetworkRequest) =>
      apiPost<Network>('/networks', data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: networkKeys.all });
    },
  });
}

export function useUpdateNetwork(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: UpdateNetworkRequest) =>
      apiPut<Network>(`/networks/${id}`, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: networkKeys.all });
      qc.invalidateQueries({ queryKey: networkKeys.detail(id) });
    },
  });
}

export function useDeleteNetwork() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => apiDelete(`/networks/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: networkKeys.all });
    },
  });
}

export function useToggleNetwork(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (enable: boolean) =>
      apiPost(`/networks/${id}/${enable ? 'enable' : 'disable'}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: networkKeys.all });
      qc.invalidateQueries({ queryKey: networkKeys.detail(id) });
    },
  });
}
