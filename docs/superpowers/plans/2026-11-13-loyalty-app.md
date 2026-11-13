# Loyalty App and loyalty-api Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A phone-first web app for loyalty.yamaletech.com, run against the local devnet. In it:
- anyone can verify a jewel;
- owners can hold and pass on jewels without ever seeing a blockchain;
- jewelers' staff can register, sell and service jewels.

Alongside it, `loyalty-api`, a small Go service that stores photos and records, sponsors owners' fees, keeps jeweler profiles, and takes jeweler applications.

**Architecture:**
- **The app** is a Vite + React workspace, `clients/loyalty`, built the way `clients/herd` is.
  - It reads the chain over ABCI and `tx_search`, through a transport promoted into `@yamale/chain`.
  - It signs with `ChainSigner`.
  - Each person's key lives on their own device, sealed under their password by the wallet's own seal, and is enrolled with the account service when that service is reachable.
- **loyalty-api** is `tools/loyaltyapi`. It stores files by content, keeps jeweler profiles, grants fees through the chain CLI the way `tools/faucet` does, and keeps an application spool.
  - Every request that changes something is signed by an account key.
  - The signing format is shared between Go and TypeScript and pinned by test vectors, as are the claim and voucher signatures.

**Tech Stack:**
- TypeScript 5.7, React 19, react-router-dom 7, Vite 6.
- Node 24's test runner with `--experimental-strip-types`.
- CosmJS 0.32, @noble/curves 1.9, @noble/hashes 1.8.
- `qrcode-generator` 2.0.4 (already in the workspace), and the browser's `BarcodeDetector` (no bundled decoder, as in `clients/app`).
- Go 1.25 `net/http`.
- ts-proto 1.181.2 through `buf`.

**Spec:** `docs/superpowers/specs/2026-11-12-jewel-loyalty-design.md`.
- This plan covers delivery step 3, run locally.
- Plan 1 (`x/jewel`) is merged on `main` at 52b170b.
- Plan 3 (the devnet-2 upgrade) is separate. It now also carries the deployment on yamale-us, because the chain there has no `x/jewel` until the upgrade.

## Where this plan departs from the spec, and why

Five changes and one addition. The user should confirm number 1 before execution starts.

1. **A claim link never passes through loyalty-api.** The spec has loyalty-api email the claim link.
   - The problem: the link carries the claim key's private half. A mail relay run by the platform would therefore hold the key to every unclaimed jewel, and the platform could claim any of them itself. That breaks the spec's own promise, in section 6, that "the platform cannot quietly take a jewel".
   - What replaces it: the link leaves the seller's device directly. At the counter it is a QR code the customer scans; for a resale, the phone's share sheet (WhatsApp, SMS, mail) or a `mailto:` link. The key travels only in the URL fragment, which browsers never send to any server, ours included.
   - What is lost: the app safeguard of asking the claimer to sign in with the email the offer went to. The offer has no email.
2. **Plan 2 sends no email at all.**
   - An offer to an account is found by its recipient in the app (number 3).
   - Applications are written to a spool directory, as `tools/notify` does.
   - Mail can come back later for receipts, but nothing here needs it.
3. **There is no event indexer.**
   - Chain state answers every read except one: "which offers are addressed to me". `tx_search` on `EventOfferMade.to` answers that one.
   - `tools/rpcgate` already allows `tx_search` through on devnet-2.
4. **A new phone does not come back with email and password alone.**
   - The deployed account service releases holdings only after an operator-run recovery: two approvers, 72 hours' notice, then a 24-hour hold (`tools/custodian/claim.go`).
   - So the app offers two things: unlock on this device, and claim after a completed recovery. It says that plainly, rather than promise a sign-in that does not exist.
5. **Hosting on yamale-us moves to Plan 3.** Nothing here is deployed or pushed.
6. **Addition: five languages, not two.** The spec asks for English and French. The shared catalogue's test fails unless every key exists in `en`, `fr`, `pt`, `sw` and `ar`, so all five are carried.

## Global Constraints

**Process**
- Commit messages carry **no Co-Authored-By trailer**.
- Commit to the working branch only. No push, no deploy, no change on any remote host.

**What the screens show**
- Screens never show addresses, hashes, fees or gas.
- The raw values sit behind an element labelled **Advanced** (`<details>`).
- The one exception is a person's own account code, shown on request as a QR code under "Show my account code".
- A public page never says who owned a jewel. It shows the number of owners and the date of each change, nothing more.
- This sentence appears on every jewel page, verbatim, in the active language:
  - en: "Changes of owner are declared by the owners; the inscription was checked by the jeweler at registration only."
  - fr: "Les changements de propriétaire sont déclarés par les propriétaires ; l'inscription n'a été vérifiée par le joaillier qu'à l'enregistrement."

**Keys and signing**
- loyalty-api never signs for a person. Its only key grants fee allowances, limited to the owner messages listed in Task 5.
- A claim key never reaches a server. It travels only in a URL fragment (`/claim#<jewel id>.<64 hex>`).
- Sign-byte formats are exact strings, and Go and TypeScript must agree byte for byte:
  - claim: `yamale-jewel-claim/v1:<chain id>:<jewel id>:<claimer>`
  - voucher: `yamale-jewel-voucher/v1:<chain id>:<benefit id>:<jewel id>:<nonce>:<expires at>`
  - ownership proof: `yamale-jewel-owner/v1:<chain id>:<jewel id>:<issued at>`
  - loyalty-api request: `yamale-loyalty/v1:<purpose>:<chain id>:<subject>:<issued at>`
  - Signatures are Cosmos secp256k1: SHA-256 of the bytes, low-S, 64 bytes `r‖s`, with a 33-byte compressed public key.
- A voucher's expiry is computed from the **chain's** latest block time, never the phone's clock, at 240 s. The chain allows at most 300 s past the redeeming block.

**Data**
- The generated TypeScript types carry every `uint64` and `int64` as a **string**. That includes jewel, jeweler and benefit ids and unix times.
- x/nft class id `jewel-<jeweler id>`, token id `jewel-<jewel id>`.
- `record_hash` is the lowercase hex SHA-256 of the record file's exact bytes.
- `record_uri` is `<public origin>/api/loyalty/v1/files/<record_hash>`.
- The app always fetches by hash and checks the hash before showing anything.

**Translations**
- Every key added to `en` in `clients/sdk/src/catalogues.ts` is added to `fr`, `pt`, `sw` and `ar` in the same commit. Each step gives `en` and `fr` verbatim.
- The implementer writes `pt`, `sw` and `ar`, following the register of the neighbouring keys. Arabic uses the same Western digits as the other catalogues.

**Code and tests**
- Code a Node test loads must import SDK files by relative path, e.g. `../../sdk/src/jewel.ts`, never `@yamale/chain`. Its index re-exports `.tsx`, which Node's type stripping cannot load. Browser code may import `@yamale/chain`.
- Account code is imported from `clients/wallet/src/account/` by relative path, not copied. Promoting it into the SDK is a later change.

**Design**
- Use the shared mobile shell in `clients/shared/yamale.css`: `.y-stage`, `.y-phone`, `.y-appbar`, `.y-screen`, `.y-tabbar`, `.y-cards`, `.y-card`, `.y-row`, `.y-chip`.
- Before a UI task is done, check it in both themes at 390 px and at 1600 px wide, against a populated chain.

**Windows**
- Every shell needs Go on its PATH: `export PATH="/c/Program Files/Go/bin:$PATH"`.
- `make` is not installed. Run the commands this plan quotes.
- Application control sometimes refuses Go-built binaries with `fork/exec … stratégie de contrôle`. That is policy, not a failing test: rerun, or build into the session scratchpad and run from there.
- Line endings: generated TypeScript and `*.sh` are pinned to LF by `.gitattributes`. For any other file, match its neighbours.

## Running it locally (used by Tasks 6–11)

Run these from the repo root in Git Bash. `$SCRATCH` is the session scratchpad.

```bash
export PATH="/c/Program Files/Go/bin:$PATH"
go build -o "$SCRATCH/blockchaind.exe" ./cmd/blockchaind
go build -o "$SCRATCH/jewelsign.exe" ./tools/jewelsign
go build -o "$SCRATCH/loyaltyapi.exe" ./tools/loyaltyapi          # from Task 4 on
BIN="$SCRATCH/blockchaind.exe" scripts/devnet/init-local-devnet.sh  # chain yamale-local-1, RPC 127.0.0.1:26957, REST 127.0.0.1:1517
BIN="$SCRATCH/blockchaind.exe" SIGN="$SCRATCH/jewelsign.exe" HOME_DIR=.devnet/local/node \
  CHAIN=yamale-local-1 NODE=tcp://127.0.0.1:26957 scripts/devnet/seed-jewel.sh
```

- loyalty-api listens on `127.0.0.1:8095`. From Task 5 on, start it with `BIN="$SCRATCH/blockchaind.exe" API_BIN="$SCRATCH/loyaltyapi.exe" scripts/devnet/loyalty-local.sh`, in the background.
- `scripts/devnet/loyalty-admit.sh "<name>" <account code>` (Task 5) admits a jeweler created in the app.
- The app's dev server runs with `npm run dev --workspace @yamale/loyalty` on `http://localhost:5390`. Start it with `preview_start` once Task 6 adds the `.claude/launch.json` entry.
- `localhost:5390`, `127.0.0.1:5390` and `[::1]:5390` are three browser origins, so they hold three separate device accounts. Tasks 8–11 use them as three people.

---

## File structure

```
Makefile                                   proto-ts also generates cosmos/nft/v1beta1 (Task 1)
clients/sdk/src/generated/**               regenerated: + blockchain/jewel, + cosmos/nft (Task 1)
clients/sdk/src/registry.ts                + every /blockchain.jewel.v1 message (Task 1)
clients/sdk/src/signing.ts                 + SignerOptions.feeGranter (Task 1)
clients/sdk/src/jewel.ts                   inscriptions, sign bytes, claim links, vouchers,
                                           ownership proofs, message constructors (Task 2)
x/jewel/types/vectors_test.go              writes x/jewel/types/testdata/vectors.json (Task 2)
clients/sdk/src/abci.ts                    ABCI, chain head and tx_search, base URL settable (Task 3)
clients/sdk/src/jewelreads.ts              every read the app makes (Task 3)
clients/sdk/src/loyaltyauth.ts             signed headers for loyalty-api (Task 4)
tools/loyaltyapi/main.go                   flags and routes (Task 4, extended in Task 5)
tools/loyaltyapi/auth.go                   signed requests (Task 4)
tools/loyaltyapi/chain.go                  jewelers from the chain's REST (Task 4)
tools/loyaltyapi/files.go                  content-addressed files (Task 4)
tools/loyaltyapi/profiles.go               jeweler logo and colours (Task 4)
tools/loyaltyapi/sponsor.go                fee grants through the chain CLI (Task 5)
tools/loyaltyapi/applications.go           jeweler applications (Task 5)
tools/loyaltyapi/*_test.go, testdata/vectors.json
clients/loyalty/                           the app (Tasks 6–10)
  package.json tsconfig.json vite.config.ts index.html
  public/manifest.webmanifest public/mark.png public/icon-192.png public/icon-512.png
  src/main.tsx App.tsx config.ts ui.tsx api.ts styles.css vite-env.d.ts
  src/verify/   record.ts history.ts Search.tsx JewelPage.tsx Photos.tsx Proof.tsx   (Task 6)
  src/account/  device.ts recover.ts session.tsx Account.tsx                        (Task 7)
  src/owner/    links.ts MyJewels.tsx Owned.tsx OfferPanel.tsx VoucherPanel.tsx Claim.tsx  (Task 8)
  src/verify/   JewelCard.tsx                                                         (Task 8)
  src/actions/  errors.ts useAction.tsx                                             (Task 8)
  src/atelier/  role.ts photo.ts RecordForm.tsx Atelier.tsx AtelierJewel.tsx Register.tsx Amend.tsx Staff.tsx  (Task 9)
  src/atelier/  Benefits.tsx Redeem.tsx Profile.tsx Apply.tsx                        (Task 10)
clients/loyalty/scripts/smoke.ts           the chain path end to end, from TypeScript (Task 11)
scripts/devnet/jewel-lib.sh                helpers shared by the jewel scripts (Task 5)
scripts/devnet/loyalty-admit.sh            admit a jeweler created in the app (Task 5)
scripts/devnet/loyalty-local.sh            run loyalty-api against the local devnet (Task 5)
```

---

### Task 1: TypeScript types for x/jewel and x/nft, the registry, and a fee granter

**Files:**
- Modify: `Makefile` (target `proto-ts`)
- Regenerate: `clients/sdk/src/generated/**`
- Modify: `clients/sdk/src/registry.ts`
- Modify: `clients/sdk/src/signing.ts` (`SignerOptions`, `fee()`)
- Test: `clients/sdk/src/registry.test.ts`, `clients/sdk/src/signing.test.ts`

**Interfaces:**
- Produces:
  - Generated modules:
    - `clients/sdk/src/generated/blockchain/jewel/v1/{tx,query,jewel,params,events,genesis}.ts`
    - `clients/sdk/src/generated/cosmos/nft/v1beta1/{query,nft,tx}.ts`
  - Every `/blockchain.jewel.v1.Msg*` type URL, registered in `chainRegistry()`.
  - `SignerOptions.feeGranter?: string`, which `ChainSigner` puts into every fee it builds.

- [ ] **Step 1: Teach `proto-ts` about x/nft**

In `Makefile`, in the `proto-ts` target:
- Add the paragraph below to the comment block above the first `buf generate` line.
- Add `--path cosmos/nft/v1beta1` to that first `buf generate` line.

```make
	@# cosmos/nft too: the jewels a person holds are x/nft's to answer
	@# (Query/NFTs by owner). x/jewel keeps no by-owner index, because
	@# ownership lives in x/nft and a second copy would be a second truth.
	@go tool github.com/bufbuild/buf/cmd/buf generate buf.build/cosmos/cosmos-sdk:65fa41963e6a41dd95a35934239029df --template proto/buf.gen.ts.yaml --path cosmos/feegrant/v1beta1 --path cosmos/bank/v1beta1 --path cosmos/nft/v1beta1
```

- [ ] **Step 2: Regenerate**

There is no `make`, so run the target's body:

```bash
cd /d/Sermium/Yamale/Blockchain
export PATH="/c/Program Files/Go/bin:$PATH"
rm -rf clients/sdk/src/generated
go tool github.com/bufbuild/buf/cmd/buf generate buf.build/cosmos/cosmos-sdk:65fa41963e6a41dd95a35934239029df --template proto/buf.gen.ts.yaml --path cosmos/feegrant/v1beta1 --path cosmos/bank/v1beta1 --path cosmos/nft/v1beta1
go tool github.com/bufbuild/buf/cmd/buf generate --template proto/buf.gen.ts.yaml
find clients/sdk/src/generated -name '*.ts' -exec sed -i 's#protobufjs/minimal\.ts#protobufjs/minimal.js#' {} +
git status --short clients/sdk/src/generated | grep -v '^??' || echo "no tracked file changed"
ls clients/sdk/src/generated/blockchain/jewel/v1 clients/sdk/src/generated/cosmos/nft/v1beta1
```

Expected:
- `no tracked file changed`.
- Both listings show `tx.ts` and `query.ts`.
- If a tracked generated file changed, stop and report: the tree had drifted before this task, and folding that drift into this commit would hide it.

- [ ] **Step 3: Write the failing registry test**

Append to `clients/sdk/src/registry.test.ts`:

```ts
const JEWEL_MESSAGES = [
  'MsgUpdateParams', 'MsgAdmitJeweler', 'MsgSuspendJeweler', 'MsgReinstateJeweler', 'MsgSetStaff',
  'MsgRegisterJewel', 'MsgAmendJewel', 'MsgOfferJewel', 'MsgCancelOffer', 'MsgAcceptOffer',
  'MsgClaimOffer', 'MsgReportStolen', 'MsgClearStolen', 'MsgAttachBenefit', 'MsgRedeemBenefit',
  'MsgProposeReassignment', 'MsgVetoReassignment', 'MsgCompleteReassignment',
];

test('every x/jewel message is registered', () => {
  const urls = new Set(CHAIN_MESSAGE_TYPES.map(([url]) => url));
  const missing = JEWEL_MESSAGES.filter((m) => !urls.has(`/blockchain.jewel.v1.${m}`));
  assert.deepEqual(missing, []);
});

test('a jewel claim keeps its key and signature bytes through encoding', () => {
  const decoded = roundTrip({
    typeUrl: '/blockchain.jewel.v1.MsgClaimOffer',
    value: {
      claimer: 'yml1claimer',
      jewelId: '7',
      claimPubKey: new Uint8Array(33).fill(2),
      claimSignature: new Uint8Array(64).fill(9),
    },
  });
  assert.equal(decoded.claimer, 'yml1claimer');
  assert.equal(decoded.jewelId, '7');
  assert.equal(decoded.claimPubKey.length, 33);
  assert.equal(decoded.claimSignature.length, 64);
  assert.equal(decoded.claimSignature[63], 9);
});

test('a benefit redemption keeps the nested voucher', () => {
  const decoded = roundTrip({
    typeUrl: '/blockchain.jewel.v1.MsgRedeemBenefit',
    value: {
      staff: 'yml1staff',
      voucher: { benefitId: '3', jewelId: '7', nonce: 'ab12', expiresAt: '1790000000' },
      ownerPubKey: new Uint8Array(33).fill(3),
      ownerSignature: new Uint8Array(64).fill(4),
    },
  });
  assert.equal(decoded.voucher.benefitId, '3');
  assert.equal(decoded.voucher.jewelId, '7');
  assert.equal(decoded.voucher.nonce, 'ab12');
  assert.equal(decoded.voucher.expiresAt, '1790000000');
});
```

- [ ] **Step 4: Run it and watch it fail**

Run: `cd clients && npm test --workspace @yamale/chain -- --test-name-pattern="jewel"`

Expected: FAIL. `every x/jewel message is registered` lists all 18 names, and the round trips throw `Unregistered type url: /blockchain.jewel.v1.MsgClaimOffer`. If an existing test in this file walks the generated tree and requires every generated `Msg` to be registered, it fails too, naming the jewel messages. That is the same red.

- [ ] **Step 5: Register the messages**

In `clients/sdk/src/registry.ts`, add the import beside the others:

```ts
import * as jewel from './generated/blockchain/jewel/v1/tx.ts';
```

Then append to `CHAIN_MESSAGE_TYPES`, after the last existing group:

```ts
  // Jewels. The office's messages and UpdateParams are registered so an
  // office proposal can be built from a browser; registering grants nobody
  // anything, the module checks who signed.
  ['/blockchain.jewel.v1.MsgUpdateParams', jewel.MsgUpdateParams],
  ['/blockchain.jewel.v1.MsgAdmitJeweler', jewel.MsgAdmitJeweler],
  ['/blockchain.jewel.v1.MsgSuspendJeweler', jewel.MsgSuspendJeweler],
  ['/blockchain.jewel.v1.MsgReinstateJeweler', jewel.MsgReinstateJeweler],
  ['/blockchain.jewel.v1.MsgSetStaff', jewel.MsgSetStaff],
  ['/blockchain.jewel.v1.MsgRegisterJewel', jewel.MsgRegisterJewel],
  ['/blockchain.jewel.v1.MsgAmendJewel', jewel.MsgAmendJewel],
  ['/blockchain.jewel.v1.MsgOfferJewel', jewel.MsgOfferJewel],
  ['/blockchain.jewel.v1.MsgCancelOffer', jewel.MsgCancelOffer],
  ['/blockchain.jewel.v1.MsgAcceptOffer', jewel.MsgAcceptOffer],
  ['/blockchain.jewel.v1.MsgClaimOffer', jewel.MsgClaimOffer],
  ['/blockchain.jewel.v1.MsgReportStolen', jewel.MsgReportStolen],
  ['/blockchain.jewel.v1.MsgClearStolen', jewel.MsgClearStolen],
  ['/blockchain.jewel.v1.MsgAttachBenefit', jewel.MsgAttachBenefit],
  ['/blockchain.jewel.v1.MsgRedeemBenefit', jewel.MsgRedeemBenefit],
  ['/blockchain.jewel.v1.MsgProposeReassignment', jewel.MsgProposeReassignment],
  ['/blockchain.jewel.v1.MsgVetoReassignment', jewel.MsgVetoReassignment],
  ['/blockchain.jewel.v1.MsgCompleteReassignment', jewel.MsgCompleteReassignment],
```

- [ ] **Step 6: Run the registry tests**

Run: `cd clients && npm test --workspace @yamale/chain -- --test-name-pattern="jewel"`
Expected: PASS, 3 tests.

- [ ] **Step 7: Write the failing fee-granter test**

Append to `clients/sdk/src/signing.test.ts`:

```ts
import { ChainSigner } from './signing.ts';
import type { OfflineSigner } from '@cosmjs/proto-signing';

const NO_SIGNER = { getAccounts: async () => [] } as unknown as OfflineSigner;
type WithFee = { fee(gas: number): { amount: unknown; gas: string; granter?: string } };

test('a signer with a fee granter names it in every fee', () => {
  const s = new ChainSigner(NO_SIGNER, { rpcUrl: 'http://x', chainId: 'c', feeGranter: 'yml1sponsor' });
  assert.equal((s as unknown as WithFee).fee(200_000).granter, 'yml1sponsor');
});

test('a signer without one leaves the granter out', () => {
  const s = new ChainSigner(NO_SIGNER, { rpcUrl: 'http://x', chainId: 'c' });
  assert.equal('granter' in (s as unknown as WithFee).fee(200_000), false);
});
```

- [ ] **Step 8: Run it and watch it fail**

Run: `cd clients && npm test --workspace @yamale/chain -- --test-name-pattern="granter"`
Expected: FAIL. `undefined !== 'yml1sponsor'`, and a TypeScript complaint about the unknown option, which type stripping does not enforce.

- [ ] **Step 9: Add the option**

In `clients/sdk/src/signing.ts`, add this to `SignerOptions`, after `gasPrice`:

```ts
  /**
   * The account whose fee allowance pays for this signer's transactions.
   * Owners on the jewel register hold no coins: the platform grants each one
   * an allowance through x/feegrant, and a transaction that does not name the
   * granter is charged to the signer, who cannot pay.
   */
  feeGranter?: string;
```

Then replace the body of `fee()`:

```ts
  private fee(gas: number): StdFee {
    const denom = this.options.feeDenom ?? 'uyml';
    const price = this.options.gasPrice ?? 0;
    // Rounded up: a fee a fraction below the node's minimum is rejected
    // outright, and the difference is a rounding error in cost terms.
    const amount = Math.ceil(gas * price).toString();
    const fee: StdFee = { amount: [{ denom, amount }], gas: gas.toString() };
    if (this.options.feeGranter) fee.granter = this.options.feeGranter;
    return fee;
  }
```

- [ ] **Step 10: Run the whole SDK suite and the typecheck**

Run: `cd clients && npm test --workspace @yamale/chain && npm run typecheck --workspace @yamale/chain`
Expected: every test passes; the typecheck prints nothing.

- [ ] **Step 11: Commit**

```bash
git add Makefile clients/sdk/src/generated clients/sdk/src/registry.ts clients/sdk/src/registry.test.ts clients/sdk/src/signing.ts clients/sdk/src/signing.test.ts
git commit -m "sdk: x/jewel and x/nft types, jewel messages registered, fee granter on ChainSigner"
```

---

### Task 2: Jewel signing in TypeScript, pinned to the chain's Go by shared vectors

**Files:**
- Create: `x/jewel/types/vectors_test.go`, `x/jewel/types/testdata/vectors.json` (generated by the test)
- Create: `clients/sdk/src/jewel.ts`, `clients/sdk/src/jewel.test.ts`
- Modify: `clients/sdk/src/index.ts` (exports), `clients/sdk/package.json` (`@cosmjs/encoding` dependency)

**Interfaces:**
- Consumes: Task 1's generated types and registry. Also `x/jewel/types` from Go: `NormaliseInscription`, `ClaimSignBytes`, `VoucherSignBytes`, `Voucher`.
- Produces, from `clients/sdk/src/jewel.ts`. Every id and time is a decimal string.

  ```ts
  export interface VoucherFields { benefitId: string; jewelId: string; nonce: string; expiresAt: string }
  export interface VoucherCode { voucher: VoucherFields; pubKey: Uint8Array; signature: Uint8Array }

  export function normaliseInscription(raw: string): string | null
  export function claimSignBytes(chainId: string, jewelId: string, claimer: string): Uint8Array
  export function voucherSignBytes(chainId: string, v: VoucherFields): Uint8Array
  export function ownershipProofBytes(chainId: string, jewelId: string, issuedAt: string): Uint8Array
  export function signArbitrary(privateKey: Uint8Array, message: Uint8Array): Uint8Array   // 64 bytes
  export function verifyArbitrary(pubKey: Uint8Array, message: Uint8Array, signature: Uint8Array): boolean
  export function compressedPublicKey(privateKey: Uint8Array): Uint8Array                    // 33 bytes
  export function addressOfPublicKey(pubKey: Uint8Array, prefix?: string): string
  export function toHex(b: Uint8Array): string
  export function fromHex(h: string): Uint8Array
  export function toBase64(b: Uint8Array): string
  export function toBase64Url(b: Uint8Array): string
  export function fromBase64Url(s: string): Uint8Array
  export function newClaimKey(): { privateKey: Uint8Array; claimKey: string }
  export function claimLink(origin: string, jewelId: string, privateKey: Uint8Array): string
  export function parseClaimFragment(fragment: string): { jewelId: string; privateKey: Uint8Array } | null
  export function makeVoucher(privateKey: Uint8Array, chainId: string, benefitId: string, jewelId: string,
                              chainNow: number, ttlSeconds?: number,
                              random?: (n: number) => Uint8Array): VoucherCode
  export function encodeVoucherCode(code: VoucherCode): string
  export function decodeVoucherCode(text: string): VoucherCode | null
  export function voucherSigner(code: VoucherCode, chainId: string): string | null           // address, or null if the signature fails
  export function makeOwnershipProof(privateKey: Uint8Array, chainId: string, jewelId: string, issuedAt: number): string
  export function verifyOwnershipProof(chainId: string, jewelId: string, query: string): { address: string; issuedAt: number } | null

  export const STAFF_MESSAGES: readonly string[]
  export const JEWEL_GAS: number
  export function registerJewel(staff: string, inscription: string, recordHash: string, recordUri: string): EncodeObject
  export function amendJewel(staff: string, jewelId: string, recordHash: string, recordUri: string, note: string): EncodeObject
  export function offerJewelToAccount(owner: string, jewelId: string, to: string): EncodeObject
  export function offerJewelByLink(owner: string, jewelId: string, claimKey: string): EncodeObject
  export function cancelJewelOffer(owner: string, jewelId: string): EncodeObject
  export function acceptJewelOffer(recipient: string, jewelId: string): EncodeObject
  export function claimJewelOffer(claimer: string, jewelId: string, claimPrivateKey: Uint8Array, chainId: string): EncodeObject
  export function reportJewelStolen(owner: string, jewelId: string): EncodeObject
  export function clearJewelStolen(owner: string, jewelId: string): EncodeObject
  export function attachJewelBenefit(staff: string, jewelId: string, kind: string, allowance: number,
                                     periodSeconds: number, expiresAt: number): EncodeObject
  export function redeemJewelBenefit(staff: string, code: VoucherCode): EncodeObject
  export function setJewelStaff(business: string, jewelerId: string, add: string[], remove: string[]): EncodeObject
  export function vetoJewelReassignment(owner: string, jewelId: string): EncodeObject
  export function completeJewelReassignment(creator: string, jewelId: string): EncodeObject
  export function grantStaffFees(business: string, staff: string, spendLimit: Coin[], expiresAt: Date): EncodeObject
  ```

- [ ] **Step 1: Write the Go side of the vectors**

`x/jewel/types/vectors_test.go`:

```go
package types_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// Test vectors shared with clients/sdk/src/jewel.ts.
//
// The app signs claims and vouchers in a browser, and the chain checks them in
// Go. A difference of one byte between the two makes every claim fail on the
// chain, and nothing short of a real claim would show it. So neither
// implementation is the reference for the other: both are checked against
// this file. The same arrangement already guards recovery/ and the wallet.
//
// Regenerate with:
//
//	go test ./x/jewel/types -run TestVectors -update-vectors
//
// Regenerating is a decision, not a fix. If a vector stops matching and
// nobody meant to change a format, the code is wrong and the vector is right.
var updateVectors = flag.Bool("update-vectors", false, "rewrite testdata/vectors.json from this implementation")

type jewelVectors struct {
	Comment      string              `json:"comment"`
	Inscriptions []inscriptionVector `json:"inscriptions"`
	Claims       []claimVector       `json:"claims"`
	Vouchers     []voucherVector     `json:"vouchers"`
}

type inscriptionVector struct {
	In  string `json:"in"`
	Out string `json:"out"` // empty when the chain refuses the input
}

type claimVector struct {
	PrivKey   string `json:"priv_key"`
	PubKey    string `json:"pub_key"`
	Address   string `json:"address"`
	ChainID   string `json:"chain_id"`
	JewelID   string `json:"jewel_id"`
	Claimer   string `json:"claimer"`
	SignBytes string `json:"sign_bytes"`
	Signature string `json:"signature"`
}

type voucherVector struct {
	PrivKey   string `json:"priv_key"`
	PubKey    string `json:"pub_key"`
	Address   string `json:"address"`
	ChainID   string `json:"chain_id"`
	BenefitID string `json:"benefit_id"`
	JewelID   string `json:"jewel_id"`
	Nonce     string `json:"nonce"`
	ExpiresAt string `json:"expires_at"`
	SignBytes string `json:"sign_bytes"`
	Signature string `json:"signature"`
}

func vectorKey(label string) *secp256k1.PrivKey {
	sum := sha256.Sum256([]byte("yamale-jewel-vector:" + label))
	return &secp256k1.PrivKey{Key: sum[:]}
}

func ymlAddress(t *testing.T, k *secp256k1.PrivKey) string {
	s, err := bech32.ConvertAndEncode("yml", k.PubKey().Address())
	require.NoError(t, err)
	return s
}

func sign(t *testing.T, k *secp256k1.PrivKey, msg []byte) string {
	sig, err := k.Sign(msg)
	require.NoError(t, err)
	return hex.EncodeToString(sig)
}

func buildJewelVectors(t *testing.T) jewelVectors {
	v := jewelVectors{Comment: "Shared by x/jewel/types and clients/sdk/src/jewel.ts. " +
		"Regenerate with: go test ./x/jewel/types -run TestVectors -update-vectors"}

	for _, in := range []string{
		"MD-2026-0001", "md 2026/0001", "a.b_c-d 1/2", "abc", "ring é", "straße-1",
		"ıstanbul-9", "ſeal-77", "K-1234", "ring\t01",
		"0123456789012345678901234567890123456789",
		"0123456789012345678901234567890123456789X",
	} {
		out, err := types.NormaliseInscription(in)
		if err != nil {
			out = ""
		}
		v.Inscriptions = append(v.Inscriptions, inscriptionVector{In: in, Out: out})
	}

	claimer := ymlAddress(t, vectorKey("claimer"))
	for i, c := range []struct {
		chain string
		jewel uint64
	}{{"yamale-local-1", 1}, {"yamale-devnet-2", 18446744073709551615}} {
		k := vectorKey("claim-" + strconv.Itoa(i))
		msg := types.ClaimSignBytes(c.chain, c.jewel, claimer)
		v.Claims = append(v.Claims, claimVector{
			PrivKey: hex.EncodeToString(k.Key), PubKey: hex.EncodeToString(k.PubKey().Bytes()),
			Address: ymlAddress(t, k), ChainID: c.chain, JewelID: strconv.FormatUint(c.jewel, 10),
			Claimer: claimer, SignBytes: string(msg), Signature: sign(t, k, msg),
		})
	}

	for i, w := range []types.Voucher{
		{BenefitId: 1, JewelId: 1, Nonce: "00112233445566778899aabbccddeeff", ExpiresAt: 1790000000},
		{BenefitId: 42, JewelId: 9001, Nonce: "n", ExpiresAt: 1},
	} {
		k := vectorKey("voucher-" + strconv.Itoa(i))
		msg := types.VoucherSignBytes("yamale-local-1", w)
		v.Vouchers = append(v.Vouchers, voucherVector{
			PrivKey: hex.EncodeToString(k.Key), PubKey: hex.EncodeToString(k.PubKey().Bytes()),
			Address: ymlAddress(t, k), ChainID: "yamale-local-1",
			BenefitID: strconv.FormatUint(w.BenefitId, 10), JewelID: strconv.FormatUint(w.JewelId, 10),
			Nonce: w.Nonce, ExpiresAt: strconv.FormatInt(w.ExpiresAt, 10),
			SignBytes: string(msg), Signature: sign(t, k, msg),
		})
	}
	return v
}

func TestVectors(t *testing.T) {
	path := filepath.Join("testdata", "vectors.json")
	built := buildJewelVectors(t)
	if *updateVectors {
		out, err := json.MarshalIndent(built, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, append(out, '\n'), 0o644))
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "run once with -update-vectors to create the file")
	var stored jewelVectors
	require.NoError(t, json.Unmarshal(raw, &stored))
	require.Equal(t, built, stored)
}
```

- [ ] **Step 2: Generate the file and check what it pins**

```bash
export PATH="/c/Program Files/Go/bin:$PATH"
go test ./x/jewel/types -run TestVectors -update-vectors -count=1
go test ./x/jewel/types -run TestVectors -count=1
```

Expected: both runs `ok`.
- Open `x/jewel/types/testdata/vectors.json` and check that `MD-2026-0001` maps to `MD20260001`, `abc` to `""`, and `straße-1` to `""`.
- `ıstanbul-9` must map to `ISTANBUL9`. Go upper-cases a dotless ı to I, and the TypeScript must agree.

- [ ] **Step 3: Write the failing TypeScript tests**

`clients/sdk/src/jewel.test.ts`:

```ts
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import { chainRegistry } from './registry.ts';
import {
  addressOfPublicKey, claimJewelOffer, claimLink, claimSignBytes, compressedPublicKey,
  decodeVoucherCode, encodeVoucherCode, fromHex, makeOwnershipProof, makeVoucher,
  normaliseInscription, parseClaimFragment, signArbitrary, toHex, verifyOwnershipProof,
  voucherSignBytes, voucherSigner,
} from './jewel.ts';

const vectors = JSON.parse(readFileSync(
  fileURLToPath(new URL('../../../x/jewel/types/testdata/vectors.json', import.meta.url)), 'utf8'));
const text = (b: Uint8Array) => new TextDecoder().decode(b);

test('inscriptions normalise exactly as the chain does', () => {
  for (const v of vectors.inscriptions) {
    assert.equal(normaliseInscription(v.in) ?? '', v.out, `input ${JSON.stringify(v.in)}`);
  }
});

test('claim keys, sign bytes and signatures match the chain byte for byte', () => {
  for (const v of vectors.claims) {
    const priv = fromHex(v.priv_key);
    const pub = compressedPublicKey(priv);
    assert.equal(toHex(pub), v.pub_key);
    assert.equal(addressOfPublicKey(pub), v.address);
    const bytes = claimSignBytes(v.chain_id, v.jewel_id, v.claimer);
    assert.equal(text(bytes), v.sign_bytes);
    // Both sides sign deterministically (RFC 6979), so the bytes must be
    // identical, not merely both valid.
    assert.equal(toHex(signArbitrary(priv, bytes)), v.signature);
  }
});

test('voucher sign bytes and signatures match the chain byte for byte', () => {
  for (const v of vectors.vouchers) {
    const priv = fromHex(v.priv_key);
    const bytes = voucherSignBytes(v.chain_id,
      { benefitId: v.benefit_id, jewelId: v.jewel_id, nonce: v.nonce, expiresAt: v.expires_at });
    assert.equal(text(bytes), v.sign_bytes);
    assert.equal(toHex(signArbitrary(priv, bytes)), v.signature);
  }
});

test('a claim link carries the jewel and the key, and nothing else parses', () => {
  const priv = fromHex(vectors.claims[0].priv_key);
  const link = claimLink('https://loyalty.example', '12', priv);
  assert.equal(link, `https://loyalty.example/claim#12.${vectors.claims[0].priv_key}`);
  const parsed = parseClaimFragment(new URL(link).hash);
  assert.ok(parsed);
  assert.equal(parsed.jewelId, '12');
  assert.equal(toHex(parsed.privateKey), vectors.claims[0].priv_key);
  for (const bad of ['', '#12', '#0.' + 'a'.repeat(64), '#12.' + 'a'.repeat(63), '#x.' + 'a'.repeat(64)]) {
    assert.equal(parseClaimFragment(bad), null, bad);
  }
});

test('a claim message signs for the claimer named in it', () => {
  const v = vectors.claims[0];
  const msg = claimJewelOffer(v.claimer, v.jewel_id, fromHex(v.priv_key), v.chain_id);
  const decoded = chainRegistry().decode({
    typeUrl: msg.typeUrl,
    value: chainRegistry().encode(msg as never),
  }) as { claimSignature: Uint8Array; claimPubKey: Uint8Array };
  assert.equal(toHex(decoded.claimSignature), v.signature);
  assert.equal(toHex(decoded.claimPubKey), v.pub_key);
});

test('a voucher code survives the trip through text and names its signer', () => {
  const priv = fromHex(vectors.vouchers[0].priv_key);
  const code = makeVoucher(priv, 'yamale-local-1', '3', '7', 1_790_000_000, 240,
    (n) => new Uint8Array(n).fill(1));
  assert.equal(code.voucher.expiresAt, '1790000240');
  assert.equal(code.voucher.nonce, '01'.repeat(16));
  const back = decodeVoucherCode(encodeVoucherCode(code));
  assert.ok(back);
  assert.deepEqual(back.voucher, code.voucher);
  assert.equal(voucherSigner(back, 'yamale-local-1'), vectors.vouchers[0].address);
  assert.equal(voucherSigner(back, 'another-chain'), null);
  const tampered = { ...back, voucher: { ...back.voucher, expiresAt: '1790000999' } };
  assert.equal(voucherSigner(tampered, 'yamale-local-1'), null);
  assert.equal(decodeVoucherCode('YJV2.' + encodeVoucherCode(code).slice(5)), null);
  assert.equal(decodeVoucherCode('not a voucher'), null);
});

test('an ownership proof verifies for its jewel only, and not once altered', () => {
  const priv = fromHex(vectors.claims[1].priv_key);
  const query = makeOwnershipProof(priv, 'yamale-local-1', '5', 1_790_000_000);
  assert.deepEqual(verifyOwnershipProof('yamale-local-1', '5', query),
    { address: vectors.claims[1].address, issuedAt: 1_790_000_000 });
  assert.equal(verifyOwnershipProof('yamale-local-1', '6', query), null);
  assert.equal(verifyOwnershipProof('yamale-local-1', '5', query.replace('t=1790000000', 't=1790000001')), null);
});
```

- [ ] **Step 4: Run them and watch them fail**

Run: `cd clients && npm test --workspace @yamale/chain -- --test-name-pattern="inscription|claim|voucher|ownership"`
Expected: FAIL with `Cannot find module '.../jewel.ts'`.

- [ ] **Step 5: Write `clients/sdk/src/jewel.ts`**

```ts
/**
 * The jewel register, as the app speaks to it.
 *
 * Three formats here are also implemented in Go, in x/jewel/types: the
 * inscription normaliser, the claim sign bytes and the voucher sign bytes.
 * They are checked against the same file,
 * x/jewel/types/testdata/vectors.json, so the browser and the chain cannot
 * drift apart without a test saying so.
 *
 * Every id and every time is a decimal string, because that is how the
 * generated types carry uint64 and int64.
 */

import { secp256k1 } from '@noble/curves/secp256k1';
import { ripemd160 } from '@noble/hashes/legacy';
import { sha256 } from '@noble/hashes/sha2';
import { toBech32 } from '@cosmjs/encoding';

import { AllowedMsgAllowance, BasicAllowance } from './generated/cosmos/feegrant/v1beta1/feegrant.ts';
import type { Coin } from './denom.ts';
import type { EncodeObject } from './signing.ts';

const T = '/blockchain.jewel.v1.';
const utf8 = new TextEncoder();
const DECIMAL = /^(0|[1-9][0-9]*)$/;
const ID = /^[1-9][0-9]*$/;

/* ------------------------------------------------------------- encodings */

export function toHex(b: Uint8Array): string {
  let s = '';
  for (const x of b) s += x.toString(16).padStart(2, '0');
  return s;
}

export function fromHex(h: string): Uint8Array {
  if (!/^([0-9a-fA-F]{2})*$/.test(h)) throw new Error('not hex');
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(h.slice(2 * i, 2 * i + 2), 16);
  return out;
}

export function toBase64(b: Uint8Array): string {
  let s = '';
  for (const x of b) s += String.fromCharCode(x);
  return btoa(s);
}

export function toBase64Url(b: Uint8Array): string {
  return toBase64(b).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export function fromBase64Url(s: string): Uint8Array {
  if (!/^[A-Za-z0-9_-]*$/.test(s)) throw new Error('not base64url');
  const padded = s.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (s.length % 4)) % 4);
  const bin = atob(padded);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

/* ----------------------------------------------------------- inscriptions */

/**
 * What the chain stores for an engraved code: upper case, with spaces and the
 * separators - . / _ removed, 4 to 40 characters. Returns null where the chain
 * would refuse.
 *
 * Upper-cased one code point at a time, and a code point whose upper case is
 * more than one character is refused. That is what Go's strings.ToUpper does
 * by never applying multi-character mappings: "ß" stays "ß" there and is
 * refused, where a whole-string JavaScript toUpperCase would turn it into
 * "SS" and accept it.
 */
export function normaliseInscription(raw: string): string | null {
  let out = '';
  for (const ch of raw) {
    const up = ch.toUpperCase();
    if (up.length === 1 && /^[A-Z0-9]$/.test(up)) {
      out += up;
    } else if (ch === ' ' || ch === '-' || ch === '.' || ch === '/' || ch === '_') {
      continue;
    } else {
      return null;
    }
  }
  return out.length >= 4 && out.length <= 40 ? out : null;
}

/* ------------------------------------------------------------- sign bytes */

export function claimSignBytes(chainId: string, jewelId: string, claimer: string): Uint8Array {
  if (!ID.test(jewelId)) throw new Error(`jewel id ${jewelId}`);
  return utf8.encode(`yamale-jewel-claim/v1:${chainId}:${jewelId}:${claimer}`);
}

export interface VoucherFields { benefitId: string; jewelId: string; nonce: string; expiresAt: string }

export function voucherSignBytes(chainId: string, v: VoucherFields): Uint8Array {
  if (!ID.test(v.benefitId) || !ID.test(v.jewelId) || !DECIMAL.test(v.expiresAt)) {
    throw new Error('voucher fields must be decimal');
  }
  return utf8.encode(
    `yamale-jewel-voucher/v1:${chainId}:${v.benefitId}:${v.jewelId}:${v.nonce}:${v.expiresAt}`);
}

/** Not on the chain: the app alone reads it, to show who shared a link. */
export function ownershipProofBytes(chainId: string, jewelId: string, issuedAt: string): Uint8Array {
  return utf8.encode(`yamale-jewel-owner/v1:${chainId}:${jewelId}:${issuedAt}`);
}

/* ------------------------------------------------------------------- keys */

/** A Cosmos secp256k1 signature: SHA-256 of the message, low-S, r‖s. */
export function signArbitrary(privateKey: Uint8Array, message: Uint8Array): Uint8Array {
  return secp256k1.sign(sha256(message), privateKey, { lowS: true }).toCompactRawBytes();
}

export function verifyArbitrary(pubKey: Uint8Array, message: Uint8Array, signature: Uint8Array): boolean {
  if (pubKey.length !== 33 || signature.length !== 64) return false;
  try {
    return secp256k1.verify(signature, sha256(message), pubKey, { lowS: true });
  } catch {
    return false;
  }
}

export function compressedPublicKey(privateKey: Uint8Array): Uint8Array {
  return secp256k1.getPublicKey(privateKey, true);
}

export function addressOfPublicKey(pubKey: Uint8Array, prefix = 'yml'): string {
  return toBech32(prefix, ripemd160(sha256(pubKey)));
}

/* ------------------------------------------------------------ claim links */

export function newClaimKey(): { privateKey: Uint8Array; claimKey: string } {
  const privateKey = secp256k1.utils.randomPrivateKey();
  return { privateKey, claimKey: toHex(compressedPublicKey(privateKey)) };
}

/**
 * The key goes in the fragment. A browser sends the path and the query to the
 * server and keeps the fragment to itself, so the private half reaches no
 * server, ours included.
 */
export function claimLink(origin: string, jewelId: string, privateKey: Uint8Array): string {
  return `${origin.replace(/\/+$/, '')}/claim#${jewelId}.${toHex(privateKey)}`;
}

export function parseClaimFragment(fragment: string): { jewelId: string; privateKey: Uint8Array } | null {
  const m = /^#?([1-9][0-9]*)\.([0-9a-fA-F]{64})$/.exec(fragment);
  if (!m) return null;
  return { jewelId: m[1]!, privateKey: fromHex(m[2]!.toLowerCase()) };
}

/* --------------------------------------------------------------- vouchers */

export interface VoucherCode { voucher: VoucherFields; pubKey: Uint8Array; signature: Uint8Array }

const defaultRandom = (n: number) => crypto.getRandomValues(new Uint8Array(n));

/**
 * The owner's consent to one use of a benefit.
 *
 * chainNow is the latest block time, in unix seconds, not the phone's clock.
 * The chain refuses a voucher that expires more than 300 s after the block
 * that redeems it, and a phone two minutes fast would produce exactly that.
 * 240 s leaves the staff a minute to scan.
 */
export function makeVoucher(
  privateKey: Uint8Array, chainId: string, benefitId: string, jewelId: string,
  chainNow: number, ttlSeconds = 240, random: (n: number) => Uint8Array = defaultRandom,
): VoucherCode {
  const voucher: VoucherFields = {
    benefitId, jewelId, nonce: toHex(random(16)), expiresAt: String(Math.floor(chainNow) + ttlSeconds),
  };
  return {
    voucher,
    pubKey: compressedPublicKey(privateKey),
    signature: signArbitrary(privateKey, voucherSignBytes(chainId, voucher)),
  };
}

/** YJV1.<benefit>.<jewel>.<nonce>.<expires>.<pub key>.<signature>, keys base64url. */
export function encodeVoucherCode(code: VoucherCode): string {
  const v = code.voucher;
  return ['YJV1', v.benefitId, v.jewelId, v.nonce, v.expiresAt,
    toBase64Url(code.pubKey), toBase64Url(code.signature)].join('.');
}

export function decodeVoucherCode(text: string): VoucherCode | null {
  const parts = text.trim().split('.');
  if (parts.length !== 7 || parts[0] !== 'YJV1') return null;
  const [, benefitId, jewelId, nonce, expiresAt, pub, sig] = parts as [string, ...string[]];
  if (!ID.test(benefitId!) || !ID.test(jewelId!) || !DECIMAL.test(expiresAt!)) return null;
  if (!/^[0-9a-f]{1,64}$/.test(nonce!)) return null;
  try {
    const pubKey = fromBase64Url(pub!);
    const signature = fromBase64Url(sig!);
    if (pubKey.length !== 33 || signature.length !== 64) return null;
    return { voucher: { benefitId: benefitId!, jewelId: jewelId!, nonce: nonce!, expiresAt: expiresAt! }, pubKey, signature };
  } catch {
    return null;
  }
}

/** The account that signed, if the signature holds for this chain. */
export function voucherSigner(code: VoucherCode, chainId: string): string | null {
  return verifyArbitrary(code.pubKey, voucherSignBytes(chainId, code.voucher), code.signature)
    ? addressOfPublicKey(code.pubKey) : null;
}

/* ------------------------------------------------------ ownership proofs */

/** The query string of /own/<jewel id>?t=…&k=…&s=… */
export function makeOwnershipProof(privateKey: Uint8Array, chainId: string, jewelId: string, issuedAt: number): string {
  const t = String(Math.floor(issuedAt));
  const sig = signArbitrary(privateKey, ownershipProofBytes(chainId, jewelId, t));
  return `t=${t}&k=${toBase64Url(compressedPublicKey(privateKey))}&s=${toBase64Url(sig)}`;
}

export function verifyOwnershipProof(chainId: string, jewelId: string, query: string): { address: string; issuedAt: number } | null {
  const p = new URLSearchParams(query.startsWith('?') ? query.slice(1) : query);
  const t = p.get('t') ?? '';
  if (!DECIMAL.test(t)) return null;
  try {
    const pub = fromBase64Url(p.get('k') ?? '');
    const sig = fromBase64Url(p.get('s') ?? '');
    if (!verifyArbitrary(pub, ownershipProofBytes(chainId, jewelId, t), sig)) return null;
    return { address: addressOfPublicKey(pub), issuedAt: Number(t) };
  } catch {
    return null;
  }
}

/* --------------------------------------------------------------- messages */

/** Enough for the heaviest jewel message, an amendment at its 50-entry cap. */
export const JEWEL_GAS = 400_000;

/** What a jeweler's business pays for when its staff sign. */
export const STAFF_MESSAGES: readonly string[] = [
  'MsgRegisterJewel', 'MsgAmendJewel', 'MsgOfferJewel', 'MsgCancelOffer',
  'MsgAttachBenefit', 'MsgRedeemBenefit',
].map((m) => T + m);

const msg = (name: string, value: Record<string, unknown>): EncodeObject => ({ typeUrl: T + name, value });

export const registerJewel = (staff: string, inscription: string, recordHash: string, recordUri: string) =>
  msg('MsgRegisterJewel', { staff, inscription, recordHash, recordUri });
export const amendJewel = (staff: string, jewelId: string, recordHash: string, recordUri: string, note: string) =>
  msg('MsgAmendJewel', { staff, jewelId, recordHash, recordUri, note });
export const offerJewelToAccount = (owner: string, jewelId: string, to: string) =>
  msg('MsgOfferJewel', { owner, jewelId, to, claimKey: '' });
export const offerJewelByLink = (owner: string, jewelId: string, claimKey: string) =>
  msg('MsgOfferJewel', { owner, jewelId, to: '', claimKey });
export const cancelJewelOffer = (owner: string, jewelId: string) => msg('MsgCancelOffer', { owner, jewelId });
export const acceptJewelOffer = (recipient: string, jewelId: string) => msg('MsgAcceptOffer', { recipient, jewelId });
export const reportJewelStolen = (owner: string, jewelId: string) => msg('MsgReportStolen', { owner, jewelId });
export const clearJewelStolen = (owner: string, jewelId: string) => msg('MsgClearStolen', { owner, jewelId });
export const vetoJewelReassignment = (owner: string, jewelId: string) => msg('MsgVetoReassignment', { owner, jewelId });
export const completeJewelReassignment = (creator: string, jewelId: string) =>
  msg('MsgCompleteReassignment', { creator, jewelId });
export const setJewelStaff = (business: string, jewelerId: string, add: string[], remove: string[]) =>
  msg('MsgSetStaff', { business, jewelerId, add, remove });

export function attachJewelBenefit(staff: string, jewelId: string, kind: string, allowance: number,
  periodSeconds: number, expiresAt: number): EncodeObject {
  return msg('MsgAttachBenefit', {
    staff, jewelId, kind, allowance, period: String(periodSeconds), expiresAt: String(expiresAt),
  });
}

export function claimJewelOffer(claimer: string, jewelId: string, claimPrivateKey: Uint8Array, chainId: string): EncodeObject {
  return msg('MsgClaimOffer', {
    claimer, jewelId,
    claimPubKey: compressedPublicKey(claimPrivateKey),
    claimSignature: signArbitrary(claimPrivateKey, claimSignBytes(chainId, jewelId, claimer)),
  });
}

export function redeemJewelBenefit(staff: string, code: VoucherCode): EncodeObject {
  return msg('MsgRedeemBenefit', {
    staff, voucher: code.voucher, ownerPubKey: code.pubKey, ownerSignature: code.signature,
  });
}

/** The business pays its staff's fees, for staff work only. */
export function grantStaffFees(business: string, staff: string, spendLimit: Coin[], expiresAt: Date): EncodeObject {
  const basic = BasicAllowance.encode(BasicAllowance.fromPartial({ spendLimit, expiration: expiresAt })).finish();
  const allowed = AllowedMsgAllowance.encode(AllowedMsgAllowance.fromPartial({
    allowance: { typeUrl: '/cosmos.feegrant.v1beta1.BasicAllowance', value: basic },
    allowedMessages: [...STAFF_MESSAGES],
  })).finish();
  return {
    typeUrl: '/cosmos.feegrant.v1beta1.MsgGrantAllowance',
    value: {
      granter: business, grantee: staff,
      allowance: { typeUrl: '/cosmos.feegrant.v1beta1.AllowedMsgAllowance', value: allowed },
    },
  };
}
```

Check `BasicAllowance.fromPartial` in the generated `feegrant.ts`. If it does not take `expiration` as a `Date` (`useDate` decides this), pass whatever the existing `grantFeeAllowance` in `signing.ts` passes, since that one is known to encode.

- [ ] **Step 6: Declare the dependency and export**

- In `clients/sdk/package.json`, add `"@cosmjs/encoding": "^0.32.4"` to `dependencies`. It is already installed, as a dependency of proto-signing.
- In `clients/sdk/src/index.ts`, add one export block:

```ts
export {
  normaliseInscription, claimSignBytes, voucherSignBytes, ownershipProofBytes, signArbitrary,
  verifyArbitrary, compressedPublicKey, addressOfPublicKey, toHex, fromHex, toBase64, toBase64Url,
  fromBase64Url, newClaimKey, claimLink, parseClaimFragment, makeVoucher, encodeVoucherCode,
  decodeVoucherCode, voucherSigner, makeOwnershipProof, verifyOwnershipProof, STAFF_MESSAGES,
  JEWEL_GAS, registerJewel, amendJewel, offerJewelToAccount, offerJewelByLink, cancelJewelOffer,
  acceptJewelOffer, claimJewelOffer, reportJewelStolen, clearJewelStolen, attachJewelBenefit,
  redeemJewelBenefit, setJewelStaff, vetoJewelReassignment, completeJewelReassignment, grantStaffFees,
  type VoucherFields, type VoucherCode,
} from './jewel.ts';
```

If a name collides with an existing export from another module, for example `toHex`, leave that one out of this block. Callers that need it import it from `./jewel.ts` directly.

- [ ] **Step 7: Run the tests**

Run: `cd clients && npm test --workspace @yamale/chain && npm run typecheck --workspace @yamale/chain`

Expected: everything passes.
- If only the two signature assertions fail while the sign bytes match, RFC 6979 nonces differ between the two libraries. Report it as DONE_WITH_CONCERNS, including both signatures.
- Do not loosen the test to "verifies".

- [ ] **Step 8: Commit**

```bash
git add x/jewel/types/vectors_test.go x/jewel/types/testdata/vectors.json clients/sdk/src/jewel.ts clients/sdk/src/jewel.test.ts clients/sdk/src/index.ts clients/sdk/package.json
git commit -m "sdk: jewel signing, claim links, vouchers and ownership proofs, pinned to x/jewel by shared vectors"
```

---

### Task 3: Reading the register: ABCI, tx_search and the jewel reads

**Files:**
- Create: `clients/sdk/src/abci.ts`, `clients/sdk/src/abci.test.ts`
- Create: `clients/sdk/src/jewelreads.ts`, `clients/sdk/src/jewelreads.test.ts`
- Modify: `clients/sdk/src/index.ts`

**Interfaces:**
- Consumes:
  - Task 1's generated query types: `blockchain/jewel/v1/query.ts`, `cosmos/nft/v1beta1/query.ts` and `cosmos/feegrant/v1beta1/query.ts`.
  - `normaliseInscription` from Task 2.
- Produces:

  ```ts
  // abci.ts
  export type Outcome<T> =
    | { ok: true; value: T; height: number }
    | { ok: false; reason: 'not-found' | 'refused' | 'unreachable' | 'offline'; detail: string };
  export function setRpcBase(base: string): void            // default '/api/rpc'
  export async function abciQuery(path: string, request?: Uint8Array): Promise<Outcome<Uint8Array>>
  export function decoded<T>(o: Outcome<Uint8Array>, decode: (b: Uint8Array) => T): Outcome<T>
  export interface ChainHead { chainId: string; height: number; time: number }   // time: unix seconds
  export async function chainHead(): Promise<Outcome<ChainHead>>
  export interface TxEvent { type: string; attributes: { key: string; value: string }[] }
  export interface TxHit { hash: string; height: number; code: number; events: TxEvent[] }
  export async function txSearch(query: string, perPage?: number): Promise<Outcome<TxHit[]>>

  // jewelreads.ts
  export interface JewelView { jewel: Jewel; owner: string; offer?: Offer; reassignment?: Reassignment; benefits: Benefit[] }
  export async function readJewel(id: string): Promise<Outcome<JewelView>>
  export async function readJeweler(id: string): Promise<Outcome<Jeweler>>
  export async function readJewelers(): Promise<Outcome<Jeweler[]>>
  export async function readJewelParams(): Promise<Outcome<Params>>
  export async function findByInscription(normalised: string): Promise<Outcome<Jewel[]>>
  export async function jewelsHeldBy(address: string): Promise<Outcome<string[]>>        // jewel ids, ascending
  export function offerSearchQuery(address: string): string
  export async function offersAddressedTo(address: string, now: number): Promise<Outcome<JewelView[]>>
  export async function feeAllowanceExists(granter: string, grantee: string): Promise<Outcome<boolean>>
  export function tokenToJewelId(tokenId: string): string | null
  export function isActive(j: Jeweler): boolean
  export function roleOf(address: string, jewelers: Jeweler[]): { jeweler: Jeweler; business: boolean; staff: boolean } | null
  ```

- [ ] **Step 1: Write the failing transport tests**

`clients/sdk/src/abci.test.ts`:

```ts
import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';

import { abciQuery, chainHead, setRpcBase, txSearch } from './abci.ts';

const realFetch = globalThis.fetch;
afterEach(() => { globalThis.fetch = realFetch; setRpcBase('/api/rpc'); });

function serve(handler: (url: URL) => unknown) {
  const seen: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = new URL(String(input), 'http://test.invalid');
    seen.push(url.toString());
    return new Response(JSON.stringify(handler(url)), { status: 200 });
  }) as typeof fetch;
  return seen;
}

test('an ABCI query quotes its path and hex-encodes its request', async () => {
  setRpcBase('http://node:26957/');
  const seen = serve(() => ({ result: { response: { code: 0, value: 'AQI=', height: '12' } } }));
  const out = await abciQuery('/blockchain.jewel.v1.Query/Jewel', new Uint8Array([8, 7]));
  assert.deepEqual(out, { ok: true, value: new Uint8Array([1, 2]), height: 12 });
  assert.equal(seen[0], 'http://node:26957/abci_query?path=%22%2Fblockchain.jewel.v1.Query%2FJewel%22&data=0x0807');
});

test('a missing record is not-found, a node error is unreachable', async () => {
  serve(() => ({ result: { response: { code: 38, log: 'jewel 9: not found', height: '3' } } }));
  assert.equal((await abciQuery('/p')).ok, false);
  assert.equal(((await abciQuery('/p')) as { reason: string }).reason, 'not-found');
  serve(() => ({ error: { message: 'boom' } }));
  assert.equal(((await abciQuery('/p')) as { reason: string }).reason, 'unreachable');
});

test('tx_search sends its query as a JSON string, newest first, and reads the events', async () => {
  setRpcBase('http://node:26957');
  const seen = serve(() => ({
    result: {
      txs: [{
        hash: 'AB', height: '40',
        tx_result: { code: 0, events: [{ type: 'blockchain.jewel.v1.EventOfferMade',
          attributes: [{ key: 'jewel_id', value: '"4"' }, { key: 'to', value: '"yml1me"' }] }] },
      }],
      total_count: '1',
    },
  }));
  const out = await txSearch(`blockchain.jewel.v1.EventOfferMade.to='"yml1me"'`);
  assert.ok(out.ok);
  assert.equal(out.value[0]!.height, 40);
  assert.equal(out.value[0]!.events[0]!.attributes[1]!.value, '"yml1me"');
  const url = new URL(seen[0]!);
  assert.equal(url.searchParams.get('query'), `"blockchain.jewel.v1.EventOfferMade.to='\\"yml1me\\"'"`);
  assert.equal(url.searchParams.get('order_by'), '"desc"');
});

test('the chain head reads its time as unix seconds', async () => {
  serve(() => ({ result: { node_info: { network: 'yamale-local-1' },
    sync_info: { latest_block_height: '99', latest_block_time: '2026-11-13T10:00:00.123456Z' } } }));
  const out = await chainHead();
  assert.deepEqual(out, { ok: true, height: 99,
    value: { chainId: 'yamale-local-1', height: 99, time: Date.UTC(2026, 10, 13, 10, 0, 0) / 1000 } });
});
```

- [ ] **Step 2: Run them and watch them fail**

Run: `cd clients && npm test --workspace @yamale/chain -- --test-name-pattern="ABCI|tx_search|head|missing"`
Expected: FAIL, `Cannot find module './abci.ts'`.

- [ ] **Step 3: Write `clients/sdk/src/abci.ts`**

```ts
/**
 * Reading the chain without a password: ABCI queries, the chain head and
 * tx_search, over the node's CometBFT RPC.
 *
 * Promoted from clients/herd/src/abci.ts, which explains why reads go over
 * ABCI rather than REST: the public proxy allowlists REST paths per module and
 * answers the rest with a 401, which a browser renders as a login box. The one
 * change is that the RPC base can be set, so a Node script can point at a node
 * while a browser keeps the same-origin proxy. herd and rwa keep their own
 * copies for now.
 *
 * Every read returns an Outcome, never a value-or-null: "the chain says there
 * is no such jewel" and "the chain did not answer" must not render alike.
 */

export type Outcome<T> =
  | { ok: true; value: T; height: number }
  | { ok: false; reason: 'not-found' | 'refused' | 'unreachable' | 'offline'; detail: string };

let rpcBase = '/api/rpc';
const TIMEOUT_MS = 9000;

export function setRpcBase(base: string): void {
  rpcBase = base.replace(/\/+$/, '');
}

function offline(): boolean {
  return typeof navigator !== 'undefined' && navigator.onLine === false;
}

function hex(bytes: Uint8Array): string {
  let out = '0x';
  for (const b of bytes) out += b.toString(16).padStart(2, '0');
  return out;
}

function fromBase64(value: string): Uint8Array {
  if (value === '') return new Uint8Array(0);
  const bin = atob(value);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

async function rpc(path: string): Promise<{ ok: true; json: any } | { ok: false; detail: string }> {
  try {
    const res = await fetch(`${rpcBase}${path}`, { signal: AbortSignal.timeout(TIMEOUT_MS) });
    if (!res.ok) return { ok: false, detail: `http ${res.status}` };
    return { ok: true, json: await res.json() };
  } catch (err) {
    return { ok: false, detail: err instanceof Error ? err.message : String(err) };
  }
}

/**
 * The path is wrapped in literal double quotes before encoding: CometBFT reads
 * the parameter as a JSON string, and an unquoted path is refused as an
 * unknown query, which reads exactly like a module that is not installed.
 */
export async function abciQuery(path: string, request: Uint8Array = new Uint8Array(0)): Promise<Outcome<Uint8Array>> {
  if (offline()) return { ok: false, reason: 'offline', detail: path };
  const r = await rpc(`/abci_query?path=${encodeURIComponent(`"${path}"`)}&data=${hex(request)}`);
  if (!r.ok) return { ok: false, reason: 'unreachable', detail: r.detail };
  if (r.json?.error) return { ok: false, reason: 'unreachable', detail: r.json.error.message ?? 'rpc error' };
  const response = r.json?.result?.response;
  if (!response) return { ok: false, reason: 'unreachable', detail: 'no response' };
  const height = Number(response.height ?? '0');
  if (response.code) {
    const log = String(response.log ?? `abci code ${response.code}`);
    return { ok: false, reason: /not ?found|no such|key not found/i.test(log) ? 'not-found' : 'refused', detail: log };
  }
  return { ok: true, value: fromBase64(response.value ?? ''), height };
}

export function decoded<T>(outcome: Outcome<Uint8Array>, decode: (bytes: Uint8Array) => T): Outcome<T> {
  if (!outcome.ok) return outcome;
  try {
    return { ok: true, value: decode(outcome.value), height: outcome.height };
  } catch (err) {
    return { ok: false, reason: 'refused', detail: `could not decode: ${err instanceof Error ? err.message : String(err)}` };
  }
}

export interface ChainHead { chainId: string; height: number; time: number }

/** The latest block. Its time, not the device clock, dates vouchers. */
export async function chainHead(): Promise<Outcome<ChainHead>> {
  if (offline()) return { ok: false, reason: 'offline', detail: 'status' };
  const r = await rpc('/status');
  if (!r.ok) return { ok: false, reason: 'unreachable', detail: r.detail };
  const sync = r.json?.result?.sync_info;
  const node = r.json?.result?.node_info;
  if (!sync || !node) return { ok: false, reason: 'unreachable', detail: 'malformed status' };
  const height = Number(sync.latest_block_height ?? '0');
  const time = Math.floor(Date.parse(String(sync.latest_block_time)) / 1000);
  if (!Number.isFinite(time)) return { ok: false, reason: 'unreachable', detail: 'no block time' };
  return { ok: true, height, value: { chainId: String(node.network ?? ''), height, time } };
}

export interface TxEvent { type: string; attributes: { key: string; value: string }[] }
export interface TxHit { hash: string; height: number; code: number; events: TxEvent[] }

/**
 * Transactions whose events match a CometBFT query, newest first. Typed
 * events carry JSON-encoded values, so an address is matched as '"yml1…"',
 * quotes included.
 */
export async function txSearch(query: string, perPage = 50): Promise<Outcome<TxHit[]>> {
  if (offline()) return { ok: false, reason: 'offline', detail: 'tx_search' };
  const r = await rpc(`/tx_search?query=${encodeURIComponent(JSON.stringify(query))}`
    + `&prove=false&page=1&per_page=${perPage}&order_by=${encodeURIComponent('"desc"')}`);
  if (!r.ok) return { ok: false, reason: 'unreachable', detail: r.detail };
  if (r.json?.error) return { ok: false, reason: 'refused', detail: r.json.error.data ?? r.json.error.message ?? 'rpc error' };
  const txs: any[] = r.json?.result?.txs ?? [];
  const height = Number(txs[0]?.height ?? '0');
  return {
    ok: true, height,
    value: txs.map((tx) => ({
      hash: String(tx.hash), height: Number(tx.height), code: Number(tx.tx_result?.code ?? 0),
      events: (tx.tx_result?.events ?? []).map((e: any) => ({
        type: String(e.type),
        attributes: (e.attributes ?? []).map((a: any) => ({ key: String(a.key), value: String(a.value) })),
      })),
    })),
  };
}
```

- [ ] **Step 4: Run the transport tests**

Run: `cd clients && npm test --workspace @yamale/chain -- --test-name-pattern="ABCI|tx_search|head|missing"`
Expected: PASS, 4 tests.

- [ ] **Step 5: Write the failing read tests**

`clients/sdk/src/jewelreads.test.ts`:

```ts
import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';

import { setRpcBase } from './abci.ts';
import { JewelerStatus } from './generated/blockchain/jewel/v1/jewel.ts';
import { QueryJewelResponse, QueryJewelersResponse } from './generated/blockchain/jewel/v1/query.ts';
import { QueryNFTsResponse } from './generated/cosmos/nft/v1beta1/query.ts';
import {
  isActive, jewelsHeldBy, offerSearchQuery, offersAddressedTo, readJewel, readJewelers, roleOf, tokenToJewelId,
} from './jewelreads.ts';

const realFetch = globalThis.fetch;
afterEach(() => { globalThis.fetch = realFetch; setRpcBase('/api/rpc'); });

const b64 = (b: Uint8Array) => Buffer.from(b).toString('base64');
const answer = (bytes: Uint8Array) => ({ result: { response: { code: 0, value: b64(bytes), height: '20' } } });

function jewelReply(id: string, offerTo: string, expiresAt: string) {
  return answer(QueryJewelResponse.encode(QueryJewelResponse.fromPartial({
    jewel: { id, jewelerId: '1', inscription: 'MD20260001', ownerCount: 1, ownerSince: ['1790000000'], registeredAt: '1789990000' },
    owner: 'yml1seller',
    offer: offerTo ? { jewelId: id, from: 'yml1seller', to: offerTo, expiresAt, maker: 'yml1seller' } : undefined,
    benefits: [],
  })).finish());
}

function route(table: Record<string, unknown>) {
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input);
    const hit = Object.keys(table).find((k) => url.includes(k));
    if (!hit) throw new Error(`unrouted ${url}`);
    return new Response(JSON.stringify(table[hit]), { status: 200 });
  }) as typeof fetch;
}

test('a jewel reads with its owner and its open offer', async () => {
  route({ 'abci_query': jewelReply('4', 'yml1me', '1790100000') });
  const out = await readJewel('4');
  assert.ok(out.ok);
  assert.equal(out.value.jewel.inscription, 'MD20260001');
  assert.equal(out.value.owner, 'yml1seller');
  assert.equal(out.value.offer?.to, 'yml1me');
  assert.deepEqual(out.value.jewel.ownerSince, ['1790000000']);
});

test('the offer query matches the JSON-encoded address', () => {
  assert.equal(offerSearchQuery('yml1me'), `blockchain.jewel.v1.EventOfferMade.to='"yml1me"'`);
});

test('offers addressed to me are the ones still open to me', async () => {
  const hit = (id: string) => ({ hash: id, height: '9', tx_result: { code: 0, events: [{
    type: 'blockchain.jewel.v1.EventOfferMade',
    attributes: [{ key: 'jewel_id', value: `"${id}"` }, { key: 'to', value: '"yml1me"' }] }] } });
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = decodeURIComponent(String(input));
    if (url.includes('tx_search')) {
      return new Response(JSON.stringify({ result: { txs: [hit('4'), hit('9'), hit('4'), hit('11')] } }));
    }
    // 4: still open to me. 9: re-offered to someone else. 11: expired.
    const data = new URL(String(input), 'http://x').searchParams.get('data')!;
    const reply = data === '0x0804' ? jewelReply('4', 'yml1me', '1790100000')
      : data === '0x0809' ? jewelReply('9', 'yml1other', '1790100000')
      : jewelReply('11', 'yml1me', '1789000000');
    return new Response(JSON.stringify(reply));
  }) as typeof fetch;
  const out = await offersAddressedTo('yml1me', 1_790_000_500);
  assert.ok(out.ok);
  assert.deepEqual(out.value.map((v) => v.jewel.id), ['4']);
});

test('the jewels an account holds come from x/nft, jewel tokens only', async () => {
  route({ 'abci_query': answer(QueryNFTsResponse.encode(QueryNFTsResponse.fromPartial({
    nfts: [{ classId: 'jewel-1', id: 'jewel-12' }, { classId: 'jewel-2', id: 'jewel-3' }, { classId: 'title-1', id: 'deed-5' }],
  })).finish()) });
  const out = await jewelsHeldBy('yml1me');
  assert.ok(out.ok);
  assert.deepEqual(out.value, ['3', '12']);
  assert.equal(tokenToJewelId('jewel-007'), null);
});

test('jewelers read across pages, and a role is found by business or staff', async () => {
  const page1 = QueryJewelersResponse.encode(QueryJewelersResponse.fromPartial({
    jewelers: [{ id: '1', name: 'Maison Diop', business: 'yml1biz', staff: ['yml1staff'], status: JewelerStatus.JEWELER_ACTIVE }],
    pagination: { nextKey: new Uint8Array([1]) },
  })).finish();
  const page2 = QueryJewelersResponse.encode(QueryJewelersResponse.fromPartial({
    jewelers: [{ id: '2', name: 'Atelier Ba', business: 'yml1ba', staff: [], status: JewelerStatus.JEWELER_SUSPENDED }],
  })).finish();
  let calls = 0;
  globalThis.fetch = (async () => new Response(JSON.stringify(answer(calls++ === 0 ? page1 : page2)))) as typeof fetch;
  const out = await readJewelers();
  assert.ok(out.ok);
  assert.equal(out.value.length, 2);
  assert.deepEqual(roleOf('yml1staff', out.value), { jeweler: out.value[0], business: false, staff: true });
  assert.equal(roleOf('yml1ba', out.value)?.business, true);
  assert.equal(isActive(out.value[1]!), false);
  assert.equal(roleOf('yml1nobody', out.value), null);
});
```

- [ ] **Step 6: Run them and watch them fail**

Run: `cd clients && npm test --workspace @yamale/chain -- --test-name-pattern="jewel|offer|role"`
Expected: FAIL, `Cannot find module './jewelreads.ts'`.

- [ ] **Step 7: Write `clients/sdk/src/jewelreads.ts`**

```ts
/**
 * Every read the loyalty app makes of the jewel register.
 *
 * Chain state answers all of them but one: "which offers are addressed to
 * me" is not an index the module keeps, so it comes from tx_search on
 * EventOfferMade and is then checked against the jewel as it stands. An
 * event says an offer was made; only the jewel says it is still open.
 */

import { abciQuery, decoded, txSearch, type Outcome } from './abci.ts';
import { JewelerStatus, type Benefit, type Jewel, type Jeweler, type Offer, type Reassignment } from './generated/blockchain/jewel/v1/jewel.ts';
import type { Params } from './generated/blockchain/jewel/v1/params.ts';
import {
  QueryJewelRequest, QueryJewelResponse, QueryJewelerRequest, QueryJewelerResponse,
  QueryJewelersRequest, QueryJewelersResponse, QueryJewelsByInscriptionRequest,
  QueryJewelsByInscriptionResponse, QueryParamsRequest, QueryParamsResponse,
} from './generated/blockchain/jewel/v1/query.ts';
import { QueryNFTsRequest, QueryNFTsResponse } from './generated/cosmos/nft/v1beta1/query.ts';
import { QueryAllowanceRequest, QueryAllowanceResponse } from './generated/cosmos/feegrant/v1beta1/query.ts';

const SVC = '/blockchain.jewel.v1.Query';

export interface JewelView {
  jewel: Jewel;
  owner: string;
  offer?: Offer;
  reassignment?: Reassignment;
  benefits: Benefit[];
}

export async function readJewel(id: string): Promise<Outcome<JewelView>> {
  const out = decoded(
    await abciQuery(`${SVC}/Jewel`, QueryJewelRequest.encode(QueryJewelRequest.fromPartial({ id })).finish()),
    (b) => QueryJewelResponse.decode(b));
  if (!out.ok) return out;
  const r = out.value;
  if (!r.jewel) return { ok: false, reason: 'not-found', detail: `jewel ${id}` };
  return {
    ok: true, height: out.height,
    value: { jewel: r.jewel, owner: r.owner, offer: r.offer, reassignment: r.reassignment, benefits: r.benefits },
  };
}

export async function readJeweler(id: string): Promise<Outcome<Jeweler>> {
  const out = decoded(
    await abciQuery(`${SVC}/Jeweler`, QueryJewelerRequest.encode(QueryJewelerRequest.fromPartial({ id })).finish()),
    (b) => QueryJewelerResponse.decode(b));
  if (!out.ok) return out;
  if (!out.value.jeweler) return { ok: false, reason: 'not-found', detail: `jeweler ${id}` };
  return { ok: true, height: out.height, value: out.value.jeweler };
}

export async function readJewelers(): Promise<Outcome<Jeweler[]>> {
  const all: Jeweler[] = [];
  let key = new Uint8Array(0);
  let height = 0;
  for (let page = 0; page < 50; page++) {
    const req = QueryJewelersRequest.fromPartial({ pagination: { key, limit: '100' } });
    const out = decoded(await abciQuery(`${SVC}/Jewelers`, QueryJewelersRequest.encode(req).finish()),
      (b) => QueryJewelersResponse.decode(b));
    if (!out.ok) return out;
    height = out.height;
    all.push(...out.value.jewelers);
    const next = out.value.pagination?.nextKey;
    if (!next || next.length === 0) return { ok: true, value: all, height };
    key = next;
  }
  return { ok: false, reason: 'refused', detail: 'more than 50 pages of jewelers' };
}

export async function readJewelParams(): Promise<Outcome<Params>> {
  const out = decoded(await abciQuery(`${SVC}/Params`, QueryParamsRequest.encode({}).finish()),
    (b) => QueryParamsResponse.decode(b));
  if (!out.ok) return out;
  if (!out.value.params) return { ok: false, reason: 'not-found', detail: 'params' };
  return { ok: true, height: out.height, value: out.value.params };
}

/** Every jewel with this code, across jewelers: two may use the same one. */
export async function findByInscription(normalised: string): Promise<Outcome<Jewel[]>> {
  const req = QueryJewelsByInscriptionRequest.fromPartial({ inscription: normalised });
  const out = decoded(await abciQuery(`${SVC}/JewelsByInscription`, QueryJewelsByInscriptionRequest.encode(req).finish()),
    (b) => QueryJewelsByInscriptionResponse.decode(b));
  if (!out.ok) return out;
  return { ok: true, height: out.height, value: out.value.jewels };
}

export function tokenToJewelId(tokenId: string): string | null {
  const m = /^jewel-([1-9][0-9]*)$/.exec(tokenId);
  return m ? m[1]! : null;
}

/** Ownership lives in x/nft; x/jewel keeps no by-owner index. */
export async function jewelsHeldBy(address: string): Promise<Outcome<string[]>> {
  const req = QueryNFTsRequest.fromPartial({ owner: address, pagination: { limit: '500' } });
  const out = decoded(await abciQuery('/cosmos.nft.v1beta1.Query/NFTs', QueryNFTsRequest.encode(req).finish()),
    (b) => QueryNFTsResponse.decode(b));
  if (!out.ok) return out;
  const ids = out.value.nfts
    .filter((n) => n.classId.startsWith('jewel-'))
    .map((n) => tokenToJewelId(n.id))
    .filter((id): id is string => id !== null)
    .sort((a, b) => (BigInt(a) < BigInt(b) ? -1 : 1));
  return { ok: true, height: out.height, value: ids };
}

export function offerSearchQuery(address: string): string {
  return `blockchain.jewel.v1.EventOfferMade.to='"${address}"'`;
}

function attr(hit: { attributes: { key: string; value: string }[] }, key: string): string | null {
  const a = hit.attributes.find((x) => x.key === key);
  if (!a) return null;
  try { return String(JSON.parse(a.value)); } catch { return a.value; }
}

/** Offers made to this account that are still open to it at `now` (unix seconds). */
export async function offersAddressedTo(address: string, now: number): Promise<Outcome<JewelView[]>> {
  const found = await txSearch(offerSearchQuery(address));
  if (!found.ok) return found;
  const ids = new Set<string>();
  for (const tx of found.value) {
    if (tx.code !== 0) continue;
    for (const e of tx.events) {
      if (e.type !== 'blockchain.jewel.v1.EventOfferMade' || attr(e, 'to') !== address) continue;
      const id = attr(e, 'jewel_id');
      if (id) ids.add(id);
    }
  }
  const open: JewelView[] = [];
  for (const id of ids) {
    const view = await readJewel(id);
    if (!view.ok) {
      if (view.reason === 'not-found') continue;
      return view;
    }
    if (view.value.offer?.to === address && Number(view.value.offer.expiresAt) > now) open.push(view.value);
  }
  return { ok: true, height: found.height, value: open };
}

export async function feeAllowanceExists(granter: string, grantee: string): Promise<Outcome<boolean>> {
  const req = QueryAllowanceRequest.encode(QueryAllowanceRequest.fromPartial({ granter, grantee })).finish();
  const out = await abciQuery('/cosmos.feegrant.v1beta1.Query/Allowance', req);
  if (!out.ok) return out.reason === 'not-found' ? { ok: true, value: false, height: 0 } : out;
  const decodedOut = decoded(out, (b) => QueryAllowanceResponse.decode(b));
  if (!decodedOut.ok) return decodedOut;
  return { ok: true, value: decodedOut.value.allowance !== undefined, height: out.height };
}

export function isActive(j: Jeweler): boolean {
  return j.status === JewelerStatus.JEWELER_ACTIVE;
}

export function roleOf(address: string, jewelers: Jeweler[]): { jeweler: Jeweler; business: boolean; staff: boolean } | null {
  for (const j of jewelers) {
    const business = j.business === address;
    const staff = j.staff.includes(address);
    if (business || staff) return { jeweler: j, business, staff };
  }
  return null;
}
```

- [ ] **Step 8: Export, run, typecheck**

Append to `clients/sdk/src/index.ts`:

```ts
export { setRpcBase, abciQuery, decoded, chainHead, txSearch, type Outcome, type ChainHead, type TxHit, type TxEvent } from './abci.ts';
export {
  readJewel, readJeweler, readJewelers, readJewelParams, findByInscription, jewelsHeldBy,
  offerSearchQuery, offersAddressedTo, feeAllowanceExists, tokenToJewelId, isActive, roleOf, type JewelView,
} from './jewelreads.ts';
export { JewelerStatus, type Jewel, type Jeweler, type Benefit, type Offer, type Reassignment, type Amendment } from './generated/blockchain/jewel/v1/jewel.ts';
```

If `Outcome` or `decoded` already exist in the index under those names, export these as `JewelOutcome` and so on, and use that name in Tasks 6–10.

Run: `cd clients && npm test --workspace @yamale/chain && npm run typecheck --workspace @yamale/chain`
Expected: all tests pass; no type errors.

- [ ] **Step 9: Commit**

```bash
git add clients/sdk/src/abci.ts clients/sdk/src/abci.test.ts clients/sdk/src/jewelreads.ts clients/sdk/src/jewelreads.test.ts clients/sdk/src/index.ts
git commit -m "sdk: ABCI and tx_search transport with a settable base, and every read of the jewel register"
```

---

### Task 4: loyalty-api, part 1: signed requests, files and jeweler profiles

**Files:**
- Create: `tools/loyaltyapi/main.go`, `auth.go`, `chain.go`, `files.go`, `profiles.go`
- Create: `tools/loyaltyapi/helpers_test.go`, `auth_test.go`, `files_test.go`, `profiles_test.go`, `vectors_test.go`, `testdata/vectors.json` (generated)
- Create: `clients/sdk/src/loyaltyauth.ts`, `clients/sdk/src/loyaltyauth.test.ts`
- Modify: `clients/sdk/src/index.ts`

**Interfaces:**
- Consumes: `types.VerifySecp256k1` from `yamale/blockchain/x/jewel/types`. In TypeScript, `signArbitrary`, `compressedPublicKey`, `toBase64` and `toHex` from Task 2.
- Produces, over HTTP:

  | Request | Who may make it | Answers |
  |---|---|---|
  | `POST /v1/files` | Signed with purpose `file`, subject = SHA-256 hex of the body. The signer must be the business or staff of an **active** jeweler | `201 {"hash":"<64 hex>"}`, `401` bad signature, `403` not staff, `413` over 8 MiB, `415` not JPEG, PNG, WebP or JSON |
  | `GET /v1/files/{hash}` | Anyone | The bytes, with `Cache-Control: public, max-age=31536000, immutable`; `400` if not 64 lowercase hex, `404` if unknown |
  | `GET /v1/jewelers/{id}/profile` | Anyone | `{"logo":"<hash or empty>","colour":"#rrggbb","accent":"#rrggbb"}`, or `404` |
  | `PUT /v1/jewelers/{id}/profile` | Signed with purpose `profile`, subject = `<id>.<sha256 hex of body>`. The signer must be that jeweler's business | `200` with the stored profile, `400`, `401`, `403` |
  | `GET /v1/health` | Anyone | `{"ok":true}` |

- Produces, in Go: `type server struct`, `func (s *server) routes() http.Handler`, `func (s *server) signer(r *http.Request, purpose string, subject func(address string) string) (string, error)`, `func signBytes(purpose, chainID, subject string, issued int64) []byte`, `type Jeweler`, `type Chain interface`, `func jewelerOf([]Jeweler, string) (Jeweler, bool)`, `writeJSON`, `refuse`.
- Produces, in TypeScript:

  ```ts
  export type LoyaltyPurpose = 'file' | 'profile' | 'sponsor' | 'application';
  export function loyaltySignBytes(purpose: string, chainId: string, subject: string, issued: number): Uint8Array
  export function signedHeaders(privateKey: Uint8Array, purpose: LoyaltyPurpose, chainId: string,
                                subject: string, issued?: number): Record<string, string>
  export function sha256Hex(bytes: Uint8Array): string
  ```

- [ ] **Step 1: Write the test helpers and the failing auth tests**

`tools/loyaltyapi/helpers_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"
)

const testChain = "yamale-test-1"

var testNow = time.Unix(1_790_000_000, 0)

type fakeChain struct {
	js  []Jeweler
	err error
}

func (f fakeChain) Jewelers(context.Context) ([]Jeweler, error) { return f.js, f.err }

func testServer(t *testing.T, js ...Jeweler) *server {
	t.Helper()
	return &server{dir: t.TempDir(), chainID: testChain, chain: fakeChain{js: js}, now: func() time.Time { return testNow }}
}

func testKey(label string) *secp256k1.PrivKey {
	sum := sha256.Sum256([]byte("loyaltyapi-test:" + label))
	return &secp256k1.PrivKey{Key: sum[:]}
}

func addrOf(k *secp256k1.PrivKey) string {
	s, err := bech32.ConvertAndEncode("yml", k.PubKey().Address())
	if err != nil {
		panic(err)
	}
	return s
}

func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func sign(t *testing.T, r *http.Request, k *secp256k1.PrivKey, purpose, subject string, issued int64) *http.Request {
	t.Helper()
	sig, err := k.Sign(signBytes(purpose, testChain, subject, issued))
	require.NoError(t, err)
	r.Header.Set(headerKey, base64.StdEncoding.EncodeToString(k.PubKey().Bytes()))
	r.Header.Set(headerSignature, base64.StdEncoding.EncodeToString(sig))
	r.Header.Set(headerIssued, strconv.FormatInt(issued, 10))
	return r
}

func request(method, path string, body []byte) *http.Request {
	return httptest.NewRequest(method, path, bytes.NewReader(body))
}

func serve(s *server, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	return w
}

func activeJeweler(id string, business *secp256k1.PrivKey, staff ...*secp256k1.PrivKey) Jeweler {
	j := Jeweler{ID: id, Name: "Maison " + id, Business: addrOf(business), Status: "JEWELER_ACTIVE"}
	for _, k := range staff {
		j.Staff = append(j.Staff, addrOf(k))
	}
	return j
}
```

`tools/loyaltyapi/auth_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestASignedRequestNamesItsSigner(t *testing.T) {
	s := testServer(t)
	k := testKey("a")
	r := sign(t, request("POST", "/x", nil), k, "file", "abc", testNow.Unix())
	got, err := s.signer(r, "file", func(string) string { return "abc" })
	require.NoError(t, err)
	require.Equal(t, addrOf(k), got)
}

func TestASignatureIsBoundToPurposeSubjectAndTime(t *testing.T) {
	s := testServer(t)
	k := testKey("a")
	subject := func(string) string { return "abc" }
	cases := map[string]func() error{
		"another purpose": func() error {
			_, err := s.signer(sign(t, request("POST", "/x", nil), k, "profile", "abc", testNow.Unix()), "file", subject)
			return err
		},
		"another subject": func() error {
			_, err := s.signer(sign(t, request("POST", "/x", nil), k, "file", "abd", testNow.Unix()), "file", subject)
			return err
		},
		"six minutes old": func() error {
			_, err := s.signer(sign(t, request("POST", "/x", nil), k, "file", "abc", testNow.Unix()-360), "file", subject)
			return err
		},
		"six minutes ahead": func() error {
			_, err := s.signer(sign(t, request("POST", "/x", nil), k, "file", "abc", testNow.Unix()+360), "file", subject)
			return err
		},
		"unsigned": func() error {
			_, err := s.signer(request("POST", "/x", nil), "file", subject)
			return err
		},
	}
	for name, run := range cases {
		require.Error(t, run(), name)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `export PATH="/c/Program Files/Go/bin:$PATH"; go test ./tools/loyaltyapi -count=1`
Expected: FAIL to compile: `undefined: server`, `undefined: signBytes`, and so on.

- [ ] **Step 3: Write `main.go`, `auth.go` and `chain.go`**

`tools/loyaltyapi/main.go`:

```go
// Command loyaltyapi is the small backend of the jewel loyalty app.
//
// It does four things, and it never signs for anyone:
//
//   - it stores photos and record files under their SHA-256, so a record on
//     the chain can be fetched by its fingerprint and checked against it;
//   - it keeps each jeweler's logo and colours;
//   - it sponsors an account's fees once, through x/feegrant, for the owner
//     messages only (sponsor.go);
//   - it takes jeweler applications for the office to review.
//
// If it is down, photos stop and new accounts wait for their sponsorship. The
// register does not stop, because the app reads the chain directly.
//
//	loyaltyapi --listen 127.0.0.1:8095 --dir ./loyalty --rest http://127.0.0.1:1517 \
//	  --chain-id yamale-local-1
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type server struct {
	dir     string
	chainID string
	chain   Chain
	now     func() time.Time
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8095", "address to listen on")
	dir := flag.String("dir", "loyalty-data", "directory for files and profiles")
	rest := flag.String("rest", "http://127.0.0.1:1517", "the chain's REST API, read for jewelers and their staff")
	chainID := flag.String("chain-id", "yamale-local-1", "the chain id every signed request names")
	flag.Parse()

	for _, sub := range []string{"files", "profiles"} {
		if err := os.MkdirAll(filepath.Join(*dir, sub), 0o750); err != nil {
			log.Fatal(err)
		}
	}
	s := &server{
		dir:     *dir,
		chainID: *chainID,
		chain:   restChain{base: strings.TrimRight(*rest, "/"), client: &http.Client{Timeout: 10 * time.Second}},
		now:     time.Now,
	}
	srv := &http.Server{
		Addr: *listen, Handler: s.routes(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 2 * time.Minute,
	}
	log.Printf("loyaltyapi listening on %s for chain %s", *listen, *chainID)
	log.Fatal(srv.ListenAndServe())
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /v1/files", s.handleUpload)
	mux.HandleFunc("GET /v1/files/{hash}", s.handleFile)
	mux.HandleFunc("GET /v1/jewelers/{id}/profile", s.handleProfileGet)
	mux.HandleFunc("PUT /v1/jewelers/{id}/profile", s.handleProfilePut)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func refuse(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
```

`tools/loyaltyapi/auth.go`:

```go
package main

// Requests that change anything are signed by an account key.
//
// The API keeps no accounts and no passwords. A person is whoever holds the
// key, and what they may do is whatever the chain says that key is: staff of
// an active jeweler, its business, or simply an account. One format, shared
// with clients/sdk/src/loyaltyauth.ts and pinned by testdata/vectors.json:
//
//	yamale-loyalty/v1:<purpose>:<chain id>:<subject>:<issued, unix seconds>
//
// signed as Cosmos signs (SHA-256, low-S, 64 bytes r‖s), and sent as three
// headers. The subject binds the signature to this request's content, so a
// captured signature cannot be replayed onto another body; the five-minute
// window bounds how long a replay of the same body works, and every signed
// request here is idempotent.

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32"

	"yamale/blockchain/x/jewel/types"
)

const (
	headerKey       = "X-Yamale-Key"
	headerSignature = "X-Yamale-Signature"
	headerIssued    = "X-Yamale-Issued"
	maxClockSkew    = 5 * time.Minute
)

func signBytes(purpose, chainID, subject string, issued int64) []byte {
	return []byte(fmt.Sprintf("yamale-loyalty/v1:%s:%s:%s:%d", purpose, chainID, subject, issued))
}

// signer returns the account that signed r for purpose. subject is a function
// of that account because one purpose, sponsorship, is about the signer's own
// address.
func (s *server) signer(r *http.Request, purpose string, subject func(address string) string) (string, error) {
	pub, err := base64.StdEncoding.DecodeString(r.Header.Get(headerKey))
	if err != nil || len(pub) != secp256k1.PubKeySize {
		return "", errors.New("the request is not signed: " + headerKey + " must be a 33-byte public key in base64")
	}
	sig, err := base64.StdEncoding.DecodeString(r.Header.Get(headerSignature))
	if err != nil || len(sig) != 64 {
		return "", errors.New("the request is not signed: " + headerSignature + " must be 64 bytes in base64")
	}
	issued, err := strconv.ParseInt(r.Header.Get(headerIssued), 10, 64)
	if err != nil {
		return "", errors.New(headerIssued + " must be unix seconds")
	}
	if d := s.now().Sub(time.Unix(issued, 0)); d > maxClockSkew || d < -maxClockSkew {
		return "", errors.New("the signature is more than five minutes from this server's clock; check the device's time and sign again")
	}
	address, err := bech32.ConvertAndEncode("yml", (&secp256k1.PubKey{Key: pub}).Address())
	if err != nil {
		return "", err
	}
	if _, ok := types.VerifySecp256k1(pub, signBytes(purpose, s.chainID, subject(address), issued), sig); !ok {
		return "", errors.New("the signature does not match this request")
	}
	return address, nil
}
```

`tools/loyaltyapi/chain.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// Jeweler is what the API needs to know about one, read from the chain's REST
// API, which encodes ids as strings and the status as its enum name.
type Jeweler struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Business string   `json:"business"`
	Staff    []string `json:"staff"`
	Status   string   `json:"status"`
}

func (j Jeweler) Active() bool { return j.Status == "JEWELER_ACTIVE" }

// Chain is the one thing the API asks the chain. It is asked on every write
// rather than cached, so removing a staff member takes effect at once.
type Chain interface {
	Jewelers(ctx context.Context) ([]Jeweler, error)
}

type restChain struct {
	base   string
	client *http.Client
}

func (c restChain) Jewelers(ctx context.Context) ([]Jeweler, error) {
	var all []Jeweler
	key := ""
	for page := 0; page < 100; page++ {
		u := c.base + "/yamale/blockchain/jewel/v1/jewelers?pagination.limit=200"
		if key != "" {
			u += "&pagination.key=" + url.QueryEscape(key)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		res, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("the chain did not answer: %w", err)
		}
		var body struct {
			Jewelers   []Jeweler `json:"jewelers"`
			Pagination struct {
				NextKey string `json:"next_key"`
			} `json:"pagination"`
		}
		err = json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("the chain answered %d for the jewelers", res.StatusCode)
		}
		if err != nil {
			return nil, fmt.Errorf("unreadable jewelers: %w", err)
		}
		all = append(all, body.Jewelers...)
		if body.Pagination.NextKey == "" {
			return all, nil
		}
		key = body.Pagination.NextKey
	}
	return nil, errors.New("more than 100 pages of jewelers")
}

// jewelerOf is the active jeweler for which address is the business or staff.
func jewelerOf(js []Jeweler, address string) (Jeweler, bool) {
	for _, j := range js {
		if !j.Active() {
			continue
		}
		if j.Business == address {
			return j, true
		}
		for _, s := range j.Staff {
			if s == address {
				return j, true
			}
		}
	}
	return Jeweler{}, false
}

func jewelerByID(js []Jeweler, id string) (Jeweler, bool) {
	for _, j := range js {
		if j.ID == id {
			return j, true
		}
	}
	return Jeweler{}, false
}
```

To compile before Step 7, add empty stubs for the four handlers to `main.go`, for example `func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {}`. Steps 6 and 9 replace them.

- [ ] **Step 4: Run the auth tests**

Run: `go test ./tools/loyaltyapi -run 'Signed|Signature' -count=1`
Expected: PASS.

- [ ] **Step 5: Write the failing file tests**

`tools/loyaltyapi/files_test.go`:

```go
package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{7}, 64)...)

func TestStaffStoreAPhotoAndAnyoneReadsItBack(t *testing.T) {
	biz, staff := testKey("biz"), testKey("staff")
	s := testServer(t, activeJeweler("1", biz, staff))
	w := serve(s, sign(t, request("POST", "/v1/files", pngBytes), staff, "file", hashHex(pngBytes), testNow.Unix()))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.JSONEq(t, `{"hash":"`+hashHex(pngBytes)+`"}`, w.Body.String())

	got := serve(s, request("GET", "/v1/files/"+hashHex(pngBytes), nil))
	require.Equal(t, http.StatusOK, got.Code)
	require.Equal(t, pngBytes, got.Body.Bytes())
	require.Equal(t, "image/png", got.Header().Get("Content-Type"))
	require.Equal(t, "public, max-age=31536000, immutable", got.Header().Get("Cache-Control"))
}

func TestARecordIsServedAsJSON(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	rec := []byte(`{"version":1,"title":"Ring"}`)
	require.Equal(t, http.StatusCreated, serve(s, sign(t, request("POST", "/v1/files", rec), biz, "file", hashHex(rec), testNow.Unix())).Code)
	require.Equal(t, "application/json", serve(s, request("GET", "/v1/files/"+hashHex(rec), nil)).Header().Get("Content-Type"))
}

func TestStrangersAndSuspendedJewelersCannotStore(t *testing.T) {
	biz, stranger, susp := testKey("biz"), testKey("stranger"), testKey("susp")
	suspended := activeJeweler("2", susp)
	suspended.Status = "JEWELER_SUSPENDED"
	s := testServer(t, activeJeweler("1", biz), suspended)
	require.Equal(t, http.StatusForbidden, serve(s, sign(t, request("POST", "/v1/files", pngBytes), stranger, "file", hashHex(pngBytes), testNow.Unix())).Code)
	require.Equal(t, http.StatusForbidden, serve(s, sign(t, request("POST", "/v1/files", pngBytes), susp, "file", hashHex(pngBytes), testNow.Unix())).Code)
}

func TestTheSignatureMustCoverTheBody(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	other := []byte(`{"version":1}`)
	require.Equal(t, http.StatusUnauthorized, serve(s, sign(t, request("POST", "/v1/files", pngBytes), biz, "file", hashHex(other), testNow.Unix())).Code)
}

func TestUnsupportedAndOversizedFilesAreRefused(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	txt := []byte("just some words")
	require.Equal(t, http.StatusUnsupportedMediaType, serve(s, sign(t, request("POST", "/v1/files", txt), biz, "file", hashHex(txt), testNow.Unix())).Code)
	big := append(append([]byte{}, pngBytes...), bytes.Repeat([]byte{1}, maxFileBytes)...)
	require.Equal(t, http.StatusRequestEntityTooLarge, serve(s, sign(t, request("POST", "/v1/files", big), biz, "file", hashHex(big), testNow.Unix())).Code)
}

func TestAFileIsNamedByItsFingerprint(t *testing.T) {
	s := testServer(t)
	require.Equal(t, http.StatusBadRequest, serve(s, request("GET", "/v1/files/ZZ", nil)).Code)
	require.Equal(t, http.StatusNotFound, serve(s, request("GET", "/v1/files/"+hashHex([]byte("absent")), nil)).Code)
}
```

- [ ] **Step 6: Write `files.go`**

```go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// Files are stored by their SHA-256 and served under it. Nobody can replace
// one: a different body is a different name. That is what lets the app check
// every photo and record against the fingerprint the chain holds.

const maxFileBytes = 8 << 20

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (s *server) filePath(hash string) string {
	return filepath.Join(s.dir, "files", hash[:2], hash)
}

func fileType(b []byte) string {
	switch t := http.DetectContentType(b); t {
	case "image/jpeg", "image/png", "image/webp":
		return t
	}
	if json.Valid(b) {
		return "application/json"
	}
	return ""
}

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFileBytes))
	if err != nil {
		refuse(w, http.StatusRequestEntityTooLarge, "a file may be at most 8 MiB")
		return
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])

	address, err := s.signer(r, "file", func(string) string { return hash })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	js, err := s.chain.Jewelers(r.Context())
	if err != nil {
		refuse(w, http.StatusBadGateway, err.Error())
		return
	}
	if _, ok := jewelerOf(js, address); !ok {
		refuse(w, http.StatusForbidden, "only the staff of an active jeweler can store files")
		return
	}
	ctype := fileType(body)
	if ctype == "" {
		refuse(w, http.StatusUnsupportedMediaType, "a photo must be JPEG, PNG or WebP, and a record must be JSON")
		return
	}

	path := s.filePath(hash)
	if _, err := os.Stat(path); err == nil {
		writeJSON(w, http.StatusCreated, map[string]string{"hash": hash})
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := writeAtomically(path+".type", []byte(ctype)); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := writeAtomically(path, body); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"hash": hash})
}

func (s *server) handleFile(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if !hashPattern.MatchString(hash) {
		refuse(w, http.StatusBadRequest, "a file is named by 64 lowercase hex characters")
		return
	}
	body, err := os.ReadFile(s.filePath(hash))
	if err != nil {
		refuse(w, http.StatusNotFound, "no such file")
		return
	}
	ctype, err := os.ReadFile(s.filePath(hash) + ".type")
	if err != nil {
		ctype = []byte("application/octet-stream")
	}
	w.Header().Set("Content-Type", string(ctype))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+hash+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}

// writeAtomically writes beside the target and renames, so a reader never
// sees half a file and a crash leaves either the old state or the new one.
func writeAtomically(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
```

Remove the `handleUpload` and `handleFile` stubs from `main.go`.

- [ ] **Step 7: Run the file tests**

Run: `go test ./tools/loyaltyapi -count=1`
Expected: PASS.

- [ ] **Step 8: Write the failing profile tests**

`tools/loyaltyapi/profiles_test.go`:

```go
package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTheBusinessSetsItsLookAndAnyoneReadsIt(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	logo := pngBytes
	require.Equal(t, http.StatusCreated, serve(s, sign(t, request("POST", "/v1/files", logo), biz, "file", hashHex(logo), testNow.Unix())).Code)

	body := []byte(`{"logo":"` + hashHex(logo) + `","colour":"#12253f","accent":"#a87b3c"}`)
	w := serve(s, sign(t, request("PUT", "/v1/jewelers/1/profile", body), biz, "profile", "1."+hashHex(body), testNow.Unix()))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := serve(s, request("GET", "/v1/jewelers/1/profile", nil))
	require.Equal(t, http.StatusOK, got.Code)
	require.JSONEq(t, string(body), got.Body.String())
}

func TestOnlyTheBusinessSetsTheLook(t *testing.T) {
	biz, staff := testKey("biz"), testKey("staff")
	s := testServer(t, activeJeweler("1", biz, staff))
	body := []byte(`{"logo":"","colour":"#12253f","accent":"#a87b3c"}`)
	require.Equal(t, http.StatusForbidden, serve(s, sign(t, request("PUT", "/v1/jewelers/1/profile", body), staff, "profile", "1."+hashHex(body), testNow.Unix())).Code)
	// Signed for jeweler 2, sent to jeweler 1.
	require.Equal(t, http.StatusUnauthorized, serve(s, sign(t, request("PUT", "/v1/jewelers/1/profile", body), biz, "profile", "2."+hashHex(body), testNow.Unix())).Code)
}

func TestAProfileIsChecked(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	for _, body := range []string{
		`{"logo":"","colour":"navy","accent":"#a87b3c"}`,
		`{"logo":"","colour":"#12253F","accent":"#a87b3c"}`,
		`{"logo":"` + hashHex([]byte("never stored")) + `","colour":"#12253f","accent":"#a87b3c"}`,
		`{"logo":"","colour":"#12253f","accent":"#a87b3c","font":"Comic"}`,
	} {
		b := []byte(body)
		require.Equal(t, http.StatusBadRequest, serve(s, sign(t, request("PUT", "/v1/jewelers/1/profile", b), biz, "profile", "1."+hashHex(b), testNow.Unix())).Code, body)
	}
	require.Equal(t, http.StatusNotFound, serve(s, request("GET", "/v1/jewelers/1/profile", nil)).Code)
}
```

- [ ] **Step 9: Write `profiles.go`**

```go
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// A jeweler's look on its own jewels' pages: a logo and two colours. It lives
// here rather than on the chain because it is presentation, and a jeweler
// should be able to change it without a transaction.

type profile struct {
	Logo   string `json:"logo"`
	Colour string `json:"colour"`
	Accent string `json:"accent"`
}

var (
	colourPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	idPattern     = regexp.MustCompile(`^[1-9][0-9]*$`)
)

func (s *server) profilePath(id string) string {
	return filepath.Join(s.dir, "profiles", id+".json")
}

func (s *server) handleProfileGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		refuse(w, http.StatusBadRequest, "a jeweler id is a positive integer")
		return
	}
	body, err := os.ReadFile(s.profilePath(id))
	if err != nil {
		refuse(w, http.StatusNotFound, "this jeweler has not set a profile")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write(body)
}

func (s *server) handleProfilePut(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		refuse(w, http.StatusBadRequest, "a jeweler id is a positive integer")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<10))
	if err != nil {
		refuse(w, http.StatusRequestEntityTooLarge, "a profile is at most 4 KiB")
		return
	}
	sum := sha256.Sum256(body)
	address, err := s.signer(r, "profile", func(string) string { return id + "." + hex.EncodeToString(sum[:]) })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	var p profile
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		refuse(w, http.StatusBadRequest, "a profile is {logo, colour, accent}: "+err.Error())
		return
	}
	if !colourPattern.MatchString(p.Colour) || !colourPattern.MatchString(p.Accent) {
		refuse(w, http.StatusBadRequest, "colours are written #rrggbb in lowercase hex")
		return
	}
	if p.Logo != "" {
		if !hashPattern.MatchString(p.Logo) {
			refuse(w, http.StatusBadRequest, "the logo is named by its fingerprint")
			return
		}
		if _, err := os.Stat(s.filePath(p.Logo)); err != nil {
			refuse(w, http.StatusBadRequest, "store the logo as a file before naming it here")
			return
		}
	}
	js, err := s.chain.Jewelers(r.Context())
	if err != nil {
		refuse(w, http.StatusBadGateway, err.Error())
		return
	}
	j, ok := jewelerByID(js, id)
	if !ok || j.Business != address {
		refuse(w, http.StatusForbidden, "only the jeweler's business account sets its profile")
		return
	}
	stored, _ := json.Marshal(p)
	if err := writeAtomically(s.profilePath(id), stored); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}
```

Remove the remaining stubs from `main.go`.

- [ ] **Step 10: Run all the API tests**

Run: `go test ./tools/loyaltyapi -count=1`
Expected: PASS.

- [ ] **Step 11: Pin the request format with vectors, Go side**

`tools/loyaltyapi/vectors_test.go`:

```go
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Shared with clients/sdk/src/loyaltyauth.ts. Regenerate with:
//
//	go test ./tools/loyaltyapi -run TestVectors -update-vectors
var updateVectors = flag.Bool("update-vectors", false, "rewrite testdata/vectors.json")

type requestVector struct {
	PrivKey   string `json:"priv_key"`
	PubKey    string `json:"pub_key"`
	Address   string `json:"address"`
	Purpose   string `json:"purpose"`
	ChainID   string `json:"chain_id"`
	Subject   string `json:"subject"`
	Issued    int64  `json:"issued"`
	SignBytes string `json:"sign_bytes"`
	Signature string `json:"signature"`
}

func TestVectors(t *testing.T) {
	var built []requestVector
	for _, c := range []struct{ label, purpose, subject string }{
		{"staff", "file", hashHex([]byte("photo"))},
		{"business", "profile", "12." + hashHex([]byte(`{"logo":""}`))},
		{"owner", "sponsor", ""},
		{"applicant", "application", hashHex([]byte(`{}`))},
	} {
		k := testKey(c.label)
		subject := c.subject
		if c.purpose == "sponsor" {
			subject = addrOf(k)
		}
		msg := signBytes(c.purpose, "yamale-local-1", subject, 1_790_000_000)
		sig, err := k.Sign(msg)
		require.NoError(t, err)
		built = append(built, requestVector{
			PrivKey: hex.EncodeToString(k.Key), PubKey: hex.EncodeToString(k.PubKey().Bytes()), Address: addrOf(k),
			Purpose: c.purpose, ChainID: "yamale-local-1", Subject: subject, Issued: 1_790_000_000,
			SignBytes: string(msg), Signature: hex.EncodeToString(sig),
		})
	}
	path := filepath.Join("testdata", "vectors.json")
	if *updateVectors {
		out, err := json.MarshalIndent(built, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, append(out, '\n'), 0o644))
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "run once with -update-vectors")
	var stored []requestVector
	require.NoError(t, json.Unmarshal(raw, &stored))
	require.Equal(t, built, stored)
}
```

Run: `go test ./tools/loyaltyapi -run TestVectors -update-vectors -count=1 && go test ./tools/loyaltyapi -count=1`
Expected: `ok` twice; `tools/loyaltyapi/testdata/vectors.json` holds 4 entries.

- [ ] **Step 12: Write the failing TypeScript test**

`clients/sdk/src/loyaltyauth.test.ts`:

```ts
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import { fromHex, toHex } from './jewel.ts';
import { loyaltySignBytes, sha256Hex, signedHeaders } from './loyaltyauth.ts';

const vectors = JSON.parse(readFileSync(
  fileURLToPath(new URL('../../../tools/loyaltyapi/testdata/vectors.json', import.meta.url)), 'utf8'));

test('signed requests match loyalty-api byte for byte', () => {
  for (const v of vectors) {
    const bytes = loyaltySignBytes(v.purpose, v.chain_id, v.subject, v.issued);
    assert.equal(new TextDecoder().decode(bytes), v.sign_bytes);
    const h = signedHeaders(fromHex(v.priv_key), v.purpose, v.chain_id, v.subject, v.issued);
    assert.equal(toHex(Uint8Array.from(atob(h['X-Yamale-Key']!), (c) => c.charCodeAt(0))), v.pub_key);
    assert.equal(toHex(Uint8Array.from(atob(h['X-Yamale-Signature']!), (c) => c.charCodeAt(0))), v.signature);
    assert.equal(h['X-Yamale-Issued'], String(v.issued));
  }
});

test('the fingerprint is lowercase hex SHA-256', () => {
  assert.equal(sha256Hex(new TextEncoder().encode('abc')),
    'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad');
});
```

Run: `cd clients && npm test --workspace @yamale/chain -- --test-name-pattern="loyalty|fingerprint"`
Expected: FAIL, `Cannot find module './loyaltyauth.ts'`.

- [ ] **Step 13: Write `clients/sdk/src/loyaltyauth.ts`**

```ts
/**
 * Signing requests to loyalty-api: the browser's half of
 * tools/loyaltyapi/auth.go, pinned by tools/loyaltyapi/testdata/vectors.json.
 *
 *   yamale-loyalty/v1:<purpose>:<chain id>:<subject>:<issued, unix seconds>
 *
 * The subject binds the signature to the request's content: the file's
 * fingerprint, "<jeweler id>.<fingerprint>" for a profile, the signer's own
 * address for sponsorship.
 */

import { sha256 } from '@noble/hashes/sha2';

import { compressedPublicKey, signArbitrary, toBase64, toHex } from './jewel.ts';

export type LoyaltyPurpose = 'file' | 'profile' | 'sponsor' | 'application';

const utf8 = new TextEncoder();

export function loyaltySignBytes(purpose: string, chainId: string, subject: string, issued: number): Uint8Array {
  return utf8.encode(`yamale-loyalty/v1:${purpose}:${chainId}:${subject}:${Math.floor(issued)}`);
}

export function sha256Hex(bytes: Uint8Array): string {
  return toHex(sha256(bytes));
}

export function signedHeaders(
  privateKey: Uint8Array, purpose: LoyaltyPurpose, chainId: string, subject: string,
  issued: number = Math.floor(Date.now() / 1000),
): Record<string, string> {
  const signature = signArbitrary(privateKey, loyaltySignBytes(purpose, chainId, subject, issued));
  return {
    'X-Yamale-Key': toBase64(compressedPublicKey(privateKey)),
    'X-Yamale-Signature': toBase64(signature),
    'X-Yamale-Issued': String(Math.floor(issued)),
  };
}
```

Append to `clients/sdk/src/index.ts`:

```ts
export { loyaltySignBytes, sha256Hex, signedHeaders, type LoyaltyPurpose } from './loyaltyauth.ts';
```

- [ ] **Step 14: Run everything this task touched**

Run: `go test ./tools/loyaltyapi -count=1 && go vet ./tools/loyaltyapi && cd clients && npm test --workspace @yamale/chain && npm run typecheck --workspace @yamale/chain`
Expected: all pass, and `go vet` prints nothing.

- [ ] **Step 15: Commit**

```bash
git add tools/loyaltyapi clients/sdk/src/loyaltyauth.ts clients/sdk/src/loyaltyauth.test.ts clients/sdk/src/index.ts
git commit -m "loyaltyapi: signed requests, content-addressed files and jeweler profiles; the TypeScript half pinned by vectors"
```

---

### Task 5: loyalty-api, part 2: fee sponsorship and jeweler applications

**Files:**
- Create: `tools/loyaltyapi/sponsor.go`, `sponsor_test.go`, `applications.go`, `applications_test.go`
- Modify: `tools/loyaltyapi/main.go` (fields, flags, routes)
- Create: `scripts/devnet/jewel-lib.sh`, `scripts/devnet/loyalty-admit.sh`, `scripts/devnet/loyalty-local.sh`
- Modify: `scripts/devnet/seed-jewel.sh` (sources the library)

**Interfaces:**
- Consumes: `server`, `signer`, `writeJSON`, `refuse`, `writeAtomically`, `hashPattern` and the test helpers from Task 4.
- Produces, over HTTP:

  | Request | Who | Answers |
  |---|---|---|
  | `GET /v1/sponsor` | Anyone | `{"granter":"<address>"}`, or `503` when sponsoring is off |
  | `POST /v1/sponsor` | Signed with purpose `sponsor`, subject = the signer's own address | `200 {"granted":true,"tx":"…","granter":"…"}` the first time; `200 {"granted":false,"already":true,"granter":"…"}` after that; `429` over the daily ceiling; `502` if the grant failed; `503` when off |
  | `POST /v1/applications` | Signed with purpose `application`, subject = SHA-256 hex of the body. The signer must be the `business` the body names | `201 {"received":true}`, `400`, `401`, `403` |
  | `GET /v1/applications` | `Authorization: Bearer <operator token>` | `200 [ {received_at, application}, … ]` newest first; `401` without the token; `404` when no token is configured |

- Produces, in Go: `type Granter interface { Grant(ctx context.Context, grantee string) (string, error); Address() string }`, `var errAlreadyGranted`, `var ownerMessages []string`, and `newCLIGranter(...)`.

The owner messages a sponsorship covers. They are also the fee rule the spec's section 4.5 asks for:

```
/blockchain.jewel.v1.MsgOfferJewel        /blockchain.jewel.v1.MsgCancelOffer
/blockchain.jewel.v1.MsgAcceptOffer       /blockchain.jewel.v1.MsgClaimOffer
/blockchain.jewel.v1.MsgReportStolen      /blockchain.jewel.v1.MsgClearStolen
/blockchain.jewel.v1.MsgVetoReassignment  /blockchain.jewel.v1.MsgCompleteReassignment
```

- [ ] **Step 1: Write the failing sponsorship tests**

`tools/loyaltyapi/sponsor_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeGranter struct {
	calls []string
	err   error
}

func (f *fakeGranter) Grant(_ context.Context, grantee string) (string, error) {
	f.calls = append(f.calls, grantee)
	if f.err != nil {
		return "", f.err
	}
	return "TX" + grantee[len(grantee)-4:], nil
}

func (f *fakeGranter) Address() string { return "yml1sponsor" }

func sponsoring(t *testing.T, daily int) (*server, *fakeGranter) {
	s := testServer(t)
	g := &fakeGranter{}
	s.granter, s.sponsorDaily = g, daily
	return s, g
}

func askSponsor(t *testing.T, s *server, label string) *http.Response {
	k := testKey(label)
	return serve(s, sign(t, request("POST", "/v1/sponsor", nil), k, "sponsor", addrOf(k), testNow.Unix())).Result()
}

func TestAnAccountIsSponsoredOnceAndOnlyItself(t *testing.T) {
	s, g := sponsoring(t, 10)
	first := askSponsor(t, s, "owner")
	require.Equal(t, http.StatusOK, first.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(first.Body).Decode(&body))
	require.Equal(t, true, body["granted"])
	require.Equal(t, "yml1sponsor", body["granter"])

	again := askSponsor(t, s, "owner")
	require.NoError(t, json.NewDecoder(again.Body).Decode(&body))
	require.Equal(t, true, body["already"])
	require.Equal(t, []string{addrOf(testKey("owner"))}, g.calls)
}

func TestSponsorshipNeedsTheAccountsOwnSignature(t *testing.T) {
	s, g := sponsoring(t, 10)
	k := testKey("owner")
	// Signed over somebody else's address.
	w := serve(s, sign(t, request("POST", "/v1/sponsor", nil), k, "sponsor", addrOf(testKey("other")), testNow.Unix()))
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Empty(t, g.calls)
}

func TestTheDailyCeilingCountsOnlyTheLastDay(t *testing.T) {
	s, _ := sponsoring(t, 2)
	old, _ := json.Marshal([]sponsorRecord{{Address: "yml1old", At: testNow.Add(-25 * time.Hour).Unix()}})
	require.NoError(t, os.WriteFile(filepath.Join(s.dir, "sponsored.json"), old, 0o600))
	require.Equal(t, http.StatusOK, askSponsor(t, s, "a").StatusCode)
	require.Equal(t, http.StatusOK, askSponsor(t, s, "b").StatusCode)
	require.Equal(t, http.StatusTooManyRequests, askSponsor(t, s, "c").StatusCode)
}

func TestTheLedgerSurvivesARestart(t *testing.T) {
	s, _ := sponsoring(t, 10)
	require.Equal(t, http.StatusOK, askSponsor(t, s, "owner").StatusCode)
	restarted := &server{dir: s.dir, chainID: testChain, chain: s.chain, now: s.now, granter: &fakeGranter{}, sponsorDaily: 10}
	var body map[string]any
	require.NoError(t, json.NewDecoder(askSponsor(t, restarted, "owner").Body).Decode(&body))
	require.Equal(t, true, body["already"])
}

func TestAnExistingAllowanceCountsAsDone(t *testing.T) {
	s, g := sponsoring(t, 10)
	g.err = errAlreadyGranted
	require.Equal(t, http.StatusOK, askSponsor(t, s, "owner").StatusCode)
	g.err = errors.New("node down")
	require.Equal(t, http.StatusBadGateway, askSponsor(t, s, "another").StatusCode)
}

func TestSponsoringOffAnswers503(t *testing.T) {
	s := testServer(t)
	require.Equal(t, http.StatusServiceUnavailable, askSponsor(t, s, "owner").StatusCode)
	require.Equal(t, http.StatusServiceUnavailable, serve(s, request("GET", "/v1/sponsor", nil)).Code)
}

func TestTheGrantIsLimitedToOwnerMessages(t *testing.T) {
	g := cliGranter{
		bin: "blockchaind", from: "loyalty-sponsor", home: "/h", keyring: "test", node: "tcp://n:1",
		chainID: "c-1", spendLimit: "2000000uyml", fees: "0uyml", lifetime: time.Hour,
		now: func() time.Time { return testNow },
	}
	require.Equal(t, []string{
		"tx", "feegrant", "grant", "loyalty-sponsor", "yml1owner",
		"--spend-limit", "2000000uyml",
		"--expiration", testNow.Add(time.Hour).UTC().Format(time.RFC3339),
		"--allowed-messages", strings.Join(ownerMessages, ","),
		"--chain-id", "c-1", "--keyring-backend", "test", "--home", "/h", "--node", "tcp://n:1",
		"--fees", "0uyml", "--gas", "250000", "-y", "--output", "json",
	}, g.grantArgs("yml1owner"))
	require.Len(t, ownerMessages, 8)
}

func TestTheCLIGranterWaitsForTheBlock(t *testing.T) {
	script := func(outputs ...string) func(context.Context, string, ...string) ([]byte, error) {
		i := 0
		return func(context.Context, string, ...string) ([]byte, error) {
			out := outputs[i]
			i++
			if strings.HasPrefix(out, "ERR:") {
				return []byte(out[4:]), errors.New("exit 1")
			}
			return []byte(out), nil
		}
	}
	base := cliGranter{now: func() time.Time { return testNow }}

	ok := base
	ok.run = script(`{"code":0,"txhash":"ABC"}`, `ERR:tx (ABC) not found`, `{"code":0}`)
	hash, err := ok.Grant(context.Background(), "yml1owner")
	require.NoError(t, err)
	require.Equal(t, "ABC", hash)

	failed := base
	failed.run = script(`{"code":0,"txhash":"ABC"}`, `{"code":5,"raw_log":"insufficient funds"}`)
	_, err = failed.Grant(context.Background(), "yml1owner")
	require.ErrorContains(t, err, "insufficient funds")

	exists := base
	exists.run = script(`{"code":18,"txhash":"","raw_log":"fee allowance already exists"}`)
	_, err = exists.Grant(context.Background(), "yml1owner")
	require.ErrorIs(t, err, errAlreadyGranted)
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./tools/loyaltyapi -count=1`
Expected: FAIL to compile, with `s.granter undefined`, `undefined: cliGranter` and so on.

- [ ] **Step 3: Add the fields and routes**

In `tools/loyaltyapi/main.go`, extend `server`:

```go
type server struct {
	dir     string
	chainID string
	chain   Chain
	now     func() time.Time

	granter       Granter // nil: sponsoring is off
	sponsorDaily  int
	operatorToken string // empty: the application list is off
	mu            sync.Mutex
}
```

Add `"sync"` to the imports. Add these routes in `routes()`:

```go
	mux.HandleFunc("GET /v1/sponsor", s.handleSponsorInfo)
	mux.HandleFunc("POST /v1/sponsor", s.handleSponsor)
	mux.HandleFunc("POST /v1/applications", s.handleApply)
	mux.HandleFunc("GET /v1/applications", s.handleApplications)
```

Add these flags after `chainID`:

```go
	bin := flag.String("bin", "blockchaind", "chain binary that signs the fee grants")
	from := flag.String("from", "", "key name of the sponsor account; empty turns sponsoring off")
	home := flag.String("home", "", "home of the chain binary, holding the keyring")
	keyring := flag.String("keyring-backend", "test", "keyring backend")
	node := flag.String("node", "tcp://127.0.0.1:26957", "CometBFT RPC the chain binary broadcasts to")
	spend := flag.String("sponsor", "2000000uyml", "fee allowance granted to each account")
	lifetime := flag.Duration("sponsor-for", 365*24*time.Hour, "how long an allowance lasts")
	fees := flag.String("fees", "0uyml", "fee the sponsor pays per grant")
	daily := flag.Int("sponsor-daily", 200, "most accounts sponsored in any 24 hours")
	tokenFile := flag.String("operator-token-file", "", "file holding the bearer token that lists applications; empty turns the list off")
```

After `flag.Parse()`, extend the `MkdirAll` loop to `"files", "profiles", "applications"`, and then:

```go
	if *from != "" {
		g, err := newCLIGranter(*bin, *from, *home, *keyring, *node, *chainID, *spend, *fees, *lifetime)
		if err != nil {
			log.Fatalf("sponsor key %q: %v", *from, err)
		}
		s.granter = g
		s.sponsorDaily = *daily
		log.Printf("sponsoring fees from %s, at most %d accounts a day", g.Address(), *daily)
	}
	if *tokenFile != "" {
		raw, err := os.ReadFile(*tokenFile)
		if err != nil || len(strings.TrimSpace(string(raw))) < 32 {
			log.Fatalf("operator token file %s must hold at least 32 characters", *tokenFile)
		}
		s.operatorToken = strings.TrimSpace(string(raw))
	}
```

Initialise `s` before this block. Change `s := &server{…}` to come right after `flag.Parse()`.

- [ ] **Step 4: Write `sponsor.go`**

```go
package main

// Owners never hold coins. Each account is granted a fee allowance once,
// limited to the messages an owner sends, so an account cannot spend the
// platform's money on anything else.
//
// The grant is signed by the chain binary, as tools/faucet does it: a key in
// a keyring the binary already knows how to protect, rather than a second
// signing implementation in this process. This is the only key loyalty-api
// uses, and it can only grant fees from its own balance.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ownerMessages = []string{
	"/blockchain.jewel.v1.MsgOfferJewel", "/blockchain.jewel.v1.MsgCancelOffer",
	"/blockchain.jewel.v1.MsgAcceptOffer", "/blockchain.jewel.v1.MsgClaimOffer",
	"/blockchain.jewel.v1.MsgReportStolen", "/blockchain.jewel.v1.MsgClearStolen",
	"/blockchain.jewel.v1.MsgVetoReassignment", "/blockchain.jewel.v1.MsgCompleteReassignment",
}

var errAlreadyGranted = errors.New("this account already has a fee allowance from the sponsor")

type Granter interface {
	// Grant gives grantee an allowance limited to ownerMessages and returns
	// the transaction hash once a block has committed it.
	Grant(ctx context.Context, grantee string) (string, error)
	// Address is the account that pays.
	Address() string
}

type sponsorRecord struct {
	Address string `json:"address"`
	Tx      string `json:"tx"`
	At      int64  `json:"at"`
}

func (s *server) ledgerPath() string { return filepath.Join(s.dir, "sponsored.json") }

func (s *server) readLedger() ([]sponsorRecord, error) {
	raw, err := os.ReadFile(s.ledgerPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []sponsorRecord
	return recs, json.Unmarshal(raw, &recs)
}

func (s *server) handleSponsorInfo(w http.ResponseWriter, _ *http.Request) {
	if s.granter == nil {
		refuse(w, http.StatusServiceUnavailable, "fees are not sponsored here")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"granter": s.granter.Address()})
}

func (s *server) handleSponsor(w http.ResponseWriter, r *http.Request) {
	if s.granter == nil {
		refuse(w, http.StatusServiceUnavailable, "fees are not sponsored here")
		return
	}
	address, err := s.signer(r, "sponsor", func(a string) string { return a })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}

	// One at a time: the ledger check and the grant must not interleave, or
	// two quick requests both pass the "not yet sponsored" check.
	s.mu.Lock()
	defer s.mu.Unlock()

	ledger, err := s.readLedger()
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	since := s.now().Add(-24 * time.Hour).Unix()
	today := 0
	for _, rec := range ledger {
		if rec.Address == address {
			writeJSON(w, http.StatusOK, map[string]any{"granted": false, "already": true, "granter": s.granter.Address()})
			return
		}
		if rec.At >= since {
			today++
		}
	}
	if today >= s.sponsorDaily {
		w.Header().Set("Retry-After", "3600")
		refuse(w, http.StatusTooManyRequests, "today's sponsorships are used up; try again in an hour")
		return
	}

	tx, err := s.granter.Grant(r.Context(), address)
	if err != nil && !errors.Is(err, errAlreadyGranted) {
		refuse(w, http.StatusBadGateway, "the grant did not go through: "+err.Error())
		return
	}
	ledger = append(ledger, sponsorRecord{Address: address, Tx: tx, At: s.now().Unix()})
	out, _ := json.MarshalIndent(ledger, "", " ")
	if err := writeAtomically(s.ledgerPath(), out); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"granted": true, "tx": tx, "granter": s.granter.Address()})
}

type cliGranter struct {
	bin, from, address, home, keyring, node, chainID, spendLimit, fees string
	lifetime time.Duration
	now      func() time.Time
	run      func(ctx context.Context, name string, args ...string) ([]byte, error)
	poll     time.Duration
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func newCLIGranter(bin, from, home, keyring, node, chainID, spend, fees string, lifetime time.Duration) (*cliGranter, error) {
	g := &cliGranter{bin: bin, from: from, home: home, keyring: keyring, node: node, chainID: chainID,
		spendLimit: spend, fees: fees, lifetime: lifetime, now: time.Now, run: runCommand, poll: time.Second}
	args := []string{"keys", "show", from, "-a", "--keyring-backend", keyring}
	if home != "" {
		args = append(args, "--home", home)
	}
	out, err := g.run(context.Background(), bin, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	g.address = strings.TrimSpace(string(out))
	return g, nil
}

func (g *cliGranter) Address() string { return g.address }

func (g cliGranter) grantArgs(grantee string) []string {
	return []string{
		"tx", "feegrant", "grant", g.from, grantee,
		"--spend-limit", g.spendLimit,
		"--expiration", g.now().Add(g.lifetime).UTC().Format(time.RFC3339),
		"--allowed-messages", strings.Join(ownerMessages, ","),
		"--chain-id", g.chainID, "--keyring-backend", g.keyring, "--home", g.home, "--node", g.node,
		"--fees", g.fees, "--gas", "250000", "-y", "--output", "json",
	}
}

// Grant broadcasts, then waits for the block. A code of 0 from the broadcast
// is only CheckTx: the grant can still fail when the block runs it.
func (g cliGranter) Grant(ctx context.Context, grantee string) (string, error) {
	out, err := g.run(ctx, g.bin, g.grantArgs(grantee)...)
	var res struct {
		Code   int    `json:"code"`
		TxHash string `json:"txhash"`
		RawLog string `json:"raw_log"`
	}
	if jerr := json.Unmarshal(out, &res); jerr != nil {
		if err == nil {
			err = jerr
		}
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	if res.Code != 0 {
		if strings.Contains(res.RawLog, "fee allowance already exists") {
			return "", errAlreadyGranted
		}
		return "", errors.New(res.RawLog)
	}
	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(g.poll):
		}
		q, qerr := g.run(ctx, g.bin, "query", "tx", res.TxHash, "--node", g.node, "--output", "json")
		if qerr != nil {
			continue // not indexed yet
		}
		var done struct {
			Code   int    `json:"code"`
			RawLog string `json:"raw_log"`
		}
		if json.Unmarshal(q, &done) != nil {
			continue
		}
		if done.Code == 0 {
			return res.TxHash, nil
		}
		if strings.Contains(done.RawLog, "fee allowance already exists") {
			return res.TxHash, errAlreadyGranted
		}
		return "", errors.New(done.RawLog)
	}
	return "", fmt.Errorf("transaction %s was not committed in 30 polls", res.TxHash)
}
```

- [ ] **Step 5: Run the sponsorship tests**

Run: `go test ./tools/loyaltyapi -run 'Sponsor|Ceiling|Ledger|Allowance|Grant|CLI' -count=1`
Expected: PASS. `poll` is zero in the tests, so they do not sleep.

- [ ] **Step 6: Write the failing application tests**

`tools/loyaltyapi/applications_test.go`:

```go
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func applicationBody(business string) []byte {
	return []byte(`{"business":"` + business + `","name":"Maison Diop","contact":"Awa Diop",` +
		`"email":"awa@example.sn","phone":"+221 77 000 00 00","country":"SN","message":"Gold and silver since 1987."}`)
}

func TestAnApplicationFromItsBusinessAccountIsSpooled(t *testing.T) {
	s := testServer(t)
	s.operatorToken = strings.Repeat("t", 32)
	k := testKey("applicant")
	body := applicationBody(addrOf(k))
	w := serve(s, sign(t, request("POST", "/v1/applications", body), k, "application", hashHex(body), testNow.Unix()))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	entries, err := os.ReadDir(filepath.Join(s.dir, "applications"))
	require.NoError(t, err)
	require.Len(t, entries, 1)

	r := request("GET", "/v1/applications", nil)
	r.Header.Set("Authorization", "Bearer "+s.operatorToken)
	list := serve(s, r)
	require.Equal(t, http.StatusOK, list.Code)
	var got []struct {
		Application struct{ Name string } `json:"application"`
	}
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &got))
	require.Equal(t, "Maison Diop", got[0].Application.Name)
}

func TestAnApplicationIsSignedByTheBusinessItNames(t *testing.T) {
	s := testServer(t)
	k := testKey("applicant")
	body := applicationBody(addrOf(testKey("somebody-else")))
	require.Equal(t, http.StatusForbidden, serve(s, sign(t, request("POST", "/v1/applications", body), k, "application", hashHex(body), testNow.Unix())).Code)
}

func TestAnApplicationIsChecked(t *testing.T) {
	s := testServer(t)
	k := testKey("applicant")
	a := addrOf(k)
	for _, body := range []string{
		strings.Replace(string(applicationBody(a)), `"SN"`, `"Senegal"`, 1),
		strings.Replace(string(applicationBody(a)), `awa@example.sn`, `awa`, 1),
		strings.Replace(string(applicationBody(a)), `"name":"Maison Diop"`, `"name":""`, 1),
		strings.Replace(string(applicationBody(a)), `}`, `,"extra":1}`, 1),
	} {
		b := []byte(body)
		require.Equal(t, http.StatusBadRequest, serve(s, sign(t, request("POST", "/v1/applications", b), k, "application", hashHex(b), testNow.Unix())).Code, body)
	}
}

func TestTheListNeedsTheOperatorToken(t *testing.T) {
	s := testServer(t)
	require.Equal(t, http.StatusNotFound, serve(s, request("GET", "/v1/applications", nil)).Code)
	s.operatorToken = strings.Repeat("t", 32)
	require.Equal(t, http.StatusUnauthorized, serve(s, request("GET", "/v1/applications", nil)).Code)
	r := request("GET", "/v1/applications", nil)
	r.Header.Set("Authorization", "Bearer wrong")
	require.Equal(t, http.StatusUnauthorized, serve(s, r).Code)
}
```

- [ ] **Step 7: Write `applications.go`**

```go
package main

// A business applies to be admitted as a jeweler. It applies from the account
// that will become its business address, and signs the application with it,
// so the office admits the key it saw, and not one typed into a form.
//
// Applications are files in a spool, as tools/notify's notices are. The
// office reads them with the operator token and admits by an x/group proposal;
// nothing here admits anybody.

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type application struct {
	Business string `json:"business"`
	Name     string `json:"name"`
	Contact  string `json:"contact"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Country  string `json:"country"`
	Message  string `json:"message"`
}

type spooled struct {
	ReceivedAt  int64       `json:"received_at"`
	Application application `json:"application"`
}

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

func (a application) check() error {
	switch {
	case len(strings.TrimSpace(a.Name)) < 2 || len(a.Name) > 120:
		return fmt.Errorf("the business name must be 2 to 120 characters")
	case len(strings.TrimSpace(a.Contact)) < 2 || len(a.Contact) > 120:
		return fmt.Errorf("the contact's name must be 2 to 120 characters")
	case !strings.Contains(a.Email, "@") || len(a.Email) > 254:
		return fmt.Errorf("the email address does not look like one")
	case len(a.Phone) > 40:
		return fmt.Errorf("the phone number is too long")
	case !countryPattern.MatchString(a.Country):
		return fmt.Errorf("the country is its two-letter ISO code, in capitals")
	case len(a.Message) > 2000:
		return fmt.Errorf("the message is at most 2,000 characters")
	}
	return nil
}

func (s *server) handleApply(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil {
		refuse(w, http.StatusRequestEntityTooLarge, "an application is at most 16 KiB")
		return
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	address, err := s.signer(r, "application", func(string) string { return hash })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	var a application
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		refuse(w, http.StatusBadRequest, "unreadable application: "+err.Error())
		return
	}
	if err := a.check(); err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}
	if a.Business != address {
		refuse(w, http.StatusForbidden, "apply from the account that will be the business's own")
		return
	}
	out, _ := json.MarshalIndent(spooled{ReceivedAt: s.now().Unix(), Application: a}, "", " ")
	name := fmt.Sprintf("%d-%s.json", s.now().Unix(), hash[:12])
	spool := filepath.Join(s.dir, "applications")
	if err := os.MkdirAll(spool, 0o750); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := writeAtomically(filepath.Join(spool, name), out); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"received": true})
}

func (s *server) handleApplications(w http.ResponseWriter, r *http.Request) {
	if s.operatorToken == "" {
		refuse(w, http.StatusNotFound, "not available here")
		return
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.operatorToken)) != 1 {
		refuse(w, http.StatusUnauthorized, "the operator token is required")
		return
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, "applications"))
	if err != nil && !os.IsNotExist(err) {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	list := []spooled{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.dir, "applications", e.Name()))
		if err != nil {
			continue
		}
		var sp spooled
		if json.Unmarshal(raw, &sp) == nil {
			list = append(list, sp)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ReceivedAt > list[j].ReceivedAt })
	writeJSON(w, http.StatusOK, list)
}
```

- [ ] **Step 8: Run all the API tests and vet**

Run: `go test ./tools/loyaltyapi -count=1 && go vet ./tools/loyaltyapi`
Expected: PASS; vet silent.

- [ ] **Step 9: Share the seeder's helpers**

Move these out of `scripts/devnet/seed-jewel.sh`, unchanged, into a new `scripts/devnet/jewel-lib.sh`:
- the functions `say`, `ok`, `bad`, `addr`, `json`, `ensure_key`, `privhex`, `committed`, `run`, `outcome`, `expect_refused`, `broadcast_json`, `run_json`, `expect_refused_json` and `as_office`;
- the two `OFFICE=` lines between them.

Start the file with:

```bash
# Helpers shared by the jewel scripts on the local devnet.
#
# Sourced, never run. The caller sets BIN, HOME_DIR, KEYRING, TX, Q and
# M1..M3 first, exactly as seed-jewel.sh does; FAILED is set to 0 here.
FAILED=0
```

In `seed-jewel.sh`, put `. "$(dirname "$0")/jewel-lib.sh"` where the functions were, right after `TX=` and `Q=` are set.

Run the seeder on a fresh devnet, as in "Running it locally". Expected: the same output as before the move, every stage `ok`.

- [ ] **Step 10: Write `scripts/devnet/loyalty-admit.sh`**

```bash
#!/usr/bin/env bash
#
# Admits a jeweler created in the loyalty app: the office's 3-of-5 vote, as in
# seed-jewel.sh stage 1, then 50 YML to the business so it can pay its staff's
# fees.
#
#   BIN=$SCRATCH/blockchaind.exe HOME_DIR=.devnet/local/node CHAIN=yamale-local-1 \
#   NODE=tcp://127.0.0.1:26957 scripts/devnet/loyalty-admit.sh "Maison Test" yml1…
set -euo pipefail
BIN=${BIN:?the chain binary}
HOME_DIR=${HOME_DIR:?the node home}
CHAIN=${CHAIN:?the chain id}
NODE=${NODE:?the node RPC}
KEYRING=${KEYRING:-test}
M1=${M1:-custodian1}; M2=${M2:-custodian2}; M3=${M3:-custodian3}
NAME=${1:?the jeweler's name}
BUSINESS=${2:?the business account code, from the app's Account tab}
TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
Q="--home $HOME_DIR --node $NODE -o json"
. "$(dirname "$0")/jewel-lib.sh"

run bank send "$(addr "$M3")" "$BUSINESS" 50000000uyml --from "$M3" >/dev/null \
  && ok "funded the business" || { bad "funding"; exit 1; }
as_office "{\"@type\":\"/blockchain.jewel.v1.MsgAdmitJeweler\",\"office\":\"$OFFICE\",\"name\":\"$NAME\",\"business\":\"$BUSINESS\"}" \
  && ok "admitted $NAME" || { bad "admission"; exit 1; }
"$BIN" query jewel jewelers $Q | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4
```

- [ ] **Step 11: Write `scripts/devnet/loyalty-local.sh`**

```bash
#!/usr/bin/env bash
#
# Runs loyalty-api against the local devnet: a sponsor key funded from a
# custodian, an operator token for the application list, and the data under
# .devnet/local/loyalty. Runs in the foreground; stop it with Ctrl-C.
#
#   BIN=$SCRATCH/blockchaind.exe API_BIN=$SCRATCH/loyaltyapi.exe scripts/devnet/loyalty-local.sh
set -euo pipefail
BIN=${BIN:?the chain binary}
API_BIN=${API_BIN:?the loyaltyapi binary}
HOME_DIR=${HOME_DIR:-.devnet/local/node}
CHAIN=${CHAIN:-yamale-local-1}
NODE=${NODE:-tcp://127.0.0.1:26957}
REST=${REST:-http://127.0.0.1:1517}
KEYRING=${KEYRING:-test}
M1=${M1:-custodian1}; M2=${M2:-custodian2}; M3=${M3:-custodian3}
DATA=${DATA:-.devnet/local/loyalty}
TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
Q="--home $HOME_DIR --node $NODE -o json"
. "$(dirname "$0")/jewel-lib.sh"

mkdir -p "$DATA"
ensure_key loyalty-sponsor
SPONSOR=$(addr loyalty-sponsor)
balance=$("$BIN" query bank balance "$SPONSOR" uyml $Q | json amount)
if [ "${balance:-0}" = "0" ]; then
  run bank send "$(addr "$M3")" "$SPONSOR" 1000000000uyml --from "$M3" >/dev/null && ok "funded the sponsor" || { bad "funding the sponsor"; exit 1; }
fi
[ -s "$DATA/operator-token" ] || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' > "$DATA/operator-token"

say "loyalty-api on 127.0.0.1:8095, sponsor $SPONSOR"
exec "$API_BIN" --listen 127.0.0.1:8095 --dir "$DATA" --rest "$REST" --chain-id "$CHAIN" \
  --bin "$BIN" --from loyalty-sponsor --home "$HOME_DIR" --keyring-backend "$KEYRING" --node "$NODE" \
  --operator-token-file "$DATA/operator-token"
```

`.devnet/` is git-ignored, so the token never reaches a commit. Check with `git check-ignore -v .devnet/local/loyalty/operator-token`.

- [ ] **Step 12: Check it against the running chain**

The unit tests fake the chain. This step runs the real thing once.

1. Build the binaries, run `init-local-devnet.sh`, then run `seed-jewel.sh` to prove Step 9's move changed nothing. All of this is in "Running it locally".
2. Start `BIN=$SCRATCH/blockchaind.exe API_BIN=$SCRATCH/loyaltyapi.exe scripts/devnet/loyalty-local.sh` in the background.
3. Run these and check the answers:
   - `curl -s http://127.0.0.1:8095/v1/health` answers `{"ok":true}`.
   - `curl -s http://127.0.0.1:8095/v1/sponsor` answers with the address that `blockchaind keys show loyalty-sponsor -a --keyring-backend test --home .devnet/local/node` prints.
   - `curl -s http://127.0.0.1:8095/v1/files/$(printf x | sha256sum | cut -d' ' -f1)` answers 404.
4. Make a key and admit it as a jeweler:
   - `blockchaind keys add jw-test-biz --keyring-backend test --home .devnet/local/node`
   - `scripts/devnet/loyalty-admit.sh "Maison Test" <that address>`
   - The script prints the new jeweler's id, and `blockchaind query jewel jeweler <id>` shows it `JEWELER_ACTIVE`.
5. Stop loyalty-api.

Git Bash rewrites `/c/…` paths into `C:/…` when it passes them to a Windows program, so `--bin` reaches loyaltyapi as a path Windows can open. If a grant later fails with "file not found", pass `--bin` as a `C:/…` path explicitly.

The real fee grant, signed through the CLI, is exercised by the smoke test in Task 11, which can sign the sponsorship request that a shell cannot.

- [ ] **Step 13: Commit**

```bash
git add tools/loyaltyapi scripts/devnet/jewel-lib.sh scripts/devnet/loyalty-admit.sh scripts/devnet/loyalty-local.sh scripts/devnet/seed-jewel.sh
git commit -m "loyaltyapi: one fee grant per account for owner messages only, a signed application spool, and the scripts that run it locally"
```

---

### Task 6: The app's shell, and Verify, the face anyone can use

**Files:**
- Modify: `clients/package.json` (workspaces), `.claude/launch.json` (create if absent)
- Create: `clients/loyalty/package.json`, `tsconfig.json`, `vite.config.ts`, `index.html`
- Create: `clients/loyalty/public/manifest.webmanifest`, copied from `clients/app/public/`: `mark.png`, `icon-192.png`, `icon-512.png`
- Create: `clients/loyalty/src/vite-env.d.ts`, `config.ts`, `main.tsx`, `App.tsx`, `ui.tsx`, `api.ts`, `styles.css`
- Create: `clients/loyalty/src/verify/record.ts`, `record.test.ts`, `history.ts`, `history.test.ts`, `Photos.tsx`, `Search.tsx`, `JewelPage.tsx`, `Proof.tsx`
- Modify: `clients/sdk/src/catalogues.ts` (the `loyalty.*` keys below, in all five locales)

**Interfaces:**
- Consumes, from `@yamale/chain` (Tasks 2–4):
  - `readJewel`, `readJeweler`, `findByInscription`, `chainHead`, `isActive`
  - `normaliseInscription`, `verifyOwnershipProof`, `sha256Hex`
  - `formatDate`, `t`, `plural`, `useLocale`, `LanguagePicker`, `registerAll`, `AVAILABLE`, `resolveLocale`, `setLocale`
- Produces, for Tasks 7–11:

  ```ts
  // config.ts
  export const CHAIN_ID: string; export const PUBLIC_ORIGIN: string; export const API: string;
  export const RPC_ENDPOINT: string; export const GAS_PRICE: number;
  export const SERVICES: { custodianUrl: string; recoveryUrl: string };
  export function recordUri(hash: string): string
  // ui.tsx
  export function useLoad<T>(load: () => Promise<T>, deps: unknown[]): [T | null, () => void]
  export function AppBar(props: { title: string; eyebrow: string; back?: boolean }): JSX.Element
  export function Problem(props: { outcome: { reason: string; detail: string } }): JSX.Element
  export function Loading(): JSX.Element
  export function NotFound(): JSX.Element
  export function Section(props: { title: string; children: ReactNode }): JSX.Element
  export function ThemeButton(): JSX.Element
  export function day(unix: number | string): string
  // api.ts
  export type Fetched = { ok: true; bytes: Uint8Array; type: string } | { ok: false; reason: 'missing' | 'mismatch' | 'unreachable' }
  export function fetchVerified(hash: string): Promise<Fetched>
  export interface JewelerProfile { logo: string; colour: string; accent: string }
  export function fetchProfile(jewelerId: string): Promise<JewelerProfile | null>
  // verify/record.ts
  export interface Stone { kind: string; carat: string; count: number }
  export interface JewelRecord { version: 1; jewelerId: string; inscription: string; title: string; metal: string;
    weightGrams: string; stones: Stone[]; origin: { workshop: string; stoneSource: string; labCertificate: string };
    photos: string[]; createdAt: string }
  export function encodeRecord(r: JewelRecord): Uint8Array
  export function parseRecord(bytes: Uint8Array): JewelRecord | null
  export type RecordState = { state: 'loading' } | { state: 'ok'; record: JewelRecord }
    | { state: 'missing' | 'mismatch' | 'unreachable' | 'unreadable' }
  export function useRecord(hash: string | null): RecordState
  export function currentRecordHash(jewel: Jewel): string        // the latest amendment's, else the original
  // verify/history.ts
  export interface OwnerStep { kind: 'stock' | 'owner'; ordinal: number; at: number }
  export function ownershipSteps(j: { registeredAt: string; ownerSince: string[] }): OwnerStep[]
  export type BenefitState = { state: 'active'; left: number; resetsAt: number | null; expiresAt: number | null }
    | { state: 'expired'; expiredAt: number } | { state: 'withdrawn' }
  export function benefitState(b: { allowance: number; period: string; expiresAt: string; uses: string[] },
                               issuerActive: boolean, now: number): BenefitState
  // verify/Photos.tsx
  export function VerifiedImage(props: { hash: string; alt: string; className?: string }): JSX.Element
  export function Gallery(props: { hashes: string[]; alt: string }): JSX.Element
  // verify/JewelPage.tsx
  export function Hallmark(props: { code: string }): JSX.Element
  export function RecordFacts(props: { record: JewelRecord }): JSX.Element
  export function BenefitList(props: { benefits: Benefit[]; issuerActive: boolean; now: number }): JSX.Element
  ```

- [ ] **Step 1: Write the failing tests for the record and the history**

`clients/loyalty/src/verify/record.test.ts`:

```ts
import assert from 'node:assert/strict';
import { test } from 'node:test';

import { encodeRecord, parseRecord, type JewelRecord } from './record.ts';

const HASH = 'a'.repeat(64);
const ring: JewelRecord = {
  version: 1, jewelerId: '1', inscription: 'MD20260001', title: 'Solitaire ring', metal: '18k yellow gold',
  weightGrams: '4.2', stones: [{ kind: 'diamond', carat: '0.50', count: 1 }],
  origin: { workshop: 'Dakar, Médina', stoneSource: 'Botswana', labCertificate: 'GIA 2141438167' },
  photos: [HASH], createdAt: '2026-11-13T10:00:00.000Z',
};

test('a record survives encoding, and the same record always encodes the same', () => {
  assert.deepEqual(parseRecord(encodeRecord(ring)), ring);
  const shuffled = { createdAt: ring.createdAt, photos: ring.photos, ...ring };
  assert.deepEqual(encodeRecord(shuffled), encodeRecord(ring));
});

test('a record that is not version 1, or names a bad photo, is not a record', () => {
  const enc = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));
  assert.equal(parseRecord(enc({ ...ring, version: 2 })), null);
  assert.equal(parseRecord(enc({ ...ring, photos: ['A'.repeat(64)] })), null);
  assert.equal(parseRecord(enc({ ...ring, photos: [] })), null);
  assert.equal(parseRecord(enc({ ...ring, stones: [{ kind: 'ruby', carat: '1', count: 0 }] })), null);
  assert.equal(parseRecord(new TextEncoder().encode('not json')), null);
});

test('fields nobody asked for do not survive parsing', () => {
  const extra = new TextEncoder().encode(JSON.stringify({ ...ring, owner: 'Awa Diop' }));
  assert.equal('owner' in (parseRecord(extra) as object), false);
});
```

`clients/loyalty/src/verify/history.test.ts`:

```ts
import assert from 'node:assert/strict';
import { test } from 'node:test';

import { benefitState, ownershipSteps } from './history.ts';

test('the history is the stock date, then one date per owner', () => {
  assert.deepEqual(ownershipSteps({ registeredAt: '100', ownerSince: ['200', '300'] }), [
    { kind: 'stock', ordinal: 0, at: 100 },
    { kind: 'owner', ordinal: 1, at: 200 },
    { kind: 'owner', ordinal: 2, at: 300 },
  ]);
});

test('a periodic benefit counts only the uses inside its window, as the chain does', () => {
  const b = { allowance: 2, period: '1000', expiresAt: '0', uses: ['100', '1500'] };
  // At 2000 the window is (1000, 2000]: only the use at 1500 counts.
  assert.deepEqual(benefitState(b, true, 2000), { state: 'active', left: 1, resetsAt: null, expiresAt: null });
  // At 1099 both count and the next one frees at 100 + 1000.
  assert.deepEqual(benefitState({ ...b, uses: ['100', '1050'] }, true, 1099),
    { state: 'active', left: 0, resetsAt: 1100, expiresAt: null });
});

test('a lifetime benefit counts every use, expiry and suspension end it', () => {
  const b = { allowance: 3, period: '0', expiresAt: '5000', uses: ['1', '2'] };
  assert.deepEqual(benefitState(b, true, 10), { state: 'active', left: 1, resetsAt: null, expiresAt: 5000 });
  assert.deepEqual(benefitState(b, true, 5000), { state: 'expired', expiredAt: 5000 });
  assert.deepEqual(benefitState(b, false, 10), { state: 'withdrawn' });
});
```

- [ ] **Step 2: Scaffold the workspace**

Add `"loyalty"` to the `workspaces` array in `clients/package.json`, after `"herd"`.

`clients/loyalty/package.json`:

```json
{
  "name": "@yamale/loyalty",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "description": "Jewels that carry their own record: verify one, hold one and pass it on, and, for a jeweler, register and service them.",
  "scripts": {
    "dev": "vite",
    "build": "tsc --noEmit && vite build",
    "preview": "vite preview",
    "typecheck": "tsc --noEmit",
    "test": "node --experimental-strip-types --test src/*/*.test.ts"
  },
  "dependencies": {
    "@cosmjs/crypto": "^0.32.4",
    "@cosmjs/encoding": "^0.32.4",
    "@cosmjs/proto-signing": "^0.32.4",
    "@noble/curves": "^1.8.1",
    "@noble/hashes": "^1.7.1",
    "@yamale/chain": "*",
    "qrcode-generator": "^2.0.4",
    "react": "^19.0.0",
    "react-dom": "^19.0.0",
    "react-router-dom": "^7.1.0"
  },
  "devDependencies": {
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^4.3.4",
    "typescript": "^5.7.0",
    "vite": "^6.0.0"
  }
}
```

`clients/loyalty/tsconfig.json`: copy `clients/herd/tsconfig.json` unchanged.

`clients/loyalty/vite.config.ts`:

```ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

/**
 * The jewel app, served at the root of its own host (loyalty.yamaletech.com),
 * so `base` is '/' and the certificate card's path /j/<jeweler>/<jewel> is a
 * real path, not a fragment: the fragment is reserved for claim keys.
 *
 * Everything the app talks to is proxied under /api, as it will be in
 * production, so the browser never needs CORS and the claim key in a
 * fragment never meets a cross-origin request.
 */
const strip = (prefix: string) => (path: string) => path.replace(new RegExp(`^${prefix}`), '');

export default defineConfig({
  base: '/',
  plugins: [react()],
  server: {
    port: 5390,
    proxy: {
      '/api/rpc': { target: process.env.YAMALE_RPC ?? 'http://127.0.0.1:26957', changeOrigin: true, rewrite: strip('/api/rpc') },
      '/api/loyalty': { target: process.env.LOYALTY_API ?? 'http://127.0.0.1:8095', changeOrigin: true, rewrite: strip('/api/loyalty') },
      '/api/custodian': { target: process.env.YAMALE_CUSTODIAN ?? 'http://127.0.0.1:8099', changeOrigin: true, rewrite: strip('/api/custodian') },
      '/api/recovery': { target: process.env.YAMALE_RECOVERY ?? 'http://127.0.0.1:8100', changeOrigin: true, rewrite: strip('/api/recovery') },
    },
  },
});
```

`clients/loyalty/index.html`: copy `clients/herd/index.html` and change only:
- the title to `Yamale Jewels`;
- the description to `Check a jewel's record, hold it, and pass it on.`;
- the icon `href` to `/mark.png`;
- add `<link rel="manifest" href="/manifest.webmanifest" />` and `<link rel="apple-touch-icon" href="/icon-192.png" />`.

`clients/loyalty/public/manifest.webmanifest`:

```json
{
  "name": "Yamale Jewels",
  "short_name": "Jewels",
  "start_url": "/",
  "display": "standalone",
  "background_color": "#f6f7f9",
  "theme_color": "#12253f",
  "icons": [
    { "src": "/icon-192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/icon-512.png", "sizes": "512x512", "type": "image/png" }
  ]
}
```

Copy `mark.png`, `icon-192.png` and `icon-512.png` from `clients/app/public/`.

In `.claude/launch.json`, add this entry to `configurations`. Create the file with `"version": "0.0.1"` if it does not exist:

```json
{ "name": "loyalty", "runtimeExecutable": "npm", "runtimeArgs": ["--prefix", "clients/loyalty", "run", "dev"], "port": 5390 }
```

Run: `cd clients && npm install`. Expected: no errors. The workspace links `@yamale/loyalty`.

- [ ] **Step 3: Run the tests and watch them fail**

Run: `cd clients && npm test --workspace @yamale/loyalty`
Expected: FAIL, `Cannot find module './record.ts'` and `'./history.ts'`.

- [ ] **Step 4: Write `record.ts` and `history.ts`**

`clients/loyalty/src/verify/record.ts`:

```ts
/**
 * The record file: what a jeweler registered about a jewel, stored by
 * loyalty-api under its SHA-256, which the chain holds as record_hash.
 *
 * It never names an owner. Owners change on the chain; the record describes
 * the jewel.
 */

import { useEffect, useState } from 'react';

import type { Jewel } from '../../../sdk/src/generated/blockchain/jewel/v1/jewel.ts';
import { fetchVerified } from '../api.ts';

export interface Stone { kind: string; carat: string; count: number }

export interface JewelRecord {
  version: 1;
  jewelerId: string;
  inscription: string;
  title: string;
  metal: string;
  weightGrams: string;
  stones: Stone[];
  origin: { workshop: string; stoneSource: string; labCertificate: string };
  photos: string[];
  createdAt: string;
}

const HASH = /^[0-9a-f]{64}$/;
const ID = /^[1-9][0-9]*$/;
const str = (v: unknown, max: number): v is string => typeof v === 'string' && v.length <= max;

/** Keys in one fixed order, so one record always has one fingerprint. */
export function encodeRecord(r: JewelRecord): Uint8Array {
  return new TextEncoder().encode(JSON.stringify({
    version: 1,
    jewelerId: r.jewelerId,
    inscription: r.inscription,
    title: r.title,
    metal: r.metal,
    weightGrams: r.weightGrams,
    stones: r.stones.map((s) => ({ kind: s.kind, carat: s.carat, count: s.count })),
    origin: { workshop: r.origin.workshop, stoneSource: r.origin.stoneSource, labCertificate: r.origin.labCertificate },
    photos: [...r.photos],
    createdAt: r.createdAt,
  }));
}

export function parseRecord(bytes: Uint8Array): JewelRecord | null {
  let v: any;
  try { v = JSON.parse(new TextDecoder().decode(bytes)); } catch { return null; }
  if (!v || v.version !== 1 || !str(v.jewelerId, 20) || !ID.test(v.jewelerId)) return null;
  if (!str(v.inscription, 40) || !str(v.title, 120) || !str(v.metal, 80) || !str(v.weightGrams, 12) || !str(v.createdAt, 40)) return null;
  if (!Array.isArray(v.stones) || v.stones.length > 50) return null;
  for (const s of v.stones) {
    if (!s || !str(s.kind, 60) || !str(s.carat, 12) || !Number.isInteger(s.count) || s.count < 1 || s.count > 9999) return null;
  }
  const o = v.origin;
  if (!o || !str(o.workshop, 120) || !str(o.stoneSource, 120) || !str(o.labCertificate, 80)) return null;
  if (!Array.isArray(v.photos) || v.photos.length < 1 || v.photos.length > 12) return null;
  if (!v.photos.every((p: unknown) => typeof p === 'string' && HASH.test(p))) return null;
  return {
    version: 1, jewelerId: v.jewelerId, inscription: v.inscription, title: v.title, metal: v.metal,
    weightGrams: v.weightGrams,
    stones: v.stones.map((s: Stone) => ({ kind: s.kind, carat: s.carat, count: s.count })),
    origin: { workshop: o.workshop, stoneSource: o.stoneSource, labCertificate: o.labCertificate },
    photos: [...v.photos], createdAt: v.createdAt,
  };
}

/** A correction replaces the record shown first; the original stays readable. */
export function currentRecordHash(jewel: Pick<Jewel, 'recordHash' | 'amendments'>): string {
  const last = jewel.amendments[jewel.amendments.length - 1];
  return last?.recordHash || jewel.recordHash;
}

export type RecordState =
  | { state: 'loading' }
  | { state: 'ok'; record: JewelRecord }
  | { state: 'missing' | 'mismatch' | 'unreachable' | 'unreadable' };

export function useRecord(hash: string | null): RecordState {
  const [state, setState] = useState<RecordState>({ state: 'loading' });
  useEffect(() => {
    if (!hash) return;
    let live = true;
    setState({ state: 'loading' });
    fetchVerified(hash).then((f) => {
      if (!live) return;
      if (!f.ok) { setState({ state: f.reason }); return; }
      const record = parseRecord(f.bytes);
      setState(record ? { state: 'ok', record } : { state: 'unreadable' });
    });
    return () => { live = false; };
  }, [hash]);
  return state;
}
```

The Node test imports only `encodeRecord` and `parseRecord`. It still loads `record.ts`, which imports `react` (a plain package Node can load) and `../api.ts`, which must itself import nothing `.tsx`. In `api.ts`, import from `../../sdk/src/loyaltyauth.ts` by relative path, never from `@yamale/chain`, for exactly that reason.

`clients/loyalty/src/verify/history.ts`:

```ts
/**
 * What the public page says about a jewel's owners and benefits: dates and
 * counts, never names. Pure, so the arithmetic is tested against the chain's.
 */

export interface OwnerStep { kind: 'stock' | 'owner'; ordinal: number; at: number }

export function ownershipSteps(j: { registeredAt: string; ownerSince: string[] }): OwnerStep[] {
  return [
    { kind: 'stock', ordinal: 0, at: Number(j.registeredAt) },
    ...j.ownerSince.map((t, i) => ({ kind: 'owner' as const, ordinal: i + 1, at: Number(t) })),
  ];
}

export type BenefitState =
  | { state: 'active'; left: number; resetsAt: number | null; expiresAt: number | null }
  | { state: 'expired'; expiredAt: number }
  | { state: 'withdrawn' };

/**
 * Counts uses exactly as RedeemBenefit in x/jewel/keeper/msg_server_benefit.go
 * does: with a period, a use counts while u > now - period; without one,
 * every use counts. A suspended issuer's benefits are shown as withdrawn,
 * because nobody will honour them.
 */
export function benefitState(
  b: { allowance: number; period: string; expiresAt: string; uses: string[] },
  issuerActive: boolean, now: number,
): BenefitState {
  if (!issuerActive) return { state: 'withdrawn' };
  const expiresAt = Number(b.expiresAt);
  if (expiresAt > 0 && now >= expiresAt) return { state: 'expired', expiredAt: expiresAt };
  const period = Number(b.period);
  const counted = b.uses.map(Number).filter((u) => period === 0 || u > now - period);
  const left = Math.max(0, b.allowance - counted.length);
  const resetsAt = left === 0 && period > 0 && counted.length > 0 ? Math.min(...counted) + period : null;
  return { state: 'active', left, resetsAt, expiresAt: expiresAt > 0 ? expiresAt : null };
}
```

Before relying on `benefitState`, read the expiry check at the top of `RedeemBenefit` in `x/jewel/keeper/msg_server_benefit.go`. If the chain treats `expires_at == 0` differently, or compares with `>` instead of `>=`, change this function and its test to match, and say so in the report.

- [ ] **Step 5: Run the tests**

Run: `cd clients && npm test --workspace @yamale/loyalty`
Expected: PASS, 6 tests.

- [ ] **Step 6: Write the configuration, the API reads and the shared UI**

`clients/loyalty/src/vite-env.d.ts`:

```ts
/// <reference types="vite/client" />
interface ImportMetaEnv {
  readonly VITE_CHAIN_ID?: string;
  readonly VITE_PUBLIC_ORIGIN?: string;
  readonly VITE_GAS_PRICE?: string;
}
interface ImportMeta { readonly env: ImportMetaEnv }
```

`clients/loyalty/src/config.ts`:

```ts
/** Where this app finds the chain, its API and the account services. */
export const CHAIN_ID: string = import.meta.env.VITE_CHAIN_ID ?? 'yamale-local-1';

/** The origin printed into links and record locations: the public one in production. */
export const PUBLIC_ORIGIN: string = (import.meta.env.VITE_PUBLIC_ORIGIN ?? window.location.origin).replace(/\/+$/, '');

export const API = '/api/loyalty';

/** CosmJS needs an absolute URL for its RPC client; reads use the relative proxy. */
export const RPC_ENDPOINT = `${window.location.origin}/api/rpc`;

export const GAS_PRICE = Number(import.meta.env.VITE_GAS_PRICE ?? '0');

export const SERVICES = { custodianUrl: '/api/custodian', recoveryUrl: '/api/recovery' };

export function recordUri(hash: string): string {
  return `${PUBLIC_ORIGIN}${API}/v1/files/${hash}`;
}
```

`clients/loyalty/src/api.ts`:

```ts
/**
 * loyalty-api, as the app uses it. Every file is checked against its
 * fingerprint before anything is shown: a photo the API serves that does not
 * hash to what the chain recorded is reported as a mismatch, never displayed.
 */

import { sha256Hex } from '../../sdk/src/loyaltyauth.ts';

// Not imported from config.ts: that module reads `window` when it loads, and
// record.ts, which Node's test runner loads, imports this one.
const API = '/api/loyalty';

export type Fetched =
  | { ok: true; bytes: Uint8Array; type: string }
  | { ok: false; reason: 'missing' | 'mismatch' | 'unreachable' };

export async function fetchVerified(hash: string): Promise<Fetched> {
  let res: Response;
  try {
    res = await fetch(`${API}/v1/files/${hash}`);
  } catch {
    return { ok: false, reason: 'unreachable' };
  }
  if (res.status === 404) return { ok: false, reason: 'missing' };
  if (!res.ok) return { ok: false, reason: 'unreachable' };
  const bytes = new Uint8Array(await res.arrayBuffer());
  if (sha256Hex(bytes) !== hash) return { ok: false, reason: 'mismatch' };
  return { ok: true, bytes, type: res.headers.get('Content-Type') ?? '' };
}

export interface JewelerProfile { logo: string; colour: string; accent: string }

export async function fetchProfile(jewelerId: string): Promise<JewelerProfile | null> {
  try {
    const res = await fetch(`${API}/v1/jewelers/${jewelerId}/profile`);
    if (!res.ok) return null;
    return (await res.json()) as JewelerProfile;
  } catch {
    return null;
  }
}
```

`clients/loyalty/src/ui.tsx`:

```tsx
import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { formatDate, t } from '@yamale/chain';

/** Load once per change of deps; the second element reloads on demand. */
export function useLoad<T>(load: () => Promise<T>, deps: unknown[]): [T | null, () => void] {
  const [value, setValue] = useState<T | null>(null);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    let live = true;
    setValue(null);
    load().then((v) => { if (live) setValue(v); });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, tick]);
  return [value, useCallback(() => setTick((n) => n + 1), [])];
}

export function day(unix: number | string): string {
  return formatDate(new Date(Number(unix) * 1000));
}

type Theme = 'light' | 'dark' | 'system';
const THEME_KEY = 'yamale.loyalty.theme';

function savedTheme(): Theme {
  try {
    const v = localStorage.getItem(THEME_KEY);
    return v === 'light' || v === 'dark' ? v : 'system';
  } catch { return 'system'; }
}

function applyTheme(theme: Theme) {
  try {
    if (theme === 'system') {
      document.documentElement.removeAttribute('data-theme');
      localStorage.removeItem(THEME_KEY);
    } else {
      document.documentElement.setAttribute('data-theme', theme);
      localStorage.setItem(THEME_KEY, theme);
    }
  } catch { /* private mode: the choice lasts this page only */ }
}

export function ThemeButton() {
  const [theme, setTheme] = useState<Theme>(savedTheme);
  const next: Theme = theme === 'dark' ? 'light' : theme === 'light' ? 'system' : 'dark';
  const label = theme === 'dark' ? t('loyalty.themeDark') : theme === 'light' ? t('loyalty.themeLight') : t('loyalty.themeSystem');
  return (
    <button type="button" className="appbar__btn" aria-label={`${t('loyalty.theme')}: ${label}`}
            onClick={() => { setTheme(next); applyTheme(next); }}>
      <span aria-hidden="true">{theme === 'dark' ? '◐' : theme === 'light' ? '○' : '◑'}</span>
    </button>
  );
}

export function AppBar({ title, eyebrow, back }: { title: string; eyebrow: string; back?: boolean }) {
  const navigate = useNavigate();
  return (
    <header className="y-appbar">
      {back ? (
        <button type="button" className="appbar__btn" onClick={() => navigate(-1)} aria-label={t('loyalty.back')}>
          <svg viewBox="0 0 20 20" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.8"
               strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M12 4 6 10l6 6" /></svg>
        </button>
      ) : (
        <img className="y-appbar__mark" src="/mark.png" alt="" width={28} height={28} />
      )}
      <div className="appbar__mid">
        <div className="y-appbar__eyebrow">{eyebrow}</div>
        <div className="y-appbar__name">{title}</div>
      </div>
      <div className="y-appbar__right"><ThemeButton /></div>
    </header>
  );
}

export function Problem({ outcome }: { outcome: { reason: string; detail: string } }) {
  const key = outcome.reason === 'not-found' ? 'loyalty.problemNotFound'
    : outcome.reason === 'offline' ? 'loyalty.problemOffline'
    : outcome.reason === 'refused' ? 'loyalty.problemRefused' : 'loyalty.problemUnreachable';
  return (
    <div className="note note--warn" role="status">
      <p>{t(key)}</p>
      <details className="advanced"><summary>{t('loyalty.advanced')}</summary><pre>{outcome.detail}</pre></details>
    </div>
  );
}

export function Loading() {
  return <p className="loading" role="status">{t('loyalty.loading')}</p>;
}

export function NotFound() {
  return (
    <>
      <AppBar title={t('loyalty.title')} eyebrow="" back />
      <main className="y-screen"><p className="note">{t('loyalty.notFoundPage')}</p></main>
    </>
  );
}

export function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="block">
      <h2 className="block__title">{title}</h2>
      {children}
    </section>
  );
}
```

- [ ] **Step 7: Write the photos, with a loupe**

`clients/loyalty/src/verify/Photos.tsx`:

```tsx
/**
 * Photos are fetched as bytes, checked against their fingerprint, and only
 * then shown, from a blob URL. An <img src> pointed at the API would display
 * whatever the API served, which is the one thing this page must not trust.
 *
 * The loupe exists for one job: comparing the engraving on the jewel in hand
 * with the engraving in the photo. So it magnifies to 4x and pans by
 * scrolling, and nothing else.
 */

import { useEffect, useRef, useState } from 'react';
import { t } from '@yamale/chain';

import { fetchVerified } from '../api.ts';

type Img = { state: 'loading' } | { state: 'ok'; url: string } | { state: 'failed'; reason: string };

function useVerifiedImage(hash: string): Img {
  const [img, setImg] = useState<Img>({ state: 'loading' });
  useEffect(() => {
    let live = true;
    let url = '';
    fetchVerified(hash).then((f) => {
      if (!live) return;
      if (!f.ok) { setImg({ state: 'failed', reason: f.reason }); return; }
      url = URL.createObjectURL(new Blob([f.bytes], { type: f.type }));
      setImg({ state: 'ok', url });
    });
    return () => { live = false; if (url) URL.revokeObjectURL(url); };
  }, [hash]);
  return img;
}

export function VerifiedImage({ hash, alt, className }: { hash: string; alt: string; className?: string }) {
  const img = useVerifiedImage(hash);
  if (img.state === 'ok') return <img className={className} src={img.url} alt={alt} />;
  return (
    <div className={`${className ?? ''} photo--absent`} role="img" aria-label={alt}>
      {img.state === 'loading' ? '' : img.reason === 'mismatch' ? t('loyalty.photoMismatch') : t('loyalty.photoUnavailable')}
    </div>
  );
}

export function Gallery({ hashes, alt }: { hashes: string[]; alt: string }) {
  const [index, setIndex] = useState(0);
  const [loupe, setLoupe] = useState(false);
  const current = hashes[Math.min(index, hashes.length - 1)]!;
  return (
    <div className="gallery">
      <button type="button" className="gallery__main" onClick={() => setLoupe(true)}
              aria-label={`${t('loyalty.loupe')}: ${t('loyalty.photoN', { n: index + 1, count: hashes.length })}`}>
        <VerifiedImage hash={current} alt={alt} className="gallery__img" />
        <span className="gallery__hint">{t('loyalty.loupe')}</span>
      </button>
      {hashes.length > 1 && (
        <div className="gallery__thumbs" role="list">
          {hashes.map((h, i) => (
            <button key={h} type="button" role="listitem" aria-current={i === index}
                    className={`gallery__thumb${i === index ? ' is-on' : ''}`} onClick={() => setIndex(i)}
                    aria-label={t('loyalty.photoN', { n: i + 1, count: hashes.length })}>
              <VerifiedImage hash={h} alt="" className="gallery__thumbimg" />
            </button>
          ))}
        </div>
      )}
      {loupe && <Loupe hash={current} alt={alt} onClose={() => setLoupe(false)} />}
    </div>
  );
}

function Loupe({ hash, alt, onClose }: { hash: string; alt: string; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [zoom, setZoom] = useState(2);
  useEffect(() => { dialog.current?.showModal(); }, []);
  return (
    <dialog ref={dialog} className="loupe" onClose={onClose} aria-label={t('loyalty.loupe')}>
      <div className="loupe__view">
        <div style={{ width: `${zoom * 100}%` }}><VerifiedImage hash={hash} alt={alt} className="loupe__img" /></div>
      </div>
      <div className="loupe__bar">
        <label className="loupe__zoom">
          <span>{t('loyalty.zoom')}</span>
          <input type="range" min={1} max={4} step={0.5} value={zoom} onChange={(e) => setZoom(Number(e.target.value))} />
          <span className="num">{zoom}×</span>
        </label>
        <button type="button" className="btn" onClick={() => dialog.current?.close()}>{t('loyalty.close')}</button>
      </div>
    </dialog>
  );
}
```

- [ ] **Step 8: Write the search screen**

`clients/loyalty/src/verify/Search.tsx`:

```tsx
import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { findByInscription, normaliseInscription, readJeweler, t, type Jewel } from '@yamale/chain';

import { AppBar, Problem } from '../ui.tsx';

type State =
  | { kind: 'idle' } | { kind: 'busy' }
  | { kind: 'failed'; outcome: { reason: string; detail: string } }
  | { kind: 'results'; items: { jewel: Jewel; jewelerName: string }[] };

export function Search() {
  const [text, setText] = useState('');
  const [state, setState] = useState<State>({ kind: 'idle' });
  const navigate = useNavigate();
  const code = normaliseInscription(text);

  async function search(e: FormEvent) {
    e.preventDefault();
    if (!code) return;
    setState({ kind: 'busy' });
    const out = await findByInscription(code);
    if (!out.ok) { setState({ kind: 'failed', outcome: out }); return; }
    const items = await Promise.all(out.value.map(async (jewel) => {
      const jr = await readJeweler(jewel.jewelerId);
      return { jewel, jewelerName: jr.ok ? jr.value.name : '' };
    }));
    if (items.length === 1) { navigate(`/j/${items[0]!.jewel.jewelerId}/${items[0]!.jewel.id}`); return; }
    setState({ kind: 'results', items });
  }

  return (
    <>
      <AppBar title={t('loyalty.verifyTitle')} eyebrow={t('loyalty.title')} />
      <main className="y-screen">
        <section className="hero">
          <h1 className="hero__title">{t('loyalty.verifyHeadline')}</h1>
          <p className="hero__lede">{t('loyalty.verifyLede')}</p>
        </section>
        <form className="stack" onSubmit={search}>
          <label className="y-label" htmlFor="code">{t('loyalty.codeLabel')}</label>
          <input id="code" className="bigfield inscription" value={text} onChange={(e) => setText(e.target.value)}
                 autoCapitalize="characters" autoComplete="off" spellCheck={false} inputMode="text" />
          {text !== '' && (
            <p className="hint" aria-live="polite">
              {code ? t('loyalty.codeReadsAs', { code }) : t('loyalty.codeInvalid')}
            </p>
          )}
          <button className="btn btn--primary" disabled={!code || state.kind === 'busy'}>{t('loyalty.verifyButton')}</button>
        </form>
        {state.kind === 'failed' && <Problem outcome={state.outcome} />}
        {state.kind === 'results' && (state.items.length === 0
          ? <p className="note">{t('loyalty.noMatch')}</p>
          : (
            <>
              <p className="hint">{t('loyalty.sameCodeHint')}</p>
              <ul className="y-cards">
                {state.items.map(({ jewel, jewelerName }) => (
                  <li key={jewel.id} className="y-card">
                    <Link className="card-link" to={`/j/${jewel.jewelerId}/${jewel.id}`}>
                      <span className="inscription">{jewel.inscription}</span>
                      <span>{jewelerName || t('loyalty.jewelerN', { id: jewel.jewelerId })}</span>
                    </Link>
                  </li>
                ))}
              </ul>
            </>
          ))}
      </main>
    </>
  );
}
```

- [ ] **Step 9: Write the jewel page**

`clients/loyalty/src/verify/JewelPage.tsx`:

```tsx
/**
 * The public page of one jewel: what it is, where it came from, how many
 * owners it has had and when each change happened, and what it is still
 * owed. Never who owned it.
 *
 * The jeweler's own colours and logo frame the page, because the certificate
 * card in the customer's hand carries them. The register's facts below are
 * read from the chain directly, so they stand when loyalty-api does not.
 */

import { type CSSProperties, type ReactNode } from 'react';
import { useParams } from 'react-router-dom';
import {
  chainHead, isActive, plural, readJewel, readJeweler, t, type Benefit, type Jewel, type JewelView,
} from '@yamale/chain';

import { fetchProfile } from '../api.ts';
import { AppBar, day, Loading, NotFound, Problem, Section, useLoad } from '../ui.tsx';
import { benefitState, ownershipSteps } from './history.ts';
import { Gallery, VerifiedImage } from './Photos.tsx';
import { currentRecordHash, useRecord, type JewelRecord } from './record.ts';

export function Hallmark({ code }: { code: string }) {
  return <span className="hallmark" aria-label={code}>{code}</span>;
}

export function RecordFacts({ record }: { record: JewelRecord }) {
  return (
    <dl className="facts">
      <dt>{t('loyalty.metal')}</dt><dd>{record.metal}</dd>
      <dt>{t('loyalty.weight')}</dt><dd className="num">{t('loyalty.grams', { n: record.weightGrams })}</dd>
      {record.stones.length > 0 && (<>
        <dt>{t('loyalty.stones')}</dt>
        <dd>{record.stones.map((s, i) => (
          <div key={i}>{t('loyalty.stoneLine', { count: s.count, kind: s.kind, carat: s.carat })}</div>
        ))}</dd>
      </>)}
      <dt>{t('loyalty.workshop')}</dt><dd>{record.origin.workshop}</dd>
      {record.origin.stoneSource && (<><dt>{t('loyalty.stoneSource')}</dt><dd>{record.origin.stoneSource}</dd></>)}
      {record.origin.labCertificate && (<><dt>{t('loyalty.labCertificate')}</dt><dd>{record.origin.labCertificate}</dd></>)}
    </dl>
  );
}

const KNOWN_KINDS = ['cleaning', 'resizing', 'polishing', 'rhodium', 'repair'];

export function benefitKind(kind: string): string {
  return KNOWN_KINDS.includes(kind) ? t(`loyalty.benefitKind.${kind}`) : kind;
}

export function BenefitList({ benefits, issuerActive, now }: { benefits: Benefit[]; issuerActive: boolean; now: number }) {
  if (benefits.length === 0) return <p className="hint">{t('loyalty.noBenefits')}</p>;
  return (
    <ul className="y-cards">
      {benefits.map((b) => {
        const s = benefitState(b, issuerActive, now);
        return (
          <li key={b.id} className="y-card benefit">
            <span className="benefit__kind">{benefitKind(b.kind)}</span>
            {s.state === 'active' && (
              <span className="benefit__state">
                <span className="y-chip y-chip--ok num">{t('loyalty.benefitLeft', { left: s.left, allowance: b.allowance })}</span>
                {s.resetsAt && <span className="hint">{t('loyalty.benefitRenews', { date: day(s.resetsAt) })}</span>}
                {s.expiresAt && <span className="hint">{t('loyalty.benefitUntil', { date: day(s.expiresAt) })}</span>}
              </span>
            )}
            {s.state === 'expired' && <span className="y-chip y-chip--mute">{t('loyalty.benefitExpired', { date: day(s.expiredAt) })}</span>}
            {s.state === 'withdrawn' && <span className="y-chip y-chip--warn">{t('loyalty.benefitWithdrawn')}</span>}
          </li>
        );
      })}
    </ul>
  );
}

function Owners({ jewel }: { jewel: Jewel }) {
  const steps = ownershipSteps(jewel);
  return (
    <>
      <p className="owners__count">
        {jewel.ownerCount === 0 ? t('loyalty.noOwnerYet') : plural('loyalty.owners', jewel.ownerCount, { count: jewel.ownerCount })}
      </p>
      <ol className="timeline">
        {steps.map((s) => (
          <li key={s.ordinal} className="timeline__step">
            <span>{s.kind === 'stock' ? t('loyalty.stepStock') : t('loyalty.stepOwner', { n: s.ordinal })}</span>
            <time className="num">{day(s.at)}</time>
          </li>
        ))}
      </ol>
      <p className="hint">{t('loyalty.ownersPrivate')}</p>
    </>
  );
}

function Corrections({ jewel }: { jewel: Jewel }) {
  if (jewel.amendments.length === 0) return null;
  return (
    <Section title={t('loyalty.corrections')}>
      <ol className="timeline">
        {jewel.amendments.map((a, i) => (
          <li key={i} className="timeline__step">
            <span>{t('loyalty.correctedOn', { date: day(a.time) })}</span>
            <span className="correction__note">{a.note}</span>
          </li>
        ))}
      </ol>
      <details className="original">
        <summary>{t('loyalty.originalRecord')}</summary>
        <OriginalRecord hash={jewel.recordHash} />
      </details>
    </Section>
  );
}

function OriginalRecord({ hash }: { hash: string }) {
  const r = useRecord(hash);
  return r.state === 'ok' ? <RecordFacts record={r.record} /> : <p className="hint">{t('loyalty.recordMissing')}</p>;
}

/** `actions` lets Task 8 add a signed-in action without forking this page. */
export function JewelPage({ actions }: { actions?: (view: JewelView, now: number) => ReactNode } = {}) {
  const { jeweler: jewelerId = '', jewel: jewelId = '' } = useParams();
  const [view] = useLoad(() => readJewel(jewelId), [jewelId]);
  const [jeweler] = useLoad(() => readJeweler(jewelerId), [jewelerId]);
  const [head] = useLoad(() => chainHead(), []);
  const [profile] = useLoad(() => fetchProfile(jewelerId), [jewelerId]);
  const recordHash = view?.ok ? currentRecordHash(view.value.jewel) : null;
  const record = useRecord(recordHash);

  if (!view || !jeweler) return <><AppBar title={t('loyalty.title')} eyebrow="" back /><main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <><AppBar title={t('loyalty.title')} eyebrow="" back /><main className="y-screen"><Problem outcome={view} /></main></>;
  const { jewel } = view.value;
  if (jewel.jewelerId !== jewelerId) return <NotFound />;

  const now = head?.ok ? head.value.time : Math.floor(Date.now() / 1000);
  const issuer = jeweler.ok ? jeweler.value : null;
  const brand = (profile ? { '--jeweler-colour': profile.colour, '--jeweler-accent': profile.accent } : {}) as CSSProperties;
  const title = record.state === 'ok' ? record.record.title : t('loyalty.jewelN', { id: jewel.id });

  return (
    <>
      <AppBar title={issuer?.name ?? t('loyalty.title')} eyebrow={t('loyalty.title')} back />
      <main className="y-screen jewel" style={brand}>
        <header className="jewel__mast">
          {profile?.logo ? <VerifiedImage hash={profile.logo} alt={issuer?.name ?? ''} className="jewel__logo" /> : null}
          <p className="jewel__by">{t('loyalty.registeredBy', { name: issuer?.name ?? t('loyalty.jewelerN', { id: jewelerId }) })}</p>
          <h1 className="jewel__title">{title}</h1>
          <Hallmark code={jewel.inscription} />
        </header>

        {jewel.stolen && (
          <div className="banner banner--bad" role="alert">
            <strong>{t('loyalty.stolen')}</strong>
            <span>{t('loyalty.stolenNote')}</span>
          </div>
        )}
        {view.value.reassignment && (
          <div className="banner banner--warn" role="status">
            {t('loyalty.reassignment', { date: day(view.value.reassignment.windowEnds) })}
          </div>
        )}

        {record.state === 'ok' && <Gallery hashes={record.record.photos} alt={title} />}
        {record.state === 'mismatch' && <div className="banner banner--bad" role="alert">{t('loyalty.recordMismatch')}</div>}
        {(record.state === 'missing' || record.state === 'unreachable' || record.state === 'unreadable') && (
          <p className="note">{t('loyalty.recordMissing')}</p>
        )}

        <p className="declared">{t('loyalty.declared')}</p>

        {actions?.(view.value, now)}

        {record.state === 'ok' && (<>
          <Section title={t('loyalty.materials')}><RecordFacts record={record.record} /></Section>
        </>)}
        <Section title={t('loyalty.ownersTitle')}><Owners jewel={jewel} /></Section>
        <Section title={t('loyalty.benefits')}>
          <BenefitList benefits={view.value.benefits} issuerActive={issuer ? isActive(issuer) : false} now={now} />
        </Section>
        <Corrections jewel={jewel} />

        <details className="advanced">
          <summary>{t('loyalty.advanced')}</summary>
          <pre>{JSON.stringify({
            chain: head?.ok ? head.value.chainId : null, readAtHeight: view.height,
            classId: `jewel-${jewel.jewelerId}`, tokenId: `jewel-${jewel.id}`,
            owner: view.value.owner, jewel, offer: view.value.offer ?? null,
            reassignment: view.value.reassignment ?? null, benefits: view.value.benefits,
          }, null, 2)}</pre>
        </details>
      </main>
    </>
  );
}
```

- [ ] **Step 10: Write the ownership-proof page**

`clients/loyalty/src/verify/Proof.tsx`:

```tsx
/**
 * /own/<jewel>?t=…&k=…&s=…: a link an owner shares to show they hold a jewel,
 * for an insurer or a buyer. It proves the person who made the link holds the
 * key that holds the jewel now. It names nobody.
 */

import { Link, useLocation, useParams } from 'react-router-dom';
import { readJewel, t, verifyOwnershipProof } from '@yamale/chain';

import { CHAIN_ID } from '../config.ts';
import { AppBar, day, Loading, Problem, useLoad } from '../ui.tsx';
import { Hallmark } from './JewelPage.tsx';

export function Proof() {
  const { jewel: jewelId = '' } = useParams();
  const { search } = useLocation();
  const proof = verifyOwnershipProof(CHAIN_ID, jewelId, search);
  const [view] = useLoad(() => readJewel(jewelId), [jewelId]);

  return (
    <>
      <AppBar title={t('loyalty.proofTitle')} eyebrow={t('loyalty.title')} back />
      <main className="y-screen">
        {!proof && <div className="banner banner--bad" role="alert">{t('loyalty.proofInvalid')}</div>}
        {proof && !view && <Loading />}
        {proof && view && !view.ok && <Problem outcome={view} />}
        {proof && view?.ok && (
          <>
            <Hallmark code={view.value.jewel.inscription} />
            {view.value.owner === proof.address
              ? <div className="banner banner--ok" role="status">{t('loyalty.proofHolds')}</div>
              : <div className="banner banner--warn" role="status">{t('loyalty.proofNoLonger')}</div>}
            <p className="hint">{t('loyalty.proofIssued', { date: day(proof.issuedAt) })}</p>
            <Link className="btn" to={`/j/${view.value.jewel.jewelerId}/${view.value.jewel.id}`}>{t('loyalty.seeJewel')}</Link>
          </>
        )}
      </main>
    </>
  );
}
```

- [ ] **Step 11: Write the shell and the entry point**

`clients/loyalty/src/App.tsx`:

```tsx
import { BrowserRouter, NavLink, Route, Routes } from 'react-router-dom';
import { LanguagePicker, t, useLocale } from '@yamale/chain';

import { NotFound } from './ui.tsx';
import { JewelPage } from './verify/JewelPage.tsx';
import { Proof } from './verify/Proof.tsx';
import { Search } from './verify/Search.tsx';

function TabBar() {
  const tabs = [{ to: '/', end: true, label: t('loyalty.navVerify') }];
  return (
    <nav className="y-tabbar" aria-label={t('loyalty.navLabel')}>
      {tabs.map((tab) => (
        <NavLink key={tab.to} to={tab.to} end={tab.end}
                 className={({ isActive }) => `y-tab${isActive ? ' y-tab--on' : ''}`}>
          <span className="y-tab__label">{tab.label}</span>
        </NavLink>
      ))}
    </nav>
  );
}

export function App() {
  useLocale();
  return (
    <BrowserRouter>
      <div className="y-stage">
        <a className="skip" href="#main">{t('loyalty.skip')}</a>
        <header className="y-stage__bar">
          <a className="stage__brand" href="/"><img src="/mark.png" alt="" width={22} height={22} /><span>Yamale</span></a>
          <span className="y-stage__title">{t('loyalty.title')}</span>
          <LanguagePicker className="stage__lang" />
        </header>
        <div className="y-phone">
          <div className="y-phone__notch" />
          <div className="y-phone__screen" id="main" tabIndex={-1}>
            <Routes>
              <Route path="/" element={<Search />} />
              <Route path="/j/:jeweler/:jewel" element={<JewelPage />} />
              <Route path="/own/:jewel" element={<Proof />} />
              <Route path="*" element={<NotFound />} />
            </Routes>
            <TabBar />
          </div>
        </div>
      </div>
    </BrowserRouter>
  );
}
```

`clients/loyalty/src/main.tsx`: copy `clients/herd/src/main.tsx` and change two things:
- the theme storage key to `'yamale.loyalty.theme'`;
- the stylesheet import to `'./styles.css'` from this directory.

Keep `../../shared/yamale.css` imported first.

- [ ] **Step 12: Write the stylesheet**

`clients/loyalty/src/styles.css` holds only what this surface adds to the shared shell. It is styled through the shell's tokens, never with colour literals, so both themes hold:

```css
/* Yamale Jewels — on top of clients/shared/yamale.css.
 *
 * One idea carries the visual identity: the engraved code is shown as a
 * hallmark, a stamped mark in a double brass border, because that is what a
 * jeweler's mark on metal looks like and it is the thing a person compares
 * against the jewel in their hand. Everything else stays quiet.
 *
 * A jeweler's own colours arrive as --jeweler-colour and --jeweler-accent on
 * the jewel page only, and fall back to the shell's accent and brass.
 */

.appbar__btn { inline-size: 40px; block-size: 40px; display: grid; place-items: center;
  border: 0; background: transparent; color: inherit; border-radius: var(--radius); cursor: pointer; }
.appbar__btn:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
.appbar__mid { flex: 1; min-inline-size: 0; }

.hero { padding-block: var(--sp-4) var(--sp-3); }
.hero__title { font-family: var(--font-display); font-size: 1.6rem; line-height: 1.15; margin: 0;
  text-wrap: balance; }
.hero__lede { color: var(--text-muted); margin-block: var(--sp-2) 0; }

.stack { display: grid; gap: var(--sp-2); }
.bigfield { font-size: 1.25rem; padding: 0.7rem 0.8rem; border: 1px solid var(--rule);
  border-radius: var(--radius); background: var(--surface); color: var(--text); inline-size: 100%; }
.bigfield:focus-visible { outline: 2px solid var(--focus); outline-offset: 1px; }
.hint { color: var(--text-muted); font-size: 0.88rem; margin: 0; }
.note { padding: var(--sp-2) var(--sp-3); border-radius: var(--radius); background: var(--wash-mute); }
.note--warn { background: var(--wash-warn); color: var(--warn-ink); }
.loading { color: var(--text-muted); }

.btn { display: inline-flex; align-items: center; justify-content: center; gap: 0.4rem;
  min-block-size: 44px; padding: 0 1rem; border-radius: var(--radius); border: 1px solid var(--rule);
  background: var(--surface); color: var(--text); font: inherit; font-weight: 600; cursor: pointer;
  text-decoration: none; }
.btn--primary { background: var(--accent); color: var(--accent-ink); border-color: var(--accent); }
.btn--danger { color: var(--bad); border-color: var(--bad); }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
.actions { display: flex; flex-wrap: wrap; gap: var(--sp-2); }

.inscription, .hallmark { font-family: var(--font-mono); letter-spacing: 0.14em;
  word-break: keep-all; overflow-wrap: normal; }
.hallmark { display: inline-block; padding: 0.35rem 0.7rem; font-size: 1.05rem; font-weight: 500;
  color: var(--brass-ink); border: 3px double var(--brass); border-radius: 2px; background: var(--wash-brass); }

.jewel__mast { border-block-start: 4px solid var(--jeweler-colour, var(--accent));
  padding-block: var(--sp-3); display: grid; gap: var(--sp-2); justify-items: start; }
.jewel__logo { max-block-size: 40px; inline-size: auto; }
.jewel__by { margin: 0; font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.08em;
  color: var(--jeweler-accent, var(--brass-ink)); }
.jewel__title { font-family: var(--font-display); font-size: 1.7rem; line-height: 1.1; margin: 0; text-wrap: balance; }
.declared { font-size: 0.9rem; color: var(--text-muted); border-inline-start: 3px solid var(--rule);
  padding-inline-start: var(--sp-2); }

.banner { display: grid; gap: 0.2rem; padding: var(--sp-2) var(--sp-3); border-radius: var(--radius); }
.banner--bad { background: var(--wash-bad); color: var(--bad); }
.banner--warn { background: var(--wash-warn); color: var(--warn-ink); }
.banner--ok { background: var(--wash-ok); color: var(--ok); }

.gallery { display: grid; gap: var(--sp-2); }
.gallery__main { position: relative; padding: 0; border: 0; background: var(--wash-mute);
  border-radius: var(--radius-lg); overflow: hidden; cursor: zoom-in; }
.gallery__img { display: block; inline-size: 100%; aspect-ratio: 1; object-fit: cover; }
.gallery__hint { position: absolute; inset-inline-end: var(--sp-2); inset-block-end: var(--sp-2);
  background: var(--surface); color: var(--text); padding: 0.2rem 0.5rem; border-radius: var(--radius); font-size: 0.8rem; }
.gallery__thumbs { display: flex; gap: var(--sp-1); overflow-x: auto; }
.gallery__thumb { padding: 0; border: 2px solid transparent; border-radius: var(--radius); background: none; cursor: pointer; }
.gallery__thumb.is-on { border-color: var(--brass); }
.gallery__thumbimg { display: block; inline-size: 56px; block-size: 56px; object-fit: cover; border-radius: 3px; }
.photo--absent { display: grid; place-items: center; min-block-size: 8rem; color: var(--text-muted);
  background: var(--wash-mute); font-size: 0.85rem; text-align: center; padding: var(--sp-2); }

.loupe { inline-size: min(100vw, 900px); max-inline-size: 100vw; block-size: 100dvh; max-block-size: 100dvh;
  margin: auto; padding: 0; border: 0; background: var(--bg); color: var(--text); }
.loupe::backdrop { background: rgb(0 0 0 / 0.7); }
.loupe__view { block-size: calc(100% - 64px); overflow: auto; }
.loupe__img { display: block; inline-size: 100%; block-size: auto; }
.loupe__bar { block-size: 64px; display: flex; align-items: center; justify-content: space-between;
  gap: var(--sp-2); padding-inline: var(--sp-3); border-block-start: 1px solid var(--rule); }
.loupe__zoom { display: flex; align-items: center; gap: var(--sp-2); }

.block { padding-block: var(--sp-3); border-block-start: 1px solid var(--rule); }
.block__title { font-family: var(--font-display); font-size: 1rem; margin: 0 0 var(--sp-2); }
.facts { display: grid; grid-template-columns: max-content 1fr; gap: 0.35rem var(--sp-3); margin: 0; }
.facts dt { color: var(--text-muted); }
.facts dd { margin: 0; }
.timeline { list-style: none; margin: 0; padding: 0; display: grid; gap: 0.3rem; }
.timeline__step { display: flex; justify-content: space-between; gap: var(--sp-2);
  padding-block: 0.3rem; border-block-end: 1px dashed var(--rule); }
.owners__count { font-weight: 600; margin: 0 0 var(--sp-2); }
.benefit { display: flex; justify-content: space-between; align-items: center; gap: var(--sp-2); flex-wrap: wrap; }
.benefit__state { display: flex; gap: var(--sp-1); align-items: center; flex-wrap: wrap; }
.card-link { display: flex; justify-content: space-between; gap: var(--sp-2); color: inherit; text-decoration: none; }

.advanced { margin-block: var(--sp-3); }
.advanced summary { cursor: pointer; color: var(--text-muted); }
.advanced pre { overflow-x: auto; font-size: 0.75rem; background: var(--wash-mute);
  padding: var(--sp-2); border-radius: var(--radius); }

.num { font-variant-numeric: tabular-nums; }

@media (prefers-reduced-motion: reduce) { * { scroll-behavior: auto !important; transition: none !important; } }
```

The shell may name its spacing tokens differently from `--sp-1` to `--sp-4`. Before writing this file, check the names in `clients/shared/yamale.css` and use its real ones.

- [ ] **Step 13: Add the catalogue keys**

Add these to `en` and `fr` in `clients/sdk/src/catalogues.ts`, and the same keys to `pt`, `sw` and `ar`. Plural families (`loyalty.owners.*`) carry each CLDR category the locale uses: `en` one/other; `fr` and `pt` one/many/other; `sw` one/other; `ar` zero/one/two/few/many/other.

| key | en | fr |
|---|---|---|
| loyalty.title | Yamale Jewels | Bijoux Yamale |
| loyalty.skip | Skip to content | Aller au contenu |
| loyalty.navLabel | Main sections | Sections principales |
| loyalty.navVerify | Verify | Vérifier |
| loyalty.back | Back | Retour |
| loyalty.theme | Theme | Thème |
| loyalty.themeDark | Dark | Sombre |
| loyalty.themeLight | Light | Clair |
| loyalty.themeSystem | Match this device | Comme l'appareil |
| loyalty.verifyTitle | Verify a jewel | Vérifier un bijou |
| loyalty.verifyHeadline | Is this the jewel its papers describe? | Ce bijou est-il celui que décrivent ses papiers ? |
| loyalty.verifyLede | Type the code engraved on the jewel, or open the link on its certificate card. You do not need an account. | Saisissez le code gravé sur le bijou, ou ouvrez le lien de sa carte de certificat. Aucun compte n'est nécessaire. |
| loyalty.codeLabel | Engraved code | Code gravé |
| loyalty.codeReadsAs | Read as {code} | Lu comme {code} |
| loyalty.codeInvalid | An engraved code has 4 to 40 letters and digits. Spaces and - . / _ are ignored. | Un code gravé compte de 4 à 40 lettres et chiffres. Les espaces et - . / _ sont ignorés. |
| loyalty.verifyButton | Look it up | Rechercher |
| loyalty.noMatch | No jeweler on the register has recorded this code. | Aucun joaillier du registre n'a enregistré ce code. |
| loyalty.sameCodeHint | More than one jeweler uses this code. Each match shows who registered it. | Plusieurs joailliers utilisent ce code. Chaque résultat indique qui l'a enregistré. |
| loyalty.jewelerN | Jeweler {id} | Joaillier {id} |
| loyalty.jewelN | Jewel no. {id} | Bijou n° {id} |
| loyalty.problemNotFound | The register has no such jewel. | Le registre ne contient pas ce bijou. |
| loyalty.problemUnreachable | The register did not answer. Try again in a moment. | Le registre n'a pas répondu. Réessayez dans un instant. |
| loyalty.problemOffline | This device is offline. | Cet appareil est hors ligne. |
| loyalty.problemRefused | The register refused the question. | Le registre a refusé la demande. |
| loyalty.notFoundPage | There is nothing at this address. | Il n'y a rien à cette adresse. |
| loyalty.loading | Reading the register… | Lecture du registre… |
| loyalty.advanced | Advanced | Avancé |
| loyalty.declared | Changes of owner are declared by the owners; the inscription was checked by the jeweler at registration only. | Les changements de propriétaire sont déclarés par les propriétaires ; l'inscription n'a été vérifiée par le joaillier qu'à l'enregistrement. |
| loyalty.registeredBy | Registered by {name} | Enregistré par {name} |
| loyalty.stolen | Reported stolen by its owner | Déclaré volé par son propriétaire |
| loyalty.stolenNote | If it is offered to you, do not buy it. | Si on vous le propose, ne l'achetez pas. |
| loyalty.reassignment | The platform has proposed to reassign this jewel because its owner lost access. The owner can refuse until {date}. | La plateforme a proposé de réattribuer ce bijou, son propriétaire ayant perdu l'accès. Le propriétaire peut refuser jusqu'au {date}. |
| loyalty.recordMissing | The photos and details are not available right now. The facts below come from the register itself. | Les photos et le détail sont indisponibles pour le moment. Les informations ci-dessous viennent du registre lui-même. |
| loyalty.recordMismatch | The file offered for this jewel does not match the fingerprint in the register. Do not rely on its photos. | Le fichier proposé pour ce bijou ne correspond pas à l'empreinte du registre. Ne vous fiez pas à ses photos. |
| loyalty.photoUnavailable | Photo unavailable | Photo indisponible |
| loyalty.photoMismatch | This photo does not match the register | Cette photo ne correspond pas au registre |
| loyalty.photoN | Photo {n} of {count} | Photo {n} sur {count} |
| loyalty.loupe | Look closer | Regarder de près |
| loyalty.zoom | Magnification | Grossissement |
| loyalty.close | Close | Fermer |
| loyalty.materials | Materials and origin | Matières et origine |
| loyalty.metal | Metal | Métal |
| loyalty.weight | Weight | Poids |
| loyalty.grams | {n} g | {n} g |
| loyalty.stones | Stones | Pierres |
| loyalty.stoneLine | {count} × {kind}, {carat} ct | {count} × {kind}, {carat} ct |
| loyalty.workshop | Made in | Fabriqué à |
| loyalty.stoneSource | Stones from | Provenance des pierres |
| loyalty.labCertificate | Laboratory certificate | Certificat de laboratoire |
| loyalty.ownersTitle | Owners | Propriétaires |
| loyalty.owners.one | {count} owner so far | {count} propriétaire à ce jour |
| loyalty.owners.other | {count} owners so far | {count} propriétaires à ce jour |
| loyalty.noOwnerYet | Still in the jeweler's stock | Encore dans le stock du joaillier |
| loyalty.stepStock | Entered the jeweler's stock | Entré dans le stock du joaillier |
| loyalty.stepOwner | Owner {n} from | Propriétaire {n} depuis |
| loyalty.ownersPrivate | Who the owners were is never shown. | L'identité des propriétaires n'est jamais affichée. |
| loyalty.benefits | Benefits that come with it | Avantages attachés |
| loyalty.noBenefits | None | Aucun |
| loyalty.benefitLeft | {left} of {allowance} left | {left} sur {allowance} restants |
| loyalty.benefitRenews | Available again {date} | De nouveau disponible le {date} |
| loyalty.benefitUntil | Until {date} | Jusqu'au {date} |
| loyalty.benefitExpired | Ended {date} | Terminé le {date} |
| loyalty.benefitWithdrawn | No longer honoured by the issuer | N'est plus honoré par l'émetteur |
| loyalty.benefitKind.cleaning | Cleaning | Nettoyage |
| loyalty.benefitKind.resizing | Resizing | Mise à taille |
| loyalty.benefitKind.polishing | Polishing | Polissage |
| loyalty.benefitKind.rhodium | Rhodium plating | Rhodiage |
| loyalty.benefitKind.repair | Repair | Réparation |
| loyalty.corrections | Corrections by the jeweler | Corrections du joaillier |
| loyalty.correctedOn | Corrected {date} | Corrigé le {date} |
| loyalty.originalRecord | The record as first registered | La fiche telle qu'enregistrée à l'origine |
| loyalty.proofTitle | Proof of ownership | Preuve de propriété |
| loyalty.proofHolds | The person who shared this link holds this jewel now. | La personne qui a partagé ce lien détient ce bijou aujourd'hui. |
| loyalty.proofNoLonger | The person who shared this link no longer holds this jewel. | La personne qui a partagé ce lien ne détient plus ce bijou. |
| loyalty.proofInvalid | This link is not a valid proof. It may have been altered. | Ce lien n'est pas une preuve valide. Il a peut-être été modifié. |
| loyalty.proofIssued | Link made {date} | Lien créé le {date} |
| loyalty.seeJewel | See the jewel | Voir le bijou |

The French plural `loyalty.owners.many` takes the `.other` text.

- [ ] **Step 14: Typecheck, test, build**

Run: `cd clients && npm test --workspace @yamale/chain && npm test --workspace @yamale/loyalty && npm run build --workspace @yamale/loyalty`
Expected: all pass, including the catalogue tests; `vite build` writes `clients/loyalty/dist` with no type errors.

- [ ] **Step 15: Check it against a populated chain**

1. Start the chain and seed it, as in "Running it locally". Leave loyalty-api off for now.
2. Start the app with `preview_start` `{name: "loyalty"}`.
3. Search for `MD-2026-0001`. The seeder's ring opens at `/j/1/1`.
4. Check each of these on that page:
   - the hallmark shows `MD20260001`;
   - "2 owners so far" with three dated steps, because the seeder sold and then resold the ring;
   - the cleaning benefit;
   - the declared sentence;
   - the note that photos are unavailable, since loyalty-api is not running;
   - Advanced holds the raw JSON.
5. Open `/j/2/1`. It must say there is nothing at this address, because the jewel and jeweler do not match.
6. Repeat at 390 × 844 and at 1600 × 1000, in dark and light, using `resize_window`. Read the result with `read_page`, and take one screenshot per width as proof.
7. Fix anything that renders badly before committing.

- [ ] **Step 16: Commit**

```bash
git add clients/package.json clients/package-lock.json clients/loyalty clients/sdk/src/catalogues.ts .claude/launch.json
git commit -m "loyalty: the app's shell and Verify: search by engraved code, the jewel's public page, ownership proofs"
```

---

### Task 7: Accounts on the device, protection, recovery, and fee sponsorship

**Files:**
- Create: `clients/loyalty/src/account/device.ts`, `device.test.ts`, `recover.ts`, `recover.test.ts`, `session.tsx`, `Account.tsx`
- Modify: `clients/loyalty/src/api.ts` (sponsorship), `clients/loyalty/src/App.tsx` (provider, tab, route), `clients/sdk/src/catalogues.ts`

**Interfaces:**
- Consumes:
  - From `clients/wallet/src/account/`: `seal`, `open`, `WrongSeal`, `Sealed` (seal.ts); `restore` (protect.ts); `canonicalEmail`, `Holding` (deposit.ts); `enrol`, `servicesAvailable`, `Services` (enrol.ts); `rememberEnrolment` (stored.ts).
  - From Task 2: `compressedPublicKey`, `addressOfPublicKey`, `toBase64`, `JEWEL_GAS`.
  - From Task 4: `signedHeaders`.
  - From Task 1: `ChainSigner`, `SubmitResult`, `EncodeObject`.
- Produces:

  ```ts
  // account/device.ts
  export interface KeyStore { getItem(k: string): string | null; setItem(k: string, v: string): void; removeItem(k: string): void }
  export interface StoredAccount { address: string; email: string; sealed: WireSealed; createdAt: string;
    protection: 'none' | 'enrolled'; sponsored: boolean }
  export const MIN_PASSWORD: number   // 10
  export class WrongPassword extends Error {}
  export function readAccount(store?: KeyStore): StoredAccount | null
  export function createAccount(email: string, password: string, store?: KeyStore, newKey?: () => Uint8Array):
    Promise<{ account: StoredAccount; privateKey: Uint8Array }>
  export function adoptKey(privateKey: Uint8Array, email: string, password: string, store?: KeyStore):
    Promise<StoredAccount>
  export function unlockAccount(account: StoredAccount, password: string): Promise<Uint8Array>
  export function updateAccount(patch: Partial<StoredAccount>, store?: KeyStore): StoredAccount
  export function forgetAccount(store?: KeyStore): void
  // account/recover.ts
  export interface RecoveryClaim { recoveryId: string; email: string; password: string;
    factor: 'email' | 'totp' | 'passkey'; answer: string }
  export type ClaimOutcome = { ok: true; privateKey: Uint8Array; address: string }
    | { ok: false; reason: 'not-ready' | 'held' | 'refused' | 'unreachable' | 'mismatch'; detail: string }
  export function claimRecovered(claim: RecoveryClaim, services: Services, fetchImpl?: typeof fetch): Promise<ClaimOutcome>
  // account/session.tsx
  export interface Session {
    account: StoredAccount | null; address: string | null; unlocked: boolean;
    sponsor: string | null;                  // the platform's granter, from GET /v1/sponsor
    create(email: string, password: string): Promise<void>;
    unlock(password: string): Promise<void>; // throws WrongPassword
    lock(): void; forget(): void;
    adopt(privateKey: Uint8Array, email: string, password: string): Promise<void>;
    withKey<T>(use: (key: Uint8Array) => T): T;     // throws if locked
    submit(messages: EncodeObject[], granter: string | null): Promise<SubmitResult>;   // staff: granter = the business
    submitAsOwner(messages: EncodeObject[]): Promise<SubmitResult>;   // sponsors first if needed; throws SponsorshipUnavailable
    ensureSponsored(): Promise<'sponsored' | 'unavailable' | 'refused'>;
    protect(password: string): Promise<'enrolled' | 'unavailable'>;
  }
  export class SponsorshipUnavailable extends Error {}
  export function SessionProvider(props: { children: ReactNode }): JSX.Element
  export function useSession(): Session
  // account/Account.tsx
  export function CreateForm(props: { onDone?: () => void }): JSX.Element
  export function UnlockForm(props: { onDone?: () => void }): JSX.Element
  export function UnlockGate(props: { children: ReactNode }): JSX.Element   // shows Create or Unlock until unlocked
  export function AccountScreen(): JSX.Element
  // api.ts (added)
  export function sponsorInfo(): Promise<string | null>
  export function requestSponsorship(key: Uint8Array, address: string, chainId: string):
    Promise<{ ok: true; granter: string } | { ok: false; status: number; error: string }>
  ```

- [ ] **Step 1: Write the failing device tests**

`clients/loyalty/src/account/device.test.ts`:

```ts
import assert from 'node:assert/strict';
import { test } from 'node:test';

import { createAccount, forgetAccount, readAccount, unlockAccount, updateAccount, WrongPassword, type KeyStore } from './device.ts';

function memory(): KeyStore {
  const m = new Map<string, string>();
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => void m.set(k, v), removeItem: (k) => void m.delete(k) };
}
const fixedKey = () => new Uint8Array(32).fill(7);

test('an account is made on the device, and opens with its password only', async () => {
  const store = memory();
  const { account, privateKey } = await createAccount('Awa@Example.sn', 'correct horse', store, fixedKey);
  assert.match(account.address, /^yml1/);
  assert.equal(account.email, 'awa@example.sn');
  assert.deepEqual(readAccount(store), account);
  assert.deepEqual(await unlockAccount(account, 'correct horse'), privateKey);
  await assert.rejects(unlockAccount(account, 'wrong horse!'), WrongPassword);
});

test('one account per device, and a short password is refused', async () => {
  const store = memory();
  await assert.rejects(createAccount('a@b.c', 'short', store, fixedKey), /at least 10/);
  await createAccount('a@b.c', 'long enough!', store, fixedKey);
  await assert.rejects(createAccount('a@b.c', 'long enough!', store, fixedKey), /already/);
});

test('flags update in place, and forgetting removes everything', async () => {
  const store = memory();
  await createAccount('a@b.c', 'long enough!', store, fixedKey);
  assert.equal(updateAccount({ sponsored: true }, store).sponsored, true);
  assert.equal(readAccount(store)?.sponsored, true);
  forgetAccount(store);
  assert.equal(readAccount(store), null);
});
```

Run: `cd clients && npm test --workspace @yamale/loyalty`
Expected: FAIL, `Cannot find module './device.ts'`.

- [ ] **Step 2: Write `device.ts`**

```ts
/**
 * The account on this device.
 *
 * One secp256k1 key, made here, sealed under the person's password with the
 * wallet's own seal (Argon2id, then XChaCha20-Poly1305), and kept in this
 * origin's storage. The key never leaves the device unsealed. Opening it
 * takes the password, every time the tab is reopened.
 *
 * No seed phrase. If the person also enrols the account with the custodian
 * and recovery services (session.tsx, protect), a lost phone can be recovered
 * through the platform's operator-run recovery. A forgotten password cannot:
 * nobody holds it.
 */

import { secp256k1 } from '@noble/curves/secp256k1';

import { open, seal, WrongSeal, type Sealed } from '../../../wallet/src/account/seal.ts';
import { canonicalEmail } from '../../../wallet/src/account/deposit.ts';
import { addressOfPublicKey, compressedPublicKey, fromBase64Url, toBase64Url } from '../../../sdk/src/jewel.ts';

export interface KeyStore {
  getItem(k: string): string | null;
  setItem(k: string, v: string): void;
  removeItem(k: string): void;
}

export interface WireSealed { salt: string; nonce: string; ciphertext: string; time: number; memory: number; threads: number }

export interface StoredAccount {
  address: string;
  email: string;
  sealed: WireSealed;
  createdAt: string;
  protection: 'none' | 'enrolled';
  sponsored: boolean;
}

export const MIN_PASSWORD = 10;
const KEY = 'yamale.loyalty.account';

export class WrongPassword extends Error {
  constructor() { super('That password does not open this account.'); this.name = 'WrongPassword'; }
}

function browserStore(): KeyStore {
  return globalThis.localStorage;
}

const toWire = (s: Sealed): WireSealed => ({
  salt: toBase64Url(s.salt), nonce: toBase64Url(s.nonce), ciphertext: toBase64Url(s.ciphertext),
  time: s.time, memory: s.memory, threads: s.threads,
});
const fromWire = (w: WireSealed): Sealed => ({
  salt: fromBase64Url(w.salt), nonce: fromBase64Url(w.nonce), ciphertext: fromBase64Url(w.ciphertext),
  time: w.time, memory: w.memory, threads: w.threads,
});

export function readAccount(store: KeyStore = browserStore()): StoredAccount | null {
  try {
    const raw = store.getItem(KEY);
    return raw ? (JSON.parse(raw) as StoredAccount) : null;
  } catch {
    return null;
  }
}

function write(account: StoredAccount, store: KeyStore): void {
  store.setItem(KEY, JSON.stringify(account));
}

export async function adoptKey(privateKey: Uint8Array, email: string, password: string,
  store: KeyStore = browserStore()): Promise<StoredAccount> {
  if (password.length < MIN_PASSWORD) throw new Error(`a password needs at least ${MIN_PASSWORD} characters`);
  const account: StoredAccount = {
    address: addressOfPublicKey(compressedPublicKey(privateKey)),
    email: canonicalEmail(email),
    sealed: toWire(await seal(privateKey, password)),
    createdAt: new Date().toISOString(),
    protection: 'none',
    sponsored: false,
  };
  write(account, store);
  return account;
}

export async function createAccount(email: string, password: string, store: KeyStore = browserStore(),
  newKey: () => Uint8Array = () => secp256k1.utils.randomPrivateKey()): Promise<{ account: StoredAccount; privateKey: Uint8Array }> {
  if (readAccount(store)) throw new Error('this device already holds an account; forget it first');
  if (password.length < MIN_PASSWORD) throw new Error(`a password needs at least ${MIN_PASSWORD} characters`);
  const privateKey = newKey();
  return { account: await adoptKey(privateKey, email, password, store), privateKey };
}

export async function unlockAccount(account: StoredAccount, password: string): Promise<Uint8Array> {
  let key: Uint8Array;
  try {
    key = await open(fromWire(account.sealed), password);
  } catch (err) {
    if (err instanceof WrongSeal) throw new WrongPassword();
    throw err;
  }
  if (addressOfPublicKey(compressedPublicKey(key)) !== account.address) throw new WrongPassword();
  return key;
}

export function updateAccount(patch: Partial<StoredAccount>, store: KeyStore = browserStore()): StoredAccount {
  const current = readAccount(store);
  if (!current) throw new Error('no account on this device');
  const next = { ...current, ...patch, address: current.address };
  write(next, store);
  return next;
}

export function forgetAccount(store: KeyStore = browserStore()): void {
  store.removeItem(KEY);
}
```

Check `canonicalEmail`'s behaviour in `deposit.ts`. The test expects it to lower-case. If it does more, such as trimming, the test still holds; if it does less, adjust the test's expectation to what it really does and say so.

Run: `cd clients && npm test --workspace @yamale/loyalty`
Expected: PASS. Each `seal` costs about a second of Argon2id at 128 MiB.

- [ ] **Step 3: Write the failing recovery-claim tests**

`clients/loyalty/src/account/recover.test.ts`:

```ts
import assert from 'node:assert/strict';
import { test } from 'node:test';

import { protect, ROLE_CUSTODIAN, ROLE_RECOVERY } from '../../../wallet/src/account/protect.ts';
import { addressOfPublicKey, compressedPublicKey, toBase64 } from '../../../sdk/src/jewel.ts';
import { claimRecovered } from './recover.ts';

const services = { custodianUrl: 'http://custodian', recoveryUrl: 'http://recovery' };
const key = new Uint8Array(32).fill(9);
const address = addressOfPublicKey(compressedPublicKey(key));

function wire(h: any) {
  return { role: h.role, salt: toBase64(h.salt), nonce: toBase64(h.nonce), time: h.time, memory: h.memory,
    threads: h.threads, share: { x: h.share.x, y: toBase64(h.share.y) } };
}

test('two released holdings and the password give the key back', async () => {
  const holdings = await protect(key, 'the long password');
  const byRole = new Map(holdings.map((h) => [h.role, h]));
  const sent: any[] = [];
  const fetchImpl = (async (url: string, init: RequestInit) => {
    sent.push(JSON.parse(String(init.body)));
    const role = url.startsWith('http://custodian') ? ROLE_CUSTODIAN : ROLE_RECOVERY;
    return new Response(JSON.stringify({ role, address, holding: wire(byRole.get(role)) }), { status: 200 });
  }) as unknown as typeof fetch;

  const out = await claimRecovered({ recoveryId: 'r-1', email: 'Awa@Example.sn', password: 'the long password',
    factor: 'email', answer: '123456' }, services, fetchImpl);
  assert.ok(out.ok);
  assert.deepEqual(out.privateKey, key);
  assert.equal(out.address, address);
  assert.equal(sent[0].id, 'r-1');
  assert.equal(sent[0].factor, 'email');
});

test('a recovery still inside its hold says so', async () => {
  const fetchImpl = (async () => new Response(JSON.stringify({ error: 'released after 24h' }),
    { status: 409, headers: { 'Retry-After': '3600' } })) as unknown as typeof fetch;
  const out = await claimRecovered({ recoveryId: 'r', email: 'a@b.c', password: 'x'.repeat(10), factor: 'email', answer: '1' },
    services, fetchImpl);
  assert.equal(out.ok, false);
  assert.equal((out as { reason: string }).reason, 'held');
});

test('services that disagree about the account are not trusted', async () => {
  const holdings = await protect(key, 'the long password');
  const byRole = new Map(holdings.map((h) => [h.role, h]));
  const fetchImpl = (async (url: string) => {
    const role = url.startsWith('http://custodian') ? ROLE_CUSTODIAN : ROLE_RECOVERY;
    const claimed = role === ROLE_CUSTODIAN ? address : 'yml1somebodyelse';
    return new Response(JSON.stringify({ role, address: claimed, holding: wire(byRole.get(role)) }), { status: 200 });
  }) as unknown as typeof fetch;
  const out = await claimRecovered({ recoveryId: 'r', email: 'a@b.c', password: 'the long password', factor: 'email', answer: '1' },
    services, fetchImpl);
  assert.equal((out as { reason: string }).reason, 'mismatch');
});
```

Run it: FAIL, `Cannot find module './recover.ts'`.

- [ ] **Step 4: Write `recover.ts`**

```ts
/**
 * Getting the key back after an operator-run recovery.
 *
 * The account service never hands a key to anyone. When a recovery has run its
 * course (two approvers, 72 hours' notice, then a 24-hour hold the owner can
 * still cancel), each service releases its holding to the account holder, who
 * proves it with their password and second factor. The two holdings and the
 * password open the key on this device, and nowhere else. See
 * tools/custodian/claim.go.
 *
 * What this cannot do: open a key whose password is forgotten. Nobody can.
 * For that the platform proposes reassigning the jewels, publicly, over 30 days.
 */

import { restore } from '../../../wallet/src/account/protect.ts';
import { canonicalEmail, type Holding } from '../../../wallet/src/account/deposit.ts';
import type { Services } from '../../../wallet/src/account/enrol.ts';
import { addressOfPublicKey, compressedPublicKey } from '../../../sdk/src/jewel.ts';

export interface RecoveryClaim {
  recoveryId: string;
  email: string;
  password: string;
  factor: 'email' | 'totp' | 'passkey';
  answer: string;
}

export type ClaimOutcome =
  | { ok: true; privateKey: Uint8Array; address: string }
  | { ok: false; reason: 'not-ready' | 'held' | 'refused' | 'unreachable' | 'mismatch'; detail: string };

function bytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function holdingFromWire(w: any): Holding {
  return {
    role: String(w.role), salt: bytes(w.salt), nonce: bytes(w.nonce),
    time: Number(w.time), memory: Number(w.memory), threads: Number(w.threads),
    share: { x: Number(w.share.x), y: bytes(w.share.y) },
  } as Holding;
}

async function claimOne(url: string, claim: RecoveryClaim, fetchImpl: typeof fetch):
  Promise<{ ok: true; address: string; holding: Holding } | Extract<ClaimOutcome, { ok: false }>> {
  let res: Response;
  try {
    res = await fetchImpl(`${url}/v1/recovery/claim`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: claim.recoveryId, email: canonicalEmail(claim.email), password: claim.password,
        factor: claim.factor, answer: claim.answer }),
    });
  } catch (err) {
    return { ok: false, reason: 'unreachable', detail: err instanceof Error ? err.message : String(err) };
  }
  const body = await res.json().catch(() => ({}));
  if (res.status === 409) {
    return { ok: false, reason: res.headers.get('Retry-After') ? 'held' : 'not-ready', detail: String(body.error ?? '') };
  }
  if (!res.ok) return { ok: false, reason: 'refused', detail: String(body.error ?? `http ${res.status}`) };
  return { ok: true, address: String(body.address), holding: holdingFromWire(body.holding) };
}

export async function claimRecovered(claim: RecoveryClaim, services: Services,
  fetchImpl: typeof fetch = fetch): Promise<ClaimOutcome> {
  const custodian = await claimOne(services.custodianUrl, claim, fetchImpl);
  if (!custodian.ok) return custodian;
  const recovery = await claimOne(services.recoveryUrl, claim, fetchImpl);
  if (!recovery.ok) return recovery;
  if (custodian.address !== recovery.address) {
    return { ok: false, reason: 'mismatch', detail: 'the two services named different accounts' };
  }
  let privateKey: Uint8Array;
  try {
    privateKey = await restore([custodian.holding, recovery.holding], claim.password);
  } catch (err) {
    return { ok: false, reason: 'refused', detail: err instanceof Error ? err.message : String(err) };
  }
  const address = addressOfPublicKey(compressedPublicKey(privateKey));
  if (address !== custodian.address) return { ok: false, reason: 'mismatch', detail: 'the holdings open a different key' };
  return { ok: true, privateKey, address };
}
```

`Holding.share` is typed in `deposit.ts` as `Share`, imported from `shamir.ts`. If `Share.y` is not a `Uint8Array`, follow `deposit.ts`'s own wire decoding instead. The vector file `recovery/testdata/vectors.json` settles it.

Run: `cd clients && npm test --workspace @yamale/loyalty`. Expected: PASS.

- [ ] **Step 5: Add sponsorship to `api.ts`**

Append to `clients/loyalty/src/api.ts`:

```ts
import { signedHeaders } from '../../sdk/src/loyaltyauth.ts';

/** The platform's fee granter, or null when sponsoring is off or the API is down. */
export async function sponsorInfo(): Promise<string | null> {
  try {
    const res = await fetch(`${API}/v1/sponsor`);
    if (!res.ok) return null;
    return String((await res.json()).granter);
  } catch {
    return null;
  }
}

export async function requestSponsorship(key: Uint8Array, address: string, chainId: string):
  Promise<{ ok: true; granter: string } | { ok: false; status: number; error: string }> {
  try {
    const res = await fetch(`${API}/v1/sponsor`, {
      method: 'POST', headers: signedHeaders(key, 'sponsor', chainId, address),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, status: res.status, error: String(body.error ?? '') };
    return { ok: true, granter: String(body.granter) };
  } catch (err) {
    return { ok: false, status: 0, error: err instanceof Error ? err.message : String(err) };
  }
}
```

Move the `import` to the top of the file with the other import.

- [ ] **Step 6: Write the session**

`clients/loyalty/src/account/session.tsx`:

```tsx
/**
 * The signed-in state: which account this device holds, whether its key is
 * open in memory, and the one way the app sends a transaction.
 *
 * The key lives in a ref, not in React state, so it never reaches a devtools
 * snapshot or a re-render log. It is dropped after ten minutes without use,
 * and on lock or reload.
 */

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { DirectSecp256k1Wallet } from '@cosmjs/proto-signing';
import { ChainSigner, JEWEL_GAS, type EncodeObject, type SubmitResult } from '@yamale/chain';

import { enrol, servicesAvailable } from '../../../wallet/src/account/enrol.ts';
import { rememberEnrolment } from '../../../wallet/src/account/stored.ts';
import { requestSponsorship, sponsorInfo } from '../api.ts';
import { CHAIN_ID, GAS_PRICE, RPC_ENDPOINT, SERVICES } from '../config.ts';
import {
  adoptKey, createAccount, forgetAccount, readAccount, unlockAccount, updateAccount, type StoredAccount,
} from './device.ts';

const IDLE_MS = 10 * 60 * 1000;

export interface Session {
  account: StoredAccount | null;
  address: string | null;
  unlocked: boolean;
  sponsor: string | null;
  create(email: string, password: string): Promise<void>;
  unlock(password: string): Promise<void>;
  lock(): void;
  forget(): void;
  adopt(privateKey: Uint8Array, email: string, password: string): Promise<void>;
  withKey<T>(use: (key: Uint8Array) => T): T;
  submit(messages: EncodeObject[], granter: string | null): Promise<SubmitResult>;
  submitAsOwner(messages: EncodeObject[]): Promise<SubmitResult>;
  ensureSponsored(): Promise<'sponsored' | 'unavailable' | 'refused'>;
  protect(password: string): Promise<'enrolled' | 'unavailable'>;
}

/** Owners hold no coins: without the platform's allowance an owner cannot act. */
export class SponsorshipUnavailable extends Error {
  constructor(public readonly state: 'unavailable' | 'refused') {
    super(`fees cannot be sponsored: ${state}`);
    this.name = 'SponsorshipUnavailable';
  }
}

const SessionContext = createContext<Session | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [account, setAccount] = useState<StoredAccount | null>(() => readAccount());
  const [unlocked, setUnlocked] = useState(false);
  const [sponsor, setSponsor] = useState<string | null>(null);
  // The granter as of now, read by submitAsOwner. State alone would hand a
  // closure the value from the render it was created in.
  const sponsorRef = useRef<string | null>(null);
  const key = useRef<Uint8Array | null>(null);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => { sponsorInfo().then((g) => { sponsorRef.current = g; setSponsor(g); }); }, []);

  const lock = useCallback(() => {
    key.current?.fill(0);
    key.current = null;
    setUnlocked(false);
  }, []);

  const touch = useCallback(() => {
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(lock, IDLE_MS);
  }, [lock]);

  const hold = useCallback((k: Uint8Array) => { key.current = k; setUnlocked(true); touch(); }, [touch]);

  const withKey = useCallback(<T,>(use: (k: Uint8Array) => T): T => {
    if (!key.current) throw new Error('locked');
    touch();
    return use(key.current);
  }, [touch]);

  const ensureSponsored = useCallback(async () => {
    const current = readAccount();
    if (!current || !key.current) return 'refused' as const;
    if (current.sponsored) return 'sponsored' as const;
    const out = await requestSponsorship(key.current, current.address, CHAIN_ID);
    if (out.ok) {
      setAccount(updateAccount({ sponsored: true }));
      sponsorRef.current = out.granter;
      setSponsor(out.granter);
      return 'sponsored' as const;
    }
    return out.status === 0 || out.status >= 500 ? 'unavailable' as const : 'refused' as const;
  }, []);

  const protect = useCallback(async (password: string) => {
    const current = readAccount();
    if (!current || !key.current) throw new Error('locked');
    if (!(await servicesAvailable(SERVICES))) return 'unavailable' as const;
    const done = await enrol(key.current, current.email, password, SERVICES);
    rememberEnrolment({ address: done.address, email: current.email, holding: done.deviceHolding,
      enrolledAt: new Date().toISOString() });
    setAccount(updateAccount({ protection: 'enrolled' }));
    return 'enrolled' as const;
  }, []);

  const submitWith = useCallback(async (messages: EncodeObject[], granter: string | null) => {
    const wallet = await withKey((k) => DirectSecp256k1Wallet.fromKey(k, 'yml'));
    const signer = new ChainSigner(wallet, {
      rpcUrl: RPC_ENDPOINT, chainId: CHAIN_ID, gasPrice: GAS_PRICE, ...(granter ? { feeGranter: granter } : {}),
    });
    try {
      return await signer.submit(messages, '', JEWEL_GAS);
    } finally {
      signer.disconnect();
    }
  }, [withKey]);

  const submitAsOwner = useCallback(async (messages: EncodeObject[]) => {
    const state = await ensureSponsored();
    if (state !== 'sponsored') throw new SponsorshipUnavailable(state);
    if (!sponsorRef.current) sponsorRef.current = await sponsorInfo();
    if (!sponsorRef.current) throw new SponsorshipUnavailable('unavailable');
    return submitWith(messages, sponsorRef.current);
  }, [ensureSponsored, submitWith]);

  const value = useMemo<Session>(() => ({
    account,
    address: account?.address ?? null,
    unlocked,
    sponsor,
    async create(email, password) {
      const made = await createAccount(email, password);
      setAccount(made.account);
      hold(made.privateKey);
    },
    async unlock(password) {
      if (!account) throw new Error('no account on this device');
      hold(await unlockAccount(account, password));
    },
    lock,
    forget() { lock(); forgetAccount(); setAccount(null); },
    async adopt(privateKey, email, password) {
      forgetAccount();
      const adopted = await adoptKey(privateKey, email, password);
      setAccount(updateAccount({ protection: 'enrolled', sponsored: adopted.sponsored }));
      hold(privateKey);
    },
    withKey,
    submit: submitWith,
    submitAsOwner,
    ensureSponsored,
    protect,
  }), [account, unlocked, sponsor, hold, lock, withKey, submitWith, submitAsOwner, ensureSponsored, protect]);

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): Session {
  const s = useContext(SessionContext);
  if (!s) throw new Error('useSession outside SessionProvider');
  return s;
}
```

`ChainSigner` and `JEWEL_GAS` come from `@yamale/chain`. Check that `ChainSigner`, `SubmitResult` and `EncodeObject` are exported from `clients/sdk/src/index.ts`. If one is not, export it there, in this task.

- [ ] **Step 7: Write the account screens**

`clients/loyalty/src/account/Account.tsx`:

```tsx
/**
 * Making an account, opening it, protecting it, and getting it back.
 *
 * The words matter more than the layout here. A person must understand three
 * things before they rely on this account: the key is made and kept on this
 * phone; the password opens it and nobody can reset it; protection lets the
 * platform help after a lost phone, but only slowly and in the open.
 */

import { useState, type FormEvent, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { t } from '@yamale/chain';

import { QrCode } from '../../../app/src/qr.tsx';
import { SERVICES } from '../config.ts';
import { AppBar, Section } from '../ui.tsx';
import { MIN_PASSWORD, WrongPassword } from './device.ts';
import { claimRecovered } from './recover.ts';
import { useSession } from './session.tsx';

export function CreateForm({ onDone }: { onDone?: () => void }) {
  const session = useSession();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [again, setAgain] = useState('');
  const [step, setStep] = useState<'form' | 'making' | 'sponsoring' | 'protecting' | 'failed'>('form');
  const [error, setError] = useState('');

  const tooShort = password.length > 0 && password.length < MIN_PASSWORD;
  const differs = again.length > 0 && again !== password;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (tooShort || differs || !email.includes('@')) return;
    try {
      setStep('making');
      await session.create(email, password);
      setStep('sponsoring');
      await session.ensureSponsored();
      setStep('protecting');
      await session.protect(password);
      onDone?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setStep('failed');
    }
  }

  if (step !== 'form' && step !== 'failed') {
    return <p className="loading" role="status">{t(`loyalty.creating.${step}`)}</p>;
  }
  return (
    <form className="stack" onSubmit={submit}>
      <p className="hint">{t('loyalty.createLede')}</p>
      <label className="y-label" htmlFor="email">{t('loyalty.email')}</label>
      <input id="email" className="bigfield" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
      <label className="y-label" htmlFor="pw">{t('loyalty.password')}</label>
      <input id="pw" className="bigfield" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
      {tooShort && <p className="hint">{t('loyalty.passwordShort', { n: MIN_PASSWORD })}</p>}
      <label className="y-label" htmlFor="pw2">{t('loyalty.passwordAgain')}</label>
      <input id="pw2" className="bigfield" type="password" autoComplete="new-password" value={again} onChange={(e) => setAgain(e.target.value)} required />
      {differs && <p className="hint">{t('loyalty.passwordDiffers')}</p>}
      <p className="note">{t('loyalty.passwordWarning')}</p>
      {step === 'failed' && <p className="note note--warn" role="alert">{error}</p>}
      <button className="btn btn--primary" disabled={tooShort || differs}>{t('loyalty.createButton')}</button>
    </form>
  );
}

export function UnlockForm({ onDone }: { onDone?: () => void }) {
  const session = useSession();
  const [password, setPassword] = useState('');
  const [state, setState] = useState<'idle' | 'busy' | 'wrong'>('idle');
  async function submit(e: FormEvent) {
    e.preventDefault();
    setState('busy');
    try {
      await session.unlock(password);
      setPassword('');
      onDone?.();
    } catch (err) {
      setState(err instanceof WrongPassword ? 'wrong' : 'idle');
    }
  }
  return (
    <form className="stack" onSubmit={submit}>
      <label className="y-label" htmlFor="unlock">{t('loyalty.passwordToOpen')}</label>
      <input id="unlock" className="bigfield" type="password" autoComplete="current-password" value={password}
             onChange={(e) => setPassword(e.target.value)} required />
      {state === 'wrong' && <p className="note note--warn" role="alert">{t('loyalty.passwordWrong')}</p>}
      <button className="btn btn--primary" disabled={state === 'busy'}>
        {state === 'busy' ? t('loyalty.opening') : t('loyalty.openButton')}
      </button>
    </form>
  );
}

/** Anything that signs sits inside this. */
export function UnlockGate({ children }: { children: ReactNode }) {
  const session = useSession();
  if (!session.account) {
    return (
      <div className="stack">
        <p>{t('loyalty.needAccount')}</p>
        <CreateForm />
      </div>
    );
  }
  if (!session.unlocked) return <UnlockForm />;
  return <>{children}</>;
}

function AccountCode({ address }: { address: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <details className="code">
      <summary>{t('loyalty.showCode')}</summary>
      <p className="hint">{t('loyalty.codeExplain')}</p>
      <QrCode text={address} size={200} />
      <button type="button" className="btn" onClick={async () => {
        await navigator.clipboard.writeText(address); setCopied(true);
      }}>{copied ? t('loyalty.copied') : t('loyalty.copyCode')}</button>
    </details>
  );
}

function ProtectButton() {
  const session = useSession();
  const [password, setPassword] = useState('');
  const [state, setState] = useState<'idle' | 'busy' | 'unavailable' | 'failed'>('idle');
  return (
    <form className="stack" onSubmit={async (e) => {
      e.preventDefault();
      setState('busy');
      try {
        setState((await session.protect(password)) === 'enrolled' ? 'idle' : 'unavailable');
      } catch { setState('failed'); }
    }}>
      <label className="y-label" htmlFor="pp">{t('loyalty.passwordToProtect')}</label>
      <input id="pp" className="bigfield" type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
      {state === 'unavailable' && <p className="note">{t('loyalty.protectUnavailable')}</p>}
      {state === 'failed' && <p className="note note--warn">{t('loyalty.protectFailed')}</p>}
      <button className="btn" disabled={state === 'busy'}>{t('loyalty.protectButton')}</button>
    </form>
  );
}

function RecoverForm() {
  const session = useSession();
  const [f, setF] = useState({ recoveryId: '', email: '', password: '', factor: 'email' as 'email' | 'totp', answer: '' });
  const [state, setState] = useState<{ kind: 'idle' | 'busy' } | { kind: 'failed'; reason: string }>({ kind: 'idle' });
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value });
  return (
    <form className="stack" onSubmit={async (e) => {
      e.preventDefault();
      setState({ kind: 'busy' });
      const out = await claimRecovered(f, SERVICES);
      if (!out.ok) { setState({ kind: 'failed', reason: out.reason }); return; }
      await session.adopt(out.privateKey, f.email, f.password);
      setState({ kind: 'idle' });
    }}>
      <p className="hint">{t('loyalty.recoverLede')}</p>
      <label className="y-label" htmlFor="rid">{t('loyalty.recoveryId')}</label>
      <input id="rid" className="bigfield" value={f.recoveryId} onChange={set('recoveryId')} required />
      <label className="y-label" htmlFor="remail">{t('loyalty.email')}</label>
      <input id="remail" className="bigfield" type="email" value={f.email} onChange={set('email')} required />
      <label className="y-label" htmlFor="rpw">{t('loyalty.password')}</label>
      <input id="rpw" className="bigfield" type="password" value={f.password} onChange={set('password')} required />
      <label className="y-label" htmlFor="rfactor">{t('loyalty.factor')}</label>
      <select id="rfactor" className="bigfield" value={f.factor} onChange={set('factor')}>
        <option value="email">{t('loyalty.factorEmail')}</option>
        <option value="totp">{t('loyalty.factorTotp')}</option>
      </select>
      <label className="y-label" htmlFor="ranswer">{t('loyalty.factorAnswer')}</label>
      <input id="ranswer" className="bigfield" inputMode="numeric" value={f.answer} onChange={set('answer')} required />
      {state.kind === 'failed' && <p className="note note--warn" role="alert">{t(`loyalty.recoverFailed.${state.reason}`)}</p>}
      <button className="btn btn--primary" disabled={state.kind === 'busy'}>{t('loyalty.recoverButton')}</button>
      <p className="hint">{t('loyalty.forgottenPassword')}</p>
    </form>
  );
}

export function AccountScreen() {
  const session = useSession();
  const [confirmForget, setConfirmForget] = useState(false);
  const a = session.account;
  return (
    <>
      <AppBar title={t('loyalty.accountTitle')} eyebrow={t('loyalty.title')} />
      <main className="y-screen">
        {!a && (<>
          <Section title={t('loyalty.createTitle')}><CreateForm /></Section>
          <Section title={t('loyalty.recoverTitle')}><RecoverForm /></Section>
        </>)}
        {a && !session.unlocked && <Section title={a.email}><UnlockForm /></Section>}
        {a && session.unlocked && (<>
          <Section title={a.email}>
            <div className="actions">
              <span className={`y-chip ${a.protection === 'enrolled' ? 'y-chip--ok' : 'y-chip--warn'}`}>
                {a.protection === 'enrolled' ? t('loyalty.protected') : t('loyalty.onlyHere')}
              </span>
              <span className={`y-chip ${a.sponsored ? 'y-chip--ok' : 'y-chip--mute'}`}>
                {a.sponsored ? t('loyalty.sponsored') : t('loyalty.notSponsored')}
              </span>
            </div>
            {!a.sponsored && <button className="btn" onClick={() => session.ensureSponsored()}>{t('loyalty.sponsorRetry')}</button>}
            <AccountCode address={a.address} />
          </Section>
          {a.protection !== 'enrolled' && <Section title={t('loyalty.protectTitle')}><ProtectButton /></Section>}
          <Section title={t('loyalty.forJewelers')}><Link className="btn" to="/apply">{t('loyalty.applyLink')}</Link></Section>
          <Section title={t('loyalty.thisDevice')}>
            <div className="actions">
              <button className="btn" onClick={session.lock}>{t('loyalty.lock')}</button>
              <button className="btn btn--danger" onClick={() => setConfirmForget(true)}>{t('loyalty.forget')}</button>
            </div>
            {confirmForget && (
              <div className="banner banner--bad" role="alert">
                <p>{a.protection === 'enrolled' ? t('loyalty.forgetProtected') : t('loyalty.forgetUnprotected')}</p>
                <div className="actions">
                  <button className="btn btn--danger" onClick={session.forget}>{t('loyalty.forgetConfirm')}</button>
                  <button className="btn" onClick={() => setConfirmForget(false)}>{t('loyalty.cancel')}</button>
                </div>
              </div>
            )}
          </Section>
        </>)}
      </main>
    </>
  );
}
```

The `/apply` route arrives in Task 10. Until then the link leads to the not-found page; that is expected.

- [ ] **Step 8: Wire it into the shell**

In `clients/loyalty/src/App.tsx`:
- wrap everything inside `<BrowserRouter>` in `<SessionProvider>`;
- add the tab `{ to: '/account', end: false, label: t('loyalty.navAccount') }` as the last tab;
- add `<Route path="/account" element={<AccountScreen />} />`.

The imports come from `./account/session.tsx` and `./account/Account.tsx`.

- [ ] **Step 9: Add the catalogue keys**

Add these in all five locales, as in Task 6.

| key | en | fr |
|---|---|---|
| loyalty.navAccount | Account | Compte |
| loyalty.accountTitle | Your account | Votre compte |
| loyalty.createTitle | Make an account | Créer un compte |
| loyalty.createLede | Your key is made on this phone and locked with your password. Nobody, us included, can use it without your password. | Votre clé est créée sur ce téléphone et verrouillée par votre mot de passe. Personne, pas même nous, ne peut s'en servir sans lui. |
| loyalty.email | Email | E-mail |
| loyalty.password | Password | Mot de passe |
| loyalty.passwordAgain | The same password again | Le même mot de passe |
| loyalty.passwordShort | At least {n} characters. | Au moins {n} caractères. |
| loyalty.passwordDiffers | The two passwords are not the same. | Les deux mots de passe diffèrent. |
| loyalty.passwordWarning | Nobody can reset this password. If you forget it, the key cannot be opened. | Personne ne peut réinitialiser ce mot de passe. S'il est oublié, la clé ne peut plus être ouverte. |
| loyalty.createButton | Make my account | Créer mon compte |
| loyalty.creating.making | Making your key. This takes a few seconds… | Création de votre clé. Quelques secondes… |
| loyalty.creating.sponsoring | Setting up the platform's cover for network fees… | Mise en place de la prise en charge des frais… |
| loyalty.creating.protecting | Protecting your account against a lost phone… | Protection de votre compte contre la perte du téléphone… |
| loyalty.passwordToOpen | Your password, to open your account | Votre mot de passe, pour ouvrir votre compte |
| loyalty.passwordWrong | That password does not open this account. | Ce mot de passe n'ouvre pas ce compte. |
| loyalty.opening | Opening… | Ouverture… |
| loyalty.openButton | Open | Ouvrir |
| loyalty.needAccount | You need an account on this phone for this. It takes a minute. | Il faut un compte sur ce téléphone. Cela prend une minute. |
| loyalty.showCode | Show my account code | Afficher mon code de compte |
| loyalty.codeExplain | Someone who wants to pass you a jewel, or a jeweler adding you to its staff, scans this. | La personne qui veut vous transmettre un bijou, ou le joaillier qui vous ajoute à son équipe, le scanne. |
| loyalty.copyCode | Copy the code | Copier le code |
| loyalty.copied | Copied | Copié |
| loyalty.protected | Protected if this phone is lost | Protégé en cas de perte du téléphone |
| loyalty.onlyHere | Only on this phone | Uniquement sur ce téléphone |
| loyalty.sponsored | Network fees covered | Frais de réseau pris en charge |
| loyalty.notSponsored | Network fees not yet covered | Frais de réseau pas encore pris en charge |
| loyalty.sponsorRetry | Try again | Réessayer |
| loyalty.protectTitle | Protect against a lost phone | Protéger contre la perte du téléphone |
| loyalty.passwordToProtect | Your password, to seal the copies | Votre mot de passe, pour sceller les copies |
| loyalty.protectButton | Protect my account | Protéger mon compte |
| loyalty.protectUnavailable | Protection is not available here right now. Your account works on this phone. | La protection n'est pas disponible ici pour le moment. Votre compte fonctionne sur ce téléphone. |
| loyalty.protectFailed | Protection did not complete. Nothing was lost; try again. | La protection n'a pas abouti. Rien n'est perdu ; réessayez. |
| loyalty.recoverTitle | I had an account on another phone | J'avais un compte sur un autre téléphone |
| loyalty.recoverLede | If you lost your phone, contact the platform. After checking who you are, it runs a recovery: you are told of it, it lasts 72 hours, and your account comes back 24 hours after that. You then receive a recovery number to enter here, with your email and your password. | Si vous avez perdu votre téléphone, contactez la plateforme. Après vérification de votre identité, elle lance une récupération : vous en êtes informé, elle dure 72 heures, et votre compte revient 24 heures plus tard. Vous recevez alors un numéro de récupération à saisir ici, avec votre e-mail et votre mot de passe. |
| loyalty.recoveryId | Recovery number | Numéro de récupération |
| loyalty.factor | How you confirm it is you | Comment vous confirmez votre identité |
| loyalty.factorEmail | A code sent by email | Un code reçu par e-mail |
| loyalty.factorTotp | A code from an authenticator app | Un code d'application d'authentification |
| loyalty.factorAnswer | The code | Le code |
| loyalty.recoverButton | Bring my account back | Récupérer mon compte |
| loyalty.recoverFailed.not-ready | This recovery has not completed yet. | Cette récupération n'est pas encore terminée. |
| loyalty.recoverFailed.held | The recovery has completed, and your account comes back after its 24-hour hold. Try again then. | La récupération est terminée ; votre compte revient après le délai de 24 heures. Réessayez ensuite. |
| loyalty.recoverFailed.refused | The details or the code were not accepted. | Les informations ou le code n'ont pas été acceptés. |
| loyalty.recoverFailed.unreachable | The recovery services did not answer. | Les services de récupération n'ont pas répondu. |
| loyalty.recoverFailed.mismatch | The two services did not agree about your account. Contact the platform. | Les deux services ne concordent pas sur votre compte. Contactez la plateforme. |
| loyalty.forgottenPassword | If you have forgotten your password, nobody can open your key. The platform can propose to move your jewels to a new account; the proposal is public for 30 days, and your old account can refuse it. | Si vous avez oublié votre mot de passe, personne ne peut ouvrir votre clé. La plateforme peut proposer de transférer vos bijoux vers un nouveau compte ; la proposition est publique pendant 30 jours et votre ancien compte peut la refuser. |
| loyalty.forJewelers | For jewelers | Pour les joailliers |
| loyalty.applyLink | Apply to register your jewels | Demander à enregistrer vos bijoux |
| loyalty.thisDevice | This phone | Ce téléphone |
| loyalty.lock | Lock now | Verrouiller |
| loyalty.forget | Remove the account from this phone | Retirer le compte de ce téléphone |
| loyalty.forgetProtected | The account stays recoverable through the platform, but only by the slow recovery. Remove it? | Le compte reste récupérable via la plateforme, mais seulement par la récupération lente. Le retirer ? |
| loyalty.forgetUnprotected | This account is not protected. Removing it destroys the key, and every jewel it holds is lost for good. | Ce compte n'est pas protégé. Le retirer détruit la clé, et chaque bijou qu'il détient est perdu pour de bon. |
| loyalty.forgetConfirm | Remove it | Le retirer |
| loyalty.cancel | Cancel | Annuler |

- [ ] **Step 10: Test, build, and check in the browser**

Run: `cd clients && npm test --workspace @yamale/chain && npm test --workspace @yamale/loyalty && npm run build --workspace @yamale/loyalty`
Expected: all pass.

Then, with the chain running and loyalty-api started with `scripts/devnet/loyalty-local.sh`:
1. Open `/account` and make an account. The three steps show in turn.
2. The chips read "Network fees covered", and "Only on this phone" if the custodian is not running locally.
3. Run `blockchaind query feegrant grants-by-grantee <address>` and check it lists the sponsor's allowance. The address is under "Show my account code".
4. Lock the account, then open it with a wrong password: refused. Open it with the right one: opens.
5. Reload the page: the account is locked again.
6. Check at 390 px and 1600 px.

- [ ] **Step 11: Commit**

```bash
git add clients/loyalty clients/sdk/src/catalogues.ts clients/sdk/src/index.ts
git commit -m "loyalty: accounts made and sealed on the device, protection, recovery claims, and fee sponsorship"
```

---

### Task 8: My jewels: hold, pass on, claim, use a benefit, report stolen

**Files:**
- Create: `clients/loyalty/src/actions/errors.ts`, `errors.test.ts`, `useAction.tsx`
- Create: `clients/loyalty/src/owner/links.ts`, `links.test.ts`, `OfferPanel.tsx`, `VoucherPanel.tsx`, `MyJewels.tsx`, `Owned.tsx`, `Claim.tsx`
- Create: `clients/loyalty/src/verify/JewelCard.tsx`
- Modify: `clients/loyalty/src/ui.tsx` (`ShareButtons`), `clients/loyalty/src/App.tsx`, `clients/sdk/src/catalogues.ts`

**Interfaces:**
- Consumes:
  - Tasks 2–3: `readJewel`, `readJeweler`, `chainHead`, `jewelsHeldBy`, `offersAddressedTo`, `isActive`, `newClaimKey`, `claimLink`, `parseClaimFragment`, `compressedPublicKey`, `toHex`, `makeVoucher`, `encodeVoucherCode`, `makeOwnershipProof`, `offerJewelToAccount`, `offerJewelByLink`, `cancelJewelOffer`, `acceptJewelOffer`, `claimJewelOffer`, `reportJewelStolen`, `clearJewelStolen`, `vetoJewelReassignment`, `completeJewelReassignment`.
  - Task 6: `useLoad`, `AppBar`, `Section`, `Problem`, `Loading`, `day`, `Hallmark`, `BenefitList`, `benefitKind`, `JewelPage`, `useRecord`, `currentRecordHash`, `VerifiedImage`, `benefitState`, `CHAIN_ID`, `PUBLIC_ORIGIN`.
  - Task 7: `useSession`, `UnlockGate`, `SponsorshipUnavailable`.
  - From the app: `QrCode` (`clients/app/src/qr.tsx`), `Scanner` (`clients/app/src/scan.tsx`).
- Produces, for Tasks 9–10:

  ```ts
  // actions/errors.ts
  export function errorKey(raw: string): string | null
  // actions/useAction.tsx
  export type ActionState = { kind: 'idle' } | { kind: 'busy' } | { kind: 'done'; result: SubmitResult }
    | { kind: 'failed'; text: string; raw: string }
  export function useAction(): [ActionState, (fn: () => Promise<SubmitResult>) => Promise<SubmitResult | null>]
  export function ActionStatus(props: { state: ActionState; done?: string }): JSX.Element | null
  export function eventValue(result: SubmitResult, type: string, key: string): string | null
  // owner/links.ts
  export function isAccountCode(text: string): boolean
  export function rememberLink(jewelId: string, claimKey: string, link: string, store?: Storage): void
  export function storedLink(jewelId: string, claimKey: string, store?: Storage): string | null
  export function forgetLinks(jewelId: string, store?: Storage): void
  // owner/OfferPanel.tsx
  export function OfferPanel(props: { view: JewelView; actor: string;
    send: (messages: EncodeObject[]) => Promise<SubmitResult>; onChange: () => void }): JSX.Element
  export function AccountCodeInput(props: { value: string; onChange: (v: string) => void; id: string }): JSX.Element
  // verify/JewelCard.tsx
  export function JewelCard(props: { id: string; to: string; extra?: ReactNode }): JSX.Element | null
  // ui.tsx (added)
  export function ShareButtons(props: { link: string; title: string; text: string }): JSX.Element
  ```

- [ ] **Step 1: Write the failing tests**

`clients/loyalty/src/actions/errors.test.ts`:

```ts
import assert from 'node:assert/strict';
import { test } from 'node:test';

import { errorKey } from './errors.ts';

const wrap = (msg: string) =>
  `failed to execute message; message index: 0: jewel 4: ${msg} [cosmos/cosmos-sdk@v0.53.6/baseapp/baseapp.go:1027] With gas wanted: '400000'`;

test('the refusals a person can hit map to their own sentence', () => {
  assert.equal(errorKey(wrap('this jewel already has an open offer; cancel it first')), 'offerOpen');
  assert.equal(errorKey(wrap('this offer has expired; ask the owner for a new one')), 'offerExpired');
  assert.equal(errorKey(wrap('the claim key or its signature does not match this offer')), 'badClaim');
  assert.equal(errorKey(wrap('this jewel is reported stolen')), 'stolen');
  assert.equal(errorKey(wrap('this voucher has already been used')), 'voucherUsed');
  assert.equal(errorKey(wrap("this benefit's allowance for the period is used up")), 'allowanceUsed');
  assert.equal(errorKey(wrap('this jeweler has already registered a jewel with this inscription')), 'inscriptionTaken');
  assert.equal(errorKey('fee-grant not found: not found'), 'fees');
  assert.equal(errorKey('message does not exist in allowed messages'), 'fees');
});

test('"not reported stolen" is not "reported stolen", and anything unknown is null', () => {
  assert.equal(errorKey(wrap('this jewel is not reported stolen')), null);
  assert.equal(errorKey('something nobody wrote a rule for'), null);
});
```

`clients/loyalty/src/owner/links.test.ts`:

```ts
import assert from 'node:assert/strict';
import { test } from 'node:test';

import { addressOfPublicKey, compressedPublicKey } from '../../../sdk/src/jewel.ts';
import { forgetLinks, isAccountCode, rememberLink, storedLink } from './links.ts';

function memory(): Storage {
  const m = new Map<string, string>();
  return {
    get length() { return m.size; },
    key: (i: number) => [...m.keys()][i] ?? null,
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, v),
    removeItem: (k: string) => void m.delete(k),
    clear: () => m.clear(),
  };
}

test('an account code is a yml address of 20 bytes, and nothing else is', () => {
  const address = addressOfPublicKey(compressedPublicKey(new Uint8Array(32).fill(5)));
  assert.equal(isAccountCode(address), true);
  assert.equal(isAccountCode(`  ${address}\n`), true);
  assert.equal(isAccountCode(address.replace(/^yml/, 'cosmos')), false);
  assert.equal(isAccountCode('yml1notanaddress'), false);
  assert.equal(isAccountCode(''), false);
});

test('a claim link is kept per offer, and forgotten with the jewel', () => {
  const store = memory();
  rememberLink('4', 'aa', 'https://x/claim#4.k1', store);
  rememberLink('4', 'bb', 'https://x/claim#4.k2', store);
  rememberLink('5', 'cc', 'https://x/claim#5.k3', store);
  assert.equal(storedLink('4', 'bb', store), 'https://x/claim#4.k2');
  forgetLinks('4', store);
  assert.equal(storedLink('4', 'aa', store), null);
  assert.equal(storedLink('5', 'cc', store), 'https://x/claim#5.k3');
});
```

Run: `cd clients && npm test --workspace @yamale/loyalty`
Expected: FAIL. The new test files are not picked up by `src/*/*.test.ts` until their directories exist; once they do, they fail with `Cannot find module './errors.ts'` and `'./links.ts'`.

- [ ] **Step 2: Write `errors.ts`, `links.ts` and `useAction.tsx`**

`clients/loyalty/src/actions/errors.ts`:

```ts
/**
 * The chain's refusals a person can hit, mapped to catalogue keys.
 *
 * Matched on the module's own message text, because CosmJS reports a code
 * without its codespace and codes repeat across modules. The texts are those
 * registered in x/jewel/types/errors.go and in x/feegrant.
 */

const RULES: ReadonlyArray<readonly [string, string]> = [
  ['already has an open offer', 'offerOpen'],
  ['has no open offer', 'noOffer'],
  ['this offer has expired', 'offerExpired'],
  ['the recipient already holds this jewel', 'selfOffer'],
  ['this offer was made to someone else', 'notRecipient'],
  ['the claim key or its signature does not match', 'badClaim'],
  ['changed owner after this offer was made', 'offerStale'],
  ['jewel is reported stolen', 'stolen'],
  ["only the jewel's owner may do this", 'notOwner'],
  ['already registered a jewel with this inscription', 'inscriptionTaken'],
  ['this jeweler is suspended', 'suspended'],
  ['you do not work for this jeweler', 'notStaff'],
  ['already works for another jeweler', 'staffTaken'],
  ["only the jeweler's business account", 'notBusiness'],
  ['this benefit has expired', 'benefitExpired'],
  ['allowance for the period is used up', 'allowanceUsed'],
  ['this voucher has already been used', 'voucherUsed'],
  ['the voucher is not valid', 'badVoucher'],
  ['veto window has not closed yet', 'windowOpen'],
  ['changed owner after the reassignment was proposed', 'reassignmentStale'],
  ['note is at most 500 bytes', 'noteTooLong'],
  ['fee-grant not found', 'fees'],
  ['fee allowance expired', 'fees'],
  ['fee limit exceeded', 'fees'],
  ['message does not exist in allowed messages', 'fees'],
  ['insufficient funds', 'fees'],
];

export function errorKey(raw: string): string | null {
  const text = raw.toLowerCase();
  for (const [needle, key] of RULES) {
    if (text.includes(needle.toLowerCase())) return key;
  }
  return null;
}
```

`clients/loyalty/src/owner/links.ts`:

```ts
/**
 * Two small things the owner screens need: recognising an account code, and
 * keeping a claim link on the seller's phone while its offer is open.
 *
 * The link is kept so the seller can show the QR again at the counter, or
 * resend it. It holds the claim key, and the phone holding it is the seller's
 * own, which could cancel the offer anyway; it is removed when the offer goes.
 */

import { fromBech32 } from '@cosmjs/encoding';

export function isAccountCode(text: string): boolean {
  try {
    const { prefix, data } = fromBech32(text.trim());
    return prefix === 'yml' && data.length === 20;
  } catch {
    return false;
  }
}

const PREFIX = 'yamale.loyalty.link.';

export function rememberLink(jewelId: string, claimKey: string, link: string, store: Storage = localStorage): void {
  try { store.setItem(`${PREFIX}${jewelId}.${claimKey}`, link); } catch { /* storage refused: the link shows once */ }
}

export function storedLink(jewelId: string, claimKey: string, store: Storage = localStorage): string | null {
  try { return store.getItem(`${PREFIX}${jewelId}.${claimKey}`); } catch { return null; }
}

export function forgetLinks(jewelId: string, store: Storage = localStorage): void {
  try {
    const doomed: string[] = [];
    for (let i = 0; i < store.length; i++) {
      const k = store.key(i);
      if (k?.startsWith(`${PREFIX}${jewelId}.`)) doomed.push(k);
    }
    for (const k of doomed) store.removeItem(k);
  } catch { /* nothing to forget */ }
}
```

`clients/loyalty/src/actions/useAction.tsx`:

```tsx
import { useState } from 'react';
import { t, type SubmitResult } from '@yamale/chain';

import { SponsorshipUnavailable } from '../account/session.tsx';
import { errorKey } from './errors.ts';

export type ActionState =
  | { kind: 'idle' } | { kind: 'busy' } | { kind: 'done'; result: SubmitResult }
  | { kind: 'failed'; text: string; raw: string };

/** One signed action: busy while it runs, then the chain's verdict in words. */
export function useAction(): [ActionState, (fn: () => Promise<SubmitResult>) => Promise<SubmitResult | null>] {
  const [state, setState] = useState<ActionState>({ kind: 'idle' });
  async function run(fn: () => Promise<SubmitResult>): Promise<SubmitResult | null> {
    setState({ kind: 'busy' });
    try {
      const result = await fn();
      if (result.succeeded) {
        setState({ kind: 'done', result });
        return result;
      }
      const raw = result.error?.raw ?? result.raw.rawLog ?? '';
      const key = errorKey(raw);
      setState({ kind: 'failed', text: key ? t(`loyalty.err.${key}`) : (result.error?.message ?? t('loyalty.err.unknown')), raw });
    } catch (err) {
      const raw = err instanceof Error ? err.message : String(err);
      setState({ kind: 'failed', text: err instanceof SponsorshipUnavailable ? t('loyalty.err.sponsorship') : t('loyalty.err.unknown'), raw });
    }
    return null;
  }
  return [state, run];
}

export function ActionStatus({ state, done }: { state: ActionState; done?: string }) {
  if (state.kind === 'busy') return <p className="loading" role="status">{t('loyalty.sending')}</p>;
  if (state.kind === 'done' && done) return <p className="banner banner--ok" role="status">{done}</p>;
  if (state.kind !== 'failed') return null;
  return (
    <div className="note note--warn" role="alert">
      <p>{state.text}</p>
      {state.raw && <details className="advanced"><summary>{t('loyalty.advanced')}</summary><pre>{state.raw}</pre></details>}
    </div>
  );
}

/** A value from a typed event of a committed transaction, JSON-decoded. */
export function eventValue(result: SubmitResult, type: string, key: string): string | null {
  for (const e of result.raw.events ?? []) {
    if (e.type !== type) continue;
    const a = e.attributes.find((x) => x.key === key);
    if (a) {
      try { return String(JSON.parse(a.value)); } catch { return a.value; }
    }
  }
  return null;
}
```

Run: `cd clients && npm test --workspace @yamale/loyalty`. Expected: PASS.

- [ ] **Step 3: Add the share buttons to `ui.tsx`**

```tsx
/** Share a link without our servers in the path: the phone's own share sheet, a copy, or mail. */
export function ShareButtons({ link, title, text }: { link: string; title: string; text: string }) {
  const [copied, setCopied] = useState(false);
  const mail = `mailto:?subject=${encodeURIComponent(title)}&body=${encodeURIComponent(`${text}\n\n${link}`)}`;
  return (
    <div className="actions">
      {typeof navigator !== 'undefined' && 'share' in navigator && (
        <button type="button" className="btn" onClick={() => navigator.share({ title, text, url: link }).catch(() => {})}>
          {t('loyalty.share')}
        </button>
      )}
      <button type="button" className="btn" onClick={async () => { await navigator.clipboard.writeText(link); setCopied(true); }}>
        {copied ? t('loyalty.copied') : t('loyalty.copyLink')}
      </button>
      <a className="btn" href={mail}>{t('loyalty.sendByEmail')}</a>
    </div>
  );
}
```

- [ ] **Step 4: Write the jewel card and the offer panel**

`clients/loyalty/src/verify/JewelCard.tsx`:

```tsx
import { type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { readJewel, t } from '@yamale/chain';

import { Loading, useLoad } from '../ui.tsx';
import { Hallmark } from './JewelPage.tsx';
import { VerifiedImage } from './Photos.tsx';
import { currentRecordHash, useRecord } from './record.ts';

export function JewelCard({ id, to, extra }: { id: string; to: string; extra?: ReactNode }) {
  const [view] = useLoad(() => readJewel(id), [id]);
  const record = useRecord(view?.ok ? currentRecordHash(view.value.jewel) : null);
  if (!view) return <li className="y-card"><Loading /></li>;
  if (!view.ok) return null;
  const title = record.state === 'ok' ? record.record.title : t('loyalty.jewelN', { id });
  return (
    <li className="y-card jewelcard">
      <Link className="jewelcard__link" to={to}>
        {record.state === 'ok'
          ? <VerifiedImage hash={record.record.photos[0]!} alt="" className="jewelcard__img" />
          : <span className="jewelcard__img photo--absent" />}
        <span className="jewelcard__text">
          <span className="jewelcard__title">{title}</span>
          <Hallmark code={view.value.jewel.inscription} />
        </span>
        {view.value.jewel.stolen && <span className="y-chip y-chip--bad">{t('loyalty.stolenShort')}</span>}
        {view.value.offer && <span className="y-chip y-chip--brass">{t('loyalty.onOffer')}</span>}
      </Link>
      {extra}
    </li>
  );
}
```

`clients/loyalty/src/owner/OfferPanel.tsx`:

```tsx
/**
 * Passing a jewel on: to someone with an account, or by a link.
 *
 * The link carries a one-time claim key in its fragment and leaves this
 * phone directly: as a QR code at the counter, through the share sheet, or
 * by mail. No server of ours ever holds it. Both ways end in the same
 * sentence before the seller confirms: once accepted, nobody but the new
 * owner can reverse it.
 */

import { useState } from 'react';
import {
  cancelJewelOffer, claimLink, newClaimKey, offerJewelByLink, offerJewelToAccount, t,
  type EncodeObject, type JewelView, type SubmitResult,
} from '@yamale/chain';

import { QrCode } from '../../../app/src/qr.tsx';
import { Scanner } from '../../../app/src/scan.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { PUBLIC_ORIGIN } from '../config.ts';
import { day, ShareButtons } from '../ui.tsx';
import { forgetLinks, isAccountCode, rememberLink, storedLink } from './links.ts';

export function AccountCodeInput({ value, onChange, id }: { value: string; onChange: (v: string) => void; id: string }) {
  const [scanning, setScanning] = useState(false);
  return (
    <div className="stack">
      <label className="y-label" htmlFor={id}>{t('loyalty.theirCode')}</label>
      <input id={id} className="bigfield inscription" value={value} onChange={(e) => onChange(e.target.value)}
             autoComplete="off" spellCheck={false} />
      {value !== '' && !isAccountCode(value) && <p className="hint">{t('loyalty.notACode')}</p>}
      {scanning
        ? <Scanner onRead={(text) => { onChange(text.trim()); setScanning(false); }} onClose={() => setScanning(false)} />
        : <button type="button" className="btn" onClick={() => setScanning(true)}>{t('loyalty.scanCode')}</button>}
    </div>
  );
}

function LinkShare({ link, expiresAt }: { link: string; expiresAt: string }) {
  return (
    <div className="stack linkshare">
      <p className="note note--warn">{t('loyalty.linkWarning', { date: day(expiresAt) })}</p>
      <div className="linkshare__qr"><QrCode text={link} size={240} /></div>
      <p className="hint">{t('loyalty.linkScanHint')}</p>
      <ShareButtons link={link} title={t('loyalty.shareTitle')} text={t('loyalty.shareText')} />
    </div>
  );
}

export function OfferPanel({ view, actor, send, onChange }: {
  view: JewelView; actor: string; send: (messages: EncodeObject[]) => Promise<SubmitResult>; onChange: () => void;
}) {
  const [action, run] = useAction();
  const [mode, setMode] = useState<'choose' | 'account' | 'link'>('choose');
  const [to, setTo] = useState('');
  const id = view.jewel.id;
  const offer = view.offer;

  if (view.jewel.stolen) return <p className="note">{t('loyalty.cannotOfferStolen')}</p>;

  if (offer) {
    const link = offer.claimKey ? storedLink(id, offer.claimKey) : null;
    return (
      <div className="stack">
        <p>{offer.to ? t('loyalty.offeredToAccount', { date: day(offer.expiresAt) }) : t('loyalty.offeredByLink', { date: day(offer.expiresAt) })}</p>
        {link && <LinkShare link={link} expiresAt={offer.expiresAt} />}
        {offer.claimKey && !link && <p className="hint">{t('loyalty.linkNotOnThisPhone')}</p>}
        <button type="button" className="btn btn--danger" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => send([cancelJewelOffer(actor, id)]))) { forgetLinks(id); onChange(); }
        }}>{t('loyalty.cancelOffer')}</button>
        <ActionStatus state={action} />
      </div>
    );
  }

  if (mode === 'choose') {
    return (
      <div className="stack">
        <button type="button" className="btn" onClick={() => setMode('account')}>{t('loyalty.offerToAccount')}</button>
        <button type="button" className="btn" onClick={() => setMode('link')}>{t('loyalty.offerByLink')}</button>
      </div>
    );
  }

  return (
    <div className="stack">
      {mode === 'account' && <AccountCodeInput id="offer-to" value={to} onChange={setTo} />}
      {mode === 'link' && <p className="hint">{t('loyalty.offerByLinkExplain')}</p>}
      <p className="note note--warn">{t('loyalty.offerIrreversible')}</p>
      <div className="actions">
        <button type="button" className="btn btn--primary"
                disabled={action.kind === 'busy' || (mode === 'account' && !isAccountCode(to))}
                onClick={async () => {
                  if (mode === 'account') {
                    if (await run(() => send([offerJewelToAccount(actor, id, to.trim())]))) onChange();
                    return;
                  }
                  const { privateKey, claimKey } = newClaimKey();
                  if (await run(() => send([offerJewelByLink(actor, id, claimKey)]))) {
                    rememberLink(id, claimKey, claimLink(PUBLIC_ORIGIN, id, privateKey));
                    onChange();
                  }
                }}>
          {t('loyalty.confirmOffer')}
        </button>
        <button type="button" className="btn" onClick={() => setMode('choose')}>{t('loyalty.cancel')}</button>
      </div>
      <ActionStatus state={action} />
    </div>
  );
}
```

- [ ] **Step 5: Write the voucher panel**

`clients/loyalty/src/owner/VoucherPanel.tsx`:

```tsx
/**
 * The owner's consent to one use of a benefit, as a code the staff scan.
 *
 * Dated by the chain's clock, not the phone's, and renewed shortly before it
 * lapses, so the code on screen is always one the chain will accept.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { chainHead, encodeVoucherCode, makeVoucher, t } from '@yamale/chain';

import { QrCode } from '../../../app/src/qr.tsx';
import { useSession } from '../account/session.tsx';
import { CHAIN_ID } from '../config.ts';

export function VoucherPanel({ jewelId, benefitId }: { jewelId: string; benefitId: string }) {
  const session = useSession();
  const [code, setCode] = useState<{ text: string; deadline: number } | null>(null);
  const [failed, setFailed] = useState(false);
  const [now, setNow] = useState(Date.now());
  const making = useRef(false);

  const make = useCallback(async () => {
    if (making.current) return;
    making.current = true;
    try {
      const head = await chainHead();
      if (!head.ok) { setFailed(true); return; }
      const v = session.withKey((k) => makeVoucher(k, CHAIN_ID, benefitId, jewelId, head.value.time));
      const lifetime = Number(v.voucher.expiresAt) - head.value.time;
      setCode({ text: encodeVoucherCode(v), deadline: Date.now() + lifetime * 1000 });
      setFailed(false);
    } finally {
      making.current = false;
    }
  }, [session, benefitId, jewelId]);

  useEffect(() => { make(); }, [make]);
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);
  useEffect(() => {
    if (code && code.deadline - now < 20_000) make();
  }, [now, code, make]);

  if (failed) return <p className="note note--warn">{t('loyalty.voucherNoChain')}</p>;
  if (!code) return <p className="loading">{t('loyalty.loading')}</p>;
  const seconds = Math.max(0, Math.round((code.deadline - now) / 1000));
  return (
    <div className="stack voucher">
      <p>{t('loyalty.voucherShow')}</p>
      <div className="linkshare__qr"><QrCode text={code.text} size={260} /></div>
      <p className="hint num" aria-live="off">{t('loyalty.voucherSeconds', { n: seconds })}</p>
      <button type="button" className="btn" onClick={() => navigator.clipboard.writeText(code.text)}>
        {t('loyalty.copyVoucher')}
      </button>
    </div>
  );
}
```

- [ ] **Step 6: Write the owner's screens**

`clients/loyalty/src/owner/MyJewels.tsx`:

```tsx
import { useState } from 'react';
import { acceptJewelOffer, chainHead, jewelsHeldBy, offersAddressedTo, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { AppBar, Problem, Section, useLoad } from '../ui.tsx';
import { JewelCard } from '../verify/JewelCard.tsx';

function AcceptButton({ jewelId, onDone }: { jewelId: string; onDone: () => void }) {
  const session = useSession();
  const [action, run] = useAction();
  const [confirm, setConfirm] = useState(false);
  if (!confirm) return <button type="button" className="btn btn--primary" onClick={() => setConfirm(true)}>{t('loyalty.accept')}</button>;
  return (
    <UnlockGate>
      <p className="note">{t('loyalty.acceptExplain')}</p>
      <div className="actions">
        <button type="button" className="btn btn--primary" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([acceptJewelOffer(session.address!, jewelId)]))) onDone();
        }}>{t('loyalty.acceptConfirm')}</button>
        <button type="button" className="btn" onClick={() => setConfirm(false)}>{t('loyalty.cancel')}</button>
      </div>
      <ActionStatus state={action} />
    </UnlockGate>
  );
}

function Holdings({ address }: { address: string }) {
  const [head] = useLoad(() => chainHead(), []);
  const now = head?.ok ? head.value.time : Math.floor(Date.now() / 1000);
  const [offers, reloadOffers] = useLoad(() => offersAddressedTo(address, now), [address, head?.ok]);
  const [held, reloadHeld] = useLoad(() => jewelsHeldBy(address), [address]);
  const reload = () => { reloadOffers(); reloadHeld(); };
  return (
    <>
      {offers?.ok && offers.value.length > 0 && (
        <Section title={t('loyalty.offersForYou')}>
          <ul className="y-cards">
            {offers.value.map((v) => (
              <JewelCard key={v.jewel.id} id={v.jewel.id} to={`/j/${v.jewel.jewelerId}/${v.jewel.id}`}
                         extra={<AcceptButton jewelId={v.jewel.id} onDone={reload} />} />
            ))}
          </ul>
        </Section>
      )}
      {offers && !offers.ok && <Problem outcome={offers} />}
      <Section title={t('loyalty.yourJewels')}>
        {!held && <p className="loading">{t('loyalty.loading')}</p>}
        {held && !held.ok && <Problem outcome={held} />}
        {held?.ok && held.value.length === 0 && <p className="hint">{t('loyalty.noJewels')}</p>}
        {held?.ok && held.value.length > 0 && (
          <ul className="y-cards">{held.value.map((id) => <JewelCard key={id} id={id} to={`/mine/${id}`} />)}</ul>
        )}
      </Section>
    </>
  );
}

export function MyJewels() {
  const session = useSession();
  return (
    <>
      <AppBar title={t('loyalty.mineTitle')} eyebrow={t('loyalty.title')} />
      <main className="y-screen">
        {session.address ? <Holdings address={session.address} /> : <UnlockGate>{null}</UnlockGate>}
      </main>
    </>
  );
}
```

`clients/loyalty/src/owner/Owned.tsx`:

```tsx
/**
 * One jewel its owner holds, and everything the owner can do with it.
 */

import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import {
  chainHead, clearJewelStolen, isActive, makeOwnershipProof, readJewel, readJeweler, reportJewelStolen, t,
  vetoJewelReassignment, type JewelView,
} from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { CHAIN_ID, PUBLIC_ORIGIN } from '../config.ts';
import { AppBar, day, Loading, Problem, Section, ShareButtons, useLoad } from '../ui.tsx';
import { benefitState } from '../verify/history.ts';
import { benefitKind, Hallmark } from '../verify/JewelPage.tsx';
import { currentRecordHash, useRecord } from '../verify/record.ts';
import { OfferPanel } from './OfferPanel.tsx';
import { VoucherPanel } from './VoucherPanel.tsx';

function Stolen({ view, onChange }: { view: JewelView; onChange: () => void }) {
  const session = useSession();
  const [action, run] = useAction();
  const [confirm, setConfirm] = useState(false);
  const id = view.jewel.id;
  if (view.jewel.stolen) {
    return (
      <UnlockGate>
        <p>{t('loyalty.youReportedStolen')}</p>
        <button type="button" className="btn" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([clearJewelStolen(session.address!, id)]))) onChange();
        }}>{t('loyalty.foundIt')}</button>
        <ActionStatus state={action} />
      </UnlockGate>
    );
  }
  if (!confirm) return <button type="button" className="btn btn--danger" onClick={() => setConfirm(true)}>{t('loyalty.reportStolen')}</button>;
  return (
    <UnlockGate>
      <p className="note note--warn">{t('loyalty.reportStolenConfirm')}</p>
      <div className="actions">
        <button type="button" className="btn btn--danger" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([reportJewelStolen(session.address!, id)]))) { setConfirm(false); onChange(); }
        }}>{t('loyalty.reportStolen')}</button>
        <button type="button" className="btn" onClick={() => setConfirm(false)}>{t('loyalty.cancel')}</button>
      </div>
      <ActionStatus state={action} />
    </UnlockGate>
  );
}

function Veto({ view, onChange }: { view: JewelView; onChange: () => void }) {
  const session = useSession();
  const [action, run] = useAction();
  if (!view.reassignment) return null;
  return (
    <div className="banner banner--warn" role="alert">
      <p>{t('loyalty.reassignmentYours', { date: day(view.reassignment.windowEnds) })}</p>
      <UnlockGate>
        <button type="button" className="btn btn--danger" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([vetoJewelReassignment(session.address!, view.jewel.id)]))) onChange();
        }}>{t('loyalty.veto')}</button>
        <ActionStatus state={action} />
      </UnlockGate>
    </div>
  );
}

function Benefits({ view, issuerActive, now }: { view: JewelView; issuerActive: boolean; now: number }) {
  const [open, setOpen] = useState<string | null>(null);
  if (view.benefits.length === 0) return <p className="hint">{t('loyalty.noBenefits')}</p>;
  return (
    <ul className="y-cards">
      {view.benefits.map((b) => {
        const s = benefitState(b, issuerActive, now);
        const usable = s.state === 'active' && s.left > 0 && !view.jewel.stolen;
        return (
          <li key={b.id} className="y-card stack">
            <div className="benefit">
              <span className="benefit__kind">{benefitKind(b.kind)}</span>
              {s.state === 'active' && <span className="y-chip y-chip--ok num">{t('loyalty.benefitLeft', { left: s.left, allowance: b.allowance })}</span>}
              {s.state === 'expired' && <span className="y-chip y-chip--mute">{t('loyalty.benefitExpired', { date: day(s.expiredAt) })}</span>}
              {s.state === 'withdrawn' && <span className="y-chip y-chip--warn">{t('loyalty.benefitWithdrawn')}</span>}
            </div>
            {usable && (open === b.id
              ? <UnlockGate><VoucherPanel jewelId={view.jewel.id} benefitId={b.id} /></UnlockGate>
              : <button type="button" className="btn" onClick={() => setOpen(b.id)}>{t('loyalty.useBenefit')}</button>)}
          </li>
        );
      })}
    </ul>
  );
}

function Proof({ jewelId }: { jewelId: string }) {
  const session = useSession();
  const [link, setLink] = useState<string | null>(null);
  if (link) return <ShareButtons link={link} title={t('loyalty.proofTitle')} text={t('loyalty.proofShareText')} />;
  return (
    <UnlockGate>
      <p className="hint">{t('loyalty.proofExplain')}</p>
      <button type="button" className="btn" onClick={async () => {
        const head = await chainHead();
        const now = head.ok ? head.value.time : Math.floor(Date.now() / 1000);
        const q = session.withKey((k) => makeOwnershipProof(k, CHAIN_ID, jewelId, now));
        setLink(`${PUBLIC_ORIGIN}/own/${jewelId}?${q}`);
      }}>{t('loyalty.makeProof')}</button>
    </UnlockGate>
  );
}

export function Owned() {
  const { jewel: id = '' } = useParams();
  const session = useSession();
  const [view, reload] = useLoad(() => readJewel(id), [id]);
  const [head] = useLoad(() => chainHead(), []);
  const [jeweler] = useLoad(() => (view?.ok ? readJeweler(view.value.jewel.jewelerId) : Promise.resolve(null)), [view?.ok]);
  const record = useRecord(view?.ok ? currentRecordHash(view.value.jewel) : null);

  if (!view) return <><AppBar title={t('loyalty.mineTitle')} eyebrow="" back /><main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <><AppBar title={t('loyalty.mineTitle')} eyebrow="" back /><main className="y-screen"><Problem outcome={view} /></main></>;
  const v = view.value;
  const now = head?.ok ? head.value.time : Math.floor(Date.now() / 1000);
  const title = record.state === 'ok' ? record.record.title : t('loyalty.jewelN', { id });
  const publicPage = `/j/${v.jewel.jewelerId}/${v.jewel.id}`;

  return (
    <>
      <AppBar title={title} eyebrow={t('loyalty.mineTitle')} back />
      <main className="y-screen">
        <header className="jewel__mast"><h1 className="jewel__title">{title}</h1><Hallmark code={v.jewel.inscription} /></header>
        <Link className="btn" to={publicPage}>{t('loyalty.seePublicPage')}</Link>
        {v.owner !== session.address ? (
          <p className="note">{t('loyalty.notYours')}</p>
        ) : (
          <>
            <Veto view={v} onChange={reload} />
            <Section title={t('loyalty.passOn')}>
              <UnlockGate>
                <OfferPanel view={v} actor={session.address!} send={(m) => session.submitAsOwner(m)} onChange={reload} />
              </UnlockGate>
            </Section>
            <Section title={t('loyalty.benefits')}>
              <Benefits view={v} issuerActive={jeweler?.ok ? isActive(jeweler.value) : false} now={now} />
            </Section>
            <Section title={t('loyalty.proveIt')}><Proof jewelId={v.jewel.id} /></Section>
            <Section title={t('loyalty.stolenTitle')}><Stolen view={v} onChange={reload} /></Section>
          </>
        )}
      </main>
    </>
  );
}
```

`clients/loyalty/src/owner/Claim.tsx`:

```tsx
/**
 * /claim#<jewel>.<claim key>: where a buyer lands from the seller's link.
 *
 * The key in the fragment signs a claim that names this account, so a copy
 * of the transaction taken from the mempool is useless to anyone else. Once
 * claimed, the fragment is wiped from the address bar and the history.
 */

import { useMemo } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  chainHead, claimJewelOffer, compressedPublicKey, parseClaimFragment, readJewel, readJeweler, t, toHex,
} from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { CHAIN_ID } from '../config.ts';
import { AppBar, Loading, Problem, useLoad } from '../ui.tsx';
import { Hallmark } from '../verify/JewelPage.tsx';
import { Gallery } from '../verify/Photos.tsx';
import { currentRecordHash, useRecord } from '../verify/record.ts';

export function Claim() {
  const { hash } = useLocation();
  const parsed = useMemo(() => parseClaimFragment(hash), [hash]);
  const session = useSession();
  const navigate = useNavigate();
  const [action, run] = useAction();
  const [view] = useLoad(() => (parsed ? readJewel(parsed.jewelId) : Promise.resolve(null)), [parsed?.jewelId]);
  const [head] = useLoad(() => chainHead(), []);
  const [jeweler] = useLoad(() => (view?.ok ? readJeweler(view.value.jewel.jewelerId) : Promise.resolve(null)), [view?.ok]);
  const record = useRecord(view?.ok ? currentRecordHash(view.value.jewel) : null);

  const bar = <AppBar title={t('loyalty.claimTitle')} eyebrow={t('loyalty.title')} />;
  if (!parsed) return <>{bar}<main className="y-screen"><p className="banner banner--bad">{t('loyalty.claimBroken')}</p></main></>;
  if (!view || !head) return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <>{bar}<main className="y-screen"><Problem outcome={view} /></main></>;

  const offer = view.value.offer;
  const claimKey = toHex(compressedPublicKey(parsed.privateKey));
  const open = offer?.claimKey === claimKey && head.ok && Number(offer.expiresAt) > head.value.time;
  const title = record.state === 'ok' ? record.record.title : t('loyalty.jewelN', { id: parsed.jewelId });

  return (
    <>
      {bar}
      <main className="y-screen">
        <header className="jewel__mast">
          <p className="jewel__by">{t('loyalty.registeredBy', { name: jeweler?.ok ? jeweler.value.name : '' })}</p>
          <h1 className="jewel__title">{title}</h1>
          <Hallmark code={view.value.jewel.inscription} />
        </header>
        {record.state === 'ok' && <Gallery hashes={record.record.photos} alt={title} />}
        {!open && <p className="banner banner--warn">{t('loyalty.claimClosed')}</p>}
        {open && (
          <UnlockGate>
            <p>{t('loyalty.claimExplain')}</p>
            <button type="button" className="btn btn--primary" disabled={action.kind === 'busy'} onClick={async () => {
              const done = await run(() => session.submitAsOwner(
                [claimJewelOffer(session.address!, parsed.jewelId, parsed.privateKey, CHAIN_ID)]));
              if (done) {
                window.history.replaceState(null, '', '/claim');
                navigate(`/mine/${parsed.jewelId}`, { replace: true });
              }
            }}>{t('loyalty.claimButton')}</button>
            <ActionStatus state={action} />
          </UnlockGate>
        )}
      </main>
    </>
  );
}
```

- [ ] **Step 7: Wire the routes, and let anyone complete a lapsed reassignment**

In `clients/loyalty/src/App.tsx`:
- Add the tab `{ to: '/mine', end: false, label: t('loyalty.navMine') }` between Verify and Account.
- Add the routes `/mine` → `<MyJewels />`, `/mine/:jewel` → `<Owned />` and `/claim` → `<Claim />`.
- Replace the jewel route's element with `<JewelPage actions={(view, now) => <CompleteReassignment view={view} now={now} />} />`, and add this component in `App.tsx`:

```tsx
/** The spec's "anyone, once the window has passed": shown only to a signed-in visitor. */
function CompleteReassignment({ view, now }: { view: JewelView; now: number }) {
  const session = useSession();
  const [action, run] = useAction();
  const r = view.reassignment;
  if (!r || !session.address || now < Number(r.windowEnds)) return null;
  return (
    <div className="stack">
      <p className="hint">{t('loyalty.reassignmentDue')}</p>
      <UnlockGate>
        <button type="button" className="btn" disabled={action.kind === 'busy'} onClick={() =>
          run(() => session.submitAsOwner([completeJewelReassignment(session.address!, view.jewel.id)]))}>
          {t('loyalty.completeReassignment')}
        </button>
        <ActionStatus state={action} done={t('loyalty.reassignmentDone')} />
      </UnlockGate>
    </div>
  );
}
```

Append to `styles.css`:

```css
.jewelcard__link { display: flex; align-items: center; gap: var(--sp-2); color: inherit; text-decoration: none; }
.jewelcard__img { flex: none; inline-size: 56px; block-size: 56px; object-fit: cover; border-radius: var(--radius);
  min-block-size: 0; }
.jewelcard__text { display: grid; gap: 0.25rem; flex: 1; min-inline-size: 0; }
.jewelcard__title { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.jewelcard .hallmark { font-size: 0.8rem; padding: 0.15rem 0.4rem; }

/* A code a camera must read: dark modules on white, whatever the theme. */
.linkshare__qr { justify-self: center; padding: var(--sp-2); background: #ffffff; border-radius: var(--radius); }
.linkshare__qr svg { display: block; }
.voucher { justify-items: center; text-align: center; }
```

The `#ffffff` is the one colour literal in the file, kept on purpose: a QR code on a dark ground fails on many phone cameras.

- [ ] **Step 8: Add the catalogue keys**

Add these in all five locales:

| key | en | fr |
|---|---|---|
| loyalty.navMine | My jewels | Mes bijoux |
| loyalty.mineTitle | My jewels | Mes bijoux |
| loyalty.offersForYou | Offered to you | Proposés pour vous |
| loyalty.accept | Accept | Accepter |
| loyalty.acceptExplain | This jewel will be yours. From then on, only you can pass it on. | Ce bijou sera à vous. Ensuite, vous seul pourrez le transmettre. |
| loyalty.acceptConfirm | Make it mine | Le prendre |
| loyalty.yourJewels | Jewels you hold | Bijoux que vous détenez |
| loyalty.noJewels | You hold no jewels yet. When a jeweler or an owner passes you one, it appears here. | Vous ne détenez aucun bijou. Quand un joaillier ou un propriétaire vous en transmet un, il apparaît ici. |
| loyalty.stolenShort | Stolen | Volé |
| loyalty.onOffer | On offer | Proposé |
| loyalty.seePublicPage | See its public page | Voir sa page publique |
| loyalty.notYours | You do not hold this jewel. | Vous ne détenez pas ce bijou. |
| loyalty.passOn | Pass it on | Le transmettre |
| loyalty.offerToAccount | To someone with an account | À quelqu'un qui a un compte |
| loyalty.offerByLink | By a link | Par un lien |
| loyalty.offerByLinkExplain | You get a link and a QR code to give the buyer. Whoever opens it first can take the jewel. | Vous obtenez un lien et un QR code à remettre à l'acheteur. La première personne qui l'ouvre peut prendre le bijou. |
| loyalty.offerIrreversible | Once the jewel is accepted, neither the platform nor the jeweler can reverse it. Only the new owner can pass it back. | Une fois le bijou accepté, ni la plateforme ni le joaillier ne peuvent revenir en arrière. Seul le nouveau propriétaire peut le rendre. |
| loyalty.confirmOffer | Offer it | Le proposer |
| loyalty.theirCode | Their account code | Son code de compte |
| loyalty.notACode | That is not an account code. Ask them to show theirs under Account. | Ce n'est pas un code de compte. Demandez-lui le sien, sous Compte. |
| loyalty.scanCode | Scan it | Le scanner |
| loyalty.offeredToAccount | Offered to an account until {date}. | Proposé à un compte jusqu'au {date}. |
| loyalty.offeredByLink | Offered by link until {date}. | Proposé par lien jusqu'au {date}. |
| loyalty.linkNotOnThisPhone | The link was made on another phone. Cancel this offer and make a new one to get a link here. | Le lien a été créé sur un autre téléphone. Annulez cette offre et créez-en une nouvelle pour obtenir un lien ici. |
| loyalty.cancelOffer | Cancel the offer | Annuler l'offre |
| loyalty.cannotOfferStolen | A jewel reported stolen cannot be offered. | Un bijou déclaré volé ne peut pas être proposé. |
| loyalty.linkWarning | Anyone holding this link can take the jewel until it is claimed, cancelled, or it expires on {date}. Give it only to the buyer. | Toute personne qui détient ce lien peut prendre le bijou jusqu'à ce qu'il soit réclamé, annulé, ou qu'il expire le {date}. Ne le remettez qu'à l'acheteur. |
| loyalty.linkScanHint | At the counter, the buyer scans this with their phone's camera. | Au comptoir, l'acheteur le scanne avec l'appareil photo de son téléphone. |
| loyalty.shareTitle | A jewel for you | Un bijou pour vous |
| loyalty.shareText | Open this link to take the jewel into your account. | Ouvrez ce lien pour prendre le bijou dans votre compte. |
| loyalty.share | Share | Partager |
| loyalty.copyLink | Copy the link | Copier le lien |
| loyalty.sendByEmail | Send by email | Envoyer par e-mail |
| loyalty.useBenefit | Use this benefit | Utiliser cet avantage |
| loyalty.voucherShow | Show this to the jeweler's staff. It allows one use. | Montrez ceci au personnel du joaillier. Il permet une utilisation. |
| loyalty.voucherSeconds | Renews in {n} s | Renouvelé dans {n} s |
| loyalty.voucherNoChain | The register cannot be reached, so no code can be made right now. | Le registre est injoignable ; aucun code ne peut être créé pour le moment. |
| loyalty.copyVoucher | Copy the code as text | Copier le code en texte |
| loyalty.proveIt | Prove you hold it | Prouver que vous le détenez |
| loyalty.proofExplain | For an insurer or a buyer: a link that shows you hold this jewel now, without naming you. | Pour un assureur ou un acheteur : un lien qui montre que vous détenez ce bijou aujourd'hui, sans vous nommer. |
| loyalty.makeProof | Make the link | Créer le lien |
| loyalty.proofShareText | This link shows that I hold this jewel. | Ce lien montre que je détiens ce bijou. |
| loyalty.stolenTitle | If it is stolen | En cas de vol |
| loyalty.reportStolen | Report it stolen | Le déclarer volé |
| loyalty.reportStolenConfirm | While it is reported, its public page says it is stolen and nobody can offer it. You can clear the report when it is found. | Tant qu'il est déclaré, sa page publique indique qu'il est volé et personne ne peut le proposer. Vous pourrez lever la déclaration quand il sera retrouvé. |
| loyalty.youReportedStolen | You reported this jewel stolen. | Vous avez déclaré ce bijou volé. |
| loyalty.foundIt | It has been found | Il a été retrouvé |
| loyalty.reassignmentYours | The platform has proposed to move this jewel to another account, because its owner lost access. If this is your jewel and you did not ask for this, refuse it before {date}. | La plateforme propose de transférer ce bijou vers un autre compte, son propriétaire ayant perdu l'accès. Si c'est votre bijou et que vous ne l'avez pas demandé, refusez avant le {date}. |
| loyalty.veto | Refuse the move | Refuser le transfert |
| loyalty.reassignmentDue | The owner's time to refuse has passed. Anyone can now complete the move. | Le délai de refus du propriétaire est passé. Tout le monde peut maintenant finaliser le transfert. |
| loyalty.completeReassignment | Complete the move | Finaliser le transfert |
| loyalty.reassignmentDone | Done. | Fait. |
| loyalty.claimTitle | A jewel for you | Un bijou pour vous |
| loyalty.claimBroken | This link is incomplete. Ask the seller to send it again. | Ce lien est incomplet. Demandez au vendeur de le renvoyer. |
| loyalty.claimClosed | This link no longer works: the jewel was taken, the offer was cancelled, or it has expired. | Ce lien ne fonctionne plus : le bijou a été pris, l'offre annulée, ou elle a expiré. |
| loyalty.claimExplain | Take the jewel into your account. Its record and its benefits come with it. | Prenez le bijou dans votre compte. Sa fiche et ses avantages l'accompagnent. |
| loyalty.claimButton | Take it | Le prendre |
| loyalty.sending | Sending to the register… | Envoi au registre… |
| loyalty.err.unknown | The register refused this. The detail is under Advanced. | Le registre a refusé. Le détail est sous Avancé. |
| loyalty.err.sponsorship | The platform cannot cover the network fee right now, so this cannot be sent. Try again later. | La plateforme ne peut pas prendre en charge les frais pour le moment ; l'envoi est impossible. Réessayez plus tard. |
| loyalty.err.fees | The network fee for this is not covered. If you are staff, ask your jeweler; otherwise try again later. | Les frais de réseau ne sont pas couverts. Si vous êtes du personnel, demandez à votre joaillier ; sinon réessayez plus tard. |
| loyalty.err.offerOpen | This jewel is already on offer. Cancel that offer first. | Ce bijou est déjà proposé. Annulez d'abord cette offre. |
| loyalty.err.noOffer | This jewel is not on offer any more. | Ce bijou n'est plus proposé. |
| loyalty.err.offerExpired | The offer has expired. Ask the owner for a new one. | L'offre a expiré. Demandez-en une nouvelle au propriétaire. |
| loyalty.err.selfOffer | That account already holds this jewel. | Ce compte détient déjà ce bijou. |
| loyalty.err.notRecipient | This offer was made to someone else. | Cette offre a été faite à quelqu'un d'autre. |
| loyalty.err.badClaim | This link does not match the offer any more. | Ce lien ne correspond plus à l'offre. |
| loyalty.err.offerStale | The jewel changed hands after this offer was made. | Le bijou a changé de mains après cette offre. |
| loyalty.err.stolen | This jewel is reported stolen. | Ce bijou est déclaré volé. |
| loyalty.err.notOwner | Only the jewel's owner can do this. | Seul le propriétaire du bijou peut faire cela. |
| loyalty.err.inscriptionTaken | Your jewelry has already registered a jewel with this code. | Votre maison a déjà enregistré un bijou avec ce code. |
| loyalty.err.suspended | This jeweler is suspended and cannot act on the register. | Ce joaillier est suspendu et ne peut plus agir sur le registre. |
| loyalty.err.notStaff | Your account is not on this jeweler's staff. | Votre compte ne fait pas partie du personnel de ce joaillier. |
| loyalty.err.staffTaken | That person already works for another jeweler. | Cette personne travaille déjà pour un autre joaillier. |
| loyalty.err.notBusiness | Only the jeweler's own account can manage its staff. | Seul le compte du joaillier peut gérer son personnel. |
| loyalty.err.benefitExpired | This benefit has ended. | Cet avantage est terminé. |
| loyalty.err.allowanceUsed | This benefit is used up for now. | Cet avantage est épuisé pour le moment. |
| loyalty.err.voucherUsed | This code has already been used. Ask the owner for a fresh one. | Ce code a déjà été utilisé. Demandez-en un nouveau au propriétaire. |
| loyalty.err.badVoucher | This code is not valid. Ask the owner to show a fresh one. | Ce code n'est pas valide. Demandez au propriétaire d'en afficher un nouveau. |
| loyalty.err.windowOpen | The owner can still refuse; it cannot be completed yet. | Le propriétaire peut encore refuser ; impossible de finaliser pour l'instant. |
| loyalty.err.reassignmentStale | The jewel changed hands after the move was proposed. | Le bijou a changé de mains après la proposition de transfert. |
| loyalty.err.noteTooLong | The note is too long. Keep it under 500 characters and put the detail in the record. | La note est trop longue. Restez sous 500 caractères et mettez le détail dans la fiche. |

- [ ] **Step 9: Test, build, and walk it through**

Run: `cd clients && npm test --workspace @yamale/chain && npm test --workspace @yamale/loyalty && npm run build --workspace @yamale/loyalty`
Expected: all pass.

Then, against the seeded chain with loyalty-api running:
1. On `localhost:5390`, make account A and copy its code from the Account tab.
2. Hand A the seeder's ring, which `jw-buyer` holds at the end of `seed-jewel.sh`:

   ```bash
   "$SCRATCH/blockchaind.exe" tx jewel offer 1 --to <A's code> --from jw-buyer --keyring-backend test \
     --home .devnet/local/node --chain-id yamale-local-1 --node tcp://127.0.0.1:26957 --gas auto --gas-adjustment 1.5 -y
   ```

3. Under My jewels, A sees the ring under "Offered to you", accepts it, and the ring moves to "Jewels you hold".
4. From A, pass it on by link. Open the link on `127.0.0.1:5390` (account B), changing only the host, and take it as B. A new account is made on the way.
5. B uses the cleaning benefit; the QR code and its countdown show.
6. B reports it stolen. The public page says so, and B's offer choices are gone. B clears it.
7. B makes an ownership-proof link. Opened on A's origin, it says "holds this jewel now".
8. Check My jewels and the claim page at 390 px and 1600 px, in both themes.

- [ ] **Step 10: Commit**

```bash
git add clients/loyalty clients/sdk/src/catalogues.ts
git commit -m "loyalty: my jewels: accept, claim by link, pass on, benefit codes, stolen reports, ownership proofs"
```

---

### Task 9: The Atelier, part 1: register, stock, sell and correct

**Files:**
- Create: `clients/loyalty/src/atelier/role.ts`, `photo.ts`, `photo.test.ts`, `RecordForm.tsx`, `Atelier.tsx`, `AtelierJewel.tsx`, `Register.tsx`, `Amend.tsx`, `Staff.tsx`
- Modify: `clients/loyalty/src/api.ts` (`uploadFile`), `clients/loyalty/src/App.tsx`, `clients/sdk/src/catalogues.ts`

**Interfaces:**
- Consumes:
  - Tasks 2–3: `readJewelers`, `roleOf`, `isActive`, `readJewel`, `jewelsHeldBy`, `findByInscription`, `normaliseInscription`, `registerJewel`, `amendJewel`, `setJewelStaff`, `grantStaffFees`, `feeAllowanceExists`.
  - From signing.ts: `revokeFeeAllowance`.
  - Task 6: `encodeRecord`, `JewelRecord`, `Stone`, `useRecord`, `currentRecordHash`, `recordUri`.
  - Task 8: `OfferPanel`, `AccountCodeInput`, `isAccountCode`, `JewelCard`, `useAction`, `ActionStatus`, `eventValue`.
- Produces:

  ```ts
  // atelier/role.ts
  export type RoleState = { state: 'loading' } | { state: 'none' }
    | { state: 'failed'; outcome: { reason: string; detail: string } }
    | { state: 'role'; jeweler: Jeweler; business: boolean; staff: boolean; active: boolean }
  export function useRole(address: string | null): [RoleState, () => void]
  // atelier/photo.ts
  export function fitWithin(width: number, height: number, max: number): { width: number; height: number }
  export function preparePhoto(file: Blob, max?: number, quality?: number): Promise<Uint8Array>
  // atelier/RecordForm.tsx
  export type Draft = Pick<JewelRecord, 'title' | 'metal' | 'weightGrams' | 'stones' | 'origin'>
  export const EMPTY_DRAFT: Draft
  export function RecordForm(props: { value: Draft; onChange: (d: Draft) => void }): JSX.Element
  export function draftComplete(d: Draft): boolean
  // api.ts (added)
  export function uploadFile(key: Uint8Array, bytes: Uint8Array, chainId: string):
    Promise<{ ok: true; hash: string } | { ok: false; status: number; error: string }>
  ```

- [ ] **Step 1: Write the failing photo test**

`clients/loyalty/src/atelier/photo.test.ts`:

```ts
import assert from 'node:assert/strict';
import { test } from 'node:test';

import { fitWithin } from './photo.ts';

test('a photo is scaled to fit, keeping its shape, and never enlarged', () => {
  assert.deepEqual(fitWithin(4000, 3000, 2400), { width: 2400, height: 1800 });
  assert.deepEqual(fitWithin(3000, 4000, 2400), { width: 1800, height: 2400 });
  assert.deepEqual(fitWithin(800, 600, 2400), { width: 800, height: 600 });
});
```

Run it: FAIL, `Cannot find module './photo.ts'`.

- [ ] **Step 2: Write `photo.ts`**

```ts
/**
 * A jeweler's photo, made fit for the record.
 *
 * Re-encoding as JPEG does two jobs. It bounds the size, so a 12-megapixel
 * phone photo does not cost a buyer on a metered connection 6 MB. And it
 * drops the EXIF block, which can hold the place a photo was taken; a
 * workshop's coordinates have no business in a public record.
 */

export function fitWithin(width: number, height: number, max: number): { width: number; height: number } {
  if (width <= max && height <= max) return { width, height };
  const scale = max / Math.max(width, height);
  return { width: Math.round(width * scale), height: Math.round(height * scale) };
}

export async function preparePhoto(file: Blob, max = 2400, quality = 0.85): Promise<Uint8Array> {
  const bitmap = await createImageBitmap(file); // honours EXIF orientation before the data is dropped
  const { width, height } = fitWithin(bitmap.width, bitmap.height, max);
  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = height;
  canvas.getContext('2d')!.drawImage(bitmap, 0, 0, width, height);
  bitmap.close();
  const blob = await new Promise<Blob>((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('the photo could not be encoded'))), 'image/jpeg', quality));
  return new Uint8Array(await blob.arrayBuffer());
}
```

Run: `cd clients && npm test --workspace @yamale/loyalty`. Expected: PASS.

- [ ] **Step 3: Add `uploadFile` to `api.ts`**

```ts
export async function uploadFile(key: Uint8Array, bytes: Uint8Array, chainId: string):
  Promise<{ ok: true; hash: string } | { ok: false; status: number; error: string }> {
  const hash = sha256Hex(bytes);
  try {
    const res = await fetch(`${API}/v1/files`, {
      method: 'POST', headers: signedHeaders(key, 'file', chainId, hash), body: bytes,
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, status: res.status, error: String(body.error ?? '') };
    if (body.hash !== hash) return { ok: false, status: 500, error: 'the API stored different bytes' };
    return { ok: true, hash };
  } catch (err) {
    return { ok: false, status: 0, error: err instanceof Error ? err.message : String(err) };
  }
}
```

- [ ] **Step 4: Write the role hook and the record form**

`clients/loyalty/src/atelier/role.ts`:

```ts
import { isActive, readJewelers, roleOf, type Jeweler } from '@yamale/chain';

import { useLoad } from '../ui.tsx';

export type RoleState =
  | { state: 'loading' } | { state: 'none' }
  | { state: 'failed'; outcome: { reason: string; detail: string } }
  | { state: 'role'; jeweler: Jeweler; business: boolean; staff: boolean; active: boolean };

/** What this account is to a jeweler, read from the chain: its business, its staff, or neither. */
export function useRole(address: string | null): [RoleState, () => void] {
  const [jewelers, reload] = useLoad(() => (address ? readJewelers() : Promise.resolve(null)), [address]);
  if (!address) return [{ state: 'none' }, reload];
  if (!jewelers) return [{ state: 'loading' }, reload];
  if (!jewelers.ok) return [{ state: 'failed', outcome: jewelers }, reload];
  const r = roleOf(address, jewelers.value);
  return [r ? { state: 'role', ...r, active: isActive(r.jeweler) } : { state: 'none' }, reload];
}
```

`clients/loyalty/src/atelier/RecordForm.tsx`:

```tsx
import { t } from '@yamale/chain';

import type { JewelRecord, Stone } from '../verify/record.ts';

export type Draft = Pick<JewelRecord, 'title' | 'metal' | 'weightGrams' | 'stones' | 'origin'>;

export const EMPTY_DRAFT: Draft = {
  title: '', metal: '', weightGrams: '', stones: [],
  origin: { workshop: '', stoneSource: '', labCertificate: '' },
};

function Field({ id, label, value, onChange, max, inputMode }: {
  id: string; label: string; value: string; onChange: (v: string) => void; max: number;
  inputMode?: 'decimal' | 'text';
}) {
  return (
    <>
      <label className="y-label" htmlFor={id}>{label}</label>
      <input id={id} className="bigfield" value={value} maxLength={max} inputMode={inputMode}
             onChange={(e) => onChange(e.target.value)} />
    </>
  );
}

export function RecordForm({ value, onChange }: { value: Draft; onChange: (d: Draft) => void }) {
  const set = (patch: Partial<Draft>) => onChange({ ...value, ...patch });
  const setOrigin = (patch: Partial<Draft['origin']>) => set({ origin: { ...value.origin, ...patch } });
  const setStone = (i: number, patch: Partial<Stone>) =>
    set({ stones: value.stones.map((s, j) => (j === i ? { ...s, ...patch } : s)) });
  return (
    <div className="stack">
      <Field id="title" label={t('loyalty.fieldTitle')} value={value.title} onChange={(v) => set({ title: v })} max={120} />
      <Field id="metal" label={t('loyalty.metal')} value={value.metal} onChange={(v) => set({ metal: v })} max={80} />
      <Field id="weight" label={t('loyalty.fieldWeight')} value={value.weightGrams} inputMode="decimal"
             onChange={(v) => set({ weightGrams: v.replace(',', '.') })} max={12} />
      <fieldset className="stack">
        <legend className="y-label">{t('loyalty.stones')}</legend>
        {value.stones.map((s, i) => (
          <div key={i} className="stone-row">
            <input aria-label={t('loyalty.stoneKind')} className="bigfield" value={s.kind} maxLength={60}
                   placeholder={t('loyalty.stoneKind')} onChange={(e) => setStone(i, { kind: e.target.value })} />
            <input aria-label={t('loyalty.stoneCarat')} className="bigfield num" value={s.carat} maxLength={12}
                   inputMode="decimal" placeholder="ct" onChange={(e) => setStone(i, { carat: e.target.value.replace(',', '.') })} />
            <input aria-label={t('loyalty.stoneCount')} className="bigfield num" type="number" min={1} max={9999}
                   value={s.count} onChange={(e) => setStone(i, { count: Math.max(1, Number(e.target.value) || 1) })} />
            <button type="button" className="btn" aria-label={t('loyalty.removeStone')}
                    onClick={() => set({ stones: value.stones.filter((_, j) => j !== i) })}>×</button>
          </div>
        ))}
        <button type="button" className="btn" onClick={() => set({ stones: [...value.stones, { kind: '', carat: '', count: 1 }] })}>
          {t('loyalty.addStone')}
        </button>
      </fieldset>
      <Field id="workshop" label={t('loyalty.workshop')} value={value.origin.workshop} onChange={(v) => setOrigin({ workshop: v })} max={120} />
      <Field id="source" label={t('loyalty.stoneSource')} value={value.origin.stoneSource} onChange={(v) => setOrigin({ stoneSource: v })} max={120} />
      <Field id="lab" label={t('loyalty.labCertificate')} value={value.origin.labCertificate} onChange={(v) => setOrigin({ labCertificate: v })} max={80} />
    </div>
  );
}

/** What must be filled before a record is worth registering. */
export function draftComplete(d: Draft): boolean {
  return d.title.trim() !== '' && d.metal.trim() !== '' && /^[0-9]+(\.[0-9]+)?$/.test(d.weightGrams)
    && d.origin.workshop.trim() !== ''
    && d.stones.every((s) => s.kind.trim() !== '' && /^[0-9]+(\.[0-9]+)?$/.test(s.carat));
}
```

- [ ] **Step 5: Write the registration screen**

`clients/loyalty/src/atelier/Register.tsx`:

```tsx
/**
 * Registering a jewel: photos, then the record, then the token.
 *
 * In that order, because the token names the record's fingerprint and the
 * record names the photos' fingerprints. If the last step fails, the files
 * stay in loyalty-api unreferenced; that costs disk, not trust, and a retry
 * re-uses them because the same bytes have the same name.
 */

import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { findByInscription, normaliseInscription, registerJewel, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, eventValue, useAction } from '../actions/useAction.tsx';
import { uploadFile } from '../api.ts';
import { CHAIN_ID, recordUri } from '../config.ts';
import { AppBar, Section } from '../ui.tsx';
import { encodeRecord } from '../verify/record.ts';
import { preparePhoto } from './photo.ts';
import { draftComplete, EMPTY_DRAFT, RecordForm, type Draft } from './RecordForm.tsx';
import { useRole } from './role.ts';

type Step = { kind: 'form' } | { kind: 'photos'; done: number; of: number } | { kind: 'record' } | { kind: 'chain' }
  | { kind: 'failed'; text: string };

export function Register() {
  const session = useSession();
  const [role] = useRole(session.address);
  const navigate = useNavigate();
  const [code, setCode] = useState('');
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT);
  const [files, setFiles] = useState<File[]>([]);
  const [step, setStep] = useState<Step>({ kind: 'form' });
  const [action, run] = useAction();
  const normalised = normaliseInscription(code);

  if (role.state !== 'role' || !role.staff || !role.active) {
    return <><AppBar title={t('loyalty.registerTitle')} eyebrow="" back /><main className="y-screen"><p className="note">{t('loyalty.staffOnly')}</p></main></>;
  }
  const jeweler = role.jeweler;

  async function submit() {
    if (!normalised || !draftComplete(draft) || files.length === 0) return;
    const existing = await findByInscription(normalised);
    const clash = existing.ok ? existing.value.find((j) => j.jewelerId === jeweler.id) : undefined;
    if (clash) { setStep({ kind: 'failed', text: t('loyalty.alreadyRegistered', { id: clash.id }) }); return; }

    const photos: string[] = [];
    for (let i = 0; i < files.length; i++) {
      setStep({ kind: 'photos', done: i, of: files.length });
      const up = await uploadFile(session.withKey((k) => k), await preparePhoto(files[i]!), CHAIN_ID);
      if (!up.ok) { setStep({ kind: 'failed', text: t('loyalty.uploadFailed', { error: up.error }) }); return; }
      photos.push(up.hash);
    }
    setStep({ kind: 'record' });
    const record = encodeRecord({ version: 1, jewelerId: jeweler.id, inscription: normalised, ...draft, photos,
      createdAt: new Date().toISOString() });
    const up = await uploadFile(session.withKey((k) => k), record, CHAIN_ID);
    if (!up.ok) { setStep({ kind: 'failed', text: t('loyalty.uploadFailed', { error: up.error }) }); return; }

    setStep({ kind: 'chain' });
    const done = await run(() => session.submit(
      [registerJewel(session.address!, normalised, up.hash, recordUri(up.hash))], jeweler.business));
    if (!done) { setStep({ kind: 'form' }); return; }
    const id = eventValue(done, 'blockchain.jewel.v1.EventJewelRegistered', 'jewel_id');
    navigate(id ? `/atelier/jewel/${id}` : '/atelier', { replace: true });
  }

  const busy = step.kind === 'photos' || step.kind === 'record' || step.kind === 'chain';
  return (
    <>
      <AppBar title={t('loyalty.registerTitle')} eyebrow={jeweler.name} back />
      <main className="y-screen">
        <UnlockGate>
          <Section title={t('loyalty.codeLabel')}>
            <input className="bigfield inscription" value={code} onChange={(e) => setCode(e.target.value)}
                   autoCapitalize="characters" autoComplete="off" spellCheck={false} aria-label={t('loyalty.codeLabel')} />
            {code !== '' && <p className="hint">{normalised ? t('loyalty.codeReadsAs', { code: normalised }) : t('loyalty.codeInvalid')}</p>}
            <p className="hint">{t('loyalty.codeMustMatch')}</p>
          </Section>
          <Section title={t('loyalty.photos')}>
            <input type="file" accept="image/*" capture="environment" multiple aria-label={t('loyalty.photos')}
                   onChange={(e) => setFiles([...files, ...Array.from(e.target.files ?? [])].slice(0, 12))} />
            <p className="hint">{t('loyalty.photosHint', { n: files.length })}</p>
            {files.length > 0 && <button type="button" className="btn" onClick={() => setFiles([])}>{t('loyalty.clearPhotos')}</button>}
          </Section>
          <Section title={t('loyalty.details')}><RecordForm value={draft} onChange={setDraft} /></Section>
          {step.kind === 'photos' && <p className="loading">{t('loyalty.stepPhotos', { n: step.done + 1, of: step.of })}</p>}
          {step.kind === 'record' && <p className="loading">{t('loyalty.stepRecord')}</p>}
          {step.kind === 'failed' && <p className="note note--warn" role="alert">{step.text}</p>}
          <ActionStatus state={action} />
          <button type="button" className="btn btn--primary" disabled={busy || !normalised || !draftComplete(draft) || files.length === 0}
                  onClick={submit}>{t('loyalty.registerButton')}</button>
        </UnlockGate>
      </main>
    </>
  );
}
```

`session.withKey((k) => k)` hands the key to `uploadFile`, which signs the request with it at once and keeps nothing.

- [ ] **Step 6: Write the dashboard, the jewel view, and corrections**

`clients/loyalty/src/atelier/Atelier.tsx`:

```tsx
import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { findByInscription, jewelsHeldBy, normaliseInscription, t } from '@yamale/chain';

import { useSession } from '../account/session.tsx';
import { AppBar, Problem, Section, useLoad } from '../ui.tsx';
import { JewelCard } from '../verify/JewelCard.tsx';
import { useRole } from './role.ts';

export function Atelier() {
  const session = useSession();
  const [role] = useRole(session.address);
  const navigate = useNavigate();
  const [code, setCode] = useState('');
  const [missing, setMissing] = useState(false);
  const business = role.state === 'role' ? role.jeweler.business : null;
  const [stock] = useLoad(() => (business ? jewelsHeldBy(business) : Promise.resolve(null)), [business]);

  if (role.state !== 'role') {
    return (
      <>
        <AppBar title={t('loyalty.atelierTitle')} eyebrow={t('loyalty.title')} />
        <main className="y-screen">
          {role.state === 'failed' ? <Problem outcome={role.outcome} /> : (
            <>
              <p className="note">{t('loyalty.notAJeweler')}</p>
              <Link className="btn" to="/apply">{t('loyalty.applyLink')}</Link>
            </>
          )}
        </main>
      </>
    );
  }
  const j = role.jeweler;

  async function find(e: FormEvent) {
    e.preventDefault();
    const n = normaliseInscription(code);
    if (!n) return;
    const out = await findByInscription(n);
    const mine = out.ok ? out.value.find((x) => x.jewelerId === j.id) : undefined;
    if (mine) navigate(`/atelier/jewel/${mine.id}`); else setMissing(true);
  }

  return (
    <>
      <AppBar title={j.name} eyebrow={t('loyalty.atelierTitle')} />
      <main className="y-screen">
        <div className="actions">
          <span className={`y-chip ${role.active ? 'y-chip--ok' : 'y-chip--bad'}`}>
            {role.active ? t('loyalty.jewelerActive') : t('loyalty.jewelerSuspended')}
          </span>
          {role.business && <span className="y-chip y-chip--brass">{t('loyalty.youAreBusiness')}</span>}
        </div>
        {!role.active && <p className="banner banner--warn">{t('loyalty.suspendedNote')}</p>}
        {role.business && !role.staff && <p className="hint">{t('loyalty.businessAddYourself')}</p>}

        <div className="actions">
          {role.staff && role.active && <Link className="btn btn--primary" to="/atelier/register">{t('loyalty.registerTitle')}</Link>}
        </div>

        <Section title={t('loyalty.findJewel')}>
          <form className="stack" onSubmit={find}>
            <input className="bigfield inscription" value={code} onChange={(e) => { setCode(e.target.value); setMissing(false); }}
                   aria-label={t('loyalty.codeLabel')} autoCapitalize="characters" spellCheck={false} />
            <button className="btn">{t('loyalty.verifyButton')}</button>
            {missing && <p className="hint">{t('loyalty.notOurs')}</p>}
          </form>
        </Section>

        <Section title={t('loyalty.stock')}>
          {stock && !stock.ok && <Problem outcome={stock} />}
          {stock?.ok && stock.value.length === 0 && <p className="hint">{t('loyalty.noStock')}</p>}
          {stock?.ok && stock.value.length > 0 && (
            <ul className="y-cards">{stock.value.map((id) => <JewelCard key={id} id={id} to={`/atelier/jewel/${id}`} />)}</ul>
          )}
        </Section>
      </main>
    </>
  );
}
```

`clients/loyalty/src/atelier/AtelierJewel.tsx`:

```tsx
import { Link, useParams } from 'react-router-dom';
import { readJewel, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { OfferPanel } from '../owner/OfferPanel.tsx';
import { AppBar, day, Loading, NotFound, Problem, Section, useLoad } from '../ui.tsx';
import { Hallmark } from '../verify/JewelPage.tsx';
import { currentRecordHash, useRecord } from '../verify/record.ts';
import { useRole } from './role.ts';

export function AtelierJewel() {
  const { jewel: id = '' } = useParams();
  const session = useSession();
  const [role] = useRole(session.address);
  const [view, reload] = useLoad(() => readJewel(id), [id]);
  const record = useRecord(view?.ok ? currentRecordHash(view.value.jewel) : null);

  if (!view || role.state === 'loading') return <><AppBar title={t('loyalty.atelierTitle')} eyebrow="" back /><main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <><AppBar title={t('loyalty.atelierTitle')} eyebrow="" back /><main className="y-screen"><Problem outcome={view} /></main></>;
  if (role.state !== 'role' || view.value.jewel.jewelerId !== role.jeweler.id) return <NotFound />;

  const v = view.value;
  const inStock = v.owner === role.jeweler.business;
  const title = record.state === 'ok' ? record.record.title : t('loyalty.jewelN', { id });
  const lastChange = v.jewel.ownerSince[v.jewel.ownerSince.length - 1];

  return (
    <>
      <AppBar title={title} eyebrow={role.jeweler.name} back />
      <main className="y-screen">
        <header className="jewel__mast"><h1 className="jewel__title">{title}</h1><Hallmark code={v.jewel.inscription} /></header>
        <div className="actions">
          <Link className="btn" to={`/j/${v.jewel.jewelerId}/${v.jewel.id}`}>{t('loyalty.seePublicPage')}</Link>
          {role.staff && role.active && v.jewel.amendments.length < 50 && (
            <Link className="btn" to={`/atelier/jewel/${id}/amend`}>{t('loyalty.correctRecord')}</Link>
          )}
        </div>
        {inStock ? (
          <Section title={t('loyalty.sell')}>
            {role.staff && role.active ? (
              <UnlockGate>
                <OfferPanel view={v} actor={session.address!} send={(m) => session.submit(m, role.jeweler.business)} onChange={reload} />
              </UnlockGate>
            ) : <p className="hint">{t('loyalty.staffOnly')}</p>}
          </Section>
        ) : (
          <p className="note">{lastChange ? t('loyalty.soldOn', { date: day(lastChange) }) : t('loyalty.notInStock')}</p>
        )}
      </main>
    </>
  );
}
```

`clients/loyalty/src/atelier/Amend.tsx`:

```tsx
/**
 * A correction appends; it never overwrites. The jewel's page then shows the
 * corrected record first and keeps the original one tap away.
 */

import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { amendJewel, readJewel, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { uploadFile } from '../api.ts';
import { CHAIN_ID, recordUri } from '../config.ts';
import { AppBar, Loading, Section, useLoad } from '../ui.tsx';
import { currentRecordHash, encodeRecord, useRecord } from '../verify/record.ts';
import { preparePhoto } from './photo.ts';
import { draftComplete, RecordForm, type Draft } from './RecordForm.tsx';
import { useRole } from './role.ts';

const MAX_NOTE = 500;

export function Amend() {
  const { jewel: id = '' } = useParams();
  const session = useSession();
  const [role] = useRole(session.address);
  const navigate = useNavigate();
  const [view] = useLoad(() => readJewel(id), [id]);
  const record = useRecord(view?.ok ? currentRecordHash(view.value.jewel) : null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [photos, setPhotos] = useState<string[]>([]);
  const [extra, setExtra] = useState<File[]>([]);
  const [note, setNote] = useState('');
  const [failed, setFailed] = useState('');
  const [action, run] = useAction();

  useEffect(() => {
    if (record.state === 'ok' && !draft) {
      const { title, metal, weightGrams, stones, origin } = record.record;
      setDraft({ title, metal, weightGrams, stones, origin });
      setPhotos(record.record.photos);
    }
  }, [record, draft]);

  if (!view?.ok || role.state !== 'role' || !draft || record.state !== 'ok') {
    return <><AppBar title={t('loyalty.correctRecord')} eyebrow="" back /><main className="y-screen"><Loading /></main></>;
  }
  const noteBytes = new TextEncoder().encode(note).length;
  const jeweler = role.jeweler;
  const base = record.record;

  async function submit() {
    setFailed('');
    const all = [...photos];
    for (const f of extra) {
      const up = await uploadFile(session.withKey((k) => k), await preparePhoto(f), CHAIN_ID);
      if (!up.ok) { setFailed(t('loyalty.uploadFailed', { error: up.error })); return; }
      all.push(up.hash);
    }
    const bytes = encodeRecord({ version: 1, jewelerId: jeweler.id, inscription: base.inscription, ...draft!,
      photos: all.slice(0, 12), createdAt: new Date().toISOString() });
    const up = await uploadFile(session.withKey((k) => k), bytes, CHAIN_ID);
    if (!up.ok) { setFailed(t('loyalty.uploadFailed', { error: up.error })); return; }
    if (await run(() => session.submit([amendJewel(session.address!, id, up.hash, recordUri(up.hash), note.trim())], jeweler.business))) {
      navigate(`/atelier/jewel/${id}`, { replace: true });
    }
  }

  return (
    <>
      <AppBar title={t('loyalty.correctRecord')} eyebrow={jeweler.name} back />
      <main className="y-screen">
        <UnlockGate>
          <p className="hint">{t('loyalty.correctExplain')}</p>
          <Section title={t('loyalty.details')}><RecordForm value={draft} onChange={setDraft} /></Section>
          <Section title={t('loyalty.photos')}>
            <p className="hint">{t('loyalty.photosKept', { n: photos.length })}</p>
            <input type="file" accept="image/*" multiple aria-label={t('loyalty.addPhotos')}
                   onChange={(e) => setExtra(Array.from(e.target.files ?? []))} />
          </Section>
          <Section title={t('loyalty.correctionNote')}>
            <textarea className="bigfield" rows={3} value={note} onChange={(e) => setNote(e.target.value)} aria-label={t('loyalty.correctionNote')} />
            <p className={`hint num${noteBytes > MAX_NOTE ? ' hint--bad' : ''}`}>{t('loyalty.noteBytes', { n: noteBytes, max: MAX_NOTE })}</p>
          </Section>
          {failed && <p className="note note--warn" role="alert">{failed}</p>}
          <ActionStatus state={action} />
          <button type="button" className="btn btn--primary"
                  disabled={action.kind === 'busy' || note.trim() === '' || noteBytes > MAX_NOTE || !draftComplete(draft)}
                  onClick={submit}>{t('loyalty.saveCorrection')}</button>
        </UnlockGate>
      </main>
    </>
  );
}
```

- [ ] **Step 7: Write staff management**

The business needs it before anyone can register: on the chain a business account is not staff until it adds itself.

`clients/loyalty/src/atelier/Staff.tsx`:

```tsx
/**
 * The business adds and removes its staff, and pays their fees for staff
 * work only. Adding and granting go in one transaction, so a staff member is
 * never on the register without the means to act, and never the other way
 * round.
 */

import { useState } from 'react';
import { feeAllowanceExists, grantStaffFees, revokeFeeAllowance, setJewelStaff, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { AccountCodeInput } from '../owner/OfferPanel.tsx';
import { isAccountCode } from '../owner/links.ts';
import { AppBar, Section } from '../ui.tsx';
import { useRole } from './role.ts';

const STAFF_BUDGET = [{ denom: 'uyml', amount: '5000000' }];
const short = (a: string) => `${a.slice(0, 8)}…${a.slice(-4)}`;

export function Staff() {
  const session = useSession();
  const [role, reloadRole] = useRole(session.address);
  const [code, setCode] = useState('');
  const [action, run] = useAction();

  if (role.state !== 'role' || !role.business) {
    return <><AppBar title={t('loyalty.staffTitle')} eyebrow="" back /><main className="y-screen"><p className="note">{t('loyalty.businessOnly')}</p></main></>;
  }
  const j = role.jeweler;
  const me = session.address!;

  async function add() {
    const who = code.trim();
    const msgs = [setJewelStaff(me, j.id, [who], [])];
    // The business pays its own fees: x/feegrant refuses a grant to oneself,
    // and that refusal would fail the whole transaction.
    if (who !== me) {
      const has = await feeAllowanceExists(me, who);
      if (!(has.ok && has.value)) msgs.push(grantStaffFees(me, who, STAFF_BUDGET, new Date(Date.now() + 365 * 86_400_000)));
    }
    if (await run(() => session.submit(msgs, null))) { setCode(''); reloadRole(); }
  }

  async function remove(who: string) {
    const msgs = [setJewelStaff(me, j.id, [], [who])];
    const has = await feeAllowanceExists(me, who);
    if (has.ok && has.value) msgs.push(revokeFeeAllowance(me, who));
    if (await run(() => session.submit(msgs, null))) reloadRole();
  }

  return (
    <>
      <AppBar title={t('loyalty.staffTitle')} eyebrow={j.name} back />
      <main className="y-screen">
        <UnlockGate>
          <Section title={t('loyalty.staffNow')}>
            {j.staff.length === 0 && <p className="hint">{t('loyalty.noStaff')}</p>}
            <ul className="y-cards">
              {j.staff.map((s) => (
                <li key={s} className="y-card benefit">
                  <span className="inscription">{short(s)}{s === me ? ` · ${t('loyalty.you')}` : ''}</span>
                  <button type="button" className="btn btn--danger" disabled={action.kind === 'busy'} onClick={() => remove(s)}>{t('loyalty.remove')}</button>
                </li>
              ))}
            </ul>
          </Section>
          <Section title={t('loyalty.addStaff')}>
            <AccountCodeInput id="staff-code" value={code} onChange={setCode} />
            <p className="hint">{t('loyalty.addStaffExplain')}</p>
            <button type="button" className="btn btn--primary" disabled={action.kind === 'busy' || !isAccountCode(code)} onClick={add}>
              {t('loyalty.addStaffButton')}
            </button>
          </Section>
          <ActionStatus state={action} />
          <details className="advanced"><summary>{t('loyalty.advanced')}</summary><pre>{JSON.stringify(j, null, 2)}</pre></details>
        </UnlockGate>
      </main>
    </>
  );
}
```

- [ ] **Step 8: Wire the Atelier in**

In `clients/loyalty/src/App.tsx`:
- `TabBar` reads `useSession()` and `useRole(session.address)`. When the role state is `'role'`, it adds `{ to: '/atelier', end: false, label: t('loyalty.navAtelier') }` before Account.
- Add the routes `/atelier`, `/atelier/register`, `/atelier/jewel/:jewel`, `/atelier/jewel/:jewel/amend` and `/atelier/staff`.
- `AtelierJewel` and `Amend` read the jewel id from the param `jewel`.
- In `Atelier.tsx`'s actions, add a `Link` to `/atelier/staff` (`loyalty.staffLink`) for the business.

Append to `styles.css`:

```css
.stone-row { display: grid; grid-template-columns: 1fr 5rem 4.5rem auto; gap: var(--sp-1); align-items: center; }
.stone-row .bigfield { font-size: 1rem; padding: 0.5rem; }
.hint--bad { color: var(--bad); }
textarea.bigfield { resize: vertical; font: inherit; }
```

- [ ] **Step 9: Add the catalogue keys**

Add these in all five locales:

| key | en | fr |
|---|---|---|
| loyalty.navAtelier | Atelier | Atelier |
| loyalty.atelierTitle | Atelier | Atelier |
| loyalty.notAJeweler | This is where a jeweler's staff work. Your account is not on any jeweler's staff. | C'est ici que travaille le personnel d'un joaillier. Votre compte ne fait partie d'aucun personnel. |
| loyalty.jewelerActive | Active | Actif |
| loyalty.jewelerSuspended | Suspended | Suspendu |
| loyalty.youAreBusiness | Business account | Compte de la maison |
| loyalty.suspendedNote | This jeweler is suspended. It cannot register or sell jewels. Its jewels and their records stay, and its benefits are shown as no longer honoured. | Ce joaillier est suspendu. Il ne peut plus enregistrer ni vendre de bijoux. Ses bijoux et leurs fiches restent, et ses avantages apparaissent comme n'étant plus honorés. |
| loyalty.businessAddYourself | To register jewels yourself, add your own account code under Staff. | Pour enregistrer vous-même des bijoux, ajoutez votre propre code de compte sous Personnel. |
| loyalty.findJewel | Find one of your jewels | Retrouver un de vos bijoux |
| loyalty.notOurs | None of your jewels has this code. | Aucun de vos bijoux n'a ce code. |
| loyalty.stock | In stock | En stock |
| loyalty.noStock | Nothing in stock. Registered jewels appear here until they are sold. | Rien en stock. Les bijoux enregistrés apparaissent ici jusqu'à leur vente. |
| loyalty.registerTitle | Register a jewel | Enregistrer un bijou |
| loyalty.staffOnly | Only staff of an active jeweler can do this. | Seul le personnel d'un joaillier actif peut faire cela. |
| loyalty.codeMustMatch | Type the code exactly as it is engraved on the jewel. A buyer will compare the two. | Saisissez le code exactement comme il est gravé sur le bijou. L'acheteur comparera les deux. |
| loyalty.photos | Photos | Photos |
| loyalty.photosHint | {n} chosen. Include one close-up of the engraving. | {n} choisie(s). Incluez un gros plan de la gravure. |
| loyalty.clearPhotos | Start the photos again | Recommencer les photos |
| loyalty.details | Details | Détails |
| loyalty.fieldTitle | What it is, in a few words | Ce que c'est, en quelques mots |
| loyalty.fieldWeight | Weight in grams | Poids en grammes |
| loyalty.stoneKind | Stone | Pierre |
| loyalty.stoneCarat | Carats | Carats |
| loyalty.stoneCount | How many | Combien |
| loyalty.addStone | Add a stone | Ajouter une pierre |
| loyalty.removeStone | Remove this stone | Retirer cette pierre |
| loyalty.registerButton | Register it | L'enregistrer |
| loyalty.alreadyRegistered | You already registered this code, as jewel no. {id}. | Vous avez déjà enregistré ce code, comme bijou n° {id}. |
| loyalty.uploadFailed | A file could not be stored: {error} | Un fichier n'a pas pu être enregistré : {error} |
| loyalty.stepPhotos | Storing photo {n} of {of}… | Enregistrement de la photo {n} sur {of}… |
| loyalty.stepRecord | Storing the record… | Enregistrement de la fiche… |
| loyalty.sell | Sell it | Le vendre |
| loyalty.soldOn | Sold. It changed hands last on {date}. | Vendu. Dernier changement de mains le {date}. |
| loyalty.notInStock | Not in your stock. | Pas dans votre stock. |
| loyalty.correctRecord | Correct the record | Corriger la fiche |
| loyalty.correctExplain | A correction is added beside the original. Both stay visible on the jewel's page. | Une correction s'ajoute à côté de l'original. Les deux restent visibles sur la page du bijou. |
| loyalty.photosKept | The {n} existing photos are kept. | Les {n} photos existantes sont conservées. |
| loyalty.addPhotos | Add photos | Ajouter des photos |
| loyalty.correctionNote | What was corrected, and why | Ce qui a été corrigé, et pourquoi |
| loyalty.noteBytes | {n} of {max} | {n} sur {max} |
| loyalty.saveCorrection | Add the correction | Ajouter la correction |
| loyalty.staffTitle | Staff | Personnel |
| loyalty.businessOnly | Only the jeweler's business account can do this. | Seul le compte de la maison peut faire cela. |
| loyalty.staffNow | Your staff | Votre personnel |
| loyalty.noStaff | Nobody yet. | Personne pour l'instant. |
| loyalty.you | you | vous |
| loyalty.remove | Remove | Retirer |
| loyalty.addStaff | Add someone | Ajouter quelqu'un |
| loyalty.addStaffExplain | They show their account code from the Account tab. Your business pays the network fees for their work, and only for that. | La personne montre son code de compte depuis l'onglet Compte. Votre maison paie les frais de réseau de son travail, et seulement ceux-là. |
| loyalty.addStaffButton | Add to staff | Ajouter au personnel |
| loyalty.staffLink | Staff | Personnel |

- [ ] **Step 10: Test, build, and register a real jewel**

Run: `cd clients && npm test --workspace @yamale/chain && npm test --workspace @yamale/loyalty && npm run build --workspace @yamale/loyalty`

Then, with loyalty-api running:
1. Admit account A (`localhost:5390`) as a jeweler: `scripts/devnet/loyalty-admit.sh "Maison Test" <A's code>`. After a reload, A has the Atelier tab, marked as the business account, with the note to add itself as staff.
2. Under Staff, A adds its own code. No allowance is granted, since a business pays its own fees. The chain now lists A as staff, and Register appears.
3. Add B's code (`127.0.0.1:5390`) as staff, then remove it. Each change shows on the list and on the chain (`blockchaind query jewel jeweler <id>`). Removing B also revokes B's allowance: `blockchaind query feegrant grant <A> <B>` answers not found.
4. Register a jewel with two photos. Its Atelier page and its public page show the photos, the hallmark and the details.
5. Sell it by link, and take it on another origin.
6. Register a second jewel and correct its record. Its public page then shows "Corrected …" and keeps the original record one tap away.
7. Check the Atelier, Register and Staff screens at both widths, in both themes.

If adding A to its own staff is refused because the chain does not let a business be its own staff, say so in the report. Then add B as staff and do steps 4 to 6 as B.

- [ ] **Step 11: Commit**

```bash
git add clients/loyalty clients/sdk/src/catalogues.ts
git commit -m "loyalty: the Atelier: register a jewel with photos, stock, sell by link or to an account, correct a record"
```

---

### Task 10: The Atelier, part 2: benefits, redeeming codes, the jeweler's look, applications

**Files:**
- Create: `clients/loyalty/src/atelier/Benefits.tsx`, `Redeem.tsx`, `Profile.tsx`, `Apply.tsx`
- Modify: `clients/loyalty/src/atelier/AtelierJewel.tsx` (benefit section), `clients/loyalty/src/atelier/Atelier.tsx` (links), `clients/loyalty/src/api.ts` (`saveProfile`, `submitApplication`), `clients/loyalty/src/App.tsx`, `clients/sdk/src/catalogues.ts`

**Interfaces:**
- Consumes:
  - `attachJewelBenefit`, `redeemJewelBenefit`, `decodeVoucherCode`, `voucherSigner`, `setJewelStaff`, `grantStaffFees`, `revokeFeeAllowance`, `feeAllowanceExists`, `chainHead`, `readJewel`, `sha256Hex`, `signedHeaders`.
  - `useRole`, `preparePhoto`, `uploadFile`, `AccountCodeInput`, `BenefitList`, `benefitKind`, `benefitState`.
- Produces:

  ```ts
  // api.ts (added)
  export function saveProfile(key: Uint8Array, jewelerId: string, profile: JewelerProfile, chainId: string):
    Promise<{ ok: true } | { ok: false; status: number; error: string }>
  export interface Application { business: string; name: string; contact: string; email: string; phone: string;
    country: string; message: string }
  export function submitApplication(key: Uint8Array, application: Application, chainId: string):
    Promise<{ ok: true } | { ok: false; status: number; error: string }>
  ```

- [ ] **Step 1: Add the two API calls**

Append to `clients/loyalty/src/api.ts`:

```ts
async function signedSend(method: 'PUT' | 'POST', path: string, key: Uint8Array, purpose: 'profile' | 'application',
  subject: string, body: Uint8Array, chainId: string): Promise<{ ok: true } | { ok: false; status: number; error: string }> {
  try {
    const res = await fetch(`${API}${path}`, {
      method, headers: { ...signedHeaders(key, purpose, chainId, subject), 'Content-Type': 'application/json' }, body,
    });
    if (res.ok) return { ok: true };
    const b = await res.json().catch(() => ({}));
    return { ok: false, status: res.status, error: String(b.error ?? '') };
  } catch (err) {
    return { ok: false, status: 0, error: err instanceof Error ? err.message : String(err) };
  }
}

export function saveProfile(key: Uint8Array, jewelerId: string, profile: JewelerProfile, chainId: string) {
  const body = new TextEncoder().encode(JSON.stringify({ logo: profile.logo, colour: profile.colour, accent: profile.accent }));
  return signedSend('PUT', `/v1/jewelers/${jewelerId}/profile`, key, 'profile', `${jewelerId}.${sha256Hex(body)}`, body, chainId);
}

export interface Application {
  business: string; name: string; contact: string; email: string; phone: string; country: string; message: string;
}

export function submitApplication(key: Uint8Array, a: Application, chainId: string) {
  const body = new TextEncoder().encode(JSON.stringify({
    business: a.business, name: a.name, contact: a.contact, email: a.email, phone: a.phone, country: a.country, message: a.message,
  }));
  return signedSend('POST', '/v1/applications', key, 'application', sha256Hex(body), body, chainId);
}
```

- [ ] **Step 2: Write benefits and redemption**

`clients/loyalty/src/atelier/Benefits.tsx`:

```tsx
import { useState } from 'react';
import { attachJewelBenefit, t, type JewelView } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { BenefitList } from '../verify/JewelPage.tsx';

const KINDS = ['cleaning', 'resizing', 'polishing', 'rhodium', 'repair'];
const PERIODS = [
  { seconds: 31_536_000, key: 'loyalty.perYear' },
  { seconds: 2_592_000, key: 'loyalty.perMonth' },
  { seconds: 0, key: 'loyalty.wholeLife' },
];

export function AttachBenefit({ view, business, onChange }: { view: JewelView; business: string; onChange: () => void }) {
  const session = useSession();
  const [action, run] = useAction();
  const [kind, setKind] = useState('cleaning');
  const [custom, setCustom] = useState('');
  const [allowance, setAllowance] = useState(2);
  const [period, setPeriod] = useState(31_536_000);
  const [until, setUntil] = useState('');
  const finalKind = kind === 'custom' ? custom.trim() : kind;
  const expiresAt = until ? Math.floor(Date.parse(`${until}T23:59:59Z`) / 1000) : 0;
  return (
    <UnlockGate>
      <div className="stack">
        <label className="y-label" htmlFor="bkind">{t('loyalty.benefitWhat')}</label>
        <select id="bkind" className="bigfield" value={kind} onChange={(e) => setKind(e.target.value)}>
          {KINDS.map((k) => <option key={k} value={k}>{t(`loyalty.benefitKind.${k}`)}</option>)}
          <option value="custom">{t('loyalty.benefitOther')}</option>
        </select>
        {kind === 'custom' && <input className="bigfield" maxLength={80} value={custom} onChange={(e) => setCustom(e.target.value)} aria-label={t('loyalty.benefitOther')} />}
        <label className="y-label" htmlFor="ballow">{t('loyalty.benefitHowMany')}</label>
        <input id="ballow" className="bigfield num" type="number" min={1} max={100} value={allowance}
               onChange={(e) => setAllowance(Math.max(1, Number(e.target.value) || 1))} />
        <label className="y-label" htmlFor="bperiod">{t('loyalty.benefitEvery')}</label>
        <select id="bperiod" className="bigfield" value={period} onChange={(e) => setPeriod(Number(e.target.value))}>
          {PERIODS.map((p) => <option key={p.seconds} value={p.seconds}>{t(p.key)}</option>)}
        </select>
        <label className="y-label" htmlFor="buntil">{t('loyalty.benefitUntilLabel')}</label>
        <input id="buntil" className="bigfield" type="date" value={until} onChange={(e) => setUntil(e.target.value)} />
        <button type="button" className="btn btn--primary" disabled={action.kind === 'busy' || finalKind === ''} onClick={async () => {
          if (await run(() => session.submit([attachJewelBenefit(session.address!, view.jewel.id, finalKind, allowance, period, expiresAt)], business))) onChange();
        }}>{t('loyalty.attachBenefit')}</button>
        <ActionStatus state={action} done={t('loyalty.benefitAttached')} />
      </div>
    </UnlockGate>
  );
}

export function JewelBenefits({ view, active, now }: { view: JewelView; active: boolean; now: number }) {
  return <BenefitList benefits={view.benefits} issuerActive={active} now={now} />;
}
```

`clients/loyalty/src/atelier/Redeem.tsx`:

```tsx
/**
 * Redeeming a benefit: the owner shows a code, the staff scan it.
 *
 * Everything the chain will check is checked here first, so a refusal is
 * explained at the counter rather than discovered after signing: whose
 * benefit it is, who signed the code, whether it is still fresh, and whether
 * any uses are left.
 */

import { useState } from 'react';
import {
  chainHead, decodeVoucherCode, readJewel, redeemJewelBenefit, t, voucherSigner, type VoucherCode,
} from '@yamale/chain';

import { Scanner } from '../../../app/src/scan.tsx';
import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { CHAIN_ID } from '../config.ts';
import { AppBar, Section } from '../ui.tsx';
import { benefitState } from '../verify/history.ts';
import { benefitKind, Hallmark } from '../verify/JewelPage.tsx';
import { useRole } from './role.ts';

type Check =
  | { kind: 'idle' } | { kind: 'checking' }
  | { kind: 'refused'; key: string }
  | { kind: 'ready'; code: VoucherCode; inscription: string; benefitKind: string; left: number };

export function Redeem() {
  const session = useSession();
  const [role] = useRole(session.address);
  const [text, setText] = useState('');
  const [scanning, setScanning] = useState(false);
  const [check, setCheck] = useState<Check>({ kind: 'idle' });
  const [action, run] = useAction();

  if (role.state !== 'role' || !role.staff || !role.active) {
    return <><AppBar title={t('loyalty.redeemTitle')} eyebrow="" back /><main className="y-screen"><p className="note">{t('loyalty.staffOnly')}</p></main></>;
  }
  const jeweler = role.jeweler;

  async function inspect(raw: string) {
    setText(raw);
    const code = decodeVoucherCode(raw);
    if (!code) { setCheck({ kind: 'refused', key: 'loyalty.voucherUnreadable' }); return; }
    setCheck({ kind: 'checking' });
    const [view, head] = await Promise.all([readJewel(code.voucher.jewelId), chainHead()]);
    if (!view.ok || !head.ok) { setCheck({ kind: 'refused', key: 'loyalty.problemUnreachable' }); return; }
    const benefit = view.value.benefits.find((b) => b.id === code.voucher.benefitId);
    if (!benefit || benefit.jewelerId !== jeweler.id) { setCheck({ kind: 'refused', key: 'loyalty.voucherNotOurs' }); return; }
    if (voucherSigner(code, CHAIN_ID) !== view.value.owner) { setCheck({ kind: 'refused', key: 'loyalty.voucherNotOwner' }); return; }
    if (Number(code.voucher.expiresAt) <= head.value.time) { setCheck({ kind: 'refused', key: 'loyalty.voucherStale' }); return; }
    if (view.value.jewel.stolen) { setCheck({ kind: 'refused', key: 'loyalty.err.stolen' }); return; }
    const s = benefitState(benefit, true, head.value.time);
    if (s.state !== 'active' || s.left === 0) { setCheck({ kind: 'refused', key: 'loyalty.err.allowanceUsed' }); return; }
    setCheck({ kind: 'ready', code, inscription: view.value.jewel.inscription, benefitKind: benefit.kind, left: s.left });
  }

  return (
    <>
      <AppBar title={t('loyalty.redeemTitle')} eyebrow={jeweler.name} back />
      <main className="y-screen">
        <UnlockGate>
          <Section title={t('loyalty.redeemScan')}>
            {scanning
              ? <Scanner onRead={(s) => { setScanning(false); inspect(s); }} onClose={() => setScanning(false)} />
              : <button type="button" className="btn btn--primary" onClick={() => setScanning(true)}>{t('loyalty.scanCode')}</button>}
            <label className="y-label" htmlFor="vtext">{t('loyalty.orPaste')}</label>
            <textarea id="vtext" className="bigfield" rows={3} value={text} onChange={(e) => inspect(e.target.value)} spellCheck={false} />
          </Section>
          {check.kind === 'checking' && <p className="loading">{t('loyalty.loading')}</p>}
          {check.kind === 'refused' && <p className="note note--warn" role="alert">{t(check.key)}</p>}
          {check.kind === 'ready' && (
            <div className="stack">
              <Hallmark code={check.inscription} />
              <p>{t('loyalty.redeemSummary', { kind: benefitKind(check.benefitKind), left: check.left })}</p>
              <button type="button" className="btn btn--primary" disabled={action.kind === 'busy' || action.kind === 'done'} onClick={() =>
                run(() => session.submit([redeemJewelBenefit(session.address!, check.code)], jeweler.business))}>
                {t('loyalty.redeemConfirm')}
              </button>
              <ActionStatus state={action} done={t('loyalty.redeemDone')} />
            </div>
          )}
        </UnlockGate>
      </main>
    </>
  );
}
```

- [ ] **Step 3: Write the look, and applications**

`clients/loyalty/src/atelier/Profile.tsx`:

```tsx
import { useEffect, useState } from 'react';
import { t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { fetchProfile, saveProfile, uploadFile } from '../api.ts';
import { CHAIN_ID } from '../config.ts';
import { AppBar, Section } from '../ui.tsx';
import { Hallmark } from '../verify/JewelPage.tsx';
import { VerifiedImage } from '../verify/Photos.tsx';
import { preparePhoto } from './photo.ts';
import { useRole } from './role.ts';

export function Profile() {
  const session = useSession();
  const [role] = useRole(session.address);
  const [logo, setLogo] = useState('');
  const [colour, setColour] = useState('#12253f');
  const [accent, setAccent] = useState('#a87b3c');
  const [state, setState] = useState<'idle' | 'busy' | 'saved' | string>('idle');
  const id = role.state === 'role' ? role.jeweler.id : null;

  useEffect(() => {
    if (!id) return;
    fetchProfile(id).then((p) => { if (p) { setLogo(p.logo); setColour(p.colour); setAccent(p.accent); } });
  }, [id]);

  if (role.state !== 'role' || !role.business) {
    return <><AppBar title={t('loyalty.lookTitle')} eyebrow="" back /><main className="y-screen"><p className="note">{t('loyalty.businessOnly')}</p></main></>;
  }

  return (
    <>
      <AppBar title={t('loyalty.lookTitle')} eyebrow={role.jeweler.name} back />
      <main className="y-screen">
        <UnlockGate>
          <div className="jewel__mast" style={{ ['--jeweler-colour' as string]: colour, ['--jeweler-accent' as string]: accent }}>
            {logo && <VerifiedImage hash={logo} alt={role.jeweler.name} className="jewel__logo" />}
            <p className="jewel__by">{t('loyalty.registeredBy', { name: role.jeweler.name })}</p>
            <Hallmark code="MD20260001" />
          </div>
          <Section title={t('loyalty.logo')}>
            <input type="file" accept="image/*" aria-label={t('loyalty.logo')} onChange={async (e) => {
              const f = e.target.files?.[0];
              if (!f) return;
              setState('busy');
              const up = await uploadFile(session.withKey((k) => k), await preparePhoto(f, 600, 0.9), CHAIN_ID);
              setState(up.ok ? 'idle' : up.error);
              if (up.ok) setLogo(up.hash);
            }} />
          </Section>
          <Section title={t('loyalty.colours')}>
            <label className="y-label">{t('loyalty.colourMain')} <input type="color" value={colour} onChange={(e) => setColour(e.target.value)} /></label>
            <label className="y-label">{t('loyalty.colourAccent')} <input type="color" value={accent} onChange={(e) => setAccent(e.target.value)} /></label>
          </Section>
          {state !== 'idle' && state !== 'busy' && state !== 'saved' && <p className="note note--warn" role="alert">{state}</p>}
          {state === 'saved' && <p className="banner banner--ok">{t('loyalty.lookSaved')}</p>}
          <button type="button" className="btn btn--primary" disabled={state === 'busy'} onClick={async () => {
            setState('busy');
            const out = await saveProfile(session.withKey((k) => k), role.jeweler.id, { logo, colour, accent }, CHAIN_ID);
            setState(out.ok ? 'saved' : out.error);
          }}>{t('loyalty.saveLook')}</button>
        </UnlockGate>
      </main>
    </>
  );
}
```

`clients/loyalty/src/atelier/Apply.tsx`:

```tsx
/**
 * A business applies from the account that will be its own. The office
 * reviews applications and admits a jeweler by a decision of its members;
 * nothing here admits anybody.
 */

import { useState, type FormEvent } from 'react';
import { t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { submitApplication } from '../api.ts';
import { CHAIN_ID } from '../config.ts';
import { AppBar } from '../ui.tsx';

export function Apply() {
  const session = useSession();
  const [f, setF] = useState({ name: '', contact: '', email: '', phone: '', country: '', message: '' });
  const [state, setState] = useState<'idle' | 'busy' | 'sent' | string>('idle');
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value });

  async function submit(e: FormEvent) {
    e.preventDefault();
    setState('busy');
    const out = await submitApplication(session.withKey((k) => k),
      { business: session.address!, ...f, country: f.country.trim().toUpperCase() }, CHAIN_ID);
    setState(out.ok ? 'sent' : (out.error || t('loyalty.err.unknown')));
  }

  return (
    <>
      <AppBar title={t('loyalty.applyTitle')} eyebrow={t('loyalty.title')} back />
      <main className="y-screen">
        <UnlockGate>
          {state === 'sent' ? <p className="banner banner--ok">{t('loyalty.applySent')}</p> : (
            <form className="stack" onSubmit={submit}>
              <p className="hint">{t('loyalty.applyLede')}</p>
              {(['name', 'contact', 'email', 'phone', 'country'] as const).map((k) => (
                <div key={k} className="stack">
                  <label className="y-label" htmlFor={`a-${k}`}>{t(`loyalty.apply.${k}`)}</label>
                  <input id={`a-${k}`} className="bigfield" value={f[k]} onChange={set(k)}
                         type={k === 'email' ? 'email' : 'text'} maxLength={k === 'country' ? 2 : 254} required={k !== 'phone'} />
                </div>
              ))}
              <label className="y-label" htmlFor="a-message">{t('loyalty.apply.message')}</label>
              <textarea id="a-message" className="bigfield" rows={4} maxLength={2000} value={f.message} onChange={set('message')} />
              {state !== 'idle' && state !== 'busy' && <p className="note note--warn" role="alert">{state}</p>}
              <button className="btn btn--primary" disabled={state === 'busy'}>{t('loyalty.applyButton')}</button>
            </form>
          )}
        </UnlockGate>
      </main>
    </>
  );
}
```

- [ ] **Step 4: Wire them in**

- In `AtelierJewel.tsx`, after the sell or sold block, add a benefits section. It shows `<JewelBenefits view={v} active={role.active} now={…chainHead time…} />`, and, for active staff, `<AttachBenefit view={v} business={role.jeweler.business} onChange={reload} />`. Read the head with `useLoad(() => chainHead(), [])`.
- In `Atelier.tsx`'s actions:
  - for active staff, add `Link`s to `/atelier/redeem` ("Redeem a benefit code");
  - for the business, add a `Link` to `/atelier/look` (`loyalty.lookLink`). Task 9 already links Staff.
- In `App.tsx`, add the routes `/atelier/redeem`, `/atelier/look` and `/apply`.

- [ ] **Step 5: Add the catalogue keys**

Add these in all five locales:

| key | en | fr |
|---|---|---|
| loyalty.benefitWhat | What it offers | Ce qu'il offre |
| loyalty.benefitOther | Something else | Autre chose |
| loyalty.benefitHowMany | How many times | Combien de fois |
| loyalty.benefitEvery | Per | Par |
| loyalty.perYear | Year | Année |
| loyalty.perMonth | Month | Mois |
| loyalty.wholeLife | For the jewel's whole life | Pour toute la vie du bijou |
| loyalty.benefitUntilLabel | Ends on (leave empty for never) | Se termine le (vide pour jamais) |
| loyalty.attachBenefit | Attach the benefit | Attacher l'avantage |
| loyalty.benefitAttached | Attached. It travels with the jewel to every owner. | Attaché. Il suit le bijou chez chaque propriétaire. |
| loyalty.redeemTitle | Redeem a benefit code | Utiliser un code d'avantage |
| loyalty.redeemScan | The owner's code | Le code du propriétaire |
| loyalty.orPaste | Or paste it | Ou collez-le |
| loyalty.voucherUnreadable | That is not a benefit code. | Ce n'est pas un code d'avantage. |
| loyalty.voucherNotOurs | This benefit was not issued by your jewelry. | Cet avantage n'a pas été émis par votre maison. |
| loyalty.voucherNotOwner | This code was not made by the jewel's owner. | Ce code n'a pas été créé par le propriétaire du bijou. |
| loyalty.voucherStale | This code has lapsed. Ask the owner to show it again. | Ce code a expiré. Demandez au propriétaire de l'afficher à nouveau. |
| loyalty.redeemSummary | {kind}: {left} left before this use. | {kind} : {left} restant(s) avant cette utilisation. |
| loyalty.redeemConfirm | Record this use | Enregistrer cette utilisation |
| loyalty.redeemDone | Recorded. | Enregistré. |
| loyalty.lookTitle | How your jewels look | L'apparence de vos bijoux |
| loyalty.logo | Logo | Logo |
| loyalty.colours | Colours | Couleurs |
| loyalty.colourMain | Main | Principale |
| loyalty.colourAccent | Accent | Accent |
| loyalty.saveLook | Save | Enregistrer |
| loyalty.lookSaved | Saved. Your jewels' pages use it now. | Enregistré. Les pages de vos bijoux l'utilisent désormais. |
| loyalty.applyTitle | Register your jewels with us | Enregistrer vos bijoux chez nous |
| loyalty.applyLede | Apply from the account that will be your jewelry's own. The platform reviews each application, and its office admits jewelers by a decision of several members. | Faites la demande depuis le compte qui sera celui de votre maison. La plateforme examine chaque demande, et son bureau admet les joailliers par une décision de plusieurs membres. |
| loyalty.apply.name | Name of the jewelry | Nom de la maison |
| loyalty.apply.contact | Your name | Votre nom |
| loyalty.apply.email | Email | E-mail |
| loyalty.apply.phone | Phone (optional) | Téléphone (facultatif) |
| loyalty.apply.country | Country (two letters, e.g. SN) | Pays (deux lettres, ex. SN) |
| loyalty.apply.message | Anything we should know | Ce que nous devrions savoir |
| loyalty.applyButton | Send the application | Envoyer la demande |
| loyalty.applySent | Received. We will reply at the email you gave. | Reçue. Nous répondrons à l'e-mail indiqué. |
| loyalty.redeemLink | Redeem a benefit code | Utiliser un code d'avantage |
| loyalty.lookLink | How your jewels look | L'apparence de vos bijoux |

- [ ] **Step 6: Test, build, and check**

Run: `cd clients && npm test --workspace @yamale/chain && npm test --workspace @yamale/loyalty && npm run build --workspace @yamale/loyalty`

In the browser, as business A, who is its own staff since Task 9:
1. Set a logo and colours. A's jewels' public pages use them.
2. Attach "Cleaning, 2 per year" to the jewel B took in Task 9.
3. B shows a code; A pastes it on Redeem. The page reads "2 left before this use" and records it; B's page then shows 1 left.
4. Paste the same code again. The app's own checks may pass it, since the use is already on the chain; the chain refuses it, and the app says "This code has already been used".
5. On origin C (`[::1]:5390`), make an account and apply. Then list the applications with `curl -H "Authorization: Bearer $(cat .devnet/local/loyalty/operator-token)" http://127.0.0.1:8095/v1/applications`. C's application is there, naming C's account.
6. Check Redeem, Look and Apply at both widths, in both themes.

- [ ] **Step 7: Commit**

```bash
git add clients/loyalty clients/sdk/src/catalogues.ts
git commit -m "loyalty: benefits attached and redeemed by code, staff with their fees, the jeweler's look, applications"
```

---

### Task 11: End to end on the local devnet, and the record of what exists

**Files:**
- Create: `clients/loyalty/scripts/smoke.ts`
- Modify: `clients/README.md`, `docs/scope/gaps.md`

**Interfaces:**
- Consumes: everything above, including the devnet scripts from Task 5.
- Produces: a TypeScript smoke test of every chain path, a full walk in the browser, and the backlog entries Plan 3 starts from.

- [ ] **Step 1: Write the TypeScript smoke test of the chain path**

The unit tests pin each format. This script proves that the formats, the registry, the fee granters and the reads all work together on a running chain. It uses real keys and real fee grants, and reaches every chain path the app uses.

`clients/loyalty/scripts/smoke.ts`:

```ts
/**
 * Every chain path the loyalty app uses, driven from TypeScript against the
 * local devnet and loyalty-api. Run after init-local-devnet.sh, seed-jewel.sh
 * and loyalty-local.sh. Each check prints ok or FAIL; any FAIL exits 1.
 */

import { secp256k1 } from '@noble/curves/secp256k1';
import { DirectSecp256k1Wallet } from '@cosmjs/proto-signing';

import { chainHead, setRpcBase } from '../../sdk/src/abci.ts';
import {
  acceptJewelOffer, addressOfPublicKey, attachJewelBenefit, claimJewelOffer, compressedPublicKey, fromHex,
  grantStaffFees, JEWEL_GAS, makeOwnershipProof, makeVoucher, newClaimKey, normaliseInscription,
  offerJewelByLink, offerJewelToAccount, redeemJewelBenefit, registerJewel, verifyOwnershipProof,
} from '../../sdk/src/jewel.ts';
import {
  feeAllowanceExists, findByInscription, jewelsHeldBy, offersAddressedTo, readJewel, readJewelers, roleOf,
} from '../../sdk/src/jewelreads.ts';
import { sha256Hex, signedHeaders } from '../../sdk/src/loyaltyauth.ts';
import { ChainSigner, type EncodeObject, type SubmitResult } from '../../sdk/src/signing.ts';

function env(name: string): string {
  const v = process.env[name];
  if (!v) { console.error(`${name} is required`); process.exit(2); }
  return v;
}
const RPC = env('RPC');
const API = env('API').replace(/\/+$/, '');
const CHAIN = env('CHAIN');
setRpcBase(RPC);
const business = fromHex(env('BUSINESS_KEY'));
const staff = fromHex(env('STAFF_KEY'));
const addr = (k: Uint8Array) => addressOfPublicKey(compressedPublicKey(k));

let failed = 0;
function check(label: string, pass: boolean, detail = ''): void {
  if (pass) { console.log(`  ok    ${label}`); return; }
  failed++;
  console.log(`  FAIL  ${label}${detail ? `: ${detail}` : ''}`);
}

async function send(key: Uint8Array, messages: EncodeObject[], granter?: string): Promise<SubmitResult> {
  const wallet = await DirectSecp256k1Wallet.fromKey(key, 'yml');
  const signer = new ChainSigner(wallet, { rpcUrl: RPC, chainId: CHAIN, ...(granter ? { feeGranter: granter } : {}) });
  try { return await signer.submit(messages, '', JEWEL_GAS); } finally { signer.disconnect(); }
}
const rawOf = (r: SubmitResult) => r.error?.raw ?? r.raw.rawLog ?? '';

function eventValue(r: SubmitResult, type: string, key: string): string {
  for (const e of r.raw.events ?? []) {
    if (e.type !== type) continue;
    const a = e.attributes.find((x) => x.key === key);
    if (a) { try { return String(JSON.parse(a.value)); } catch { return a.value; } }
  }
  return '';
}

async function upload(key: Uint8Array, bytes: Uint8Array): Promise<string | null> {
  const hash = sha256Hex(bytes);
  const res = await fetch(`${API}/v1/files`, { method: 'POST', headers: signedHeaders(key, 'file', CHAIN, hash), body: bytes });
  const body = await res.json().catch(() => ({}));
  return res.status === 201 && body.hash === hash ? hash : null;
}

async function sponsor(key: Uint8Array): Promise<{ status: number; body: any }> {
  const res = await fetch(`${API}/v1/sponsor`, { method: 'POST', headers: signedHeaders(key, 'sponsor', CHAIN, addr(key)) });
  return { status: res.status, body: await res.json().catch(() => ({})) };
}

async function chainNow(): Promise<number> {
  const h = await chainHead();
  if (!h.ok) throw new Error(`no chain head: ${h.detail}`);
  return h.value.time;
}

const bizAddr = addr(business);
const staffAddr = addr(staff);

console.log("1  staff fees are the business's");
const has = await feeAllowanceExists(bizAddr, staffAddr);
if (has.ok && has.value) {
  check('the staff allowance is already there', true);
} else {
  const g = await send(business, [grantStaffFees(bizAddr, staffAddr, [{ denom: 'uyml', amount: '5000000' }], new Date(Date.now() + 86_400_000))]);
  check('the business granted its staff an allowance', g.succeeded, rawOf(g));
}
const jewelers = await readJewelers();
const role = jewelers.ok ? roleOf(staffAddr, jewelers.value) : null;
check('the staff key is staff of a jeweler', role?.staff === true);
const jewelerId = role?.jeweler.id ?? '1';

console.log('2  files');
const png = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, ...new Array<number>(64).fill(7)]);
const photo = await upload(staff, png);
check('a photo is stored under its fingerprint', photo !== null);
const code = `SMOKE-${Math.floor(Date.now() / 1000)}`;
const normalised = normaliseInscription(code)!;
const record = new TextEncoder().encode(JSON.stringify({
  version: 1, jewelerId, inscription: normalised, title: 'Smoke ring', metal: 'silver', weightGrams: '1', stones: [],
  origin: { workshop: 'Dakar', stoneSource: '', labCertificate: '' }, photos: [photo], createdAt: new Date().toISOString(),
}));
const recordHash = await upload(staff, record);
check('the record is stored under its fingerprint', recordHash !== null);

console.log('3  registration');
const reg = await send(staff, [registerJewel(staffAddr, code, recordHash!, `${API}/v1/files/${recordHash}`)], bizAddr);
check('staff registered a jewel, the business paying', reg.succeeded, rawOf(reg));
const jewelId = eventValue(reg, 'blockchain.jewel.v1.EventJewelRegistered', 'jewel_id');
check('the registration names the jewel', jewelId !== '');
const found = await findByInscription(normalised);
check('the engraved code finds it', found.ok && found.value.some((j) => j.id === jewelId));

console.log('4  sponsorship');
const buyer = secp256k1.utils.randomPrivateKey();
const first = await sponsor(buyer);
check('a new account is sponsored', first.status === 200 && first.body.granted === true, JSON.stringify(first.body));
const granter = String(first.body.granter);
const repeat = await sponsor(buyer);
check('only once', repeat.status === 200 && repeat.body.already === true, JSON.stringify(repeat.body));

console.log('5  a claim by link');
const { privateKey: claimPriv, claimKey } = newClaimKey();
const byLink = await send(staff, [offerJewelByLink(staffAddr, jewelId, claimKey)], bizAddr);
check('staff offered it by link', byLink.succeeded, rawOf(byLink));
const claim = await send(buyer, [claimJewelOffer(addr(buyer), jewelId, claimPriv, CHAIN)], granter);
check('the buyer claimed it, the platform paying', claim.succeeded, rawOf(claim));
let view = await readJewel(jewelId);
check('the buyer owns it, as owner 1', view.ok && view.value.owner === addr(buyer) && view.value.jewel.ownerCount === 1);

console.log('6  a benefit used once');
const attach = await send(staff, [attachJewelBenefit(staffAddr, jewelId, 'cleaning', 2, 31_536_000, 0)], bizAddr);
const benefitId = eventValue(attach, 'blockchain.jewel.v1.EventBenefitAttached', 'benefit_id');
check('a cleaning benefit is attached', attach.succeeded && benefitId !== '', rawOf(attach));
const voucher = makeVoucher(buyer, CHAIN, benefitId, jewelId, await chainNow());
const use = await send(staff, [redeemJewelBenefit(staffAddr, voucher)], bizAddr);
check("the owner's code is redeemed", use.succeeded, rawOf(use));
const again = await send(staff, [redeemJewelBenefit(staffAddr, voucher)], bizAddr);
check('the same code is refused', !again.succeeded && rawOf(again).includes('already been used'), rawOf(again));
view = await readJewel(jewelId);
check('one use is recorded', view.ok && view.value.benefits.find((b) => b.id === benefitId)?.uses.length === 1);

console.log('7  a resale found by tx_search');
const next = secp256k1.utils.randomPrivateKey();
check('a second account is sponsored', (await sponsor(next)).status === 200);
const toNext = await send(buyer, [offerJewelToAccount(addr(buyer), jewelId, addr(next))], granter);
check('the buyer offered it to the second account', toNext.succeeded, rawOf(toNext));
let seen = false;
for (let i = 0; i < 10 && !seen; i++) {
  const offers = await offersAddressedTo(addr(next), await chainNow());
  seen = offers.ok && offers.value.some((v) => v.jewel.id === jewelId);
  if (!seen) await new Promise((r) => setTimeout(r, 1000));
}
check('the second account finds the offer', seen);
const accept = await send(next, [acceptJewelOffer(addr(next), jewelId)], granter);
check('and accepts it', accept.succeeded, rawOf(accept));
view = await readJewel(jewelId);
check('owner 2, with two dated changes', view.ok && view.value.owner === addr(next) && view.value.jewel.ownerSince.length === 2);

console.log('8  ownership proof');
const proof = verifyOwnershipProof(CHAIN, jewelId, makeOwnershipProof(next, CHAIN, jewelId, await chainNow()));
check('the proof names the owner', proof !== null && view.ok && proof.address === view.value.owner);

console.log("9  the allowance's scope");
const misuse = await send(next, [registerJewel(addr(next), `X-${code}`, recordHash!, `${API}/v1/files/${recordHash}`)], granter);
check("the platform's allowance does not pay for staff work", !misuse.succeeded && rawOf(misuse).includes('allowed messages'), rawOf(misuse));

console.log('10 holdings');
const held = await jewelsHeldBy(addr(next));
check('x/nft lists it under the second account', held.ok && held.value.includes(jewelId));

console.log(failed ? `\n${failed} check(s) failed` : '\nall checks passed');
process.exit(failed ? 1 : 0);
```

If `signing.ts` pulls in a `.tsx` file through its imports, Node cannot load this script. In that case, import what the script needs from narrower modules, rather than from `index.ts`.

Run it:

```bash
cd clients/loyalty
BUSINESS_KEY=$(yes | "$SCRATCH/blockchaind.exe" keys export jw-business --unarmored-hex --unsafe --keyring-backend test --home ../../.devnet/local/node 2>/dev/null | tail -1) \
STAFF_KEY=$(yes | "$SCRATCH/blockchaind.exe" keys export jw-staff --unarmored-hex --unsafe --keyring-backend test --home ../../.devnet/local/node 2>/dev/null | tail -1) \
RPC=http://127.0.0.1:26957 API=http://127.0.0.1:8095 CHAIN=yamale-local-1 \
node --experimental-strip-types scripts/smoke.ts
```

Expected: every line `ok`, then "all checks passed", exit 0. The keys are test-keyring keys of a throwaway local devnet. Do not print them, and do not write them to a file.

- [ ] **Step 2: The whole thing, in the browser**

Start fresh:
1. Run `init-local-devnet.sh`, then `seed-jewel.sh`.
2. Start `loyalty-local.sh` in the background.
3. Start the app with `preview_start`.

Then walk it as three people on three origins: A on `localhost:5390`, B on `127.0.0.1:5390`, C on `[::1]:5390`.

1. A makes an account and applies. Admit A with `loyalty-admit.sh "Maison Test" <A's code>`.
2. The Atelier tab appears for A. A adds A as staff, sets a look, and registers a ring with three photos, one a close-up of the engraving.
3. A sells the ring by link. B opens the link with the host changed to `127.0.0.1`, makes an account, and takes it.
4. A attaches a cleaning benefit. B shows the code; A redeems it.
5. C makes an account and shows its code. B passes the ring to C's account. C accepts under "Offered to you".
6. C reports it stolen: the public page says so for everyone. C clears it.
7. C shares a proof link. A opens it and sees "holds this jewel now".
8. On the public page:
   - the jeweler's look shows;
   - the loupe zooms;
   - "2 owners so far" shows with its dates;
   - the sentence is there;
   - no address or hash appears outside Advanced.
9. Repeat the public page and My jewels at 390 px and 1600 px, in light and dark. Take one screenshot of each width as proof. Check the tab bar, the hallmark and the loupe for overflow.
10. Stop loyalty-api and reload the public page. The register's facts still show, with the note that photos are unavailable.

Fix whatever this finds, in the task it belongs to, and rerun Steps 1 and 2.

- [ ] **Step 3: Record what exists**

`clients/README.md`:
- add `loyalty/` to the tree at the top, with the line `the jewel register: verify, hold, pass on; the jeweler's Atelier`;
- add a short section on how to run it, with the commands from "Running it locally".

`docs/scope/gaps.md`:
- In "Merged, but not yet running", extend the x/jewel row: the app and loyalty-api are built and run locally, with the commit range.
- In the section "x/jewel, to settle before the devnet-2 upgrade fixes its formats (2026-11-13)", add:
  - **The claim-link decision** (Plan 2, departure 1). The link leaves the seller's device and never passes through loyalty-api, so the "sign in with the email the offer went to" safeguard does not exist. Reverse it only knowing that a platform mail relay would hold every unclaimed jewel's key.
  - **Plan 3's deployment list:**
    - loyalty-api and the built app on yamale-us;
    - Caddy routes for `/`, `/api/loyalty`, `/api/rpc` (through rpcgate), `/api/custodian` and `/api/recovery`;
    - per-address rate limits at the proxy on `POST /v1/sponsor`, `/v1/files` and `/v1/applications`;
    - the sponsor key in the node's keyring, funded, with a balance alert in the monitor;
    - the operator token;
    - a nightly backup of the loyalty data directory, because photos exist nowhere else;
    - an SPA fallback so `/j/…` and `/claim` serve `index.html`;
    - `VITE_PUBLIC_ORIGIN=https://loyalty.yamaletech.com` and `VITE_CHAIN_ID` set at build time;
    - the monitor's health check on `/api/loyalty/v1/health`.
  - **Follow-ups that are not defects:**
    - move `herd` and `rwa` onto `sdk/src/abci.ts`;
    - promote `clients/wallet/src/account` into the SDK;
    - outbound email for receipts, if wanted.

- [ ] **Step 4: Run everything once more, then commit**

```bash
export PATH="/c/Program Files/Go/bin:$PATH"
go test ./x/jewel/... ./tools/loyaltyapi/... -count=1
cd clients && npm test --workspace @yamale/chain && npm test --workspace @yamale/loyalty && npm run build --workspace @yamale/loyalty && cd ..
git add clients/loyalty/scripts/smoke.ts clients/README.md docs/scope/gaps.md
git commit -m "loyalty: run locally end to end; the smoke test of every chain path; what Plan 3 must deploy"
```

Expected: every suite passes before the commit. Fixes found in Steps 1 and 2 go in their own commits, named after the task they belong to. Nothing is pushed.
