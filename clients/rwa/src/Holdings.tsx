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
import { fractionDenomParts, t } from '@yamale/chain';

import { entitlement, supplyOf, vehicle as fetchVehicle } from './chain.ts';
import { useBalances } from './data.ts';
import { addressOf, type Account } from './address.ts';
import { Amount, Chip, Empty, Loading, Percent, Unreachable } from './ui.tsx';
import {
  bpsToPercent,
  shareholding,
  statusKey,
  statusTone,
  type Coin,
  type VehicleStatus,
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
      const [record, supply, owed] = await Promise.all([
        fetchVehicle(parts.assetId),
        supplyOf(coin.denom),
        entitlement(parts.assetId, address),
      ]);

      return {
        assetId: parts.assetId,
        symbol: parts.symbol,
        denom: coin.denom,
        balance: coin.amount,
        supply,
        holderShareBps: record.ok ? record.value.asset.holderShareBps : 0,
        status: record.ok ? record.value.asset.status : 'STATUS_UNSPECIFIED',
        owed: owed.ok ? owed.value : null,
        // A failed read is carried rather than flattened to zero: telling a
        // holder they are owed nothing because a query timed out is the truth
        // about the network and a lie about their money.
        owedUnknown: !owed.ok && owed.reason === 'unreachable',
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

  return (
    <div className="page">
      <header className="page__head">
        <p className="y-eyebrow">{t('rwa.holdEyebrow')}</p>
        <h1 className="page__title">{t('rwa.navHoldings')}</h1>
        <p className="page__lede">{t('rwa.holdLede')}</p>
      </header>

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
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => {
                const share = shareholding(r.balance, r.supply ?? '0', r.holderShareBps);
                return (
                  <tr key={r.denom}>
                    <th scope="row">
                      <Link to={`/v/${r.assetId}`} className="linkish">
                        {t('rwa.vehicleN', { id: r.assetId })}
                      </Link>
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

