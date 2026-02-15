import { useEffect, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { getToken } from '@/api/client';
import { peerKeys } from '@/api/peers';
import type { Peer, PeerStatus } from '@/types/api';

export function useSSE(networkId: number) {
  const queryClient = useQueryClient();
  const eventSourceRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!networkId || networkId <= 0) return;

    const token = getToken();
    const url = `/api/networks/${networkId}/events${token ? `?token=${encodeURIComponent(token)}` : ''}`;
    const es = new EventSource(url);
    eventSourceRef.current = es;

    es.addEventListener('status', (event) => {
      const statuses: PeerStatus[] = JSON.parse(event.data);
      queryClient.setQueryData<Peer[]>(
        peerKeys.list(networkId),
        (old) =>
          old?.map((p) => {
            const update = statuses.find((s) => s.peer_id === p.id);
            return update
              ? {
                  ...p,
                  online: update.online,
                  last_handshake: update.last_handshake,
                  transfer_rx: update.transfer_rx,
                  transfer_tx: update.transfer_tx,
                }
              : p;
          }),
      );
    });

    es.onerror = () => {
      es.close();
    };

    return () => {
      es.close();
      eventSourceRef.current = null;
    };
  }, [networkId, queryClient]);
}
