/**
 * Claims to validate: buyers who scanned an ownership card. Validating moves
 * the jewel to the claimer at once; it is the shop saying "this person paid".
 * Rejecting leaves the card valid, so the real buyer can claim with it.
 */

import { useState } from 'react';
import { approveJewelClaim, claimsByJeweler, rejectJewelClaim, t, type Claim } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { day, Problem, Section, useLoad } from '../ui.tsx';
import { JewelCard } from '../verify/JewelCard.tsx';

export function ClaimsToValidate({ jewelerId, business, canAct, now, onChange }: {
  jewelerId: string; business: string; canAct: boolean; now: number; onChange: () => void;
}) {
  const [claims, reload] = useLoad(() => claimsByJeweler(jewelerId), [jewelerId]);
  if (!claims) return null;
  if (!claims.ok) return <Problem outcome={claims} />;
  if (claims.value.length === 0) return null;
  return (
    <Section title={t('traceability.claimsToValidate', { n: claims.value.length })}>
      <ul className="y-cards">
        {claims.value.map(({ jewel, claim }) => (
          <JewelCard key={jewel.id} id={jewel.id} now={now} to={`/atelier/jewel/${jewel.id}`}
                     extra={<ClaimDecision jewelId={jewel.id} claim={claim} business={business} canAct={canAct}
                                           onDone={() => { reload(); onChange(); }} />} />
        ))}
      </ul>
    </Section>
  );
}

/** Who claimed, when, and the shop's two answers. */
export function ClaimDecision({ jewelId, claim, business, canAct, onDone }: {
  jewelId: string; claim: Claim; business: string; canAct: boolean; onDone: () => void;
}) {
  const session = useSession();
  const [action, run] = useAction();
  const [asking, setAsking] = useState<'approve' | 'reject' | null>(null);
  const who = (
    <p className="claimrow__who">
      <span>{t('traceability.claimedBy')}</span>
      <code className="inscription">{claim.claimer}</code>
      <span className="hint">{t('traceability.claimedOn', { date: day(claim.filedAt) })}</span>
    </p>
  );
  if (!canAct) return <div className="claimrow">{who}<p className="hint">{t('traceability.staffOnly')}</p></div>;
  return (
    <div className="claimrow">
      {who}
      {asking === null && (
        <div className="actions">
          <button type="button" className="btn btn--primary" onClick={() => setAsking('approve')}>{t('traceability.claimValidate')}</button>
          <button type="button" className="btn" onClick={() => setAsking('reject')}>{t('traceability.claimReject')}</button>
        </div>
      )}
      {asking !== null && (
        <UnlockGate>
          <p className="note note--warn">{t(asking === 'approve' ? 'traceability.claimValidateExplain' : 'traceability.claimRejectExplain')}</p>
          <div className="actions">
            <button type="button" className={`btn ${asking === 'approve' ? 'btn--primary' : 'btn--danger'}`} disabled={action.kind === 'busy'}
                    onClick={async () => {
                      const m = asking === 'approve' ? approveJewelClaim(session.address!, jewelId) : rejectJewelClaim(session.address!, jewelId);
                      if (await run(() => session.submit([m], business))) onDone();
                    }}>
              {t(asking === 'approve' ? 'traceability.claimValidateConfirm' : 'traceability.claimRejectConfirm')}
            </button>
            <button type="button" className="btn" onClick={() => setAsking(null)}>{t('traceability.cancel')}</button>
          </div>
          <ActionStatus state={action} />
        </UnlockGate>
      )}
    </div>
  );
}
