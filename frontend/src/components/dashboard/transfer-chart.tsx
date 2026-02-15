import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useNetworkStats, useNetworks } from '@/api/networks';
import { formatBytes } from '@/lib/format';

function NetworkChart({ networkId, name }: { networkId: number; name: string }) {
  const now = Math.floor(Date.now() / 1000);
  const from = now - 86400; // 24 hours ago
  const { data: stats, isLoading } = useNetworkStats(networkId, from, now);

  if (isLoading) {
    return <Skeleton className="h-64 w-full" />;
  }

  const chartData = (stats ?? []).map((s) => ({
    time: new Date(s.timestamp * 1000).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
    }),
    rx: s.transfer_rx,
    tx: s.transfer_tx,
  }));

  if (chartData.length === 0) {
    return (
      <p className="flex h-48 items-center justify-center text-sm text-muted-foreground">
        No transfer data available for {name}
      </p>
    );
  }

  return (
    <ResponsiveContainer width="100%" height={240}>
      <LineChart data={chartData}>
        <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
        <XAxis
          dataKey="time"
          tick={{ fontSize: 12 }}
          className="text-muted-foreground"
        />
        <YAxis
          tickFormatter={(v: number) => formatBytes(v)}
          tick={{ fontSize: 12 }}
          className="text-muted-foreground"
          width={70}
        />
        <Tooltip
          formatter={(value) => formatBytes(Number(value ?? 0))}
        />
        <Legend />
        <Line
          type="monotone"
          dataKey="rx"
          name="Received"
          stroke="hsl(var(--chart-1))"
          strokeWidth={2}
          dot={false}
        />
        <Line
          type="monotone"
          dataKey="tx"
          name="Sent"
          stroke="hsl(var(--chart-2))"
          strokeWidth={2}
          dot={false}
        />
      </LineChart>
    </ResponsiveContainer>
  );
}

export function TransferChart() {
  const { data: networks, isLoading } = useNetworks();

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Transfer History (24h)</CardTitle>
        </CardHeader>
        <CardContent>
          <Skeleton className="h-64 w-full" />
        </CardContent>
      </Card>
    );
  }

  if (!networks || networks.length === 0) {
    return null;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Transfer History (24h)</CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        {networks.map((net) => (
          <div key={net.id}>
            <h4 className="mb-2 text-sm font-medium">
              {net.name} ({net.interface})
            </h4>
            <NetworkChart networkId={net.id} name={net.name} />
          </div>
        ))}
      </CardContent>
    </Card>
  );
}
