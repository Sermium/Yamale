/**
 * Attaching an account, in either of the two modes address.ts describes.
 *
 * The React half only. Everything that can be decided without a browser lives
 * in address.ts and is tested there.
 */
import { useCallback, useEffect, useState } from 'react';
import { connect, availableWallets, type WalletProvider } from '@yamale/chain';

import { looksLikeAddress, type Account } from './address.ts';
import { CHAIN_ID } from './chain.ts';

const WATCH_KEY = 'yamale.rwa.watch';

const CHAIN = {
  chainId: CHAIN_ID,
  chainName: 'Yamale',
  rpcUrl: `${window.location.origin}/api/rpc/`,
  restUrl: `${window.location.origin}/api/rest`,
  baseDenom: 'uyml',
  displayDenom: 'YML',
  exponent: 6,
  bech32Prefix: 'yml',
  gasPrice: 0,
};

export interface AccountApi {
  account: Account;
  wallets: WalletProvider[];
  /** Set while a connection is in flight, so a button can say so. */
  connecting: boolean;
  /** Catalogue key for the last failure, or null. */
  problemKey: string | null;
  connectWallet(id: WalletProvider['id']): Promise<void>;
  watch(address: string): boolean;
  forget(): void;
}

export function useAccount(): AccountApi {
  const [account, setAccount] = useState<Account>({ mode: 'none' });
  const [connecting, setConnecting] = useState(false);
  const [problemKey, setProblemKey] = useState<string | null>(null);

  // A watched address is restored; a connection is not. Re-attaching a signer
  // silently on load would mean a wallet prompt appearing on a page somebody
  // opened only to read, which trains people to approve prompts they did not
  // ask for.
  useEffect(() => {
    try {
      const saved = localStorage.getItem(WATCH_KEY);
      if (saved && looksLikeAddress(saved)) setAccount({ mode: 'watching', address: saved });
    } catch {
      // Private browsing. Watching still works for this session.
    }
  }, []);

  const connectWallet = useCallback(async (id: WalletProvider['id']) => {
    setConnecting(true);
    setProblemKey(null);
    try {
      const signer = await connect(id, CHAIN);
      const accounts = await signer.getAccounts();
      const address = accounts[0]?.address ?? '';
      if (!address) throw new Error('no accounts');
      setAccount({ mode: 'connected', address, signer, wallet: id });
    } catch {
      // Refused, cancelled, or not installed — all the same to the reader,
      // which is that this did not connect.
      setProblemKey('rwa.connectFailed');
    } finally {
      setConnecting(false);
    }
  }, []);

  const watch = useCallback((address: string): boolean => {
    const trimmed = address.trim();
    if (!looksLikeAddress(trimmed)) {
      setProblemKey('rwa.badAddress');
      return false;
    }
    setProblemKey(null);
    setAccount({ mode: 'watching', address: trimmed });
    try { localStorage.setItem(WATCH_KEY, trimmed); } catch { /* private mode */ }
    return true;
  }, []);

  const forget = useCallback(() => {
    setAccount({ mode: 'none' });
    setProblemKey(null);
    try { localStorage.removeItem(WATCH_KEY); } catch { /* private mode */ }
  }, []);

  return {
    account,
    wallets: availableWallets(),
    connecting,
    problemKey,
    connectWallet,
    watch,
    forget,
  };
}
