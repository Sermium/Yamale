import { useQuery } from '@tanstack/react-query';
import {
  describeFreshness,
  formatDecimal,
  formatDuration,
  formatPercent,
  resolveDenom,
  timeAgo,
  truncateAddress,
  type Rate, t,} from '@yamale/chain';

import { client, useViewMode } from '../chain.ts';
import { Card, Empty, ErrorState, Loading, Meter, RawJson } from '../components.tsx';

/**
 * Prices answer "what is this worth, and can I rely on that?"
 *
 * The second half is the part interfaces usually drop. A price with no age on it
 * looks equally trustworthy whether it was agreed a minute ago or has been
 * frozen since the feed stopped, and the difference is exactly when somebody
 * gets hurt. So every figure here carries how old it is and how much of the
 * validator set stood behind it, and a rate the chain would refuse to act on is
 * shown as refused rather than quietly rendered as a number.
 */
export function PricesPage() {
  const { mode } = useViewMode();
  const expert = mode === 'expert';

  const rates = useQuery({ queryKey: ['rates'], queryFn: () => client.exchangeRates(), refetchInterval: 15_000 });
  const params = useQuery({ queryKey: ['oracle-params'], queryFn: () => client.oracleParams() });
  const misses = useQuery({ queryKey: ['oracle-misses'], queryFn: () => client.missCounters(), refetchInterval: 60_000 });
  const names = useQuery({ queryKey: ['validator-names'], queryFn: () => client.validatorNames() });
  const appraisers = useQuery({ queryKey: ['appraisers'], queryFn: () => client.appraisers() });

  if (rates.isPending) return <Loading label="Fetching prices" />;
  if (rates.isError) return <ErrorState error={rates.error} what="prices" />;

  const list = rates.data ?? [];
  const quote = params.data?.quoteSymbol ?? 'USD';

  return (
    <>
      <h1>{t('exp.prices')}</h1>
      <p className="lede">
        Validators each report what a currency is worth and the chain takes the middle of those reports,
        weighted by how much each has staked. Moving that number takes half the network, not one participant.
      </p>

      <Card title={`Agreed rates${list.length ? ` (${list.length})` : ''}`} flush>
        {list.length === 0 ? (
          <Empty
            title="No prices agreed yet"
            hint="Validators have not reported any prices, or too few reported for the chain to agree one. Until then nothing here has a value."
          />
        ) : (
          <ul className="rows">
            {list.map((rate) => (
              <RateRow key={rate.denom} rate={rate} quote={quote} expert={expert} />
            ))}
          </ul>
        )}
      </Card>

      {params.data ? (
        <Card>
          <p className="small muted" style={{ margin: 0 }}>
            A new price is agreed every <strong>{params.data.votePeriod}</strong> blocks, and one older than{' '}
            <strong>{formatDuration(params.data.maxRateAgeSeconds)}</strong> stops being usable — the chain
            refuses to lend or liquidate against it rather than treating a stopped feed as a steady price.
            Everything is quoted in <strong>{quote}</strong>.
          </p>
        </Card>
      ) : null}

      <Reliability
        counters={misses.data ?? []}
        names={names.data ?? {}}
        expert={expert}
      />

      <Valuers list={appraisers.data ?? []} expert={expert} />
    </>
  );
}

function RateRow({ rate, quote, expert }: { rate: Rate; quote: string; expert: boolean }) {
  const info = resolveDenom(rate.denom);
  const badge =
    rate.freshness === 'stale' ? 'badge badge--bad' : rate.freshness === 'ageing' ? 'badge badge--warn' : 'badge badge--ok';

  return (
    <li className="row">
      <div className="row__main">
        <div className="spread" style={{ gap: '0.6rem' }}>
          <strong>
            {/* Quoting the quote currency against itself reads as a mistake even
                though the chain genuinely agrees a rate for it, so it is named
                rather than restated. */}
            {info.symbol === quote ? (
              <>
                {info.symbol} — the currency everything else is priced in
              </>
            ) : (
              <>
                1 {info.symbol} = {formatDecimal(rate.value)} {quote}
              </>
            )}
          </strong>
          <span className={badge}>
            <span aria-hidden="true">{rate.freshness === 'stale' ? '✕' : rate.freshness === 'ageing' ? '!' : '✓'}</span>
            {rate.freshness === 'stale' ? 'Not usable' : rate.freshness === 'ageing' ? 'Ageing' : 'Current'}
          </span>
        </div>

        <p className="small muted" style={{ margin: '0.25rem 0 0' }}>
          {describeFreshness(rate, (seconds) => timeAgo(new Date(Date.now() - seconds * 1000)))}
        </p>

        {rate.thinlyAgreed ? (
          <p className="small" style={{ margin: '0.25rem 0 0' }}>
            Fewer than two thirds of validators contributed to this price. It is valid, but a broader
            agreement would be a stronger claim.
          </p>
        ) : null}

        <Meter
          value={rate.votingPower}
          label={`Share of stake that agreed the ${info.symbol} price`}
          tone={rate.votingPower >= 0.67 ? 'good' : 'neutral'}
          caption={`${formatPercent(rate.votingPower, 0)} of staked tokens agreed this price`}
        />

        {expert ? (
          <p className="small faint mono" style={{ marginTop: '0.4rem' }}>
            {rate.denom} · {rate.rate} · height {rate.updatedHeight} · {rate.ageSeconds}s old ·{' '}
            {rate.votingPower * 10000} bps
          </p>
        ) : null}
      </div>
    </li>
  );
}

/**
 * How reliably each validator reports.
 *
 * Shown because absence is the failure mode that matters here and nothing
 * punishes it automatically: the chain records who did not report, and the
 * consequence is meant to be that people can see it.
 */
function Reliability({
  counters,
  names,
  expert,
}: {
  counters: Array<{ validator: string; misses: number; windows: number }>;
  names: Record<string, string>;
  expert: boolean;
}) {
  if (counters.length === 0) return null;

  const rows = counters
    .map((c) => ({ ...c, rate: c.windows > 0 ? 1 - c.misses / c.windows : 0 }))
    .sort((a, b) => a.rate - b.rate);

  return (
    <Card title="Who is reporting" flush>
      <div className="scroll-x">
        <table className="grid">
          <thead>
            <tr>
              <th scope="col">{t('col.validator')}</th>
              <th scope="col">{t('col.reportedIn')}</th>
              <th scope="col">{t('col.missed')}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.validator}>
                <td>{names[row.validator] ?? <span className="mono">{truncateAddress(row.validator)}</span>}</td>
                <td>
                  {formatPercent(row.rate, 0)}
                  <span className="faint small">
                    {' '}
                    ({row.windows - row.misses} of {row.windows})
                  </span>
                </td>
                <td className={row.misses > 0 ? undefined : 'faint'}>{row.misses}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {expert ? <RawJson value={counters} label="Raw miss counters" /> : null}
    </Card>
  );
}

/**
 * The people allowed to value real-world assets.
 *
 * Named rather than counted, because for an attested value — as opposed to one
 * a market discovered — who signed it is the whole basis for trusting it.
 */
function Valuers({ list, expert }: { list: any[]; expert: boolean }) {
  if (list.length === 0) return null;

  const approved = list.filter((a) => a.status === 'APPRAISER_STATUS_APPROVED');
  const pending = list.filter((a) => a.status === 'APPRAISER_STATUS_PENDING');

  return (
    <Card title="Approved valuers">
      <p className="small muted" style={{ marginTop: 0 }}>
        Real-world assets are not priced by a market. An independent party governance has admitted inspects
        the asset and signs a value, and every figure they publish stays attributed to them.
      </p>

      {approved.length === 0 ? (
        <Empty title="Nobody has been admitted yet" />
      ) : (
        <ul className="rows">
          {approved.map((a) => (
            <li key={a.address} className="row">
              <div className="row__main">
                <strong>{a.name || truncateAddress(a.address)}</strong>
                <p className="small muted" style={{ margin: '0.2rem 0 0' }}>
                  {a.credentials}
                  {a.class_ids?.length ? ` · may value: ${a.class_ids.join(', ')}` : ' · may value any asset type'}
                </p>
                {expert ? <p className="small faint mono" style={{ margin: '0.2rem 0 0' }}>{a.address}</p> : null}
              </div>
            </li>
          ))}
        </ul>
      )}

      {pending.length > 0 ? (
        <p className="small muted" style={{ marginBottom: 0 }}>
          {pending.length} {pending.length === 1 ? 'application is' : 'applications are'} waiting on a
          governance vote.
        </p>
      ) : null}
    </Card>
  );
}
