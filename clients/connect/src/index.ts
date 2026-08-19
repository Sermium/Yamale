/**
 * `@yamale/connect` — how an application talks to the Yamale wallet.
 *
 * Applications import {@link YamaleWallet}; it implements CosmJS's
 * `OfflineDirectSigner`, so it drops straight into `ChainSigner` and every
 * signing path that already exists.
 *
 * The wallet itself imports {@link serveWalletRequests}.
 */
export { YamaleWallet, WalletError, type ConnectOptions } from './client.ts';
export { serveWalletRequests, type ServeOptions } from './provider.ts';
export {
  PROTOCOL_VERSION,
  fromBase64,
  toBase64,
  type AccountInfo,
  type AnyRequest,
  type AnyResult,
  type SignDirectRequest,
} from './protocol.ts';
