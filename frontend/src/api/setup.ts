import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiGet, apiPost } from './client';
import type {
  DetectIPResponse,
  SetupStatusResponse,
  SetupStep1Request,
  SetupStep1Response,
  SetupStep2Request,
  SetupStep3Request,
  SetupStep3Response,
  SetupStep4Request,
  SetupStep4Response,
} from '@/types/api';

export const setupKeys = {
  status: ['setup', 'status'] as const,
  detectIP: ['setup', 'detect-ip'] as const,
};

export function useSetupStatus() {
  return useQuery({
    queryKey: setupKeys.status,
    queryFn: () => apiGet<SetupStatusResponse>('/setup/status'),
  });
}

export function useDetectIP() {
  return useQuery({
    queryKey: setupKeys.detectIP,
    queryFn: () => apiGet<DetectIPResponse>('/setup/detect-ip'),
    enabled: false, // Only fetch when manually triggered.
  });
}

export function useSetupStep1() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: SetupStep1Request) =>
      apiPost<SetupStep1Response>('/setup/step/1', data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: setupKeys.status });
    },
  });
}

export function useSetupStep2() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: SetupStep2Request) =>
      apiPost<{ status: string }>('/setup/step/2', data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: setupKeys.status });
    },
  });
}

export function useSetupStep3() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: SetupStep3Request) =>
      apiPost<SetupStep3Response>('/setup/step/3', data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: setupKeys.status });
    },
  });
}

export function useSetupStep4() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: SetupStep4Request) =>
      apiPost<SetupStep4Response>('/setup/step/4', data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: setupKeys.status });
    },
  });
}
