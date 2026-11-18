/**
 * /own/<jewel>?t=…&k=…&s=…: a link an owner shares to show they hold a jewel,
 * for an insurer or a buyer. It proves the link was made, on its date, by the
 * account that holds the jewel now. It is not bound to a sender or a window,
 * so anyone can forward it: an old one is flagged, and a sale wants a fresh one.
 * It names nobody.
 */

import { Link, useLocation, useParams } from 'react-router-dom';
import { chainHead, readJewel, t, verifyOwnershipProof } from '@yamale/chain';

import { CHAIN_ID } from '../config.ts';
import { AppBar, day, isId, Loading, NotFound, Problem, useLoad } from '../ui.tsx';
import { Hallmark } from './JewelPage.tsx';

export function Proof() {
  const { jewel: jewelId = '' } = useParams();
  const { search } = useLocation();
  const proof = verifyOwnershipProof(CHAIN_ID, jewelId, search);
  const valid = isId(jewelId);
  const [head] = useLoad(() => chainHead(), []);
  const [view] = useLoad(() => (valid ? readJewel(jewelId) : Promise.resolve(null as never)), [jewelId, valid]);
  if (!valid) return <NotFound />;
  // By the chain's clock; if it cannot be read the age is simply not claimed.
  const age = proof && head?.ok ? head.value.time - proof.issuedAt : null;

  return (
    <>
      <AppBar title={t('traceability.proofTitle')} eyebrow={t('traceability.title')} back />
      <main className="y-screen">
        {!proof && <div className="banner banner--bad" role="alert">{t('traceability.proofInvalid')}</div>}
        {proof && !view && <Loading />}
        {proof && view && !view.ok && <Problem outcome={view} />}
        {proof && view?.ok && (
          <>
            <Hallmark code={view.value.jewel.inscription} />
            {view.value.owner === proof.address
              ? <div className="banner banner--ok" role="status">{t('traceability.proofHolds', { date: day(proof.issuedAt) })}</div>
              : <div className="banner banner--warn" role="status">{t('traceability.proofNoLonger')}</div>}
            <p className="num"><strong>{t('traceability.proofIssued', { date: day(proof.issuedAt) })}</strong></p>
            {age !== null && age > 600 && (
              <span className="y-chip y-chip--warn">
                {age >= 7200
                  ? t('traceability.proofAgeHours', { n: Math.floor(age / 3600) })
                  : t('traceability.proofAgeMinutes', { n: Math.floor(age / 60) })}
              </span>
            )}
            <Link className="btn" to={`/j/${view.value.jewel.jewelerId}/${view.value.jewel.id}`}>{t('traceability.seeJewel')}</Link>
          </>
        )}
      </main>
    </>
  );
}
