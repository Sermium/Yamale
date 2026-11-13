/**
 * One jewel its owner holds, and everything the owner can do with it.
 */

import { useState } from 'react';
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
import { benefitState } from '../verify/history.ts';
import { benefitKind, Hallmark } from '../verify/JewelPage.tsx';
import { currentRecordHash, useRecord } from '../verify/record.ts';
import { OfferPanel } from './OfferPanel.tsx';
import { VoucherPanel } from './VoucherPanel.tsx';

function Stolen({ view, onChange }: { view: JewelView; onChange: () => void }) {
  const session = useSession();
  const [action, run] = useAction();
  const [confirm, setConfirm] = useState(false);
  const id = view.jewel.id;
  if (view.jewel.stolen) {
    return (
      <UnlockGate>
        <p>{t('loyalty.youReportedStolen')}</p>
        <button type="button" className="btn" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([clearJewelStolen(session.address!, id)]))) onChange();
        }}>{t('loyalty.foundIt')}</button>
        <ActionStatus state={action} />
      </UnlockGate>
    );
  }
  if (!confirm) return <button type="button" className="btn btn--danger" onClick={() => setConfirm(true)}>{t('loyalty.reportStolen')}</button>;
  return (
    <UnlockGate>
      <p className="note note--warn">{t('loyalty.reportStolenConfirm')}</p>
      <div className="actions">
        <button type="button" className="btn btn--danger" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([reportJewelStolen(session.address!, id)]))) { setConfirm(false); onChange(); }
        }}>{t('loyalty.reportStolen')}</button>
        <button type="button" className="btn" onClick={() => setConfirm(false)}>{t('loyalty.cancel')}</button>
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
      <p>{t('loyalty.reassignmentYours', { date: day(view.reassignment.windowEnds) })}</p>
      <UnlockGate>
        <button type="button" className="btn btn--danger" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([vetoJewelReassignment(session.address!, view.jewel.id)]))) onChange();
        }}>{t('loyalty.veto')}</button>
        <ActionStatus state={action} />
      </UnlockGate>
    </div>
  );
}

function Benefits({ view, issuerActive, now }: { view: JewelView; issuerActive: boolean; now: number }) {
  const [open, setOpen] = useState<string | null>(null);
  if (view.benefits.length === 0) return <p className="hint">{t('loyalty.noBenefits')}</p>;
  return (
    <ul className="y-cards">
      {view.benefits.map((b) => {
        const s = benefitState(b, issuerActive, now);
        const usable = s.state === 'active' && s.left > 0 && !view.jewel.stolen;
        return (
          <li key={b.id} className="y-card stack">
            <div className="benefit">
              <span className="benefit__kind">{benefitKind(b.kind)}</span>
              {s.state === 'active' && <span className="y-chip y-chip--ok num">{t('loyalty.benefitLeft', { left: s.left, allowance: b.allowance })}</span>}
              {s.state === 'expired' && <span className="y-chip y-chip--mute">{t('loyalty.benefitExpired', { date: day(s.expiredAt) })}</span>}
              {s.state === 'withdrawn' && <span className="y-chip y-chip--warn">{t('loyalty.benefitWithdrawn')}</span>}
            </div>
            {usable && (open === b.id
              ? <UnlockGate><VoucherPanel jewelId={view.jewel.id} benefitId={b.id} /></UnlockGate>
              : <button type="button" className="btn" onClick={() => setOpen(b.id)}>{t('loyalty.useBenefit')}</button>)}
          </li>
        );
      })}
    </ul>
  );
}

function Proof({ jewelId }: { jewelId: string }) {
  const session = useSession();
  const [link, setLink] = useState<string | null>(null);
  if (link) return <ShareButtons link={link} title={t('loyalty.proofTitle')} text={t('loyalty.proofShareText')} />;
  return (
    <UnlockGate>
      <p className="hint">{t('loyalty.proofExplain')}</p>
      <button type="button" className="btn" onClick={async () => {
        const head = await chainHead();
        const now = head.ok ? head.value.time : Math.floor(Date.now() / 1000);
        const q = session.withKey((k) => makeOwnershipProof(k, CHAIN_ID, jewelId, now));
        setLink(`${PUBLIC_ORIGIN}/own/${jewelId}?${q}`);
      }}>{t('loyalty.makeProof')}</button>
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
  const record = useRecord(view?.ok ? currentRecordHash(view.value.jewel) : null);

  if (!valid) return <NotFound />;
  if (!view) return <><AppBar title={t('loyalty.mineTitle')} eyebrow="" back /><main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <><AppBar title={t('loyalty.mineTitle')} eyebrow="" back /><main className="y-screen"><Problem outcome={view} /></main></>;
  const v = view.value;
  const now = head?.ok ? head.value.time : Math.floor(Date.now() / 1000);
  const title = record.state === 'ok' ? record.record.title : t('loyalty.jewelN', { id });
  const publicPage = `/j/${v.jewel.jewelerId}/${v.jewel.id}`;

  return (
    <>
      <AppBar title={title} eyebrow={t('loyalty.mineTitle')} back />
      <main className="y-screen">
        <header className="jewel__mast"><h1 className="jewel__title">{title}</h1><Hallmark code={v.jewel.inscription} /></header>
        <Link className="btn" to={publicPage}>{t('loyalty.seePublicPage')}</Link>
        {v.owner !== session.address ? (
          <p className="note">{t('loyalty.notYours')}</p>
        ) : (
          <>
            <Veto view={v} onChange={reload} />
            <Section title={t('loyalty.passOn')}>
              <UnlockGate>
                <OfferPanel view={v} actor={session.address!} send={(m) => session.submitAsOwner(m)} onChange={reload} />
              </UnlockGate>
            </Section>
            <Section title={t('loyalty.benefits')}>
              {jeweler && head ? <Benefits view={v} issuerActive={jeweler.ok ? isActive(jeweler.value) : false} now={now} /> : <Loading />}
            </Section>
            <Section title={t('loyalty.proveIt')}><Proof jewelId={v.jewel.id} /></Section>
            <Section title={t('loyalty.stolenTitle')}><Stolen view={v} onChange={reload} /></Section>
          </>
        )}
      </main>
    </>
  );
}
