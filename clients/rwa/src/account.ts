/**
 * Attaching an account, in either of the two modes address.ts describes.
 *
 * The React half only. Everything that can be decided without a browser lives
 * in address.ts and is tested there.
 */
import { useCallback, useEffect, useState } from 'react';
import { YamaleWallet } from '@yamale/connect';

import { looksLikeAddress, type Account } from './address.ts';
import { CHAIN_ID } from './chain.ts';

const WATCH_KEY = 'yamale.rwa.watch';

/**
 * Where the wallet is.
 *
 * Same origin by default: the site serves every console as a subdirectory, so
 * the wallet sits beside this app rather than apart from it. That weakens the
 * isolation @yamale/connect was designed around — on one origin an injection
 * into any console can read the wallet's storage — and a separate host is how a
 * deployment would give it a separate origin, which it should — changed here
 * rather than by configuration, matching how chain.ts holds the RPC and REST
 * endpoints, because this app has no build-time environment.
 */
const WALLET_URL = new URL('/wallet', window.location.origin).toString();

export interface AccountApi {
  account: Account;
  /** Set while a connection is in flight, so a button can say so. */
  connecting: boolean;
  /** Catalogue key for the last failure, or null. */
  problemKey: string | null;
  signIn(): Promise<void>;
  watch(address: string): boolean;
  forget(): void;
}

export function useAccount(): AccountApi {
  const [account, setAccount] = useState<Account>({ mode: 'none' });
  const [connecting, setConnecting] = useState(false);
  const [problemKey, setProblemKey] = useState<string | null>(null);

  // A watched address is restored; a connection is not. Re-attaching a signer
  // silently on load would mean the wallet opening on a page somebody opened
  // only to read, which trains people to approve prompts they did not ask for.
  useEffect(() => {
    try {
      const saved = localStorage.getItem(WATCH_KEY);
      if (saved && looksLikeAddress(saved)) setAccount({ mode: 'watching', address: saved });
    } catch {
      // Private browsing. Watching still works for this session.
    }
  }, []);

  const signIn = useCallback(async () => {
    setConnecting(true);
    setProblemKey(null);
    try {
      const wallet = new YamaleWallet({
        walletUrl: WALLET_URL,
        chainId: CHAIN_ID,
        appName: 'Yamale investments',
      });
      await wallet.connect();
      const accounts = await wallet.getAccounts();
      const address = accounts[0]?.address ?? '';
      if (!address) throw new Error('no accounts');
      setAccount({ mode: 'connected', address, signer: wallet });
    } catch {
      // Refused, cancelled, or the wallet never opened — all the same to the
      // reader, which is that this did not connect.
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

  return { account, connecting, problemKey, signIn, watch, forget };
}
