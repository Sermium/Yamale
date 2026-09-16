import { useState } from 'react';
import { useQueries, useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import {
  committed,
  formatAmount,
  formatCoins,
  resolveDenom,
  spendable,
  t,
  type TreasuryBalance,
} from '@yamale/chain';

import { client } from '../chain.ts';
import { Address } from '../Address.tsx';
import { CustodyFigure, Split } from '../Custody.tsx';
import { Folders, Kpi, KpiBand } from '../ui.tsx';
import { Unknown } from '../Unknown.tsx';

type Folder = 'treasuries' | 'custody';

/**
 * Every treasury on the chain, with the two numbers that matter side by side.
 *
 * Available first, committed second, and the total nowhere — because the total
 * is the number that misleads. A treasury holding 100,000 with 90,000 vested to
 * staff has 10,000 to spend, and a list that led with 100,000 would be inviting
 * somebody to propose a payment that cannot clear.
 *
 * Restructured 2026-09-16. The page opened with the custody diagram: an
 * explainer roughly a screen tall, above the data. The argument it makes is the
 * right argument — what separates this from a shared spreadsheet with
 * signatures is that the committed side is enforced by the chain against
 * everybody, including the people who run the treasury — but a reader who came
 * to check a balance had to scroll past a lecture to reach one. It is a folder
 * now: still on the front page, still one click, no longer in the way.
 *
 * What replaces it above the fold is the same claim in figures rather than in
 * prose: a band of counts, and a per-currency table of available against
 * committed across every treasury on the chain. The invariant is structural on
 * the front page instead of asserted on it.
 */
export function TreasuriesPage() {
  const [folder, setFolder] = useState<Folder>('treasuries');
  const treasuries = useQuery({ queryKey: ['treasuries'], queryFn: () => client.treasuries() });

  const list = treasuries.data ?? [];

  // Balances for every treasury, so the front page can state a chain-wide
  // position. useQueries rather than a loop of hooks: the number of treasuries
  // is not known until the list resolves, and a hook count that changes between
  // renders is the one thing React will not tolerate.
  const balanceQueries = useQueries({
    queries: list.map((treasury) => ({
      queryKey: ['treasury-balances', treasury.id],
      queryFn: () => client.treasuryBalances(treasury.id),
    })),
  });

  // Same-denom addition only. `ungn + ungn` is a balance; `ungn + uxof` is an
  // invented exchange rate, and this is the screen where somebody decides what
  // they can spend.
  // Not memoised. The input is a handful of balances and the correct
  // dependency would be every query's result, which is a new array on each
  // poll — a memo keyed on that recomputes anyway, and one keyed on anything
  // cheaper goes stale holding a figure a treasurer acts on.
  const byDenom = sumByDenom(balanceQueries.map((q) => q.data));

  const commitments = useQuery({
    queryKey: ['all-locks', list.map((x) => x.id).join(',')],
    queryFn: async () => {
      const counts = await Promise.all(
        list.map((treasury) => client.treasuryLocks(treasury.id).then((l) => l.length).catch(() => null)),
      );
      // One unreadable treasury makes the total unknowable, not smaller. A
      // count that quietly omitted it would understate what is promised away.
      if (counts.some((c) => c === null)) return null;
      return counts.reduce<number>((sum, c) => sum + (c ?? 0), 0);
    },
    enabled: list.length > 0,
  });

  const read = treasuries.isSuccess;
  const frozen = list.filter((x) => x.paused).length;

  return (
    <>
      <h1>{t('safe.treasuries')}</h1>
      <p className="lede">{t('safe.lede')}</p>

      {/* The figures before the list. Each says where it came from, because a
          count of treasuries is exact and a count of commitments depends on
          every treasury answering. */}
      <KpiBand>
        <Kpi
          label={t('sd.kpi.treasuries')}
          value={read ? list.length : '—'}
          hint={read ? t('sd.kpi.treasuriesHint') : t('sd.kpi.unread')}
          tone={treasuries.isError ? 'bad' : undefined}
        />
        <Kpi
          label={t('sd.kpi.currencies')}
          value={read ? byDenom.length : '—'}
          hint={read ? t('sd.kpi.currenciesHint') : t('sd.kpi.unread')}
          tone={treasuries.isError ? 'bad' : undefined}
        />
        <Kpi
          label={t('sd.kpi.commitments')}
          // null is "a treasury did not answer", which is not zero.
          value={commitments.data ?? '—'}
          hint={
            commitments.data === null || commitments.data === undefined
              ? t('sd.kpi.unread')
              : t('sd.kpi.commitmentsHint')
          }
          tone={commitments.data === null ? 'bad' : 'lock'}
        />
        <Kpi
          label={t('sd.kpi.frozen')}
          value={read ? frozen : '—'}
          hint={read ? t('sd.kpi.frozenHint') : t('sd.kpi.unread')}
          // A frozen treasury is a finding, not decoration. Zero of them is not.
          tone={frozen > 0 ? 'warn' : undefined}
        />
      </KpiBand>

      <Folders
        label={t('safe.treasuries')}
        active={folder}
        onSelect={(id) => setFolder(id as Folder)}
        tabs={[
          { id: 'treasuries', label: t('sd.fold.treasuries'), count: read ? list.length : null },
          { id: 'custody', label: t('sd.fold.custody') },
        ]}
      />

      {folder === 'custody' ? (
        <CustodyFigure />
      ) : (
        <>
          {/* The invariant as a position rather than as a promise: one row per
              currency, available against committed, across the whole chain. */}
          {byDenom.length > 0 && (
            <section className="card">
              <h2>{t('sd.custody.summary')}</h2>
              <p className="small muted">{t('sd.custody.summaryLede')}</p>
              <div className="scroll-x">
                <table className="table">
                  <thead>
                    <tr>
                      <th>{t('sd.col.currency')}</th>
                      <th>{t('sd.col.split')}</th>
                      <th className="num">{t('safe.availableToSpend')}</th>
                      <th className="num">{t('safe.committed')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {byDenom.map((row) => {
                      const info = resolveDenom(row.denom);
                      const total = (row.available + row.locked).toString();
                      return (
                        <tr key={row.denom}>
                          <td>
                            <strong>{info.symbol}</strong>
                            <span className="small muted"> {info.name}</span>
                          </td>
                          <td className="splitcell">
                            <Split
                              total={total}
                              locked={row.locked.toString()}
                              label={t('safe.splitAlt', {
                                available: formatAmount(row.available.toString(), row.denom),
                                committed: formatAmount(row.locked.toString(), row.denom),
                              })}
                            />
                          </td>
                          <td className="num strong">
                            {formatAmount(row.available.toString(), row.denom, {
                              withSymbol: false,
                            })}
                          </td>
                          <td className="num">
                            <span className="num--lock">
                              {formatAmount(row.locked.toString(), row.denom, {
                                withSymbol: false,
                              })}
                            </span>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </section>
          )}

          <h2 className="section">{t('safe.onThisChain')}</h2>

          {/* Waiting, and the two ways of not having an answer, in the order
              they happen. A chain UI spends most of its life in one of these
              three states, and only the third one used to have a design. */}
          {treasuries.isPending && (
            <div className="grid">
              {[0, 1, 2].map((n) => (
                <section className="card" key={n} aria-hidden="true">
                  <div className="skeleton">
                    <i />
                    <i />
                    <i />
                  </div>
                </section>
              ))}
              <p className="small muted" role="status">
                {t('safe.reading')}
              </p>
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
            (list.length === 0 ? (
              <section className="empty">
                <h2>{t('safe.noTreasuries')}</h2>
                <p>{t('safe.noTreasuriesBody')}</p>
              </section>
            ) : (
              <div className="grid">
                {list.map((treasury) => (
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
      )}
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
  // Nothing at all, rendered as nothing at all. A dash here would be the third
  // dash on a card that already says "–" twice, and it would be set in the
  // warning colour the unknown state uses — an empty treasury is not a problem.
  const withAny = balances.filter((b) => b.total !== '0');
  if (withAny.length === 0) return null;

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

/**
 * Chain-wide holdings, added within each currency and never across them.
 *
 * `ungn + ungn` is a balance. `ungn + uxof` is an invented exchange rate, and
 * this is the screen where somebody decides what they can spend, so the shape
 * of the answer is one row per currency rather than one figure.
 *
 * BigInt throughout: a treasury balance routinely exceeds what a double holds
 * exactly, and a total is where a float error compounds instead of cancelling.
 */
function sumByDenom(
  results: (TreasuryBalance[] | undefined)[],
): { denom: string; available: bigint; locked: bigint }[] {
  const totals = new Map<string, { available: bigint; locked: bigint }>();

  for (const balances of results) {
    for (const balance of balances ?? []) {
      const row = totals.get(balance.denom) ?? { available: 0n, locked: 0n };
      try {
        row.available += BigInt(balance.available || '0');
        row.locked += BigInt(balance.locked || '0');
      } catch {
        // A balance the node returned in a shape we cannot parse is dropped
        // rather than guessed at — it would otherwise land in a figure
        // somebody acts on.
        continue;
      }
      totals.set(balance.denom, row);
    }
  }

  return [...totals]
    .map(([denom, sums]) => ({ denom, ...sums }))
    .sort((a, b) => resolveDenom(a.denom).symbol.localeCompare(resolveDenom(b.denom).symbol));
}
