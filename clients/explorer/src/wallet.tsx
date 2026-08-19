/**
 * Wallet connection, and the one place a signature is requested.
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
 */

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import {
  ChainSigner,
  availableWallets,
  connect as connectWallet,
  translateError,
  type ChainInfo,
  type EncodeObject,
  type SubmitResult,
  type WalletProvider,
} from '@yamale/chain';

/**
 * The chain, as a wallet needs it described.
 *
 * Read from the build environment so a hosted explorer points at the network it
 * was built for; the defaults are the local devnet.
 */
export const CHAIN: ChainInfo = {
  chainId: import.meta.env.VITE_CHAIN_ID ?? 'yamale-devnet-1',
  chainName: import.meta.env.VITE_CHAIN_NAME ?? 'Yamale',
  rpcUrl: import.meta.env.VITE_PUBLIC_RPC_URL ?? 'http://localhost:26657',
  restUrl: import.meta.env.VITE_PUBLIC_REST_URL ?? 'http://localhost:1317',
  baseDenom: 'uyml',
  displayDenom: 'YML',
  exponent: 6,
  bech32Prefix: 'yml',
  gasPrice: Number(import.meta.env.VITE_GAS_PRICE ?? 0),
};

interface WalletState {
  address: string | null;
  connecting: boolean;
  error: string | null;
  providers: WalletProvider[];
  connect: (id: WalletProvider['id']) => Promise<void>;
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

  // Computed once per render rather than stored: an extension can be installed
  // while the page is open, and a cached empty list would keep telling somebody
  // they have no wallet after they have just installed one.
  const providers = availableWallets();

  const connect = useCallback(async (id: WalletProvider['id']) => {
    setConnecting(true);
    setError(null);
    try {
      const offlineSigner = await connectWallet(id, CHAIN);
      const chainSigner = new ChainSigner(offlineSigner, {
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
    setSigner(null);
    setAddress(null);
  }, [signer]);

  const submit = useCallback(
    async (messages: EncodeObject[], memo = '', gas = 200_000) => {
      if (!signer) throw new Error('no wallet is connected');
      return signer.submit(messages, memo, gas);
    },
    [signer],
  );

  const value = useMemo<WalletState>(
    () => ({ address, connecting, error, providers, connect, disconnect, submit }),
    [address, connecting, error, providers, connect, disconnect, submit],
  );

  return <WalletContext.Provider value={value}>{children}</WalletContext.Provider>;
}

/** The connect control in the masthead. */
export function WalletButton() {
  const { address, connecting, error, providers, connect, disconnect } = useWallet();

  if (address) {
    return (
      <button type="button" className="wallet wallet--connected" onClick={disconnect} title={address}>
        <span className="wallet__dot" aria-hidden="true" />
        {address.slice(0, 9)}…{address.slice(-4)}
      </button>
    );
  }

  if (providers.length === 0) {
    return (
      <a
        className="wallet"
        href="https://www.keplr.app/"
        target="_blank"
        rel="noreferrer"
        title="A browser wallet is needed to sign; the explorer works without one"
      >
        Get a wallet
      </a>
    );
  }

  return (
    <span className="inline" style={{ gap: '0.4rem' }}>
      {providers.map((p) => (
        <button
          key={p.id}
          type="button"
          className="wallet"
          onClick={() => void connect(p.id)}
          disabled={connecting}
        >
          {connecting ? 'Connecting…' : `Connect ${p.label}`}
        </button>
      ))}
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
        Connect a wallet to {label.toLowerCase()}.
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
