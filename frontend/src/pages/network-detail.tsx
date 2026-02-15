import { useParams } from '@tanstack/react-router';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { useNetwork } from '@/api/networks';
import { PeerTable } from '@/components/peers/peer-table';
import { modeLabel } from '@/lib/format';

export function NetworkDetailPage() {
  const { networkId } = useParams({ strict: false });
  const id = Number(networkId);
  const { data: network, isLoading } = useNetwork(id);

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (!network) {
    return (
      <div className="flex h-64 items-center justify-center text-muted-foreground">
        Network not found
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-bold">{network.name}</h1>
          <Badge variant={network.enabled ? 'default' : 'secondary'}>
            {network.enabled ? 'Active' : 'Disabled'}
          </Badge>
          <Badge variant="outline">{modeLabel(network.mode)}</Badge>
        </div>
        <p className="mt-1 text-sm text-muted-foreground">
          {network.interface} &middot; {network.subnet} &middot; Port{' '}
          {network.listen_port}
        </p>
      </div>

      <PeerTable networkId={id} />
    </div>
  );
}
