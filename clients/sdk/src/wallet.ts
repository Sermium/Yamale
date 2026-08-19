/**
 * Connecting a browser wallet.
 *
 * Thin on purpose. Everything that decides anything lives in signing.ts, which
 * takes an injected signer; this file only finds an extension and describes the
 * chain to it. That split is what lets the signing path be tested against a
 * real chain with a key held in the test, rather than only by hand with an
 * extension installed.
 */

import type { OfflineSigner } from '@cosmjs/proto-signing';

export interface ChainInfo {
  chainId: string;
  chainName: string;
  rpcUrl: string;
  restUrl: string;
  /** Base denom, e.g. uyml. */
  baseDenom: string;
  /** Display denom, e.g. YML. */
  displayDenom: string;
  exponent: number;
  bech32Prefix: string;
  /** Base units of the fee denom per unit of gas. */
  gasPrice: number;
}

/** A wallet extension the page can see. */
export interface WalletProvider {
  id: 'keplr' | 'leap';
  label: string;
}

interface InjectedWallet {
  experimentalSuggestChain(info: unknown): Promise<void>;
  enable(chainId: string): Promise<void>;
  getOfflineSigner(chainId: string): OfflineSigner;
}

function injected(id: WalletProvider['id']): InjectedWallet | undefined {
  return (globalThis as Record<string, any>)[id];
}

/**
 * Which wallets are actually present.
 *
 * Returned rather than picked, so the interface can say "install one of these"
 * when the list is empty instead of failing at the moment somebody tries to
 * sign — the point at which they have already decided to act.
 */
export function availableWallets(): WalletProvider[] {
  const candidates: WalletProvider[] = [
    { id: 'keplr', label: 'Keplr' },
    { id: 'leap', label: 'Leap' },
  ];
  return candidates.filter((w) => injected(w.id) !== undefined);
}

/**
 * Connects a wallet and returns a signer for this chain.
 *
 * Describes the chain to the wallet first. A wallet has no idea what Yamale is
 * — it is not one of the networks shipped with the extension — so without this
 * the connection fails with an unhelpful "chain not found".
 */
export async function connect(id: WalletProvider['id'], chain: ChainInfo): Promise<OfflineSigner> {
  const wallet = injected(id);
  if (!wallet) {
    throw new Error(`${id} is not installed in this browser`);
  }

  await wallet.experimentalSuggestChain(describeChain(chain));
  await wallet.enable(chain.chainId);
  return wallet.getOfflineSigner(chain.chainId);
}

/**
 * The chain, in the shape a wallet expects.
 *
 * The currency appears three times — as the fee token, the staking token and in
 * the list — because wallets read each from a different field and silently show
 * a raw base denom for any they cannot resolve.
 */
export function describeChain(chain: ChainInfo) {
  const currency = {
    coinDenom: chain.displayDenom,
    coinMinimalDenom: chain.baseDenom,
    coinDecimals: chain.exponent,
  };

  return {
    chainId: chain.chainId,
    chainName: chain.chainName,
    rpc: chain.rpcUrl,
    rest: chain.restUrl,
    bip44: { coinType: 118 },
    bech32Config: {
      bech32PrefixAccAddr: chain.bech32Prefix,
      bech32PrefixAccPub: `${chain.bech32Prefix}pub`,
      bech32PrefixValAddr: `${chain.bech32Prefix}valoper`,
      bech32PrefixValPub: `${chain.bech32Prefix}valoperpub`,
      bech32PrefixConsAddr: `${chain.bech32Prefix}valcons`,
      bech32PrefixConsPub: `${chain.bech32Prefix}valconspub`,
    },
    currencies: [currency],
    feeCurrencies: [
      {
        ...currency,
        gasPriceStep: {
          low: chain.gasPrice,
          average: chain.gasPrice,
          high: chain.gasPrice * 2,
        },
      },
    ],
    stakeCurrency: currency,
  };
}
