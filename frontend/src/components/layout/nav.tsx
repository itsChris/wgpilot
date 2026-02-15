import { Link, useMatchRoute } from '@tanstack/react-router';
import {
  LayoutDashboard,
  Network,
  Shield,
} from 'lucide-react';
import { useNetworks } from '@/api/networks';
import { cn } from '@/lib/utils';

export function Nav() {
  const { data: networks } = useNetworks();
  const matchRoute = useMatchRoute();

  return (
    <nav className="flex flex-col gap-1 p-3">
      <Link
        to="/"
        className={cn(
          'flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-accent',
          matchRoute({ to: '/' }) && 'bg-accent text-accent-foreground',
        )}
      >
        <LayoutDashboard className="h-4 w-4" />
        Dashboard
      </Link>
      <Link
        to="/networks"
        className={cn(
          'flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-accent',
          matchRoute({ to: '/networks', fuzzy: true }) &&
            'bg-accent text-accent-foreground',
        )}
      >
        <Network className="h-4 w-4" />
        Networks
      </Link>

      {networks && networks.length > 0 && (
        <div className="ml-4 mt-1 flex flex-col gap-0.5">
          {networks.map((net) => (
            <Link
              key={net.id}
              to="/networks/$networkId"
              params={{ networkId: String(net.id) }}
              className={cn(
                'flex items-center gap-2 rounded-md px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground',
                matchRoute({
                  to: '/networks/$networkId',
                  params: { networkId: String(net.id) },
                }) && 'bg-accent text-accent-foreground',
              )}
            >
              <Shield className="h-3.5 w-3.5" />
              <span className="truncate">{net.name}</span>
              <span
                className={cn(
                  'ml-auto h-2 w-2 rounded-full',
                  net.enabled ? 'bg-green-500' : 'bg-gray-300',
                )}
              />
            </Link>
          ))}
        </div>
      )}
    </nav>
  );
}
