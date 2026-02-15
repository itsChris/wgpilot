import {
  createRouter,
  createRoute,
  createRootRoute,
  RouterProvider,
  redirect,
  Outlet,
} from '@tanstack/react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { getToken } from '@/api/client';
import { AppShell } from '@/components/layout/app-shell';
import { LoginPage } from '@/components/auth/login-page';
import { DashboardPage } from '@/pages/dashboard';
import { NetworksPage } from '@/pages/networks';
import { NetworkDetailPage } from '@/pages/network-detail';

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

// Login route (no auth required)
const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  component: LoginPage,
});

// Authenticated layout route
const authLayoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'auth',
  beforeLoad: () => {
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
