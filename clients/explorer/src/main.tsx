import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { App } from './App.tsx';
import { registerAll, AVAILABLE, setLocale, resolveLocale } from '@yamale/chain';
import { registerExplorerStrings } from './strings.ts';
// The shared visual system first: typefaces, the type scale, elevation and the
// semantic colours. Imported from clients/shared rather than copied, so this
// surface cannot drift from the others — a copy is a copy somebody forgets.
// styles.css layers the explorer's own components on top and defines nothing
// the shared file already defines.
import '../../shared/yamale.css';
import './styles.css';
import './explorer-ui.css';

// Language before first paint: setLocale writes documentElement.dir, and every
// logical property in the stylesheet reads it. Doing this after render means a
// visible left-to-right flash for Arabic readers.
registerAll();
registerExplorerStrings();
setLocale(resolveLocale(AVAILABLE));


/**
 * The chain moves, so the explorer polls. Five seconds is comfortably longer
 * than a block, which keeps the page current without hammering a node that may
 * be somebody's laptop.
 */
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchInterval: 5000,
      refetchOnWindowFocus: true,
      retry: 1,
      staleTime: 2000,
    },
  },
});

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter basename="/explorer">
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
