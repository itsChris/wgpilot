import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
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
import { useSetupStep1 } from '@/api/setup';
import { setToken } from '@/api/client';
import { ApiClientError } from '@/api/client';

const step1Schema = z.object({
  otp: z.string().min(1, 'Setup password is required'),
  username: z
    .string()
    .min(1, 'Username is required')
    .max(64, 'Username must be at most 64 characters')
    .regex(
      /^[a-zA-Z0-9_-]+$/,
      'Only letters, numbers, hyphens, and underscores',
    ),
  password: z.string().min(10, 'Password must be at least 10 characters'),
  confirmPassword: z.string(),
}).refine((data) => data.password === data.confirmPassword, {
  message: 'Passwords do not match',
  path: ['confirmPassword'],
});

type Step1Values = z.infer<typeof step1Schema>;

interface StepAdminProps {
  onComplete: () => void;
}

export function StepAdmin({ onComplete }: StepAdminProps) {
  const step1 = useSetupStep1();
  const [error, setError] = useState<string | null>(null);

  const form = useForm<Step1Values>({
    resolver: zodResolver(step1Schema),
    defaultValues: { otp: '', username: 'admin', password: '', confirmPassword: '' },
  });

  const onSubmit = async (values: Step1Values) => {
    setError(null);
    try {
      const res = await step1.mutateAsync({
        otp: values.otp,
        username: values.username,
        password: values.password,
      });
      // The server sets a cookie, but the SPA also needs a token for API calls.
      // The step1 response includes the user but the token comes via cookie.
      // For cookie-based auth, we don't need to store a token — the cookie is set
      // automatically. But our client.ts uses localStorage. The response doesn't
      // include a token field, so we use the cookie directly. For the SPA pattern,
      // we need to mark auth as ready. We'll just set a marker.
      if (res.user) {
        // Token is in the cookie — for Bearer auth fallback, re-login would be needed.
        // But the setup endpoints after step1 use cookie auth which is set automatically.
        setToken('setup-session');
      }
      onComplete();
    } catch (err) {
      if (err instanceof ApiClientError) {
        setError(err.message);
      } else {
        setError('Failed to create admin account. Please try again.');
      }
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Create Admin Account</h2>
        <p className="text-sm text-muted-foreground">
          Enter the setup password shown in the server logs, then create your
          admin account.
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
            name="otp"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Setup Password</FormLabel>
                <FormControl>
                  <Input
                    type="password"
                    placeholder="Paste from server logs"
                    autoComplete="off"
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  Found in the server startup logs or terminal output.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="username"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Username</FormLabel>
                <FormControl>
                  <Input autoComplete="username" {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="password"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Password</FormLabel>
                <FormControl>
                  <Input
                    type="password"
                    autoComplete="new-password"
                    {...field}
                  />
                </FormControl>
                <FormDescription>At least 10 characters.</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="confirmPassword"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Confirm Password</FormLabel>
                <FormControl>
                  <Input
                    type="password"
                    autoComplete="new-password"
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <Button
            type="submit"
            className="w-full"
            disabled={step1.isPending}
          >
            {step1.isPending ? 'Creating account...' : 'Create Admin Account'}
          </Button>
        </form>
      </Form>
    </div>
  );
}
