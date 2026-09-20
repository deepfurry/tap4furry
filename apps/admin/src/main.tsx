import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from '@tanstack/react-router';
import { router } from './router';
import '@tap4furry/design/global.scss';
import './layout.css';

const queryClient = new QueryClient({ defaultOptions: { mutations: { gcTime: 0, retry: false } } });
const element = document.getElementById('root');
if (!element) throw new Error('Missing admin root');
createRoot(element).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
