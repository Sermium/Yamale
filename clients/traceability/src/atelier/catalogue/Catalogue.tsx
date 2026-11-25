/**
 * The catalogue: designs read from the jeweler's own website, drafted by
 * Claude, confirmed by staff. Nothing here is on the register; pieces of a
 * confirmed design are registered from "Register pieces from a CSV".
 */

import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { t } from '@yamale/chain';

import { UnlockGate } from '../../account/Account.tsx';
import { useSession } from '../../account/session.tsx';
import { confirmDesign, fetchDesignImage, readCatalogue, startImport, type Catalogue as CatalogueData, type Design } from '../../api.ts';
import { CHAIN_ID } from '../../config.ts';
import { AppBar, Loading, Section } from '../../ui.tsx';
import { confirmAll, filterDesigns, importErrorKey, missingForConfirm, readyToConfirm, shopOrigin, type BulkResult, type Filter } from './catalogue.ts';
import { FailureNote } from '../shared/FailureNote.tsx';
import { paced } from '../register/register.ts';
import { useRole } from '../shared/role.ts';
import { uploadPhoto } from '../shared/uploads.ts';

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));
/** The gate's 429 and the API's 503 say "later", not "no". */
const busy = (r: { ok: boolean; status?: number }) => !r.ok && (r.status === 429 || r.status === 503);

const STATUS_KEY: Record<string, string> = {
  imported: 'traceability.statusImported', drafted: 'traceability.statusDrafted', changed: 'traceability.statusChanged',
  confirmed: 'traceability.statusConfirmed', gone: 'traceability.statusGone',
};

export function Catalogue() {
  const session = useSession();
  const [role] = useRole(session.address);
  const bar = <AppBar title={t('traceability.catalogueTitle')} eyebrow="" back />;
  if (role.state === 'loading') return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (role.state !== 'role' || !role.active) return <>{bar}<main className="y-screen"><p className="note">{t('traceability.staffOnly')}</p></main></>;
  return (
    <>
      <AppBar title={t('traceability.catalogueTitle')} eyebrow={role.jeweler.name} back />
      <main className="y-screen"><UnlockGate><CatalogueBody jewelerId={role.jeweler.id} /></UnlockGate></main>
    </>
  );
}

function CatalogueBody({ jewelerId }: { jewelerId: string }) {
  const session = useSession();
  const [cat, setCat] = useState<CatalogueData | null>(null);
  const [failed, setFailed] = useState<{ text: string; raw: string } | null>(null);
  const [address, setAddress] = useState('');
  const [filter, setFilter] = useState<Filter>('review');
  const [tick, setTick] = useState(0);
  const running = cat?.job.state === 'reading' || cat?.job.state === 'drafting';
  const [bulkTick, setBulkTick] = useState(false);
  const [bulk, setBulk] = useState<{ done: number; of: number; title: string } | null>(null);
  const [bulkResult, setBulkResult] = useState<BulkResult | null>(null);
  const stop = useRef(false);

  useEffect(() => {
    let live = true;
    session.withKey((k) => readCatalogue(k, jewelerId, CHAIN_ID)).then((r) => {
      if (!live) return;
      if (r.ok) { setCat(r.value); setAddress((a) => a || r.value.origin); return; }
      setFailed({ text: t('traceability.catalogueUnavailable'), raw: `${r.status} ${r.error}` });
    });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jewelerId, tick]);

  useEffect(() => {
    if (!running) return;
    const id = setInterval(() => setTick((n) => n + 1), 3000);
    return () => clearInterval(id);
  }, [running]);

  async function read() {
    setFailed(null);
    const origin = shopOrigin(address, CHAIN_ID);
    if (!origin) { setFailed({ text: t('traceability.importErr.refused'), raw: '' }); return; }
    const r = await session.withKey((k) => startImport(k, origin, CHAIN_ID));
    if (!r.ok) {
      const key = importErrorKey(r.code);
      setFailed({ text: key ? t(key) : t('traceability.catalogueUnavailable'), raw: `${r.status} ${r.error}` });
      return;
    }
    setTick((n) => n + 1);
  }

  async function confirmReady(ready: Design[]) {
    setFailed(null); setBulkResult(null);
    stop.current = false;
    const res = await confirmAll(ready, {
      async image(d, n) {
        const r = await paced(() => session.withKey((k) => fetchDesignImage(k, jewelerId, d.id, n, CHAIN_ID)), busy, sleep);
        return r.ok ? r.value : null;
      },
      async upload(photo) {
        const up = await paced(() => uploadPhoto(photo, session.withKey, 'photo'), (u) => !u.ok && /^(429|503)\b/.test(u.raw), sleep);
        return up.ok ? up : { ok: false, raw: up.raw || up.text };
      },
      async confirm(d, photos) {
        const r = await paced(() => session.withKey((k) => confirmDesign(k, d, { title: d.title, fields: d.fields, photos }, CHAIN_ID)), busy, sleep);
        return r.ok ? { ok: true } : { ok: false, raw: `${r.status} ${r.error}` };
      },
      progress: (done, of, current) => setBulk(current ? { done: done + 1, of, title: current.title || current.key } : null),
      stopped: () => stop.current,
    });
    setBulk(null); setBulkTick(false); setBulkResult(res);
    setTick((n) => n + 1);
  }

  if (!cat && !failed) return <Loading />;
  const designs = cat ? filterDesigns(cat.designs, filter) : [];
  const job = cat?.job;
  const ready = cat ? readyToConfirm(cat.designs) : [];
  const needsPerson = cat ? filterDesigns(cat.designs, 'review').length - ready.length : 0;
  const titleOf = (id: string) => { const d = cat?.designs.find((x) => x.id === id); return d ? d.title || d.key : id; };
  return (
    <>
      <p className="hint">{t('traceability.catalogueIntro')}</p>
      <Section title={t('traceability.shopAddress')}>
        <div className="stack">
          <input className="bigfield" value={address} onChange={(e) => setAddress(e.target.value)} inputMode="url"
                 autoCapitalize="off" spellCheck={false} aria-label={t('traceability.shopAddress')} placeholder="https://www.yourshop.com" />
          <p className="hint">{t('traceability.shopAddressHint')}</p>
          <button type="button" className="btn btn--primary" disabled={running || address.trim() === ''} onClick={read}>
            {cat?.origin ? t('traceability.refreshWebsite') : t('traceability.readWebsite')}
          </button>
          {job?.state === 'reading' && <p className="loading" role="status">{t('traceability.jobReading')}</p>}
          {job?.state === 'drafting' && <p className="loading" role="status">{t('traceability.jobDrafting', { done: job.done, of: job.of })}</p>}
          {job?.state === 'failed' && <FailureNote text={t(importErrorKey(job.error) ?? 'traceability.importErr.unreachable')} raw={job.error} />}
          {failed && <FailureNote text={failed.text} raw={failed.raw} />}
        </div>
      </Section>
      {cat && cat.designs.length > 0 && (
        <>
          <div className="actions" role="tablist">
            {(['review', 'confirmed', 'gone'] as const).map((f) => (
              <button key={f} type="button" role="tab" aria-selected={filter === f} className={`btn${filter === f ? ' btn--primary' : ''}`}
                      onClick={() => setFilter(f)}>
                {t(`traceability.filter.${f}`)} <span className="num">({filterDesigns(cat.designs, f).length})</span>
              </button>
            ))}
            <Link className="btn" to="/atelier/catalogue/pieces">{t('traceability.piecesLink')}</Link>
          </div>
          {filter === 'review' && (ready.length > 0 || bulk || bulkResult) && (
            <div className="stack">
              {ready.length > 0 && !bulk && (
                <label className="tick">
                  <input type="checkbox" checked={bulkTick} onChange={(e) => setBulkTick(e.target.checked)} />
                  {t('traceability.confirmAllTick', { n: ready.length })}
                </label>
              )}
              <div className="actions">
                {ready.length > 0 && (
                  <button type="button" className="btn btn--primary" disabled={!bulkTick || bulk !== null || running} onClick={() => confirmReady(ready)}>
                    {t('traceability.confirmAll', { n: ready.length })}
                  </button>
                )}
                {bulk && <button type="button" className="btn" onClick={() => { stop.current = true; }}>{t('traceability.confirmAllStop')}</button>}
              </div>
              {bulk && <p className="loading" role="status">{t('traceability.confirmAllProgress', bulk)}</p>}
              {bulkResult && bulkResult.confirmed.length > 0 && (
                <p className="banner banner--ok" role="status">
                  {t('traceability.confirmAllDone', { n: bulkResult.confirmed.length })} <Link to="/atelier/catalogue/pieces">{t('traceability.piecesLink')}</Link>
                </p>
              )}
              {bulkResult && bulkResult.failed.length > 0 && (
                <FailureNote text={t('traceability.confirmAllFailed', { n: bulkResult.failed.length })}
                             raw={bulkResult.failed.map((f) => `${titleOf(f.id)}: ${f.raw}`).join('; ')} />
              )}
              {needsPerson > 0 && <p className="hint">{t('traceability.confirmAllHint', { n: needsPerson })}</p>}
            </div>
          )}
          <ul className="y-cards">
            {designs.map((d) => (
              <li key={d.id} className="y-card">
                <Link to={`/atelier/catalogue/design/${d.id}`} className="design-row">
                  <span className="design-row__title">{d.title || d.key}</span>
                  <span className="hint">{d.key}</span>
                  {filter === 'review' && missingForConfirm(d).length > 0 && (
                    <span className="hint hint--bad">{missingForConfirm(d).map((m) => t(`traceability.missing.${m}`)).join(' · ')}</span>
                  )}
                  <span className={`y-chip${d.status === 'confirmed' ? ' y-chip--ok' : d.status === 'changed' ? ' y-chip--bad' : ''}`}>{t(STATUS_KEY[d.status]!)}</span>
                </Link>
              </li>
            ))}
          </ul>
        </>
      )}
      {cat && cat.designs.length === 0 && !running && <p className="hint">{t('traceability.noDesigns')}</p>}
    </>
  );
}
