/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Public REST endpoint. Defaults to the dev-server proxy at /api/rest. */
  readonly VITE_REST_URL?: string;
  /** Public CometBFT RPC endpoint. Defaults to the dev-server proxy at /api/rpc. */
  readonly VITE_RPC_URL?: string;
  /**
   * The chain this build targets, printed into the commands the placement step
   * hands over. A command carrying the wrong --chain-id is refused at signing,
   * which reads as a broken key rather than a wrong build.
   */
  readonly VITE_CHAIN_ID?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
