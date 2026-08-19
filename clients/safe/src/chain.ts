/** The safe's connection to the chain. */

import { ChainClient } from '@yamale/chain';

export const client = new ChainClient({
  restUrl: import.meta.env.VITE_REST_URL ?? '/api/rest',
  rpcUrl: import.meta.env.VITE_RPC_URL ?? '/api/rpc',
});

export const CHAIN_ID = import.meta.env.VITE_CHAIN_ID ?? 'yamale-testnet-1';
