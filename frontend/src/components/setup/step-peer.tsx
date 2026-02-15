import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { QRCodeSVG } from 'qrcode.react';
import { Copy, Download, Check } from 'lucide-react';
import { Button } from '@/components/ui/button';
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
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormDescription,
} from '@/components/ui/form';
import { useSetupStep4 } from '@/api/setup';
import { ApiClientError } from '@/api/client';
import type { SetupStep4Response } from '@/types/api';

const step4Schema = z.object({
  name: z
    .string()
    .min(1, 'Name is required')
    .max(64, 'Name must be at most 64 characters'),
  role: z.enum(['client', 'site-gateway']),
  tunnel_type: z.enum(['full', 'split']),
});

type Step4Values = z.infer<typeof step4Schema>;

interface StepPeerProps {
  onComplete: () => void;
}

export function StepPeer({ onComplete }: StepPeerProps) {
  const step4 = useSetupStep4();
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<SetupStep4Response | null>(null);
  const [copied, setCopied] = useState(false);

  const form = useForm<Step4Values>({
    resolver: zodResolver(step4Schema),
    defaultValues: { name: 'My Phone', role: 'client', tunnel_type: 'full' },
  });

  const onSubmit = async (values: Step4Values) => {
    setError(null);
    try {
      const res = await step4.mutateAsync(values);
      setResult(res);
    } catch (err) {
      if (err instanceof ApiClientError) {
        setError(err.message);
      } else {
        setError('Failed to create peer. Please try again.');
      }
    }
  };

  const handleCopyConfig = async () => {
    if (!result) return;
    await navigator.clipboard.writeText(result.config);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDownloadConfig = () => {
    if (!result) return;
    const blob = new Blob([result.config], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${form.getValues('name').replace(/\s+/g, '-').toLowerCase()}.conf`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  // After peer is created, show config + QR.
  if (result) {
    return (
      <div className="space-y-6">
        <div>
          <h2 className="text-lg font-semibold">Peer Created</h2>
          <p className="text-sm text-muted-foreground">
            Scan the QR code with the WireGuard app or download the configuration
            file.
          </p>
        </div>

        <div className="flex justify-center">
          <div className="rounded-lg border bg-white p-4">
            <QRCodeSVG value={result.config} size={200} />
          </div>
        </div>

        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Configuration</span>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleCopyConfig}
              >
                {copied ? (
                  <Check className="mr-1 h-3 w-3" />
                ) : (
                  <Copy className="mr-1 h-3 w-3" />
                )}
                {copied ? 'Copied' : 'Copy'}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleDownloadConfig}
              >
                <Download className="mr-1 h-3 w-3" />
                Download
              </Button>
            </div>
          </div>
          <pre className="max-h-48 overflow-auto rounded-md bg-muted p-3 text-xs">
            {result.config}
          </pre>
        </div>

        <Button className="w-full" onClick={onComplete}>
          Finish Setup
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Add First Peer</h2>
        <p className="text-sm text-muted-foreground">
          Create your first WireGuard peer. You'll get a QR code and config
          file to set up the client.
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
                <FormLabel>Peer Name</FormLabel>
                <FormControl>
                  <Input placeholder="My Phone" {...field} />
                </FormControl>
                <FormDescription>
                  A friendly name for this device.
                </FormDescription>
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
                  Client for end-user devices. Site Gateway for connecting
                  remote LANs.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="tunnel_type"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Tunnel Type</FormLabel>
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
                    <SelectItem value="full">Full Tunnel</SelectItem>
                    <SelectItem value="split">Split Tunnel</SelectItem>
                  </SelectContent>
                </Select>
                <FormDescription>
                  Full tunnel routes all traffic through the VPN. Split tunnel
                  only routes VPN subnet traffic.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <Button
            type="submit"
            className="w-full"
            disabled={step4.isPending}
          >
            {step4.isPending ? 'Creating peer...' : 'Create Peer'}
          </Button>
        </form>
      </Form>
    </div>
  );
}
