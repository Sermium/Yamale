import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import {
  EMPTY_AMOUNT,
  ROLE_LABELS,
  checkSpend,
  committed,
  formatAmount,
  formatCoins,
  formatDuration,
  policyRefusals,
  resolveDenom,
  spendable,
  timeAgo,
  timeUntil,
  toBaseUnitsOf,
  t as tr,
  type SpendPolicy,
  type TreasuryLock,
  type RoleAssignment,
  type TreasuryBalance,
} from '@yamale/chain';

import { client } from '../chain.ts';
import { Address } from '../Address.tsx';
import { DenomRow, Split } from '../Custody.tsx';
import { Unknown } from '../Unknown.tsx';

type Tab = 'assets' | 'spend' | 'limits' | 'members' | 'commitments';

/**
 * One treasury, in five views.
 *
 * The tabs are ordered the way a treasurer's questions arrive: what is in here,
 * can I pay somebody, what would stop me, who else can, and what is already
 * promised. The last one is not an afterthought — it is the explanation for the
 * first.
 *
 * "Limits" is new, and it was the largest hole in this console. A treasury's
 * spending policy is the whole reason a spender role is safe to hand out — it
 * bounds a compromised operational key to one period's limit rather than to the
 * treasury — and the chain has always served it. Nothing read it, so a
 * treasurer discovered a limit by hitting it, which is the one way to find out
 * that costs a round of signatures.
 */
export function TreasuryPage({ id }: { id: string }) {
  const [tab, setTab] = useState<Tab>('assets');

  const treasury = useQuery({ queryKey: ['treasury', id], queryFn: () => client.treasury(id) });
  const balances = useQuery({
    queryKey: ['treasury-balances', id],
    queryFn: () => client.treasuryBalances(id),
  });
  const roles = useQuery({ queryKey: ['treasury-roles', id], queryFn: () => client.treasuryRoles(id) });
  const locks = useQuery({ queryKey: ['treasury-locks', id], queryFn: () => client.treasuryLocks(id) });

  if (treasury.isPending) {
    return (
      <section className="card" aria-busy="true">
        <div className="skeleton"><i /><i /><i /></div>
        <p className="small muted" role="status">{tr('safe.readingTreasury', { id })}</p>
      </section>
    );
  }

  if (treasury.isError) {
    return (
      <section className="card">
        <h1>{tr('safe.cannotReach')}</h1>
        <Unknown
          what={tr('safe.existenceUnknown', { id })}
          error={treasury.error}
          onRetry={() => treasury.refetch()}
        />
      </section>
    );
  }

  if (!treasury.data) {
    return (
      <section className="empty">
        <h1>{tr('safe.noSuchTreasury', { id })}</h1>
        <p>{tr('safe.noSuchTreasuryBody')}</p>
        <p><Link to="/">{tr('safe.backToList')}</Link></p>
      </section>
    );
  }

  const t = treasury.data;
  const list = balances.data ?? [];

  return (
    <>
      <p className="crumb">
        <Link to="/">{tr('safe.treasuries')}</Link> / {t.name || tr('safe.treasuryN', { id })}
      </p>

      <div className="page-head">
        <h1>{t.name || tr('safe.treasuryN', { id })}</h1>
        {t.paused && <span className="badge badge-paused">{tr('safe.paused')}</span>}
      </div>
      <p className="small muted">
        {tr('safe.admin')} <Address address={t.admin} /> ·{' '}
        {/* A block height is not a time. Somebody asking how old a treasury is
            wants "three months ago", and the height stays available for anybody
            who wants to look the block up. */}
        <span title={`block ${t.createdAtHeight.toLocaleString()}`}>
          {tr('safe.openedAtBlock', { height: t.createdAtHeight.toLocaleString() })}
        </span>
      </p>

      {t.paused && (
        <div className="notice notice--bad">
          <strong>{tr('safe.frozenTitle')}</strong> {tr('safe.frozenBody')}
        </div>
      )}

      <nav className="tabs" role="tablist">
        {(
          [
            ['assets', tr('safe.assets')],
            ['spend', tr('safe.paySomeone')],
            ['limits', tr('safe.limits')],
            ['members', tr('safe.members')],
            ['commitments', tr('safe.commitments')],
          ] as [Tab, string][]
        ).map(([key, label]) => (
          <button
            key={key}
            role="tab"
            aria-selected={tab === key}
            className={tab === key ? 'tab tab--on' : 'tab'}
            onClick={() => setTab(key)}
            type="button"
          >
            {label}
          </button>
        ))}
      </nav>

      {tab === 'assets' && (
        <Assets
          balances={list}
          pending={balances.isPending}
          error={balances.isError ? balances.error : null}
          onRetry={() => balances.refetch()}
        />
      )}
      {tab === 'spend' && (
        <Spend treasuryId={id} balances={list} paused={t.paused} name={t.name} admin={t.admin} />
      )}
      {tab === 'limits' && <Limits treasuryId={id} balances={list} />}
      {tab === 'members' && (
        <Members
          admin={t.admin}
          roles={roles.data ?? []}
          pending={roles.isPending}
          error={roles.isError ? roles.error : null}
          onRetry={() => roles.refetch()}
        />
      )}
      {tab === 'commitments' && (
        <Commitments
          locks={locks.data ?? []}
          pending={locks.isPending}
          error={locks.isError ? locks.error : null}
          onRetry={() => locks.refetch()}
        />
      )}
    </>
  );
}

function Assets({
  balances,
  pending,
  error,
  onRetry,
}: {
  balances: TreasuryBalance[];
  pending: boolean;
  error: unknown;
  onRetry: () => void;
}) {
  if (pending) {
    return (
      <section className="card" aria-busy="true">
        <div className="skeleton"><i /><i /><i /></div>
      </section>
    );
  }

  // Before the empty state, never after it. `balances.data ?? []` rendered a
  // node that did not answer as a treasury with nothing in it, which is a
  // different and much more alarming fact than the one that was true.
  if (error) {
    return (
      <section className="card">
        <h2>{tr('safe.assets')}</h2>
        <Unknown what={tr('safe.balancesUnknown')} error={error} onRetry={onRetry} />
      </section>
    );
  }

  if (balances.length === 0) {
    return (
      <section className="empty">
        <h2>{tr('safe.nothingInIt')}</h2>
        <p>{tr('msg.emptyTreasury')}</p>
      </section>
    );
  }

  const anyLocked = balances.some((b) => b.locked !== '0');

  return (
    <section className="card" role="region" aria-label="Assets">
      {/* Its own scroll box. Four numeric columns overflow a phone, and a
          horizontal scrollbar on the document takes the whole layout with it. */}
      <div className="y-scroll">
        <table className="table">
          <thead>
            <tr>
              <th>{tr('safe.currency')}</th>
              <th className="num">{tr('safe.available')}</th>
              <th className="num">{tr('safe.committed')}</th>
              <th className="num">{tr('safe.total')}</th>
            </tr>
          </thead>
          <tbody>
            {balances.map((b) => (
              <DenomRow key={b.denom} balance={b} />
            ))}
          </tbody>
        </table>
      </div>

      {/* Stated only when it is true of this treasury. A claim about committed
          funds printed under a table with nothing committed is a claim nobody
          can check, and the ones that can be checked are worth more. */}
      <p className={anyLocked ? 'refuses' : 'small muted card__foot'}>
        {anyLocked && <span className="refuses__k">{tr('safe.refuses')}</span>}
        {anyLocked ? tr('safe.refusesCommitted') : tr('safe.nothingCommittedHere')}
      </p>
    </section>
  );
}

/**
 * Proposing a payment.
 *
 * The check runs *before* the approvals are collected. Discovering at execution
 * that the money was committed elsewhere wastes every signature already given
 * and reads as a chain failure rather than as the policy it is.
 *
 * The amount is converted to base units by string. It used to be
 * `Math.round(Number(amount) * 1e6)`, which is wrong twice: `1e6` is a guess
 * about the denom — right for everything this chain mints and an order of
 * magnitude out for the first IBC voucher that arrives — and multiplying a
 * binary float by a million makes whether the payment is correct depend on
 * which decimal the treasurer happened to type.
 */
function Spend({
  treasuryId,
  balances,
  paused,
  name,
  admin,
}: {
  treasuryId: string;
  balances: TreasuryBalance[];
  paused: boolean;
  name: string;
  admin: string;
}) {
  const [recipient, setRecipient] = useState('');
  const [denom, setDenom] = useState(balances[0]?.denom ?? 'uyml');
  const [amount, setAmount] = useState('');
  const [memo, setMemo] = useState('');

  // The policy for the chosen denom, read from the chain rather than assumed
  // absent. A payment that fits the balance and breaks the limit is refused at
  // execution, after the signatures — which is the expensive way to find out.
  const policy = useQuery({
    queryKey: ['treasury-policy', treasuryId, denom],
    queryFn: () => client.treasurySpendPolicy(treasuryId, denom),
  });

  const info = resolveDenom(denom);
  const parsed = amount.trim() === '' ? null : toBaseUnitsOf(amount, denom);
  const typedButUnreadable = amount.trim() !== '' && parsed === null;

  const verdict = checkSpend(
    { id: treasuryId, name, admin, paused, createdAtHeight: 0 },
    balances,
    parsed ? [{ denom, amount: parsed.base }] : [],
  );

  const overPerTransaction =
    parsed && policy.data?.perTransaction
      ? BigInt(parsed.base) > BigInt(policy.data.perTransaction)
      : false;
  const offAllowlist =
    recipient.trim() !== '' &&
    (policy.data?.allowlist.length ?? 0) > 0 &&
    !policy.data!.allowlist.includes(recipient.trim());
  const onBlocklist = recipient.trim() !== '' && (policy.data?.blocklist ?? []).includes(recipient.trim());

  const message = {
    '@type': '/blockchain.treasury.v1.MsgSpend',
    spender: '<your address, or the group policy>',
    treasury_id: treasuryId,
    recipient: recipient || '<recipient>',
    amount: [{ denom, amount: parsed?.base ?? '0' }],
    memo,
  };

  return (
    <section className="card">
      <h2>{tr('safe.paySomeone')}</h2>

      <label className="field">
        <span>{tr('safe.recipient')}</span>
        <input
          value={recipient}
          onChange={(e) => setRecipient(e.target.value)}
          placeholder="yml1…"
          spellCheck={false}
          autoComplete="off"
        />
      </label>

      <div className="field-row">
        <label className="field amount-field">
          <span>{tr('safe.amountIn', { symbol: info.symbol })}</span>
          <input value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="0.00" inputMode="decimal" />
        </label>
        <label className="field">
          <span>{tr('safe.currency')}</span>
          <select value={denom} onChange={(e) => setDenom(e.target.value)}>
            {balances.map((b) => (
              <option key={b.denom} value={b.denom}>
                {resolveDenom(b.denom).symbol}
              </option>
            ))}
          </select>
        </label>
      </div>

      <label className="field">
        <span>{tr('safe.memo')}</span>
        <input value={memo} onChange={(e) => setMemo(e.target.value)} placeholder="March payroll" />
      </label>

      {/* Refused at the field rather than at execution. An amount that cannot
          be parsed must never quietly become zero in the payload below. */}
      {typedButUnreadable && (
        <p className="field-note field-note--bad">{tr('safe.notAnAmount')}</p>
      )}
      {parsed?.truncated && (
        <p className="field-note field-note--warn">
          {tr('safe.truncated', {
            symbol: info.symbol,
            places: String(info.exponent),
            amount: formatAmount(parsed.base, denom),
          })}
        </p>
      )}

      {parsed && !verdict.ok && <div className="notice notice--bad">{verdict.reason}</div>}

      {/* The policy checks, separate from the balance check. "You have enough
          but the policy says no" and "you do not have enough" are different
          problems with different remedies, and merging them sends a treasurer
          looking for money that is already there. */}
      {overPerTransaction && (
        <div className="notice notice--bad">
          {tr('safe.overPerTransaction', {
            limit: formatAmount(policy.data!.perTransaction!, denom),
          })}
        </div>
      )}
      {offAllowlist && <div className="notice notice--bad">{tr('safe.offAllowlist')}</div>}
      {onBlocklist && <div className="notice notice--bad">{tr('safe.onBlocklist')}</div>}

      {parsed && verdict.ok && !overPerTransaction && !offAllowlist && !onBlocklist && (
        <div className="notice notice--ok">
          {tr('safe.canPay', { amount: formatAmount(parsed.base, denom) })}{' '}
          {tr('safe.availableNow', { amount: formatCoins(spendable(balances)) })}
          {committed(balances).length > 0 && (
            <> · {tr('safe.committedNow', { amount: formatCoins(committed(balances)) })}</>
          )}
        </div>
      )}

      {/* The proposal payload, shown rather than hidden. A signer approving a
          spend should be able to read exactly what they are approving, and an
          M-of-N group needs this JSON to submit it. */}
      <details className="payload">
        <summary>{tr('safe.theMessage')}</summary>
        <pre className="payload__pre">{JSON.stringify(message, null, 2)}</pre>
        <p className="small muted">{tr('safe.theMessageNote')}</p>
      </details>
    </section>
  );
}

/**
 * What this treasury refuses, per currency.
 *
 * Written as refusals rather than as permissions, because that is the question
 * somebody arrives with: not "what may I do" but "why did that not go
 * through". A treasury with no policy for a currency is not one with a limit of
 * zero — it is one where a spender is bounded only by the balance, and saying
 * so plainly is worth a row of its own.
 */
function Limits({ treasuryId, balances }: { treasuryId: string; balances: TreasuryBalance[] }) {
  const denoms = balances.length > 0 ? balances.map((b) => b.denom) : ['uyml'];

  return (
    <section className="card">
      <h2>{tr('safe.limits')}</h2>
      <p className="small muted">{tr('safe.limitsLede')}</p>
      {denoms.map((denom) => (
        <LimitRow key={denom} treasuryId={treasuryId} denom={denom} />
      ))}
    </section>
  );
}

function LimitRow({ treasuryId, denom }: { treasuryId: string; denom: string }) {
  const policy = useQuery({
    queryKey: ['treasury-policy', treasuryId, denom],
    queryFn: () => client.treasurySpendPolicy(treasuryId, denom),
  });
  const capacity = useQuery({
    queryKey: ['treasury-capacity', treasuryId, denom],
    queryFn: () => client.spendCapacity(treasuryId, denom),
  });

  const info = resolveDenom(denom);

  if (policy.isPending) {
    return (
      <div className="limit">
        <h3 className="limit__k">{info.symbol}</h3>
        <div className="skeleton"><i /></div>
      </div>
    );
  }

  if (policy.isError) {
    return (
      <div className="limit">
        <h3 className="limit__k">{info.symbol}</h3>
        <Unknown
          what={tr('safe.limitsUnknown')}
          error={policy.error}
          onRetry={() => policy.refetch()}
        />
      </div>
    );
  }

  const refusals = policyRefusals(
    policy.data as SpendPolicy | null,
    (amount, d) => formatAmount(amount, d),
    (seconds) => formatDuration(seconds),
  );

  return (
    <div className="limit">
      <h3 className="limit__k">
        {info.symbol} <span className="small muted">{info.name}</span>
      </h3>

      {refusals.length === 0 ? (
        <p className="muted">{tr('safe.noPolicy')}</p>
      ) : (
        <>
          <p className="refuses">
            <span className="refuses__k">{tr('safe.refuses')}</span>
          </p>
          <ul className="limit__list">
            {refusals.map((refusal, i) => (
              <li key={i}>{tr(refusal.key, refusal.vars)}</li>
            ))}
          </ul>
        </>
      )}

      {/* The live figure: what is actually left in this window. A limit without
          it answers "how much per day"; a treasurer is asking "how much now". */}
      {capacity.data && policy.data?.perPeriod && (
        <p className="limit__now">
          <span className="y-label">{tr('safe.leftThisPeriod')}</span>{' '}
          <span className="y-num">{formatAmount(capacity.data.remainingThisPeriod, denom)}</span>
          {/* formatDuration, not timeUntil. timeUntil returns a whole phrase
              ("23 hours left"), which inside "resets {when}" reads as
              "resets 23 hours left". A duration is the part that belongs in
              this sentence. */}
          {capacity.data.periodResetsAt ? (
            <span className="small muted">
              {' '}
              ·{' '}
              {tr('safe.resets', {
                when: formatDuration(
                  Math.max(0, capacity.data.periodResetsAt - Math.floor(Date.now() / 1000)),
                ),
              })}
            </span>
          ) : null}
        </p>
      )}
    </div>
  );
}

function Members({
  admin,
  roles,
  pending,
  error,
  onRetry,
}: {
  admin: string;
  roles: RoleAssignment[];
  pending: boolean;
  error: unknown;
  onRetry: () => void;
}) {
  return (
    <section className="card">
      <h2>{tr('safe.members')}</h2>

      {pending && <div className="skeleton"><i /><i /></div>}

      {/* An unread role list rendered as "only the admin" would understate who
          can move money out of this treasury, which is the worst direction for
          this particular error to be wrong in. */}
      {error ? <Unknown what={tr('safe.rolesUnknown')} error={error} onRetry={onRetry} /> : null}

      {!pending && !error && (
        <div className="y-scroll">
          <table className="table">
            <tbody>
              <tr>
                <td><Address address={admin} /></td>
                <td><span className="badge">{tr('safe.admin')}</span></td>
              </tr>
              {roles.map((r) => (
                <tr key={`${r.address}-${r.role}`}>
                  <td><Address address={r.address} /></td>
                  <td><span className="badge">{ROLE_LABELS[r.role] ?? r.role}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <p className="small muted card__foot">{tr('safe.membersNote')}</p>
    </section>
  );
}

/**
 * What has been promised, and how far along each promise is.
 *
 * A commitment is not one number either. Part of it has been released and
 * claimed, part has been released and is sitting there for the beneficiary to
 * take, and part is still ahead of the schedule — and only the last part is
 * what "committed" means on the assets tab. A table of totals hides all three
 * distinctions, so each row carries its own progress.
 */
function Commitments({
  locks,
  pending,
  error,
  onRetry,
}: {
  locks: TreasuryLock[];
  pending: boolean;
  error: unknown;
  onRetry: () => void;
}) {
  if (pending) {
    return (
      <section className="card" aria-busy="true">
        <div className="skeleton"><i /><i /></div>
      </section>
    );
  }

  // Before the empty state. "Nothing committed — every coin this treasury holds
  // is available to spend" printed because a request failed is a false
  // statement about somebody's money in the place they went to check it.
  if (error) {
    return (
      <section className="card">
        <h2>{tr('safe.commitments')}</h2>
        <Unknown what={tr('safe.commitmentsUnknown')} error={error} onRetry={onRetry} />
      </section>
    );
  }

  if (locks.length === 0) {
    return (
      <section className="empty">
        <h2>{tr('safe.nothingCommitted')}</h2>
        <p>{tr('safe.nothingCommittedBody')}</p>
      </section>
    );
  }

  return (
    <section className="card">
      <h2>{tr('safe.commitments')}</h2>
      <p className="refuses">
        <span className="refuses__k">{tr('safe.refuses')}</span>
        {tr('safe.refusesCommitted')}
      </p>

      <div className="y-scroll">
        <table className="table">
          <thead>
            <tr>
              <th>{tr('safe.beneficiary')}</th>
              <th>{tr('safe.progress')}</th>
              <th className="num">{tr('safe.stillLocked')}</th>
              <th>{tr('safe.releases')}</th>
              <th>{tr('safe.revocable')}</th>
            </tr>
          </thead>
          <tbody>
            {locks.map((l) => (
              <LockRow key={l.id} lock={l} />
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function LockRow({ lock }: { lock: TreasuryLock }) {
  // Still locked = committed minus what the beneficiary has already taken. This
  // is the figure that appears in the treasury's `locked` balance, so a row
  // showing only the total would not reconcile against the assets tab.
  let remaining = '0';
  try {
    const left = BigInt(lock.amount) - BigInt(lock.claimed);
    remaining = (left > 0n ? left : 0n).toString();
  } catch {
    remaining = '0';
  }

  return (
    <tr className={lock.revoked ? 'row--off' : undefined}>
      <td><Address address={lock.beneficiary} /></td>
      <td className="lock__cell">
        <span className="lock__total">{formatAmount(lock.amount, lock.denom)}</span>
        {/* The bar is drawn the other way round from the treasury's: here the
            claimed part is the part that has *gone*, so the hatched remainder
            is what is still held. Same visual grammar, same meaning — hatched
            is what nobody can spend. */}
        <Split
          total={lock.amount}
          locked={remaining}
          label={tr('safe.lockAlt', {
            claimed: formatAmount(lock.claimed, lock.denom),
            remaining: formatAmount(remaining, lock.denom),
          })}
        />
        <span className="small muted">
          {tr('safe.claimedSoFar', { amount: formatAmount(lock.claimed, lock.denom) })}
        </span>
      </td>
      <td className="num">
        <span className="num--lock">{formatAmount(remaining, lock.denom, { withSymbol: false })}</span>
      </td>
      {/* Human time. "Releases in 12 days" is what a beneficiary is asking; the
          date stays in the title for anybody reconciling against a contract. */}
      <td className="small">{releaseWhen(lock.endTime)}</td>
      <td className="small muted">
        {lock.revoked
          ? tr('safe.revoked')
          : lock.revocable
            ? tr('safe.revocableYes')
            : tr('safe.revocableNo')}
      </td>
    </tr>
  );
}

function releaseWhen(endTime: number | undefined) {
  if (!endTime) return <span className="muted">{EMPTY_AMOUNT}</span>;
  const at = new Date(endTime * 1000);
  const future = at.getTime() > Date.now();
  return (
    <span title={at.toLocaleString()}>
      {future
        ? tr('safe.releasesIn', {
            when: formatDuration(Math.round((at.getTime() - Date.now()) / 1000)),
          })
        : timeAgo(at.toISOString())}
    </span>
  );
}
