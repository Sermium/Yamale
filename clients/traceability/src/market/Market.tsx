/**
 * /market: every piece for sale, newest jewel last, with filters for the
 * jeweler and the currency. Readable without an account.
 */

import { useState } from 'react';
import { chainHead, readJewelers, readListings, resolveDenom, t } from '@yamale/chain';

import { AppBar, Loading, Problem, useLoad } from '../ui.tsx';
import { JewelCard } from '../verify/JewelCard.tsx';
import { useSession } from '../account/session.tsx';
import { money } from './money.ts';
import { Purse } from './Purse.tsx';

export function Market() {
  const session = useSession();
  const [listings] = useLoad(() => readListings(), []);
  const [jewelers] = useLoad(() => readJewelers(), []);
  const [head] = useLoad(() => chainHead(), []);
  const [jeweler, setJeweler] = useState('');
  const [denom, setDenom] = useState('');

  const bar = <AppBar title={t('traceability.marketTitle')} eyebrow={t('traceability.title')} />;
  if (!listings || !head) return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (!listings.ok) return <>{bar}<main className="y-screen"><Problem outcome={listings} /></main></>;
  const now = head.ok ? head.value.time : Math.floor(Date.now() / 1000);
  const names = new Map((jewelers?.ok ? jewelers.value : []).map((j) => [j.id, j.name]));
  const shown = listings.value.filter((l) => (!jeweler || l.jewel.jewelerId === jeweler) && (!denom || l.listing.price?.denom === denom));
  const usedJewelers = [...new Set(listings.value.map((l) => l.jewel.jewelerId))];
  const usedDenoms = [...new Set(listings.value.map((l) => l.listing.price?.denom).filter((d): d is string => !!d))];

  return (
    <>
      {bar}
      <main className="y-screen">
        <p className="lede">{t('traceability.marketLede')}</p>
        {session.address && <Purse address={session.address} />}
        {listings.value.length > 0 && (
          <div className="actions">
            <select className="bigfield" value={jeweler} onChange={(e) => setJeweler(e.target.value)} aria-label={t('traceability.marketJeweler')}>
              <option value="">{t('traceability.marketAllJewelers')}</option>
              {usedJewelers.map((id) => <option key={id} value={id}>{names.get(id) ?? t('traceability.jewelerN', { id })}</option>)}
            </select>
            <select className="bigfield" value={denom} onChange={(e) => setDenom(e.target.value)} aria-label={t('traceability.sellCurrency')}>
              <option value="">{t('traceability.marketAllCurrencies')}</option>
              {usedDenoms.map((d) => <option key={d} value={d}>{resolveDenom(d).symbol}</option>)}
            </select>
          </div>
        )}
        {shown.length === 0 && <p className="hint">{t('traceability.marketEmpty')}</p>}
        <ul className="y-cards">
          {shown.map((l) => (
            <JewelCard key={l.jewel.id} id={l.jewel.id} now={now} to={`/j/${l.jewel.jewelerId}/${l.jewel.id}`}
                       extra={
                         <p className="marketrow">
                           <span className="hint">{names.get(l.jewel.jewelerId) ?? ''}</span>
                           <strong className="num">{l.listing.price ? money(l.listing.price) : ''}</strong>
                         </p>
                       } />
          ))}
        </ul>
      </main>
    </>
  );
}
