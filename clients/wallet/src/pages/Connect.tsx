import { t } from '@yamale/chain';
import { useEffect, useState } from 'react';
import { DirectSecp256k1HdWallet } from '@cosmjs/proto-signing';
import { TxBody } from 'cosmjs-types/cosmos/tx/v1beta1/tx';
import { serveWalletRequests, fromBase64, toBase64 } from '@yamale/connect';
import type { AnyRequest, AnyResult } from '@yamale/connect';

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

  return (
    <>
      <h1>{pending.request.kind === 'signDirect' ? 'Approve this transaction' : 'Connect'}</h1>

      <section className="card">
        <h2>Who is asking</h2>
        <p>
          <code className="address">{pending.origin}</code>
        </p>
        <p className="small muted">
          This is the address the browser reports, not a name the site chose for itself.
        </p>
      </section>

      {pending.request.kind === 'signDirect' ? (
        <SigningSummary request={pending.request} />
      ) : (
        <section className="card">
          <h2>What it gets</h2>
          <p>
            Your address, <code>{summary.address}</code>, and your public key. Not your recovery
            phrase, and not permission to move anything — every payment comes back here first.
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
 */
function SigningSummary({ request }: { request: Extract<AnyRequest, { kind: 'signDirect' }> }) {
  let messages: string[] = [];
  let memo = '';
  let failed = false;

  try {
    const body = TxBody.decode(fromBase64(request.bodyBytes));
    messages = body.messages.map((m) => m.typeUrl);
    memo = body.memo;
  } catch {
    failed = true;
  }

  return (
    <section className="card">
      <h2>What you are signing</h2>
      {failed ? (
        <div className="notice notice--bad">
          These bytes could not be decoded. Do not approve them.
        </div>
      ) : (
        <>
          <ul className="plain">
            {messages.map((typeUrl, i) => (
              <li key={i}>
                <code>{typeUrl}</code>
              </li>
            ))}
          </ul>
          {memo && (
            <p className="small muted">
              Memo: <em>{memo}</em>
            </p>
          )}
          <p className="small muted">
            Chain <code>{request.chainId}</code>, signing as{' '}
            <code>{request.signerAddress}</code>.
          </p>
          {/* Named rather than quietly omitted: showing a type URL is not the
              same as showing an amount, and a wallet that implies it checked
              more than it did is worse than one that admits the gap. */}
          <p className="small muted">
            Amounts and recipients are inside these messages and are not yet decoded here — read
            them in the application before approving.
          </p>
        </>
      )}
    </section>
  );
}
