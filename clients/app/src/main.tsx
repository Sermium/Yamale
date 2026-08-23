import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { registerAll, AVAILABLE, setLocale, resolveLocale } from '@yamale/chain';

import { App } from './App.tsx';
// The shared visual system first, then this surface's own rules on top of it.
import '../../shared/yamale.css';
import './styles.css';

// Language before first paint: setLocale writes documentElement.dir, and every
// logical property in the stylesheet reads it.
// Theme before first paint, for the same reason as direction: a screen that
// flashes white before going dark is worse than one that was never dark.
const savedTheme = localStorage.getItem('yamale.app.theme');
if (savedTheme === 'light' || savedTheme === 'dark') {
  document.documentElement.setAttribute('data-theme', savedTheme);
}

registerAll();
setLocale(resolveLocale(AVAILABLE));

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

// Register the service worker so the app can be installed to a home screen and
// still open without a signal. Registration failure is not fatal — the app is a
// perfectly good website without it.
if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('sw.js', { scope: './' }).catch(() => {});
  });
}
