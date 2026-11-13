/**
 * The business adds and removes its staff, and pays their fees for staff
 * work only. Adding and granting go in one transaction, so a staff member is
 * never on the register without the means to act, and never the other way
 * round.
 */

import { fromBech32, toBech32 } from '@cosmjs/encoding';
import { useState } from 'react';
import { feeAllowanceExists, grantStaffFees, revokeFeeAllowance, setJewelStaff, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { AccountCodeInput } from '../owner/OfferPanel.tsx';
import { isAccountCode } from '../owner/links.ts';
import { AppBar, Loading, Problem, Section } from '../ui.tsx';
import { useRole } from './role.ts';

const STAFF_BUDGET = [{ denom: 'uyml', amount: '5000000' }];
/** The one spelling of an account code: lowercase bech32, so two spellings of one account compare equal. */
function canonical(code: string): string {
  const { prefix, data } = fromBech32(code.trim());
  return toBech32(prefix, data);
}
const short = (a: string) => `${a.slice(0, 8)}…${a.slice(-4)}`;

export function Staff() {
  const session = useSession();
  const [role, reloadRole] = useRole(session.address);
  const [code, setCode] = useState('');
  const [action, run] = useAction();
  const [refused, setRefused] = useState(false);

  const bar = <AppBar title={t('loyalty.staffTitle')} eyebrow="" back />;
  if (role.state === 'loading') return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (role.state === 'failed') return <>{bar}<main className="y-screen"><Problem outcome={role.outcome} /></main></>;
  if (role.state !== 'role' || !role.business) {
    return <>{bar}<main className="y-screen"><p className="note">{t('loyalty.businessOnly')}</p></main></>;
  }
  const j = role.jeweler;
  const me = session.address!;

  async function add() {
    setRefused(false);
    const who = canonical(code);
    const msgs = [setJewelStaff(me, j.id, [who], [])];
    // The business pays its own fees: x/feegrant refuses a grant to oneself,
    // and that refusal would fail the whole transaction.
    if (who !== me) {
      const has = await feeAllowanceExists(me, who);
      if (!has.ok) { setRefused(true); return; }
      if (!has.value) msgs.push(grantStaffFees(me, who, STAFF_BUDGET, new Date(Date.now() + 365 * 86_400_000)));
    }
    if (await run(() => session.submit(msgs, null))) { setCode(''); reloadRole(); }
  }

  async function remove(who: string) {
    setRefused(false);
    const msgs = [setJewelStaff(me, j.id, [], [who])];
    if (who !== me) {
      // If the allowance cannot be read, removing without revoking would leave the person able to spend.
      const has = await feeAllowanceExists(me, who);
      if (!has.ok) { setRefused(true); return; }
      if (has.value) msgs.push(revokeFeeAllowance(me, who));
    }
    if (await run(() => session.submit(msgs, null))) reloadRole();
  }

  return (
    <>
      <AppBar title={t('loyalty.staffTitle')} eyebrow={j.name} back />
      <main className="y-screen">
        <UnlockGate>
          <Section title={t('loyalty.staffNow')}>
            {j.staff.length === 0 && <p className="hint">{t('loyalty.noStaff')}</p>}
            <ul className="y-cards">
              {j.staff.map((s) => (
                <li key={s} className="y-card staff-row">
                  <span className="inscription">{short(s)}{s === me ? ` · ${t('loyalty.you')}` : ''}</span>
                  <button type="button" className="btn btn--danger" disabled={action.kind === 'busy'} onClick={() => remove(s)}>{t('loyalty.remove')}</button>
                </li>
              ))}
            </ul>
          </Section>
          <Section title={t('loyalty.addStaff')}>
            <AccountCodeInput id="staff-code" value={code} onChange={setCode} />
            <p className="hint">{t('loyalty.addStaffExplain')}</p>
            <button type="button" className="btn btn--primary" disabled={action.kind === 'busy' || !isAccountCode(code)} onClick={add}>
              {t('loyalty.addStaffButton')}
            </button>
          </Section>
          {refused && <p className="note note--warn" role="alert">{t('loyalty.staffReadFailed')}</p>}
          <ActionStatus state={action} />
          <details className="advanced"><summary>{t('loyalty.advanced')}</summary><pre>{JSON.stringify(j, null, 2)}</pre></details>
        </UnlockGate>
      </main>
    </>
  );
}
