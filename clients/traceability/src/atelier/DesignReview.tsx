/**
 * Checking one design: the website's own words beside the editable fields,
 * the photos, and Confirm. Confirming stores the chosen photos the way every
 * photo is stored (re-encoded on this device, EXIF dropped), then signs the
 * fields, the photos and the website text that was checked.
 */

import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { confirmDesign, fetchDesignImage, readCatalogue, saveDesign, type Design, type DesignFields } from '../api.ts';
import { CHAIN_ID } from '../config.ts';
import { AppBar, Loading, Section } from '../ui.tsx';
import { canConfirmDesign } from './catalogue.ts';
import { designComplete } from './draft.ts';
import { DesignForm } from './DesignForm.tsx';
import { FailureNote } from './FailureNote.tsx';
import { useRole } from './role.ts';
import { uploadPhoto } from './uploads.ts';

type Img = { n: number; state: 'loading' } | { n: number; state: 'ok'; blob: Blob; url: string } | { n: number; state: 'failed'; why: string };

export function DesignReview() {
  const session = useSession();
  const [role] = useRole(session.address);
  const bar = <AppBar title={t('traceability.designTitle')} eyebrow="" back />;
  if (role.state === 'loading') return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (role.state !== 'role' || !role.active) return <>{bar}<main className="y-screen"><p className="note">{t('traceability.staffOnly')}</p></main></>;
  return (
    <>
      <AppBar title={t('traceability.designTitle')} eyebrow={role.jeweler.name} back />
      <main className="y-screen"><UnlockGate><Review jewelerId={role.jeweler.id} /></UnlockGate></main>
    </>
  );
}

function Review({ jewelerId }: { jewelerId: string }) {
  const { id = '' } = useParams();
  const session = useSession();
  const navigate = useNavigate();
  const [design, setDesign] = useState<Design | null>(null);
  const [title, setTitle] = useState('');
  const [fields, setFields] = useState<DesignFields>({ metal: '', weightGrams: '', stones: [] });
  const [images, setImages] = useState<Img[]>([]);
  const [chosen, setChosen] = useState<Set<number>>(new Set());
  const [rights, setRights] = useState(false);
  const [stage, setStage] = useState<string | null>(null);
  const [failed, setFailed] = useState<{ text: string; raw: string } | null>(null);
  const [note, setNote] = useState<string | null>(null);
  // Anything changed on this page since the design was loaded, saved or confirmed.
  const [edited, setEdited] = useState(false);
  const editTitle = (v: string) => { setTitle(v); setEdited(true); };
  const editFields = (v: DesignFields) => { setFields(v); setEdited(true); };
  const editChosen = (f: (s: Set<number>) => Set<number>) => { setChosen(f); setEdited(true); };

  useEffect(() => {
    let live = true;
    session.withKey((k) => readCatalogue(k, jewelerId, CHAIN_ID)).then((r) => {
      if (!live) return;
      if (!r.ok) { setFailed({ text: t('traceability.catalogueUnavailable'), raw: `${r.status} ${r.error}` }); return; }
      const d = r.value.designs.find((x) => x.id === id) ?? null;
      if (!d) { navigate('/atelier/catalogue', { replace: true }); return; }
      setDesign(d);
      setTitle(d.title);
      setFields(d.fields);
      setChosen(new Set(d.imageUrls.map((_, n) => n)));
      setImages(d.imageUrls.map((_, n) => ({ n, state: 'loading' })));
    });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jewelerId, id]);

  // Images one at a time: the website paces us, and so do we.
  useEffect(() => {
    if (!design) return;
    let live = true;
    const urls: string[] = [];
    (async () => {
      for (let n = 0; n < design.imageUrls.length && live; n++) {
        const r = await session.withKey((k) => fetchDesignImage(k, jewelerId, design.id, n, CHAIN_ID));
        if (!live) return;
        const next: Img = r.ok ? { n, state: 'ok', blob: r.value, url: URL.createObjectURL(r.value) }
          : { n, state: 'failed', why: r.error || String(r.status) };
        if (next.state === 'ok') urls.push(next.url);
        setImages((all) => all.map((im) => (im.n === n ? next : im)));
      }
    })();
    return () => { live = false; urls.forEach((u) => URL.revokeObjectURL(u)); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [design?.id]);

  if (!design) return failed ? <FailureNote text={failed.text} raw={failed.raw} /> : <Loading />;
  const gone = design.status === 'gone';
  const settled = design.status === 'confirmed' && !edited;
  const usable = images.filter((im): im is Extract<Img, { state: 'ok' }> => im.state === 'ok' && chosen.has(im.n));
  const canConfirm = canConfirmDesign({
    gone, rights, settled, complete: designComplete(title, fields), busy: stage !== null,
    images: images.map((im) => ({ state: im.state, chosen: chosen.has(im.n) })),
  });

  async function save() {
    setFailed(null); setNote(null);
    const r = await session.withKey((k) => saveDesign(k, design!, title, fields, CHAIN_ID));
    if (!r.ok) { setFailed({ text: r.code === 'gone' ? t('traceability.designGone') : t('traceability.catalogueUnavailable'), raw: `${r.status} ${r.error}` }); return; }
    setDesign(r.value);
    setEdited(false);
    setNote(t('traceability.saved'));
  }

  async function confirm() {
    setFailed(null); setNote(null);
    try {
      const photos: string[] = [];
      for (let i = 0; i < usable.length; i++) {
        setStage(t('traceability.stepImage', { n: i + 1, of: usable.length }));
        const up = await uploadPhoto(usable[i]!.blob, session.withKey, `photo ${usable[i]!.n + 1}`);
        if (!up.ok) { setFailed({ text: up.text, raw: up.raw }); return; }
        photos.push(up.hash);
      }
      setStage(t('traceability.sending'));
      const r = await session.withKey((k) => confirmDesign(k, design!, { title, fields, photos }, CHAIN_ID));
      if (!r.ok) {
        const text = r.code === 'stale' ? t('traceability.designStale') : r.code === 'gone' ? t('traceability.designGone') : t('traceability.catalogueUnavailable');
        setFailed({ text, raw: `${r.status} ${r.error}` });
        return;
      }
      setDesign(r.value);
      setEdited(false);
      window.scrollTo({ top: 0, behavior: 'smooth' });
    } catch (e) {
      setFailed({ text: t('traceability.interrupted'), raw: e instanceof Error ? e.message : String(e) });
    } finally {
      setStage(null);
    }
  }

  return (
    <>
      {design.status === 'confirmed' && (
        <p className="banner banner--ok" role="status">
          {t('traceability.confirmed')} <Link to="/atelier/catalogue/pieces">{t('traceability.piecesLink')}</Link>
        </p>
      )}
      {design.status === 'changed' && <p className="banner banner--warn">{t('traceability.designChanged')}</p>}
      {gone && <p className="banner banner--warn">{t('traceability.designGone')}</p>}
      {design.draftNote === 'unavailable' && <p className="note">{t('traceability.draftUnavailable')}</p>}
      {design.draftNote === 'paused' && <p className="note">{t('traceability.draftPaused')}</p>}
      <Section title={t('traceability.designFromSite')}>
        <dl className="facts">
          <dt>{t('traceability.designKey')}</dt><dd className="num">{design.key}</dd>
          {design.listPrice && (<><dt>{t('traceability.listPrice')}</dt><dd className="num">{design.listPrice.amount} {design.listPrice.currency}</dd></>)}
        </dl>
        <p className="story">{design.description}</p>
        {design.productUrl && <a href={design.productUrl} target="_blank" rel="noopener noreferrer nofollow">{t('traceability.viewOnSite')}</a>}
      </Section>
      <Section title={t('traceability.details')}>
        {design.status === 'changed' && design.draft && (
          <button type="button" className="btn" onClick={() => editFields(design.draft!)}>{t('traceability.useNewDraft')}</button>
        )}
        <DesignForm title={title} fields={fields} onTitle={editTitle} onFields={editFields} />
      </Section>
      <Section title={t('traceability.designPhotos')}>
        <ul className="photo-picks">
          {images.map((im) => (
            <li key={im.n}>
              {im.state === 'loading' && <Loading />}
              {im.state === 'failed' && (
                <label>
                  <input type="checkbox" checked={chosen.has(im.n)} onChange={(e) => editChosen((s) => {
                    const next = new Set(s); if (e.target.checked) next.add(im.n); else next.delete(im.n); return next;
                  })} />
                  <p className="hint hint--bad">{t('traceability.imageFailed', { n: im.n + 1, why: im.why })}</p>
                </label>
              )}
              {im.state === 'ok' && (
                <label>
                  <input type="checkbox" checked={chosen.has(im.n)} onChange={(e) => editChosen((s) => {
                    const next = new Set(s); if (e.target.checked) next.add(im.n); else next.delete(im.n); return next;
                  })} />
                  <img src={im.url} alt="" />
                </label>
              )}
            </li>
          ))}
        </ul>
      </Section>
      <label className="tick">
        <input type="checkbox" checked={rights} onChange={(e) => setRights(e.target.checked)} /> {t('traceability.rightsTick')}
      </label>
      {stage && <p className="loading" role="status">{stage}</p>}
      {note && <p className="banner banner--ok" role="status">{note}</p>}
      {failed && <FailureNote text={failed.text} raw={failed.raw} />}
      {design.status === 'confirmed' && edited && <p className="hint">{t('traceability.saveUnconfirms')}</p>}
      <div className="actions">
        <button type="button" className="btn" disabled={gone || settled || stage !== null} onClick={save}>{t('traceability.saveDesign')}</button>
        <button type="button" className="btn btn--primary" disabled={!canConfirm} onClick={confirm}>{t('traceability.confirmDesign')}</button>
      </div>
    </>
  );
}
