import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  ChainSigner,
  formatAmount,
  formatUserId,
  listContacts,
  resolveDenom,
  saveContact,
  send,
  truncateAddress,
  t,
} from '@yamale/chain';

import { client } from '../chain.ts';
import { Named } from '../Named.tsx';
import { getUnlocked, openVault, setUnlocked, touch, vaultSummary } from '../vault.ts';

const CHAIN_ID = import.meta.env.VITE_CHAIN_ID ?? 'yamale-devnet-1';
/**
 * The trailing slash is load-bearing.
 *
 * nginx matches `location /api/rpc/`, so a POST to `/api/rpc` misses it, falls
 * through to the single-page-app fallback, and comes back as index.html —
 * which CosmJS then tries to parse as JSON-RPC and reports as
 * `Unexpected token '<'`. The error names JSON, so it reads like a chain fault
 * rather than a URL that is one character short.
 */
const RPC = `${window.location.origin}/api/rpc/`;

/**
 * One shape rather than a discriminated union: `label` is read on the success
 * screen after `address` has already been asserted, and a union forced a narrow
 * at every use for no safety — `how` is what actually distinguishes the states.
 */
type Resolved = {
  address: string | null;
  how: 'userId' | 'pseudonym' | 'address' | 'empty' | 'unknown' | 'looking';
  label?: string;
};

type Stage = 'compose' | 'confirm' | 'sending' | 'sent';

/**
 * Sending money to a person rather than to a string.
 *
 * The whole point of the identity layer arrives here: you type a user ID, a
 * name from your address book, or an address, and the app tells you *who* that
 * is before you part with anything. Everything else on this screen exists to
 * make the moment before signing honest.
 *
 * Two deliberate frictions:
 *
 * **A confirm step you cannot skip.** A single-screen send with the button
 * under the amount field is how people pay the wrong account. The confirmation
 * restates the recipient, and shows the full address even when a friendly name
 * was matched — a name is what you recognise, an address is what actually
 * receives.
 *
 * **The password is asked for at signing, not at page load.** Unlocking early
 * would leave a key in memory for a screen somebody might abandon.
 */
function SendDirect() {
  const account = vaultSummary();
  const [typed, setTyped] = useState('');
  const [resolved, setResolved] = useState<Resolved>({ address: null, how: 'empty' });
  const [denom, setDenom] = useState('uyml');
  const [amount, setAmount] = useState('');
  const [balances, setBalances] = useState<{ denom: string; amount: string }[]>([]);
  const [stage, setStage] = useState<Stage>('compose');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [txHash, setTxHash] = useState('');
  const [remember, setRemember] = useState('');

  useEffect(() => {
    if (!account) return;
    void client.balances(account.address).then(setBalances).catch(() => setBalances([]));
  }, [account?.address]);

  // Resolution runs as you type, but only the chain lookup is async — an
  // address or an address-book name is answered locally and instantly.
  useEffect(() => {
    const value = typed.trim();
    if (!value) {
      setResolved({ address: null, how: 'empty' });
      return;
    }
    if (/^yml1[0-9a-z]{38,}$/.test(value)) {
      setResolved({ address: value, how: 'address' });
      return;
    }
    const contact = listContacts().find(
      (c) => c.pseudonym.toLowerCase() === value.toLowerCase(),
    );
    if (contact) {
      setResolved({ address: contact.address, how: 'pseudonym', label: contact.pseudonym });
      return;
    }

    let cancelled = false;
    setResolved({ address: null, how: 'looking' });
    void client.addressOfUserId(value).then((addr) => {
      if (cancelled) return;
      setResolved(
        addr
          ? { address: addr, how: 'userId', label: formatUserId(value) }
          : { address: null, how: 'unknown' },
      );
    });
    return () => {
      cancelled = true;
    };
  }, [typed]);

  if (!account) {
    return (
      <>
        <h1>{t('send.title')}</h1>
        <section className="card">
          <h2>{t('send.noAccount')}</h2>
          <p className="muted">
            {t('send.needKey')} <Link to="/create">{t('send.createAccount')}</Link> or{' '}
            <Link to="/import">load one</Link>.
          </p>
        </section>
      </>
    );
  }

  const held = balances.filter((b) => !b.denom.startsWith('amm/pool/'));
  const chosen = held.find((b) => b.denom === denom);
  const info = resolveDenom(denom);
  const base = amount ? String(Math.round(Number(amount) * 10 ** (info.exponent ?? 6))) : '';
  const enough = chosen && base && BigInt(base) <= BigInt(chosen.amount);
  const ready = resolved.address && base && BigInt(base) > 0n && enough;

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
      if (!result.succeeded) throw new Error(result.error?.message ?? `Rejected with code ${result.code}`);

      setTxHash(result.hash);
      if (remember.trim()) {
        saveContact({
          address: resolved.address!,
          pseudonym: remember.trim(),
          userId: resolved.how === 'userId' ? resolved.label : undefined,
        });
      }
      setStage('sent');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'The transfer failed.');
      setStage('confirm');
    }
  }

  if (stage === 'sent') {
    return (
      <>
        <h1>{t('send.sent')}</h1>
        <section className="card">
          <p>
            <strong>{formatAmount(base, denom)}</strong> is on its way to{' '}
            {resolved.label ?? <Named address={resolved.address!} />}.
          </p>
          <p className="small muted">
            Transaction <code>{txHash}</code>
          </p>
          <p>
            <Link to={`/a/${account.address}`}>{t('send.backToAccount')} →</Link>
          </p>
        </section>
      </>
    );
  }

  return (
    <>
      <p className="lede">
        To a user ID, a name from your address book, or an address.
      </p>

      <section className="card">
        <h2>{t('send.who')}</h2>
        <label className="field">
          <span>{t('send.whoLabel')}</span>
          <input
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            placeholder="NG-CAQ3-C04Z-M · Acme Ltd · yml1…"
            spellCheck={false}
            autoComplete="off"
            disabled={stage !== 'compose'}
          />
        </label>

        {resolved.how === 'looking' && <p className="small muted">{t('send.asking')}</p>}
        {resolved.how === 'unknown' && (
          <p className="notice notice--bad">
            No account answers to that. Check the ID — it carries a check character covering the
            country too, so a single mistyped letter is caught here rather than by sending to
            nobody.
          </p>
        )}
        {resolved.address && (
          <p className="notice notice--ok">
            {resolved.how === 'userId' ? (
              <>
                User ID <strong>{resolved.label}</strong>
              </>
            ) : resolved.how === 'pseudonym' ? (
              <>
                <strong>{resolved.label}</strong> from your address book
              </>
            ) : (
              <>{t('send.thatAddress')}</>
            )}{' '}
            → <code>{truncateAddress(resolved.address)}</code>
          </p>
        )}
      </section>

      <section className="card">
        <h2>{t('send.howMuch')}</h2>
        <div className="actions__row">
          {held.map((b) => {
            const d = resolveDenom(b.denom);
            return (
              <button
                key={b.denom}
                type="button"
                className={denom === b.denom ? 'chip chip--on' : 'chip'}
                onClick={() => setDenom(b.denom)}
                disabled={stage !== 'compose'}
              >
                {d.symbol}
              </button>
            );
          })}
        </div>
        <label className="field">
          <span>Amount in {info.symbol}</span>
          <input
            value={amount}
            onChange={(e) => setAmount(e.target.value.replace(/[^0-9.]/g, ''))}
            placeholder="0.00"
            inputMode="decimal"
            disabled={stage !== 'compose'}
          />
        </label>
        {chosen && (
          <p className="small muted">
            You hold {formatAmount(chosen.amount, denom)}.
          </p>
        )}
        {base && !enough && (
          <p className="notice notice--bad">{t('send.tooMuch')}</p>
        )}
      </section>

      {stage === 'compose' ? (
        <div className="claim-all">
          <button type="button" onClick={() => setStage('confirm')} disabled={!ready}>
            Review this payment
          </button>
        </div>
      ) : (
        <section className="card">
          <h2>{t('send.confirm')}</h2>
          {/* The full address, even when a name matched. The name is what you
              recognise; the address is what actually receives, and only one of
              those is checkable. */}
          <p>
            Sending <strong>{formatAmount(base, denom)}</strong> to{' '}
            {resolved.label ? <strong>{resolved.label}</strong> : 'this account'}:
          </p>
          <p>
            <code className="address">{resolved.address}</code>
          </p>

          {resolved.how !== 'pseudonym' && (
            <label className="field">
              <span>{t('send.saveAs')}</span>
              <input
                value={remember}
                onChange={(e) => setRemember(e.target.value)}
                placeholder="Acme Ltd"
              />
            </label>
          )}

          {!getUnlocked() && (
            <label className="field">
              <span>Password for “{account.label}”</span>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoFocus
              />
            </label>
          )}

          {error && <div className="notice notice--bad">{error}</div>}

          <div className="actions__row">
            <button
              type="button"
              onClick={confirmAndSend}
              disabled={stage === 'sending' || (!getUnlocked() && !password)}
            >
              {stage === 'sending' ? 'Sending…' : 'Send it'}
            </button>
            <button
              type="button"
              className="chip"
              onClick={() => setStage('compose')}
              disabled={stage === 'sending'}
            >
              Back
            </button>
          </div>
        </section>
      )}
    </>
  );
}

/**
 * The three ways to move money, on one screen.
 *
 * They were separate ideas in separate places, which made the safe option
 * invisible: somebody about to pay a stranger had no reason to discover that
 * escrow existed. Putting them side by side turns "how much do I trust this
 * person" into a visible choice made at the moment it matters.
 *
 *   - **Send**    — irreversible, instant, cheapest. For people you know.
 *   - **Escrow**  — the chain holds it until both sides agree, or a named
 *                   moderator decides. For people you do not.
 *   - **Demand**  — a signed request for payment; nothing moves until they act.
 */
export function SendPage() {
  const [folder, setFolder] = useState<'send' | 'escrow' | 'demand'>('send');

  const tabs = [
    { id: 'send', label: t('send.tabSend') },
    { id: 'escrow', label: t('send.tabEscrow') },
    { id: 'demand', label: t('send.tabDemand') },
  ] as const;

  return (
    <>
      <h1>{t('send.title')}</h1>

      <div className="folders" role="tablist">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={folder === tab.id}
            className={folder === tab.id ? 'folders__tab folders__tab--on' : 'folders__tab'}
            onClick={() => setFolder(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {folder === 'send' && <SendDirect />}
      {folder === 'escrow' && <SendEscrow />}
      {folder === 'demand' && <SendDemand />}
    </>
  );
}

/**
 * Escrow, from the wallet.
 *
 * The wallet does not implement the escrow transaction itself — that lives in
 * the Pay app, against x/treasury, where the moderator list and the dispute
 * flow already exist. Duplicating it here would mean two implementations of the
 * one screen that decides who gets somebody's money, which is exactly the sort
 * of duplication that ends with the two disagreeing.
 *
 * So this explains the mechanism honestly and hands off. Better an accurate
 * signpost than a second half-built custody flow.
 */
function SendEscrow() {
  return (
    <>
      <p className="lede">{t('send.escrowLede')}</p>
      <section className="card">
        <h2>{t('send.escrowHow')}</h2>
        <ol className="steps">
          <li>{t('send.escrowStep1')}</li>
          <li>{t('send.escrowStep2')}</li>
          <li>{t('send.escrowStep3')}</li>
        </ol>
        <p className="small muted">{t('send.escrowFee')}</p>
        <p>
          <a className="button" href="/app/" target="_blank" rel="noreferrer">
            {t('send.escrowOpen')}
          </a>
        </p>
      </section>
    </>
  );
}

/**
 * Asking to be paid.
 *
 * A demand is not a transaction: it is a signed statement of who wants what,
 * which the payer can verify came from the account it claims. Nothing moves
 * until they act on it, so this is safe to hand to a stranger.
 */
function SendDemand() {
  return (
    <>
      <p className="lede">{t('send.demandLede')}</p>
      <section className="card">
        <h2>{t('send.demandHow')}</h2>
        <p>{t('send.demandBody')}</p>
        <p className="small muted">{t('send.demandNote')}</p>
        <p>
          <a className="button" href="/app/" target="_blank" rel="noreferrer">
            {t('send.demandOpen')}
          </a>
        </p>
      </section>
    </>
  );
}
