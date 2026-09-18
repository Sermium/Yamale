import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { App } from './App.tsx';
import { registerAll, AVAILABLE, setLocale, resolveLocale } from '@yamale/chain';
// The shared visual system first, then this surface's own rules on top of it.
import '../../shared/yamale.css';
import './styles.css';

// Theme before first paint. A screen that flashes white before going dark is
// worse than one that was never dark, and nothing is written unless a choice
// was made — the un-stamped state, where only prefers-color-scheme decides, is
// the one most readers are in and it has to keep working.
try {
  const saved = localStorage.getItem('yamale.wallet.theme');
  if (saved === 'light' || saved === 'dark') {
    document.documentElement.setAttribute('data-theme', saved);
  }
} catch { /* private mode */ }

// Language before first paint: setLocale writes documentElement.dir, and every
// logical property in the stylesheet reads it. Doing this after render means a
// visible left-to-right flash for Arabic readers.
registerAll();
setLocale(resolveLocale(AVAILABLE));


const queryClient = new QueryClient({
  defaultOptions: {
    queries: { refetchInterval: 10_000, refetchOnWindowFocus: true, retry: 1, staleTime: 5_000 },
  },
});

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter basename="/wallet">
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
