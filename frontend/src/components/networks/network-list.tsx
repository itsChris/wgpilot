import { useState } from 'react';
import { Link } from '@tanstack/react-router';
import { Plus, MoreVertical, Trash2, Pencil, Power, ArrowLeftRight } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Skeleton } from '@/components/ui/skeleton';
import { useNetworks, useDeleteNetwork, useToggleNetwork } from '@/api/networks';
import { useBridges } from '@/api/bridges';
import { modeLabel } from '@/lib/format';
import { NetworkForm } from './network-form';
import type { Network } from '@/types/api';

export function NetworkList() {
  const { data: networks, isLoading } = useNetworks();
  const { data: bridges } = useBridges();
  const deleteMutation = useDeleteNetwork();
  const [formOpen, setFormOpen] = useState(false);
  const [editingNetwork, setEditingNetwork] = useState<Network | null>(null);

  const handleEdit = (net: Network) => {
    setEditingNetwork(net);
    setFormOpen(true);
  };

  const handleCreate = () => {
    setEditingNetwork(null);
    setFormOpen(true);
  };

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold">Networks</h1>
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-40" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Networks</h1>
        <Button onClick={handleCreate}>
          <Plus className="mr-2 h-4 w-4" />
          Create Network
        </Button>
      </div>

      {(!networks || networks.length === 0) ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12">
            <p className="mb-4 text-muted-foreground">No networks configured</p>
            <Button onClick={handleCreate}>
              <Plus className="mr-2 h-4 w-4" />
              Create your first network
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {networks.map((net) => {
            const bridgeCount = bridges?.filter(
              (b) => b.network_a_id === net.id || b.network_b_id === net.id,
            ).length ?? 0;
            return (
              <NetworkCard
                key={net.id}
                network={net}
                bridgeCount={bridgeCount}
                onEdit={handleEdit}
                onDelete={() => deleteMutation.mutate(net.id)}
              />
            );
          })}
        </div>
      )}

      <NetworkForm
        open={formOpen}
        onOpenChange={setFormOpen}
        network={editingNetwork}
      />
    </div>
  );
}

function NetworkCard({
  network,
  bridgeCount,
  onEdit,
  onDelete,
}: {
  network: Network;
  bridgeCount: number;
  onEdit: (net: Network) => void;
  onDelete: () => void;
}) {
  const toggleMutation = useToggleNetwork(network.id);

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
        <div>
          <Link
            to="/networks/$networkId"
            params={{ networkId: String(network.id) }}
            className="hover:underline"
          >
            <CardTitle className="text-lg">{network.name}</CardTitle>
          </Link>
          <p className="text-sm text-muted-foreground">{network.interface}</p>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="h-8 w-8">
              <MoreVertical className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => onEdit(network)}>
              <Pencil className="mr-2 h-4 w-4" />
              Edit
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => toggleMutation.mutate(!network.enabled)}
            >
              <Power className="mr-2 h-4 w-4" />
              {network.enabled ? 'Disable' : 'Enable'}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={onDelete}
              className="text-destructive focus:text-destructive"
            >
              <Trash2 className="mr-2 h-4 w-4" />
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <Badge variant={network.enabled ? 'default' : 'secondary'}>
              {network.enabled ? 'Active' : 'Disabled'}
            </Badge>
            <Badge variant="outline">{modeLabel(network.mode)}</Badge>
            {bridgeCount > 0 && (
              <Badge variant="outline" className="gap-1">
                <ArrowLeftRight className="h-3 w-3" />
                {bridgeCount}
              </Badge>
            )}
          </div>
          <div className="grid grid-cols-2 gap-1 text-sm text-muted-foreground">
            <span>Subnet:</span>
            <span className="font-mono">{network.subnet}</span>
            <span>Port:</span>
            <span className="font-mono">{network.listen_port}</span>
            <span>DNS:</span>
            <span className="font-mono truncate">{network.dns_servers || '—'}</span>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
