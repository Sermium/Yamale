import {
  envelope,
  isEnvelope,
  PROTOCOL_VERSION,
  type AnyRequest,
  type AnyResult,
} from './protocol.ts';

/**
 * The wallet's half of the protocol.
 *
 * This runs inside the wallet page and is the only code in the system that ever
 * touches a private key. It is written as a plain listener with a callback
 * rather than as a React hook so that the security-relevant part — who is
 * allowed to ask, and what gets shown before a signature — is readable on its
 * own, without a component tree around it.
 *
 * The opener's origin is captured once, from the browser's own `event.origin`
 * on the first message, and every later message is checked against it. That
 * value cannot be forged by the calling page: it is stamped by the browser.
 * An application that claims to be someone else in its `appName` therefore
 * changes only a label, never a permission.
 */
export interface ServeOptions {
  /** Origins permitted to connect. `'*'` accepts any, which is acceptable on a
   *  devnet where every app is yours and nothing is worth stealing — and is the
   *  wrong default for a network with value on it. */
  allowedOrigins: string[] | '*';
  /**
   * Asked to decide a request. Resolve with the result to approve, or reject
   * to refuse. This is where the wallet renders what is about to be signed —
   * the decision belongs to a person, so this function must not resolve on its
   * own.
   */
  onRequest: (request: AnyRequest, origin: string) => Promise<AnyResult>;
}

export function serveWalletRequests(options: ServeOptions): () => void {
  let openerOrigin: string | null = null;

  const allowed = (origin: string) =>
    options.allowedOrigins === '*' || options.allowedOrigins.includes(origin);

  const onMessage = async (event: MessageEvent) => {
    if (!isEnvelope(event.data)) return;

    // First contact fixes the origin for the life of this window. Re-deriving it
    // per message would let a second, different opener slip a request in.
    if (openerOrigin === null) {
      if (!allowed(event.origin)) return;
      openerOrigin = event.origin;
    }
    if (event.origin !== openerOrigin) return;

    const { id, version } = event.data;
    const reply = (payload: AnyResult) =>
      (event.source as Window | null)?.postMessage(envelope(id, payload), openerOrigin!);

    if (version !== PROTOCOL_VERSION) {
      reply({
        kind: 'error',
        code: 'unsupported',
        message: `This wallet speaks protocol ${PROTOCOL_VERSION}.`,
      });
      return;
    }

    try {
      reply(await options.onRequest(event.data.payload as AnyRequest, event.origin));
    } catch (err) {
      // A thrown error is a refusal, not a crash: the caller is waiting on a
      // promise and silence would hang it until the timeout.
      reply({
        kind: 'error',
        code: err instanceof Error && err.message === 'rejected' ? 'rejected' : 'internal',
        message: err instanceof Error ? err.message : 'The wallet could not complete the request.',
      });
    }
  };

  window.addEventListener('message', onMessage);

  // Announce readiness to whoever opened this window. The client holds its
  // request until it sees this, because a popup that has not finished loading
  // has no listener attached and the message would be lost with no error.
  if (window.opener) {
    window.opener.postMessage(envelope('ready', { kind: 'ready' }), '*');
  }

  return () => window.removeEventListener('message', onMessage);
}
