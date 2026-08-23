import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { App } from './App.tsx';
import { registerAll, AVAILABLE, setLocale, resolveLocale } from '@yamale/chain';
// The shared visual system first, then this surface's own rules on top of it.
// Order matters: styles.css maps the local component names onto yamale.css's
// tokens, and a token has to exist before something reads it.
import '../../shared/yamale.css';
import './styles.css';

// Language before first paint: setLocale writes documentElement.dir, and every
// logical property in the stylesheet reads it. Doing this after render means a
// visible left-to-right flash for Arabic readers.
registerAll();
setLocale(resolveLocale(AVAILABLE));


/**
 * Slower polling than the explorer's. A treasury changes when somebody
 * proposes or approves something, not every block, and a safe that refetched
 * constantly would spend a signer's afternoon re-rendering a list that had not
 * moved.
 */
const queryClient = new QueryClient({
  defaultOptions: {
    queries: { refetchInterval: 10_000, refetchOnWindowFocus: true, retry: 1, staleTime: 5_000 },
  },
});

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter basename="/safe">
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
