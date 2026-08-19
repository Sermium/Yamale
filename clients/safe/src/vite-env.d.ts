/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Public REST endpoint. Defaults to the dev-server proxy at /api/rest. */
  readonly VITE_REST_URL?: string;
  /** Public CometBFT RPC endpoint. Defaults to the dev-server proxy at /api/rpc. */
  readonly VITE_RPC_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
