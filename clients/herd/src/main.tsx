import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { AVAILABLE, registerAll, resolveLocale, setLocale } from '@yamale/chain';

import { App } from './App.tsx';
// The shared visual system first, then this surface's own rules on top of it.
import '../../shared/yamale.css';
import './styles.css';

// Theme before first paint. A screen that flashes white before going dark is
// worse than one that was never dark, and on a device carried between direct
// sun and a byre the un-stamped state is the one most readers are in — so
// nothing is written unless a choice was made.
try {
  const saved = localStorage.getItem('yamale.herd.theme');
  if (saved === 'light' || saved === 'dark') {
    document.documentElement.setAttribute('data-theme', saved);
  }
} catch { /* private mode */ }

// Language before first paint too: setLocale writes documentElement.dir, and
// every logical property in the stylesheets reads it.
registerAll();
setLocale(resolveLocale(AVAILABLE));

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
