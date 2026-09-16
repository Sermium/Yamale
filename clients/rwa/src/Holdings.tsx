/**
 * What one address owns across every vehicle.
 *
 * Found from the account's balances rather than from a query, because there is
 * no "positions by holder" read on x/tokenisation: the positions map is keyed
 * (asset, holder), so listing one holder's vehicles would be a scan of every
 * asset. The shareholding is an ordinary bank balance in a `tok/<id>/<SYMBOL>`
 * denom, so the wallet already knows the answer and one balances call finds it.
 *
 * The entitlement is then read per vehicle from Query/Entitlement, which is the
 * only correct source: it is not derivable from a balance, and a screen that
 * multiplied a balance by an index would be wrong for anybody who had bought or
 * sold since the last distribution.
 */
import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { formatAmount, fractionDenomParts, t } from '@yamale/chain';

import { entitlement, position as fetchPosition, supplyOf, vehicle as fetchVehicle } from './chain.ts';
import { useBalances } from './data.ts';
import { addressOf, type Account } from './address.ts';
import { costOf, int, returnOn, type Cost, type ReturnView } from './position.ts';
import { Amount, Chip, Empty, Loading, Percent, Unreachable, Kpi, KpiBand} from './ui.tsx';
import {
  bpsToPercent,
  shareholding,
  statusKey,
  statusTone,
  type Coin,
  type VehicleStatus,
  descriptorOf,
} from './vehicle.ts';

interface Row {
  assetId: string;
  symbol: string;
  denom: string;
  balance: string;
  supply: string | null;
  holderShareBps: number;
  status: VehicleStatus;
  owed: Coin | null;
  owedUnknown: boolean;
  /** The readable name, from the asset's uri. Null when it carries none. */
  name: string | null;
  /** Whether a document manifest is recorded against it. */
  hasPapers: boolean;
  /** Everything ever paid into its vault, summed across denominations. */
  funded: string;
  /** The denomination that vault pays in. */
  incomeDenom: string;
  /** What this holding cost, split into the covered and uncovered parts. */
  cost: Cost;
  /** What the covered part has returned. Null figures carry their own reason. */
  ret: ReturnView;
  /** Set when the position could not be read, so "not recorded" is not implied. */
  costUnknown: boolean;
}

function useHoldingRows(address: string): { loading: boolean; rows: Row[] } {
  const [balances] = useBalances(address);
  const [rows, setRows] = useState<Row[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let live = true;
    if (balances.loading) { setLoading(true); return; }
    if (!balances.outcome.ok) { setLoading(false); setRows([]); return; }

    const shares = balances.outcome.value
      .map((c) => ({ coin: c, parts: fractionDenomParts(c.denom) }))
      .filter((x): x is { coin: Coin; parts: { assetId: string; symbol: string } } =>
        x.parts !== null);

    if (shares.length === 0) { setLoading(false); setRows([]); return; }

    setLoading(true);
    Promise.all(shares.map(async ({ coin, parts }) => {
      const [record, supply, owed, position] = await Promise.all([
        fetchVehicle(parts.assetId),
        supplyOf(coin.denom),
        entitlement(parts.assetId, address),
        fetchPosition(parts.assetId, address),
      ]);

      const desc = descriptorOf(record.ok ? record.value.asset.uri : '');
      const vault = record.ok ? record.value.vault : null;

      const cost = costOf(position.ok ? position.value : null, coin.amount);
      // owed is passed as null when the query failed, so a network error reads
      // as "not known" rather than as a return of zero.
      const ret = returnOn(cost, owed.ok ? owed.value.amount : null, coin.amount);

      return {
        assetId: parts.assetId,
        symbol: parts.symbol,
        denom: coin.denom,
        balance: coin.amount,
        supply,
        holderShareBps: record.ok ? record.value.asset.holderShareBps : 0,
        status: record.ok ? record.value.asset.status : 'STATUS_UNSPECIFIED',
        owed: owed.ok ? owed.value : null,
        name: desc.name,
        hasPapers: desc.docsDigest !== null,
        funded: vault
          ? vault.funded.reduce((sum, c) => sum + BigInt(c.amount || '0'), 0n).toString()
          : '0',
        incomeDenom: vault?.denom ?? '',
        // A failed read is carried rather than flattened to zero: telling a
        // holder they are owed nothing because a query timed out is the truth
        // about the network and a lie about their money.
        owedUnknown: !owed.ok && owed.reason === 'unreachable',
        cost,
        ret,
        costUnknown: !position.ok,
      } satisfies Row;
    })).then((out) => {
      if (!live) return;
      // Newest vehicle first. Ids are uint64 and exceed what a double holds
      // exactly, so the comparison is on BigInt rather than on Number.
      out.sort((x, y) => (BigInt(y.assetId) > BigInt(x.assetId) ? 1 : -1));
      setRows(out);
      setLoading(false);
    });

    return () => { live = false; };
  }, [balances, address]);

  return { loading, rows };
}

export function Holdings({ account }: { account: Account }) {
  const address = addressOf(account);
  const { loading, rows } = useHoldingRows(address);
  const [balances, reload] = useBalances(address);

  if (account.mode === 'none') {
    return (
      <div className="page">
        <Empty title={t('rwa.holdNoAccountTitle')}>
          <p>{t('rwa.holdNoAccountBody')}</p>
        </Empty>
      </div>
    );
  }

  if (!balances.loading && !balances.outcome.ok) {
    return <div className="page"><Unreachable onRetry={reload} /></div>;
  }

  if (loading) return <div className="page"><Loading what={t('rwa.reading')} /></div>;

  // Claimable across the portfolio, SUBTOTALLED PER CURRENCY.
  //
  // Not one number. Each tokenisation pays in one currency and a portfolio may
  // span several, so a single total would be dollars added to francs: a figure
  // true of nothing. The chain does carry exchange rates, and converting would
  // produce one number -- at the cost of making today's claimable depend on
  // today's rate, which is a different and less checkable claim than "you may
  // withdraw this much of each".
  //
  // An earlier version refused to total a mixed portfolio and printed an
  // explanation instead. That answered a bad question. A subtotal per currency
  // is simply correct, and in the ordinary case there is exactly one of them.
  const byDenom = new Map<string, bigint>();
  let anyUnknown = false;
  for (const r of rows) {
    if (r.owedUnknown) { anyUnknown = true; continue; }
    const denom = r.owed?.denom || r.incomeDenom;
    if (!denom) continue;
    byDenom.set(denom, (byDenom.get(denom) ?? 0n) + BigInt(r.owed?.amount || '0'));
  }
  const claimable = [...byDenom.entries()]
    .map(([denom, amount]) => ({ denom, amount: amount.toString() }))
    .sort((x, y) => (BigInt(y.amount) > BigInt(x.amount) ? 1 : -1));

  // The same, for what the vaults have already paid on this holding.
  const paidByDenom = new Map<string, bigint>();
  for (const r of rows) {
    const supply = BigInt(r.supply || '0');
    if (supply === 0n || !r.incomeDenom) continue;
    const share = (BigInt(r.balance || '0') * BigInt(r.funded || '0')) / supply;
    paidByDenom.set(r.incomeDenom, (paidByDenom.get(r.incomeDenom) ?? 0n) + share);
  }
  const attributed = [...paidByDenom.entries()]
    .map(([denom, amount]) => ({ denom, amount: amount.toString() }))
    .sort((x, y) => (BigInt(y.amount) > BigInt(x.amount) ? 1 : -1));

  // Invested and returned, PER CURRENCY and over the COVERED part only.
  //
  // Per currency for the reason the claimable subtotal is: a portfolio spanning
  // two currencies has no single total, and one would be a figure true of
  // nothing. Over the covered part only because the whole rule of this feature
  // is that a known cost is never averaged with an unknown one -- a holding
  // whose basis covers half its tokens contributes that half's cost and that
  // half's income, and its other half contributes to neither.
  //
  // A row whose position could not be read contributes nothing and is counted
  // in `costUnread` instead, so a timed-out query cannot quietly deflate the
  // denominator and inflate the portfolio's return.
  const investedByDenom = new Map<string, bigint>();
  const returnedByDenom = new Map<string, bigint>();
  let noCostRecorded = 0;
  let costUnread = 0;
  for (const r of rows) {
    if (r.costUnknown) { costUnread++; continue; }
    if (r.cost.state === 'none') { noCostRecorded++; continue; }
    const denom = r.owed?.denom || r.incomeDenom;
    if (!denom) continue;
    investedByDenom.set(denom, (investedByDenom.get(denom) ?? 0n) + int(r.cost.basis));
    returnedByDenom.set(denom, (returnedByDenom.get(denom) ?? 0n) + int(r.ret.income));
  }
  const invested = [...investedByDenom.entries()]
    .map(([denom, amount]) => ({ denom, amount: amount.toString() }))
    .sort((x, y) => (BigInt(y.amount) > BigInt(x.amount) ? 1 : -1));

  // One percentage per currency. A ratio taken across currencies would divide
  // francs by dollars, which is not a smaller number than the right one -- it
  // is not a number at all.
  const returns = [...investedByDenom.entries()]
    .filter(([, basis]) => basis > 0n)
    .map(([denom, basis]) => ({
      denom,
      percent: Number(((returnedByDenom.get(denom) ?? 0n) * 1_000_000n) / basis) / 1_000_000 * 100,
    }))
    .sort((x, y) => y.percent - x.percent);

  // Concentration of the investor's OWN portfolio: the largest single position
  // as a share of everything they hold, measured in the income each throws off
  // rather than in token counts, which are not comparable across tokenisations.
  const weights = rows.map((r) => {
    const supply = BigInt(r.supply || '0');
    if (supply === 0n) return 0n;
    return (BigInt(r.balance || '0') * BigInt(r.funded || '0')) / supply;
  });
  const weightTotal = weights.reduce((a3, b) => a3 + b, 0n);
  const largestPosition = weightTotal > 0n
    ? Number((weights.reduce((a3, b) => (b > a3 ? b : a3), 0n) * 10000n) / weightTotal) / 100
    : null;

  return (
    <div className="page">
      <header className="page__head">
        <p className="y-eyebrow">{t('rwa.holdEyebrow')}</p>
        <h1 className="page__title">{t('rwa.navHoldings')}</h1>
        <p className="page__lede">{t('rwa.holdLede')}</p>
      </header>

      {/* The portfolio, before the table of it. Every figure is read from the
          chain.

          A RETURN now appears here, and until today it could not: the chain
          recorded what an account held and never what it paid, so any
          percentage would have been computed against an assumed entry price.
          x/tokenisation's Position carries `cost_basis` and `cost_qty`, so the
          denominator is now a fact somebody stated on the chain rather than one
          this page invented.

          What is still NOT shown is a yield or a valuation. Nothing on this
          chain knows what a token is worth until a sale is reported, so a
          portfolio "value" would be the one invented number on the page. */}
      {rows.length > 0 && (
        <KpiBand>
          <Kpi
            label={t('rwa.pfHeld')}
            value={String(rows.length)}
            hint={t('rwa.pfHeldHint')}
          />
          <Kpi
            label={t('rwa.pfClaimable')}
            value={claimable.length === 0
              ? <Amount amount="0" denom="" />
              : <span className="kpi__stack">
                  {claimable.map((c) => (
                    <span key={c.denom}><Amount amount={c.amount} denom={c.denom} /></span>
                  ))}
                </span>}
            tone={claimable.some((c) => BigInt(c.amount) > 0n) ? 'ok' : undefined}
            hint={anyUnknown ? t('rwa.pfClaimablePartial') : t('rwa.pfClaimableHint')}
          />
          <Kpi
            label={t('rwa.pfInvested')}
            value={invested.length === 0
              ? t('rwa.cost.notRecordedShort')
              : <span className="kpi__stack">
                  {invested.map((c) => (
                    <span key={c.denom}><Amount amount={c.amount} denom={c.denom} /></span>
                  ))}
                </span>}
            hint={noCostRecorded > 0
              ? t('rwa.pfInvestedPartial', { n: noCostRecorded })
              : t('rwa.pfInvestedHint')}
          />
          <Kpi
            label={t('rwa.pfReturn')}
            value={returns.length === 0
              ? '—'
              : <span className="kpi__stack">
                  {returns.map((r) => (
                    <span key={r.denom}><Percent value={r.percent} places={2} /></span>
                  ))}
                </span>}
            tone={returns.some((r) => r.percent > 0) ? 'ok' : undefined}
            hint={costUnread > 0
              ? t('rwa.pfReturnUnread', { n: costUnread })
              : t('rwa.pfReturnHint')}
          />
          <Kpi
            label={t('rwa.pfAttributed')}
            value={attributed.length === 0
              ? '—'
              : <span className="kpi__stack">
                  {attributed.map((c) => (
                    <span key={c.denom}><Amount amount={c.amount} denom={c.denom} /></span>
                  ))}
                </span>}
            hint={t('rwa.pfAttributedHint')}
          />
          <Kpi
            label={t('rwa.pfLargest')}
            value={<Percent value={largestPosition} places={1} />}
            tone={largestPosition !== null && largestPosition >= 50 ? 'warn' : undefined}
            hint={t('rwa.pfLargestHint')}
          />
        </KpiBand>
      )}

      {rows.length === 0 ? (
        <Empty title={t('rwa.holdEmptyTitle')}>
          <p>{t('rwa.holdEmptyBody')}</p>
          <p><Link to="/" className="linkish">{t('rwa.backToList')}</Link></p>
        </Empty>
      ) : (
        <div className="y-scroll">
          <table className="table table--holdings">
            <thead>
              <tr>
                <th scope="col">{t('rwa.vehicle')}</th>
                <th scope="col">{t('rwa.status')}</th>
                <th scope="col" className="num">{t('rwa.youHold')}</th>
                <th scope="col" className="num">{t('rwa.ofShares')}</th>
                <th scope="col" className="num">{t('rwa.ofAsset')}</th>
                <th scope="col" className="num">{t('rwa.claimableNow')}</th>
                <th scope="col" className="num">{t('rwa.cost.paidShort')}</th>
                <th scope="col" className="num">{t('rwa.cost.returnShort')}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => {
                const share = shareholding(r.balance, r.supply ?? '0', r.holderShareBps);
                return (
                  <tr key={r.denom}>
                    <th scope="row">
                      <Link to={`/v/${r.assetId}`} className="linkish">
                        {r.name ?? t('rwa.vehicleN', { id: r.assetId })}
                      </Link>
                      {r.hasPapers && (
                        <span className="holdings__aside">{t('rwa.pfHasPapers')}</span>
                      )}
                      <span className="y-mono holdings__sym">{r.symbol}</span>
                    </th>
                    <td><Chip tone={statusTone(r.status)}>{t(statusKey(r.status))}</Chip></td>
                    <td className="num"><Amount amount={r.balance} denom={r.denom} /></td>
                    <td className="num"><Percent value={share.ofSupply} places={3} /></td>
                    <td className="num">
                      <Percent value={share.ofAsset} places={3} />
                      <span className="holdings__aside">
                        {t('rwa.tokensCarryShort', { percent: bpsToPercent(r.holderShareBps) })}
                      </span>
                    </td>
                    <td className="num">
                      {r.owedUnknown
                        ? <span className="muted">{t('rwa.owedUnknown')}</span>
                        : <Amount amount={r.owed?.amount ?? null} denom={r.owed?.denom ?? ''} />}
                    </td>
                    {/* Cost and return, side by side and never merged into one
                        "profit" column. A holding with no recorded cost says so
                        in the cost cell and shows nothing in the return cell —
                        a dash in both would read as a return of zero on a cost
                        of zero, which is the one reading that must not happen. */}
                    <td className="num">
                      {r.costUnknown
                        ? <span className="muted">{t('rwa.cost.unreadableShort')}</span>
                        : r.cost.state === 'none'
                          ? <span className="muted">{t('rwa.cost.notRecordedShort')}</span>
                          : (
                            <>
                              <Amount amount={r.cost.basis} denom={r.incomeDenom} />
                              {r.cost.state === 'partial' && (
                                <span className="holdings__aside">
                                  {t('rwa.cost.partialShort', {
                                    tokens: formatAmount(r.cost.uncovered, r.denom),
                                  })}
                                </span>
                              )}
                            </>
                          )}
                    </td>
                    <td className="num">
                      {r.ret.percent === null
                        ? <span className="muted">
                            {r.ret.whyKey ? t(r.ret.whyKey) : t('rwa.cost.notRecordedShort')}
                          </span>
                        : <Percent value={r.ret.percent} places={2} />}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <p className="page__foot muted">{t('rwa.holdFoot')}</p>
    </div>
  );
}

