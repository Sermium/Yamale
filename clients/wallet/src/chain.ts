/** The wallet's connection to the chain. Read-only: this app never signs. */

import { ChainClient } from '@yamale/chain';

export const client = new ChainClient({
  restUrl: import.meta.env.VITE_REST_URL ?? '/api/rest',
  rpcUrl: import.meta.env.VITE_RPC_URL ?? '/api/rpc',
});
