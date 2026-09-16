import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { delegate, formatNumber, formatPercent, t, type Validator } from '@yamale/chain';

import { client, useViewMode } from '../chain.ts';
import { Card, Empty, ErrorState, Loading } from '../components.tsx';
import { displayAmount, localDuration, shareOf } from '../amount.ts';
import { faultTolerance } from '../health.ts';
import { ARC_TONES, Donut, Kpi, KpiBand, Swatch } from '../ui.tsx';
import { SignAction, useWallet } from '../wallet.tsx';

/**
 * Staking answers two questions, and they are not the same question.
 *
 * A delegator asks "where do I put my tokens, and what do I get?" A supervisor
 * asks "who actually holds this chain?" The page used to answer the first one
 * with three small figures and a table sorted by size, and the second one with
 * a 12px amber badge on one row reading "over one third".
 *
 * On devnet-2 that badge was hiding the single most important fact about this
 * network: there are two validators and one of them holds about seventy per
 * cent of the bonded stake. That is not a footnote, it is the security model,
 * and a console sold to a central bank that soft-pedals it is worse than one
 * that says nothing. So concentration is now a section with a ring, a ranked
 * table with a running total, and the consequence spelled out in a sentence.
 */
export function StakingPage() {
  const { mode } = useViewMode();
  const expert = mode === 'expert';

  const staking = useQuery({
    queryKey: ['staking'],
    queryFn: () => client.stakingOverview(),
    refetchInterval: 15_000,
  });

  // The supply, for the bonded share.
  //
  // NOT `overview.bondedRatio`. The SDK computes that with an integer division
  // by ten thousand before converting to a number, which quantises every share
  // to four decimal places — so this chain's true 0.0175% was rendering as
  // "0.0% of all tokens", a figure the chain does not hold and one that reads
  // as *nothing is staked* when the truth is *very little is*. The status strip
  // was fixed for this months ago; this page was not.
  const supply = useQuery({
    queryKey: ['supply'],
    queryFn: () => client.totalSupply(),
    refetchInterval: 60_000,
  });

  if (staking.isPending) return <Loading label={t('xp.feed.loadingActivity')} />;
  if (staking.isError) return <ErrorState error={staking.error} what="staking data" />;

  const data = staking.data!;
  const validators = data.validators;

  if (validators.length === 0) {
    return (
      <>
        <h1>{t('exp.staking')}</h1>
        <Empty title={t('xp.stake.noneTitle')} hint={t('xp.stake.noneBody')} />
      </>
    );
  }

  const ranked = [...validators].sort((a, b) => b.votingPower - a.votingPower);
  const largest = ranked[0];

  const bonded = displayAmount(data.bonded, 'uyml');
  const issued = supply.data?.find((c) => c.denom === 'uyml');
  const bondedShare = issued ? shareOf(data.bonded, issued.amount) : null;

  const { spare, fragile } = faultTolerance(validators.map((v) => v.votingPower));

  return (
    <>
      <h1>{t('exp.staking')}</h1>
      <p className="lede">{t('xp.stake.lede')}</p>

      {/* The figures first, at a size that says they are figures. The page
          opened with 1.25rem values against 0.78rem labels, so "174,375 YML"
          and "securing the chain" carried the same weight. */}
      <KpiBand>
        <Kpi
          label={t('xp.status.validators')}
          value={formatNumber(validators.length)}
          hint={
            fragile
              ? t('xp.status.fragile')
              : t('xp.status.spare', { count: String(spare) })
          }
          tone={fragile ? 'warn' : undefined}
        />
        <Kpi
          label={t('xp.status.bonded')}
          value={bonded.value}
          unit={bonded.symbol}
          hint={
            // A share we could not compute is not a share of zero. The supply
            // read is separate from the staking read and either can fail.
            bondedShare === null
              ? t('xp.kpi.unread')
              : t('xp.status.ofSupply', { percent: sharePercent(bondedShare) })
          }
        />
        <Kpi
          label={t('xp.stake.largest')}
          value={formatPercent(largest.votingPower, 1)}
          hint={t('xp.stake.largestHint', {
            name: largest.moniker || t('xp.stake.unnamed'),
          })}
          // Over a third is a warning, over two thirds is a finding. The word
          // in the hint carries it; the edge colour only reinforces.
          tone={
            largest.votingPower > 2 / 3 ? 'bad' : largest.votingPower > 1 / 3 ? 'warn' : undefined
          }
        />
        <Kpi
          label={t('xp.stake.delay')}
          value={localDuration(data.unbondingSeconds)}
          hint={t('xp.stake.afterAsking')}
        />
      </KpiBand>

      <div className="notice">
        <strong>{t('xp.stake.noticeTitle')}</strong>{' '}
        {t('xp.stake.noticeBody', { delay: localDuration(data.unbondingSeconds) })}
      </div>

      <Concentration ranked={ranked} />

      {/* Not titled "Staking": that is the h1 four lines above it, and a card
          repeating the page's own heading tells a reader nothing about what is
          inside it. */}
      <Card title={t('xp.status.validators')} flush>
        <div className="y-scroll">
          <table className="grid">
            <thead>
              <tr>
                <th>{t('col.validator')}</th>
                <th className="share">{t('col.theyKeep')}</th>
                <th className="share">{t('col.shareOfStake')}</th>
                <th className="share">{t('col.staked')}</th>
                {expert ? <th>{t('col.status')}</th> : null}
                <th>
                  <span className="visually-hidden">{t('col.stake')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {ranked.map((v) => (
                <ValidatorRow
                  key={v.operatorAddress}
                  validator={v}
                  expert={expert}
                  unbondingSeconds={data.unbondingSeconds}
                />
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </>
  );
}

/**
 * Who holds the chain, drawn and then itemised.
 *
 * The running total is the column that does the work. "70.2%" and "29.8%" are
 * two numbers a reader has to add; a cumulative column means the threshold
 * questions — who can stall a block, who can commit one — are answered by
 * reading down until the number passes 33.3 or 66.7, which is a scan rather
 * than arithmetic.
 */
function Concentration({ ranked }: { ranked: Validator[] }) {
  // Past the eighth holder the arcs become hairlines, so the tail is one slice.
  // A large validator set is the success case for this chain and a chart that
  // stopped working at that point would fail exactly when the argument is won.
  const slices = useMemo(() => {
    const head = ranked.slice(0, 8);
    const tailShare = ranked.slice(8).reduce((sum, v) => sum + v.votingPower, 0);
    const out = head.map((v, i) => ({
      label: v.moniker || t('xp.stake.unnamed'),
      fraction: v.votingPower,
      // The largest holder takes the warning colour when it is over a third,
      // so the ring and the table agree about which slice is the problem.
      tone: i === 0 && v.votingPower > 1 / 3 ? 'var(--warn)' : ARC_TONES[i % ARC_TONES.length],
    }));
    if (tailShare > 0) {
      out.push({ label: t('xp.stake.others'), fraction: tailShare, tone: 'var(--text-faint)' });
    }
    return out;
  }, [ranked]);

  let running = 0;

  return (
    <Card title={t('xp.stake.concentration')}>
      <p className="small muted">{t('xp.stake.concLede')}</p>

      <div className="concentration">
        <Donut
          slices={slices}
          label={slices.map((s) => `${s.label} ${(s.fraction * 100).toFixed(1)}%`).join(', ')}
        />

        <div className="y-scroll">
          <table className="grid">
            <thead>
              <tr>
                <th>{t('col.validator')}</th>
                <th className="share">{t('col.shareOfStake')}</th>
                <th className="share">{t('xp.stake.cumulative')}</th>
              </tr>
            </thead>
            <tbody>
              {slices.map((s, i) => {
                running += s.fraction;
                const crosses = s.fraction > 2 / 3 ? 'bad' : s.fraction > 1 / 3 ? 'warn' : null;
                return (
                  <tr key={`${s.label}-${i}`}>
                    <td>
                      <span className="inline">
                        <Swatch tone={s.tone} />
                        {s.label}
                      </span>
                      {/* The consequence in words, on the row it applies to.
                          A colour alone would leave this unreadable in a
                          monochrome screenshot and to a reader who cannot
                          separate the hues. */}
                      {crosses ? (
                        <div className={`y-chip y-chip--${crosses}`}>
                          {crosses === 'bad'
                            ? t('xp.stake.overTwoThirds')
                            : t('xp.stake.overThird')}
                        </div>
                      ) : null}
                    </td>
                    <td className="share">{formatPercent(s.fraction, 1)}</td>
                    <td className="share muted">{formatPercent(running, 1)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </Card>
  );
}

function ValidatorRow({
  validator,
  expert,
  unbondingSeconds,
}: {
  validator: Validator;
  expert: boolean;
  unbondingSeconds: number;
}) {
  const staked = displayAmount(validator.tokens, 'uyml');

  return (
    <tr>
      <td>
        <div>{validator.moniker || <span className="faint">{t('xp.stake.unnamed')}</span>}</div>
        {expert ? <div className="faint mono small">{validator.operatorAddress}</div> : null}
        {validator.jailed ? <span className="badge badge--bad">{t('col.suspended')}</span> : null}
      </td>
      <td className="share">
        {formatPercent(validator.commission, 1)}
        <div className="faint small">{t('xp.stake.ofRewards')}</div>
      </td>
      <td className="share">{formatPercent(validator.votingPower, 1)}</td>
      <td className="share y-num">
        {staked.value} {staked.symbol}
      </td>
      {expert ? <td className="faint small">{validator.status.replace('BOND_STATUS_', '')}</td> : null}
      <td>
        <StakeAction validator={validator} unbondingSeconds={unbondingSeconds} />
      </td>
    </tr>
  );
}

/**
 * A share, printed to enough places to be non-zero.
 *
 * Measured on this chain: 174,900 YML of 997,766,566 is 0.0175%, which at one
 * decimal place prints as "0.0% of supply" — a figure that reads as *nothing is
 * staked* when the truth is *very little is*. Those are different statements
 * and only one of them is true.
 */
function sharePercent(ratio: number): string {
  const percent = ratio * 100;
  if (percent >= 1) return formatPercent(ratio, 1);
  if (percent >= 0.1) return formatPercent(ratio, 2);
  return formatPercent(ratio, 3);
}

/**
 * Staking with one validator.
 *
 * The consequence is stated before the amount is asked for, not after: the
 * tokens stop being available, the wait to get them back is fixed and long, and
 * part of them can be lost if this validator misbehaves. Somebody should decide
 * with that in front of them rather than discover it when they try to withdraw.
 */
function StakeAction({
  validator,
  unbondingSeconds,
}: {
  validator: Validator;
  unbondingSeconds: number;
}) {
  const { address } = useWallet();
  const [amount, setAmount] = useState('');

  if (!address) return <span className="small faint">—</span>;

  const base = toBaseUnits(amount, 6);
  const display = base === null ? null : displayAmount(base, 'uyml');

  return (
    <div className="stakebox">
      <div className="inline stakebox__entry">
        <input
          className="stakebox__input y-num"
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          placeholder="0.00"
          inputMode="decimal"
          aria-label={t('xp.stake.amountFor', {
            name: validator.moniker || validator.operatorAddress,
          })}
        />
        <span className="small muted">YML</span>
      </div>

      <SignAction
        label={t('xp.stake.action')}
        disabled={base === null}
        gas={300_000}
        consequence={
          display === null
            ? t('xp.stake.enterAmount')
            : t('xp.stake.consequence', {
                amount: `${display.value} ${display.symbol}`,
                delay: localDuration(unbondingSeconds),
              })
        }
        build={(signer) => [
          delegate(signer, validator.operatorAddress, { denom: 'uyml', amount: base! }),
        ]}
      />
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
