import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { AVAILABLE, registerAll, resolveLocale, setLocale } from '@yamale/chain';

import { App } from './App.tsx';
import '../../shared/yamale.css';
import './styles.css';

registerAll();
setLocale(resolveLocale(AVAILABLE));

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
