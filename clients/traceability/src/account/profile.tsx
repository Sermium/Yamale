/**
 * The person behind the account: first and last name, country, date of birth.
 *
 * The name and date of birth are kept by the traceability API, written only by
 * this account's key, never on the chain. Other accounts of the app see the
 * name next to the identifier; the date of birth is shown to nobody else. The
 * country is the one x/alias records, and once it is recorded it is read from
 * the identifier rather than edited here: a correction is an authority's act.
 */

import { useEffect, useId, useState, type FormEvent } from 'react';
import { t } from '@yamale/chain';

import { lookupPerson, readMyPerson, savePerson } from '../api.ts';
import { CHAIN_ID } from '../config.ts';
import { claimIdentifier, CountrySelect, guessCountry } from './identifier.tsx';
import { forgetName, setNameReader, useAccountLabel } from './names.tsx';
import { useSession, type Session } from './session.tsx';

const IDENTIFIER = /\b([A-Z]{2})-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]\b/;

/** Lets labels read people's names with this account's key while it is unlocked. */
export function NamesBridge() {
  const session = useSession();
  useEffect(() => {
    if (!session.unlocked || !session.address) { setNameReader(null); return; }
    setNameReader((a) => session.withKey((k) => lookupPerson(k, a, CHAIN_ID))
      .then((p) => (p ? `${p.first_name} ${p.last_name}` : null)));
    return () => setNameReader(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.unlocked, session.address]);
  return null;
}

function countryName(code: string): string {
  const lang = (typeof document !== 'undefined' && document.documentElement.lang) || 'en';
  try { return new Intl.DisplayNames([lang, 'en'], { type: 'region' }).of(code) ?? code; } catch { return code; }
}

export function ProfileSection({ session }: { session: Session }) {
  const uid = useId();
  const me = session.address ?? '';
  const label = useAccountLabel(me);
  const identifier = `${label.name} ${label.detail ?? ''}`.match(IDENTIFIER);
  const [loaded, setLoaded] = useState<'loading' | 'ready' | 'failed'>('loading');
  const [first, setFirst] = useState('');
  const [last, setLast] = useState('');
  const [birth, setBirth] = useState('');
  const [country, setCountry] = useState(guessCountry);
  const [state, setState] = useState<'idle' | 'busy' | 'saved' | 'failed' | 'noIdentifier'>('idle');

  useEffect(() => {
    if (!me) return;
    let live = true;
    session.withKey((k) => readMyPerson(k, me, CHAIN_ID)).then((r) => {
      if (!live) return;
      if (!r.ok) { setLoaded('failed'); return; }
      if (r.value) { setFirst(r.value.first_name); setLast(r.value.last_name); setBirth(r.value.birth_date ?? ''); }
      setLoaded('ready');
    }).catch(() => { if (live) setLoaded('failed'); });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [me]);

  async function save(e: FormEvent) {
    e.preventDefault();
    if (!first.trim() || !last.trim()) return;
    setState('busy');
    const saved = await session.withKey((k) => savePerson(k, me, { first_name: first.trim(), last_name: last.trim(), birth_date: birth }, CHAIN_ID));
    if (!saved.ok) { setState('failed'); return; }
    forgetName(me);
    if (!identifier && country) {
      const claimed = await claimIdentifier(session, country);
      if (claimed !== 'done') { setState('noIdentifier'); return; }
    }
    setState('saved');
  }

  if (!me) return null;
  return (
    <section className="panel stack profileform" aria-labelledby={uid + '-title'}>
      <h2 id={uid + '-title'} className="panel__title panel__title--plain">{t('traceability.profileTitle')}</h2>
      {loaded === 'loading' ? <p className="loading" role="status">{t('traceability.loading')}</p> : (
        <form className="stack" onSubmit={save}>
          {loaded === 'failed' && <p className="note note--warn" role="alert">{t('traceability.profileUnread')}</p>}
          <div className="profileform__names">
            <div className="stack">
              <label className="y-label" htmlFor={uid + '-first'}>{t('traceability.firstName')}</label>
              <input id={uid + '-first'} className="bigfield" autoComplete="given-name" value={first} maxLength={60}
                     onChange={(e) => setFirst(e.target.value)} required />
            </div>
            <div className="stack">
              <label className="y-label" htmlFor={uid + '-last'}>{t('traceability.lastName')}</label>
              <input id={uid + '-last'} className="bigfield" autoComplete="family-name" value={last} maxLength={60}
                     onChange={(e) => setLast(e.target.value)} required />
            </div>
          </div>
          <label className="y-label" htmlFor={uid + '-country'}>{t('traceability.country')}</label>
          {identifier ? (
            <p className="profileform__fixed" id={uid + '-country'}>
              {countryName(identifier[1])}
              <span className="hint">{t('traceability.countryRecorded', { id: identifier[0] })}</span>
            </p>
          ) : (<>
            <CountrySelect id={uid + '-country'} value={country} onChange={setCountry} />
            <p className="hint">{t('traceability.identifierExplain')}</p>
          </>)}
          <label className="y-label" htmlFor={uid + '-birth'}>{t('traceability.birthDate')}</label>
          <input id={uid + '-birth'} className="bigfield" type="date" autoComplete="bday" value={birth}
                 max={new Date().toISOString().slice(0, 10)} min="1900-01-01" onChange={(e) => setBirth(e.target.value)} />
          <p className="hint">{t('traceability.profileVisibility')}</p>
          <button className="btn btn--primary" disabled={state === 'busy' || !first.trim() || !last.trim()}>
            {t('traceability.profileSave')}
          </button>
          {state === 'saved' && <p className="note" role="status">{t('traceability.profileSaved')}</p>}
          {state === 'failed' && <p className="note note--warn" role="alert">{t('traceability.profileFailed')}</p>}
          {state === 'noIdentifier' && <p className="note note--warn" role="alert">{t('traceability.identifierUnavailable')}</p>}
        </form>
      )}
    </section>
  );
}
