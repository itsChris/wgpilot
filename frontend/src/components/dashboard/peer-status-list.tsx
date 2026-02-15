import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useStatus } from '@/api/status';
import { formatBytes, formatRelativeTime } from '@/lib/format';

export function PeerStatusList() {
  const { data: status, isLoading } = useStatus();

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Peer Status</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </CardContent>
      </Card>
    );
  }

  const networks = status?.networks ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Peer Status</CardTitle>
      </CardHeader>
      <CardContent>
        {networks.length === 0 ? (
          <p className="text-sm text-muted-foreground">No networks configured</p>
        ) : (
          <div className="space-y-4">
            {networks.map((net) => (
              <div key={net.id}>
                <h4 className="mb-2 text-sm font-medium text-muted-foreground">
                  {net.name} ({net.interface})
                </h4>
                {(net.peers ?? []).length === 0 ? (
                  <p className="text-sm text-muted-foreground">No peers</p>
                ) : (
                  <div className="space-y-2">
                    {(net.peers ?? []).map((peer, idx) => (
                      <div
                        key={peer.peer_id || idx}
                        className="flex items-center gap-3 rounded-md border p-3"
                      >
                        <span
                          className={`h-2.5 w-2.5 rounded-full ${
                            peer.online ? 'bg-green-500' : 'bg-gray-300'
                          }`}
                        />
                        <div className="flex-1 min-w-0">
                          <p className="text-sm font-medium truncate">
                            {peer.name || `Peer #${peer.peer_id}`}
                          </p>
                          <p className="text-xs text-muted-foreground">
                            {formatRelativeTime(peer.last_handshake)}
                          </p>
                        </div>
                        <div className="text-right text-xs text-muted-foreground">
                          <p>{formatBytes(peer.transfer_rx)} rx</p>
                          <p>{formatBytes(peer.transfer_tx)} tx</p>
                        </div>
                        <Badge variant={peer.online ? 'default' : 'secondary'}>
                          {peer.online ? 'Online' : 'Offline'}
                        </Badge>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
