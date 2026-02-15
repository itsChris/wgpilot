import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useCreatePeer, useUpdatePeer } from '@/api/peers';
import type { Peer } from '@/types/api';

const peerSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  email: z.string().email('Must be a valid email').or(z.literal('')),
  role: z.enum(['client', 'site-gateway']),
  persistent_keepalive: z.coerce.number().int().min(0).max(65535),
});

type PeerFormValues = z.infer<typeof peerSchema>;

interface PeerFormProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  networkId: number;
  peer: Peer | null;
}

export function PeerForm({ open, onOpenChange, networkId, peer }: PeerFormProps) {
  const isEditing = peer !== null;
  const createMutation = useCreatePeer(networkId);
  const updateMutation = useUpdatePeer(networkId, peer?.id ?? 0);

  const form = useForm<PeerFormValues>({
    resolver: zodResolver(peerSchema),
    defaultValues: {
      name: '',
      email: '',
      role: 'client',
      persistent_keepalive: 25,
    },
  });

  useEffect(() => {
    if (open && peer) {
      form.reset({
        name: peer.name,
        email: peer.email,
        role: peer.role,
        persistent_keepalive: peer.persistent_keepalive,
      });
    } else if (open) {
      form.reset({
        name: '',
        email: '',
        role: 'client',
        persistent_keepalive: 25,
      });
    }
  }, [open, peer, form]);

  const onSubmit = async (values: PeerFormValues) => {
    const data = {
      name: values.name,
      email: values.email || undefined,
      role: values.role as 'client' | 'site-gateway',
      persistent_keepalive: values.persistent_keepalive || undefined,
    };
    if (isEditing) {
      await updateMutation.mutateAsync(data);
    } else {
      await createMutation.mutateAsync(data);
    }
    onOpenChange(false);
  };

  const isPending = createMutation.isPending || updateMutation.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{isEditing ? 'Edit Peer' : 'Add Peer'}</DialogTitle>
          <DialogDescription>
            {isEditing
              ? 'Update the peer configuration.'
              : 'Add a new peer to this network. Keys will be generated automatically.'}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Name</FormLabel>
                  <FormControl>
                    <Input placeholder="My Phone" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="email"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Email (optional)</FormLabel>
                  <FormControl>
                    <Input placeholder="user@example.com" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="role"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Role</FormLabel>
                  <Select
                    onValueChange={field.onChange}
                    defaultValue={field.value}
                    value={field.value}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="client">Client</SelectItem>
                      <SelectItem value="site-gateway">Site Gateway</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    Client for standard peers, Site Gateway for site-to-site endpoints.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="persistent_keepalive"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Persistent Keepalive (seconds)</FormLabel>
                  <FormControl>
                    <Input type="number" min={0} max={65535} {...field} />
                  </FormControl>
                  <FormDescription>
                    Set to 25 for peers behind NAT. 0 to disable.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={isPending}>
                {isPending
                  ? 'Saving...'
                  : isEditing
                    ? 'Save Changes'
                    : 'Add Peer'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
