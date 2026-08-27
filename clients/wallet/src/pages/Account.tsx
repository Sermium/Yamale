import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import {
  formatAmount,
  formatCoins,
  placementVerdict,
  resolveDenom,
  t,
  translateError,
  truncateAddress,
} from '@yamale/chain';

import { client } from '../chain.ts';
import { Identifier } from '../Identifier.tsx';
import { Named } from '../Named.tsx';

/**
 * One account, answering the three questions somebody actually has.
 *
 * What do I hold; can I move it; who is paying the fee. The third is not
 * decoration on this chain — fees are payable in YML, so an account full of
 * naira with no sponsor is an account that cannot move a single unit, and a
 * wallet showing only a healthy balance would be hiding the one fact that
 * matters.
 *
 * Every one of those questions has a fourth answer this page used to be unable
 * to give: **unknown**. `balances.data ?? []` renders a node that did not
 * respond identically to an account with nothing in it, and the two are not the
 * same fact — one of them means "you have not been paid", the other means "we
 * could not ask". The same substitution silently hid a freeze: with the query
 * failed, `freeze.data?.frozen === true` is false, so a frozen account rendered
 * as a healthy one.
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
  // Both, because they are two facts that can disagree and the disagreement is
  // itself worth reporting: the chain issues an identifier when it records a
  // country, so a country with no identifier is a fault rather than a state.
  const placement = useQuery({
    queryKey: ['placement', address],
    queryFn: async () => ({
      country: await client.jurisdictionOf(address),
      userId: await client.userIdOf(address),
    }),
  });

  const held = balances.data ?? [];
  const native = held.find((c) => c.denom === 'uyml');
  const sponsored = (allowances.data ?? []).length > 0;
  const frozen = freeze.data?.frozen === true;

  return (
    <>
      <p className="crumb">
        <Link to="/">{t('nav.lookUp')}</Link> / {truncateAddress(address)}
      </p>

      {/* The name if the chain issued one, the truncation otherwise, and the
          full address one click away either way.
          Not a full bech32 as the heading: 43 characters wrap to three lines on
          a phone before saying anything a person can recognise. And not a name
          above a second line carrying the address, which was the first attempt
          — on an account with no user ID both render the same truncation, so
          the page opened with the same string twice. */}
      <h1 className="account__who">
        <Identifier value={address} />
      </h1>

      {/* Ordered by consequence. A freeze stops everything, so it is first and
          it says which case did it — an account holder refused a transfer needs
          the case number, not the word "frozen". */}
      {frozen && freeze.data?.case && (
        <div className="notice notice--bad">
          <strong>{t('acct.cannotSend')}</strong> {t('acct.frozenBy', { id: freeze.data.case.id })}{' '}
          “{freeze.data.case.reason}”. {t('acct.canStillReceive')}
        </div>
      )}

      {/* A freeze that could not be read is not the absence of a freeze. This
          is the one query on the page whose failure is dangerous in the
          permissive direction, so it says so rather than staying quiet. */}
      {freeze.isError && <Unknown what={t('acct.freezeUnknown')} error={freeze.error} />}

      {placement.data && placement.data.country === null && (
        <div className="notice">
          <strong>{placementVerdict(placement.data).headline}.</strong>{' '}
          {placementVerdict(placement.data).consequence}{' '}
          {placementVerdict(placement.data).remedy}
        </div>
      )}

      {placement.data?.country && !placement.data.userId && (
        <div className="notice notice--bad">
          <strong>{placementVerdict(placement.data).headline}.</strong>{' '}
          {placementVerdict(placement.data).consequence}{' '}
          {placementVerdict(placement.data).remedy}
        </div>
      )}

      {/* Only asserted when both facts were actually read. Saying "no YML and
          nobody sponsoring you" on the strength of two failed queries would be
          inventing a problem. */}
      {!frozen &&
        balances.isSuccess &&
        allowances.isSuccess &&
        !native &&
        !sponsored &&
        held.length > 0 && (
          <div className="notice">
            <strong>{t('acct.noYmlTitle')}</strong> {t('acct.noYmlBody')}
          </div>
        )}

      {balances.isPending ? (
        <section className="card" aria-busy="true">
          <div className="skeleton"><i /><i /><i /></div>
          <p className="small muted" role="status">{t('acct.reading')}</p>
        </section>
      ) : balances.isError ? (
        <section className="card">
          <h2>{t('acct.balancesUnknown')}</h2>
          <Unknown what={t('acct.balancesUnknownBody')} error={balances.error} />
          <p>
            <button type="button" className="chip" onClick={() => balances.refetch()}>
              {t('acct.tryAgain')}
            </button>
          </p>
        </section>
      ) : held.length === 0 ? (
        <section className="card">
          <h2>{t('acct.nothingHere')}</h2>
          <p className="muted">{t('acct.nothingHereBody')}</p>
        </section>
      ) : (
        <section className="card" aria-label="Balances">
          <h2>{t('acct.holds')}</h2>
          <div className="y-scroll">
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
                      <td className="num">
                        {formatAmount(coin.amount, coin.denom, { withSymbol: false })}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>
      )}

      <section className="card">
        <h2>{t('acct.fees')}</h2>
        {allowances.isPending ? (
          <div className="skeleton"><i /></div>
        ) : allowances.isError ? (
          <Unknown what={t('acct.feesUnknown')} error={allowances.error} />
        ) : sponsored ? (
          (allowances.data ?? []).map((a) => (
            <p key={a.granter}>
              <Named address={a.granter} /> {t('acct.isPaying')} {formatCoins(a.spendLimit)}
              {a.expiration ? ` ${t('acct.until', { date: new Date(a.expiration).toLocaleDateString() })}` : ''}.
            </p>
          ))
        ) : native ? (
          <p className="muted">{t('acct.paysOwn')}</p>
        ) : (
          <p className="muted">{t('acct.noSponsor')}</p>
        )}
      </section>
    </>
  );
}

/**
 * A question that could not be asked.
 *
 * Deliberately not styled as a failure of the account. Nothing is wrong with
 * the money; the interface simply could not see it, and the difference matters
 * enough to be the whole content of this component. The raw fault stays behind
 * a disclosure, because "it did not work" is not a bug report anyone can act
 * on.
 */
function Unknown({ what, error }: { what: string; error: unknown }) {
  const translated = translateError(error instanceof Error ? error.message : String(error));
  return (
    <div className="notice">
      <strong>{what}</strong> {translated.message}. {translated.reason}
      <details className="payload">
        <summary>{t('sign.whatTheChainSaid')}</summary>
        <pre className="payload__pre">{translated.raw}</pre>
      </details>
    </div>
  );
}
