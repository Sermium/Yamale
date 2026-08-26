/**
 * Send money.
 *
 * The most boring screen in the product, deliberately: recipient, amount,
 * reference, status. It was inside App.tsx among twenty-two other components;
 * it is here because this is the screen the whole application exists for and it
 * should be readable on its own.
 *
 * What is new is the purpose code and the end-to-end id. Both are ISO 20022
 * fields, both are what a reconciliation actually joins on, and both now travel
 * with the payment instead of being dropped. They travel in the transaction
 * memo, because `MsgSendPayment` is unreachable on this chain — and rather than
 * hide that, the screen can show the instruction it would have sent, field for
 * field, with the chain's own answer for why it did not.
 */
import { useEffect, useState } from 'react';
import { t } from '@yamale/chain';

import type { Signer } from './account.ts';
import * as book from './book.ts';
import * as chain from './chain.ts';
import { CURRENCIES, currencyOf, display, parseAmount, rawAmount, defaultDenom } from './money.ts';
import { balances } from './topup.ts';
import { describe, readSigned, type PaymentRequest } from './request.ts';
import { Scanner } from './scan.tsx';
import { InstructionPanel } from './Instruction.tsx';
import { useMyUserId } from './identity.ts';
import { publish } from './draft.ts';
import { approvedParticipants } from './standing.ts';
import {
  MEMO_LIMIT, PURPOSES, encodeMemo, freshEndToEndId, instructionFor, purposeKey,
  type Instruction, type Standing,
} from './iso.ts';

/** The reference field's own cap, which is what the form enforces. */
const REFERENCE_LIMIT = 64;

export function Pay({ signer }: { signer: Signer }) {
  const [to, setTo] = useState('');
  const [amount, setAmount] = useState('');
  const [reference, setReference] = useState('');
  const [purpose, setPurpose] = useState('');
  const [denom, setDenom] = useState(defaultDenom());
  const [code, setCode] = useState('');
  const [checking, setChecking] = useState(false);
  const [verdict, setVerdict] = useState<'none' | 'trusted' | 'untrusted' | 'unreadable'>('none');
  const [incoming, setIncoming] = useState<PaymentRequest | null>(null);
  const [scanning, setScanning] = useState(false);
  const [confirmingNew, setConfirmingNew] = useState(false);

  /**
   * One id per composition, not one per keystroke.
   *
   * ISO 20022 says an EndToEndId is unique per instructing party, and the payer
   * is entitled to read it off the screen before signing and find that same
   * string on the receipt afterwards. Regenerating it as the form changed would
   * make it a number that means nothing until the moment it is sent.
   */
  const [e2e, setE2e] = useState(() => freshEndToEndId());

  // What the account actually holds, so the action can be disabled with the
  // precondition stated rather than the payment refused after signing.
  const [held, setHeld] = useState<Map<string, string> | null>(null);
  // The identifier is registered a moment after sign-in, not before it, and on
  // this chain it may never arrive at all. `useMyUserId` holds the three-way
  // answer; see identity.ts for why the third one is not a spinner.
  const me = useMyUserId(signer);
  useEffect(() => {
    if (!me.address) return;
    let live = true;
    void balances(me.address).then((map) => { if (live) setHeld(map); });
    return () => { live = false; };
  }, [me.address]);

  /**
   * The chain's own answer on whether the ISO path is open, read once.
   *
   * Not polled: an approval is a governance action measured in days, and a
   * screen that re-asks every few seconds is spending somebody's data plan to
   * watch a number that will not move. Refreshed when the screen is opened,
   * which is when the question is being asked.
   */
  const [standing, setStanding] = useState<Standing>({ known: false, whyKey: 'iso.standingChecking' });
  useEffect(() => {
    let live = true;
    void approvedParticipants().then((s) => { if (live) setStanding(s); });
    return () => { live = false; };
  }, []);

  // Who the typed id resolves to on the chain. Resolved before the money
  // moves, because "no account answers to that" is a thing to say now and not
  // after a failed transaction.
  const [payee, setPayee] = useState<{ id: string; address: string | null; looking: boolean }>({
    id: '', address: null, looking: false,
  });
  useEffect(() => {
    const id = to.trim();
    if (id === '') { setPayee({ id: '', address: null, looking: false }); return; }
    let live = true;
    setPayee({ id, address: null, looking: true });
    void book.resolve(id).then((address) => {
      if (live) setPayee({ id, address, looking: false });
    });
    return () => { live = false; };
  }, [to]);

  const [stage, setStage] = useState<'compose' | 'sending' | 'done'>('compose');
  const [outcome, setOutcome] = useState<chain.PaymentOutcome | null>(null);

  const currency = currencyOf(denom);
  const parsed = amount.trim() === '' ? null : parseAmount(amount, denom);
  const unreadableAmount = amount.trim() !== '' && parsed === null;
  const balance = held?.get(denom) ?? '0';
  const enough = parsed !== null && BigInt(parsed.base) > 0n && BigInt(parsed.base) <= BigInt(balance);

  const msg: Instruction = instructionFor({
    debtorAddress: me.address,
    debtorUserId: me.userId,
    creditorAddress: payee.address ?? '',
    creditorUserId: payee.id,
    denom,
    amount: parsed?.base ?? '',
    purposeCode: purpose,
    remittanceInformation: reference.trim(),
    endToEndId: e2e,
  });

  const memo = encodeMemo({ e2e, purpose, remittance: reference.trim() });
  // Prevention, not error handling: a memo over the node's limit is refused
  // after the payer has signed, which is the worst possible moment to find out.
  const memoTooLong = memo.length > MEMO_LIMIT;

  const ready = payee.address !== null && enough && !memoTooLong && stage === 'compose';

  /**
   * Keep the desk beside the phone showing this payment.
   *
   * Published from an effect rather than from each setter so there is one place
   * it happens, and cleared on unmount so leaving the screen does not leave a
   * half-typed payment on a boardroom projector.
   */
  useEffect(() => {
    if (stage !== 'compose') return;
    publish({
      debtorAddress: me.address,
      debtorUserId: me.userId,
      creditorAddress: payee.address ?? '',
      creditorUserId: payee.id,
      denom,
      amount: parsed?.base ?? '',
      purposeCode: purpose,
      remittanceInformation: reference.trim(),
      endToEndId: e2e,
    });
  }, [stage, me.address, me.userId, payee.address, payee.id, denom, parsed?.base, purpose, reference, e2e]);

  useEffect(() => () => publish(null), []);

  const names: Record<string, string> = {};
  if (me.address && me.userId) names[me.address] = me.userId;
  if (payee.address && payee.id) names[payee.address] = payee.id;

  /**
   * Sign it, wait for the block, and report what the chain did.
   *
   * The address book is written *after* success. Recording a payee for a
   * payment that failed is how a "recent" list fills up with people who were
   * never paid.
   */
  async function send() {
    if (!payee.address || !parsed) return;
    setStage('sending');
    const result = await chain.pay(signer, payee.address, parsed.base, denom, memo);
    setOutcome(result);
    if (result.ok) book.remember(payee.id, incoming?.payeeName);
    setStage('done');
  }

  function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!ready) return;
    if (!book.isKnown(to)) { setConfirmingNew(true); return; }
    void send();
  }

  /**
   * Read a code somebody sent, and say plainly whether it can be trusted.
   *
   * A code travels over channels nobody controls — a forwarded message, a
   * photographed poster, a number read down a phone. None of those can be made
   * private, and none of them need to be: what matters is that a tampered or
   * impersonated code is detectable here, before any money moves.
   */
  async function readCode() {
    setChecking(true);
    setVerdict('none');
    try {
      const result = await readSigned(code, book.resolve);
      if (!result) { setVerdict('unreadable'); return; }
      setIncoming(result.request);
      setVerdict(result.trusted ? 'trusted' : 'untrusted');
      if (result.trusted) {
        setTo(result.request.to);
        if (result.request.amount) setAmount(result.request.amount);
        if (result.request.reference) setReference(result.request.reference);
        const c = CURRENCIES.find((x) => x.code === result.request.currency);
        if (c) setDenom(c.denom);
      }
    } finally {
      setChecking(false);
    }
  }

  if (stage === 'sending') {
    return (
      <div className="done">
        {/* Pending, with the figure already visible. A spinner with nothing
            beside it leaves somebody unsure whether they pressed the button. */}
        <div className="done__pending" aria-hidden="true"><i /><i /><i /></div>
        <h2>{display(parsed?.base ?? '0', denom)}</h2>
        <p className="muted" role="status">{t('app.paySending')}</p>
        <p className="small-note muted">{t('iso.waitingForBlock')}</p>
      </div>
    );
  }

  if (stage === 'done' && outcome) {
    return (
      <div className="done">
        {/* The tick is set from execution, never from broadcast. A node
            accepting a transaction into its mempool has moved nothing. */}
        <div className={outcome.ok ? 'done__tick' : 'done__cross'} aria-hidden="true">
          {outcome.ok ? '✓' : '!'}
        </div>
        <h2>{outcome.ok ? t('app.paymentSent') : t('app.payFailed')}</h2>

        {outcome.ok ? (
          <>
            <p className="done__amount">{display(parsed?.base ?? '0', denom)}</p>
            <p className="muted">{book.displayName(payee.id)}</p>
            <p className="small-note muted">{t('app.paySettled', { height: outcome.height })}</p>

            {/* The reference fields, as a receipt rather than as a footnote.
                This is what somebody matches against an invoice, so it is the
                part of this screen worth reading. */}
            <dl className="receipt">
              <div>
                <dt>{t('iso.e2eLabel')}</dt>
                <dd className="y-mono">{e2e}</dd>
              </div>
              <div>
                <dt>{t('iso.purpose')}</dt>
                <dd>{purpose ? `${purpose} · ${t(purposeKey(purpose) ?? '')}` : t('iso.purposeNone')}</dd>
              </div>
              <div>
                <dt>{t('app.reference')}</dt>
                <dd>{reference.trim() || <span className="iso__none">{t('iso.notSet')}</span>}</dd>
              </div>
            </dl>

            {/* What this screen actually did, and what it did not. */}
            <p className="small-note muted">{t('app.payViaTransfer')}</p>

            <details className="raw iso__disclosure">
              <summary className="muted">{t('iso.showInstruction')}</summary>
              <InstructionPanel msg={msg} memo={memo} standing={standing} names={names} />
            </details>
          </>
        ) : (
          <>
            {/* What happened, why, and the one next action — with the raw text
                behind a disclosure. */}
            <p><strong>{outcome.error?.message}</strong></p>
            {outcome.error?.reason && <p className="muted">{outcome.error.reason}</p>}
            {outcome.error?.nextStep && <p>{outcome.error.nextStep}</p>}
            {outcome.error?.raw && outcome.error.raw !== outcome.error.message && (
              <details className="raw">
                <summary className="muted">{t('app.reveal')}</summary>
                <pre>{outcome.error.raw}</pre>
              </details>
            )}
          </>
        )}

        <button
          className="primary"
          onClick={() => {
            setStage('compose');
            setOutcome(null);
            if (outcome.ok) {
              setTo(''); setAmount(''); setReference(''); setPurpose('');
              // A new payment is a new end-to-end id. Reusing one would make two
              // payments indistinguishable in exactly the record meant to tell
              // them apart.
              setE2e(freshEndToEndId());
            }
          }}
        >
          {outcome.ok ? t('app.payAgain') : t('app.done')}
        </button>
      </div>
    );
  }

  return (
    <>
      <h2 className="screen__title">{t('app.sendMoney')}</h2>

      <div className="form">
        <label>
          <span>{t('app.pasteCode')}</span>
          <input value={code} onChange={(e) => setCode(e.target.value)} placeholder={t('app.pasteHint')} />
        </label>
        <button type="button" className="ghost" onClick={readCode} disabled={!code || checking}>
          {checking ? t('app.checking') : t('app.readCode')}
        </button>
        {!scanning && (
          <button type="button" className="ghost" onClick={() => setScanning(true)}>
            {t('app.scanCode')}
          </button>
        )}
      </div>

      {scanning && (
        <Scanner
          onRead={(raw) => { setScanning(false); setCode(raw); }}
          onClose={() => setScanning(false)}
        />
      )}

      {/* The verdict is stated in the payer's own terms. "Signature invalid"
          tells somebody nothing they can act on; "this code was not made by the
          person it claims to pay" tells them to stop. */}
      {verdict === 'trusted' && incoming && (
        <p className="notice notice--good">
          {t('app.codeTrusted', { name: incoming.payeeName ?? incoming.to })} · {describe(incoming)}
        </p>
      )}
      {verdict === 'untrusted' && (
        <p className="notice notice--bad">{t('app.codeUntrusted')}</p>
      )}
      {verdict === 'unreadable' && (
        <p className="notice notice--bad">{t('app.codeUnreadable')}</p>
      )}

      {/* Paying somebody for the first time gets one deliberate pause. Most
          wrong payments are not fraud, they are a mistyped id or the wrong row
          tapped in a hurry, and both are caught by being asked once. Paying a
          known contact is not interrupted — a warning shown every time is a
          warning nobody reads. */}
      {confirmingNew && (
        <div className="confirm">
          <p><strong>{t('app.firstTimePaying')}</strong></p>
          <p className="muted">{t('app.firstTimeDetail', { id: to })}</p>
          <div className="confirm__row">
            <button type="button" className="ghost" onClick={() => setConfirmingNew(false)}>
              {t('app.cancel')}
            </button>
            <button type="button" className="primary" onClick={() => { setConfirmingNew(false); void send(); }}>
              {t('app.payAnyway')}
            </button>
          </div>
        </div>
      )}

      <form className="form" onSubmit={submit}>
        <label>
          {/* Never "address". A person pays a person, identified by the id the
              chain assigned them or the name they saved. */}
          <span>{t('app.payTo')}</span>
          <input value={to} onChange={(e) => setTo(e.target.value)} placeholder={t('app.payToHint')} required />
        </label>
        {/* Resolved before anything moves, and named. "No account answers to
            that" is worth saying while the money is still here. */}
        {payee.looking && <p className="small-note muted">{t('app.searching')}</p>}
        {!payee.looking && payee.id !== '' && payee.address === null && (
          <p className="notice notice--bad">{t('app.unknownId')}</p>
        )}
        {payee.address && (
          <p className="notice notice--good">{book.displayName(payee.id)}</p>
        )}

        <label>
          <span>{t('app.currency')}</span>
          <select value={denom} onChange={(e) => setDenom(e.target.value)}>
            {CURRENCIES.map((c) => <option key={c.denom} value={c.denom}>{c.name}</option>)}
          </select>
        </label>
        <label>
          <span>{t('app.amount')}</span>
          <input className="y-num" inputMode="decimal" value={amount}
                 /* The decimal comma survives. A field allowing only [0-9.]
                    turns the 1250,50 a French or Portuguese reader types into
                    125050 — a hundred times the payment they meant. */
                 onChange={(e) => setAmount(e.target.value.replace(/[^0-9.,\s  ]/g, ''))}
                 required />
        </label>

        {/* Prevention rather than error handling: the balance, one tap to use
            all of it, and the action disabled with the reason stated. */}
        {held !== null && BigInt(balance) > 0n && (
          <button type="button" className="linkish available"
                  onClick={() => setAmount(rawAmount(balance, denom))}>
            {t('app.available', { amount: display(balance, denom) })}
          </button>
        )}
        {held !== null && BigInt(balance) === 0n && (
          <p className="small-note muted">{t('app.emptyBalance')}</p>
        )}
        {unreadableAmount && <p className="notice notice--bad">{t('app.payNotAnAmount')}</p>}
        {parsed?.truncated && currency && (
          <p className="notice">
            {t('app.payTruncated', {
              code: currency.code,
              places: currency.exponent,
              amount: display(parsed.base, denom),
            })}
          </p>
        )}
        {parsed !== null && BigInt(parsed.base) > BigInt(balance) && (
          <p className="notice notice--bad">{t('app.payTooMuch')}</p>
        )}

        {/* The reference and the purpose, as first-class content rather than an
            afterthought. This is a payments surface: reconciliation is what the
            reference exists for, and a purpose code is what makes a payment
            classifiable by anybody other than the two parties. Both sit in the
            form beside the amount, not behind a disclosure. */}
        <label>
          <span>{t('app.reference')}</span>
          <input value={reference} onChange={(e) => setReference(e.target.value)}
                 placeholder={t('app.referenceHint')} maxLength={REFERENCE_LIMIT} />
        </label>
        <label>
          <span>{t('iso.purpose')}</span>
          <select value={purpose} onChange={(e) => setPurpose(e.target.value)}>
            <option value="">{t('iso.purposeNone')}</option>
            {PURPOSES.map((p) => (
              <option key={p.code} value={p.code}>{t(p.key)} · {p.code}</option>
            ))}
          </select>
        </label>
        <p className="small-note muted">{t('iso.purposeHint')}</p>

        {memoTooLong && (
          <p className="notice notice--bad">
            {t('iso.memoTooLong', { limit: MEMO_LIMIT, over: memo.length - MEMO_LIMIT })}
          </p>
        )}

        <button className="primary" disabled={!ready}>{t('app.sendNow')}</button>
      </form>

      {/* Mechanism, one click deeper. The screen above stays boring; anybody
          who wants to know what is actually being signed can open this without
          leaving the payment. */}
      <details className="raw iso__disclosure">
        <summary className="muted">{t('iso.showInstruction')}</summary>
        <InstructionPanel msg={msg} memo={memo} standing={standing} names={names} />
      </details>
    </>
  );
}
