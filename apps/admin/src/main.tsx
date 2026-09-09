import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createRootRoute, createRoute, createRouter, Outlet, redirect, RouterProvider } from '@tanstack/react-router';
import { AdminRequestError, Login, requireAdmin, Workspace } from './Auth';
import '@gofurry/design/global.scss';
import './layout.css';

const rootRoute = createRootRoute({ component: Outlet });
const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: '/', component: Workspace, beforeLoad: async () => {
  try { await requireAdmin(); }
  catch (error) {
    if (error instanceof AdminRequestError && (error.status === 401 || error.status === 403)) throw redirect({ to: '/login' });
    throw error;
  }
}, errorComponent: () => <main className="p-8"><h1>Admin is unavailable</h1><p>Please try again in a moment.</p><a href="/">Retry</a></main> });
const loginRoute = createRoute({ getParentRoute: () => rootRoute, path: '/login', component: Login });
const router = createRouter({ routeTree: rootRoute.addChildren([indexRoute, loginRoute]) });
const queryClient = new QueryClient({ defaultOptions: { mutations: { gcTime: 0 } } });
declare module '@tanstack/react-router' { interface Register { router: typeof router } }
const element = document.getElementById('root');
if (!element) throw new Error('Missing admin root');
createRoot(element).render(<StrictMode><QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider></StrictMode>);
