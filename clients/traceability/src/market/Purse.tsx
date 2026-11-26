/**
 * The purse: what the account holds, in the currencies people pay with. The
 * network's own fee token is left out — the platform pays the fees, and a
 * balance nobody spends would only raise the question of what it is for.
 */

import { bankBalances, formatAmount, resolveDenom, t } from '@yamale/chain';

import { Icon } from '../icons.tsx';
import { useLoad } from '../ui.tsx';
import { PAY_APP } from './money.ts';

/** The network's fee token: held for fees, never shown as money. */
const FEE_DENOM = 'uyml';

export function Purse({ address }: { address: string }) {
  const [held, reload] = useLoad(() => bankBalances(address), [address]);
  const coins = held?.ok
    ? held.value
        .filter((c) => c.denom !== FEE_DENOM && BigInt(c.amount) > 0n)
        .map((c) => ({ ...c, info: resolveDenom(c.denom) }))
        .sort((a, b) => a.info.symbol.localeCompare(b.info.symbol))
    : [];

  return (
    <section className="purse" aria-labelledby="purse-title">
      <header className="purse__head">
        <h2 id="purse-title" className="purse__title"><Icon name="wallet" /> {t('traceability.purseTitle')}</h2>
        <a className="purse__topup" href={PAY_APP} target="_blank" rel="noreferrer">{t('traceability.purseTopUp')}</a>
      </header>
      {!held && <p className="purse__empty">{t('traceability.loading')}</p>}
      {held && !held.ok && (
        <p className="purse__empty">
          {t('traceability.purseUnread')}{' '}
          <button type="button" className="linkbtn" onClick={reload}>{t('traceability.retry')}</button>
        </p>
      )}
      {held?.ok && coins.length === 0 && <p className="purse__empty">{t('traceability.purseEmpty')}</p>}
      {coins.length > 0 && (
        <ul className="purse__list">
          {coins.map((c) => (
            <li key={c.denom} className="purse__row">
              <span className="purse__name">{c.info.name}</span>
              <span className="purse__amount num">{formatAmount(c.amount, c.denom)}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
