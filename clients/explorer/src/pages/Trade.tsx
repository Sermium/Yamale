import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  comparePoolToRates,
  formatAmount,
  formatDecimal,
  formatPercent,
  minimumReceived,
  quoteSwap,
  resolveDenom,
  type Pool,
  type Rate, t,} from '@yamale/chain';

import { client, useViewMode } from '../chain.ts';
import { Card, Empty, ErrorState, Loading, Stat } from '../components.tsx';

/** Slippage tolerances, as basis points. */
const TOLERANCES = [
  { bps: 50, label: '0.5%' },
  { bps: 100, label: '1%' },
  { bps: 500, label: '5%' },
];

/**
 * Trading answers "what will I get, and what could go wrong?"
 *
 * The quote is computed with the chain's own formula and its rounding
 * direction, so the number shown is the number the chain would produce. A
 * client that rounded the other way would quote an amount the chain then
 * refuses, which a user experiences as the trade failing at random.
 */
export function TradePage() {
  const { mode } = useViewMode();
  const pools = useQuery({ queryKey: ['pools'], queryFn: () => client.pools(), refetchInterval: 10_000 });
  const rates = useQuery({ queryKey: ['rates'], queryFn: () => client.exchangeRates(), refetchInterval: 30_000 });

  if (pools.isPending) return <Loading label="Fetching pools" />;
  if (pools.isError) return <ErrorState error={pools.error} what="pools" />;

  const list = pools.data ?? [];
  const byDenom = new Map((rates.data ?? []).map((r) => [r.denom, r]));

  return (
    <>
      <h1>{t('exp.trade')}</h1>
      <p className="lede">
        Swap one currency for another against a liquidity pool. The price comes from the pool's own
        reserves, so larger trades move it against you.
      </p>

      {list.length === 0 ? (
        <Card>
          <Empty
            title="No pools yet"
            hint="Nobody has opened a liquidity pool on this chain. Until one exists there is nothing to trade against."
          />
        </Card>
      ) : (
        list.map((pool) => (
          <PoolCard
            key={pool.id}
            pool={pool}
            expert={mode === 'expert'}
            rateA={byDenom.get(pool.denomA)}
            rateB={byDenom.get(pool.denomB)}
          />
        ))
      )}
    </>
  );
}

function PoolCard({
  pool,
  expert,
  rateA,
  rateB,
}: {
  pool: Pool;
  expert: boolean;
  rateA?: Rate;
  rateB?: Rate;
}) {
  const a = resolveDenom(pool.denomA);
  const b = resolveDenom(pool.denomB);
  const comparison = comparePoolToRates(pool.price, rateA, rateB);

  return (
    <Card
      title={
        <>
          {a.symbol} / {b.symbol}
          {expert ? <span className="faint mono small"> · pool #{pool.id}</span> : null}
        </>
      }
      actions={<span className="badge">{formatPercent(pool.feeRate, 2)} fee</span>}
    >
      <div className="stats" style={{ marginBottom: '1rem' }}>
        <Stat
          label="Price"
          value={pool.price === null ? '—' : formatDecimal(pool.price)}
          note={`${b.symbol} per ${a.symbol}`}
        />
        <Stat label={`${a.symbol} in the pool`} value={formatAmount(pool.reserveA, pool.denomA, { maxDecimals: 2 })} />
        <Stat label={`${b.symbol} in the pool`} value={formatAmount(pool.reserveB, pool.denomB, { maxDecimals: 2 })} />
      </div>

      {comparison?.notable ? (
        <div className="notice" style={{ marginBottom: '1rem' }}>
          <strong>
            This pool prices {a.symbol} {comparison.divergence > 0 ? 'above' : 'below'} the rate validators
            agreed, by {formatPercent(Math.abs(comparison.divergence), 1)}.
          </strong>{' '}
          {comparison.stale
            ? 'The agreed rate is itself out of date, so treat the comparison with caution.'
            : `Either the pool is thin and this price will not hold, or it has drifted and will be corrected. Validators put ${a.symbol} at ${formatDecimal(comparison.fairPrice)} ${b.symbol}.`}
        </div>
      ) : null}

      <SwapCalculator pool={pool} expert={expert} />
    </Card>
  );
}

/**
 * A quote, not a trade. The explorer does not hold keys and does not sign;
 * showing what a swap would return is useful on its own, and pretending to
 * offer execution we cannot deliver would be worse than not offering it.
 *
 * It still shows the minimum received, because that is the figure somebody has
 * to put into the swap they sign elsewhere, and quoting only the expected
 * amount teaches people to sign without a floor.
 */
function SwapCalculator({ pool, expert }: { pool: Pool; expert: boolean }) {
  const a = resolveDenom(pool.denomA);
  const b = resolveDenom(pool.denomB);

  const [amount, setAmount] = useState('');
  const [forward, setForward] = useState(true);
  const [toleranceBps, setToleranceBps] = useState(100);

  const denomIn = forward ? pool.denomA : pool.denomB;
  const denomOut = forward ? pool.denomB : pool.denomA;
  const infoIn = forward ? a : b;

  // Typed in display units; the chain works in base units.
  const base = toBaseUnits(amount, infoIn.exponent);
  const quote = base ? quoteSwap(pool, base, denomIn) : null;
  const floor = quote ? minimumReceived(quote.amountOut, toleranceBps) : null;

  return (
    <div>
      <div className="inline" style={{ marginBottom: '0.6rem' }}>
        <label htmlFor={`amt-${pool.id}`} className="small muted">
          If I swap
        </label>
        <input
          id={`amt-${pool.id}`}
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          placeholder="0.00"
          inputMode="decimal"
          style={{
            width: 130,
            padding: '0.35rem 0.6rem',
            font: 'inherit',
            background: 'var(--bg)',
            color: 'var(--text)',
            border: '1px solid var(--border-strong)',
            borderRadius: 'var(--radius-sm)',
          }}
        />
        <strong>{infoIn.symbol}</strong>
        <button
          type="button"
          className="badge"
          onClick={() => setForward((f) => !f)}
          style={{ cursor: 'pointer' }}
          title="Swap the direction"
        >
          ⇄ other way
        </button>
      </div>

      {quote === null || floor === null ? (
        <p className="small faint" style={{ margin: 0 }}>
          Enter an amount to see what it would return.
        </p>
      ) : (
        <>
          <p style={{ margin: '0 0 0.3rem' }}>
            You would receive about <strong>{formatAmount(quote.amountOut, denomOut, { maxDecimals: 6 })}</strong>
          </p>

          <div className="inline small" style={{ gap: '0.5rem', marginBottom: '0.4rem', flexWrap: 'wrap' }}>
            <span className="muted">At worst, if the price moves by</span>
            {TOLERANCES.map((t) => (
              <button
                key={t.bps}
                type="button"
                className={t.bps === toleranceBps ? 'badge badge--warn' : 'badge'}
                onClick={() => setToleranceBps(t.bps)}
                style={{ cursor: 'pointer' }}
                aria-pressed={t.bps === toleranceBps}
              >
                {t.label}
              </button>
            ))}
            <span className="muted">
              you get at least <strong>{formatAmount(floor, denomOut, { maxDecimals: 6 })}</strong>
            </span>
          </div>

          {quote.priceImpact > 0.01 ? (
            <div className={quote.priceImpact > 0.05 ? 'notice notice--bad' : 'notice'} style={{ marginTop: '0.5rem' }}>
              <strong>This trade moves the price by {formatPercent(quote.priceImpact, 1)}.</strong> It is large
              relative to the pool, so you get a materially worse rate than the quoted price. A smaller trade
              would get a better one.
            </div>
          ) : (
            <p className="small faint" style={{ margin: 0 }}>
              Price impact {formatPercent(quote.priceImpact, 2)} · fee {formatPercent(pool.feeRate, 2)} kept by
              liquidity providers
            </p>
          )}

          {expert ? (
            <p className="small faint mono" style={{ marginTop: '0.4rem' }}>
              {base} {denomIn} → {quote.amountOut} {denomOut} · min_amount_out {floor}
            </p>
          ) : null}
        </>
      )}
    </div>
  );
}

/** Converts a typed display amount into base units, or null if unusable. */
function toBaseUnits(value: string, exponent: number): string | null {
  const trimmed = value.trim();
  if (!trimmed || !/^\d*\.?\d*$/.test(trimmed)) return null;

  const [whole = '0', fraction = ''] = trimmed.split('.');
  const padded = (fraction + '0'.repeat(exponent)).slice(0, exponent);
  const combined = `${whole}${padded}`.replace(/^0+(?=\d)/, '');
  if (!combined || combined === '0') return null;
  return combined;
}
