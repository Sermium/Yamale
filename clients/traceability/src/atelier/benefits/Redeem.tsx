/**
 * Redeeming a benefit: the owner shows a code, the staff scan it.
 *
 * Everything the chain will check is checked here first (see redeem.ts), so a
 * refusal is explained at the counter rather than discovered after signing:
 * whose benefit it is, who signed the code, whether it is still fresh by the
 * chain's clock, and whether any uses are left.
 */

import { useRef, useState } from 'react';
import {
  chainHead, decodeVoucherCode, readJewel, redeemJewelBenefit, t, voucherSigner, type VoucherCode,
} from '@yamale/chain';

import { Scanner } from '../../../../app/src/scan.tsx';
import { UnlockGate } from '../../account/Account.tsx';
import { useSession } from '../../account/session.tsx';
import { ActionStatus, useAction } from '../../actions/useAction.tsx';
import { CHAIN_ID } from '../../config.ts';
import { AppBar, Loading, Problem, Section } from '../../ui.tsx';
import { benefitKind, Hallmark } from '../../verify/JewelPage.tsx';
import { checkRedeem } from './redeem.ts';
import { useRole } from '../shared/role.ts';

type Check =
  | { kind: 'idle' } | { kind: 'checking' }
  | { kind: 'refused'; key: string }
  | { kind: 'ready'; code: VoucherCode; inscription: string; benefitKind: string; left: number };

function ReadyToRecord({ check, business }: { check: Extract<Check, { kind: 'ready' }>; business: string }) {
  const session = useSession();
  const [action, run] = useAction();
  return (
    <div className="stack">
      <Hallmark code={check.inscription} />
      <p>{t('traceability.redeemSummary', { kind: benefitKind(check.benefitKind), left: check.left })}</p>
      <button type="button" className="btn btn--primary" disabled={action.kind === 'busy' || action.kind === 'done'} onClick={() =>
        run(() => session.submit([redeemJewelBenefit(session.address!, check.code)], business))}>
        {t('traceability.redeemConfirm')}
      </button>
      <ActionStatus state={action} done={t('traceability.redeemDone')} />
    </div>
  );
}

export function Redeem() {
  const session = useSession();
  const [role] = useRole(session.address);
  const [text, setText] = useState('');
  const [scanning, setScanning] = useState(false);
  const [check, setCheck] = useState<Check>({ kind: 'idle' });
  // Typing fires a check per keystroke; only the latest one may speak.
  const latest = useRef(0);

  const bar = (eyebrow: string) => <AppBar title={t('traceability.redeemTitle')} eyebrow={eyebrow} back />;
  if (role.state === 'loading') return <>{bar('')}<main className="y-screen"><Loading /></main></>;
  if (role.state === 'failed') return <>{bar('')}<main className="y-screen"><Problem outcome={role.outcome} /></main></>;
  if (role.state !== 'role' || !role.staff || !role.active) {
    return <>{bar('')}<main className="y-screen"><p className="note">{t('traceability.staffOnly')}</p></main></>;
  }
  const jeweler = role.jeweler;

  async function inspect(raw: string) {
    const mine = ++latest.current;
    const say = (c: Check) => { if (mine === latest.current) setCheck(c); };
    setText(raw);
    if (raw.trim() === '') { say({ kind: 'idle' }); return; }
    const code = decodeVoucherCode(raw);
    if (!code) { say({ kind: 'refused', key: 'traceability.voucherUnreadable' }); return; }
    say({ kind: 'checking' });
    try {
      const [view, head] = await Promise.all([readJewel(code.voucher.jewelId), chainHead()]);
      if (!view.ok || !head.ok) { say({ kind: 'refused', key: 'traceability.problemUnreachable' }); return; }
      const benefit = view.value.benefits.find((b) => b.id === code.voucher.benefitId);
      const verdict = checkRedeem({
        benefit, voucherJewelId: code.voucher.jewelId, jewelerId: jeweler.id, signer: voucherSigner(code, CHAIN_ID), owner: view.value.owner,
        codeExpiresAt: Number(code.voucher.expiresAt), stolen: view.value.jewel.stolen, now: head.value.time,
      });
      if (!verdict.ok) { say({ kind: 'refused', key: verdict.key }); return; }
      say({ kind: 'ready', code, inscription: view.value.jewel.inscription, benefitKind: benefit!.kind, left: verdict.left });
    } catch {
      say({ kind: 'refused', key: 'traceability.problemUnreachable' });
    }
  }

  return (
    <>
      {bar(jeweler.name)}
      <main className="y-screen">
        <UnlockGate>
          <Section title={t('traceability.redeemScan')}>
            <div className="stack">
            {scanning
              ? <Scanner onRead={(s) => { setScanning(false); void inspect(s); }} onClose={() => setScanning(false)} />
              : <button type="button" className="btn btn--primary" onClick={() => setScanning(true)}>{t('traceability.scanCode')}</button>}
            <label className="y-label" htmlFor="vtext">{t('traceability.orPaste')}</label>
            <textarea id="vtext" className="bigfield" rows={3} value={text} onChange={(e) => void inspect(e.target.value)} spellCheck={false} />
            </div>
          </Section>
          {check.kind === 'checking' && <p className="loading">{t('traceability.loading')}</p>}
          {check.kind === 'refused' && <p className="note note--warn" role="alert">{t(check.key)}</p>}
          {check.kind === 'ready' && <ReadyToRecord key={text} check={check} business={jeweler.business} />}
        </UnlockGate>
      </main>
    </>
  );
}
