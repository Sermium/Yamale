import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { formatAmount, formatCoins, resolveDenom, truncateAddress } from '@yamale/chain';

import { client } from '../chain.ts';

/**
 * One account, answering the three questions somebody actually has.
 *
 * What do I hold; can I move it; who is paying the fee. The third is not
 * decoration on this chain — fees are payable in YML, so an account full of
 * naira with no sponsor is an account that cannot move a single unit, and a
 * wallet showing only a healthy balance would be hiding the one fact that
 * matters.
 */
export function AccountPage({ address }: { address: string }) {
  const balances = useQuery({
    queryKey: ['balances', address],
    queryFn: () => client.balances(address),
  });
  const freeze = useQuery({
    queryKey: ['freeze', address],
    queryFn: () => client.freezeStatus(address),
  });
  const allowances = useQuery({
    queryKey: ['allowances', address],
    queryFn: () => client.feeAllowances(address),
  });

  const held = balances.data ?? [];
  const native = held.find((c) => c.denom === 'uyml');
  const sponsored = (allowances.data ?? []).length > 0;
  const frozen = freeze.data?.frozen === true;

  return (
    <>
      <p className="crumb">
        <Link to="/">Watch</Link> / {truncateAddress(address)}
      </p>
      <h1 className="address">{address}</h1>

      {/* Ordered by consequence. A freeze stops everything, so it is first and
          it says which case did it — an account holder refused a transfer needs
          the case number, not the word "frozen". */}
      {frozen && freeze.data?.case && (
        <div className="notice notice--bad">
          <strong>This account cannot send.</strong> Enforcement case {freeze.data.case.id} froze it:
          “{freeze.data.case.reason}”. It can still receive. The case is public — anyone can read the
          grounds and how the validators voted.
        </div>
      )}

      {!frozen && !native && !sponsored && held.length > 0 && (
        <div className="notice">
          <strong>No YML, and nobody sponsoring the fees.</strong> Network fees are payable in YML,
          so this account cannot move what it holds until an institution grants it a fee allowance
          or somebody sends it a little YML.
        </div>
      )}

      {balances.isPending ? (
        <p className="muted">Reading the chain…</p>
      ) : held.length === 0 ? (
        <section className="card">
          <h2>Nothing here yet</h2>
          <p className="muted">
            This account holds no money. That is also what an address that has never been used looks
            like — the chain does not distinguish between the two, and neither does this page.
          </p>
        </section>
      ) : (
        <section className="card" aria-label="Balances">
          <h2>Holds</h2>
          <table className="table">
            <tbody>
              {held.map((coin) => {
                const info = resolveDenom(coin.denom);
                return (
                  <tr key={coin.denom}>
                    <td className="denom">
                      <strong>{info.symbol}</strong>
                      <span className="small muted"> {info.name}</span>
                    </td>
                    <td className="num">{formatAmount(coin.amount, coin.denom, { withSymbol: false })}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </section>
      )}

      <section className="card">
        <h2>Fees</h2>
        {sponsored ? (
          (allowances.data ?? []).map((a) => (
            <p key={a.granter}>
              <strong>{truncateAddress(a.granter)}</strong> is paying this account's network fees,
              up to {formatCoins(a.spendLimit)}
              {a.expiration ? ` until ${new Date(a.expiration).toLocaleDateString()}` : ''}.
            </p>
          ))
        ) : native ? (
          <p className="muted">
            This account pays its own fees from its YML balance. A payment costs a fraction of a
            cent.
          </p>
        ) : (
          <p className="muted">
            Nobody is sponsoring this account and it holds no YML, so it cannot send anything yet.
          </p>
        )}
      </section>
    </>
  );
}
