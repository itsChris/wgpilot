import { useState } from 'react';
import { ArrowRight, ArrowLeftRight, Plus, Trash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useBridges, useDeleteBridge } from '@/api/bridges';
import { BridgeForm } from './bridge-form';
import type { Bridge } from '@/types/api';

function directionIcon(direction: string) {
  if (direction === 'bidirectional') return <ArrowLeftRight className="h-4 w-4" />;
  return <ArrowRight className="h-4 w-4" />;
}

function directionLabel(bridge: Bridge) {
  switch (bridge.direction) {
    case 'a_to_b':
      return `${bridge.interface_a} \u2192 ${bridge.interface_b}`;
    case 'b_to_a':
      return `${bridge.interface_b} \u2192 ${bridge.interface_a}`;
    case 'bidirectional':
      return `${bridge.interface_a} \u2194 ${bridge.interface_b}`;
    default:
      return bridge.direction;
  }
}

interface BridgeListProps {
  networkId?: number;
}

export function BridgeList({ networkId }: BridgeListProps) {
  const { data: bridges, isLoading } = useBridges();
  const deleteMutation = useDeleteBridge();
  const [formOpen, setFormOpen] = useState(false);

  const filtered = networkId
    ? bridges?.filter(
        (b) => b.network_a_id === networkId || b.network_b_id === networkId,
      )
    : bridges;

  if (isLoading) return null;

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Bridges</h2>
        <Button size="sm" variant="outline" onClick={() => setFormOpen(true)}>
          <Plus className="mr-1 h-3 w-3" />
          Add Bridge
        </Button>
      </div>

      {(!filtered || filtered.length === 0) ? (
        <p className="text-sm text-muted-foreground">
          No bridges configured. Bridges allow traffic routing between networks.
        </p>
      ) : (
        <div className="space-y-2">
          {filtered.map((bridge) => (
            <Card key={bridge.id}>
              <CardContent className="flex items-center justify-between py-3">
                <div className="flex items-center gap-3">
                  {directionIcon(bridge.direction)}
                  <div>
                    <div className="flex items-center gap-2 text-sm font-medium">
                      <span>{bridge.network_a_name}</span>
                      <span className="text-muted-foreground">
                        {directionLabel(bridge)}
                      </span>
                      <span>{bridge.network_b_name}</span>
                    </div>
                  </div>
                  <Badge variant="outline" className="ml-2">
                    {bridge.direction === 'bidirectional'
                      ? 'Bidirectional'
                      : 'One-way'}
                  </Badge>
                </div>
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-8 w-8 text-destructive hover:text-destructive"
                  onClick={() => deleteMutation.mutate(bridge.id)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <BridgeForm open={formOpen} onOpenChange={setFormOpen} />
    </div>
  );
}
