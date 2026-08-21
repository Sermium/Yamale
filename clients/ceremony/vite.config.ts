import { defineConfig } from 'vite';

// The build for `ceremony host`.
//
// Three settings here are decisions rather than defaults, and each one exists
// because of something the page has to be able to claim about itself.
//
// It builds into tools/ceremony/hosted, which is committed. The host embeds that
// directory, so one binary is everything a coordinator needs to serve the
// ceremony — no node, no npm, no dist directory to forget to copy. It also means
// `go build ./...` works on a fresh clone without a JavaScript toolchain, which
// a gitignored dist directory would break.
//
// It is not minified. The page tells the custodian they are trusting its code
// and shows the hash of the bundle they can compare against a published value.
// Both of those invite somebody to actually read it, and a single line of
// mangled identifiers is an invitation nobody can accept.
//
// Everything is one file with relative URLs. The host serves under a path
// prefix (/ceremony/ behind the existing certificate on pay.yamalelegal.com),
// so absolute asset URLs would 404 the moment it sat anywhere but the root.
export default defineConfig({
  base: './',
  build: {
    outDir: '../../tools/ceremony/hosted',
    emptyOutDir: true,
    target: 'es2022',
    assetsDir: '.',
    minify: false,
    cssCodeSplit: false,
    modulePreload: false,
    // A single deterministic filename, because the coordinator publishes the
    // bundle's SHA-256 and a custodian compares it: a content hash in the name
    // would make every rebuild a different URL and the published value would
    // have to name the file as well as the digest.
    rollupOptions: {
      output: {
        entryFileNames: 'ceremony.js',
        chunkFileNames: 'ceremony.js',
        assetFileNames: '[name][extname]',
        inlineDynamicImports: true,
      },
    },
  },
});
