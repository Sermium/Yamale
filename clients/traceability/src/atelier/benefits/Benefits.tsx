import { useState } from 'react';
import { attachJewelBenefit, t, type JewelView } from '@yamale/chain';

import { UnlockGate } from '../../account/Account.tsx';
import { useSession } from '../../account/session.tsx';
import { kindProblem } from '../shared/rules.ts';
import { ActionStatus, useAction } from '../../actions/useAction.tsx';
import { BenefitList } from '../../verify/JewelPage.tsx';

const KINDS = ['cleaning', 'resizing', 'polishing', 'rhodium', 'repair'];
const PERIODS = [
  { seconds: 31_536_000, key: 'traceability.perYear' },
  { seconds: 2_592_000, key: 'traceability.perMonth' },
  { seconds: 0, key: 'traceability.wholeLife' },
];

/** `now` is the chain's latest block time: an end date already past by it is no benefit. */
export function AttachBenefit({ view, business, now, onChange }:
  { view: JewelView; business: string; now: number; onChange: () => void }) {
  const session = useSession();
  const [action, run] = useAction();
  const [kind, setKind] = useState('cleaning');
  const [custom, setCustom] = useState('');
  const [allowanceText, setAllowanceText] = useState('2');
  const [period, setPeriod] = useState(31_536_000);
  const [until, setUntil] = useState('');
  const allowance = Math.min(100, Math.max(1, Math.floor(Number(allowanceText)) || 1));
  const finalKind = kind === 'custom' ? custom.trim() : kind;
  const kindIssue = kind === 'custom' ? kindProblem(custom) : null;
  const expiresAt = until ? Math.floor(Date.parse(`${until}T23:59:59Z`) / 1000) : 0;
  const pastEnd = until !== '' && !(expiresAt > now);
  return (
    <UnlockGate>
      <div className="stack">
        <label className="y-label" htmlFor="bkind">{t('traceability.benefitWhat')}</label>
        <select id="bkind" className="bigfield" value={kind} onChange={(e) => setKind(e.target.value)}>
          {KINDS.map((k) => <option key={k} value={k}>{t(`traceability.benefitKind.${k}`)}</option>)}
          <option value="custom">{t('traceability.benefitOther')}</option>
        </select>
        {kind === 'custom' && <input className="bigfield" maxLength={80} value={custom} onChange={(e) => setCustom(e.target.value)} aria-label={t('traceability.benefitOther')} />}
        <label className="y-label" htmlFor="ballow">{t('traceability.benefitHowMany')}</label>
        <input id="ballow" className="bigfield num" type="number" min={1} max={100} value={allowanceText}
               onChange={(e) => setAllowanceText(e.target.value)} onBlur={() => setAllowanceText(String(allowance))} />
        <label className="y-label" htmlFor="bperiod">{t('traceability.benefitEvery')}</label>
        <select id="bperiod" className="bigfield" value={period} onChange={(e) => setPeriod(Number(e.target.value))}>
          {PERIODS.map((p) => <option key={p.seconds} value={p.seconds}>{t(p.key)}</option>)}
        </select>
        <label className="y-label" htmlFor="buntil">{t('traceability.benefitUntilLabel')}</label>
        <input id="buntil" className="bigfield" type="date" value={until} onChange={(e) => setUntil(e.target.value)} />
        {kindIssue === 'long' && <p className="note note--warn" role="alert">{t('traceability.benefitKindLong')}</p>}
        {kindIssue === 'empty' && <p className="hint">{t('traceability.benefitKindEmpty')}</p>}
        {pastEnd && <p className="note note--warn" role="alert">{t('traceability.benefitEndPast')}</p>}
        <button type="button" className="btn btn--primary" disabled={action.kind === 'busy' || kindIssue !== null || pastEnd} onClick={async () => {
          if (await run(() => session.submit([attachJewelBenefit(session.address!, view.jewel.id, finalKind, allowance, period, expiresAt)], business))) onChange();
        }}>{t('traceability.attachBenefit')}</button>
        <ActionStatus state={action} done={t('traceability.benefitAttached')} />
      </div>
    </UnlockGate>
  );
}

export function JewelBenefits({ view, active, now }: { view: JewelView; active: boolean; now: number }) {
  return <BenefitList benefits={view.benefits} issuer={active ? 'active' : 'suspended'} now={now} />;
}
