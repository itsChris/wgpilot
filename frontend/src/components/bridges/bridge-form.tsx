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
import { useNetworks } from '@/api/networks';
import { useCreateBridge } from '@/api/bridges';

const bridgeSchema = z.object({
  network_a_id: z.coerce.number().int().positive('Select a network'),
  network_b_id: z.coerce.number().int().positive('Select a network'),
  direction: z.enum(['a_to_b', 'b_to_a', 'bidirectional']),
});

type BridgeFormValues = z.infer<typeof bridgeSchema>;

interface BridgeFormProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function BridgeForm({ open, onOpenChange }: BridgeFormProps) {
  const { data: networks } = useNetworks();
  const createMutation = useCreateBridge();

  const form = useForm<BridgeFormValues>({
    resolver: zodResolver(bridgeSchema),
    defaultValues: {
      network_a_id: 0,
      network_b_id: 0,
      direction: 'bidirectional',
    },
  });

  const onSubmit = async (values: BridgeFormValues) => {
    await createMutation.mutateAsync({
      network_a_id: values.network_a_id,
      network_b_id: values.network_b_id,
      direction: values.direction,
    });
    onOpenChange(false);
    form.reset();
  };

  const directionLabel = (d: string) => {
    switch (d) {
      case 'a_to_b': return 'A to B (one-way)';
      case 'b_to_a': return 'B to A (one-way)';
      case 'bidirectional': return 'Bidirectional';
      default: return d;
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Create Bridge</DialogTitle>
          <DialogDescription>
            Bridge two networks to allow traffic routing between them.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="network_a_id"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Network A</FormLabel>
                  <Select
                    onValueChange={field.onChange}
                    value={field.value ? String(field.value) : undefined}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder="Select network" />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {networks?.map((net) => (
                        <SelectItem key={net.id} value={String(net.id)}>
                          {net.name} ({net.interface})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="network_b_id"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Network B</FormLabel>
                  <Select
                    onValueChange={field.onChange}
                    value={field.value ? String(field.value) : undefined}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder="Select network" />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {networks?.map((net) => (
                        <SelectItem key={net.id} value={String(net.id)}>
                          {net.name} ({net.interface})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="direction"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Direction</FormLabel>
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
                      <SelectItem value="bidirectional">{directionLabel('bidirectional')}</SelectItem>
                      <SelectItem value="a_to_b">{directionLabel('a_to_b')}</SelectItem>
                      <SelectItem value="b_to_a">{directionLabel('b_to_a')}</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    Controls which direction traffic can flow between the networks.
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
              <Button type="submit" disabled={createMutation.isPending}>
                {createMutation.isPending ? 'Creating...' : 'Create Bridge'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
