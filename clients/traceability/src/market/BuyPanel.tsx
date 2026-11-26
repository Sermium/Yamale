/**
 * Buying a listed piece from its public page. The buyer sees the price, what
 * they hold of that currency, and that a purchase cannot be undone; the chain
 * then pays the seller and moves the piece in one step.
 */

import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { bankBalance, buyJewel, t, type JewelView } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { Section, useLoad } from '../ui.tsx';
import { money, PAY_APP, shortfall } from './money.ts';

export function BuyPanel({ view }: { view: JewelView }) {
  const listing = view.listing;
  if (!listing?.price || view.jewel.stolen) return null;
  return (
    <Section title={t('traceability.forSale')}>
      <p className="price-tag"><strong className="num">{money(listing.price)}</strong></p>
      <Buy view={view} />
    </Section>
  );
}

function Buy({ view }: { view: JewelView }) {
  const session = useSession();
  const navigate = useNavigate();
  const price = view.listing!.price!;
  const me = session.address;
  const [balance] = useLoad(() => (me ? bankBalance(me, price.denom) : Promise.resolve(null)), [me, price.denom], () => null);
  const [action, run] = useAction();
  const [sure, setSure] = useState(false);

  if (!me) return <UnlockGate><span /></UnlockGate>;
  if (me === view.owner) return <p className="hint">{t('traceability.buyYours')}</p>;
  const missing = balance?.ok ? shortfall(price.amount, balance.value) : null;

  async function buy() {
    if (await run(() => session.submitAsOwner([buyJewel(me!, view.jewel.id, price)]))) navigate(`/mine/${view.jewel.id}`);
  }

  return (
    <div className="stack">
      {balance?.ok && <p className="hint num">{t('traceability.buyBalance', { amount: money({ denom: price.denom, amount: balance.value }) })}</p>}
      {missing !== null && missing > 0n ? (
        <p className="note note--warn">
          {t('traceability.buyShort', { amount: money({ denom: price.denom, amount: missing.toString() }) })}{' '}
          <a href={PAY_APP} target="_blank" rel="noreferrer">{t('traceability.buyTopUp')}</a>
        </p>
      ) : (
        <UnlockGate>
          <label className="tick">
            <input type="checkbox" checked={sure} onChange={(e) => setSure(e.target.checked)} />
            {t('traceability.buyFinal', { amount: money(price) })}
          </label>
          <button type="button" className="btn btn--primary" disabled={!sure || action.kind === 'busy'} onClick={buy}>
            {t('traceability.buyButton', { amount: money(price) })}
          </button>
        </UnlockGate>
      )}
      <ActionStatus state={action} />
    </div>
  );
}
