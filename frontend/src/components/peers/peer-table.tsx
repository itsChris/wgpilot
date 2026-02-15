import { useState } from 'react';
import {
  Plus,
  MoreVertical,
  Trash2,
  Pencil,
  Power,
  QrCode,
  Download,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { usePeers, useDeletePeer, useTogglePeer, peerConfigUrl } from '@/api/peers';
import { useSSE } from '@/hooks/use-sse';
import { formatBytes, formatRelativeTime } from '@/lib/format';
import { PeerForm } from './peer-form';
import { PeerConfigModal } from './peer-config-modal';
import type { Peer } from '@/types/api';

interface PeerTableProps {
  networkId: number;
}

export function PeerTable({ networkId }: PeerTableProps) {
  const { data: peers, isLoading } = usePeers(networkId);
  const deleteMutation = useDeletePeer(networkId);
  const [formOpen, setFormOpen] = useState(false);
  const [editingPeer, setEditingPeer] = useState<Peer | null>(null);
  const [configPeer, setConfigPeer] = useState<Peer | null>(null);

  // Live SSE updates for peer status
  useSSE(networkId);

  const handleCreate = () => {
    setEditingPeer(null);
    setFormOpen(true);
  };

  const handleEdit = (peer: Peer) => {
    setEditingPeer(peer);
    setFormOpen(true);
  };

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold">Peers</h2>
        </div>
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Peers</h2>
        <Button onClick={handleCreate}>
          <Plus className="mr-2 h-4 w-4" />
          Add Peer
        </Button>
      </div>

      {(!peers || peers.length === 0) ? (
        <div className="flex flex-col items-center justify-center rounded-md border py-12">
          <p className="mb-4 text-muted-foreground">No peers in this network</p>
          <Button onClick={handleCreate}>
            <Plus className="mr-2 h-4 w-4" />
            Add your first peer
          </Button>
        </div>
      ) : (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-8">Status</TableHead>
                <TableHead>Name</TableHead>
                <TableHead className="hidden md:table-cell">IP</TableHead>
                <TableHead className="hidden lg:table-cell">Last Seen</TableHead>
                <TableHead className="hidden md:table-cell">Transfer</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {peers.map((peer) => (
                <PeerRow
                  key={peer.id}
                  peer={peer}
                  networkId={networkId}
                  onEdit={handleEdit}
                  onDelete={() => deleteMutation.mutate(peer.id)}
                  onShowConfig={() => setConfigPeer(peer)}
                />
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <PeerForm
        open={formOpen}
        onOpenChange={setFormOpen}
        networkId={networkId}
        peer={editingPeer}
      />

      <PeerConfigModal
        open={configPeer !== null}
        onOpenChange={(open) => !open && setConfigPeer(null)}
        networkId={networkId}
        peer={configPeer}
      />
    </div>
  );
}

function PeerRow({
  peer,
  networkId,
  onEdit,
  onDelete,
  onShowConfig,
}: {
  peer: Peer;
  networkId: number;
  onEdit: (peer: Peer) => void;
  onDelete: () => void;
  onShowConfig: () => void;
}) {
  const toggleMutation = useTogglePeer(networkId, peer.id);

  return (
    <TableRow>
      <TableCell>
        <span
          className={`inline-block h-2.5 w-2.5 rounded-full ${
            peer.online ? 'bg-green-500' : 'bg-gray-300'
          }`}
        />
      </TableCell>
      <TableCell>
        <div>
          <p className="font-medium">{peer.name}</p>
          {peer.email && (
            <p className="text-xs text-muted-foreground">{peer.email}</p>
          )}
        </div>
      </TableCell>
      <TableCell className="hidden font-mono text-sm md:table-cell">
        {peer.allowed_ips}
      </TableCell>
      <TableCell className="hidden text-sm lg:table-cell">
        {formatRelativeTime(peer.last_handshake)}
      </TableCell>
      <TableCell className="hidden text-sm md:table-cell">
        <span className="text-muted-foreground">
          {formatBytes(peer.transfer_rx)} / {formatBytes(peer.transfer_tx)}
        </span>
      </TableCell>
      <TableCell>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="h-8 w-8">
              <MoreVertical className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={onShowConfig}>
              <QrCode className="mr-2 h-4 w-4" />
              Show Config
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <a href={peerConfigUrl(networkId, peer.id)} download>
                <Download className="mr-2 h-4 w-4" />
                Download .conf
              </a>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => onEdit(peer)}>
              <Pencil className="mr-2 h-4 w-4" />
              Edit
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => toggleMutation.mutate(!peer.enabled)}
            >
              <Power className="mr-2 h-4 w-4" />
              {peer.enabled ? 'Disable' : 'Enable'}
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
      </TableCell>
    </TableRow>
  );
}
