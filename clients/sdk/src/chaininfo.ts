/**
 * How this chain is described to anything that needs to know.
 *
 * This file is what is left of wallet.ts, which found Keplr or Leap in the page
 * and handed back their signer. The extensions are gone: every application now
 * signs through the Yamale wallet over `@yamale/connect`, whose `YamaleWallet`
 * is an `OfflineDirectSigner` and drops into `ChainSigner` exactly as an
 * extension's signer did.
 *
 * # Why they were removed rather than kept alongside
 *
 * An account held in an extension cannot be enrolled with the custodian and
 * recovery services, so it cannot be recovered if the device is lost — and in
 * every screen it looks identical to an account that can. Offering both would
 * mean no interface in this repository could truthfully answer "what happens if
 * I lose this device?" about the account in front of it.
 *
 * The description below survives because a chain still has to be described —
 * to a signer, to a page, to a person reading a footer — and because the fields
 * are the same ones any Cosmos tool asks for.
 */

/** The chain, as anything connecting to it needs it described. */
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

/**
 * The chain in the shape the wider Cosmos ecosystem expects.
 *
 * The currency appears three times — as the fee token, the staking token and in
 * the list — because tools read each from a different field and silently show a
 * raw base denom for any they cannot resolve. That was written for a browser
 * extension and remains true of every other consumer of this shape, which is
 * why the function outlived the extensions.
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
