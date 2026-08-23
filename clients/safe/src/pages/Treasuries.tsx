import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { committed, formatCoins, spendable, t } from '@yamale/chain';

import { client } from '../chain.ts';
import { Address } from '../Address.tsx';

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

  return (
    <>
      <h1>{t('safe.treasuries')}</h1>
      <p className="lede">
        Shared funds with named signers, spending limits, and commitments that a later proposal
        cannot touch — not even one that reaches the signing threshold.
      </p>

      {/* Waiting, and the two ways of not having an answer, in the order they
          happen. A chain UI spends most of its life in one of these three
          states, and only the third one used to have a design. */}
      {treasuries.isPending && (
        <div className="grid">
          {[0, 1, 2].map((n) => (
            <section className="card" key={n} aria-hidden="true">
              <div className="skeleton">
                <i /><i /><i />
              </div>
            </section>
          ))}
          <p className="small muted" role="status">
            Reading the chain…
          </p>
        </div>
      )}

      {treasuries.isError && (
        <section className="card">
          <h2>{t('safe.cannotReach')}</h2>
          <p className="muted">
            The node this safe points at did not answer. Nothing is wrong with your treasuries —
            this interface simply cannot see them right now.
          </p>
          <p className="small muted">
            <button type="button" className="chip" onClick={() => treasuries.refetch()}>
              Try again
            </button>
          </p>
          {/* The raw fault behind a disclosure. A treasurer does not need it; the
              person they telephone does, and "it did not work" is not a bug
              report anybody can act on. */}
          <details className="payload">
            <summary>What the node said</summary>
            <pre className="payload__pre">
              {treasuries.error instanceof Error ? treasuries.error.message : String(treasuries.error)}
            </pre>
          </details>
        </section>
      )}

      {treasuries.isSuccess &&
        ((treasuries.data ?? []).length === 0 ? (
          <section className="empty">
            <h2>{t('safe.noTreasuries')}</h2>
            <p>
              A treasury is created with <code>tx treasury create-treasury</code>. Creating one is
              permissionless: an empty treasury grants nobody anything, and gating creation behind
              governance would make the feature unusable for the teams it is for.
            </p>
          </section>
        ) : (
          <div className="grid">
            {treasuries.data.map((treasury) => (
              <TreasuryCard
                key={treasury.id}
                id={treasury.id}
                name={treasury.name}
                admin={treasury.admin}
                paused={treasury.paused}
              />
            ))}
          </div>
        ))}
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
    <section className="card">
      <div className="card__head">
        <h2>
          <Link to={`/treasury/${id}`}>{name || `Treasury ${id}`}</Link>
        </h2>
        {/* The word as well as the colour. A frozen treasury read as an amber
            dot is a frozen treasury nobody noticed. */}
        {paused && <span className="badge badge-paused">{t('safe.paused')}</span>}
      </div>

      <dl className="figures">
        <div>
          <dt>{t('safe.availableToSpend')}</dt>
          <dd className="figure">
            {balances.isPending ? <span className="muted">…</span> : formatCoins(spendable(list))}
          </dd>
        </div>
        <div>
          <dt>{t('safe.committed')}</dt>
          <dd className="figure figure--muted">
            {balances.isPending ? <span className="muted">…</span> : formatCoins(committed(list))}
          </dd>
        </div>
      </dl>

      <p className="small muted">
        {t('safe.admin')} <Address address={admin} />
      </p>
    </section>
  );
}
