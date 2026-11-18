/** Where this app finds the chain, its API and the account services. */
export const CHAIN_ID: string = import.meta.env.VITE_CHAIN_ID ?? 'yamale-local-1';

/** The origin printed into links and record locations: the public one in production. */
export const PUBLIC_ORIGIN: string = (import.meta.env.VITE_PUBLIC_ORIGIN ?? window.location.origin).replace(/\/+$/, '');

export const API = '/api/traceability';

/** CosmJS needs an absolute URL for its RPC client; reads use the relative proxy. */
export const RPC_ENDPOINT = `${window.location.origin}/api/rpc`;

export const GAS_PRICE = Number(import.meta.env.VITE_GAS_PRICE ?? '0');

export const SERVICES = { custodianUrl: '/api/custodian', recoveryUrl: '/api/recovery' };

export function recordUri(hash: string): string {
  return `${PUBLIC_ORIGIN}${API}/v1/files/${hash}`;
}
