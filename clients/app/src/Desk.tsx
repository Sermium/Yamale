/**
 * The desk beside the phone.
 *
 * On a desktop screen this app used to be a phone frame floating in a large
 * empty navy field. That is a fair device preview and a weak use of a
 * boardroom projector, which is where this actually gets shown. The options
 * were to stretch the app across the width — which would have meant designing a
 * second product, and a worse one, since the real thing runs on a phone — or to
 * give the empty half a job.
 *
 * This is the job. The phone is what a person holds; the desk is the same
 * payment as the ledger records it, composed live as it is typed. It is aimed
 * at the second audience the design rules name: the one that loses trust when
 * it cannot audit what is being signed. Nothing here is a control. There is
 * nothing to click that moves money, because the thing that moves money is on
 * the phone and there should be exactly one of those.
 *
 * Below 1080px it is not rendered at all — the CSS hides it and this component
 * still mounts, so the only cost is a status query. Keeping the DOM out of the
 * phone layout is deliberate: it must never become a column the phone has to
 * share a screen with.
 */
import { useEffect, useState } from 'react';
import { t } from '@yamale/chain';

import { useDraft } from './draft.ts';
import { InstructionTable, StandingNote } from './Instruction.tsx';
import { encodeMemo, instructionFor, type Standing } from './iso.ts';
import { approvedParticipants, head, type Head } from './standing.ts';

/** How often the head is re-read. Blocks land in seconds; this is a heartbeat. */
const HEAD_EVERY_MS = 10_000;

export function Desk() {
  const draft = useDraft();

  const [chainHead, setChainHead] = useState<Head>({ known: false });
  const [standing, setStanding] = useState<Standing>({ known: false, whyKey: 'iso.standingChecking' });
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    let live = true;
    const tick = () => { void head().then((h) => { if (live) { setChainHead(h); setNow(Date.now()); } }); };
    tick();
    const timer = setInterval(tick, HEAD_EVERY_MS);
    return () => { live = false; clearInterval(timer); };
  }, []);

  useEffect(() => {
    let live = true;
    void approvedParticipants().then((s) => { if (live) setStanding(s); });
    return () => { live = false; };
  }, []);

  const msg = draft ? instructionFor(draft) : null;
  const memo = draft
    ? encodeMemo({ e2e: draft.endToEndId, purpose: draft.purposeCode, remittance: draft.remittanceInformation })
    : '';

  const names: Record<string, string> = {};
  if (draft?.debtorAddress && draft.debtorUserId) names[draft.debtorAddress] = draft.debtorUserId;
  if (draft?.creditorAddress && draft.creditorUserId) names[draft.creditorAddress] = draft.creditorUserId;

  // Something is being composed once there is a payee or a figure. A draft that
  // is only an end-to-end id is a screen somebody opened, not a payment.
  const composing = Boolean(draft && (draft.creditorAddress || draft.amount));

  return (
    <aside className="desk" aria-label={t('iso.deskTitle')}>
      <header className="desk__head">
        <h2 className="desk__title">{t('iso.deskTitle')}</h2>
        <p className="desk__lede">{t('iso.deskLede')}</p>
      </header>

      <section className="desk__panel">
        <h3 className="desk__h">{t('iso.chainTitle')}</h3>
        <dl className="facts">
          <Fact label={t('iso.chainId')} value={chainHead.known ? chainHead.chainId : null} mono />
          <Fact
            label={t('iso.height')}
            value={chainHead.known ? chainHead.height.toLocaleString() : null}
            mono
          />
          <Fact
            label={t('iso.lastBlock')}
            value={chainHead.known ? age(now, chainHead.at) : null}
          />
          <Fact
            label={t('iso.standingTitle')}
            value={standing.known ? String(standing.participants.length) : null}
            mono
          />
        </dl>
        {/* Honest about staleness rather than silent about it: a page showing a
            block height that stopped moving looks identical to one showing a
            live chain, and this is the only thing that tells them apart. */}
        {!chainHead.known && <p className="desk__warn">{t('iso.chainUnreachable')}</p>}
        {chainHead.known && chainHead.catchingUp && <p className="desk__warn">{t('iso.chainCatchingUp')}</p>}
      </section>

      <section className="desk__panel">
        <h3 className="desk__h">{t('iso.title')}</h3>
        <p className="desk__note">{t('iso.lede')}</p>

        {composing && msg ? (
          <>
            <InstructionTable msg={msg} names={names} />
            <h4 className="desk__h4">{t('iso.memoTitle')}</h4>
            {memo
              ? <p className="ref-shown y-mono">{memo}</p>
              : <p className="iso__none">{t('iso.memoEmpty')}</p>}
          </>
        ) : (
          /* The empty state, drawn rather than blank. This app spends most of
             its life here — nobody signed in, or nothing typed yet — and a
             blank half-screen would say the desk was broken. */
          <p className="desk__empty">{t('iso.deskEmpty')}</p>
        )}

        <StandingNote standing={standing} />
      </section>

      <section className="desk__panel">
        <h3 className="desk__h">{t('iso.refusesTitle')}</h3>
        <ul className="refuses">
          <li>{t('iso.refuse1')}</li>
          <li>{t('iso.refuse2')}</li>
          <li>{t('iso.refuse3')}</li>
          <li>{t('iso.refuse4')}</li>
        </ul>
      </section>
    </aside>
  );
}

/** One figure, or the word for not having it. Never a zero standing in for a
 *  number that could not be read. */
function Fact({ label, value, mono }: { label: string; value: string | null; mono?: boolean }) {
  return (
    <div className="facts__row">
      <dt>{label}</dt>
      <dd className={value === null ? 'facts__unknown' : mono ? 'y-mono' : undefined}>
        {value ?? t('iso.unknown')}
      </dd>
    </div>
  );
}

function age(now: number, at: Date): string {
  const seconds = Math.max(0, Math.round((now - at.getTime()) / 1000));
  if (seconds < 90) return t('iso.secondsAgo', { n: seconds });
  return t('iso.minutesAgo', { n: Math.round(seconds / 60) });
}
