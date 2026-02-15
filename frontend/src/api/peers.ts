import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiDelete, apiGet, apiPost, apiPut } from './client';
import type { CreatePeerRequest, Peer, UpdatePeerRequest } from '@/types/api';

export const peerKeys = {
  list: (networkId: number) => ['networks', networkId, 'peers'] as const,
  detail: (networkId: number, peerId: number) =>
    ['networks', networkId, 'peers', peerId] as const,
};

export function usePeers(networkId: number) {
  return useQuery({
    queryKey: peerKeys.list(networkId),
    queryFn: () => apiGet<Peer[]>(`/networks/${networkId}/peers`),
    enabled: networkId > 0,
  });
}

export function usePeer(networkId: number, peerId: number) {
  return useQuery({
    queryKey: peerKeys.detail(networkId, peerId),
    queryFn: () =>
      apiGet<Peer>(`/networks/${networkId}/peers/${peerId}`),
    enabled: networkId > 0 && peerId > 0,
  });
}

export function useCreatePeer(networkId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreatePeerRequest) =>
      apiPost<Peer>(`/networks/${networkId}/peers`, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: peerKeys.list(networkId) });
    },
  });
}

export function useUpdatePeer(networkId: number, peerId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: UpdatePeerRequest) =>
      apiPut<Peer>(`/networks/${networkId}/peers/${peerId}`, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: peerKeys.list(networkId) });
      qc.invalidateQueries({
        queryKey: peerKeys.detail(networkId, peerId),
      });
    },
  });
}

export function useDeletePeer(networkId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (peerId: number) =>
      apiDelete(`/networks/${networkId}/peers/${peerId}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: peerKeys.list(networkId) });
    },
  });
}

export function useTogglePeer(networkId: number, peerId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (enable: boolean) =>
      apiPost(
        `/networks/${networkId}/peers/${peerId}/${enable ? 'enable' : 'disable'}`,
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: peerKeys.list(networkId) });
    },
  });
}

export function peerConfigUrl(networkId: number, peerId: number): string {
  return `/api/networks/${networkId}/peers/${peerId}/config`;
}

export function peerQrUrl(networkId: number, peerId: number): string {
  return `/api/networks/${networkId}/peers/${peerId}/qr`;
}
