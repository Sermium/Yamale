/**
 * Paying somebody.
 *
 * The identity layer arrives here: you type a user ID, a name from your address
 * book, or an address, and the wallet tells you *who* that is before you part
 * with anything. Everything else on these screens exists to make the moment
 * before the signature honest.
 *
 * Three deliberate frictions, all of them kept from the desktop version:
 *
 * **A review step you cannot skip.** A single screen with the button under the
 * amount field is how people pay the wrong account.
 *
 * **The full address on the review, even when a friendly name matched.** A name
 * is what you recognise; an address is what actually receives, and only one of
 * those is checkable.
 *
 * **The password is asked for at signing, not on arrival.** Unlocking early
 * leaves a key in memory for a screen somebody may walk away from.
 *
 * And one thing the desktop version did not do, which is the house rule
 * everywhere in this repository (see clients/rwa/src/Act.tsx): the review
 * states what leaves the account, what arrives, and what cannot be undone,
 * above the button rather than beneath it.
 */
import { useEffect, useState } from 'react';
import {
  ChainSigner,
  formatUserId,
  listContacts,
  resolveDenom,
  saveContact,
  send,
  toBaseUnitsOf,
  translateError,
  t,
  type Coin,
  type TranslatedError,
} from '@yamale/chain';

import { client } from '../chain.ts';
import { Named } from '../Named.tsx';
import { Identifier } from '../Identifier.tsx';
import { getUnlocked, openVault, setUnlocked, touch, vaultSummary } from '../vault.ts';
import { Amount, Back, Field, Note, ScreenTitle } from '../ui.tsx';

const CHAIN_ID = import.meta.env.VITE_CHAIN_ID ?? 'yamale-devnet-2';
/**
 * The trailing slash is load-bearing.
 *
 * nginx matches `location /api/rpc/`, so a POST to `/api/rpc` misses it, falls
 * through to the single-page-app fallback and comes back as index.html — which
 * CosmJS then tries to parse as JSON-RPC and reports as `Unexpected token '<'`.
 * The error names JSON, so it reads like a chain fault rather than a URL that
 * is one character short.
 */
const RPC = `${window.location.origin}/api/rpc/`;

type Resolved = {
  address: string | null;
  how: 'userId' | 'pseudonym' | 'address' | 'empty' | 'unknown' | 'looking';
  label?: string;
};

type Stage = 'compose' | 'review' | 'sending' | 'sent';

export function SendScreen({ onGoHome }: { onGoHome: () => void }) {
  const account = vaultSummary();
  const [typed, setTyped] = useState('');
  const [resolved, setResolved] = useState<Resolved>({ address: null, how: 'empty' });
  const [denom, setDenom] = useState('uyml');
  const [amount, setAmount] = useState('');
  // null means "not read", and it is not the same as an account holding
  // nothing. With it null the screen refuses to claim what is affordable
  // rather than comparing against a zero it invented.
  const [balances, setBalances] = useState<Coin[] | null>(null);
  const [sponsored, setSponsored] = useState<boolean | null>(null);
  const [stage, setStage] = useState<Stage>('compose');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<TranslatedError | null>(null);
  const [txHash, setTxHash] = useState('');
  const [height, setHeight] = useState(0);
  const [remember, setRemember] = useState('');

  useEffect(() => {
    if (!account) return;
    let live = true;
    void client.balances(account.address)
      .then((b) => live && setBalances(b))
      .catch(() => live && setBalances(null));
    void client.feeAllowances(account.address)
      .then((a) => live && setSponsored(a.length > 0))
      .catch(() => live && setSponsored(null));
    return () => { live = false; };
  }, [account?.address]);

  // Resolution runs as you type, but only the chain lookup is async — an
  // address or an address-book name is answered locally and instantly.
  useEffect(() => {
    const value = typed.trim();
    if (!value) { setResolved({ address: null, how: 'empty' }); return; }
    if (/^yml1[0-9a-z]{38,}$/.test(value)) { setResolved({ address: value, how: 'address' }); return; }
    const contact = listContacts().find((c) => c.pseudonym.toLowerCase() === value.toLowerCase());
    if (contact) {
      setResolved({ address: contact.address, how: 'pseudonym', label: contact.pseudonym });
      return;
    }
    let cancelled = false;
    setResolved({ address: null, how: 'looking' });
    void client.addressOfUserId(value).then((addr) => {
      if (cancelled) return;
      setResolved(addr
        ? { address: addr, how: 'userId', label: formatUserId(value) }
        : { address: null, how: 'unknown' });
    });
    return () => { cancelled = true; };
  }, [typed]);

  if (!account) {
    return (
      <>
        <ScreenTitle lede={t('send.needKey')}>{t('send.noAccount')}</ScreenTitle>
        <button type="button" className="wal-btn" onClick={onGoHome}>{t('wal.attachTitle')}</button>
      </>
    );
  }

  const held = (balances ?? []).filter((b) => !b.denom.startsWith('amm/pool/'));
  const chosen = held.find((b) => b.denom === denom);
  const info = resolveDenom(denom);

  // Parsed by string, with the exponent from the chain's own denom metadata.
  // `Math.round(Number(amount) * 10 ** exponent)` is exact only below 2^53 and
  // turns 0.07 into 70000.00000000001 on the way — so whether the amount that
  // moves is the amount that was typed depended on which decimal the sender
  // happened to choose.
  const parsed = amount.trim() === '' ? null : toBaseUnitsOf(amount, denom);
  const unreadable = amount.trim() !== '' && parsed === null;
  const base = parsed?.base ?? '';
  const positive = base !== '' && BigInt(base) > 0n;
  // Affordability is a claim about a balance. With the balance unread the
  // screen does not make it — it lets the review go ahead and says, in words,
  // that it could not check.
  const enough = chosen ? BigInt(base || '0') <= BigInt(chosen.amount) : null;
  const ready = Boolean(resolved.address) && positive && enough !== false;

  async function confirmAndSend() {
    setError(null);
    setStage('sending');
    try {
      let wallet = getUnlocked();
      if (!wallet) {
        wallet = await openVault(password);
        setUnlocked(wallet);
      }
      touch();

      const signer = new ChainSigner(wallet, { rpcUrl: RPC, chainId: CHAIN_ID });
      const result = await signer.submit([
        send(account!.address, resolved.address!, [{ denom, amount: base }]),
      ]);
      // `submit` waits for the block and reports execution, not acceptance. A
      // transaction can broadcast cleanly and then fail inside the block — a
      // frozen account, a fee allowance that has run out — so the success
      // screen is reached only when the chain actually did it.
      if (!result.succeeded) {
        setError(result.error ?? translateError(`the chain refused it with code ${result.code}`));
        setStage('review');
        return;
      }
      setTxHash(result.hash);
      setHeight(result.height);
      if (remember.trim()) {
        saveContact({
          address: resolved.address!,
          pseudonym: remember.trim(),
          userId: resolved.how === 'userId' ? resolved.label : undefined,
        });
      }
      setStage('sent');
    } catch (err) {
      // Anything reaching here is a fault rather than a chain refusal: a wrong
      // password, an unreachable node. translateError keeps the raw text either
      // way, because "it failed" is not a bug report.
      setError(translateError(err instanceof Error ? err.message : String(err)));
      setStage('review');
    }
  }

  /* ------------------------------------------------------------- the done */

  if (stage === 'sent') {
    return (
      <div className="wal-done">
        <span className="wal-done__tick" aria-hidden="true">✓</span>
        {/* Past tense, because it has happened. submit() waits for the block,
            so by the time this draws the recipient's balance has changed. */}
        <h1 className="wal-done__title">{t('send.sent')}</h1>
        <Amount amount={base} denom={denom} size="hero" />
        <p className="wal-muted">
          {t('wal.arrivedWith')}{' '}
          {resolved.label ?? <Named address={resolved.address!} />}
        </p>
        <p className="wal-small wal-muted">
          {t('wal.settledIn', { height: height.toLocaleString() })}
        </p>
        <Identifier value={txHash} />
        <button type="button" className="wal-btn" onClick={() => {
          setStage('compose'); setTyped(''); setAmount(''); setRemember('');
        }}>
          {t('wal.sendAnother')}
        </button>
        <button type="button" className="wal-btn wal-btn--ghost" onClick={onGoHome}>
          {t('send.backToAccount')}
        </button>
      </div>
    );
  }

  /* ----------------------------------------------------------- the review */

  if (stage === 'review' || stage === 'sending') {
    return (
      <>
        <Back onClick={() => setStage('compose')}>{t('wal.change')}</Back>
        <ScreenTitle>{t('send.confirm')}</ScreenTitle>

        {/*
          Leaves first, arrives second, irreversible third, and only then the
          button. This is the highest-stakes moment in the product and the order
          is the argument: a screen that leads with the recipient and mentions
          the money underneath is confirming an address rather than a payment.
        */}
        <div className="wal-ledger">
          <div className="wal-ledger__side">
            <span className="wal-fact__label">{t('wal.leavesAccount')}</span>
            <Amount amount={base} denom={denom} size="row" className="wal-ledger__fig wal-ledger__fig--out" />
          </div>
          <div className="wal-ledger__side">
            <span className="wal-fact__label">{t('wal.theyReceive')}</span>
            <Amount amount={base} denom={denom} size="row" className="wal-ledger__fig" />
          </div>
        </div>

        {/*
          The fee is stated as a fact and never as a figure, because this screen
          does not have the figure. Simulating the gas for a number that would
          then be printed beside the amount is how an interface starts showing a
          number it does not have.
        */}
        <Note tone="plain">
          {sponsored === true ? t('wal.feeSponsored')
            : sponsored === false ? t('wal.feeFromYou')
            : t('wal.feeUnknownWhoPays')}
        </Note>

        <h2 className="y-screen__sub">{t('term.recipient')}</h2>
        <div className="y-card">
          {resolved.label && <p className="wal-row__strong">{resolved.label}</p>}
          {/* The full address, even when a name matched. */}
          <code className="y-addr wal-addrFull">{resolved.address}</code>
        </div>

        <Note tone="bad" title={t('wal.irreversible')}>{t('wal.irreversibleBody')}</Note>

        {/* What the chain will actually receive, field by field. The house
            pattern from clients/rwa/src/Act.tsx: a person approving a payment
            is entitled to see the message, not a paraphrase of it. */}
        <details className="wal-plan">
          <summary>{t('wal.whatIsSigned')}</summary>
          <p className="wal-plan__type y-mono">/cosmos.bank.v1beta1.MsgSend</p>
          <dl className="wal-plan__rows">
            <div><dt className="y-mono">from_address</dt><dd className="y-addr">{account.address}</dd></div>
            <div><dt className="y-mono">to_address</dt><dd className="y-addr">{resolved.address}</dd></div>
            <div><dt className="y-mono">amount</dt><dd className="y-mono wal-nobreak">{base} {denom}</dd></div>
          </dl>
        </details>

        {resolved.how !== 'pseudonym' && (
          <Field label={t('send.saveAs')}>
            <input className="wal-input" value={remember} onChange={(e) => setRemember(e.target.value)}
                   placeholder="Acme Ltd" />
          </Field>
        )}

        {!getUnlocked() && (
          <Field label={t('wal.passwordFor', { label: account.label })}>
            <input className="wal-input" type="password" value={password}
                   onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" autoFocus />
          </Field>
        )}

        {/* What happened, why, and the one thing to do next — with the raw chain
            text behind a disclosure for whoever has to debug it. */}
        {error && (
          <Note tone="bad" title={error.message}>
            {error.reason}
            {error.nextStep && <p className="wal-row__strong">{error.nextStep}</p>}
            {error.raw && error.raw !== error.message && (
              <details className="wal-raw">
                <summary>{t('sign.whatTheChainSaid')}</summary>
                <pre className="wal-raw__pre y-scroll">{error.raw}</pre>
              </details>
            )}
          </Note>
        )}

        <button type="button" className="wal-btn wal-btn--commit" onClick={confirmAndSend}
                disabled={stage === 'sending' || (!getUnlocked() && !password)}>
          {stage === 'sending' ? t('wal.sending') : t('wal.sendIt')}
        </button>
      </>
    );
  }

  /* ---------------------------------------------------------- the compose */

  return (
    <>
      <ScreenTitle lede={t('wal.sendLede')}>{t('send.title')}</ScreenTitle>

      <Field label={t('send.whoLabel')}>
        <input
          className="wal-input"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          placeholder="NG-CAQ3-C04Z-M · Acme Ltd · yml1…"
          spellCheck={false}
          autoComplete="off"
        />
      </Field>

      {resolved.how === 'looking' && <p className="wal-small wal-muted">{t('send.asking')}</p>}
      {resolved.how === 'unknown' && <Note tone="bad">{t('wal.noSuchAccount')}</Note>}
      {resolved.address && (
        <Note tone="ok">
          {resolved.how === 'userId' ? <>{t('wal.userIdIs')} <strong>{resolved.label}</strong></>
            : resolved.how === 'pseudonym' ? <><strong>{resolved.label}</strong> {t('wal.fromYourBook')}</>
            : t('send.thatAddress')}
          <br />
          <code className="y-addr wal-addrFull">{resolved.address}</code>
        </Note>
      )}

      <h2 className="y-screen__sub">{t('send.howMuch')}</h2>

      {balances === null ? (
        <Note tone="warn">{t('wal.balancesNotReadSend')}</Note>
      ) : held.length === 0 ? (
        <Note tone="warn" title={t('acct.nothingHere')}>{t('acct.nothingHereBody')}</Note>
      ) : (
        <div className="wal-chips y-scroll">
          {held.map((b) => (
            <button key={b.denom} type="button"
                    className={denom === b.denom ? 'wal-chip wal-chip--on' : 'wal-chip'}
                    onClick={() => setDenom(b.denom)}>
              {resolveDenom(b.denom).symbol}
            </button>
          ))}
        </div>
      )}

      <Field label={t('wal.amountIn', { symbol: info.symbol })}>
        <input
          className="wal-input wal-input--num y-mono"
          value={amount}
          /* A decimal comma is kept, not stripped. A French or Portuguese
             reader types 1250,50, and a filter allowing only [0-9.] silently
             turned that into 125050 — a hundred times the payment they meant to
             make. Parsing decides what it means; the field does not. */
          onChange={(e) => setAmount(e.target.value.replace(/[^0-9.,\s  ]/g, ''))}
          placeholder="0.00"
          inputMode="decimal"
        />
      </Field>

      {chosen && (
        <p className="wal-small wal-muted wal-youHold">
          {t('wal.youHold')} <Amount amount={chosen.amount} denom={denom} />
          {' · '}
          {/* Prevention rather than error handling: the whole balance is one
              tap, so nobody has to transcribe it and get it wrong. */}
          <button type="button" className="wal-link"
                  onClick={() => setAmount(
                    // Ungrouped on purpose: the field is parsed, and a grouped
                    // figure pasted back into it is a second parse to get wrong.
                    (formatAmountPlain(chosen.amount, denom)))}>
            {t('wal.sendAllOfIt')}
          </button>
        </p>
      )}

      {unreadable && <Note tone="bad">{t('wal.notAnAmount')}</Note>}
      {parsed?.truncated && (
        <Note tone="warn">
          {t('wal.truncated', { symbol: info.symbol, places: String(info.exponent) })}{' '}
          <Amount amount={parsed.base} denom={denom} />
        </Note>
      )}
      {enough === false && <Note tone="bad">{t('send.tooMuch')}</Note>}

      <button type="button" className="wal-btn" disabled={!ready} onClick={() => setStage('review')}>
        {t('wal.review')}
      </button>
    </>
  );
}

/** The balance as the amount field would accept it: no symbol, no grouping. */
function formatAmountPlain(base: string, denom: string): string {
  const { exponent } = resolveDenom(denom);
  if (exponent === 0) return base;
  const padded = base.padStart(exponent + 1, '0');
  const whole = padded.slice(0, padded.length - exponent);
  const fraction = padded.slice(padded.length - exponent).replace(/0+$/, '');
  return fraction ? `${whole}.${fraction}` : whole;
}
