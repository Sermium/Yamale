/**
 * Registering a jewel: photos, then the record, then the token.
 *
 * In that order, because the token names the record's fingerprint and the
 * record names the photos' fingerprints. If the last step fails, the files
 * stay in loyalty-api unreferenced; that costs disk, not trust, and a retry
 * re-uses them because the same bytes have the same name.
 */

import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { findByInscription, normaliseInscription, registerJewel, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, eventValue, useAction } from '../actions/useAction.tsx';
import { recordUri } from '../config.ts';
import { AppBar, Loading, Problem, Section } from '../ui.tsx';
import { encodeRecord } from '../verify/record.ts';
import { draftComplete, EMPTY_DRAFT, RecordForm, type Draft } from './RecordForm.tsx';
import { useRole } from './role.ts';
import { uploadPhoto, uploadRecord } from './uploads.ts';

type Step = { kind: 'form' } | { kind: 'photos'; done: number; of: number } | { kind: 'record' } | { kind: 'chain' }
  | { kind: 'failed'; text: string };

export function Register() {
  const session = useSession();
  const [role] = useRole(session.address);
  const navigate = useNavigate();
  const [code, setCode] = useState('');
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT);
  const [files, setFiles] = useState<File[]>([]);
  const [step, setStep] = useState<Step>({ kind: 'form' });
  const [action, run] = useAction();
  const normalised = normaliseInscription(code);

  const bar = <AppBar title={t('loyalty.registerTitle')} eyebrow="" back />;
  if (role.state === 'loading') return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (role.state === 'failed') return <>{bar}<main className="y-screen"><Problem outcome={role.outcome} /></main></>;
  if (role.state !== 'role' || !role.staff || !role.active) {
    return <>{bar}<main className="y-screen"><p className="note">{t('loyalty.staffOnly')}</p></main></>;
  }
  const jeweler = role.jeweler;

  async function submit() {
    if (!normalised || !draftComplete(draft) || files.length === 0) return;
    const existing = await findByInscription(normalised);
    const clash = existing.ok ? existing.value.find((j) => j.jewelerId === jeweler.id) : undefined;
    if (clash) { setStep({ kind: 'failed', text: t('loyalty.alreadyRegistered', { id: clash.id }) }); return; }

    const photos: string[] = [];
    for (let i = 0; i < files.length; i++) {
      setStep({ kind: 'photos', done: i, of: files.length });
      const up = await uploadPhoto(files[i]!, session.withKey);
      if (!up.ok) { setStep({ kind: 'failed', text: up.text }); return; }
      photos.push(up.hash);
    }
    setStep({ kind: 'record' });
    const record = encodeRecord({ version: 1, jewelerId: jeweler.id, inscription: normalised, ...draft, photos,
      createdAt: new Date().toISOString() });
    const up = await uploadRecord(record, session.withKey);
    if (!up.ok) { setStep({ kind: 'failed', text: up.text }); return; }

    setStep({ kind: 'chain' });
    const done = await run(() => session.submit(
      [registerJewel(session.address!, normalised, up.hash, recordUri(up.hash))], jeweler.business));
    if (!done) { setStep({ kind: 'form' }); return; }
    const id = eventValue(done, 'blockchain.jewel.v1.EventJewelRegistered', 'jewel_id');
    navigate(id ? `/atelier/jewel/${id}` : '/atelier', { replace: true });
  }

  const busy = step.kind === 'photos' || step.kind === 'record' || step.kind === 'chain';
  return (
    <>
      <AppBar title={t('loyalty.registerTitle')} eyebrow={jeweler.name} back />
      <main className="y-screen">
        <UnlockGate>
          <Section title={t('loyalty.codeLabel')}>
            <input className="bigfield inscription" value={code} onChange={(e) => setCode(e.target.value)}
                   autoCapitalize="characters" autoComplete="off" spellCheck={false} aria-label={t('loyalty.codeLabel')} />
            {code !== '' && <p className="hint">{normalised ? t('loyalty.codeReadsAs', { code: normalised }) : t('loyalty.codeInvalid')}</p>}
            <p className="hint">{t('loyalty.codeMustMatch')}</p>
          </Section>
          <Section title={t('loyalty.photos')}>
            <input type="file" accept="image/*" capture="environment" multiple aria-label={t('loyalty.photos')}
                   onChange={(e) => { setFiles([...files, ...Array.from(e.target.files ?? [])].slice(0, 12)); e.target.value = ''; }} />
            <p className="hint">{t('loyalty.photosHint', { n: files.length })}</p>
            {files.length > 0 && <button type="button" className="btn" onClick={() => setFiles([])}>{t('loyalty.clearPhotos')}</button>}
          </Section>
          <Section title={t('loyalty.details')}><RecordForm value={draft} onChange={setDraft} /></Section>
          {step.kind === 'photos' && <p className="loading">{t('loyalty.stepPhotos', { n: step.done + 1, of: step.of })}</p>}
          {step.kind === 'record' && <p className="loading">{t('loyalty.stepRecord')}</p>}
          {step.kind === 'failed' && <p className="note note--warn" role="alert">{step.text}</p>}
          <ActionStatus state={action} />
          <button type="button" className="btn btn--primary" disabled={busy || !normalised || !draftComplete(draft) || files.length === 0}
                  onClick={submit}>{t('loyalty.registerButton')}</button>
        </UnlockGate>
      </main>
    </>
  );
}
