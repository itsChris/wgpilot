import { StatsCards } from '@/components/dashboard/stats-cards';
import { PeerStatusList } from '@/components/dashboard/peer-status-list';
import { TransferChart } from '@/components/dashboard/transfer-chart';

export function DashboardPage() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Dashboard</h1>
      <StatsCards />
      <div className="grid gap-6 lg:grid-cols-2">
        <PeerStatusList />
        <TransferChart />
      </div>
    </div>
  );
}
