import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { findByInscription, normaliseInscription, readJeweler, t, type Jewel } from '@yamale/chain';

import { AppBar, Problem } from '../ui.tsx';

type State =
  | { kind: 'idle' } | { kind: 'busy' }
  | { kind: 'failed'; outcome: { reason: string; detail: string } }
  | { kind: 'results'; items: { jewel: Jewel; jewelerName: string }[] };

export function Search() {
  const [text, setText] = useState('');
  const [state, setState] = useState<State>({ kind: 'idle' });
  const navigate = useNavigate();
  const code = normaliseInscription(text);

  async function search(e: FormEvent) {
    e.preventDefault();
    if (!code) return;
    setState({ kind: 'busy' });
    const out = await findByInscription(code);
    if (!out.ok) { setState({ kind: 'failed', outcome: out }); return; }
    const items = await Promise.all(out.value.map(async (jewel) => {
      const jr = await readJeweler(jewel.jewelerId);
      return { jewel, jewelerName: jr.ok ? jr.value.name : '' };
    }));
    if (items.length === 1) { navigate(`/j/${items[0]!.jewel.jewelerId}/${items[0]!.jewel.id}`); return; }
    setState({ kind: 'results', items });
  }

  return (
    <>
      <AppBar title={t('traceability.verifyTitle')} eyebrow={t('traceability.title')} />
      <main className="y-screen">
        <section className="hero">
          <h1 className="hero__title">{t('traceability.verifyHeadline')}</h1>
          <p className="hero__lede">{t('traceability.verifyLede')}</p>
        </section>
        <form className="stack" onSubmit={search}>
          <label className="y-label" htmlFor="code">{t('traceability.codeLabel')}</label>
          <input id="code" className="bigfield inscription" value={text} onChange={(e) => setText(e.target.value)}
                 autoCapitalize="characters" autoComplete="off" spellCheck={false} inputMode="text" />
          {text !== '' && (
            <p className="hint" aria-live="polite">
              {code ? t('traceability.codeReadsAs', { code }) : t('traceability.codeInvalid')}
            </p>
          )}
          <button className="btn btn--primary" disabled={!code || state.kind === 'busy'}>{t('traceability.verifyButton')}</button>
        </form>
        {state.kind === 'failed' && <Problem outcome={state.outcome} />}
        {state.kind === 'results' && (state.items.length === 0
          ? <p className="note">{t('traceability.noMatch')}</p>
          : (
            <>
              <p className="hint">{t('traceability.sameCodeHint')}</p>
              <ul className="y-cards">
                {state.items.map(({ jewel, jewelerName }) => (
                  <li key={jewel.id} className="y-card">
                    <Link className="card-link" to={`/j/${jewel.jewelerId}/${jewel.id}`}>
                      <span className="inscription">{jewel.inscription}</span>
                      <span>{jewelerName || t('traceability.jewelerN', { id: jewel.jewelerId })}</span>
                    </Link>
                  </li>
                ))}
              </ul>
            </>
          ))}
      </main>
    </>
  );
}
