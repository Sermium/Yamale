/**
 * The moment before a signature.
 *
 * Three actions, one dialog, and the differences between them are deliberate
 * rather than decorative:
 *
 *   - **Claim** takes income and changes nothing about what is owned. It can be
 *     done again next time the vault is funded, so it asks once and gets on
 *     with it.
 *   - **Redeem** destroys shares. It is the exit, the burn *is* the claim, and
 *     there is no later step — so it states what will be destroyed and what
 *     will be paid, in that order, and requires the word to be typed. A
 *     confirmation somebody can click through by reflex is not a confirmation.
 *   - **Dispute** takes money out of the challenger's account in the same block,
 *     and nothing in `MsgDisputeSale` says so. The bond is stated as a figure,
 *     above the button, before anything is signed.
 *
 * Each shows the exact message the chain will receive, field by field, with the
 * consequences the signed bytes do not mention marked as such.
 */
import { useState } from 'react';
import { formatAmount, resolveDenom, t } from '@yamale/chain';

import { claim as sendClaim, disputeSale, redeem as sendRedeem, type Signed } from './chain.ts';
import { messagePlan, type Draft } from './message.ts';
import { Amount, Note } from './ui.tsx';
import type { Account } from './address.ts';
import { redeemPayout } from './vehicle.ts';

/* ------------------------------------------------------- the message table */

function PlanTable({ draft }: { draft: Draft }) {
  const plan = messagePlan(draft);

  return (
    <div className="plan">
      <p className="plan__type y-mono">{plan.typeUrl}</p>
      <ol className="plan__rows">
        {plan.rows.map((row) => (
          <li key={row.field} className={`plan__row plan__row--${row.carried}`}>
            <span className="plan__field y-mono">{row.field}</span>
            <span className="plan__value">
              {row.denom
                ? <Amount amount={row.value} denom={row.denom} />
                : <span className="y-mono plan__plain">{row.value || '—'}</span>}
              {row.noteKey && <span className="plan__note">{t(row.noteKey)}</span>}
            </span>
            {/*
              Never colour alone: the word carries the distinction and the dot
              and border reinforce it, so a monochrome screenshot still reads.
            */}
            <span className={`plan__where plan__where--${row.carried}`}>
              <span className="plan__dot" aria-hidden="true" />
              {t(row.carried === 'ledger' ? 'rwa.msg.onLedger' : 'rwa.msg.derived')}
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
}

/* --------------------------------------------------------------- the frame */

type Phase = { at: 'form' } | { at: 'signing' } | { at: 'done'; result: Signed };

function Outcome({ result, onClose }: { result: Signed; onClose: () => void }) {
  if (result.ok) {
    return (
      <>
        <Note tone="ok" title={t('rwa.done')}>
          {t('rwa.doneAt', { height: result.height })}
        </Note>
        <button type="button" className="btn" onClick={onClose}>{t('rwa.close')}</button>
      </>
    );
  }

  return (
    <>
      <Note tone="bad" title={t('rwa.refused')}>
        {result.error?.message ?? t('error.unknown')}
      </Note>
      {result.error?.reason && <p className="muted">{result.error.reason}</p>}
      {result.error?.nextStep && <p className="muted"><strong>{result.error.nextStep}</strong></p>}
      {result.error?.raw && (
        <details className="raw">
          <summary className="raw__summary">{t('rwa.raw')}</summary>
          <pre className="raw__body y-mono y-scroll">{result.error.raw}</pre>
        </details>
      )}
      <button type="button" className="btn btn--ghost" onClick={onClose}>{t('action.cancel')}</button>
    </>
  );
}

function Dialog({ title, lede, tone, children, onClose }: {
  title: string;
  lede: string;
  tone: 'ok' | 'warn' | 'bad';
  children: React.ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="sheet" role="dialog" aria-modal="true" aria-label={title}>
      <div className={`sheet__box sheet__box--${tone}`}>
        <header className="sheet__head">
          <h2 className="sheet__title">{title}</h2>
          <button type="button" className="sheet__x" onClick={onClose}
                  aria-label={t('action.cancel')}>×</button>
        </header>
        <p className="sheet__lede">{lede}</p>
        {children}
      </div>
    </div>
  );
}

/* ---------------------------------------------------------------- claiming */

export function ClaimDialog({ account, assetId, owed, owedDenom, onClose, onDone }: {
  account: Extract<Account, { mode: 'connected' }>;
  assetId: string;
  owed: string;
  owedDenom: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [phase, setPhase] = useState<Phase>({ at: 'form' });

  const draft: Draft = {
    kind: 'claim', holder: account.address, assetId, owed, owedDenom,
  };

  const go = async () => {
    setPhase({ at: 'signing' });
    const result = await sendClaim(account.signer, account.address, assetId);
    setPhase({ at: 'done', result });
    if (result.ok) onDone();
  };

  return (
    <Dialog title={t('rwa.claimTitle')} lede={t('rwa.claimLede')} tone="ok" onClose={onClose}>
      <div className="sheet__headline">
        <span className="y-label">{t('rwa.youReceive')}</span>
        <Amount amount={owed} denom={owedDenom} className="sheet__figure" />
      </div>

      <Note tone="ok">{t('rwa.claimKeeps')}</Note>

      <PlanTable draft={draft} />

      {phase.at === 'done' ? (
        <Outcome result={phase.result} onClose={onClose} />
      ) : (
        <div className="sheet__actions">
          <button type="button" className="btn btn--ghost" onClick={onClose}>
            {t('action.cancel')}
          </button>
          <button type="button" className="btn" disabled={phase.at === 'signing'} onClick={go}>
            {phase.at === 'signing' ? t('rwa.signing') : t('rwa.sign')}
          </button>
        </div>
      )}
    </Dialog>
  );
}

/* --------------------------------------------------------------- redeeming */

const CONFIRM_WORD = 'REDEEM';

export function RedeemDialog({ account, assetId, balance, shareDenom, accrued, payoutDenom, onClose, onDone }: {
  account: Extract<Account, { mode: 'connected' }>;
  assetId: string;
  balance: string;
  shareDenom: string;
  /** What the holder is owed in total, which the payout is a fraction of. */
  accrued: string;
  payoutDenom: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [phase, setPhase] = useState<Phase>({ at: 'form' });
  const [amount, setAmount] = useState(balance);
  const [typed, setTyped] = useState('');

  // Shares are counted whole — resolveDenom gives them exponent 0 — so the
  // input is a plain integer and needs no decimal conversion. Anything else is
  // refused rather than coerced.
  const clean = /^\d+$/.test(amount.trim()) ? amount.trim() : '';
  const payout = clean ? redeemPayout(accrued, clean, balance) : null;
  const wholeHolding = clean !== '' && clean === balance;

  const ready = clean !== '' && payout !== null && typed.trim().toUpperCase() === CONFIRM_WORD;

  const draft: Draft = {
    kind: 'redeem',
    holder: account.address,
    assetId,
    amount: clean || '0',
    shareDenom,
    payout: payout ?? '0',
    payoutDenom,
  };

  const go = async () => {
    setPhase({ at: 'signing' });
    const result = await sendRedeem(account.signer, account.address, assetId, clean);
    setPhase({ at: 'done', result });
    if (result.ok) onDone();
  };

  return (
    <Dialog title={t('rwa.redeemTitle')} lede={t('rwa.redeemLede')} tone="bad" onClose={onClose}>
      {/*
        Destroyed first, received second. The order is the argument: a screen
        that leads with the money and mentions the burn underneath is selling
        the exit rather than describing it.
      */}
      <div className="sheet__headline sheet__headline--pair">
        <div>
          <span className="y-label">{t('rwa.youDestroy')}</span>
          <Amount amount={clean || '0'} denom={shareDenom} className="sheet__figure sheet__figure--bad" />
        </div>
        <div>
          <span className="y-label">{t('rwa.youReceive')}</span>
          <Amount amount={payout} denom={payoutDenom} className="sheet__figure" />
        </div>
      </div>

      <Note tone="bad" title={t('rwa.irreversible')}>{t('rwa.redeemIrreversible')}</Note>
      {wholeHolding && <Note tone="warn">{t('rwa.redeemAll')}</Note>}

      <label className="field2">
        <span className="y-label">{t('rwa.sharesToBurn')}</span>
        <input className="y-num" inputMode="numeric" value={amount}
               onChange={(e) => setAmount(e.target.value)} />
        <span className="field2__hint">
          {t('rwa.youHold')} <Amount amount={balance} denom={shareDenom} />
          {' · '}
          <button type="button" className="linkish" onClick={() => setAmount(balance)}>
            {t('rwa.useAll')}
          </button>
        </span>
      </label>

      {clean !== '' && payout === null && (
        <Note tone="bad">{t('rwa.redeemTooMuch')}</Note>
      )}

      <PlanTable draft={draft} />

      {phase.at === 'done' ? (
        <Outcome result={phase.result} onClose={onClose} />
      ) : (
        <>
          <label className="field2">
            <span className="y-label">{t('rwa.typeToConfirm', { word: CONFIRM_WORD })}</span>
            <input className="y-mono" value={typed} autoComplete="off" spellCheck={false}
                   onChange={(e) => setTyped(e.target.value)} />
          </label>

          <div className="sheet__actions">
            <button type="button" className="btn btn--ghost" onClick={onClose}>
              {t('action.cancel')}
            </button>
            <button type="button" className="btn btn--bad"
                    disabled={!ready || phase.at === 'signing'} onClick={go}>
              {phase.at === 'signing' ? t('rwa.signing') : t('rwa.redeemGo')}
            </button>
          </div>
        </>
      )}
    </Dialog>
  );
}

/* --------------------------------------------------------------- disputing */

export function DisputeDialog({ account, assetId, bond, bondDenom, price, onClose, onDone }: {
  account: Extract<Account, { mode: 'connected' }>;
  assetId: string;
  bond: string;
  bondDenom: string;
  price: { denom: string; amount: string };
  onClose: () => void;
  onDone: () => void;
}) {
  const [phase, setPhase] = useState<Phase>({ at: 'form' });
  const [reason, setReason] = useState('');

  const ready = reason.trim().length >= 12;

  const draft: Draft = {
    kind: 'dispute', challenger: account.address, assetId, reason: reason.trim(), bond, bondDenom,
  };

  const go = async () => {
    setPhase({ at: 'signing' });
    const result = await disputeSale(account.signer, account.address, assetId, reason.trim());
    setPhase({ at: 'done', result });
    if (result.ok) onDone();
  };

  return (
    <Dialog title={t('rwa.disputeTitle')} lede={t('rwa.disputeLede')} tone="warn" onClose={onClose}>
      {/*
        The bond is the whole reason this dialog is not a one-click button. It
        leaves the challenger's account in the same block as the signature and
        appears nowhere in the message, so it is stated as a figure, first.
      */}
      <div className="sheet__headline">
        <span className="y-label">{t('rwa.youStake')}</span>
        <Amount amount={bond} denom={bondDenom} className="sheet__figure sheet__figure--warn" />
      </div>

      <Note tone="warn">{t('rwa.bondFate')}</Note>
      <p className="muted">
        {t('rwa.disputeAgainst')}{' '}
        <strong>{formatAmount(price.amount, price.denom)}</strong>
        {' '}({resolveDenom(price.denom).name})
      </p>

      <label className="field2">
        <span className="y-label">{t('rwa.disputeReason')}</span>
        <textarea rows={4} value={reason} onChange={(e) => setReason(e.target.value)} />
        <span className="field2__hint">{t('rwa.disputeReasonHint')}</span>
      </label>

      <PlanTable draft={draft} />

      {phase.at === 'done' ? (
        <Outcome result={phase.result} onClose={onClose} />
      ) : (
        <div className="sheet__actions">
          <button type="button" className="btn btn--ghost" onClick={onClose}>
            {t('action.cancel')}
          </button>
          <button type="button" className="btn btn--warn"
                  disabled={!ready || phase.at === 'signing'} onClick={go}>
            {phase.at === 'signing' ? t('rwa.signing') : t('rwa.disputeGo')}
          </button>
        </div>
      )}
    </Dialog>
  );
}
