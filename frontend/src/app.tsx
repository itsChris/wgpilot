import {
  createRouter,
  createRoute,
  createRootRoute,
  RouterProvider,
  redirect,
  Outlet,
} from '@tanstack/react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { apiGet, getToken } from '@/api/client';
import { AppShell } from '@/components/layout/app-shell';
import { LoginPage } from '@/components/auth/login-page';
import { SetupWizard } from '@/components/setup/wizard';
import { DashboardPage } from '@/pages/dashboard';
import { NetworksPage } from '@/pages/networks';
import { NetworkDetailPage } from '@/pages/network-detail';
import type { SetupStatusResponse } from '@/types/api';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30000,
      retry: 1,
    },
  },
});

// Root route
const rootRoute = createRootRoute({
  component: Outlet,
});

// Setup route (no auth required — setup status is public)
const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/setup',
  component: SetupWizard,
});

// Login route (no auth required)
const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  beforeLoad: async () => {
    // If setup is not complete, redirect to setup wizard.
    try {
      const status = await queryClient.fetchQuery({
        queryKey: ['setup', 'status'],
        queryFn: () => apiGet<SetupStatusResponse>('/setup/status'),
        staleTime: 10000,
      });
      if (!status.complete) {
        throw redirect({ to: '/setup' });
      }
    } catch (err) {
      if (err && typeof err === 'object' && 'to' in err) throw err;
      // If the status check fails, proceed to login.
    }
  },
  component: LoginPage,
});

// Authenticated layout route
const authLayoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'auth',
  beforeLoad: async () => {
    // Check setup status first.
    try {
      const status = await queryClient.fetchQuery({
        queryKey: ['setup', 'status'],
        queryFn: () => apiGet<SetupStatusResponse>('/setup/status'),
        staleTime: 10000,
      });
      if (!status.complete) {
        throw redirect({ to: '/setup' });
      }
    } catch (err) {
      if (err && typeof err === 'object' && 'to' in err) throw err;
      // If status check fails, proceed with auth check.
    }
    if (!getToken()) {
      throw redirect({ to: '/login' });
    }
  },
  component: AppShell,
});

// Dashboard
const dashboardRoute = createRoute({
  getParentRoute: () => authLayoutRoute,
  path: '/',
  component: DashboardPage,
});

// Networks list
const networksRoute = createRoute({
  getParentRoute: () => authLayoutRoute,
  path: '/networks',
  component: NetworksPage,
});

// Network detail with peer table
const networkDetailRoute = createRoute({
  getParentRoute: () => authLayoutRoute,
  path: '/networks/$networkId',
  component: NetworkDetailPage,
});

const routeTree = rootRoute.addChildren([
  setupRoute,
  loginRoute,
  authLayoutRoute.addChildren([
    dashboardRoute,
    networksRoute,
    networkDetailRoute,
  ]),
]);

const router = createRouter({ routeTree });

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}
