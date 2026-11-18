import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { t } from '@yamale/chain';
import { acceptMove, type MoveOutcome } from './movein.ts';
import { AppBar } from '../ui.tsx';

/** Lands here from loyalty.yamaletech.com with the sealed account in the fragment. */
export function MoveIn() {
  const navigate = useNavigate();
  const [out, setOut] = useState<MoveOutcome | null>(null);
  useEffect(() => {
    const result = acceptMove(window.location.hash, window.localStorage);
    // Take the sealed account out of the address bar and the history entry first.
    window.history.replaceState(null, '', '/move-in');
    if (result.kind === 'stored' || result.kind === 'same' || result.kind === 'invalid') {
      navigate(result.to, { replace: true });
      if (result.to.includes('#')) window.location.hash = result.to.slice(result.to.indexOf('#'));
      return;
    }
    setOut(result);
  }, [navigate]);
  if (!out || out.kind !== 'other') return null;
  return (
    <>
      <AppBar title={t('traceability.moveTitle')} eyebrow={t('traceability.title')} />
      <main className="y-screen stack">
        <p className="note">{t('traceability.moveOther', { here: out.here, incoming: out.incoming })}</p>
        <button className="btn btn--primary" onClick={() => navigate(out.to, { replace: true })}>{t('traceability.moveContinue')}</button>
      </main>
    </>
  );
}
