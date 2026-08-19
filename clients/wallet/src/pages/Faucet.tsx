import { useState } from 'react';
import { Link } from 'react-router-dom';
import { formatAmount, listContacts, resolveDenom, truncateAddress, t, validUserId } from '@yamale/chain';

import { vaultSummary } from '../vault.ts';

/**
 * What the faucet is stocked with — every currency the chain issues.
 *
 * This used to be a hand-written six while the chain had grown to 45, so most
 * of the money on the network was unreachable from this page. It is now
 * generated from scripts/currencies/african-currencies.json, the same table
 * that builds the faucet's own --currencies list, so the page cannot offer
 * something the faucet does not hold, or omit something it does.
 */
import { CLAIMABLE, CLAIM_GROUPS, type Denom } from '../claimable.ts';

/** One grant waits a block so the faucet's transactions cannot collide. */
const SECONDS_PER_GRANT = 7;

type Status =
  | { state: 'idle' }
  | { state: 'busy' }
  | { state: 'sent'; sent: string; txHash: string }
  | { state: 'error'; message: string; retryAfter?: number };

/**
 * Claiming test funds.
 *
 * Requires a connected account. Not to gate the money — the funds are worthless
 * and the endpoint is open — but because everything else on this page reads
 * better once there is an account: the destination is known, the results have
 * somewhere to go, and the common case needs no typing at all.
 *
 * The destination field therefore appears only to *override* that default. A
 * blank address box shown to somebody whose address the page already knows is
 * an invitation to paste the wrong thing.
 */
export function FaucetPage() {
  const account = vaultSummary();
  const [elsewhere, setElsewhere] = useState(false);
  const [typed, setTyped] = useState('');
  const [status, setStatus] = useState<Record<string, Status>>({});
  const [allBusy, setAllBusy] = useState(false);
  const [progress, setProgress] = useState(0);

  if (!account) {
    return (
      <>
        <h1>{t('faucet.title')}</h1>
        <section className="card">
          <h2>{t('faucet.connectFirst')}</h2>
          <p className="muted">
            The faucet sends to an account, so there has to be one. Generate a new account or load
            one you already have — it takes a few seconds and stays on this device.
          </p>
          <p>
            <Link to="/create">Create an account</Link> · <Link to="/import">Load an existing one</Link>
          </p>
        </section>
      </>
    );
  }

  const resolved = elsewhere ? resolveDestination(typed) : { address: account.address, how: 'self' as const };
  const destination = resolved.address;

  async function claim(denom: Denom): Promise<void> {
    if (!destination) return;
    setStatus((s) => ({ ...s, [denom]: { state: 'busy' } }));
    try {
      const response = await fetch('/api/faucet/', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        // `denom` names an *additional* currency the faucet was configured to
        // hand out. YML is the default grant and is not in that list, so asking
        // for it by name is refused — the field has to be omitted instead.
        body: JSON.stringify(
          denom === 'uyml' ? { address: destination } : { address: destination, denom },
        ),
      });
      const body = await response.json();
      setStatus((s) => ({
        ...s,
        [denom]: body.error
          ? { state: 'error', message: body.error, retryAfter: body.retry_after }
          : { state: 'sent', sent: body.sent, txHash: body.tx_hash },
      }));
    } catch {
      setStatus((s) => ({
        ...s,
        [denom]: { state: 'error', message: 'Could not reach the faucet.' },
      }));
    }
  }

  /**
   * Sequential, not parallel. The faucet holds a lock for one block per grant
   * so its transactions cannot collide on the account sequence — firing six at
   * once would queue on that lock while the browser showed six spinners and no
   * progress. One at a time makes the waiting legible, and the counter below
   * makes it finite.
   */
  async function claimAll() {
    setAllBusy(true);
    setProgress(0);
    for (const [i, denom] of CLAIMABLE.entries()) {
      setProgress(i + 1);
      await claim(denom);
    }
    setAllBusy(false);
    setProgress(0);
  }

  const total = CLAIMABLE.length * SECONDS_PER_GRANT;

  return (
    <>
      <h1>{t('faucet.title')}</h1>
      <p className="lede">
        Free tokens on the Yamale devnet. They are worth nothing and the chain they live on can be
        reset at any time — do not treat a balance here as money.
      </p>

      <section className="card">
        <h2>{t('faucet.sendingTo')}</h2>
        {!elsewhere ? (
          <>
            <p>
              <strong>{account.label}</strong> — <code>{truncateAddress(account.address)}</code>
            </p>
            <button type="button" className="linkish" onClick={() => setElsewhere(true)}>
              Send somewhere else instead
            </button>
          </>
        ) : (
          <>
            <label className="field">
              <span>Address, user ID, or a name from your address book</span>
              <input
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
                placeholder="yml1… · K3M9-7QRT-B · Acme Ltd"
                spellCheck={false}
                autoComplete="off"
                autoFocus
              />
            </label>
            <DestinationHint typed={typed} resolved={resolved} />
            <button type="button" className="linkish" onClick={() => { setElsewhere(false); setTyped(''); }}>
              ← Send to my own account
            </button>
          </>
        )}
      </section>

      {/* Said before anything is clicked, not discovered halfway through. The
          wait is inherent — each grant deliberately waits out a block so the
          faucet's transactions cannot collide — so the honest move is to state
          the cost up front rather than leave somebody watching a spinner and
          wondering whether it has hung. */}
      <div className="claim-all">
        <button type="button" onClick={claimAll} disabled={!destination || allBusy}>
          {allBusy
            ? `Claiming ${progress} of ${CLAIMABLE.length}…`
            : `Claim all ${CLAIMABLE.length} currencies (~${Math.round((CLAIMABLE.length * SECONDS_PER_GRANT) / 60)} min)`}
        </button>
        <p className="small muted">
          {allBusy
            ? `About ${Math.max(0, (CLAIMABLE.length - progress) * SECONDS_PER_GRANT)} seconds left. Leave this page open.`
            : `Takes about ${total} seconds. Each grant waits for a block to be produced before the next is sent, so the faucet's transactions cannot collide — that wait is why it is not instant.`}
        </p>
      </div>

      {/* Grouped rather than one flat run of forty-five. The reserve and the
          two settlement stablecoins come first because that is what most
          testers actually want; the rest are by region, because "which of
          these is my country's" is the question being asked and a map is how
          people hold that. */}
      {CLAIM_GROUPS.map((group) => (
        <section key={group.id} className="claim-group">
          <h2 className="claim-group__title">
            {group.title}
            <span className="claim-group__count">{group.denoms.length}</span>
          </h2>
          <div className="grid">
            {group.denoms.map((denom) => (
              <CurrencyPanel
                key={denom}
                denom={denom as Denom}
                status={status[denom] ?? { state: 'idle' }}
                disabled={!destination || allBusy}
                onClaim={() => claim(denom as Denom)}
              />
            ))}
          </div>
        </section>
      ))}
    </>
  );
}

type Resolution =
  | { address: string; how: 'self' | 'address' | 'pseudonym'; label?: string }
  | { address: null; how: 'empty' | 'userid-unsupported' | 'unknown' };

/**
 * Turns whatever was typed into an address.
 *
 * Accepts three things on purpose. A raw address is what the chain wants; a
 * name from the address book is what a person remembers; a user ID is what the
 * chain hands out. The third is recognised and refused *explicitly* rather than
 * falling through to "not found", because "that is a user ID and this screen
 * cannot resolve one" and "no such contact" are different problems and only one
 * of them is the user's mistake.
 *
 * This stays synchronous, which is why the user ID is refused rather than
 * resolved: resolving means a request, and the faucet's destination field is
 * validated on every keystroke.
 */
function resolveDestination(input: string): Resolution {
  const value = input.trim();
  if (!value) return { address: null, how: 'empty' };

  if (/^yml1[0-9a-z]{38,}$/.test(value)) return { address: value, how: 'address' };

  const contact = listContacts().find(
    (c) => c.pseudonym.toLowerCase() === value.toLowerCase(),
  );
  if (contact) return { address: contact.address, how: 'pseudonym', label: contact.pseudonym };

  // A user ID, recognised by the chain's own rule rather than by a regex that
  // has to be updated here every time the identifier changes shape — which is
  // exactly what happened when the country prefix was added.
  if (validUserId(value)) {
    return { address: null, how: 'userid-unsupported' };
  }
  return { address: null, how: 'unknown' };
}

function DestinationHint({ typed, resolved }: { typed: string; resolved: Resolution }) {
  if (!typed.trim()) return null;

  if (resolved.how === 'pseudonym') {
    return (
      <p className="notice notice--ok">
        <strong>{resolved.label}</strong> from your address book —{' '}
        <code>{truncateAddress(resolved.address!)}</code>
      </p>
    );
  }
  if (resolved.how === 'address') {
    return <p className="small muted">{t('msg.sendingToAddress')}</p>;
  }
  if (resolved.how === 'userid-unsupported') {
    return (
      <p className="notice">
        That is a user ID. This screen resolves addresses and address-book names only — send from
        the Send screen to pay a user ID, or paste the address here.
      </p>
    );
  }
  return (
    <p className="notice notice--bad">
      Not an address, and not a name in your address book. Yamale addresses start with{' '}
      <code>yml1</code>.
    </p>
  );
}

function CurrencyPanel({
  denom,
  status,
  disabled,
  onClaim,
}: {
  denom: Denom;
  status: Status;
  disabled: boolean;
  onClaim: () => void;
}) {
  const info = resolveDenom(denom);

  return (
    <article className="card">
      <h3>
        {info.symbol} <span className="small muted">{info.name}</span>
      </h3>

      {/* The fee point belongs beside the currencies it applies to, and is not
          worth saying under YML — "an account holding only YML can move it
          without holding YML" is what the shared wording produced. */}
      {denom === 'uyml' ? (
        <p className="small muted">{t('msg.paysFee')}</p>
      ) : (
        <p className="small muted">{t('msg.feeAllowance')}</p>
      )}

      <button type="button" onClick={onClaim} disabled={disabled || status.state === 'busy'}>
        {status.state === 'busy' ? 'Asking…' : `Claim ${info.symbol}`}
      </button>

      {status.state === 'sent' && (
        <div className="notice notice--ok">
          <strong>{formatAmount(status.sent.replace(/[a-z]+$/, ''), denom)}</strong> sent.
          <br />
          <code className="small">{status.txHash.slice(0, 16)}…</code>
        </div>
      )}

      {status.state === 'error' && (
        <div className="notice notice--bad">
          {/* The faucet's own message already ends with "Try again in 6h0m0s".
              Appending a humanised wait on top printed it twice, in two
              formats. Strip the machine one, keep the readable one. */}
          {status.message.replace(/\s*Try again in [^.]*\.\s*$/i, '')}
          {status.retryAfter ? ` Try again in ${humanWait(status.retryAfter)}.` : ''}
        </div>
      )}
    </article>
  );
}

/**
 * A wait, in words somebody can act on.
 *
 * Rounding seconds to minutes produced "try again in 0m" for a 22-second wait,
 * which reads as broken rather than as "nearly ready" — so each scale keeps a
 * unit that cannot round to zero.
 */
function humanWait(seconds: number): string {
  if (seconds < 60) return `${Math.max(1, Math.round(seconds))} seconds`;
  if (seconds < 3600) return `${Math.round(seconds / 60)} minutes`;
  return `${Math.round(seconds / 360) / 10} hours`;
}
