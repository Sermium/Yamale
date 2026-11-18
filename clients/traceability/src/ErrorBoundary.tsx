/**
 * The last net: a screen that throws while rendering would otherwise blank the
 * whole app. This shows a plain sentence and a way back.
 */

import { Component, type ReactNode } from 'react';
import { t } from '@yamale/chain';

export class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() { return { failed: true }; }

  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <main className="y-screen stack" role="alert">
        <p>{t('traceability.crashed')}</p>
        <button type="button" className="btn" onClick={() => window.location.reload()}>{t('traceability.reload')}</button>
      </main>
    );
  }
}
