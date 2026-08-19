import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { formatCoins, spendable, committed, truncateAddress, t} from '@yamale/chain';

import { client } from '../chain.ts';
import { Named } from '../Named.tsx';

/**
 * Every treasury on the chain, with the two numbers that matter side by side.
 *
 * Available first, committed second, and the total nowhere — because the total
 * is the number that misleads. A treasury holding 100,000 with 90,000 vested to
 * staff has 10,000 to spend, and a list that led with 100,000 would be inviting
 * somebody to propose a payment that cannot clear.
 */
export function TreasuriesPage() {
  const treasuries = useQuery({ queryKey: ['treasuries'], queryFn: () => client.treasuries() });

  if (treasuries.isPending) return <p className="muted">Loading treasuries…</p>;
  if (treasuries.isError) {
    return (
      <section className="card">
        <h1>{t('safe.cannotReach')}</h1>
        <p className="muted">
          The node this safe points at did not answer. Nothing is wrong with your treasuries — this
          interface simply cannot see them right now.
        </p>
      </section>
    );
  }

  const list = treasuries.data ?? [];

  return (
    <>
      <h1>{t('safe.treasuries')}</h1>
      <p className="lede">
        Shared funds with named signers, spending limits, and commitments that a later proposal
        cannot touch — not even one that reaches the signing threshold.
      </p>

      {list.length === 0 ? (
        <section className="card">
          <h2>{t('safe.noTreasuries')}</h2>
          <p className="muted">
            A treasury is created with <code>tx treasury create-treasury</code>. Creating one is
            permissionless: an empty treasury grants nobody anything, and gating creation behind
            governance would make the feature unusable for the teams it is for.
          </p>
        </section>
      ) : (
        <div className="grid">
          {list.map((t) => (
            <TreasuryCard key={t.id} id={t.id} name={t.name} admin={t.admin} paused={t.paused} />
          ))}
        </div>
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

  const list = balances.data ?? [];

  return (
    <Link to={`/treasury/${id}`} className="card card--link">
      <div className="card__head">
        <h2>{name || `Treasury ${id}`}</h2>
        {paused && <span className="badge badge-paused">{t('safe.paused')}</span>}
      </div>

      <dl className="figures">
        <div>
          <dt>{t('safe.availableToSpend')}</dt>
          <dd className="figure">{formatCoins(spendable(list))}</dd>
        </div>
        <div>
          <dt>{t('safe.committed')}</dt>
          <dd className="figure figure--muted">{formatCoins(committed(list))}</dd>
        </div>
      </dl>

      <p className="small muted">Admin <Named address={admin} /></p>
    </Link>
  );
}
