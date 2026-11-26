/**
 * The purse: what the account holds, one currency at a time. The currency
 * chosen here is the account's active currency in this app: the one a sale is
 * priced in by default (SellPanel). The others wait in the dropdown.
 *
 * The network's own fee token is left out: the platform pays the fees, and a
 * balance nobody spends would only raise the question of what it is for.
 */

import { useEffect, useId, useState } from 'react';
import { bankBalances, formatAmount, resolveDenom, t } from '@yamale/chain';

import { Icon } from '../icons.tsx';
import { useLoad } from '../ui.tsx';
import { PAY_APP, activeDenom, setActiveDenom } from './money.ts';

/** The network's fee token: held for fees, never shown as money. */
const FEE_DENOM = 'uyml';

export function Purse({ address }: { address: string }) {
  const uid = useId();
  const [held, reload] = useLoad(() => bankBalances(address), [address]);
  const coins = held?.ok
    ? held.value
        .filter((c) => c.denom !== FEE_DENOM && BigInt(c.amount) > 0n)
        .map((c) => ({ ...c, info: resolveDenom(c.denom) }))
        .sort((a, b) => a.info.symbol.localeCompare(b.info.symbol))
    : [];
  const [active, setActive] = useState<string | null>(activeDenom);
  // The active currency is one the account holds; until it chooses, USDC, the
  // currency sales are usually priced in, or else the first.
  const shown = coins.find((c) => c.denom === active) ?? coins.find((c) => c.denom === "uusdc") ?? coins[0];
  useEffect(() => {
    if (shown && shown.denom !== active) { setActive(shown.denom); setActiveDenom(shown.denom); }
  }, [shown?.denom, active]);

  return (
    <section className="purse" aria-labelledby={uid}>
      <header className="purse__head">
        <h2 id={uid} className="purse__title"><Icon name="wallet" /> {t('traceability.purseTitle')}</h2>
        <a className="purse__topup" href={PAY_APP} target="_blank" rel="noreferrer">{t('traceability.purseTopUp')}</a>
      </header>
      {!held && <p className="purse__empty">{t('traceability.loading')}</p>}
      {held && !held.ok && (
        <p className="purse__empty">
          {t('traceability.purseUnread')}{' '}
          <button type="button" className="linkbtn" onClick={reload}>{t('traceability.retry')}</button>
        </p>
      )}
      {held?.ok && !shown && <p className="purse__empty">{t('traceability.purseEmpty')}</p>}
      {shown && (<>
        <p className="purse__amount num">{formatAmount(shown.amount, shown.denom)}</p>
        <div className="purse__pick">
          <label className="purse__label" htmlFor={uid + '-cur'}>{t('traceability.purseActive')}</label>
          <select id={uid + '-cur'} className="purse__select" value={shown.denom}
                  onChange={(e) => { setActive(e.target.value); setActiveDenom(e.target.value); }}>
            {coins.map((c) => (
              <option key={c.denom} value={c.denom}>{c.info.symbol} · {c.info.name} · {formatAmount(c.amount, c.denom, { withSymbol: false })}</option>
            ))}
          </select>
        </div>
        <p className="hint">{t('traceability.purseActiveHint')}</p>
      </>)}
    </section>
  );
}
