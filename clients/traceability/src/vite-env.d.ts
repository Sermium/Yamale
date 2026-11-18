/// <reference types="vite/client" />
interface ImportMetaEnv {
  readonly VITE_CHAIN_ID?: string;
  readonly VITE_PUBLIC_ORIGIN?: string;
  readonly VITE_GAS_PRICE?: string;
}
interface ImportMeta { readonly env: ImportMetaEnv }
