/**
 * /card#<jewel>.<secret>: where the QR on an ownership card lands.
 *
 * The page shows the jewel, then lets the person on this phone claim it into
 * their account: signed in, unlocked, signing in, or making an account here,
 * all inside UnlockGate, so the page never leaves. The card is also kept in
 * this tab (cards.ts) in case it reloads. Filing a claim does not move the
 * jewel: the shop validates it once it knows this person paid.
 */

import { useMemo, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { chainHead, fileJewelClaim, readJewel, readJeweler, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { CHAIN_ID } from '../config.ts';
import { AppBar, Loading, Problem, useLoad } from '../ui.tsx';
import { Hallmark } from '../verify/JewelPage.tsx';
import { Gallery } from '../verify/Photos.tsx';
import { useRecord } from '../verify/record.ts';
import { cardFromLocation, forgetScannedCard } from './cards.ts';

const NO_SUCH = { ok: false as const, reason: 'not-found' as const, detail: '' };

export function CardPage() {
  const { hash } = useLocation();
  const card = useMemo(() => cardFromLocation(hash), [hash]);
  const session = useSession();
  const [action, run] = useAction();
  const [filed, setFiled] = useState(false);
  const [view, reload] = useLoad(() => (card ? readJewel(card.jewelId) : Promise.resolve(NO_SUCH)), [card?.jewelId]);
  const [head] = useLoad(() => chainHead(), []);
  const [jeweler] = useLoad(() => (view?.ok ? readJeweler(view.value.jewel.jewelerId) : Promise.resolve(NO_SUCH)), [view?.ok]);
  const record = useRecord(view?.ok ? view.value.jewel : null);

  const bar = <AppBar title={t('traceability.cardTitle')} eyebrow={t('traceability.title')} />;
  if (!card) return <>{bar}<main className="y-screen"><p className="banner banner--bad">{t('traceability.cardBroken')}</p></main></>;
  if (!view || !head || !jeweler) return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <>{bar}<main className="y-screen"><Problem outcome={view} /></main></>;

  const v = view.value;
  const shop = jeweler.ok ? jeweler.value : null;
  const shopName = shop?.name ?? t('traceability.jewelerN', { id: v.jewel.jewelerId });
  const title = record.state === 'ok' ? record.record.title : t('traceability.jewelN', { id: card.jewelId });
  const me = session.address;
  const inStock = !!shop && v.owner === shop.business;
  const mine = !!me && v.owner === me;
  const myClaim = !!me && v.claim?.claimer === me;

  // What the card can still do, from the jewel as it stands.
  let state: 'mine' | 'claimed-by-me' | 'claimed-by-other' | 'spent' | 'no-card' | 'stolen' | 'shop' | 'open';
  if (mine) state = 'mine';
  else if (myClaim || filed) state = 'claimed-by-me';
  else if (v.jewel.stolen) state = 'stolen';
  else if (!inStock) state = 'spent';
  else if (!v.hasCard) state = 'no-card';
  else if (v.claim) state = 'claimed-by-other';
  else if (me && shop && me === shop.business) state = 'shop';
  else state = 'open';

  return (
    <>
      {bar}
      <main className="y-screen">
        <header className="jewel__mast">
          <p className="jewel__by">{t('traceability.registeredBy', { name: shopName })}</p>
          <h1 className="jewel__title">{title}</h1>
          <Hallmark code={v.jewel.inscription} />
        </header>
        {record.state === 'ok' && <Gallery hashes={record.record.photos} alt={title} />}
        {record.state === 'mismatch' && <div className="banner banner--bad" role="alert">{t('traceability.recordMismatch')}</div>}
        {(record.state === 'missing' || record.state === 'unreachable' || record.state === 'unreadable') && (
          <p className="note">{t('traceability.recordMissing')}</p>
        )}
        <Link className="btn" to={`/j/${v.jewel.jewelerId}/${v.jewel.id}`}>{t('traceability.seePublicPage')}</Link>

        {state === 'mine' && (
          <div className="banner banner--ok" role="status">
            <span>{t('traceability.cardYours')}</span> <Link to={`/mine/${v.jewel.id}`}>{t('traceability.navMine')}</Link>
          </div>
        )}
        {state === 'claimed-by-me' && (
          <div className="banner banner--ok" role="status">
            <span>{t('traceability.cardClaimed', { shop: shopName })}</span> <Link to="/mine">{t('traceability.navMine')}</Link>
          </div>
        )}
        {state === 'stolen' && <p className="banner banner--bad">{t('traceability.cannotOfferStolen')}</p>}
        {state === 'spent' && <p className="banner banner--warn">{t('traceability.cardSpent')}</p>}
        {state === 'no-card' && <p className="banner banner--warn">{t('traceability.cardCancelled', { shop: shopName })}</p>}
        {state === 'claimed-by-other' && <p className="banner banner--warn">{t('traceability.cardClaimedOther', { shop: shopName })}</p>}
        {state === 'shop' && <p className="note">{t('traceability.cardShopAccount')}</p>}

        {state === 'open' && (
          <UnlockGate>
            <p>{t('traceability.cardClaimExplain', { shop: shopName })}</p>
            {me && (
              <p className="claimrow__who">
                <span>{t('traceability.cardLandsIn')}</span>
                <code className="inscription">{me}</code>
              </p>
            )}
            <button type="button" className="btn btn--primary" disabled={action.kind === 'busy' || !me} onClick={async () => {
              const done = await run(() => session.submitAsOwner([fileJewelClaim(me!, card.jewelId, card.privateKey, CHAIN_ID)]));
              if (done) {
                forgetScannedCard();
                window.history.replaceState(null, '', '/card');
                setFiled(true);
                reload();
              }
            }}>{t('traceability.cardClaimButton')}</button>
            <ActionStatus state={action} />
          </UnlockGate>
        )}
      </main>
    </>
  );
}
