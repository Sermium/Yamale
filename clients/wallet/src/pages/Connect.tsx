import {
  describeFee,
  describeRequestMessage,
  formatAmount,
  summariseSigningRequest,
  t,
  truncateAddress,
  type RequestMessage,
  type SigningRequestSummary,
} from '@yamale/chain';
import { useEffect, useMemo, useState } from 'react';
import { serveWalletRequests, fromBase64, toBase64 } from '@yamale/connect';
import type { AnyRequest, AnyResult } from '@yamale/connect';

import { Named } from '../Named.tsx';
import { getUnlocked, openVault, setUnlocked, touch, vaultSummary } from '../vault.ts';

/**
 * The approval window.
 *
 * This is the page an application opens when it wants an address or a
 * signature. It is the only screen in the system where a person authorises
 * something, so it is built around one question — *what exactly am I agreeing
 * to, and who is asking?* — and it answers that before offering a button.
 *
 * The origin shown is the browser's own `event.origin`, not anything the
 * calling page told us. A site claiming to be "Yamale Safe" in its name shows
 * its real address here regardless.
 */
type Pending = { request: AnyRequest; origin: string; resolve: (r: AnyResult) => void };

export function ConnectPage() {
  const [pending, setPending] = useState<Pending | null>(null);
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const summary = vaultSummary();

  useEffect(() => {
    return serveWalletRequests({
      // Every app here is served by the same nginx as this wallet. On a network
      // carrying value this becomes an explicit list.
      allowedOrigins: '*',
      onRequest: (request, origin) =>
        new Promise<AnyResult>((resolve) => setPending({ request, origin, resolve })),
    });
  }, []);

  if (!summary) {
    return (
      <section className="card">
        <h1>No account on this device</h1>
        <p className="muted">
          This wallet holds nothing yet. Create or import an account first, then the application can
          ask again.
        </p>
      </section>
    );
  }

  if (!pending) {
    return (
      <section className="card">
        <h1>Waiting</h1>
        <p className="muted">{t('msg.noRequest')}</p>
      </section>
    );
  }

  const unlocked = getUnlocked() !== null;

  async function approve() {
    setBusy(true);
    setError(null);
    try {
      let wallet = getUnlocked();
      if (!wallet) {
        wallet = await openVault(password);
        setUnlocked(wallet);
      }
      touch();

      const [account] = await wallet.getAccounts();
      const info = {
        address: account!.address,
        pubkey: toBase64(account!.pubkey),
        algo: 'secp256k1' as const,
      };

      const { request, resolve } = pending!;
      if (request.kind === 'connect' || request.kind === 'accounts') {
        resolve({ kind: request.kind, accounts: [info] } as AnyResult);
      } else {
        // Signed exactly as received. The wallet is entitled to alter the
        // document, but this one does not -- so it returns the same bytes it
        // put a signature over, which is what the caller must broadcast.
        const signed = await wallet.signDirect(request.signerAddress, {
          bodyBytes: fromBase64(request.bodyBytes),
          authInfoBytes: fromBase64(request.authInfoBytes),
          chainId: request.chainId,
          accountNumber: BigInt(request.accountNumber) as never,
        });
        resolve({
          kind: 'signDirect',
          signature: signed.signature.signature,
          bodyBytes: toBase64(signed.signed.bodyBytes),
          authInfoBytes: toBase64(signed.signed.authInfoBytes),
        });
      }
      setPending(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not complete that.');
    } finally {
      setBusy(false);
    }
  }

  function reject() {
    pending!.resolve({ kind: 'error', code: 'rejected', message: 'You declined.' });
    setPending(null);
    window.close();
  }

  // Narrowed once here rather than compared again inside the JSX: the `kind`
  // check has to be the thing that produces the typed request, or the compiler
  // cannot know the sign-only fields exist.
  const signRequest = pending.request.kind === 'signDirect' ? pending.request : null;

  return (
    <>
      <h1>{signRequest ? 'Approve this transaction' : 'Connect'}</h1>

      <section className="card">
        <h2>Who is asking</h2>
        <p>
          <code className="address">{pending.origin}</code>
        </p>
        <p className="small muted">
          This is the address the browser reports, not a name the site chose for itself.
        </p>
      </section>

      {signRequest ? (
        <SigningSummary request={signRequest} />
      ) : (
        <section className="card">
          <h2>What it gets</h2>
          <p>
            Your address, <code className="address">{summary.address}</code>, and your public key.
            Not your recovery phrase, and not permission to move anything — every payment comes
            back here first.
          </p>
        </section>
      )}

      <section className="card">
        {!unlocked && (
          <label className="field">
            <span>Password for “{summary.label}”</span>
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
          <button type="button" onClick={approve} disabled={busy || (!unlocked && !password)}>
            {busy ? 'Working…' : 'Approve'}
          </button>
          <button type="button" className="chip" onClick={reject} disabled={busy}>
            Reject
          </button>
        </div>
      </section>
    </>
  );
}

/**
 * What is actually in the transaction.
 *
 * Decoded from the bytes about to be signed rather than from anything the
 * application described — a summary taken on trust from the requester is worth
 * nothing, because a malicious one would simply describe a different payment
 * than it sent.
 *
 * This screen used to stop at the type URL. It listed
 * `/cosmos.group.v1.MsgSubmitProposal` and then said, honestly, that amounts
 * and recipients "are not yet decoded here — read them in the application
 * before approving". Which is the one instruction a wallet must never give: the
 * application is the party being checked. The decode now runs through the
 * chain's own generated encoders (see the SDK's signrequest.ts), so the sentence
 * on screen is derived from the bytes and from nothing else.
 */
function SigningSummary({ request }: { request: Extract<AnyRequest, { kind: 'signDirect' }> }) {
  // Memoised on the bytes: decoding builds a protobuf registry, and this
  // component re-renders on every keystroke in the password field below it.
  const summary: SigningRequestSummary = useMemo(
    () => summariseSigningRequest(fromBase64(request.bodyBytes), fromBase64(request.authInfoBytes)),
    [request.bodyBytes, request.authInfoBytes],
  );

  if (summary.undecodable) {
    return (
      <section className="card">
        <h2>What you are signing</h2>
        <div className="notice notice--bad">
          <strong>These bytes are not a transaction.</strong> They could not be decoded at all,
          which means nothing can be said about what signing them would do.{' '}
          <strong>Do not approve.</strong>
        </div>
        <details className="payload">
          <summary>Why it could not be read</summary>
          <pre className="payload__pre">{summary.undecodable}</pre>
        </details>
      </section>
    );
  }

  const fee = describeFee(summary.fee);

  return (
    <section className="card">
      <h2>What you are signing</h2>

      {/* One sentence first. Somebody who reads nothing else on this screen
          should still have been told what leaves their account. */}
      <p className="sign__headline">
        {summary.messages.length === 1
          ? describeRequestMessage(summary.messages[0]!)
          : `${summary.messages.length} actions, in this order:`}
      </p>

      {summary.incomplete && (
        <div className="notice notice--bad">
          <strong>Part of this could not be read.</strong> At least one message in it is not a type
          this wallet knows, so what it does is unknown — not harmless. Approve it only if you can
          account for every line below.
        </div>
      )}

      <ol className="sign__list">
        {summary.messages.map((message, i) => (
          <MessageRow key={i} message={message} depth={0} />
        ))}
      </ol>

      {/* The fee, only when there is one. On this chain it is normally zero or
          sponsored, and printing "fee: 0 YML" on every approval is how people
          learn to skip the line where a real fee would appear. */}
      {fee && <p className="sign__fee">{fee}</p>}

      {summary.memo && (
        <p className="small">
          <span className="y-label">On the ledger as a memo</span>
          <br />
          <em>{summary.memo}</em>{' '}
          <span className="muted">— public and permanent.</span>
        </p>
      )}

      <p className="small muted sign__who">
        Signing as <Named address={request.signerAddress} /> on{' '}
        <code>{request.chainId}</code>.
      </p>
    </section>
  );
}

/**
 * One message, its amounts and its nesting.
 *
 * A nested message is indented rather than flattened. The difference between
 * "this account pays 250 YML" and "this account proposes that a shared account
 * pays 250 YML" is the whole of what a signer is deciding, and flattening the
 * two loses it.
 */
function MessageRow({ message, depth }: { message: RequestMessage; depth: number }) {
  if (message.problem) {
    return (
      <li className="sign__row sign__row--bad">
        <strong>{describeRequestMessage(message)}</strong>
        <p className="small muted">
          {message.problem === 'unregistered'
            ? 'This wallet has no decoder for that message type, so it cannot say what it does.'
            : 'The bytes do not match the type they claim to be.'}
        </p>
        <code className="y-addr">{message.typeUrl}</code>
      </li>
    );
  }

  const decoded = message.decoded!;

  return (
    <li className="sign__row">
      <div className="sign__title">{decoded.title}</div>
      <p className="sign__summary">{decoded.summary}</p>

      {/* The amount again, on its own, in mono. It is in the sentence above,
          but a figure inside a sentence is not a figure anybody double-checks. */}
      {decoded.coins && decoded.coins.length > 0 && (
        <p className="sign__amount">
          {decoded.coins.map((c) => formatAmount(c.amount, c.denom)).join(' + ')}
        </p>
      )}

      {decoded.counterparty && (
        <p className="small muted">
          To <Named address={decoded.counterparty} />{' '}
          <code className="y-addr">{truncateAddress(decoded.counterparty)}</code>
        </p>
      )}

      {decoded.details && decoded.details.length > 0 && (
        <dl className="sign__details">
          {decoded.details.map((d, i) => (
            <div key={i}>
              <dt>{d.label}</dt>
              <dd className={d.address ? 'y-addr' : undefined}>
                {d.address && d.value.startsWith('yml1') ? truncateAddress(d.value) : d.value}
              </dd>
            </div>
          ))}
        </dl>
      )}

      {message.contains.length > 0 && (
        <>
          <p className="y-label sign__contains">
            {message.contains.length === 1 ? 'It carries this action' : 'It carries these actions'}
          </p>
          <ol className="sign__list">
            {message.contains.map((inner, i) => (
              <MessageRow key={i} message={inner} depth={depth + 1} />
            ))}
          </ol>
        </>
      )}

      {/* The raw message, always reachable. An expert who cannot audit what
          they are signing stops trusting the wallet, and that trust is the
          product. */}
      <details className="payload">
        <summary>Raw</summary>
        <pre className="payload__pre">{JSON.stringify(decoded.raw, null, 2)}</pre>
      </details>
    </li>
  );
}
