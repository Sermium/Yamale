import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { delegate, formatAmount, formatDuration, formatPercent, type Validator, t} from '@yamale/chain';

import { client, useViewMode } from '../chain.ts';
import { Card, Empty, ErrorState, Loading, Stat } from '../components.tsx';
import { SignAction, useWallet } from '../wallet.tsx';

/**
 * Staking answers "where do I put my tokens, and what do I get?"
 *
 * It leads with the two things that decide that — what a validator keeps, and
 * how long your money is unavailable if you change your mind — rather than with
 * a leaderboard by size, which just tells people to pick whoever is already
 * largest.
 */
export function StakingPage() {
  const { mode } = useViewMode();
  const staking = useQuery({ queryKey: ['staking'], queryFn: () => client.stakingOverview(), refetchInterval: 15_000 });

  if (staking.isPending) return <Loading label="Fetching the validator set" />;
  if (staking.isError) return <ErrorState error={staking.error} what="staking data" />;

  const data = staking.data!;
  const concentrated = data.validators.filter((v) => v.concerningPower);

  return (
    <>
      <h1>{t('exp.staking')}</h1>
      <p className="lede">
        Lock tokens with a validator to help secure the network and earn a share of new issuance and
        transaction fees.
      </p>

      <div className="notice" style={{ marginBottom: '1rem' }}>
        <strong>Your tokens are not available while staked.</strong> Withdrawing takes{' '}
        {formatDuration(data.unbondingSeconds)} from the moment you ask, and they earn nothing during that
        time. If a validator misbehaves, part of what you staked with them can be lost.
      </div>

      <div className="stats" style={{ marginBottom: '1rem' }}>
        <Stat label="Validators" value={data.validators.length} note="securing the chain" />
        <Stat
          label="Staked"
          value={formatAmount(data.bonded, 'uyml', { maxDecimals: 0 })}
          note={`${formatPercent(data.bondedRatio, 1)} of all tokens`}
        />
        <Stat
          label="Withdrawal delay"
          value={formatDuration(data.unbondingSeconds)}
          note="after you ask to unstake"
        />
      </div>

      {concentrated.length > 0 ? (
        <div className="notice" style={{ marginBottom: '1rem' }}>
          <strong>Stake is concentrated.</strong>{' '}
          {concentrated.length === 1
            ? `${concentrated[0].moniker || 'One validator'} holds`
            : `${concentrated.length} validators each hold`}{' '}
          more than a third of all staked tokens, which is enough to stall the chain. Staking elsewhere
          makes the network more resilient.
        </div>
      ) : null}

      <Card title="Validators" flush>
        {data.validators.length === 0 ? (
          <Empty title="No validators yet" hint="The chain has no bonded validators." />
        ) : (
          <div className="scroll-x">
            <table className="grid">
              <thead>
                <tr>
                  <th>{t('col.validator')}</th>
                  <th>{t('col.theyKeep')}</th>
                  <th>{t('col.shareOfStake')}</th>
                  <th>{t('col.staked')}</th>
                  {mode === 'expert' ? <th>{t('col.status')}</th> : null}
                  <th><span className="visually-hidden">{t('col.stake')}</span></th>
                </tr>
              </thead>
              <tbody>
                {data.validators.map((v) => (
                  <ValidatorRow
                    key={v.operatorAddress}
                    validator={v}
                    expert={mode === 'expert'}
                    unbondingSeconds={data.unbondingSeconds}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </>
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
  return (
    <tr>
      <td>
        <div>{validator.moniker || <span className="faint">unnamed</span>}</div>
        {expert ? <div className="faint mono small">{validator.operatorAddress}</div> : null}
        {validator.jailed ? <span className="badge badge--bad">{t('col.suspended')}</span> : null}
      </td>
      <td>
        {formatPercent(validator.commission, 1)}
        <div className="faint small">of your rewards</div>
      </td>
      <td>
        {formatPercent(validator.votingPower, 1)}
        {validator.concerningPower ? (
          <div className="badge badge--warn" style={{ marginTop: '0.2rem' }}>
            over one third
          </div>
        ) : null}
      </td>
      <td>{formatAmount(validator.tokens, 'uyml', { maxDecimals: 0 })}</td>
      {expert ? <td className="faint small">{validator.status.replace('BOND_STATUS_', '')}</td> : null}
      <td>
        <StakeAction validator={validator} unbondingSeconds={unbondingSeconds} />
      </td>
    </tr>
  );
}

/**
 * Staking with one validator.
 *
 * The consequence is stated before the amount is asked for, not after: the
 * tokens stop being available, the wait to get them back is fixed and long, and
 * part of them can be lost if this validator misbehaves. Somebody should decide
 * with that in front of them rather than discover it when they try to withdraw.
 */
function StakeAction({ validator, unbondingSeconds }: { validator: Validator; unbondingSeconds: number }) {
  const { address } = useWallet();
  const [amount, setAmount] = useState('');

  if (!address) return <span className="small faint">—</span>;

  const base = toBaseUnits(amount, 6);

  return (
    <div style={{ minWidth: 190 }}>
      <div className="inline" style={{ gap: '0.35rem' }}>
        <input
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          placeholder="0.00"
          inputMode="decimal"
          aria-label={`Amount of YML to stake with ${validator.moniker || validator.operatorAddress}`}
          style={{
            width: 92,
            padding: '0.3rem 0.5rem',
            font: 'inherit',
            fontSize: '0.85rem',
            background: 'var(--bg)',
            color: 'var(--text)',
            border: '1px solid var(--border-strong)',
            borderRadius: 'var(--radius-sm)',
          }}
        />
        <span className="small muted">YML</span>
      </div>

      <SignAction
        label="Stake"
        disabled={base === null}
        gas={300_000}
        consequence={
          base === null ? (
            'Enter an amount.'
          ) : (
            <>
              {formatAmount(base, 'uyml')} stops being available to you. Getting it back takes{' '}
              {formatDuration(unbondingSeconds)} from the moment you ask, and it earns nothing during that
              time. If this validator misbehaves, part of it can be lost.
            </>
          )
        }
        build={(signer) => [delegate(signer, validator.operatorAddress, { denom: 'uyml', amount: base! })]}
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
