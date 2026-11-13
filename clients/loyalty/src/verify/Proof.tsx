/**
 * /own/<jewel>?t=…&k=…&s=…: a link an owner shares to show they hold a jewel,
 * for an insurer or a buyer. It proves the person who made the link holds the
 * key that holds the jewel now. It names nobody.
 */

import { Link, useLocation, useParams } from 'react-router-dom';
import { readJewel, t, verifyOwnershipProof } from '@yamale/chain';

import { CHAIN_ID } from '../config.ts';
import { AppBar, day, Loading, Problem, useLoad } from '../ui.tsx';
import { Hallmark } from './JewelPage.tsx';

export function Proof() {
  const { jewel: jewelId = '' } = useParams();
  const { search } = useLocation();
  const proof = verifyOwnershipProof(CHAIN_ID, jewelId, search);
  const [view] = useLoad(() => readJewel(jewelId), [jewelId]);

  return (
    <>
      <AppBar title={t('loyalty.proofTitle')} eyebrow={t('loyalty.title')} back />
      <main className="y-screen">
        {!proof && <div className="banner banner--bad" role="alert">{t('loyalty.proofInvalid')}</div>}
        {proof && !view && <Loading />}
        {proof && view && !view.ok && <Problem outcome={view} />}
        {proof && view?.ok && (
          <>
            <Hallmark code={view.value.jewel.inscription} />
            {view.value.owner === proof.address
              ? <div className="banner banner--ok" role="status">{t('loyalty.proofHolds')}</div>
              : <div className="banner banner--warn" role="status">{t('loyalty.proofNoLonger')}</div>}
            <p className="hint">{t('loyalty.proofIssued', { date: day(proof.issuedAt) })}</p>
            <Link className="btn" to={`/j/${view.value.jewel.jewelerId}/${view.value.jewel.id}`}>{t('loyalty.seeJewel')}</Link>
          </>
        )}
      </main>
    </>
  );
}
