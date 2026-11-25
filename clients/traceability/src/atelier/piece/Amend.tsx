/**
 * A correction appends; it never overwrites. The jewel's page then shows the
 * corrected record first and keeps the original one tap away.
 */

import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { amendJewel, readJewel, t } from '@yamale/chain';

import { UnlockGate } from '../../account/Account.tsx';
import { useSession } from '../../account/session.tsx';
import { ActionStatus, useAction } from '../../actions/useAction.tsx';
import { recordUri } from '../../config.ts';
import { AppBar, isId, Loading, NotFound, Problem, Section, useLoad } from '../../ui.tsx';
import { encodeRecord, useRecord } from '../../verify/record.ts';
import { FailureNote } from '../shared/FailureNote.tsx';
import { draftComplete, RecordForm, type Draft } from '../register/RecordForm.tsx';
import { useRole } from '../shared/role.ts';
import { uploadPhoto, uploadRecord } from '../shared/uploads.ts';

const MAX_NOTE = 500;
const MAX_AMENDMENTS = 50;
const NO_SUCH = { ok: false as const, reason: 'not-found' as const, detail: '' };

export function Amend() {
  const { jewel: id = '' } = useParams();
  const valid = isId(id);
  const session = useSession();
  const [role] = useRole(session.address);
  const navigate = useNavigate();
  const [view] = useLoad(() => (valid ? readJewel(id) : Promise.resolve(NO_SUCH)), [id, valid]);
  const record = useRecord(view?.ok ? view.value.jewel : null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [photos, setPhotos] = useState<string[]>([]);
  const [extra, setExtra] = useState<File[]>([]);
  const [note, setNote] = useState('');
  const [failed, setFailed] = useState<{ text: string; raw: string } | null>(null);
  const [stage, setStage] = useState<{ kind: 'photos'; n: number; of: number } | { kind: 'record' } | null>(null);
  const [working, setWorking] = useState(false);
  const [action, run] = useAction();

  useEffect(() => {
    if (record.state === 'ok' && !draft) {
      const { title, metal, weightGrams, stones, origin } = record.record;
      setDraft({ title, metal, weightGrams, stones, origin });
      setPhotos(record.record.photos);
    }
  }, [record, draft]);

  const bar = <AppBar title={t('traceability.correctRecord')} eyebrow="" back />;
  const capped = view?.ok && view.value.jewel.amendments.length >= MAX_AMENDMENTS;
  if (!valid) return <NotFound />;
  if (view && !view.ok) return <>{bar}<main className="y-screen"><Problem outcome={view} /></main></>;
  if (role.state === 'failed') return <>{bar}<main className="y-screen"><Problem outcome={role.outcome} /></main></>;
  if (view?.ok && role.state !== 'loading'
      && (role.state !== 'role' || view.value.jewel.jewelerId !== role.jeweler.id)) return <NotFound />;
  if (role.state === 'role' && (!role.staff || !role.active)) {
    return <>{bar}<main className="y-screen"><p className="note">{t('traceability.staffOnly')}</p></main></>;
  }
  if (capped) return <>{bar}<main className="y-screen"><p className="note">{t('traceability.amendCap')}</p></main></>;
  if (record.state === 'missing' || record.state === 'mismatch' || record.state === 'unreachable' || record.state === 'unreadable') {
    return <>{bar}<main className="y-screen"><p className="note note--warn" role="alert">{t('traceability.recordUnreadable')}</p></main></>;
  }
  if (!view?.ok || role.state !== 'role' || !draft || record.state !== 'ok') {
    return <>{bar}<main className="y-screen"><Loading /></main></>;
  }
  const noteBytes = new TextEncoder().encode(note).length;
  const jeweler = role.jeweler;
  const base = record.record;
  const room = Math.max(0, 12 - photos.length);

  async function submit() {
    setFailed(null);
    setWorking(true);
    try {
      const all = [...photos];
      for (let i = 0; i < extra.length; i++) {
        setStage({ kind: 'photos', n: i + 1, of: extra.length });
        const up = await uploadPhoto(extra[i]!, session.withKey);
        if (!up.ok) { setFailed({ text: up.text, raw: up.raw }); return; }
        all.push(up.hash);
      }
      setStage({ kind: 'record' });
      // A correction keeps the record's version, and a v2 record's import fields.
      const bytes = encodeRecord({ ...base, jewelerId: jeweler.id, ...draft!, photos: all.slice(0, 12),
        createdAt: new Date().toISOString() });
      const up = await uploadRecord(bytes, session.withKey);
      if (!up.ok) { setFailed({ text: up.text, raw: up.raw }); return; }
      if (await run(() => session.submit([amendJewel(session.address!, id, up.hash, recordUri(up.hash), note.trim())], jeweler.business))) {
        navigate(`/atelier/jewel/${id}`, { replace: true });
      }
    } catch (e) {
      setFailed({ text: t('traceability.interrupted'), raw: e instanceof Error ? e.message : String(e) });
    } finally {
      setStage(null);
      setWorking(false);
    }
  }

  return (
    <>
      <AppBar title={t('traceability.correctRecord')} eyebrow={jeweler.name} back />
      <main className="y-screen">
        <UnlockGate>
          <p className="hint">{t('traceability.correctExplain')}</p>
          <Section title={t('traceability.details')}><RecordForm value={draft} onChange={setDraft} unstated={base.version === 2} /></Section>
          <Section title={t('traceability.photos')}>
            <p className="hint">{t('traceability.photosKept', { n: photos.length })}</p>
            {room > 0 && (
              <input type="file" accept="image/*" multiple aria-label={t('traceability.addPhotos')}
                     onChange={(e) => setExtra(Array.from(e.target.files ?? []).slice(0, room))} />
            )}
          </Section>
          <Section title={t('traceability.correctionNote')}>
            <textarea className="bigfield" rows={3} value={note} onChange={(e) => setNote(e.target.value)} aria-label={t('traceability.correctionNote')} />
            <p className={`hint num${noteBytes > MAX_NOTE ? ' hint--bad' : ''}`}>{t('traceability.noteBytes', { n: noteBytes, max: MAX_NOTE })}</p>
          </Section>
          {working && action.kind !== 'busy' && stage?.kind === 'photos' && <p className="loading" role="status">{t('traceability.stepPhotos', { n: stage.n, of: stage.of })}</p>}
          {working && action.kind !== 'busy' && stage?.kind === 'record' && <p className="loading" role="status">{t('traceability.stepRecord')}</p>}
          {failed && <FailureNote text={failed.text} raw={failed.raw} />}
          <ActionStatus state={action} />
          <button type="button" className="btn btn--primary"
                  disabled={working || action.kind === 'busy' || note.trim() === '' || noteBytes > MAX_NOTE || !draftComplete(draft, base.version)}
                  onClick={submit}>{t('traceability.saveCorrection')}</button>
        </UnlockGate>
      </main>
    </>
  );
}
