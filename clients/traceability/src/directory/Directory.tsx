/**
 * /jewelers: the shops registered on Yamale, in good standing, with how to
 * find them. A shop's look and contact details are its own profile; the name
 * and the standing are the chain's.
 */

import { useMemo, useState, type CSSProperties } from 'react';
import { Link } from 'react-router-dom';
import { isActive, readJewelers, t, useLocale } from '@yamale/chain';

import { fetchProfile } from '../api.ts';
import { mapLimit } from '../owner/holdings.ts';
import { AppBar, Loading, Problem, useLoad } from '../ui.tsx';
import { VerifiedImage } from '../verify/Photos.tsx';
import { countryNamer, matches, sortListed, type Listed } from './directory.ts';

async function loadDirectory() {
  const js = await readJewelers();
  if (!js.ok) return js;
  const active = js.value.filter(isActive);
  const listed = await mapLimit(active, 6, async (j): Promise<Listed> => {
    const p = await fetchProfile(j.id);
    return { id: j.id, name: j.name, contact: p?.contact, logo: p?.logo || undefined, colour: p?.colour || undefined };
  });
  return { ok: true as const, height: js.height, value: sortListed(listed) };
}

export function Directory() {
  const locale = useLocale();
  const names = useMemo(() => countryNamer(locale), [locale]);
  const [list] = useLoad(loadDirectory, []);
  const [query, setQuery] = useState('');
  const shown = list?.ok ? list.value.filter((s) => matches(s, query, names)) : [];

  return (
    <>
      <AppBar title={t('traceability.directoryTitle')} eyebrow={t('traceability.title')} />
      <main className="y-screen">
        <p className="lede">{t('traceability.directoryLede')}</p>
        <input className="bigfield" type="search" value={query} onChange={(e) => setQuery(e.target.value)}
               placeholder={t('traceability.directorySearch')} aria-label={t('traceability.directorySearch')} />
        {!list && <Loading />}
        {list && !list.ok && <Problem outcome={list} />}
        {list?.ok && list.value.length === 0 && <p className="hint">{t('traceability.directoryEmpty')}</p>}
        {list?.ok && list.value.length > 0 && shown.length === 0 && <p className="hint">{t('traceability.directoryNoMatch')}</p>}
        {shown.length > 0 && (
          <ul className="shoplist">
            {shown.map((s) => (
              <li key={s.id}>
                <Link className="shopcard" to={`/jewelers/${s.id}`}
                      style={(s.colour ? { '--jeweler-colour': s.colour } : {}) as CSSProperties}>
                  {s.logo
                    ? <VerifiedImage hash={s.logo} alt="" className="shopcard__logo" />
                    : <span className="shopcard__logo shopcard__logo--none" aria-hidden="true">{s.name.slice(0, 1)}</span>}
                  <span className="shopcard__text">
                    <span className="shopcard__name">{s.name}</span>
                    {(s.contact?.city || s.contact?.country) && (
                      <span className="hint">
                        {[s.contact?.city, s.contact?.country ? names(s.contact.country) : ''].filter(Boolean).join(', ')}
                      </span>
                    )}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
        <Link className="btn" to="/apply">{t('traceability.directoryJoin')}</Link>
      </main>
    </>
  );
}
