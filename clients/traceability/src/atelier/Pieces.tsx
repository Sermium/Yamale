/**
 * Register pieces from a CSV: one row per piece (its engraved serial and its
 * design), judged in the browser, then registered in batches the staff member
 * signs. The chain is the progress record: running the same file again turns
 * what was registered into "already registered" and registers the rest.
 */

import { Fragment, useState } from 'react';
import { findByInscription, registerJewel, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { eventValues } from '../actions/useAction.tsx';
import { errorKey } from '../actions/errors.ts';
import { readCatalogue, uploadFile } from '../api.ts';
import { CHAIN_ID, recordUri } from '../config.ts';
import { AppBar, Loading, Section } from '../ui.tsx';
import { encodeRecord } from '../verify/record.ts';
import { parseCsv, type Csv } from './csv.ts';
import { FailureNote } from './FailureNote.tsx';
import { findRegistered, judgePieces, pieceRecord, readPieces, VERDICTS, withColumns, type Judged } from './pieces.ts';
import { paced, registerPieces, type PieceOutcome } from './register.ts';
import { useRole } from './role.ts';

type Phase =
  | { kind: 'pick' }
  | { kind: 'checking' }
  | { kind: 'judged'; csv: Csv; judged: Judged[]; ignored: string[] }
  | { kind: 'registering'; csv: Csv; judged: Judged[]; done: number; of: number }
  | { kind: 'finished'; csv: Csv; judged: Judged[]; outcomes: Map<number, PieceOutcome>; stopped: { text: string; raw: string } | null; interrupted: boolean };

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

function download(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/csv;charset=utf-8' }));
  const a = document.createElement('a');
  a.href = url; a.download = name; a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function Pieces() {
  const session = useSession();
  const [role] = useRole(session.address);
  const bar = <AppBar title={t('traceability.piecesTitle')} eyebrow="" back />;
  if (role.state === 'loading') return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (role.state !== 'role' || !role.staff || !role.active) return <>{bar}<main className="y-screen"><p className="note">{t('traceability.staffOnly')}</p></main></>;
  return (
    <>
      <AppBar title={t('traceability.piecesTitle')} eyebrow={role.jeweler.name} back />
      <main className="y-screen"><UnlockGate><PiecesBody jeweler={role.jeweler} /></UnlockGate></main>
    </>
  );
}

function PiecesBody({ jeweler }: { jeweler: { id: string; name: string; business: string } }) {
  const session = useSession();
  const [phase, setPhase] = useState<Phase>({ kind: 'pick' });
  const [failed, setFailed] = useState<{ text: string; raw: string } | null>(null);

  async function choose(file: File) {
    setFailed(null);
    setPhase({ kind: 'checking' });
    try {
      await check(file);
    } catch (e) {
      // Typically the account locked itself while the catalogue was being read.
      setFailed({ text: t('traceability.interrupted'), raw: e instanceof Error ? e.message : String(e) });
      setPhase({ kind: 'pick' });
    }
  }

  async function check(file: File) {
    const parsed = parseCsv(await file.text());
    if (!parsed.ok) { setFailed({ text: t(`traceability.csvErr.${parsed.problem}`), raw: '' }); setPhase({ kind: 'pick' }); return; }
    const pieces = readPieces(parsed.csv);
    if (!pieces.ok) { setFailed({ text: t(`traceability.csvErr.${pieces.problem}`), raw: '' }); setPhase({ kind: 'pick' }); return; }
    const cat = await session.withKey((k) => readCatalogue(k, jeweler.id, CHAIN_ID));
    if (!cat.ok) { setFailed({ text: t('traceability.catalogueUnavailable'), raw: `${cat.status} ${cat.error}` }); setPhase({ kind: 'pick' }); return; }
    const first = judgePieces(pieces.rows, cat.value.designs, new Map(), jeweler);
    const candidates = first.filter((j) => j.verdict === 'ready').map((j) => j.inscription!);
    const found = await findRegistered(candidates, jeweler.id, findByInscription);
    if (!found.ok) { setFailed({ text: t('traceability.registerUnreadable'), raw: found.detail }); setPhase({ kind: 'pick' }); return; }
    setPhase({ kind: 'judged', csv: parsed.csv, judged: judgePieces(pieces.rows, cat.value.designs, found.registered, jeweler), ignored: pieces.ignored });
  }

  async function register(csv: Csv, judged: Judged[]) {
    setFailed(null);
    const ready = judged.filter((j) => j.verdict === 'ready');
    const now = new Date();
    const pieces = ready.map((j) => ({ id: j.row.index, inscription: j.inscription!, record: encodeRecord(pieceRecord(j, jeweler, now)) }));
    setPhase({ kind: 'registering', csv, judged, done: 0, of: pieces.length });
    const out = await registerPieces(pieces, {
      async taken(ins) {
        const r = await findRegistered(ins, jeweler.id, findByInscription);
        return r.ok ? { ok: true, value: r.registered } : { ok: false, text: t('traceability.registerUnreadable'), raw: r.detail };
      },
      async upload(record) {
        const up = await paced(() => uploadFile(session.withKey((k) => k), record, CHAIN_ID),
          (r) => !r.ok && (r.status === 429 || r.status === 503), sleep);
        return up.ok ? { ok: true, value: up.hash } : { ok: false, text: t('traceability.uploadFailed'), raw: `${up.status} ${up.error}` };
      },
      async submit(batch) {
        const msgs = batch.map((b) => registerJewel(session.address!, b.inscription, b.hash, recordUri(b.hash)));
        const result = await session.submit(msgs, jeweler.business);
        if (!result.succeeded) {
          const raw = result.error?.raw ?? result.raw.rawLog ?? '';
          const key = errorKey(raw);
          return { ok: false, text: key ? t(`traceability.err.${key}`) : (result.error?.message ?? t('traceability.err.unknown')), raw };
        }
        return { ok: true, value: eventValues(result, 'blockchain.jewel.v1.EventJewelRegistered', 'jewel_id') };
      },
      progress(done, of) { setPhase({ kind: 'registering', csv, judged, done, of }); },
    });
    const outcomes = out.stopped ? await reconcile(ready, out.outcomes) : out.outcomes;
    setPhase({ kind: 'finished', csv, judged, outcomes, stopped: out.stopped, interrupted: out.interrupted });
  }

  /**
   * After a stop the register is the judge: a batch whose transaction was sent
   * just before the lock may have landed. Anything the register holds is
   * registered, whatever the run believed. If the register cannot be read the
   * outcomes stay as they were.
   */
  async function reconcile(ready: Judged[], outcomes: Map<number, PieceOutcome>): Promise<Map<number, PieceOutcome>> {
    const open = ready.filter((j) => outcomes.get(j.row.index)?.status !== 'registered');
    if (open.length === 0) return outcomes;
    try {
      const found = await findRegistered(open.map((j) => j.inscription!), jeweler.id, findByInscription);
      if (!found.ok) return outcomes;
      const next = new Map(outcomes);
      for (const j of open) {
        const id = found.registered.get(j.inscription!);
        if (id) next.set(j.row.index, { status: 'registered', jewelId: id });
      }
      return next;
    } catch {
      return outcomes;
    }
  }

  return (
    <>
      <p className="hint">{t('traceability.piecesIntro')}</p>
      {(phase.kind === 'pick' || phase.kind === 'judged' || phase.kind === 'finished') && (
        <Section title={t('traceability.chooseCsv')}>
          <input type="file" accept=".csv,text/csv" aria-label={t('traceability.chooseCsv')}
                 onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ''; if (f) void choose(f); }} />
        </Section>
      )}
      {phase.kind === 'checking' && <p className="loading" role="status">{t('traceability.checking')}</p>}
      {failed && <FailureNote text={failed.text} raw={failed.raw} />}
      {phase.kind === 'judged' && <Judgement phase={phase} onRegister={() => register(phase.csv, phase.judged)} />}
      {phase.kind === 'registering' && <p className="loading" role="status">{t('traceability.registeringProgress', { done: phase.done, of: phase.of })}</p>}
      {phase.kind === 'finished' && <Finished phase={phase} />}
    </>
  );
}

function Judgement({ phase, onRegister }: { phase: Extract<Phase, { kind: 'judged' }>; onRegister: () => void }) {
  const counts = VERDICTS.map((v) => [v, phase.judged.filter((j) => j.verdict === v).length] as const).filter(([, n]) => n > 0);
  const bad = phase.judged.filter((j) => j.verdict !== 'ready' && j.verdict !== 'alreadyRegistered');
  const ready = phase.judged.filter((j) => j.verdict === 'ready').length;
  return (
    <Section title={t('traceability.piecesChecked')}>
      {phase.ignored.length > 0 && <p className="hint">{t('traceability.ignoredColumns', { list: phase.ignored.join(', ') })}</p>}
      <dl className="facts">
        {counts.map(([v, n]) => (<Fragment key={v}><dt>{t(`traceability.verdict.${v}`)}</dt><dd className="num">{n}</dd></Fragment>))}
      </dl>
      {bad.length > 0 && (
        <ul className="y-cards">
          {bad.slice(0, 50).map((j) => (
            <li key={j.row.index} className="y-card"><span className="num">{t('traceability.rowN', { n: j.row.index + 1 })}</span> {j.row.serial || '—'} · {t(`traceability.verdict.${j.verdict}`)}</li>
          ))}
        </ul>
      )}
      <div className="actions">
        {bad.length > 0 && (
          <button type="button" className="btn" onClick={() => download('rows-to-fix.csv',
            withColumns(phase.csv, bad, (j) => ({ problem: t(`traceability.verdict.${j.verdict}`) })))}>{t('traceability.downloadProblems')}</button>
        )}
        <button type="button" className="btn btn--primary" disabled={ready === 0} onClick={onRegister}>
          {t('traceability.registerPieces', { n: ready })}
        </button>
      </div>
    </Section>
  );
}

function Finished({ phase }: { phase: Extract<Phase, { kind: 'finished' }> }) {
  const outcome = (j: Judged): { status: string; jewelId: string } => {
    const o = phase.outcomes.get(j.row.index);
    // A batch whose fate the register could not settle counts as failed.
    if (o) return o.status === 'unknown' ? { status: 'failed', jewelId: '' } : o;
    return j.verdict === 'alreadyRegistered' ? { status: 'alreadyRegistered', jewelId: j.jewelId } : { status: j.verdict, jewelId: '' };
  };
  const all = phase.judged.map(outcome);
  const registered = all.filter((o) => o.status === 'registered').length;
  const failedN = all.filter((o) => o.status === 'failed').length;
  const skipped = all.length - registered - failedN;
  return (
    <Section title={t('traceability.piecesDone')}>
      {phase.stopped && <FailureNote text={`${phase.interrupted ? t('traceability.interrupted') : phase.stopped.text} ${t('traceability.runStopped')}`} raw={phase.stopped.raw} />}
      <p className="banner banner--ok" role="status">{t('traceability.piecesSummary', { registered, skipped, failed: failedN })}</p>
      <button type="button" className="btn" onClick={() => download('registration-result.csv',
        withColumns(phase.csv, phase.judged, (j) => { const o = outcome(j); return { jewel_id: o.jewelId, status: o.status }; }))}>
        {t('traceability.downloadResult')}
      </button>
    </Section>
  );
}
