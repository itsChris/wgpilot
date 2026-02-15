import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormDescription,
} from '@/components/ui/form';
import { useSetupStep2 } from '@/api/setup';
import { apiGet } from '@/api/client';
import { ApiClientError } from '@/api/client';
import type { DetectIPResponse } from '@/types/api';

const step2Schema = z.object({
  public_ip: z
    .string()
    .refine(
      (v) => v === '' || /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(v),
      'Must be a valid IPv4 address',
    ),
  hostname: z.string(),
  dns_servers: z
    .string()
    .refine(
      (v) => {
        if (v === '') return true;
        const parts = v.split(',').map((s) => s.trim());
        if (parts.length > 3) return false;
        return parts.every((p) => /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(p));
      },
      'Up to 3 comma-separated IPv4 addresses',
    ),
});

type Step2Values = z.infer<typeof step2Schema>;

interface StepServerProps {
  onComplete: () => void;
}

export function StepServer({ onComplete }: StepServerProps) {
  const step2 = useSetupStep2();
  const [error, setError] = useState<string | null>(null);
  const [detecting, setDetecting] = useState(false);

  const form = useForm<Step2Values>({
    resolver: zodResolver(step2Schema),
    defaultValues: { public_ip: '', hostname: '', dns_servers: '1.1.1.1,8.8.8.8' },
  });

  const handleDetectIP = async () => {
    setDetecting(true);
    try {
      const res = await apiGet<DetectIPResponse>('/setup/detect-ip');
      if (res.public_ip) {
        form.setValue('public_ip', res.public_ip, { shouldValidate: true });
      }
    } catch {
      // Ignore detection failures — user can enter manually.
    } finally {
      setDetecting(false);
    }
  };

  const onSubmit = async (values: Step2Values) => {
    setError(null);
    try {
      await step2.mutateAsync(values);
      onComplete();
    } catch (err) {
      if (err instanceof ApiClientError) {
        setError(err.message);
      } else {
        setError('Failed to save server settings. Please try again.');
      }
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Server Identity</h2>
        <p className="text-sm text-muted-foreground">
          Configure your server's public-facing identity. Peers will use this
          to connect.
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
            name="public_ip"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Public IP Address</FormLabel>
                <div className="flex gap-2">
                  <FormControl>
                    <Input placeholder="203.0.113.1" {...field} />
                  </FormControl>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={handleDetectIP}
                    disabled={detecting}
                  >
                    {detecting ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      'Detect'
                    )}
                  </Button>
                </div>
                <FormDescription>
                  The IP address peers will connect to.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="hostname"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Hostname (optional)</FormLabel>
                <FormControl>
                  <Input placeholder="vpn.example.com" {...field} />
                </FormControl>
                <FormDescription>
                  A domain name pointing to this server, if available.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="dns_servers"
            render={({ field }) => (
              <FormItem>
                <FormLabel>DNS Servers</FormLabel>
                <FormControl>
                  <Input placeholder="1.1.1.1,8.8.8.8" {...field} />
                </FormControl>
                <FormDescription>
                  Comma-separated DNS servers for peers (max 3).
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <Button
            type="submit"
            className="w-full"
            disabled={step2.isPending}
          >
            {step2.isPending ? 'Saving...' : 'Continue'}
          </Button>
        </form>
      </Form>
    </div>
  );
}
