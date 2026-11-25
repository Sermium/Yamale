/**
 * Moving the shop's business account to an account of its own. The business
 * account holds the stock, so a person whose own account runs the shop can
 * never own one of its pieces; handing the shop to its own account fixes that.
 * The business proposes here (Staff page); the named account accepts from the
 * Atelier, on its own device. Nothing moves until it does.
 */

import { fromBech32, toBech32 } from '@cosmjs/encoding';
import { useState } from 'react';
import { acceptBusinessMove, chainHead, grantStaffFees, proposeBusinessMove, readJewelers, t, type Jeweler } from '@yamale/chain';

import { UnlockGate } from '../../account/Account.tsx';
import { useSession } from '../../account/session.tsx';
import { ActionStatus, useAction } from '../../actions/useAction.tsx';
import { AccountCodeInput } from '../../owner/OfferPanel.tsx';
import { isAccountCode } from '../../owner/links.ts';
import { Section, useLoad } from '../../ui.tsx';

/** What the new business account grants each member of staff, as the Staff page does when adding one. */
export const STAFF_BUDGET = [{ denom: 'uyml', amount: '5000000' }];
const YEAR = 365 * 86_400_000;

const short = (a: string) => `${a.slice(0, 8)}…${a.slice(-4)}`;
const date = (unix: string) => new Date(Number(unix) * 1000).toLocaleDateString();

/** The business account's side: propose a move, or withdraw it. */
export function ProposeBusinessMove({ jeweler, me, isStaff, onChange }: {
  jeweler: Jeweler; me: string; isStaff: boolean; onChange: () => void;
}) {
  const session = useSession();
  const [code, setCode] = useState('');
  const [action, run] = useAction();
  const pending = jeweler.pendingBusiness;

  async function propose(to: string) {
    if (await run(() => session.submit([proposeBusinessMove(me, jeweler.id, to)], null))) { setCode(''); onChange(); }
  }

  return (
    <Section title={t('traceability.shopMoveTitle')}>
      <p className="hint">{t('traceability.shopMoveExplain')}</p>
      {!isStaff && <p className="note note--warn">{t('traceability.shopMoveNotStaff')}</p>}
      {pending ? (
        <>
          <p className="note">{t('traceability.shopMovePending', { account: short(pending), date: date(jeweler.pendingBusinessUntil) })}</p>
          <button type="button" className="btn" disabled={action.kind === 'busy'} onClick={() => propose('')}>
            {t('traceability.shopMoveWithdraw')}
          </button>
        </>
      ) : (
        <>
          <AccountCodeInput id="move-code" value={code} onChange={setCode} />
          <button type="button" className="btn btn--primary" disabled={action.kind === 'busy' || !isAccountCode(code)}
                  onClick={() => { const { prefix, data } = fromBech32(code.trim()); propose(toBech32(prefix, data)); }}>
            {t('traceability.shopMoveButton')}
          </button>
        </>
      )}
      <ActionStatus state={action} />
    </Section>
  );
}

/**
 * The named account's side, shown in the Atelier: accept, and in the same
 * transaction give each member of staff the fee allowance the old account gave.
 */
export function BusinessMoveInvite({ me, onDone }: { me: string; onDone: () => void }) {
  const session = useSession();
  const [jewelers] = useLoad(() => readJewelers(), [me]);
  const [head] = useLoad(() => chainHead(), []);
  const [action, run] = useAction();
  if (!jewelers?.ok || !head?.ok) return null;
  const now = head.value.time;
  const j = jewelers.value.find((x) => x.pendingBusiness === me && Number(x.pendingBusinessUntil) >= now);
  if (!j) return null;

  async function accept(shop: Jeweler) {
    // A brand-new account must exist on the chain before it can sign: the platform's sponsorship creates it.
    await session.ensureSponsored().catch(() => 'unavailable');
    const until = new Date(Date.now() + YEAR);
    const msgs = [
      acceptBusinessMove(me, shop.id),
      ...shop.staff.filter((s) => s !== me).map((s) => grantStaffFees(me, s, STAFF_BUDGET, until)),
    ];
    if (await run(() => session.submit(msgs, null))) onDone();
  }

  return (
    <div className="banner banner--warn stack" role="status">
      <strong>{t('traceability.shopInviteTitle', { shop: j.name })}</strong>
      <p>{t('traceability.shopInviteExplain')}</p>
      <UnlockGate>
        <button type="button" className="btn btn--primary" disabled={action.kind === 'busy'} onClick={() => accept(j)}>
          {t('traceability.shopInviteAccept')}
        </button>
      </UnlockGate>
      <ActionStatus state={action} />
    </div>
  );
}
