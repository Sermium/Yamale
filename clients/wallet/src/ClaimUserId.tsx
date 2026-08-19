import { useEffect, useState } from 'react';
import { ChainSigner, formatUserId, registerAlias } from '@yamale/chain';

import { client } from './chain.ts';
import { getUnlocked, openVault, setUnlocked, touch } from './vault.ts';

const CHAIN_ID = import.meta.env.VITE_CHAIN_ID ?? 'yamale-devnet-1';
// The trailing slash is load-bearing: nginx matches `location /api/rpc/`, and
// without it the POST falls through to the app's own index.html, which CosmJS
// then reports as a JSON parse error.
const RPC = `${window.location.origin}/api/rpc/`;

/**
 * Claiming the account's user ID.
 *
 * Offered rather than done automatically. It is a transaction: it costs a fee,
 * it needs the vault unlocked, and it puts a permanent public record against
 * the address. Doing that silently during account creation would be spending
 * somebody's money and publishing their account without asking.
 *
 * There is nothing to choose. The chain derives the identifier, so this is a
 * button and not a form — which is the whole reason the identifier cannot be
 * squatted or spoofed.
 */
export function ClaimUserId({ address, label }: { address: string; label: string }) {
  const [id, setId] = useState<string | null | undefined>(undefined);
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [asking, setAsking] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void client.userIdOf(address).then((v) => !cancelled && setId(v));
    return () => {
      cancelled = true;
    };
  }, [address]);

  async function claim() {
    setBusy(true);
    setError(null);
    try {
      let wallet = getUnlocked();
      if (!wallet) {
        wallet = await openVault(password);
        setUnlocked(wallet);
      }
      touch();

      const signer = new ChainSigner(wallet, { rpcUrl: RPC, chainId: CHAIN_ID });
      const result = await signer.submit([registerAlias(address)]);
      if (!result.succeeded) {
        throw new Error(result.error?.message ?? `Rejected with code ${result.code}`);
      }

      // The identifier is assigned by the chain, so it is read back rather than
      // predicted. A short wait lets the block commit before asking.
      await new Promise((r) => setTimeout(r, 6000));
      setId(await client.userIdOf(address));
      setAsking(false);
      setPassword('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not claim an ID.');
    } finally {
      setBusy(false);
    }
  }

  if (id === undefined) {
    return (
      <p className="acct__row">
        <span className="acct__label">User ID</span>
        <span className="small muted">checking…</span>
      </p>
    );
  }

  if (id) {
    return (
      <p className="acct__row">
        <span className="acct__label">User ID</span>
        <code>{formatUserId(id)}</code>
        <span className="small muted">
          People can pay you with this instead of your address. The first two letters are the
          country your account is registered in.
        </span>
      </p>
    );
  }

  return (
    <div className="acct__row">
      <span className="acct__label">User ID</span>
      {!asking ? (
        <>
          <span className="small muted">None yet.</span>
          <button type="button" className="acct__minor" onClick={() => setAsking(true)}>
            Claim one
          </button>
        </>
      ) : (
        <>
          <span className="small muted">
            The chain picks it — there is nothing to choose, which is what stops anyone taking a
            name that looks like yours. It carries the country your account is registered in, so
            your bank has to have recorded that first. Costs a small fee and is permanent.
          </span>
          {!getUnlocked() && (
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={`Password for ${label}`}
              autoFocus
            />
          )}
          {error && <span className="notice notice--bad">{error}</span>}
          <button type="button" onClick={claim} disabled={busy || (!getUnlocked() && !password)}>
            {busy ? 'Claiming…' : 'Claim my user ID'}
          </button>
        </>
      )}
    </div>
  );
}
