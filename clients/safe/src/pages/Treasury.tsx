import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import {
  ROLE_LABELS,
  checkSpend,
  committed,
  formatAmount,
  formatCoins,
  formatTimestamp,
  spendable,
  truncateAddress, t as tr,} from '@yamale/chain';

import { client } from '../chain.ts';
import { Named } from '../Named.tsx';

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

  if (treasury.isPending) return <p className="muted">Loading…</p>;
  if (!treasury.data) {
    return (
      <section className="card">
        <h1>No treasury {id}</h1>
        <p className="muted">
          <Link to="/">Back to the list</Link>
        </p>
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
        Admin <Named address={t.admin} /> · opened at block {t.createdAtHeight.toLocaleString()}
      </p>

      {t.paused && (
        <div className="notice">
          This treasury is paused. Nothing can leave it until an admin unpauses it — proposals can
          still be made, and will fail at execution.
        </div>
      )}

      <nav className="tabs">
        {(
          [
            ['assets', 'Assets'],
            ['spend', 'Pay someone'],
            ['members', 'Members'],
            ['commitments', 'Commitments'],
          ] as [Tab, string][]
        ).map(([key, label]) => (
          <button
            key={key}
            className={tab === key ? 'tab tab--on' : 'tab'}
            onClick={() => setTab(key)}
            type="button"
          >
            {label}
          </button>
        ))}
      </nav>

      {tab === 'assets' && <Assets balances={list} />}
      {tab === 'spend' && <Spend treasuryId={id} balances={list} paused={t.paused} />}
      {tab === 'members' && <Members admin={t.admin} roles={roles.data ?? []} />}
      {tab === 'commitments' && <Commitments locks={locks.data ?? []} />}
    </>
  );
}

function Assets({ balances }: { balances: any[] }) {
  if (balances.length === 0) {
    return <p className="muted">{tr('msg.emptyTreasury')}</p>;
  }

  return (
    <section className="card" role="region" aria-label="Assets">
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
          {balances.map((b) => (
            <tr key={b.denom}>
              <td>{formatAmount('0', b.denom, { withSymbol: true }).split(' ').pop()}</td>
              <td className="num strong">{formatAmount(b.available, b.denom, { withSymbol: false })}</td>
              <td className="num muted">{formatAmount(b.locked, b.denom, { withSymbol: false })}</td>
              <td className="num muted">{formatAmount(b.total, b.denom, { withSymbol: false })}</td>
            </tr>
          ))}
        </tbody>
      </table>

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
 */
function Spend({
  treasuryId,
  balances,
  paused,
}: {
  treasuryId: string;
  balances: any[];
  paused: boolean;
}) {
  const [recipient, setRecipient] = useState('');
  const [denom, setDenom] = useState(balances[0]?.denom ?? 'uyml');
  const [amount, setAmount] = useState('');
  const [memo, setMemo] = useState('');

  const base = amount ? String(Math.round(Number(amount) * 1e6)) : '0';
  const verdict = checkSpend(
    { id: treasuryId, name: '', admin: '', paused, createdAtHeight: 0 },
    balances,
    amount ? [{ denom, amount: base }] : [],
  );

  const message = {
    '@type': '/blockchain.treasury.v1.MsgSpend',
    spender: '<your address, or the group policy>',
    treasury_id: treasuryId,
    recipient: recipient || '<recipient>',
    amount: [{ denom, amount: base }],
    memo,
  };

  return (
    <section className="card">
      <h2>{tr('safe.paySomeone')}</h2>

      <label className="field">
        <span>Recipient</span>
        <input value={recipient} onChange={(e) => setRecipient(e.target.value)} placeholder="yml1…" />
      </label>

      <div className="field-row">
        <label className="field">
          <span>Amount</span>
          <input value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="0.00" inputMode="decimal" />
        </label>
        <label className="field">
          <span>{tr('safe.currency')}</span>
          <select value={denom} onChange={(e) => setDenom(e.target.value)}>
            {balances.map((b) => (
              <option key={b.denom} value={b.denom}>
                {b.denom}
              </option>
            ))}
          </select>
        </label>
      </div>

      <label className="field">
        <span>What it is for</span>
        <input value={memo} onChange={(e) => setMemo(e.target.value)} placeholder="March hosting" />
      </label>

      {amount && !verdict.ok && <div className="notice notice--bad">{verdict.reason}</div>}
      {amount && verdict.ok && (
        <div className="notice notice--ok">
          This treasury can pay it. {formatCoins(spendable(balances))} available.
        </div>
      )}

      {/* The proposal payload, shown rather than hidden. A signer approving a
          spend should be able to read exactly what they are approving, and an
          M-of-N group needs this JSON to submit it. */}
      <details className="payload">
        <summary>The message this produces</summary>
        <pre>{JSON.stringify(message, null, 2)}</pre>
        <p className="small muted">
          Sign it directly if you hold a spender role, or put it inside an x/group proposal for an
          M-of-N treasury. Either way the chain checks the same policy.
        </p>
      </details>
    </section>
  );
}

function Members({ admin, roles }: { admin: string; roles: any[] }) {
  return (
    <section className="card">
      <h2>{tr('safe.members')}</h2>
      <table className="table">
        <tbody>
          <tr>
            <td><Named address={admin} /></td>
            <td>
              <span className="badge">{tr('safe.admin')}</span>
            </td>
          </tr>
          {roles.map((r) => (
            <tr key={`${r.address}-${r.role}`}>
              <td><Named address={r.address} /></td>
              <td>
                <span className="badge">{ROLE_LABELS[r.role] ?? r.role}</span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="small muted card__foot">
        For an M-of-N treasury the admin is a group policy address rather than a person, and the
        signers are that group's members. The threshold lives in x/group; the spending limits live
        here.
      </p>
    </section>
  );
}

function Commitments({ locks }: { locks: any[] }) {
  if (locks.length === 0) {
    return (
      <p className="muted">
        Nothing is committed. Every coin this treasury holds is available to spend.
      </p>
    );
  }

  return (
    <section className="card">
      <h2>{tr('safe.commitments')}</h2>
      <p className="small muted">
        Funds promised to somebody. They have left the spendable balance and cannot be redirected —
        a later proposal that tried would be refused by the chain.
      </p>

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
              <td><Named address={l.beneficiary} /></td>
              <td className="num">{formatAmount(l.amount, l.denom)}</td>
              <td className="num muted">{formatAmount(l.claimed, l.denom, { withSymbol: false })}</td>
              <td className="small">
                {l.endTime ? formatTimestamp(new Date(l.endTime * 1000).toISOString()) : '—'}
              </td>
              <td className="small muted">{l.revoked ? 'Revoked' : l.revocable ? 'Yes' : 'No'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
