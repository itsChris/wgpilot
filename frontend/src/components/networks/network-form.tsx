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
import { Switch } from '@/components/ui/switch';
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
import { useCreateNetwork, useUpdateNetwork } from '@/api/networks';
import type { Network } from '@/types/api';

const networkSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  mode: z.enum(['gateway', 'site-to-site', 'hub-routed']),
  subnet: z
    .string()
    .min(1, 'Subnet is required')
    .regex(
      /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\/\d{1,2}$/,
      'Must be a valid CIDR (e.g. 10.0.0.0/24)',
    ),
  listen_port: z.coerce.number().int().min(1).max(65535),
  dns_servers: z.string(),
  nat_enabled: z.boolean(),
  inter_peer_routing: z.boolean(),
});

type NetworkFormValues = z.infer<typeof networkSchema>;

interface NetworkFormProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  network: Network | null;
}

export function NetworkForm({ open, onOpenChange, network }: NetworkFormProps) {
  const isEditing = network !== null;
  const createMutation = useCreateNetwork();
  const updateMutation = useUpdateNetwork(network?.id ?? 0);

  const form = useForm<NetworkFormValues>({
    resolver: zodResolver(networkSchema),
    defaultValues: {
      name: '',
      mode: 'gateway',
      subnet: '10.0.0.0/24',
      listen_port: 51820,
      dns_servers: '1.1.1.1,8.8.8.8',
      nat_enabled: true,
      inter_peer_routing: false,
    },
  });

  useEffect(() => {
    if (open && network) {
      form.reset({
        name: network.name,
        mode: network.mode,
        subnet: network.subnet,
        listen_port: network.listen_port,
        dns_servers: network.dns_servers,
        nat_enabled: network.nat_enabled,
        inter_peer_routing: network.inter_peer_routing,
      });
    } else if (open) {
      form.reset({
        name: '',
        mode: 'gateway',
        subnet: '10.0.0.0/24',
        listen_port: 51820,
        dns_servers: '1.1.1.1,8.8.8.8',
        nat_enabled: true,
        inter_peer_routing: false,
      });
    }
  }, [open, network, form]);

  const onSubmit = async (values: NetworkFormValues) => {
    if (isEditing) {
      await updateMutation.mutateAsync(values);
    } else {
      await createMutation.mutateAsync(values);
    }
    onOpenChange(false);
  };

  const isPending = createMutation.isPending || updateMutation.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isEditing ? 'Edit Network' : 'Create Network'}
          </DialogTitle>
          <DialogDescription>
            {isEditing
              ? 'Update the network configuration.'
              : 'Create a new WireGuard network.'}
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
                    <Input placeholder="Home VPN" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="mode"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Topology Mode</FormLabel>
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
                      <SelectItem value="gateway">VPN Gateway</SelectItem>
                      <SelectItem value="site-to-site">Site-to-Site</SelectItem>
                      <SelectItem value="hub-routed">Hub with Peer Routing</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    Determines how peers connect and route traffic.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="grid grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="subnet"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Subnet</FormLabel>
                    <FormControl>
                      <Input placeholder="10.0.0.0/24" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="listen_port"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Listen Port</FormLabel>
                    <FormControl>
                      <Input type="number" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormField
              control={form.control}
              name="dns_servers"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>DNS Servers</FormLabel>
                  <FormControl>
                    <Input placeholder="1.1.1.1,8.8.8.8" {...field} />
                  </FormControl>
                  <FormDescription>Comma-separated DNS servers for peers.</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="flex items-center gap-6">
              <FormField
                control={form.control}
                name="nat_enabled"
                render={({ field }) => (
                  <FormItem className="flex items-center gap-2 space-y-0">
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormLabel className="font-normal">NAT (Masquerade)</FormLabel>
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="inter_peer_routing"
                render={({ field }) => (
                  <FormItem className="flex items-center gap-2 space-y-0">
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormLabel className="font-normal">Inter-peer Routing</FormLabel>
                  </FormItem>
                )}
              />
            </div>
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
                    : 'Create Network'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
