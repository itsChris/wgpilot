import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Button } from '@/components/ui/button';
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
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormDescription,
} from '@/components/ui/form';
import { useSetupStep3 } from '@/api/setup';
import { ApiClientError } from '@/api/client';

const step3Schema = z.object({
  name: z.string().min(1, 'Name is required').max(64),
  mode: z.enum(['gateway', 'site-to-site', 'hub-routed']),
  subnet: z
    .string()
    .min(1, 'Subnet is required')
    .regex(
      /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\/\d{1,2}$/,
      'Must be a valid CIDR (e.g. 10.0.0.0/24)',
    ),
  listen_port: z.coerce.number().int().min(1).max(65535),
  nat_enabled: z.boolean(),
  inter_peer_routing: z.boolean(),
});

type Step3Values = z.infer<typeof step3Schema>;

interface StepNetworkProps {
  onComplete: () => void;
}

export function StepNetwork({ onComplete }: StepNetworkProps) {
  const step3 = useSetupStep3();
  const [error, setError] = useState<string | null>(null);

  const form = useForm<Step3Values>({
    resolver: zodResolver(step3Schema),
    defaultValues: {
      name: 'Home VPN',
      mode: 'gateway',
      subnet: '10.0.0.0/24',
      listen_port: 51820,
      nat_enabled: true,
      inter_peer_routing: false,
    },
  });

  const onSubmit = async (values: Step3Values) => {
    setError(null);
    try {
      await step3.mutateAsync(values);
      onComplete();
    } catch (err) {
      if (err instanceof ApiClientError) {
        setError(err.message);
      } else {
        setError('Failed to create network. Please try again.');
      }
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Create First Network</h2>
        <p className="text-sm text-muted-foreground">
          Set up your first WireGuard network. You can add more networks later.
        </p>
      </div>

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
          {error && (
            <div className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </div>
          )}
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Network Name</FormLabel>
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
                  Gateway routes all traffic through this server. Site-to-Site
                  connects remote LANs. Hub routes traffic between peers.
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
          <Button
            type="submit"
            className="w-full"
            disabled={step3.isPending}
          >
            {step3.isPending ? 'Creating network...' : 'Create Network'}
          </Button>
        </form>
      </Form>
    </div>
  );
}
