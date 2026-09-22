/**
 * Signing in, and the one place a signature is requested.
 *
 * The explorer was read-only by design, and mostly still is. What changed is
 * that everything it explains — staking, voting — previously ended with "now go
 * and run this on a command line", which is a fine answer for a validator
 * operator and no answer at all for anyone else.
 *
 * The moment before a signature is the highest-stakes moment in the product, so
 * it gets its own component rather than being improvised per action: every
 * action states plainly what will change before it asks, and reports what the
 * chain actually did afterwards.
 *
 * # This no longer looks for a browser extension
 *
 * It used to offer Keplr and Leap. It now opens the Yamale wallet, which holds
 * an enrolled account: an ordinary secp256k1 key sealed under a password and
 * split three ways, so losing the device is survivable. See
 * clients/wallet/src/account/ and the recovery package.
 *
 * The reason to drop the extensions rather than keep them alongside is not
 * tidiness. An extension account cannot be recovered by this system, cannot be
 * enrolled, and looks identical in every screen to one that can — so offering
 * both would mean the interface could not truthfully answer "what happens if I
 * lose this device?" for the account in front of it.
 *
 * `YamaleWallet` implements CosmJS's `OfflineDirectSigner`, so nothing below
 * the connection changed: ChainSigner takes it exactly as it took an extension.
 */

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { YamaleWallet } from '@yamale/connect';
import {
  ChainSigner,
  translateError,
  type ChainInfo,
  type EncodeObject,
  type SubmitResult,
} from '@yamale/chain';

/**
 * The chain, as a wallet needs it described.
 *
 * Read from the build environment so a hosted explorer points at the network it
 * was built for; the defaults are the local devnet.
 */
export const CHAIN: ChainInfo = {
  chainId: import.meta.env.VITE_CHAIN_ID ?? 'yamale-devnet-2',
  chainName: import.meta.env.VITE_CHAIN_NAME ?? 'Yamale',
  rpcUrl: import.meta.env.VITE_PUBLIC_RPC_URL ?? 'http://localhost:26657',
  restUrl: import.meta.env.VITE_PUBLIC_REST_URL ?? 'http://localhost:1317',
  baseDenom: 'uyml',
  displayDenom: 'YML',
  exponent: 6,
  bech32Prefix: 'yml',
  gasPrice: Number(import.meta.env.VITE_GAS_PRICE ?? 0),
};

/**
 * Where the wallet is.
 *
 * Same origin by default, because the site serves every console as a
 * subdirectory — see deploy/stage-site.sh. That is worth saying out loud rather
 * than leaving implicit: the wallet is meant to be its OWN origin, serving no
 * third-party script, and on one origin an injection into any console can read
 * its storage. The postMessage checks in @yamale/connect still run, they just
 * are not carrying the weight they were designed to carry.
 *
 * VITE_WALLET_URL is how a deployment gives the wallet a separate origin, and
 * it should.
 */
const WALLET_URL: string =
  import.meta.env.VITE_WALLET_URL ?? new URL('/wallet', globalThis.location.origin).toString();

interface WalletState {
  address: string | null;
  connecting: boolean;
  error: string | null;
  connect: () => Promise<void>;
  disconnect: () => void;
  submit: (messages: EncodeObject[], memo?: string, gas?: number) => Promise<SubmitResult>;
}

const WalletContext = createContext<WalletState | null>(null);

export function useWallet(): WalletState {
  const state = useContext(WalletContext);
  if (!state) throw new Error('useWallet used outside WalletProvider');
  return state;
}

export function WalletProviderScope({ children }: { children: ReactNode }) {
  const [signer, setSigner] = useState<ChainSigner | null>(null);
  const [address, setAddress] = useState<string | null>(null);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Held in a ref-like state so disconnect can tell the wallet window to go
  // away rather than only forgetting about it locally.
  const [wallet, setWallet] = useState<YamaleWallet | null>(null);

  const connect = useCallback(async () => {
    setConnecting(true);
    setError(null);
    try {
      const opened = new YamaleWallet({
        walletUrl: WALLET_URL,
        chainId: CHAIN.chainId,
        appName: 'Yamale explorer',
      });
      await opened.connect();
      setWallet(opened);
      const chainSigner = new ChainSigner(opened, {
        rpcUrl: CHAIN.rpcUrl,
        chainId: CHAIN.chainId,
        feeDenom: CHAIN.baseDenom,
        gasPrice: CHAIN.gasPrice,
      });
      setAddress(await chainSigner.address());
      setSigner(chainSigner);
    } catch (e) {
      // A refused connection is the common case and is not an error worth
      // shouting about — somebody changed their mind.
      const message = e instanceof Error ? e.message : String(e);
      setError(/reject|denied/i.test(message) ? null : message);
    } finally {
      setConnecting(false);
    }
  }, []);

  const disconnect = useCallback(() => {
    signer?.disconnect();
    wallet?.disconnect();
    setWallet(null);
    setSigner(null);
    setAddress(null);
  }, [signer, wallet]);

  const submit = useCallback(
    async (messages: EncodeObject[], memo = '', gas = 200_000) => {
      if (!signer) throw new Error('no wallet is connected');
      return signer.submit(messages, memo, gas);
    },
    [signer],
  );

  const value = useMemo<WalletState>(
    () => ({ address, connecting, error, connect, disconnect, submit }),
    [address, connecting, error, connect, disconnect, submit],
  );

  return <WalletContext.Provider value={value}>{children}</WalletContext.Provider>;
}

/** The sign-in control in the masthead. */
export function WalletButton() {
  const { address, connecting, error, connect, disconnect } = useWallet();

  if (address) {
    return (
      <button type="button" className="wallet wallet--connected" onClick={disconnect} title={address}>
        <span className="wallet__dot" aria-hidden="true" />
        {address.slice(0, 9)}…{address.slice(-4)}
      </button>
    );
  }

  // No "install an extension" branch any more, and nothing to choose between.
  // There is one kind of account and either you have one or the wallet will
  // offer to make you one, which is its job rather than the explorer's.
  return (
    <span className="inline" style={{ gap: '0.4rem' }}>
      <button type="button" className="wallet" onClick={() => void connect()} disabled={connecting}>
        {connecting ? 'Waiting for the wallet…' : 'Sign in'}
      </button>
      {error ? (
        <span className="small" style={{ color: 'var(--negative)' }} role="alert">
          {error}
        </span>
      ) : null}
    </span>
  );
}

/**
 * An action that requires a signature.
 *
 * Takes what will happen as a sentence, shows it before asking, and afterwards
 * reports what the chain did rather than what the broadcast returned — the two
 * differ exactly when it matters, because a transaction the node accepts can
 * still fail when the block runs it.
 */
export function SignAction({
  label,
  consequence,
  build,
  gas = 200_000,
  disabled,
  onDone,
}: {
  label: string;
  consequence: ReactNode;
  build: (address: string) => EncodeObject[];
  gas?: number;
  disabled?: boolean;
  onDone?: () => void;
}) {
  const { address, submit } = useWallet();
  const [state, setState] = useState<'idle' | 'signing' | 'done' | 'failed'>('idle');
  const [detail, setDetail] = useState<string | null>(null);

  if (!address) {
    return (
      <p className="small faint" style={{ margin: '0.4rem 0 0' }}>
        Sign in to {label.toLowerCase()}.
      </p>
    );
  }

  async function run() {
    setState('signing');
    setDetail(null);
    try {
      const result = await submit(build(address!), '', gas);
      if (result.succeeded) {
        setState('done');
        setDetail(`Confirmed in block ${result.height}.`);
        onDone?.();
      } else {
        setState('failed');
        setDetail(result.error?.message ?? `The chain rejected it (code ${result.code}).`);
      }
    } catch (e) {
      // A refused signature is a decision, not a failure.
      const raw = e instanceof Error ? e.message : String(e);
      if (/reject|denied/i.test(raw)) {
        setState('idle');
        return;
      }
      setState('failed');
      setDetail(translateError(raw).message);
    }
  }

  return (
    <div style={{ marginTop: '0.6rem' }}>
      <p className="small muted" style={{ margin: '0 0 0.4rem' }}>
        {consequence}
      </p>
      <button
        type="button"
        className="button-primary"
        onClick={() => void run()}
        disabled={disabled || state === 'signing'}
      >
        {state === 'signing' ? 'Waiting for the chain…' : label}
      </button>
      {detail ? (
        <p
          className="small"
          style={{ margin: '0.4rem 0 0', color: state === 'failed' ? 'var(--negative)' : 'var(--positive)' }}
          role="status"
        >
          {state === 'failed' ? '✕ ' : '✓ '}
          {detail}
        </p>
      ) : null}
    </div>
  );
}
