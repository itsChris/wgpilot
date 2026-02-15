import { Network, Users, Wifi, ArrowUpDown } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useStatus } from '@/api/status';
import { formatBytes } from '@/lib/format';

export function StatsCards() {
  const { data: status, isLoading } = useStatus();

  if (isLoading) {
    return (
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i}>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <Skeleton className="h-4 w-24" />
            </CardHeader>
            <CardContent>
              <Skeleton className="h-8 w-16" />
            </CardContent>
          </Card>
        ))}
      </div>
    );
  }

  const networks = status?.networks ?? [];
  const totalNetworks = networks.length;
  const allPeers = networks.flatMap((n) => n.peers ?? []);
  const totalPeers = allPeers.length;
  const onlinePeers = allPeers.filter((p) => p.online).length;
  const totalRx = allPeers.reduce((sum, p) => sum + p.transfer_rx, 0);
  const totalTx = allPeers.reduce((sum, p) => sum + p.transfer_tx, 0);

  const cards = [
    {
      title: 'Networks',
      value: String(totalNetworks),
      icon: Network,
      subtitle: `${networks.filter((n) => n.up).length} active`,
    },
    {
      title: 'Total Peers',
      value: String(totalPeers),
      icon: Users,
      subtitle: `Across ${totalNetworks} network${totalNetworks !== 1 ? 's' : ''}`,
    },
    {
      title: 'Online Peers',
      value: String(onlinePeers),
      icon: Wifi,
      subtitle: totalPeers
        ? `${Math.round((onlinePeers / totalPeers) * 100)}% connected`
        : 'No peers',
    },
    {
      title: 'Total Transfer',
      value: formatBytes(totalRx + totalTx),
      icon: ArrowUpDown,
      subtitle: `${formatBytes(totalRx)} rx / ${formatBytes(totalTx)} tx`,
    },
  ];

  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      {cards.map((card) => (
        <Card key={card.title}>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">{card.title}</CardTitle>
            <card.icon className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{card.value}</div>
            <p className="text-xs text-muted-foreground">{card.subtitle}</p>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
