/**
 * Where the wallet is, and who may ask it to sign, worked out from the address bar.
 *
 * Two deployments serve the same build. The old hosts (pay.yamalelegal.com, the
 * Tailscale funnel) put every app under ONE origin, /wallet/ beside /explorer/.
 * yamaletech.com gives every app its own subdomain, and the wallet one of its
 * own: wallet.yamaletech.com. That separation is what @yamale/connect was
 * designed for, and what the apps' own comments asked for: on one origin, a
 * script injected into any console can read the wallet's storage, and the
 * postMessage checks run without carrying the weight they were built to carry.
 *
 * It is decided at run time rather than at build time so one build serves both,
 * and so the old hosts keep their same-origin wallet. That matters: an account
 * lives in the browser's storage for the wallet's origin, so pointing an old
 * host at the new wallet would show its owner an empty one.
 */

export const DOMAIN = 'yamaletech.com';

/** Every app's subdomain on yamaletech.com. The wallet answers these and no others. */
export const APP_HOSTS: readonly string[] = [
  'pay', 'explorer', 'safe', 'rwa', 'herd', 'ladoum', 'land', 'markets', 'syndicate',
  'institution', 'governance', 'foundation', 'validator', 'oversight', 'keys', 'demo', 'chain',
].map((sub) => `${sub}.${DOMAIN}`);

const WALLET_HOST = `wallet.${DOMAIN}`;

/** Is this hostname yamaletech.com or one of its subdomains, on a dot boundary. */
function onOurDomain(hostname: string): boolean {
  return hostname === DOMAIN || hostname.endsWith(`.${DOMAIN}`);
}

/** The wallet an app at `origin` should open. */
export function walletUrlFor(origin: string): string {
  const here = new URL(origin);
  if (here.protocol === 'https:' && onOurDomain(here.hostname)) {
    return `https://${WALLET_HOST}/wallet`;
  }
  return new URL('/wallet', here.origin).toString().replace(/\/$/, '');
}

/**
 * The origins a wallet at `walletOrigin` serves. Never '*': development builds
 * pass '*' themselves, because their dev servers sit on arbitrary ports.
 */
export function appOriginsFor(walletOrigin: string): string[] {
  const here = new URL(walletOrigin);
  if (here.protocol === 'https:' && here.hostname === WALLET_HOST) {
    return APP_HOSTS.map((host) => `https://${host}`);
  }
  return [here.origin];
}
