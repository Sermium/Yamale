import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { t } from '@yamale/chain';
import { acceptMove, type MoveOutcome } from './movein.ts';
import { AppBar } from '../ui.tsx';
import { useSession } from './session.tsx';

/** Lands here from loyalty.yamaletech.com with the sealed account in the fragment. */
export function MoveIn() {
  const navigate = useNavigate();
  const session = useSession();
  const [out, setOut] = useState<MoveOutcome | null>(null);
  // StrictMode runs effects twice in dev; the second run would see the emptied fragment.
  const ran = useRef(false);
  useEffect(() => {
    if (ran.current) return;
    ran.current = true;
    const result = acceptMove(window.location.hash, window.localStorage);
    // Take the sealed account out of the address bar and the history entry first.
    window.history.replaceState(null, '', '/move-in');
    // The session read storage before this account was written; let it see it.
    if (result.kind === 'stored') session.refresh();
    if (result.kind === 'stored' || result.kind === 'same' || result.kind === 'invalid') {
      navigate(result.to, { replace: true });
      return;
    }
    setOut(result);
  }, [navigate, session]);
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
