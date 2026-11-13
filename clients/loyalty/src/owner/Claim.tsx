/**
 * /claim#<jewel>.<claim key>: where a buyer lands from the seller's link.
 *
 * The key in the fragment signs a claim that names this account, so a copy
 * of the transaction taken from the mempool is useless to anyone else. Once
 * claimed, the fragment is wiped from the address bar and the history.
 */

import { useMemo } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  chainHead, claimJewelOffer, compressedPublicKey, parseClaimFragment, readJewel, readJeweler, t, toHex,
} from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { CHAIN_ID } from '../config.ts';
import { offerIsOpen } from './offers.ts';
import { AppBar, Loading, Problem, useLoad } from '../ui.tsx';
import { Hallmark } from '../verify/JewelPage.tsx';
import { Gallery } from '../verify/Photos.tsx';
import { useRecord } from '../verify/record.ts';

const NO_SUCH = { ok: false as const, reason: 'not-found' as const, detail: '' };

export function Claim() {
  const { hash } = useLocation();
  const parsed = useMemo(() => parseClaimFragment(hash), [hash]);
  const session = useSession();
  const navigate = useNavigate();
  const [action, run] = useAction();
  const [view] = useLoad(() => (parsed ? readJewel(parsed.jewelId) : Promise.resolve(NO_SUCH)), [parsed?.jewelId]);
  const [head] = useLoad(() => chainHead(), []);
  const [jeweler] = useLoad(() => (view?.ok ? readJeweler(view.value.jewel.jewelerId) : Promise.resolve(NO_SUCH)), [view?.ok]);
  const record = useRecord(view?.ok ? view.value.jewel : null);

  const bar = <AppBar title={t('loyalty.claimTitle')} eyebrow={t('loyalty.title')} />;
  if (!parsed) return <>{bar}<main className="y-screen"><p className="banner banner--bad">{t('loyalty.claimBroken')}</p></main></>;
  if (!view || !head) return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <>{bar}<main className="y-screen"><Problem outcome={view} /></main></>;
  if (!head.ok) return <>{bar}<main className="y-screen"><Problem outcome={head} /></main></>;

  const offer = view.value.offer;
  const claimKey = toHex(compressedPublicKey(parsed.privateKey));
  const open = offer?.claimKey === claimKey && offerIsOpen(offer, head.value.time);
  const title = record.state === 'ok' ? record.record.title : t('loyalty.jewelN', { id: parsed.jewelId });

  return (
    <>
      {bar}
      <main className="y-screen">
        <header className="jewel__mast">
          <p className="jewel__by">{t('loyalty.registeredBy', { name: jeweler?.ok ? jeweler.value.name : '' })}</p>
          <h1 className="jewel__title">{title}</h1>
          <Hallmark code={view.value.jewel.inscription} />
        </header>
        {record.state === 'ok' && <Gallery hashes={record.record.photos} alt={title} />}
        {record.state === 'mismatch' && <div className="banner banner--bad" role="alert">{t('loyalty.recordMismatch')}</div>}
        {(record.state === 'missing' || record.state === 'unreachable' || record.state === 'unreadable') && (
          <p className="note">{t('loyalty.recordMissing')}</p>
        )}
        <p className="declared">{t('loyalty.declared')}</p>
        {!open && <p className="banner banner--warn">{t('loyalty.claimClosed')}</p>}
        {open && (
          <UnlockGate>
            <p>{t('loyalty.claimExplain')}</p>
            <button type="button" className="btn btn--primary" disabled={action.kind === 'busy'} onClick={async () => {
              const done = await run(() => session.submitAsOwner(
                [claimJewelOffer(session.address!, parsed.jewelId, parsed.privateKey, CHAIN_ID)]));
              if (done) {
                window.history.replaceState(null, '', '/claim');
                navigate(`/mine/${parsed.jewelId}`, { replace: true });
              }
            }}>{t('loyalty.claimButton')}</button>
            <ActionStatus state={action} />
          </UnlockGate>
        )}
      </main>
    </>
  );
}
