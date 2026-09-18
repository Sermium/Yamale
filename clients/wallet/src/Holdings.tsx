/**
 * What an account holds, on a phone.
 *
 * The same four questions the desktop page asked — what is in it, can it move,
 * who pays the fee, is it on the rail at all — and the same fourth answer to
 * each of them: **unknown**. `balances.data ?? []` renders a node that did not
 * answer identically to an account with nothing in it, and those are not the
 * same fact. The same substitution silently hid a freeze, because with the
 * query failed `freeze.data?.frozen === true` is false and a frozen account
 * drew as a healthy one.
 *
 * What changed for the phone is the shape, not the honesty. A table of denoms
 * became a hero for the currency being looked at and one line per currency
 * underneath, because on a 390px screen the first question is "how much do I
 * have" and the answer should be the largest thing on the glass.
 */
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { placementVerdict, resolveDenom, t } from '@yamale/chain';

import { client } from './chain.ts';
import { Named } from './Named.tsx';
import { Amount, Note, Unreadable } from './ui.tsx';

/** Pool shares are a position in an AMM, not money somebody spends. */
function spendable(denom: string): boolean {
  return !denom.startsWith('amm/pool/');
}

export function Holdings({ address, own }: { address: string; own: boolean }) {
  const [focus, setFocus] = useState<string | null>(null);

  const balances = useQuery({
    queryKey: ['balances', address],
    queryFn: () => client.balances(address),
  });
  const freeze = useQuery({
    queryKey: ['freeze', address],
    queryFn: () => client.freezeStatus(address),
  });
  const allowances = useQuery({
    queryKey: ['allowances', address],
    queryFn: () => client.feeAllowances(address),
  });
  // Both, because they are two facts that can disagree and the disagreement is
  // itself worth reporting: the chain issues an identifier when it records a
  // country, so a country with no identifier is a fault rather than a state.
  const placement = useQuery({
    queryKey: ['placement', address],
    queryFn: async () => ({
      country: await client.jurisdictionOf(address),
      userId: await client.userIdOf(address),
    }),
  });

  const held = (balances.data ?? []).filter((c) => spendable(c.denom));
  const native = held.find((c) => c.denom === 'uyml');
  const sponsored = (allowances.data ?? []).length > 0;
  const frozen = freeze.data?.frozen === true;

  /**
   * Whether the freeze question was answered at all.
   *
   * `client.freezeStatus` swallows its own fault and resolves with `null`, so
   * the query never enters an error state and the old screen's
   * `freeze.isError` branch could not fire: a node that refused to answer drew
   * as an unfrozen account. `null` is the failure, and it has to be read as one.
   */
  const freezeUnknown = freeze.isError || (freeze.isSuccess && freeze.data === null);

  // YML leads unless the holder tapped something else, because YML is what pays
  // the fee and therefore what decides whether anything else can move at all.
  const primary = held.find((c) => c.denom === (focus ?? 'uyml')) ?? held[0];
  const others = held.filter((c) => c !== primary);

  return (
    <>
      {/* Ordered by consequence. A freeze stops everything, so it is first, and
          it names the case — an account holder refused a transfer needs the
          case number, not the word "frozen". */}
      {frozen && freeze.data?.case && (
        <Note tone="bad" title={t('acct.cannotSend')}>
          {t('acct.frozenBy', { id: freeze.data.case.id })} “{freeze.data.case.reason}”.{' '}
          {t('acct.canStillReceive')}
        </Note>
      )}

      {/* A freeze that could not be read is not the absence of a freeze. It is
          the one query here whose failure is dangerous in the permissive
          direction, so it says so instead of staying quiet. */}
      {freezeUnknown && <Unreadable what={t('acct.freezeUnknown')} error={freeze.error} />}

      {balances.isPending ? (
        <div className="wal-hero wal-hero--waiting" aria-busy="true">
          <span className="wal-hero__label">{t('term.balance')}</span>
          <span className="wal-hero__waiting" role="status">{t('acct.reading')}</span>
        </div>
      ) : balances.isError ? (
        <>
          <div className="wal-hero wal-hero--waiting">
            <span className="wal-hero__label">{t('term.balance')}</span>
            <span className="wal-hero__waiting">{t('wal.notRead')}</span>
          </div>
          <Unreadable what={t('acct.balancesUnknownBody')} error={balances.error} />
          <button type="button" className="wal-btn wal-btn--ghost" onClick={() => balances.refetch()}>
            {t('acct.tryAgain')}
          </button>
        </>
      ) : !primary ? (
        <Note tone="plain" title={t('acct.nothingHere')}>{t('acct.nothingHereBody')}</Note>
      ) : (
        <>
          <div className="wal-hero">
            <span className="wal-hero__label">{resolveDenom(primary.denom).name}</span>
            <Amount amount={primary.amount} denom={primary.denom} size="hero" withSymbol={false} />
            <span className="wal-hero__sym">{resolveDenom(primary.denom).symbol}</span>
          </div>

          {others.length > 0 && (
            <>
              <h2 className="y-screen__sub">{t('wal.alsoHolds')}</h2>
              <ul className="y-cards wal-list">
                {others.map((coin) => {
                  const info = resolveDenom(coin.denom);
                  return (
                    <li key={coin.denom}>
                      <button type="button" className="y-row" onClick={() => setFocus(coin.denom)}>
                        <span className="y-row__main">
                          <span className="y-row__title">{info.symbol}</span>
                          <span className="y-row__sub">{info.name}</span>
                        </span>
                        <span className="y-row__end">
                          <Amount amount={coin.amount} denom={coin.denom} size="row" withSymbol={false} />
                        </span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </>
          )}
        </>
      )}

      {/* Only asserted when both facts were actually read. Saying "no YML and
          nobody sponsoring you" on the strength of two failed queries would be
          inventing a problem that may not exist. */}
      {!frozen && balances.isSuccess && allowances.isSuccess && !native && !sponsored && held.length > 0 && (
        <Note tone="warn" title={t('acct.noYmlTitle')}>{t('acct.noYmlBody')}</Note>
      )}

      {placement.data && placement.data.country === null && (
        <Note tone="warn" title={`${placementVerdict(placement.data).headline}.`}>
          {placementVerdict(placement.data).consequence} {placementVerdict(placement.data).remedy}
        </Note>
      )}
      {placement.data?.country && !placement.data.userId && (
        <Note tone="bad" title={`${placementVerdict(placement.data).headline}.`}>
          {placementVerdict(placement.data).consequence} {placementVerdict(placement.data).remedy}
        </Note>
      )}

      <h2 className="y-screen__sub">{t('acct.fees')}</h2>
      <div className="y-card">
        {allowances.isPending ? (
          <p className="wal-muted">{t('acct.reading')}</p>
        ) : allowances.isError ? (
          <Unreadable what={t('acct.feesUnknown')} error={allowances.error} />
        ) : sponsored ? (
          (allowances.data ?? []).map((a) => (
            <p key={a.granter} className="wal-muted">
              <Named address={a.granter} /> {t('acct.isPaying')}{' '}
              {a.spendLimit.map((c) => (
                <Amount key={c.denom} amount={c.amount} denom={c.denom} />
              ))}
              {a.expiration
                ? ` ${t('acct.until', { date: new Date(a.expiration).toLocaleDateString() })}`
                : ''}
              .
            </p>
          ))
        ) : native ? (
          <p className="wal-muted">{t('acct.paysOwn')}</p>
        ) : (
          <p className="wal-muted">{t('acct.noSponsor')}</p>
        )}
      </div>

      {own && !frozen && !freezeUnknown && freeze.isSuccess && (
        <p className="wal-small wal-muted">{t('wal.notFrozen')}</p>
      )}
    </>
  );
}
