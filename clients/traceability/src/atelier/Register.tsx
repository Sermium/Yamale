/**
 * Registering a jewel. When its serial starts with the code of a confirmed
 * design (JOA-071-0001 is a piece of JOA-071), the design supplies the photos,
 * the title, the materials and the description, exactly as the CSV import does,
 * and only what is particular to this piece is asked for. Otherwise: photos,
 * then the record, then the token.
 *
 * In that order, because the token names the record's fingerprint and the
 * record names the photos' fingerprints. If the last step fails, the files
 * stay in traceability-api unreferenced; that costs disk, not trust, and a retry
 * re-uses them because the same bytes have the same name.
 */

import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { findByInscription, normaliseInscription, registerJewel, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, eventValue, useAction } from '../actions/useAction.tsx';
import { readCatalogue, type Design } from '../api.ts';
import { CHAIN_ID, recordUri } from '../config.ts';
import { AppBar, Loading, Problem, Section } from '../ui.tsx';
import { VerifiedImage } from '../verify/Photos.tsx';
import { designForSerial, judgePieces, pieceRecord } from './pieces.ts';
import { encodeRecord } from '../verify/record.ts';
import { FailureNote } from './FailureNote.tsx';
import { draftComplete, EMPTY_DRAFT, RecordForm, type Draft } from './RecordForm.tsx';
import { useRole } from './role.ts';
import { uploadPhoto, uploadRecord } from './uploads.ts';

type Step = { kind: 'form' } | { kind: 'photos'; done: number; of: number } | { kind: 'record' } | { kind: 'chain' }
  | { kind: 'failed'; text: string; raw: string };

export function Register() {
  const session = useSession();
  const [role] = useRole(session.address);
  const navigate = useNavigate();
  const [code, setCode] = useState('');
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT);
  const [files, setFiles] = useState<File[]>([]);
  const [step, setStep] = useState<Step>({ kind: 'form' });
  const [action, run] = useAction();
  // Set before the first await, so a second tap cannot start a second sequence.
  const running = useRef(false);
  const normalised = normaliseInscription(code);
  // The confirmed designs, read once the key is available; null until then or when unreadable.
  const [designs, setDesigns] = useState<Design[] | null>(null);
  const [picked, setPicked] = useState('');           // '' = by the serial, '-' = none, else a design key
  const [piece, setPiece] = useState({ weight: '', cert: '', notes: '' });
  const jewelerId = role.state === 'role' ? role.jeweler.id : null;
  useEffect(() => {
    if (!jewelerId || !session.unlocked || designs) return;
    let live = true;
    session.withKey((k) => readCatalogue(k, jewelerId, CHAIN_ID)).then((r) => {
      if (live && r.ok) setDesigns(r.value.designs.filter((d) => d.status === 'confirmed' && d.confirmed));
    }).catch(() => undefined);
    return () => { live = false; };
  }, [jewelerId, session.unlocked, designs, session]);
  const confirmed = useMemo(() => [...(designs ?? [])].sort((a, b) => a.confirmed!.title.localeCompare(b.confirmed!.title)), [designs]);
  const design = picked === '-' ? null : picked ? confirmed.find((d) => d.key === picked) ?? null : designForSerial(code, confirmed);

  const bar = <AppBar title={t('traceability.registerTitle')} eyebrow="" back />;
  if (role.state === 'loading') return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (role.state === 'failed') return <>{bar}<main className="y-screen"><Problem outcome={role.outcome} /></main></>;
  if (role.state !== 'role' || !role.staff || !role.active) {
    return <>{bar}<main className="y-screen"><p className="note">{t('traceability.staffOnly')}</p></main></>;
  }
  const jeweler = role.jeweler;

  /** A piece of a confirmed design: its record is the design's, plus this piece's weight, certificate and notes. */
  async function submitFromDesign(d: Design) {
    if (running.current || !normalised) return;
    running.current = true;
    setStep({ kind: 'record' });
    try {
      const existing = await findByInscription(normalised);
      const clash = existing.ok ? existing.value.find((j) => j.jewelerId === jeweler.id) : undefined;
      if (clash) { setStep({ kind: 'failed', text: t('traceability.alreadyRegistered', { id: clash.id }), raw: '' }); return; }
      const [judged] = judgePieces([{ index: 0, cells: [], serial: code, sku: d.key, weight: piece.weight,
        labCertificate: piece.cert.trim(), notes: piece.notes.trim() }], [d], new Map(), jeweler);
      if (!judged || judged.verdict !== 'ready') {
        setStep({ kind: 'failed', text: t(`traceability.verdict.${judged?.verdict ?? 'unknownSku'}`), raw: '' });
        return;
      }
      const up = await uploadRecord(encodeRecord(pieceRecord(judged, jeweler, new Date())), session.withKey);
      if (!up.ok) { setStep({ kind: 'failed', text: up.text, raw: up.raw }); return; }
      setStep({ kind: 'chain' });
      const done = await run(() => session.submit(
        [registerJewel(session.address!, normalised, up.hash, recordUri(up.hash))], jeweler.business));
      if (!done) { setStep({ kind: 'form' }); return; }
      const id = eventValue(done, 'blockchain.jewel.v1.EventJewelRegistered', 'jewel_id');
      navigate(id ? `/atelier/jewel/${id}` : '/atelier', { replace: true });
    } catch (e) {
      setStep({ kind: 'failed', text: t('traceability.interrupted'), raw: e instanceof Error ? e.message : String(e) });
    } finally {
      running.current = false;
    }
  }

  async function submit() {
    if (running.current || !normalised || !draftComplete(draft) || files.length === 0) return;
    running.current = true;
    setStep({ kind: 'photos', done: 0, of: files.length });
    try {
      const existing = await findByInscription(normalised);
      const clash = existing.ok ? existing.value.find((j) => j.jewelerId === jeweler.id) : undefined;
      if (clash) { setStep({ kind: 'failed', text: t('traceability.alreadyRegistered', { id: clash.id }), raw: '' }); return; }

      const photos: string[] = [];
      for (let i = 0; i < files.length; i++) {
        setStep({ kind: 'photos', done: i, of: files.length });
        const up = await uploadPhoto(files[i]!, session.withKey);
        if (!up.ok) { setStep({ kind: 'failed', text: up.text, raw: up.raw }); return; }
        photos.push(up.hash);
      }
      setStep({ kind: 'record' });
      const record = encodeRecord({ version: 1, jewelerId: jeweler.id, inscription: normalised, ...draft, photos,
        createdAt: new Date().toISOString() });
      const up = await uploadRecord(record, session.withKey);
      if (!up.ok) { setStep({ kind: 'failed', text: up.text, raw: up.raw }); return; }

      setStep({ kind: 'chain' });
      const done = await run(() => session.submit(
        [registerJewel(session.address!, normalised, up.hash, recordUri(up.hash))], jeweler.business));
      if (!done) { setStep({ kind: 'form' }); return; }
      const id = eventValue(done, 'blockchain.jewel.v1.EventJewelRegistered', 'jewel_id');
      navigate(id ? `/atelier/jewel/${id}` : '/atelier', { replace: true });
    } catch (e) {
      // Typically the account locked itself during a long upload.
      setStep({ kind: 'failed', text: t('traceability.interrupted'), raw: e instanceof Error ? e.message : String(e) });
    } finally {
      running.current = false;
    }
  }

  const busy = step.kind === 'photos' || step.kind === 'record' || step.kind === 'chain';
  return (
    <>
      <AppBar title={t('traceability.registerTitle')} eyebrow={jeweler.name} back />
      <main className="y-screen">
        <UnlockGate>
          <Section title={t('traceability.codeLabel')}>
            <input className="bigfield inscription" value={code} onChange={(e) => setCode(e.target.value)}
                   autoCapitalize="characters" autoComplete="off" spellCheck={false} aria-label={t('traceability.codeLabel')} />
            {code !== '' && <p className="hint">{normalised ? t('traceability.codeReadsAs', { code: normalised }) : t('traceability.codeInvalid')}</p>}
            <p className="hint">{t('traceability.codeMustMatch')}</p>
          </Section>
          {confirmed.length > 0 && (
            <Section title={t('traceability.registerDesign')}>
              <select className="bigfield" value={picked} onChange={(e) => setPicked(e.target.value)} aria-label={t('traceability.registerDesign')}>
                <option value="">{design && !picked ? t('traceability.registerDesignAuto', { title: design.confirmed!.title }) : t('traceability.registerDesignBySerial')}</option>
                {confirmed.map((d) => <option key={d.key} value={d.key}>{`${d.confirmed!.title} · ${d.key}`}</option>)}
                <option value="-">{t('traceability.registerDesignNone')}</option>
              </select>
              {design && (
                <div className="jewelcard__link">
                  {design.confirmed!.photos[0]
                    ? <VerifiedImage hash={design.confirmed!.photos[0]} alt="" className="jewelcard__img" />
                    : <span className="jewelcard__img photo--absent" />}
                  <span className="jewelcard__text">
                    <span className="jewelcard__title">{design.confirmed!.title}</span>
                    <span className="hint">{[design.confirmed!.fields.metal, design.confirmed!.fields.weightGrams ? `${design.confirmed!.fields.weightGrams} g` : '',
                      ...new Set(design.confirmed!.fields.stones.map((s) => s.kind))].filter(Boolean).join(' · ')}</span>
                    <span className="hint">{t('traceability.registerDesignPhotos', { n: design.confirmed!.photos.length })}</span>
                  </span>
                </div>
              )}
            </Section>
          )}
          {design ? (
            <Section title={t('traceability.registerThisPiece')}>
              <label className="y-label">{t('traceability.registerWeight')}
                <input className="bigfield" inputMode="decimal" value={piece.weight} placeholder={design.confirmed!.fields.weightGrams}
                       onChange={(e) => setPiece({ ...piece, weight: e.target.value })} />
              </label>
              {!design.confirmed!.fields.weightGrams && <p className="hint">{t('traceability.registerWeightNeeded')}</p>}
              <label className="y-label">{t('traceability.registerCertificate')}
                <input className="bigfield" maxLength={200} value={piece.cert} onChange={(e) => setPiece({ ...piece, cert: e.target.value })} />
              </label>
              <label className="y-label">{t('traceability.registerNotes')}
                <textarea className="bigfield" rows={2} maxLength={500} value={piece.notes} onChange={(e) => setPiece({ ...piece, notes: e.target.value })} />
              </label>
              {step.kind === 'record' && <p className="loading">{t('traceability.stepRecord')}</p>}
              {step.kind === 'failed' && <FailureNote text={step.text} raw={step.raw} />}
              <ActionStatus state={action} />
              <button type="button" className="btn btn--primary"
                      disabled={busy || !normalised || (!design.confirmed!.fields.weightGrams && !piece.weight.trim())}
                      onClick={() => void submitFromDesign(design)}>{t('traceability.registerButton')}</button>
            </Section>
          ) : (<>
          <Section title={t('traceability.photos')}>
            <input type="file" accept="image/*" capture="environment" multiple aria-label={t('traceability.photos')}
                   onChange={(e) => { setFiles([...files, ...Array.from(e.target.files ?? [])].slice(0, 12)); e.target.value = ''; }} />
            <p className="hint">{t('traceability.photosHint', { n: files.length })}</p>
            {files.length > 0 && <button type="button" className="btn" onClick={() => setFiles([])}>{t('traceability.clearPhotos')}</button>}
          </Section>
          <Section title={t('traceability.details')}><RecordForm value={draft} onChange={setDraft} /></Section>
          {step.kind === 'photos' && <p className="loading">{t('traceability.stepPhotos', { n: step.done + 1, of: step.of })}</p>}
          {step.kind === 'record' && <p className="loading">{t('traceability.stepRecord')}</p>}
          {step.kind === 'failed' && <FailureNote text={step.text} raw={step.raw} />}
          <ActionStatus state={action} />
          <button type="button" className="btn btn--primary" disabled={busy || !normalised || !draftComplete(draft) || files.length === 0}
                  onClick={submit}>{t('traceability.registerButton')}</button>
          </>)}
        </UnlockGate>
      </main>
    </>
  );
}
