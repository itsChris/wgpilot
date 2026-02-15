import { useQuery } from '@tanstack/react-query';
import { apiGet } from './client';
import type { StatusResponse } from '@/types/api';

export const statusKeys = {
  all: ['status'] as const,
};

export function useStatus() {
  return useQuery({
    queryKey: statusKeys.all,
    queryFn: () => apiGet<StatusResponse>('/status'),
    refetchInterval: 30000,
  });
}
