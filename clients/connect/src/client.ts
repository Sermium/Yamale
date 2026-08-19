import type { AccountData, DirectSignResponse, OfflineDirectSigner } from '@cosmjs/proto-signing';

import {
  envelope,
  fromBase64,
  isEnvelope,
  toBase64,
  type AccountInfo,
  type AnyRequest,
  type AnyResult,
  PROTOCOL_VERSION,
} from './protocol.ts';

export interface ConnectOptions {
  /** Origin of the wallet, e.g. `http://10.0.0.188:8092`. */
  walletUrl: string;
  chainId: string;
  appName: string;
  /** How long to wait for the user. Signing needs a person to read and decide,
   *  so this is generous; connecting is the same flow and shares it. */
  timeoutMs?: number;
}

export class WalletError extends Error {
  constructor(
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = 'WalletError';
  }

  /** True when the user simply declined — not something to report as a fault. */
  get rejected() {
    return this.code === 'rejected';
  }
}

/**
 * An application's handle on the Yamale wallet.
 *
 * The important property of this class is what it implements:
 * `OfflineDirectSigner`. That is the interface CosmJS signs with, and the one
 * `ChainSigner` in the SDK already accepts — so every existing signing path in
 * the explorer, the Safe and the transfer app works through the wallet with no
 * change beyond which signer is handed in. Nothing had to learn a new API.
 *
 * Each request opens a popup rather than an iframe. An iframe would be
 * smoother, but a page cannot be trusted to render an approval dialog for a
 * document it also composed — the user must see the wallet's own window, with
 * the wallet's own address bar, or "approve" means nothing.
 */
export class YamaleWallet implements OfflineDirectSigner {
  private accounts: AccountInfo[] = [];
  private readonly walletOrigin: string;

  constructor(private readonly options: ConnectOptions) {
    this.walletOrigin = new URL(options.walletUrl).origin;
  }

  get connected() {
    return this.accounts.length > 0;
  }

  /** The address currently in use, or undefined before connecting. */
  get address(): string | undefined {
    return this.accounts[0]?.address;
  }

  async connect(): Promise<AccountInfo[]> {
    const result = await this.request({
      kind: 'connect',
      chainId: this.options.chainId,
      appName: this.options.appName,
    });
    if (result.kind !== 'connect') throw new WalletError('internal', 'Unexpected reply.');
    this.accounts = result.accounts;
    return result.accounts;
  }

  disconnect() {
    this.accounts = [];
  }

  /** OfflineDirectSigner. */
  async getAccounts(): Promise<readonly AccountData[]> {
    if (!this.connected) await this.connect();
    return this.accounts.map((account) => ({
      address: account.address,
      algo: account.algo,
      pubkey: fromBase64(account.pubkey),
    }));
  }

  /** OfflineDirectSigner. */
  async signDirect(signerAddress: string, signDoc: any): Promise<DirectSignResponse> {
    const result = await this.request({
      kind: 'signDirect',
      chainId: signDoc.chainId ?? this.options.chainId,
      signerAddress,
      bodyBytes: toBase64(signDoc.bodyBytes),
      authInfoBytes: toBase64(signDoc.authInfoBytes),
      accountNumber: signDoc.accountNumber.toString(),
    });
    if (result.kind !== 'signDirect') throw new WalletError('internal', 'Unexpected reply.');

    // The wallet returns the document it actually signed. Broadcasting the one
    // we sent instead would produce a signature over different bytes, which
    // fails verification in a way that looks like a chain fault.
    return {
      signed: {
        bodyBytes: fromBase64(result.bodyBytes),
        authInfoBytes: fromBase64(result.authInfoBytes),
        chainId: signDoc.chainId,
        accountNumber: signDoc.accountNumber,
      },
      signature: {
        pub_key: { type: 'tendermint/PubKeySecp256k1', value: this.accounts[0]!.pubkey },
        signature: result.signature,
      },
    } as DirectSignResponse;
  }

  private request(payload: AnyRequest): Promise<AnyResult> {
    return new Promise((resolve, reject) => {
      const id = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
      const popup = window.open(
        `${this.options.walletUrl}/connect`,
        'yamale-wallet',
        'width=420,height=640,resizable=yes,scrollbars=yes',
      );
      if (!popup) {
        reject(
          new WalletError(
            'internal',
            'The wallet window was blocked. Allow popups for this site and try again.',
          ),
        );
        return;
      }

      let settled = false;
      const finish = (fn: () => void) => {
        if (settled) return;
        settled = true;
        window.removeEventListener('message', onMessage);
        clearInterval(closedCheck);
        clearTimeout(timer);
        fn();
      };

      const onMessage = (event: MessageEvent) => {
        // Both halves of rule 1: the sender must be the wallet's origin, and the
        // reply must be to a request we made.
        if (event.origin !== this.walletOrigin) return;
        if (!isEnvelope(event.data)) return;
        if (event.data.payload && (event.data.payload as any).kind === 'ready') {
          popup.postMessage(envelope(id, payload), this.walletOrigin);
          return;
        }
        if (event.data.id !== id) return;
        if (event.data.version !== PROTOCOL_VERSION) {
          finish(() =>
            reject(new WalletError('unsupported', 'The wallet speaks a different version.')),
          );
          return;
        }

        const result = event.data.payload as AnyResult;
        finish(() => {
          popup.close();
          if (result.kind === 'error') reject(new WalletError(result.code, result.message));
          else resolve(result);
        });
      };

      window.addEventListener('message', onMessage);

      // A closed window is a refusal. Without this the promise hangs forever and
      // the calling app sits on a spinner with nothing to cancel it.
      const closedCheck = setInterval(() => {
        if (popup.closed) {
          finish(() => reject(new WalletError('rejected', 'The wallet window was closed.')));
        }
      }, 400);

      const timer = setTimeout(
        () =>
          finish(() => {
            popup.close();
            reject(new WalletError('rejected', 'The wallet did not answer in time.'));
          }),
        this.options.timeoutMs ?? 180_000,
      );
    });
  }
}
