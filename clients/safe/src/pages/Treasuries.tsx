import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import {
  EMPTY_AMOUNT,
  committed,
  formatCoins,
  spendable,
  t,
  type TreasuryBalance,
} from '@yamale/chain';

import { client } from '../chain.ts';
import { Address } from '../Address.tsx';
import { CustodyFigure, Split } from '../Custody.tsx';
import { Unknown } from '../Unknown.tsx';

/**
 * Every treasury on the chain, with the two numbers that matter side by side.
 *
 * Available first, committed second, and the total nowhere — because the total
 * is the number that misleads. A treasury holding 100,000 with 90,000 vested to
 * staff has 10,000 to spend, and a list that led with 100,000 would be inviting
 * somebody to propose a payment that cannot clear.
 *
 * The figure at the top is the application's whole argument, and it is here
 * rather than on a page somebody would have to find. What separates this from
 * a shared spreadsheet with signatures on it is not the approval flow — every
 * product has one — it is that the committed side is enforced by the chain
 * against everybody, including the people who run the treasury. That is not a
 * feature you can put in a footnote.
 */
export function TreasuriesPage() {
  const treasuries = useQuery({ queryKey: ['treasuries'], queryFn: () => client.treasuries() });

  return (
    <>
      <h1>{t('safe.treasuries')}</h1>
      <p className="lede">{t('safe.lede')}</p>

      <CustodyFigure />

      <h2 className="section">{t('safe.onThisChain')}</h2>

      {/* Waiting, and the two ways of not having an answer, in the order they
          happen. A chain UI spends most of its life in one of these three
          states, and only the third one used to have a design. */}
      {treasuries.isPending && (
        <div className="grid">
          {[0, 1, 2].map((n) => (
            <section className="card" key={n} aria-hidden="true">
              <div className="skeleton">
                <i /><i /><i />
              </div>
            </section>
          ))}
          <p className="small muted" role="status">{t('safe.reading')}</p>
        </div>
      )}

      {treasuries.isError && (
        <section className="card">
          <h2>{t('safe.cannotReach')}</h2>
          <Unknown
            what={t('safe.listUnknown')}
            error={treasuries.error}
            onRetry={() => treasuries.refetch()}
          />
        </section>
      )}

      {treasuries.isSuccess &&
        ((treasuries.data ?? []).length === 0 ? (
          <section className="empty">
            <h2>{t('safe.noTreasuries')}</h2>
            <p>{t('safe.noTreasuriesBody')}</p>
          </section>
        ) : (
          <div className="grid">
            {treasuries.data.map((treasury) => (
              <TreasuryCard
                key={treasury.id}
                id={treasury.id}
                name={treasury.name}
                admin={treasury.admin}
                paused={treasury.paused}
              />
            ))}
          </div>
        ))}
    </>
  );
}

function TreasuryCard({
  id,
  name,
  admin,
  paused,
}: {
  id: string;
  name: string;
  admin: string;
  paused: boolean;
}) {
  const balances = useQuery({
    queryKey: ['treasury-balances', id],
    queryFn: () => client.treasuryBalances(id),
  });

  const list: TreasuryBalance[] = balances.data ?? [];

  return (
    <section className="card">
      <div className="card__head">
        <h2>
          <Link to={`/treasury/${id}`}>{name || t('safe.treasuryN', { id })}</Link>
        </h2>
        {/* The word as well as the colour. A frozen treasury read as an amber
            dot is a frozen treasury nobody noticed. */}
        {paused && <span className="badge badge-paused">{t('safe.paused')}</span>}
      </div>

      {balances.isError ? (
        // Not "–". A dash beside "available to spend" reads as a treasury with
        // nothing in it, and this one may be full.
        <p className="card__unknown">{t('safe.balancesUnknown')}</p>
      ) : (
        <>
          {balances.isSuccess && <TotalsBar balances={list} />}
          <dl className="figures">
            <div>
              <dt>{t('safe.availableToSpend')}</dt>
              <dd className="figure">
                {balances.isPending ? <span className="muted">…</span> : formatCoins(spendable(list))}
              </dd>
            </div>
            <div>
              <dt>{t('safe.committed')}</dt>
              <dd className="figure figure--lock">
                {balances.isPending ? <span className="muted">…</span> : formatCoins(committed(list))}
              </dd>
            </div>
          </dl>
        </>
      )}

      <p className="small muted">
        {t('safe.admin')} <Address address={admin} />
      </p>
    </section>
  );
}

/**
 * One bar per currency the treasury holds, stacked.
 *
 * Not one bar for the treasury: adding a naira balance to a rand balance
 * produces a number that means nothing, and a proportion computed across
 * currencies would be an invented exchange rate on the screen where somebody
 * decides what they can spend.
 */
function TotalsBar({ balances }: { balances: TreasuryBalance[] }) {
  const withAny = balances.filter((b) => b.total !== '0');
  if (withAny.length === 0) return <p className="card__unknown">{EMPTY_AMOUNT}</p>;

  return (
    <div className="splits">
      {withAny.slice(0, 4).map((b) => (
        <Split
          key={b.denom}
          total={b.total}
          locked={b.locked}
          label={t('safe.splitAlt', { available: b.available, committed: b.locked })}
        />
      ))}
      {withAny.length > 4 && (
        <p className="small muted">{t('safe.andMore', { n: String(withAny.length - 4) })}</p>
      )}
    </div>
  );
}
