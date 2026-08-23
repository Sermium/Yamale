import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import {
  ROLE_LABELS,
  checkSpend,
  committed,
  formatAmount,
  formatCoins,
  resolveDenom,
  spendable,
  timeAgo,
  timeUntil,
  toBaseUnitsOf,
  t as tr,
  type TreasuryLock,
  type RoleAssignment,
  type TreasuryBalance,
} from '@yamale/chain';

import { client } from '../chain.ts';
import { Address } from '../Address.tsx';

type Tab = 'assets' | 'spend' | 'members' | 'commitments';

/**
 * One treasury, in four views.
 *
 * The tabs are ordered the way a treasurer's questions arrive: what is in here,
 * can I pay somebody, who else can, and what is already promised. The last one
 * is not an afterthought — it is the explanation for the first.
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
        <p className="small muted" role="status">Reading treasury {id}…</p>
      </section>
    );
  }

  if (treasury.isError) {
    return (
      <section className="card">
        <h1>{tr('safe.cannotReach')}</h1>
        <p className="muted">
          The node did not answer, so this page cannot say whether treasury {id} exists. That is
          different from it not existing.
        </p>
        <p>
          <button type="button" className="chip" onClick={() => treasury.refetch()}>Try again</button>
        </p>
      </section>
    );
  }

  if (!treasury.data) {
    return (
      <section className="empty">
        <h1>No treasury {id}</h1>
        <p>
          The chain answered, and there is no treasury with that number. Treasury ids start at
          zero and are handed out in order, so a number above the count has simply not been
          created yet.
        </p>
        <p><Link to="/">Back to the list</Link></p>
      </section>
    );
  }

  const t = treasury.data;
  const list = balances.data ?? [];

  return (
    <>
      <p className="crumb">
        <Link to="/">{tr('safe.treasuries')}</Link> / {t.name || `Treasury ${id}`}
      </p>

      <div className="page-head">
        <h1>{t.name || `Treasury ${id}`}</h1>
        {t.paused && <span className="badge badge-paused">{tr('safe.paused')}</span>}
      </div>
      <p className="small muted">
        {tr('safe.admin')} <Address address={t.admin} /> · opened{' '}
        {/* A block height is not a time. Somebody asking how old a treasury is
            wants "three months ago", and the height stays available for anybody
            who wants to look the block up. */}
        <span title={`block ${t.createdAtHeight.toLocaleString()}`}>
          at block {t.createdAtHeight.toLocaleString()}
        </span>
      </p>

      {t.paused && (
        <div className="notice notice--bad">
          <strong>Frozen.</strong> Nothing can leave this treasury until an admin unfreezes it.
          Proposals can still be made and will fail at execution, so it is worth unfreezing first.
        </div>
      )}

      <nav className="tabs" role="tablist">
        {(
          [
            ['assets', 'Assets'],
            ['spend', tr('safe.paySomeone')],
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

      {tab === 'assets' && <Assets balances={list} pending={balances.isPending} />}
      {tab === 'spend' && (
        <Spend treasuryId={id} balances={list} paused={t.paused} name={t.name} admin={t.admin} />
      )}
      {tab === 'members' && <Members admin={t.admin} roles={roles.data ?? []} />}
      {tab === 'commitments' && <Commitments locks={locks.data ?? []} pending={locks.isPending} />}
    </>
  );
}

function Assets({ balances, pending }: { balances: TreasuryBalance[]; pending: boolean }) {
  if (pending) {
    return (
      <section className="card" aria-busy="true">
        <div className="skeleton"><i /><i /><i /></div>
      </section>
    );
  }

  if (balances.length === 0) {
    return (
      <section className="empty">
        <h2>Nothing in it</h2>
        <p>{tr('msg.emptyTreasury')}</p>
      </section>
    );
  }

  return (
    <section className="card" role="region" aria-label="Assets">
      {/* Its own scroll box. Four numeric columns overflow a phone, and a
          horizontal scrollbar on the document takes the whole layout with it. */}
      <div className="y-scroll">
        <table className="table">
          <thead>
            <tr>
              <th>{tr('safe.currency')}</th>
              <th className="num">Available</th>
              <th className="num">{tr('safe.committed')}</th>
              <th className="num">Total</th>
            </tr>
          </thead>
          <tbody>
            {balances.map((b) => {
              const info = resolveDenom(b.denom);
              return (
                <tr key={b.denom}>
                  <td>
                    {/* The symbol and the name, not the base denom. `uxof` is
                        what the chain stores; "XOF — West African CFA franc" is
                        what a treasurer reconciles against. */}
                    <strong>{info.symbol}</strong>
                    <span className="small muted"> {info.name}</span>
                  </td>
                  <td className="num strong">{formatAmount(b.available, b.denom, { withSymbol: false })}</td>
                  <td className="num muted">{formatAmount(b.locked, b.denom, { withSymbol: false })}</td>
                  <td className="num muted">{formatAmount(b.total, b.denom, { withSymbol: false })}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <p className="small muted card__foot">
        Committed funds are promised to a beneficiary and cannot be spent by any proposal, including
        one that reaches the signing threshold. That is enforced by the chain, not by this page.
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

  const info = resolveDenom(denom);
  const parsed = amount.trim() === '' ? null : toBaseUnitsOf(amount, denom);
  const typedButUnreadable = amount.trim() !== '' && parsed === null;

  const verdict = checkSpend(
    { id: treasuryId, name, admin, paused, createdAtHeight: 0 },
    balances,
    parsed ? [{ denom, amount: parsed.base }] : [],
  );

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
        <span>Recipient</span>
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
          <span>Amount in {info.symbol}</span>
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

      {/* Refused at the field rather than at execution. An amount that cannot
          be parsed must never quietly become zero in the payload below. */}
      {typedButUnreadable && (
        <p className="field-note field-note--bad">
          That is not an amount. Digits and one decimal separator — a group separator
          (<code>1 250,50</code>) is fine, both separators at once is ambiguous.
        </p>
      )}
      {parsed?.truncated && (
        <p className="field-note field-note--warn">
          {info.symbol} is held to {info.exponent} decimal places on this chain, so the digits past
          that are not in the amount that will move: {formatAmount(parsed.base, denom)}.
        </p>
      )}

      {parsed && !verdict.ok && <div className="notice notice--bad">{verdict.reason}</div>}
      {parsed && verdict.ok && (
        <div className="notice notice--ok">
          This treasury can pay {formatAmount(parsed.base, denom)}.{' '}
          {formatCoins(spendable(balances))} available
          {committed(balances).length > 0 && <> · {formatCoins(committed(balances))} committed</>}.
        </div>
      )}

      {/* The proposal payload, shown rather than hidden. A signer approving a
          spend should be able to read exactly what they are approving, and an
          M-of-N group needs this JSON to submit it. */}
      <details className="payload">
        <summary>The message this produces</summary>
        <pre className="payload__pre">{JSON.stringify(message, null, 2)}</pre>
        <p className="small muted">
          Sign it directly if you hold a spender role, or put it inside an x/group proposal for an
          M-of-N treasury. Either way the chain checks the same policy.
        </p>
      </details>
    </section>
  );
}

function Members({ admin, roles }: { admin: string; roles: RoleAssignment[] }) {
  return (
    <section className="card">
      <h2>{tr('safe.members')}</h2>
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
      <p className="small muted card__foot">
        For an M-of-N treasury the admin is a group policy address rather than a person, and the
        signers are that group's members. The threshold lives in x/group; the spending limits live
        here.
      </p>
    </section>
  );
}

function Commitments({ locks, pending }: { locks: TreasuryLock[]; pending: boolean }) {
  if (pending) {
    return (
      <section className="card" aria-busy="true">
        <div className="skeleton"><i /><i /></div>
      </section>
    );
  }

  if (locks.length === 0) {
    return (
      <section className="empty">
        <h2>Nothing committed</h2>
        <p>Every coin this treasury holds is available to spend.</p>
      </section>
    );
  }

  return (
    <section className="card">
      <h2>{tr('safe.commitments')}</h2>
      <p className="small muted">
        Funds promised to somebody. They have left the spendable balance and cannot be redirected —
        a later proposal that tried would be refused by the chain.
      </p>

      <div className="y-scroll">
        <table className="table">
          <thead>
            <tr>
              <th>{tr('safe.beneficiary')}</th>
              <th className="num">Amount</th>
              <th className="num">Claimed</th>
              <th>Releases</th>
              <th>Revocable</th>
            </tr>
          </thead>
          <tbody>
            {locks.map((l) => (
              <tr key={l.id} className={l.revoked ? 'row--off' : undefined}>
                <td><Address address={l.beneficiary} /></td>
                <td className="num">{formatAmount(l.amount, l.denom)}</td>
                <td className="num muted">{formatAmount(l.claimed, l.denom, { withSymbol: false })}</td>
                {/* Human time. "Releases in 12 days" is what a beneficiary is
                    asking; the date stays in the title for anybody reconciling
                    against a contract. */}
                <td className="small">{releaseWhen(l.endTime)}</td>
                <td className="small muted">
                  {l.revoked ? 'Revoked' : l.revocable ? 'Yes' : 'No'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function releaseWhen(endTime: number | undefined) {
  if (!endTime) return <span className="muted">—</span>;
  const at = new Date(endTime * 1000);
  const iso = at.toISOString();
  const future = at.getTime() > Date.now();
  return (
    <span title={at.toLocaleString()}>{future ? `in ${timeUntil(iso)}` : timeAgo(iso)}</span>
  );
}
