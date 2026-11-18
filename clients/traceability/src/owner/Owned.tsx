/**
 * One jewel its owner holds, and everything the owner can do with it.
 */

import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import {
  chainHead, clearJewelStolen, isActive, makeOwnershipProof, readJewel, readJeweler, reportJewelStolen, t,
  vetoJewelReassignment, type JewelView,
} from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { CHAIN_ID, PUBLIC_ORIGIN } from '../config.ts';
import { AppBar, day, isId, Loading, NotFound, Problem, Section, ShareButtons, useLoad } from '../ui.tsx';
import { benefitState, type IssuerStatus } from '../verify/history.ts';
import { benefitKind, Hallmark } from '../verify/JewelPage.tsx';
import { useRecord } from '../verify/record.ts';
import { forgetStaleLinks } from './links.ts';
import { OfferPanel } from './OfferPanel.tsx';
import { liveClaimKey } from './offers.ts';
import { VoucherPanel } from './VoucherPanel.tsx';

function Stolen({ view, onChange }: { view: JewelView; onChange: () => void }) {
  const session = useSession();
  const [action, run] = useAction();
  const [confirm, setConfirm] = useState(false);
  const id = view.jewel.id;
  if (view.jewel.stolen) {
    return (
      <UnlockGate>
        <p>{t('traceability.youReportedStolen')}</p>
        <button type="button" className="btn" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([clearJewelStolen(session.address!, id)]))) onChange();
        }}>{t('traceability.foundIt')}</button>
        <ActionStatus state={action} />
      </UnlockGate>
    );
  }
  if (!confirm) return <button type="button" className="btn btn--danger" onClick={() => setConfirm(true)}>{t('traceability.reportStolen')}</button>;
  return (
    <UnlockGate>
      <p className="note note--warn">{t('traceability.reportStolenConfirm')}</p>
      <div className="actions">
        <button type="button" className="btn btn--danger" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([reportJewelStolen(session.address!, id)]))) { setConfirm(false); onChange(); }
        }}>{t('traceability.reportStolen')}</button>
        <button type="button" className="btn" onClick={() => setConfirm(false)}>{t('traceability.cancel')}</button>
      </div>
      <ActionStatus state={action} />
    </UnlockGate>
  );
}

function Veto({ view, onChange }: { view: JewelView; onChange: () => void }) {
  const session = useSession();
  const [action, run] = useAction();
  if (!view.reassignment) return null;
  return (
    <div className="banner banner--warn" role="alert">
      <p>{t('traceability.reassignmentYours', { date: day(view.reassignment.windowEnds) })}</p>
      <UnlockGate>
        <button type="button" className="btn btn--danger" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([vetoJewelReassignment(session.address!, view.jewel.id)]))) onChange();
        }}>{t('traceability.veto')}</button>
        <ActionStatus state={action} />
      </UnlockGate>
    </div>
  );
}

function Benefits({ view, issuer, now }: { view: JewelView; issuer: IssuerStatus; now: number }) {
  const [open, setOpen] = useState<string | null>(null);
  if (view.benefits.length === 0) return <p className="hint">{t('traceability.noBenefits')}</p>;
  return (
    <ul className="y-cards">
      {view.benefits.map((b) => {
        const s = benefitState(b, issuer, now);
        // With the issuer unread nobody can say the benefit will be honoured, so no use is offered.
        const usable = s.state === 'active' && s.left > 0 && !view.jewel.stolen && issuer === 'active';
        return (
          <li key={b.id} className="y-card stack">
            <div className="benefit">
              <span className="benefit__kind">{benefitKind(b.kind)}</span>
              {s.state === 'active' && <span className="y-chip y-chip--ok num">{t('traceability.benefitLeft', { left: s.left, allowance: b.allowance })}</span>}
              {s.state === 'expired' && <span className="y-chip y-chip--mute">{t('traceability.benefitExpired', { date: day(s.expiredAt) })}</span>}
              {s.state === 'withdrawn' && <span className="y-chip y-chip--warn">{t('traceability.benefitWithdrawn')}</span>}
            </div>
            {usable && (open === b.id
              ? <UnlockGate><VoucherPanel jewelId={view.jewel.id} benefitId={b.id} /></UnlockGate>
              : <button type="button" className="btn" onClick={() => setOpen(b.id)}>{t('traceability.useBenefit')}</button>)}
          </li>
        );
      })}
    </ul>
  );
}

function Proof({ jewelId }: { jewelId: string }) {
  const session = useSession();
  const [link, setLink] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  if (link) return <ShareButtons link={link} title={t('traceability.proofTitle')} text={t('traceability.proofShareText')} />;
  return (
    <UnlockGate>
      <p className="hint">{t('traceability.proofExplain')}</p>
      <button type="button" className="btn" onClick={async () => {
        try {
          // Dated by the chain's clock; without it a proof could be refused as from the future.
          const head = await chainHead();
          if (!head.ok) { setFailed(true); return; }
          const q = session.withKey((k) => makeOwnershipProof(k, CHAIN_ID, jewelId, head.value.time));
          setFailed(false);
          setLink(`${PUBLIC_ORIGIN}/own/${jewelId}?${q}`);
        } catch {
          setFailed(true);
        }
      }}>{t('traceability.makeProof')}</button>
      {failed && <p className="note note--warn" role="alert">{t('traceability.proofFailed')}</p>}
    </UnlockGate>
  );
}

const NO_SUCH = { ok: false as const, reason: 'not-found' as const, detail: '' };

export function Owned() {
  const { jewel: id = '' } = useParams();
  const valid = isId(id);
  const session = useSession();
  const [view, reload] = useLoad(() => (valid ? readJewel(id) : Promise.resolve(NO_SUCH)), [id, valid]);
  const [head] = useLoad(() => chainHead(), []);
  const [jeweler] = useLoad(() => (view?.ok ? readJeweler(view.value.jewel.jewelerId) : Promise.resolve(NO_SUCH)), [view?.ok]);
  const record = useRecord(view?.ok ? view.value.jewel : null);
  const headTime = head?.ok ? head.value.time : null;
  useEffect(() => {
    // Links are kept only for the offer that is open: sweep the rest once the jewel and the clock are read.
    if (view?.ok && headTime !== null) forgetStaleLinks(id, liveClaimKey(view.value.offer, headTime));
  }, [view, headTime, id]);

  if (!valid) return <NotFound />;
  if (!session.account) {
    return <><AppBar title={t('traceability.mineTitle')} eyebrow="" back /><main className="y-screen"><UnlockGate>{null}</UnlockGate></main></>;
  }
  if (!view || !head) return <><AppBar title={t('traceability.mineTitle')} eyebrow="" back /><main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <><AppBar title={t('traceability.mineTitle')} eyebrow="" back /><main className="y-screen"><Problem outcome={view} /></main></>;
  if (!head.ok) return <><AppBar title={t('traceability.mineTitle')} eyebrow="" back /><main className="y-screen"><Problem outcome={head} /></main></>;
  const v = view.value;
  const now = head.value.time;
  const title = record.state === 'ok' ? record.record.title : t('traceability.jewelN', { id });
  const publicPage = `/j/${v.jewel.jewelerId}/${v.jewel.id}`;

  return (
    <>
      <AppBar title={title} eyebrow={t('traceability.mineTitle')} back />
      <main className="y-screen">
        <header className="jewel__mast"><h1 className="jewel__title">{title}</h1><Hallmark code={v.jewel.inscription} /></header>
        {record.state === 'mismatch' && <div className="banner banner--bad" role="alert">{t('traceability.recordMismatch')}</div>}
        <Link className="btn" to={publicPage}>{t('traceability.seePublicPage')}</Link>
        {v.owner !== session.address ? (
          <p className="note">{t('traceability.notYours')}</p>
        ) : (
          <>
            <Veto view={v} onChange={reload} />
            <Section title={t('traceability.passOn')}>
              <UnlockGate>
                <OfferPanel view={v} actor={session.address!} now={now} send={(m) => session.submitAsOwner(m)} onChange={reload} />
              </UnlockGate>
            </Section>
            <Section title={t('traceability.benefits')}>
              {jeweler
                ? <>
                    {jeweler.ok ? null : <Problem outcome={jeweler} />}
                    <Benefits view={v} issuer={jeweler.ok ? (isActive(jeweler.value) ? 'active' : 'suspended') : 'unknown'} now={now} />
                  </>
                : <Loading />}
            </Section>
            <Section title={t('traceability.proveIt')}><Proof jewelId={v.jewel.id} /></Section>
            <Section title={t('traceability.stolenTitle')}><Stolen view={v} onChange={reload} /></Section>
          </>
        )}
      </main>
    </>
  );
}
