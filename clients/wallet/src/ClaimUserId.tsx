import { useEffect, useState } from 'react';
import { ChainSigner, formatUserId, registerAlias, t } from '@yamale/chain';

import { client } from './chain.ts';
import { getUnlocked, openVault, setUnlocked, touch } from './vault.ts';

const CHAIN_ID = import.meta.env.VITE_CHAIN_ID ?? 'yamale-devnet-2';
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
        throw new Error(result.error?.message ?? t('wal.rejectedWithCode', { code: String(result.code) }));
      }

      // The identifier is assigned by the chain, so it is read back rather than
      // predicted. A short wait lets the block commit before asking.
      await new Promise((r) => setTimeout(r, 6000));
      setId(await client.userIdOf(address));
      setAsking(false);
      setPassword('');
    } catch (err) {
      setError(err instanceof Error ? err.message : t('error.unknown'));
    } finally {
      setBusy(false);
    }
  }

  if (id === undefined) {
    return <p className="wal-muted wal-small">{t('acct.reading')}</p>;
  }

  if (id) {
    return (
      <>
        <p className="wal-idBig y-mono">{formatUserId(id)}</p>
        <p className="wal-small wal-muted">{t('wal.userIdBody')}</p>
      </>
    );
  }

  if (!asking) {
    return (
      <>
        <p className="wal-muted">{t('wal.noUserIdYet')}</p>
        <button type="button" className="wal-chip" onClick={() => setAsking(true)}>
          {t('wal.claimOne')}
        </button>
      </>
    );
  }

  return (
    <>
      <p className="wal-small wal-muted">{t('wal.claimExplain')}</p>
      {!getUnlocked() && (
        <label className="wal-field">
          <span className="wal-field__label">{t('wal.passwordFor', { label })}</span>
          <input
            className="wal-input"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            autoFocus
          />
        </label>
      )}
      {error && <div className="wal-note wal-note--bad">{error}</div>}
      <button type="button" className="wal-btn" onClick={claim}
              disabled={busy || (!getUnlocked() && !password)}>
        {busy ? t('wal.claiming') : t('wal.claimMyUserId')}
      </button>
    </>
  );
}
