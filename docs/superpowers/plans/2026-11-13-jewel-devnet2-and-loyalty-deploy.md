# x/jewel on devnet-2, and loyalty.yamaletech.com, Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Tasks 1–9 are code and configuration:** one implementer subagent per task. **Tasks 10–13 act on live hosts:** the controller runs them in its own session. Every governance vote, and every admission of a jeweler, is handed to the user as a command; the controller never casts it.

**Goal:** Fix x/jewel's three open formats, put the module on `yamale-devnet-2` by a coordinated upgrade, appoint its office, and serve the loyalty app and loyalty-api publicly at `https://loyalty.yamaletech.com` from yamale-us.

**Architecture:**
- **Chain changes, before the formats freeze.**
  - The claim signature binds the offer, through its expiry.
  - A suspended jeweler's own stock cannot be sold.
  - Two Amino names are shortened to fit a Ledger.
- **The upgrade, `jewel-register`.**
  - It adds the jewel store and runs the module's genesis.
  - It closes x/nft's bare send in x/circuit within the upgrade itself, so there is no block in which a jewel token could move outside x/jewel.
  - The office is appointed afterwards by a governance proposal the user votes on: a new 2-of-3 x/group.
- **Hosting.**
  - The app is served at the root of its own host. A second server block in the chain's nginx gate (127.0.0.1:8082) serves the static app and proxies `/api/rpc`, `/api/custodian`, `/api/recovery` and `/api/loyalty`, with the gate's security headers and per-visitor rate limits.
  - Caddy (yamale-platform) routes `loyalty.yamaletech.com` to the gate.
  - loyalty-api runs as its own system user, with its own keyring holding only the sponsor key.

**Tech Stack:** Cosmos SDK 0.50 (x/upgrade, x/circuit, x/group, x/feegrant), Go 1.25, TypeScript 5.7 / React 19 / Vite 6, nginx (stable-alpine in Docker), Caddy, systemd.

**Spec:** `docs/superpowers/specs/2026-11-12-jewel-loyalty-design.md`, delivery step 4. Plan 1 (x/jewel) and Plan 2 (the app and loyalty-api) are merged on local `main` at `f3210e8`. The open items are in `docs/scope/gaps.md`, in the section "x/jewel, to settle before the devnet-2 upgrade fixes its formats".

## Decisions taken on 2026-11-13 (the user, in chat)

1. **The claim signature binds the offer.** The signed bytes become `yamale-jewel-claim/v2:<chain id>:<jewel id>:<offer expires at>:<claimer>`. If a claim key is reused for a later offer, the earlier signature no longer verifies.
2. **A suspended jeweler stops selling its stock.** While a jeweler is suspended:
   - its business account cannot make an offer on a jewel of that jeweler which it still holds;
   - an offer it made earlier cannot be taken.

   Reinstatement restores both. A private owner's resale is unaffected.
3. **The two long Amino names are shortened now:**
   - `blockchain/x/jewel/MsgProposeReassign` (37 characters)
   - `blockchain/x/jewel/MsgCompleteReassign` (38 characters)

   The proto message names do not change.
4. **The office is a new 2-of-3 group**, "Registre des bijoux — Yamale".
   - Its three member keys live in a keyring of their own on yamale-us.
   - It is appointed by a governance `MsgUpdateParams` that the user votes on.

## What this plan does not settle

These stay open in `docs/scope/gaps.md`. The deployment is a public demonstrator on devnet-2, not a service anyone relies on.
- Recovery claims answer 503, because the app enrols no second factor.
- The wallet's enrol continues after a 409.
- The logo hash comes from the API's unauthenticated profile.
- The small app defects left by the final review of Plan 2.
- The seeder's intermittent failures.
- The pagination caps, the forwardable ownership proof, and truncated staff codes.

Task 5 closes the `JEWEL_GAS` item. Task 12 checks RTL rendering.

## Global Constraints

**Process**
- Commit messages carry **no Co-Authored-By trailer**.
- Commit to local branches only. **No `git push`, no pull request,** in either repository (`Blockchain`, `yamale-platform`).
- Hosts change only in Tasks 10–13, and only the hosts those tasks name. Production databases are never touched.
- Never print a secret into a transcript, a log or a commit: no private key, mnemonic, claim link, voucher, keyring file, operator token or password. Key creation sends the mnemonic to `/dev/null`.
- **Governance is the user's.** Submit a proposal only after the user says so in chat, then hand over the vote command and watch the tally. The same applies to admitting a jeweler.
- Never delete data on a host. A directory being replaced is moved aside to `<name>.prev-<date>`.
- On any host, `journalctl` is always bounded (`-n 200` at most, or `--since`).
- Never use a bare `git stash`.

**Commands handed to the user**
- Each command must survive PowerShell: one line, at most one outer pair of double quotes, no nested quotes. A name with spaces is passed as separate words to a wrapper that joins them.

**Chain formats (after Tasks 1–3)**
- Claim: `yamale-jewel-claim/v2:<chain id>:<jewel id>:<offer expires at>:<claimer>`. The expiry is the offer's `expires_at`: a unix second, decimal, no padding.
- Voucher, ownership proof and loyalty-api request formats are unchanged from Plan 2.
- Every `option (amino.name)` in `proto/blockchain/jewel/v1/tx.proto` is at most 39 characters.
- The upgrade is named **`jewel-register`**. A name cannot change once proposed.

**Windows (the development machine)**
- Every shell needs Go on its PATH: `export PATH="/c/Program Files/Go/bin:$PATH"`.
- `make` is not installed. Run the commands this plan quotes; each Makefile recipe it needs is quoted in full.
- Application control sometimes refuses Go-built binaries with `fork/exec … stratégie de contrôle`. That is policy, not a failing test: rerun, or build into the scratchpad and run from there.
- `*.sh` and generated TypeScript are LF (`.gitattributes`). For any other file, match its neighbours.
- `$SCRATCH` is the session scratchpad directory.

**Hosts (measured 2026-11-13)**

| Host | ssh | Arch | Validator | Power |
|---|---|---|---|---|
| yamale-us | `ssh yamale-us` | amd64 | `pi` (master) | 122,430 (69.5%) |
| Pi | `ssh -p 2222 -i ~/.ssh/id_ed25519 zubulmuk92@100.68.207.17` (the `yamale-pi` alias names a stale LAN address) | arm64 | `pi-2` | 48,905 (27.8%) |
| VM | `ssh -i ~/.ssh/yamale_oracle ubuntu@92.4.151.72` | amd64 | `oracle` | 4,802 (2.7%) |

- All three hosts run `/opt/yamale/bin/blockchaind` under the unit `yamale-devnet`, and each already has `/opt/yamale/bin/swap-upgrade-binary.sh`.
- Homes:
  - yamale-us: `/opt/yamale/node`, user `yamale-chain`;
  - Pi: `/opt/yamale/join-node`;
  - VM: `/opt/yamale/node`, whose test keyring holds `alice`.
- The last upgrade applied on devnet-2 is `one-identifier-one-animal`, at height 796,281. Since its commit (`b94e8dc`), the only consensus changes are x/jewel's.
- On devnet-2:
  - the gov voting period is 30 minutes and `min_deposit` is 1 YML;
  - x/circuit has no accounts and an empty disabled list, so only the gov authority can trip or reset a breaker;
  - x/nft is present, with no classes;
  - `minimum-gas-prices` is `0uyml`;
  - the tx indexer is `kv`.
- Gov authority: `yml10d07y265gmmuvt4z0w9aw880jnsr700jz5s386`.

---

## File structure

```
x/jewel/types/signing.go                   ClaimSignBytes v2, binding the offer's expiry (Task 1)
x/jewel/types/vectors_test.go              claim vectors carry offer_expires_at (Task 1)
x/jewel/types/testdata/vectors.json        regenerated (Task 1)
x/jewel/types/types_test.go                v2 bytes in the signature test (Task 1)
x/jewel/keeper/msg_server_offer.go         ClaimOffer signs over the offer (Task 1); stock rule (Task 2)
x/jewel/keeper/keeper.go                   requireSellable (Task 2)
x/jewel/keeper/fixture_test.go             claimSig helper (Task 1)
x/jewel/keeper/offer_test.go               claim replay test (Task 1); suspension test (Task 2)
x/jewel/keeper/events_test.go              claimSig (Task 1)
x/jewel/keeper/regression_test.go          claimSig (Task 1)
x/jewel/types/amino_names_test.go          every amino name fits a Ledger (Task 3)
proto/blockchain/jewel/v1/tx.proto         claim comment (Task 1), suspend comment (Task 2), amino names (Task 3)
x/jewel/types/tx.pb.go                     regenerated (Tasks 1–3)
clients/sdk/src/generated/blockchain/jewel/v1/tx.ts   regenerated (Tasks 1–2)
docs/reference/**                          regenerated (Tasks 1–3)
tools/jewelsign/main.go, main_test.go      claim takes the offer's expiry (Task 1)
scripts/devnet/seed-jewel.sh               passes the offer's expiry (Task 1)
clients/sdk/src/jewel.ts, jewel.test.ts    claimSignBytes / claimJewelOffer v2 (Task 1)
clients/loyalty/src/owner/Claim.tsx        signs over the open offer (Task 1); stock rule (Task 2)
clients/loyalty/src/owner/offers.ts, .test stockOfferBlocked (Task 2)
clients/loyalty/scripts/smoke.ts           reads the offer before claiming (Task 1)
app/profile_upgrades_on.go, _off.go        the jewel-register upgrade (Task 4)
app/profile_upgrades_on_test.go            its tests (Task 4)
scripts/devnet/init-devnet.sh              a relaunched devnet closes x/nft's send at genesis (Task 4)
scripts/devnet/init-local-devnet.sh        SKIP_NFT_BREAKER, for the rehearsal (Task 5)
scripts/devnet/jewel-lib.sh                as_office votes OFFICE_VOTES members (Task 5)
scripts/devnet/loyalty-admit.sh            FUNDER (Task 5)
scripts/devnet/rehearse-jewel-upgrade.sh   the upgrade, end to end, on a local chain (Task 5)
scripts/devnet/measure-amend-gas.sh        gas of an amendment at the 50-entry cap (Task 5)
scripts/devnet/await-halt-then-swap.sh     a per-host job that waits for the halt, then swaps (Task 5)
clients/sdk/src/jewel.ts                   JEWEL_GAS from the measurement (Task 5)
tools/loyaltyapi/balance.go, balance_test.go  GET /v1/health/sponsor (Task 6)
tools/loyaltyapi/main.go                   --sponsor-floor, the route (Task 6)
deploy/yamale-us/gate/nginx.conf           zones, maps, the loyalty server on :8082 (Task 7)
deploy/yamale-us/gate/loyalty.conf         /api/loyalty routes and the SPA fallback (Task 7)
deploy/yamale-us/gate/docker-compose.yml   mounts /srv/loyalty (Task 7)
deploy/yamale-us/gate/deploy-gate.sh       ships loyalty.conf, probes :8082 (Task 7)
deploy/yamale-us/loyalty/install-loyalty.sh   builds and installs app, API, keys, unit, backup (Task 8)
deploy/yamale-us/loyalty/jewel-admit       the office's admit wrapper, installed on yamale-us (Task 8)
../yamale-platform/caddy/sites/loyalty.yamaletech.com.caddy   the site (Task 9)
../yamale-platform/PORTS.md                8082 and 8095 (Task 9)
../yamale-platform/monitor/yamale-monitor-host.conf          unit and probes (Task 9)
docs/scope/gaps.md, clients/README.md      the outcome (Task 13)
```

---

### Task 1: The claim signature binds the offer

**Files:**
- Modify: `x/jewel/types/signing.go`, `x/jewel/types/vectors_test.go`, `x/jewel/types/types_test.go`, `x/jewel/types/testdata/vectors.json` (regenerated)
- Modify: `x/jewel/keeper/msg_server_offer.go`, `x/jewel/keeper/fixture_test.go`, `x/jewel/keeper/offer_test.go`, `x/jewel/keeper/events_test.go`, `x/jewel/keeper/regression_test.go`
- Modify: `proto/blockchain/jewel/v1/tx.proto` (the `claim_signature` comment), then regenerate `x/jewel/types/tx.pb.go`, `clients/sdk/src/generated/**` and `docs/reference/**`
- Modify: `tools/jewelsign/main.go`, `tools/jewelsign/main_test.go`, `scripts/devnet/seed-jewel.sh`
- Modify: `clients/sdk/src/jewel.ts`, `clients/sdk/src/jewel.test.ts`, `clients/loyalty/src/owner/Claim.tsx`, `clients/loyalty/scripts/smoke.ts`

**Interfaces:**
- Produces (Go): `func ClaimSignBytes(chainID string, jewelID uint64, offerExpiresAt int64, claimer string) []byte`
- Produces (TS): `claimSignBytes(chainId: string, jewelId: string, offerExpiresAt: string, claimer: string): Uint8Array` and `claimJewelOffer(claimer: string, jewelId: string, offerExpiresAt: string, claimPrivateKey: Uint8Array, chainId: string): EncodeObject`
- Produces (CLI): `jewelsign claim <priv-hex> <chain-id> <jewel-id> <offer-expires-at> <claimer>`
- Produces (test helper): `func (f *fixture) claimSig(t *testing.T, key *secp256k1.PrivKey, id uint64, claimer string) []byte`

- [ ] **Step 1: Write the failing keeper test: a signature for an earlier offer does not claim a later one**

Add to `x/jewel/keeper/fixture_test.go`, after `owner`:

```go
// claimSig signs a claim over the jewel's open offer, as the buyer's phone
// does: the bytes name the offer's expiry, so a signature made for one offer
// never claims another.
func (f *fixture) claimSig(t *testing.T, key *secp256k1.PrivKey, id uint64, claimer string) []byte {
	t.Helper()
	o, err := f.k.Offer.Get(f.env.Ctx, id)
	require.NoError(t, err)
	sig, err := key.Sign(types.ClaimSignBytes("yamale-test-1", id, o.ExpiresAt, claimer))
	require.NoError(t, err)
	return sig
}
```

Add the `secp256k1` import (`github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1`) if `fixture_test.go` lacks it.

Add to `x/jewel/keeper/offer_test.go`, after `TestAClaimKeyHandsTheJewelToWhoeverSignsWithIt`:

```go
// A seller who reuses a claim key for a later offer must not revive the
// earlier one's signature: the signed bytes name the offer by its expiry.
func TestASignatureForAnEarlierOfferDoesNotClaimALaterOne(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	claimKey := secp256k1.GenPrivKey()
	offer := &types.MsgOfferJewel{Owner: staff, JewelId: id, ClaimKey: hex.EncodeToString(claimKey.PubKey().Bytes())}
	_, err := f.srv.OfferJewel(f.env.Ctx, offer)
	require.NoError(t, err)
	_, customer := f.env.Addr(t)
	stale := f.claimSig(t, claimKey, id, customer)

	_, err = f.srv.CancelOffer(f.env.Ctx, &types.MsgCancelOffer{Owner: staff, JewelId: id})
	require.NoError(t, err)
	f.advance(time.Second)
	_, err = f.srv.OfferJewel(f.env.Ctx, offer)
	require.NoError(t, err)

	_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: customer, JewelId: id,
		ClaimPubKey: claimKey.PubKey().Bytes(), ClaimSignature: stale})
	require.ErrorIs(t, err, types.ErrBadClaim, "the signature names the first offer, which is gone")

	_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: customer, JewelId: id,
		ClaimPubKey: claimKey.PubKey().Bytes(), ClaimSignature: f.claimSig(t, claimKey, id, customer)})
	require.NoError(t, err)
	require.Equal(t, customer, f.owner(t, id))
}
```

- [ ] **Step 2: Change the format and every Go caller**

`x/jewel/types/signing.go`: replace `ClaimSignBytes` with:

```go
// ClaimSignBytes is what a claim key signs. It names the claimer, which makes
// a claim copied out of the mempool useless to anyone else, and the offer's
// expiry, which makes a signature for one offer useless for any later offer
// made to the same key.
func ClaimSignBytes(chainID string, jewelID uint64, offerExpiresAt int64, claimer string) []byte {
	return []byte(fmt.Sprintf("yamale-jewel-claim/v2:%s:%d:%d:%s", chainID, jewelID, offerExpiresAt, claimer))
}
```

`x/jewel/keeper/msg_server_offer.go`, in `ClaimOffer`: replace `msg := types.ClaimSignBytes(k.chainID(ctx), j.Id, claimer)` with `msg := types.ClaimSignBytes(k.chainID(ctx), j.Id, o.ExpiresAt, claimer)`. Update the doc comment above `ClaimOffer` so it says the key signs the claimer's address and the offer.

Replace each direct signing in the keeper tests with the helper:
- `offer_test.go`, `TestAClaimKeyHandsTheJewelToWhoeverSignsWithIt`:
  - `sig, err := claimKey.Sign(types.ClaimSignBytes("yamale-test-1", id, customer))` plus its `require.NoError` becomes `sig := f.claimSig(t, claimKey, id, customer)`;
  - `wrongSig, _ := wrongKey.Sign(types.ClaimSignBytes("yamale-test-1", id, thief))` becomes `wrongSig := f.claimSig(t, wrongKey, id, thief)`.
- `events_test.go` (around line 49): `sig, err := claimKey.Sign(...)` plus its `require.NoError` becomes `sig := f.claimSig(t, claimKey, id, friend)`.
- `regression_test.go` (around line 46): the same, with `accomplice`. The offer still exists at that point, because suspension does not delete it.

`x/jewel/types/types_test.go`, `TestSignaturesVerifyOnlyOverTheirOwnBytes`: pass an expiry of `1790000000` in every call. Add one more refusal: the same signature must not verify over bytes with expiry `1790000001`, with the message `"a claim signed for one offer must not verify for another"`.

`x/jewel/types/vectors_test.go`:
- Add `OfferExpiresAt string \`json:"offer_expires_at"\`` to `claimVector`, after `JewelID`.
- In `buildJewelVectors`, give the loop's struct an `expires int64` field, with values `1790000000` and `9223372036854775807`. Sign `types.ClaimSignBytes(c.chain, c.jewel, c.expires, claimer)` and set `OfferExpiresAt: strconv.FormatInt(c.expires, 10)`.

`tools/jewelsign/main.go`:
- `claim` takes `need(7)`, and `claimOffer(privHex, chainID, jewelIDStr, expiresStr, claimer string)` signs `types.ClaimSignBytes(chainID, u64(jewelIDStr), i64(expiresStr), claimer)`.
- Update `usage()` to `claim <priv-hex> <chain-id> <jewel-id> <offer-expires-at> <claimer>`.
- Update `tools/jewelsign/main_test.go` to match: same expiry in the verification bytes, plus one check that a different expiry does not verify.

- [ ] **Step 3: Update the proto comment and regenerate the vectors**

In `proto/blockchain/jewel/v1/tx.proto`, the `claim_signature` field comment becomes:

```proto
  // claim_signature signs ClaimSignBytes(chain id, jewel id, the offer's
  // expires_at, claimer): "yamale-jewel-claim/v2:<chain>:<jewel>:<expires>:<claimer>".
```

Run (Git Bash, repo root):

```bash
export PATH="/c/Program Files/Go/bin:$PATH"
go test ./x/jewel/types -run TestVectors -update-vectors
git diff --stat x/jewel/types/testdata/vectors.json
```

Expected: only the `claims` entries change. Each gains `offer_expires_at`, and the `sign_bytes` begin `yamale-jewel-claim/v2:`.

- [ ] **Step 4: Regenerate Go, TypeScript and the reference**

These are the Makefile's `proto-gen`, `proto-ts` and `docs` recipes, written out because `make` is absent:

```bash
rm -rf yamale
go tool github.com/bufbuild/buf/cmd/buf generate --template proto/buf.gen.gogo.yaml
cp -r yamale/blockchain/x/. x/
rm -rf yamale

SDKPROTO="$(go env GOMODCACHE)/github.com/cosmos/cosmos-sdk@$(go list -m -f '{{.Version}}' github.com/cosmos/cosmos-sdk)/proto"
rm -rf clients/sdk/src/generated
go tool github.com/bufbuild/buf/cmd/buf generate "$SDKPROTO" --template proto/buf.gen.ts.yaml --path "$SDKPROTO/cosmos/nft/v1beta1"
go tool github.com/bufbuild/buf/cmd/buf generate buf.build/cosmos/cosmos-sdk:65fa41963e6a41dd95a35934239029df --template proto/buf.gen.ts.yaml --path cosmos/feegrant/v1beta1 --path cosmos/bank/v1beta1
go tool github.com/bufbuild/buf/cmd/buf generate --template proto/buf.gen.ts.yaml
find clients/sdk/src/generated -name '*.ts' -exec sed -i 's#protobufjs/minimal\.ts#protobufjs/minimal.js#' {} +

go tool github.com/bufbuild/buf/cmd/buf build --as-file-descriptor-set --exclude-imports -o .docgen-descriptor.binpb
go run ./tools/docgen --descriptor .docgen-descriptor.binpb --out docs/reference --root .
rm -f .docgen-descriptor.binpb
git status --short clients/sdk/src/generated x docs/reference
```

Expected: only these change:
- `x/jewel/types/tx.pb.go`;
- `clients/sdk/src/generated/blockchain/jewel/v1/tx.ts` (the comment);
- the jewel page(s) under `docs/reference`.

If any other generated file changes, stop and report it: that would be drift from before this task.

- [ ] **Step 5: Run the Go tests**

```bash
go test ./x/jewel/... ./tools/jewelsign/... -count=1
```

Expected: PASS, including `TestASignatureForAnEarlierOfferDoesNotClaimALaterOne` and `TestVectors`.

- [ ] **Step 6: Change the TypeScript side, test first**

In `clients/sdk/src/jewel.test.ts`:
- The claim-vector test calls `claimSignBytes(v.chain_id, v.jewel_id, v.offer_expires_at, v.claimer)`.
- The claim-message test calls `claimJewelOffer(v.claimer, v.jewel_id, v.offer_expires_at, fromHex(v.priv_key), v.chain_id)`.
- Add: `assert.throws(() => claimSignBytes('c', '1', '01', 'yml1x'))` and `assert.throws(() => claimSignBytes('c', '1', '-1', 'yml1x'))`. The expiry must be a canonical decimal.

Run: `(cd clients/sdk && npm test)`. Expected: FAIL, because `offer_expires_at` is not used yet.

In `clients/sdk/src/jewel.ts`:

```ts
export function claimSignBytes(chainId: string, jewelId: string, offerExpiresAt: string, claimer: string): Uint8Array {
  if (!ID.test(jewelId)) throw new Error(`jewel id ${jewelId}`);
  if (!DECIMAL.test(offerExpiresAt)) throw new Error(`offer expiry ${offerExpiresAt}`);
  return utf8.encode(`yamale-jewel-claim/v2:${chainId}:${jewelId}:${offerExpiresAt}:${claimer}`);
}
```

```ts
/** `offerExpiresAt` is the open offer's expires_at as the chain holds it: the signature is good for that offer only. */
export function claimJewelOffer(
  claimer: string, jewelId: string, offerExpiresAt: string, claimPrivateKey: Uint8Array, chainId: string,
): EncodeObject {
  return msg('MsgClaimOffer', {
    claimer, jewelId,
    claimPubKey: compressedPublicKey(claimPrivateKey),
    claimSignature: signArbitrary(claimPrivateKey, claimSignBytes(chainId, jewelId, offerExpiresAt, claimer)),
  });
}
```

Before relying on `DECIMAL`, check its definition in `jewel.ts`. If it accepts a leading zero, use `/^(0|[1-9][0-9]*)$/` for the expiry check.

Run: `(cd clients/sdk && npm test)`. Expected: PASS.

- [ ] **Step 7: Update the claimers**

`clients/loyalty/src/owner/Claim.tsx`: the button signs over the open offer, which this page has already read:

```tsx
const done = await run(() => session.submitAsOwner(
  [claimJewelOffer(session.address!, parsed.jewelId, offer!.expiresAt, parsed.privateKey, CHAIN_ID)]));
```

(`open` is only true when `offer` exists, so the non-null assertion is safe there.)

`clients/loyalty/scripts/smoke.ts`, step 5: after `byLink`, read the jewel and sign over its offer:

```ts
const offered = await readJewel(jewelId);
const offerExpires = offered.ok ? offered.value.offer?.expiresAt ?? '' : '';
check('the link offer is open on the chain', offerExpires !== '');
const claim = await send(buyer, [claimJewelOffer(addr(buyer), jewelId, offerExpires, claimPriv, CHAIN)], granter);
```

`scripts/devnet/seed-jewel.sh`, stage 5: read the offer's expiry from the chain, then pass it to `jewelsign`:

```bash
EXPIRES=$("$BIN" query jewel jewel "$RING" $Q | grep -oE '"offer":[[:space:]]*\{[^}]*\}' | json expires_at)
[ -n "$EXPIRES" ] || bad "the link offer is not on the chain"
run_json "$("$SIGN" claim "$CLAIM_PRIV" "$CHAIN" "$RING" "$EXPIRES" "$CUSTOMER")" jw-customer && ok "claimed by the customer" || bad "claim"
```

Check the query's actual name and output shape with `"$BIN" query jewel --help` on a local devnet first. If the subcommand differs, use the one that returns the jewel's view, including its offer.

- [ ] **Step 8: Typecheck, test and prove the chain path**

```bash
(cd clients && npm run typecheck --workspaces --if-present)
(cd clients/loyalty && npm test)
go build -o "$SCRATCH/blockchaind.exe" ./cmd/blockchaind && go build -o "$SCRATCH/jewelsign.exe" ./tools/jewelsign
BIN="$SCRATCH/blockchaind.exe" scripts/devnet/init-local-devnet.sh
BIN="$SCRATCH/blockchaind.exe" SIGN="$SCRATCH/jewelsign.exe" HOME_DIR=.devnet/local/node CHAIN=yamale-local-1 NODE=tcp://127.0.0.1:26957 scripts/devnet/seed-jewel.sh
```

Expected: typecheck clean, loyalty tests pass, and seed stage 5 reports `ok claimed by the customer`. Stop the node afterwards (`kill $(cat .devnet/local/node.pid)`).

- [ ] **Step 9: Commit**

```bash
git add x/jewel tools/jewelsign scripts/devnet/seed-jewel.sh proto/blockchain/jewel/v1/tx.proto clients/sdk/src clients/loyalty docs/reference
git commit -m "x/jewel: a claim signature names the offer it claims (claim/v2)"
```

---

### Task 2: A suspended jeweler stops selling its stock

**Files:**
- Modify: `x/jewel/keeper/keeper.go` (add `requireSellable`), `x/jewel/keeper/msg_server_offer.go`
- Test: `x/jewel/keeper/offer_test.go`
- Modify: `proto/blockchain/jewel/v1/tx.proto` (the `SuspendJeweler` rpc comment), regenerate as in Task 1 Step 4
- Modify: `clients/loyalty/src/owner/offers.ts`, `clients/loyalty/src/owner/offers.test.ts`, `clients/loyalty/src/owner/Claim.tsx`

**Interfaces:**
- Consumes: `ErrJewelerSuspended` (code 8, `"this jeweler is suspended and can no longer act"`), which `clients/loyalty/src/actions/errors.ts` already maps to `suspended`.
- Produces (Go): `func (k Keeper) requireSellable(ctx context.Context, owner string, j types.Jewel) error`
- Produces (TS): `stockOfferBlocked(offer: { from: string } | null | undefined, business: string | null, active: boolean): boolean`

- [ ] **Step 1: Write the failing keeper test**

Add to `x/jewel/keeper/offer_test.go`:

```go
// While its jeweler is suspended, the business cannot sell its stock: no new
// offer, and an offer made before the suspension cannot be taken. A jewel
// that has left the shop is its owner's to sell whatever the shop's standing,
// and reinstatement restores selling.
func TestASuspendedJewelerCannotSellItsStock(t *testing.T) {
	f := setup(t)
	jid, business, staff := f.admit(t, "Maison Diop")
	ring := f.register(t, staff, "MD-0001")
	sold := f.register(t, staff, "MD-0002")
	owner := f.sell(t, staff, sold)
	_, buyer := f.env.Addr(t)

	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: business, JewelId: ring, To: buyer})
	require.NoError(t, err)
	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "complaint 2026-118"})
	require.NoError(t, err)

	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: ring})
	require.ErrorIs(t, err, types.ErrJewelerSuspended, "an offer from stock cannot be taken while the jeweler is suspended")
	_, err = f.srv.CancelOffer(f.env.Ctx, &types.MsgCancelOffer{Owner: business, JewelId: ring})
	require.NoError(t, err, "the business may still withdraw it")
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: business, JewelId: ring, To: buyer})
	require.ErrorIs(t, err, types.ErrJewelerSuspended, "and may not offer its stock again")

	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: owner, JewelId: sold, To: buyer})
	require.NoError(t, err, "a private owner's resale is untouched")
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: sold})
	require.NoError(t, err)

	_, err = f.srv.ReinstateJeweler(f.env.Ctx, &types.MsgReinstateJeweler{Office: f.office, JewelerId: jid})
	require.NoError(t, err)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: business, JewelId: ring, To: buyer})
	require.NoError(t, err, "reinstatement restores selling")
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: ring})
	require.NoError(t, err)
}
```

If `MsgReinstateJeweler` requires a reason, or `f.sell` has a different signature, read `jeweler_test.go` and `fixture_test.go` and adapt the calls. Keep the assertions.

Run: `go test ./x/jewel/keeper -run TestASuspendedJewelerCannotSellItsStock -count=1`. Expected: FAIL, because the first `AcceptOffer` succeeds.

- [ ] **Step 2: Implement**

In `x/jewel/keeper/keeper.go`, after `actsForOwner`:

```go
// requireSellable refuses to let a suspended jeweler sell its stock. Stock is
// a jewel its jeweler's business still owns; a jewel that has left the shop
// is its owner's to sell whatever the shop's standing.
func (k Keeper) requireSellable(ctx context.Context, owner string, j types.Jewel) error {
	jr, err := k.getJeweler(ctx, j.JewelerId)
	if err != nil {
		return err
	}
	if owner == jr.Business && jr.Status != types.JEWELER_ACTIVE {
		return types.ErrJewelerSuspended.Wrapf("%s cannot sell its stock while suspended", jr.Name)
	}
	return nil
}
```

In `msg_server_offer.go`:
- In `OfferJewel`, immediately after `maker, err := k.canonical(req.Owner)` and its error check, add:

```go
	if err := k.requireSellable(ctx, owner, j); err != nil {
		return nil, err
	}
```

- In `openOffer`, after the `if o.From != owner { … ErrOfferStale }` block, add:

```go
	if err := k.requireSellable(ctx, o.From, j); err != nil {
		return o, err
	}
```

- Update `openOffer`'s doc comment: an offer from stock also dies while its jeweler is suspended.

In `tx.proto`, the rpc comment becomes `// SuspendJeweler stops a jeweler acting: no registration, no sale of its stock, and its staff's offers die. Office only.` Regenerate Go, TS and docs exactly as in Task 1, Step 4.

- [ ] **Step 3: Run the keeper tests**

```bash
go test ./x/jewel/... -count=1
```

Expected: PASS, every existing test included. `regression_test.go`'s staff-offer case still fails with `ErrJewelerSuspended`, now possibly from either check; both give the same error.

- [ ] **Step 4: Teach the claim page the same rule, test first**

Add to `clients/loyalty/src/owner/offers.test.ts`:

```ts
test('an offer from a suspended jeweler\'s own stock is blocked, and nothing else is', () => {
  assert.equal(stockOfferBlocked({ from: 'yml1shop' }, 'yml1shop', false), true);
  assert.equal(stockOfferBlocked({ from: 'yml1shop' }, 'yml1shop', true), false);
  assert.equal(stockOfferBlocked({ from: 'yml1owner' }, 'yml1shop', false), false);
  assert.equal(stockOfferBlocked(undefined, 'yml1shop', false), false);
  assert.equal(stockOfferBlocked({ from: 'yml1shop' }, null, false), false);
});
```

(Import `stockOfferBlocked` beside the existing imports.) Run `(cd clients/loyalty && npm test)`. Expected: FAIL.

Add to `clients/loyalty/src/owner/offers.ts`:

```ts
/**
 * The chain refuses an offer from a jeweler's own stock while that jeweler is
 * suspended. `business` is null when the jeweler could not be read: then the
 * chain decides, and the screen does not guess.
 */
export function stockOfferBlocked(offer: { from: string } | null | undefined, business: string | null, active: boolean): boolean {
  return !!offer && business !== null && offer.from === business && !active;
}
```

In `Claim.tsx`, `open` also requires that the offer is not blocked:

```tsx
const issuer = jeweler?.ok ? jeweler.value : null;
const open = offer?.claimKey === claimKey && offerIsOpen(offer, head.value.time)
  && !stockOfferBlocked(offer, issuer?.business ?? null, issuer ? isActive(issuer) : true);
```

Import `isActive` from `@yamale/chain` and `stockOfferBlocked` from `./offers.ts`. The existing `claimClosed` banner then covers the case.

- [ ] **Step 5: Typecheck, test, commit**

```bash
(cd clients && npm run typecheck --workspaces --if-present) && (cd clients/loyalty && npm test)
git add x/jewel proto/blockchain/jewel/v1/tx.proto clients/sdk/src/generated clients/loyalty/src docs/reference
git commit -m "x/jewel: a suspended jeweler cannot sell its stock; the claim page says so"
```

---

### Task 3: Every Amino name fits a Ledger

**Files:**
- Create: `x/jewel/types/amino_names_test.go`
- Modify: `proto/blockchain/jewel/v1/tx.proto`; regenerate `x/jewel/types/tx.pb.go` and `docs/reference/**`

- [ ] **Step 1: Write the failing test**

```go
package types_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// A Ledger signs amino JSON and refuses a message whose amino name is longer
// than 39 characters. Read from the proto source, which is where the name is
// declared; proto-check guards the generated descriptor against drift from it.
func TestEveryAminoNameFitsALedger(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "proto", "blockchain", "jewel", "v1", "tx.proto"))
	require.NoError(t, err)
	names := regexp.MustCompile(`option \(amino\.name\) = "([^"]+)"`).FindAllStringSubmatch(string(src), -1)
	require.Len(t, names, 18, "every Msg in tx.proto carries an amino name; a count of zero would pass for the wrong reason")
	for _, m := range names {
		require.LessOrEqualf(t, len(m[1]), 39, "amino name %q is %d characters; a Ledger refuses more than 39", m[1], len(m[1]))
	}
}
```

Run: `go test ./x/jewel/types -run TestEveryAminoNameFitsALedger -count=1`. Expected: FAIL on the two reassignment names.

- [ ] **Step 2: Shorten the two names**

In `tx.proto`:
- `option (amino.name) = "blockchain/x/jewel/MsgProposeReassignment";` → `"blockchain/x/jewel/MsgProposeReassign"`
- `option (amino.name) = "blockchain/x/jewel/MsgCompleteReassignment";` → `"blockchain/x/jewel/MsgCompleteReassign"`

Add one comment line above each: `// Shortened from the message name: a Ledger accepts at most 39 characters.`

Regenerate Go and the docs (Task 1, Step 4: the gogo block and the docs block; the TS block is not needed, because ts-proto does not emit amino names). Then search for the old names: `grep -rn "MsgProposeReassignment\"\|MsgCompleteReassignment\"" x app clients/sdk/src --include=*.go --include=*.ts`. Expected: no amino-name use outside generated descriptors. Type URLs (`/blockchain.jewel.v1.MsgProposeReassignment`) are unchanged and correct.

- [ ] **Step 3: Run the tests**

```bash
go test ./x/jewel/... ./app/... -count=1
```

Expected: PASS. The app tests build the full codec, so a broken amino registration would fail here.

- [ ] **Step 4: Commit**

```bash
git add x/jewel proto/blockchain/jewel/v1/tx.proto docs/reference
git commit -m "x/jewel: the reassignment messages' amino names fit a Ledger"
```

---

### Task 4: The `jewel-register` upgrade

**Files:**
- Modify: `app/profile_upgrades_on.go` (append to `profileUpgrades`), `app/profile_upgrades_off.go` (append the settlement half)
- Test: `app/profile_upgrades_on_test.go`
- Modify: `scripts/devnet/init-devnet.sh`

**Interfaces:**
- Consumes: `jeweltypes.StoreKey` (= `"jewel"`), `jeweltypes.NFTSendTypeURL` (`/cosmos.nft.v1beta1.MsgSend`), `app.JewelKeeper.Params`, `app.JewelKeeper.Jeweler`, `app.CircuitBreakerKeeper.DisableList`; the test helpers `newUpgradeTestApp`, `findUpgrade`, `priorVersions` (all in `app/upgrades_test.go`).
- Produces: the upgrade named `jewel-register` in both profiles.

- [ ] **Step 1: Write the failing tests**

Append to `app/profile_upgrades_on_test.go`. Add imports `jeweltypes "yamale/blockchain/x/jewel/types"` and `jewelmoduletypes` only if they differ.

```go
func TestTheJewelRegisterAddsItsStore(t *testing.T) {
	u := findUpgrade(t, "jewel-register")
	require.Contains(t, u.StoreUpgrades.Added, jeweltypes.StoreKey)
}

// The upgrade closes x/nft's own send in the same block the module arrives.
// Left open for even one block, a jewel token could move outside x/jewel; and
// x/jewel refuses to admit any jeweler while it is open, so without this no
// jeweler could ever be admitted.
func TestTheJewelRegisterClosesTheBareNFTSendAndAppointsNobody(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})
	require.NoError(t, app.CircuitBreakerKeeper.DisableList.Remove(ctx, jeweltypes.NFTSendTypeURL))

	u := findUpgrade(t, "jewel-register")
	_, err := u.Handler(ctx, app, priorVersions(app, jeweltypes.ModuleName))
	require.NoError(t, err)

	closed, err := app.CircuitBreakerKeeper.DisableList.Has(ctx, jeweltypes.NFTSendTypeURL)
	require.NoError(t, err)
	require.True(t, closed)

	params, err := app.JewelKeeper.Params.Get(ctx)
	require.NoError(t, err)
	require.Empty(t, params.Office, "the office is appointed by a vote, never by a migration")
	want := jeweltypes.DefaultParams()
	require.Equal(t, want, params, "InitGenesis must have run: fromVM must not pre-set the module")
}

// An upgrade that found an office already appointed would be installing an
// authority nobody voted for; it refuses rather than carry on.
func TestTheJewelRegisterRefusesAnOfficeItDidNotAppoint(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})
	p := jeweltypes.DefaultParams()
	p.Office = "yml1dlszg2sst9r69my4f84l3mj66zxcf3umcgujys30t84srg95dgvsrmuayr"
	require.NoError(t, app.JewelKeeper.Params.Set(ctx, p))

	u := findUpgrade(t, "jewel-register")
	_, err := u.Handler(ctx, app, priorVersions(app)) // jewel already at its version: no InitGenesis
	require.ErrorContains(t, err, "office")
}
```

If `DisableList.Remove` errors on an absent key in this SDK version, swallow only `collections.ErrNotFound`.

Run: `go test ./app -run 'JewelRegister' -count=1`. Expected: FAIL (`upgrade "jewel-register" is no longer declared`).

- [ ] **Step 2: Implement the default-profile half**

Append to `profileUpgrades` in `app/profile_upgrades_on.go`, after `one-identifier-one-animal`. Import `jeweltypes "yamale/blockchain/x/jewel/types"`.

```go
	{
		// x/jewel: the register of jewels (loyalty.yamaletech.com).
		//
		// A new store, so it is added before any migration runs, and x/jewel's
		// InitGenesis arrives through the standard migration path — fromVM must
		// NOT carry "jewel", or the module's genesis never runs and its params
		// stay unwritten.
		//
		// The handler shuts x/nft's own MsgSend in x/circuit BEFORE the module
		// arrives, in the same block. x/nft's send checks only that the sender
		// owns the token, so while it is open an owner could move a jewel
		// around every rule x/jewel exists for, and x/jewel itself refuses to
		// admit a jeweler until it is shut. Only the gov authority can reset a
		// breaker on devnet-2 (x/circuit has no accounts), so reopening it would
		// take a vote.
		//
		// The office arrives empty, asserted rather than assumed: it is appointed
		// by governance, and an upgrade that installed one would be admitting an
		// authority over other people's jewels that nobody voted for.
		Name:          "jewel-register",
		StoreUpgrades: storetypes.StoreUpgrades{Added: []string{jeweltypes.StoreKey}},
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			if err := app.CircuitBreakerKeeper.DisableList.Set(ctx, jeweltypes.NFTSendTypeURL); err != nil {
				return nil, fmt.Errorf("closing x/nft's own send: %w", err)
			}
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			params, err := app.JewelKeeper.Params.Get(ctx)
			if err != nil {
				return nil, fmt.Errorf("reading the jewel parameters this upgrade just wrote: %w", err)
			}
			if params.Office != "" {
				return nil, fmt.Errorf("x/jewel arrived with office %s appointed; the office is "+
					"appointed by a vote, not by a migration", params.Office)
			}
			jewelers := 0
			if err := app.JewelKeeper.Jeweler.Walk(ctx, nil,
				func(uint64, jeweltypes.Jeweler) (bool, error) { jewelers++; return false, nil },
			); err != nil {
				return nil, fmt.Errorf("reading the jewelers this upgrade just wrote: %w", err)
			}
			if jewelers != 0 {
				return nil, fmt.Errorf("x/jewel arrived with %d jeweler(s) admitted; the office admits them", jewelers)
			}

			ctx.Logger().Info(
				"the register of jewels arrived",
				"jewel", "jewelers are admitted by an office governance appoints; until then nothing can be registered",
				"nft", "x/nft's own MsgSend is disabled in x/circuit from this height; a jewel moves only by "+
					"offer and acceptance, a claim key, or a reassignment",
				"params", fmt.Sprintf("offer_ttl=%ds voucher_max_ttl=%ds reassignment_window=%ds",
					params.OfferTtl, params.VoucherMaxTtl, params.ReassignmentWindow),
			)
			return vm, nil
		},
	},
```

If any field name differs from the code (for example `Jeweler.Walk`'s value type), read `x/jewel/keeper/keeper.go` and `x/jewel/types/params.pb.go` and use the real names.

- [ ] **Step 3: Implement the settlement half**

Append to `profileUpgrades` in `app/profile_upgrades_off.go`:

```go
	{
		// The settlement half of jewel-register. x/jewel is not linked into a
		// settlement build, so there is no store to add and no breaker to trip.
		// The entry exists because an upgrade name a binary does not know panics
		// the node at the halt height.
		Name: "jewel-register",
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}
			ctx.Logger().Info("the register of jewels: nothing to do",
				"absent", "x/jewel is not linked into a settlement build")
			return vm, nil
		},
	},
```

- [ ] **Step 4: A relaunched devnet closes the send at genesis**

In `scripts/devnet/init-devnet.sh`, in the first python block (the one that ends by setting `p["ombudsman"]`), insert just before its `json.dump(g, open(path, "w"), indent=2)`:

```python
# x/jewel refuses to admit a jeweler while x/nft's own send is open (see the
# jewel-register upgrade), so a devnet relaunched from genesis closes it at
# height zero, as the upgrade does on the running chain.
circuit = g["app_state"].setdefault("circuit", {})
urls = circuit.setdefault("disabled_type_urls", [])
if "/cosmos.nft.v1beta1.MsgSend" not in urls:
    urls.append("/cosmos.nft.v1beta1.MsgSend")
```

Confirm by reading the block that `g` is the genesis dict there.

- [ ] **Step 5: Run the tests in both profiles**

```bash
go test ./app/... -count=1
go test -tags settlement ./app/... -count=1
go vet ./app/...
```

Expected: PASS in both, including `TestBothProfilesKnowTheSameUpgradeNames` and `TestEveryDeclaredUpgradeHasAHandler`.

- [ ] **Step 6: Commit**

```bash
git add app scripts/devnet/init-devnet.sh
git commit -m "app: the jewel-register upgrade adds x/jewel and closes x/nft's own send in the same block"
```

---

### Task 5: Rehearse the upgrade locally, and measure the heaviest message

**Files:**
- Modify: `scripts/devnet/init-local-devnet.sh` (honour `SKIP_NFT_BREAKER=1`)
- Modify: `scripts/devnet/jewel-lib.sh` (`OFFICE_VOTES`), `scripts/devnet/loyalty-admit.sh` (`FUNDER`)
- Create: `scripts/devnet/rehearse-jewel-upgrade.sh`, `scripts/devnet/measure-amend-gas.sh`, `scripts/devnet/await-halt-then-swap.sh`
- Modify: `clients/sdk/src/jewel.ts` (`JEWEL_GAS`, only if the measurement requires it)

**Interfaces:**
- Produces: `rehearse-jewel-upgrade.sh <old-binary> <new-binary>`. It exits 0 only if the upgraded chain passes every check below.
- Produces: `await-halt-then-swap.sh <upgrade-name>`. Run as root on a validator host. It waits until the node halts at the scheduled plan height, then runs `/opt/yamale/bin/swap-upgrade-binary.sh <upgrade-name>`. Task 10 runs it as a `systemd-run` job.
- Produces: `jewel-lib.sh`'s `as_office` votes with the first `OFFICE_VOTES` of `M1 M2 M3` (default 3), the last with `--exec try`.
- Produces: `loyalty-admit.sh` funds the business from `FUNDER` (default `$M3`).

- [ ] **Step 1: Parametrise the office scripts**

In `jewel-lib.sh`'s `as_office`, replace the three fixed `tx group vote` calls with a loop over the first `OFFICE_VOTES` of `"$M1" "$M2" "$M3"`. Every vote but the last omits `--exec try`; the last includes it. Each is followed by `committed "$out" || return 1`, as now. Add near the top: `OFFICE_VOTES=${OFFICE_VOTES:-3}`. Update the comment above `as_office` ("a policy of M1..M3; OFFICE_VOTES of them vote, the last executes").

In `loyalty-admit.sh`, add `FUNDER=${FUNDER:-$M3}` and fund from `"$(addr "$FUNDER")"` with `--from "$FUNDER"`. Update the header comment to say the office is 3-of-5 locally and 2-of-3 on devnet-2 (`OFFICE_VOTES=2`).

In `init-local-devnet.sh`, guard the circuit line: wrap the `g.app_state.circuit…` statements in `if (process.env.SKIP_NFT_BREAKER !== "1") { … }`. Add a comment: the rehearsal must start with the send open, so it can prove the upgrade closes it.

Check that `node -e` in that script sees the environment. It runs as a child of the script, so an exported or prefixed `SKIP_NFT_BREAKER` is inherited.

- [ ] **Step 2: Write `await-halt-then-swap.sh`**

```bash
#!/usr/bin/env bash
# Wait for this node to halt at the scheduled upgrade, then cross it.
#
# Run as root on each validator host, as a systemd-run job, so the chain never
# depends on a laptop's ssh session staying up: on 2026-10-12 the watcher ran
# over ssh, the connection reset, and devnet-2 sat halted for seven hours.
#
#   sudo systemd-run --unit yamale-upgrade-<name> --collect \
#     /opt/yamale/bin/await-halt-then-swap.sh <name>
#
# Halted means: the height has not moved for 20 s AND it equals the plan's
# height. "Stopped" alone can mean crashed; the plan height alone can mean the
# node is about to reach it.
set -euo pipefail
NAME=${1:?the upgrade name}
BIN=/opt/yamale/bin/blockchaind
NODE=http://127.0.0.1:26657
LIMIT=${LIMIT:-21600}   # give up after 6 h: a plan that never comes is not this job's to wait for

h() { "$BIN" status --node "$NODE" -o json 2>/dev/null | python3 -c 'import sys,json;print(json.load(sys.stdin)["sync_info"]["latest_block_height"])' 2>/dev/null || true; }
plan() { "$BIN" query upgrade plan --node "$NODE" -o json 2>/dev/null | python3 -c 'import sys,json;p=json.load(sys.stdin).get("plan",{});print(p.get("name",""),p.get("height",""))' 2>/dev/null || true; }

read -r PNAME PHEIGHT <<<"$(plan)"
[ "$PNAME" = "$NAME" ] || { echo "the scheduled plan is '${PNAME:-none}', not '$NAME'"; exit 1; }
echo "waiting for $NAME at $PHEIGHT (now $(h))"
start=$(date +%s)
while :; do
  a=$(h); sleep 20; b=$(h)
  if [ -n "$a" ] && [ "$a" = "$b" ] && [ "$a" = "$PHEIGHT" ]; then break; fi
  [ $(( $(date +%s) - start )) -lt "$LIMIT" ] || { echo "no halt at $PHEIGHT after ${LIMIT}s"; exit 1; }
done
echo "halted at $PHEIGHT; crossing"
exec /opt/yamale/bin/swap-upgrade-binary.sh "$NAME"
```

`swap-upgrade-binary.sh` calls `sudo`. As root, that is a no-op where sudo exists, and all three hosts have it.

- [ ] **Step 3: Write `measure-amend-gas.sh`**

The script runs against a running local devnet that has an office and an admitted jeweler with staff. Run it right after `seed-jewel.sh`, which admits "Maison Diop" and leaves `jw-staff` in the keyring. It:
1. registers a fresh jewel `GAS-<unix time>` as `jw-staff`;
2. applies 49 amendments, each with a 500-byte note and a new record hash (`run jewel amend …`; check the CLI's name and flags with `"$BIN" tx jewel --help`);
3. simulates the 50th with `--gas auto --gas-adjustment 1 --dry-run` (or `tx … --generate-only` then `tx simulate`, whichever this SDK's CLI supports), and prints `amendment 50 of 50: <gas> gas`.

The script exits 1 if any step fails. Use the `jewel-lib.sh` helpers (`run`, `committed`, `ok`, `bad`) and the same environment variables as `seed-jewel.sh`.

- [ ] **Step 4: Write `rehearse-jewel-upgrade.sh`**

```bash
#!/usr/bin/env bash
# The jewel-register upgrade, end to end, on a local chain, before devnet-2.
#
#   OLD=$SCRATCH/blockchaind-old.exe NEW=$SCRATCH/blockchaind.exe SIGN=$SCRATCH/jewelsign.exe \
#     scripts/devnet/rehearse-jewel-upgrade.sh
#
# OLD is built from the commit before x/jewel was wired (56feacc^), which is
# what devnet-2 runs in substance. The chain starts with x/nft's own send OPEN,
# so the rehearsal proves the upgrade closes it rather than inheriting it.
set -euo pipefail
OLD=${OLD:?the binary before x/jewel}; NEW=${NEW:?the binary with jewel-register}; SIGN=${SIGN:?jewelsign}
HOME_DIR=.devnet/local/node; CHAIN=yamale-local-1; NODE=tcp://127.0.0.1:26957; KEYRING=test
NAME=jewel-register
GOV=$("$OLD" query auth module-account gov --home "$HOME_DIR" --node "$NODE" -o json 2>/dev/null | grep -oE 'yml1[0-9a-z]{38,}' | head -1 || true)
```

Then, in order. Every step uses `ok`/`bad` from `jewel-lib.sh`, which must be sourced *after* the office exists, because it reads `OFFICE` from the params at load. So `say`/`ok`/`bad` are defined locally until step 6, or the lib is sourced late.

1. `SKIP_NFT_BREAKER=1 BIN="$OLD" scripts/devnet/init-local-devnet.sh`. Then re-read `GOV` with the old binary, and assert `"$OLD" query circuit disabled-list` does not contain `nft`.
2. Submit a `MsgSoftwareUpgrade` (`authority` = gov, `plan.name` = `jewel-register`, `plan.height` = current height + 40) as a gov proposal from `validator`, with deposit `2000uyml` (the local min). Vote `yes` from `validator`. The local voting period is 20 s, and on a local chain voting for oneself is the rehearsal, not a governance act.
3. Wait until the height stops at the plan height (the same test as `await-halt-then-swap.sh`), then `kill "$(cat .devnet/local/node.pid)"`.
4. Restart with `NEW`, using the same command `init-local-devnet.sh` uses (`nohup "$NEW" start --home "$HOME_DIR" --minimum-gas-prices 0uyml > .devnet/local/node.log 2>&1 &`), and write the new pid to `node.pid`. Wait until the height passes the plan height.
5. Assert:
   - `query upgrade applied jewel-register` names the plan height;
   - `query circuit disabled-list` contains `/cosmos.nft.v1beta1.MsgSend`;
   - `query jewel params` shows an empty office and `offer_ttl` 1209600.
6. Appoint the local foundation policy as office through a gov `MsgUpdateParams` (`params` = the queried params with `office` set to the foundation's group policy address, which `init-local-devnet.sh` prints and `query group group-policies-by-group 1` returns). Vote `yes` from `validator`, wait for `PROPOSAL_STATUS_PASSED`, and assert the office is set.
7. Run `BIN="$NEW" SIGN="$SIGN" HOME_DIR=… CHAIN=… NODE=… scripts/devnet/seed-jewel.sh`. Every stage must report ok, including stage 1 (admission, which needs the breaker closed).
8. Run `BIN="$NEW" … scripts/devnet/measure-amend-gas.sh` and print its line.

Print `rehearsal passed` at the end, or exit 1 at the first failure, with the chain's answer bounded as `bad()` does.

- [ ] **Step 5: Build both binaries and run the rehearsal**

```bash
git worktree add "$SCRATCH/old-tree" 56feacc^
(cd "$SCRATCH/old-tree" && go build -o "$SCRATCH/blockchaind-old.exe" ./cmd/blockchaind)
git worktree remove "$SCRATCH/old-tree"
go build -o "$SCRATCH/blockchaind.exe" ./cmd/blockchaind && go build -o "$SCRATCH/jewelsign.exe" ./tools/jewelsign
OLD="$SCRATCH/blockchaind-old.exe" NEW="$SCRATCH/blockchaind.exe" SIGN="$SCRATCH/jewelsign.exe" scripts/devnet/rehearse-jewel-upgrade.sh
```

Expected: `rehearsal passed` and an `amendment 50 of 50: N gas` line.

If the rehearsal fails at seed stage 5 or later, intermittently, while stages 1–4 pass, check whether it is the seeder flake recorded in gaps.md. Run seed-jewel.sh once more against the same chain. If it then passes, record both outputs in the report. Do not hide a real failure behind a rerun.

- [ ] **Step 6: Set `JEWEL_GAS` from the measurement**

If N × 1.3 > 400,000, set `JEWEL_GAS` in `clients/sdk/src/jewel.ts` to N × 1.3, rounded up to the next 50,000. Write the measurement in the constant's comment: "amendment 50 of 50 measured at N gas on 2026-11-13; 30% headroom". If it fits, only add that comment. Run `(cd clients/sdk && npm test)`.

- [ ] **Step 7: Run the app smoke test on the upgraded chain**

With the rehearsed chain still running:

```bash
go build -o "$SCRATCH/loyaltyapi.exe" ./tools/loyaltyapi
BIN="$SCRATCH/blockchaind.exe" API_BIN="$SCRATCH/loyaltyapi.exe" scripts/devnet/loyalty-local.sh   # in the background
```

Then run `clients/loyalty/scripts/smoke.ts` exactly as `clients/README.md` describes. Expected: `all checks passed`. Stop loyalty-api and the node afterwards.

- [ ] **Step 7b: Check the local admission path the office will use**

On the same chain, run `OFFICE_VOTES=3 BIN=… HOME_DIR=… CHAIN=… NODE=… scripts/devnet/loyalty-admit.sh Maison Rehearsal <a fresh keyring address>`. Expected: `ok admitted Maison Rehearsal`.

- [ ] **Step 8: Commit**

```bash
git add scripts/devnet clients/sdk/src/jewel.ts
git commit -m "devnet: rehearse jewel-register end to end; a halt watcher that runs on the host; amendment gas measured"
```

---

### Task 6: loyalty-api reports a low sponsor balance

**Files:**
- Create: `tools/loyaltyapi/balance.go`, `tools/loyaltyapi/balance_test.go`
- Modify: `tools/loyaltyapi/main.go` (flag, server fields, route)

**Interfaces:**
- Produces: `type BalanceReader interface { Balance(ctx context.Context, address, denom string) (*big.Int, error) }`, implemented by `restChain`.
- Produces: `GET /v1/health/sponsor`:
  - 200 `{"sponsoring":false}` when sponsoring is off;
  - 200 `{"ok":true,"balance":"…","floor":"…"}` when the balance is at or above the floor;
  - 503 `{"ok":false,…}` when it is below;
  - 502 from `chainUnreachable` when the balance cannot be read.
- Produces: the flag `--sponsor-floor` (default `10000000uyml`). The denom is parsed from the value.

- [ ] **Step 1: Write the failing tests**

`tools/loyaltyapi/balance_test.go`:

```go
package main

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeBalance struct {
	amount *big.Int
	err    error
}

func (f fakeBalance) Balance(context.Context, string, string) (*big.Int, error) { return f.amount, f.err }

type fixedGranter struct{}

func (fixedGranter) Address() string                                  { return "yml1sponsor" }
func (fixedGranter) Grant(context.Context, string) (string, error)    { return "", nil }

func sponsorHealth(t *testing.T, s *server) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/health/sponsor", nil))
	return w
}

func TestSponsorHealthWhenSponsoringIsOff(t *testing.T) {
	s := testServer(t)
	w := sponsorHealth(t, s)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"sponsoring":false}`, w.Body.String())
}

func TestSponsorHealthAnswers503BelowTheFloor(t *testing.T) {
	s := testServer(t)
	s.granter = fixedGranter{}
	s.sponsorFloor, s.sponsorDenom = big.NewInt(10_000_000), "uyml"
	s.balances = fakeBalance{amount: big.NewInt(9_999_999)}
	require.Equal(t, http.StatusServiceUnavailable, sponsorHealth(t, s).Code)
	s.balances = fakeBalance{amount: big.NewInt(10_000_000)}
	require.Equal(t, http.StatusOK, sponsorHealth(t, s).Code)
}

func TestSponsorHealthHidesTheChainError(t *testing.T) {
	s := testServer(t)
	s.granter = fixedGranter{}
	s.sponsorFloor, s.sponsorDenom = big.NewInt(1), "uyml"
	s.balances = fakeBalance{err: errors.New(`Get "http://10.0.0.7:1317/cosmos/bank": down`)}
	w := sponsorHealth(t, s)
	require.Equal(t, http.StatusBadGateway, w.Code)
	require.NotContains(t, w.Body.String(), "10.0.0.7")
}
```

If the `Granter` interface in `sponsor.go` has a different method set, make `fixedGranter` satisfy it. Run: `go test ./tools/loyaltyapi -run SponsorHealth -count=1`. Expected: FAIL to compile (`s.balances` undefined).

- [ ] **Step 2: Implement**

`tools/loyaltyapi/balance.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
)

// BalanceReader reads one account's balance of one denom.
type BalanceReader interface {
	Balance(ctx context.Context, address, denom string) (*big.Int, error)
}

func (c restChain) Balance(ctx context.Context, address, denom string) (*big.Int, error) {
	u := c.base + "/cosmos/bank/v1beta1/balances/" + url.PathEscape(address) + "/by_denom?denom=" + url.QueryEscape(denom)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the chain did not answer: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the chain answered %d for a balance", res.StatusCode)
	}
	var body struct {
		Balance struct {
			Amount string `json:"amount"`
		} `json:"balance"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("unreadable balance: %w", err)
	}
	n, ok := new(big.Int).SetString(body.Balance.Amount, 10)
	if !ok {
		return nil, fmt.Errorf("unreadable amount %q", body.Balance.Amount)
	}
	return n, nil
}

// handleSponsorHealth lets the monitor alert before the sponsor runs dry: a
// sponsor that cannot pay leaves every new owner unable to accept a jewel.
func (s *server) handleSponsorHealth(w http.ResponseWriter, r *http.Request) {
	if s.granter == nil {
		writeJSON(w, http.StatusOK, map[string]any{"sponsoring": false})
		return
	}
	have, err := s.balances.Balance(r.Context(), s.granter.Address(), s.sponsorDenom)
	if err != nil {
		chainUnreachable(w, err)
		return
	}
	body := map[string]any{"ok": have.Cmp(s.sponsorFloor) >= 0, "balance": have.String() + s.sponsorDenom,
		"floor": s.sponsorFloor.String() + s.sponsorDenom}
	if have.Cmp(s.sponsorFloor) < 0 {
		writeJSON(w, http.StatusServiceUnavailable, body)
		return
	}
	writeJSON(w, http.StatusOK, body)
}
```

`main.go`:
- Add the server fields `balances BalanceReader`, `sponsorFloor *big.Int` and `sponsorDenom string`.
- Add `floor := flag.String("sponsor-floor", "10000000uyml", "below this sponsor balance /v1/health/sponsor answers 503")`.
- Parse it with a small helper `splitCoin(s string) (*big.Int, string, error)` (digits, then a denom matching `[a-z][a-z0-9/]{2,127}`); `log.Fatal` if it is malformed.
- Set `s.balances = s.chain.(restChain)` from the `restChain` value constructed there; keep the value in a variable and assign it to both `chain` and `balances`.
- Register `mux.HandleFunc("GET /v1/health/sponsor", s.handleSponsorHealth)`.
- Add a `TestSplitCoin` table test in `balance_test.go`: `"10000000uyml"` parses; `"uyml"`, `"10"` and `"-5uyml"` are refused.

- [ ] **Step 3: Run the tests and commit**

```bash
gofmt -l tools/loyaltyapi; go vet ./tools/loyaltyapi && go test ./tools/loyaltyapi -count=1
git add tools/loyaltyapi
git commit -m "loyaltyapi: /v1/health/sponsor answers 503 when the sponsor runs low"
```

---

### Task 7: The gate serves loyalty.yamaletech.com

**Files:**
- Create: `deploy/yamale-us/gate/loyalty.conf`
- Modify: `deploy/yamale-us/gate/nginx.conf`, `deploy/yamale-us/gate/docker-compose.yml`, `deploy/yamale-us/gate/deploy-gate.sh`

**Interfaces:**
- Produces: a server on `127.0.0.1:8082` with root `/srv/loyalty`. It serves the app with an SPA fallback, `/api/rpc/` (rpcgate), `/api/custodian/` and `/api/recovery/` (the existing account snippet), and `/api/loyalty/` (loyalty-api on 127.0.0.1:8095), with the gate's security headers.

- [ ] **Step 1: Write `loyalty.conf`**

```nginx
# loyalty.yamaletech.com: the jewel app at the root of its own host, and its API.
#
# The app is built with base "/" (its certificate paths /j/<jeweler>/<jewel>
# are real paths, and the fragment is reserved for claim keys), so it cannot
# share /srv/chain's one-path-per-app layout; it has this server of its own.
#
# No location here calls add_header: one that did would drop every header the
# server block sets (nginx-add-header-replaces), the CSP among them, and the
# CSP is what keeps an injected script from sending a claim key anywhere.

# The API. Reads at the general rate; writes (a photo, a record, a profile, an
# application) at a per-visitor write rate, because loyalty-api has no
# per-client limit of its own and files and the application spool are
# unbounded.
location /api/loyalty/ {
    limit_req zone=yamale_public burst=200 nodelay;
    limit_req zone=yamale_loyalty_write burst=30 nodelay;
    limit_req_status 429;
    # A photo is at most 8 MiB (tools/loyaltyapi/files.go maxFileBytes).
    client_max_body_size 9m;
    # Streamed, not buffered: loyalty-api hashes while it writes, and the
    # container's tmpfs is memory.
    proxy_request_buffering off;
    proxy_read_timeout 120s;
    proxy_pass http://127.0.0.1:8095/;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
}

# Sponsorship: a fresh key per request would otherwise exhaust the daily
# ceiling (--sponsor-daily) for everybody. GET (who sponsors) stays at the
# general rate; the map gives POST alone a key.
location = /api/loyalty/v1/sponsor {
    limit_req zone=yamale_public burst=200 nodelay;
    limit_req zone=yamale_loyalty_sponsor burst=2 nodelay;
    limit_req_status 429;
    proxy_pass http://127.0.0.1:8095/v1/sponsor;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
}

# The operator's list of applications is for the operator, from the host.
location = /api/loyalty/v1/applications {
    limit_req zone=yamale_loyalty_write burst=5 nodelay;
    limit_req_status 429;
    if ($request_method = GET) { return 404; }
    client_max_body_size 32k;
    proxy_pass http://127.0.0.1:8095/v1/applications;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
}

# The balance check is for the monitor, on the host.
location = /api/loyalty/v1/health/sponsor { return 404; }

# Built assets never fall back to the app: a missing chunk is a 404, not
# index.html served as JavaScript.
location /assets/ { try_files $uri =404; }

# Every other path is a route of the app (/j/…, /claim, /mine/…, /atelier/…).
location / { try_files $uri $uri/ /index.html; }
```

- [ ] **Step 2: Wire it into `nginx.conf`**

In the `http` block, after the existing `limit_req_zone` lines:

```nginx
    # loyalty.yamaletech.com. The key is empty for methods that write nothing,
    # and an empty key is not limited.
    map $request_method $loyalty_write_key { POST $binary_remote_addr; PUT $binary_remote_addr; default ""; }
    map $request_method $loyalty_sponsor_key { POST $binary_remote_addr; default ""; }
    limit_req_zone $loyalty_write_key zone=yamale_loyalty_write:4m rate=30r/m;
    limit_req_zone $loyalty_sponsor_key zone=yamale_loyalty_sponsor:4m rate=2r/m;
```

After the existing `server { … }` block:

```nginx
    # loyalty.yamaletech.com, at the root of its own host (see snippets/loyalty.conf).
    server {
        listen 127.0.0.1:8082;
        server_name _;
        set_real_ip_from 127.0.0.1;
        set_real_ip_from ::1;
        real_ip_header X-Forwarded-For;

        root /srv/loyalty;
        index index.html;
        include /etc/nginx/snippets/yamale-headers.conf;
        include /etc/nginx/snippets/yamale-rpc.conf;
        include /etc/nginx/snippets/yamale-account.conf;
        include /etc/nginx/snippets/loyalty.conf;
    }
```

Read `deploy/nginx/yamale-rpc.conf` and `yamale-account.conf` first:
- Confirm that neither depends on a zone or map defined only in `yamale-api.conf` or `yamale-visibility.conf`.
- Confirm that the zones they name (`yamale_rpc`, `yamale_enrol`, `yamale_sign`, `yamale_public`) are all in this `http` block.

If `yamale-rpc.conf` needs the visibility snippet, include `yamale-api.conf` instead, and record why in a comment.

- [ ] **Step 3: Mount the app and ship the snippet**

`docker-compose.yml`, under `volumes`: `- /srv/loyalty:/srv/loyalty:ro`. Update the header comment's list of upstreams with `loyalty-api on 8095`.

`deploy-gate.sh`:
- after the `for f in …` loop: `tr -d '\r' < loyalty.conf > "$T/snippets/loyalty.conf"`;
- in the remote part, before the validation container: `sudo install -d -m 755 /srv/loyalty`;
- in the validation `docker run`: add `-v /srv/loyalty:/srv/loyalty:ro`;
- at the end, three more probes:

```bash
curl -s -o /dev/null -w "loyalty /                       %{http_code}\n" http://127.0.0.1:8082/
curl -s -o /dev/null -w "loyalty /api/rpc/status          %{http_code}\n" http://127.0.0.1:8082/api/rpc/status
curl -s -o /dev/null -w "loyalty /api/loyalty/v1/health   %{http_code}\n" http://127.0.0.1:8082/api/loyalty/v1/health
```

Before Task 8 has installed the app, `/` answers 404 and the API answers 502. The script only prints these; it does not fail on them.

- [ ] **Step 4: Validate locally**

```bash
MSYS_NO_PATHCONV=1 docker run --rm -v "$PWD/deploy/yamale-us/gate/nginx.conf:/etc/nginx/nginx.conf:ro" \
  -v "$PWD/deploy/nginx:/etc/nginx/snippets-src:ro" nginx:stable-alpine sh -c \
  'mkdir -p /etc/nginx/snippets /etc/nginx/private /srv/chain /srv/loyalty && cp /etc/nginx/snippets-src/*.conf /etc/nginx/snippets/ && touch /etc/nginx/yamale-supervisors && nginx -t'
```

Copy `deploy/yamale-us/gate/loyalty.conf` into the snippets the same way (mount it and `cp` it). If Docker is not available on this machine, say so in the report. `deploy-gate.sh` runs the same `nginx -t` on the host before swapping anything, and Task 12 runs it.

Expected: `syntax is ok` and `test is successful`.

- [ ] **Step 5: Commit**

```bash
git add deploy/yamale-us/gate
git commit -m "gate: loyalty.yamaletech.com on :8082, with per-visitor limits on writes and sponsorship"
```

---

### Task 8: The installer for yamale-us

**Files:**
- Create: `deploy/yamale-us/loyalty/install-loyalty.sh`, `deploy/yamale-us/loyalty/jewel-admit`

**Interfaces:**
- Produces on yamale-us:
  - user `yamale-loyalty`;
  - `/opt/yamale/bin/yamale-loyaltyapi`;
  - keyring home `/opt/yamale/loyalty`, holding `loyalty-sponsor` only;
  - data `/var/lib/yamale-loyalty`;
  - token `/etc/yamale/loyalty-operator-token` (root:yamale-loyalty 640);
  - unit `yamale-loyalty.service` (127.0.0.1:8095);
  - timer `yamale-loyalty-backup.timer`;
  - the app in `/srv/loyalty`;
  - `/opt/yamale/bin/jewel-admit`, `jewel-lib.sh`, `loyalty-admit.sh`.
- Consumes: the faucet on `127.0.0.1:8085` (`POST {"address":"…"}` → 1000 YML), and the gate from Task 7.
- `install-loyalty.sh` has two modes: `--api` (binary, keys, unit, backup) and `--app` (the static app only). It runs both when given neither.

- [ ] **Step 1: Write `install-loyalty.sh`**

Model it on `deploy/yamale-us/faucet/install-faucet.sh`: build locally, tar over ssh, one remote script. Requirements, each a line or block in the script:

**Local half**
- Refuse if any `clients/loyalty/.env*` file exists. Vite would inline it, as happened in V2 (memory: v2-build-inherits-env-local).
- Build the app:

  ```bash
  (cd clients && VITE_CHAIN_ID=yamale-devnet-2 VITE_PUBLIC_ORIGIN=https://loyalty.yamaletech.com VITE_GAS_PRICE=0 npm run build --workspace @yamale/loyalty)
  ```

  Then check the bundle: `grep -rl yamale-devnet-2 clients/loyalty/dist/assets/*.js` must match, and `grep -rl yamale-local-1` must not.
- Build the API: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$T/yamale-loyaltyapi" ./tools/loyaltyapi`.
- Copy `scripts/devnet/jewel-lib.sh`, `scripts/devnet/loyalty-admit.sh` and `deploy/yamale-us/loyalty/jewel-admit` into the tarball.

**Remote half: the API (`--api`)**
- `id yamale-loyalty || useradd --system --home-dir /opt/yamale/loyalty --no-create-home --shell /usr/sbin/nologin yamale-loyalty`.
- Install the binary to `/opt/yamale/bin/yamale-loyaltyapi` (root, 755), plus the three scripts (755).
- Directories:
  - `/opt/yamale/loyalty` and `/opt/yamale/loyalty/keyring-test`: yamale-loyalty, 700;
  - `/var/lib/yamale-loyalty`: yamale-loyalty, 750;
  - `/var/backups/yamale-loyalty`: root, 700.
- The sponsor key, created only if absent. The mnemonic is never shown:

  ```bash
  sudo -u yamale-loyalty /opt/yamale/bin/blockchaind keys show loyalty-sponsor -a --keyring-backend test --home /opt/yamale/loyalty >/dev/null 2>&1 \
    || sudo -u yamale-loyalty /opt/yamale/bin/blockchaind keys add loyalty-sponsor --keyring-backend test --home /opt/yamale/loyalty >/dev/null 2>&1
  ```

  Then print its address (an address is not a secret), and assert that the keyring lists exactly one key.
- Fund the sponsor if its balance is below 100 YML:
  - `curl -fsS -X POST http://127.0.0.1:8085/ -H 'Content-Type: application/json' -d "{\"address\":\"$ADDR\"}"`;
  - then poll `blockchaind query bank balance` for up to 30 s, and fail if the balance is still below 100 YML.
- The operator token, only if absent. It is never printed:

  ```bash
  sudo test -s /etc/yamale/loyalty-operator-token || (umask 077; head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' | sudo tee /etc/yamale/loyalty-operator-token >/dev/null)
  sudo chown root:yamale-loyalty /etc/yamale/loyalty-operator-token && sudo chmod 640 /etc/yamale/loyalty-operator-token
  ```

- The unit (unquoted heredoc, as the faucet's):

  ```ini
  [Unit]
  Description=Yamale loyalty-api (loyalty.yamaletech.com): files, profiles, applications, fee sponsorship
  After=network-online.target yamale-devnet.service
  Wants=network-online.target

  [Service]
  User=yamale-loyalty
  Group=yamale-loyalty
  ExecStart=/opt/yamale/bin/yamale-loyaltyapi --listen 127.0.0.1:8095 --dir /var/lib/yamale-loyalty --rest http://127.0.0.1:1317 --chain-id yamale-devnet-2 --bin /opt/yamale/bin/blockchaind --from loyalty-sponsor --home /opt/yamale/loyalty --keyring-backend test --node tcp://127.0.0.1:26657 --sponsor 2000000uyml --sponsor-for 8760h --fees 0uyml --sponsor-daily 200 --sponsor-floor 10000000uyml --operator-token-file /etc/yamale/loyalty-operator-token
  Restart=always
  RestartSec=5
  MemoryMax=256M
  NoNewPrivileges=true
  ProtectSystem=strict
  ProtectHome=true
  PrivateTmp=true
  ReadWritePaths=/var/lib/yamale-loyalty /opt/yamale/loyalty

  [Install]
  WantedBy=multi-user.target
  ```

- The backup: `yamale-loyalty-backup.service` (Type=oneshot, root) and `yamale-loyalty-backup.timer` (`OnCalendar=*-*-* 03:40:00 UTC`, `Persistent=true`). ExecStart runs:

  ```bash
  /bin/sh -c 'set -e; f=/var/backups/yamale-loyalty/loyalty-$(date -u +%%Y%%m%%d).tgz; o=; [ -d /opt/yamale/jewel-office ] && o=opt/yamale/jewel-office; tar -czf "$f.partial" -C / var/lib/yamale-loyalty opt/yamale/loyalty $o; gzip -t "$f.partial"; mv "$f.partial" "$f"'
  ```

  It also covers the office's keyring (`/opt/yamale/jewel-office`, created in Task 11) once that exists: its mnemonics are never kept, so the keyring is the only copy.

  with ExecStartPost `find /var/backups/yamale-loyalty -name "loyalty-*.tgz" -mtime +14 -delete`. Comment in the unit: the copy is on the same machine and protects against mistakes, not against losing the server; an off-server copy is open (gaps.md).
- `systemctl daemon-reload`, `enable --now yamale-loyalty yamale-loyalty-backup.timer`, `restart yamale-loyalty`. Then poll `curl -fsS 127.0.0.1:8095/v1/health` for 15 s and print `curl -s 127.0.0.1:8095/v1/health/sponsor`.

**Remote half: the app (`--app`)**
- Unpack `dist` into a staging directory.
- Keep the old tree: `cp -a /srv/loyalty /srv/loyalty.prev-$(date -u +%Y%m%d%H%M)` when it has content.
- Then **copy into** `/srv/loyalty` (`find /srv/loyalty -mindepth 1 -delete && cp -a "$S/dist/." /srv/loyalty/`). Do not swap by rename: the gate container bind-mounts `/srv/loyalty`, and a rename would leave it serving the old directory.
- Owner root, directories 755, files 644.
- Probe: `curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8082/j/1/1` must print 200 (the SPA fallback).

Header comment: what the script installs, the two modes, that it never prints a key or token, and the bind-mount reason for copying in place.

- [ ] **Step 2: Write `jewel-admit`**

```bash
#!/usr/bin/env bash
# jewel-admit <jeweler name…> <business account>: the office admits a jeweler on devnet-2.
#
# Two of the office's three members vote (OFFICE_VOTES=2), the third funds the
# business with 50 YML. The name is every argument but the last, joined by
# single spaces, so the command survives PowerShell and ssh without quoting:
#
#   ssh -t yamale-us sudo /opt/yamale/bin/jewel-admit Maison Diop yml1...
set -euo pipefail
[ $# -ge 2 ] || { echo "usage: jewel-admit <name…> <business account>" >&2; exit 2; }
BUSINESS=${!#}
NAME=${*:1:$#-1}
export BIN=/opt/yamale/bin/blockchaind HOME_DIR=/opt/yamale/jewel-office CHAIN=yamale-devnet-2 \
  NODE=tcp://127.0.0.1:26657 KEYRING=test M1=jewel-office-1 M2=jewel-office-2 M3=jewel-office-3 \
  OFFICE_VOTES=2 FUNDER=jewel-office-3
exec /opt/yamale/bin/loyalty-admit.sh "$NAME" "$BUSINESS"
```

`loyalty-admit.sh` sources `"$(dirname "$0")/jewel-lib.sh"`, so both are installed side by side in `/opt/yamale/bin`.

- [ ] **Step 3: Check and commit**

```bash
bash -n deploy/yamale-us/loyalty/install-loyalty.sh && bash -n deploy/yamale-us/loyalty/jewel-admit
grep -n "keys add\|keys export\|mnemonic" deploy/yamale-us/loyalty/install-loyalty.sh
git add deploy/yamale-us/loyalty
git commit -m "deploy: install loyalty-api and the jewel app on yamale-us"
```

The `grep` output must show only the `keys add … >/dev/null 2>&1` line.

---

### Task 9: Caddy, ports and the monitor (yamale-platform)

**Repository:** `D:\Sermium\Yamale\yamale-platform`. Commit there, locally only.

**Files:**
- Create: `caddy/sites/loyalty.yamaletech.com.caddy`
- Modify: `PORTS.md`, `monitor/yamale-monitor-host.conf`

- [ ] **Step 1: The site**

```caddy
# loyalty.yamaletech.com: the jewel app (Blockchain repo, clients/loyalty), served at the
# root of its own host by the chain gate's second server (127.0.0.1:8082). Not chain_app:
# that snippet redirects / to /<app>/ and this app lives at /.
loyalty.yamaletech.com {
	import common
	import access_log loyalty.yamaletech.com
	reverse_proxy 127.0.0.1:8082 {
		import visitor
	}
}
```

- [ ] **Step 2: Ports**

In `PORTS.md`:
- add a row `| 8082 | chain gate, loyalty server (app + /api/loyalty) | Blockchain | loyalty.yamaletech.com |`;
- replace the 8095 row with `| 8095 | loyalty-api (\`yamale-loyalty\`) | Blockchain | via 8082 only |`.

- [ ] **Step 3: The monitor**

In `monitor/yamale-monitor-host.conf`:
- append `,yamale-loyalty=Loyalty API` to `MONITOR_UNITS`;
- append `,Loyalty=https://loyalty.yamaletech.com/j/1/1,Loyalty API=https://loyalty.yamaletech.com/api/loyalty/v1/health,Loyalty sponsor=http://127.0.0.1:8095/v1/health/sponsor` to `MONITOR_PROBES`.

Keep each `Environment=` on one line, as the file does.

- [ ] **Step 4: Check and commit (local only)**

```bash
cd /d/Sermium/Yamale/yamale-platform
./deploy.sh --check
git add caddy/sites/loyalty.yamaletech.com.caddy PORTS.md monitor/yamale-monitor-host.conf
git commit -m "loyalty.yamaletech.com: the site, its ports and its probes"
```

`--check` validates against the live Caddy without installing. Expected: every check passes. It may report the site as unreachable until Task 12: that is expected, not a failure.

---

### Task 10: The upgrade on devnet-2 (controller, with the user)

**Gate:** Tasks 1–9 merged to local `main`, the full Go suite green in both profiles, and the rehearsal from Task 5 passed on that `main`.

- [ ] **Step 1: Build and stage**

```bash
export PATH="/c/Program Files/Go/bin:$PATH"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$SCRATCH/blockchaind-linux-amd64" ./cmd/blockchaind
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$SCRATCH/blockchaind-linux-arm64" ./cmd/blockchaind
```

On each host:
- copy the binary for its architecture to `/tmp`, then `sudo install -m 755 /tmp/blockchaind-linux-<arch> /opt/yamale/bin/blockchaind.next`;
- install `scripts/devnet/await-halt-then-swap.sh` AND `scripts/devnet/swap-upgrade-binary.sh` to `/opt/yamale/bin/` (755). The hosts carry an older swap script that moves `.next` and loses it on a rollback; the branch's copy keeps it (final review, 2026-11-14);
- check the node's data directory is owned by the user its unit runs as (`systemctl cat yamale-devnet | grep -E "^User|--home"`, then `stat -c %U <home>/data`). At the halt the old binary writes `upgrade-info.json` there, and the new binary needs it to add the jewel store; the Pi's `/opt/yamale/join-node/data` has been root-owned before;
- on the Pi and the VM, check the master answers from there: `/opt/yamale/bin/blockchaind status --node https://rpc.yamaletech.com:443 -o json` must print a height. That address is the followers' `AFTER_NODE` in Step 5;
- run `/opt/yamale/bin/blockchaind.next version` (exit 0 required) and `… query jewel --help` (must list the jewel queries).

Use the ssh commands from the hosts table. Check that `/opt/yamale/bin/blockchaind.pre-jewel-register` does not exist yet on any host.

- [ ] **Step 2: Pre-flight, read-only**
- Voting power: `curl -s 127.0.0.1:26657/validators` on yamale-us. The master must hold more than 2/3, so yamale-us restarts the chain alone.
- Which key carries the vote: on the VM, `blockchaind query staking delegations <alice address> --node tcp://127.0.0.1:26657 -o json`. Sum alice's delegations to bonded validators against `query staking pool`'s bonded tokens. Quorum is 33.4%.
- If alice's share is below quorum, say so and name the key that carries it (re-derive as in the devnet2-upgrade-run-recipe memory) before proposing anything.
- Current height `H0`; halt height `H = H0 + 1100` (about 90 minutes at about 4.9 s a block).
- `query upgrade plan` is empty.

- [ ] **Step 3: Ask the user, in chat, whether to submit now.** Wait for a yes.

- [ ] **Step 4: Submit the proposal from the VM**

Write `/tmp/jewel-register.json` on the VM:

```json
{
  "messages": [{
    "@type": "/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade",
    "authority": "yml10d07y265gmmuvt4z0w9aw880jnsr700jz5s386",
    "plan": { "name": "jewel-register", "height": "<H>", "info": "x/jewel: the register of jewels; closes x/nft's own MsgSend in x/circuit" }
  }],
  "deposit": "1000000uyml",
  "title": "jewel-register: the register of jewels",
  "summary": "Adds x/jewel (loyalty.yamaletech.com). The upgrade closes x/nft's own send in x/circuit in the same block, and appoints no office: that is a separate proposal."
}
```

Then:

```bash
/opt/yamale/bin/blockchaind tx gov submit-proposal /tmp/jewel-register.json --from alice --keyring-backend test --home /opt/yamale/node --chain-id yamale-devnet-2 --node tcp://127.0.0.1:26657 --gas 400000 --fees 2000uyml -y -o json
```

Confirm in the block with `query tx <hash>`: `code: 0` in the CLI output is CheckTx only. Read the proposal id.

- [ ] **Step 5: Install the halt jobs on all three hosts, then hand over the vote**

On yamale-us (the master; it never waits for another node):

```bash
sudo systemd-run --unit yamale-upgrade-jewel-register --collect /opt/yamale/bin/await-halt-then-swap.sh jewel-register
```

On the Pi and the VM (followers: after their own halt they wait until the master is past `H`, so their 60 s start check cannot roll them back):

```bash
sudo systemd-run --unit yamale-upgrade-jewel-register --collect --setenv=AFTER_NODE=https://rpc.yamaletech.com:443 /opt/yamale/bin/await-halt-then-swap.sh jewel-register
```

The watcher waits for the plan to be scheduled, so starting it before the vote is correct. Check each started: `journalctl -u yamale-upgrade-jewel-register -n 5` shows "no plan scheduled yet" polls. If the plan is cancelled and re-proposed, stop and restart the jobs: the watcher reads the plan height once.

Then hand the user this command, filling in the id. It is one line with one pair of quotes, so it survives PowerShell:

```
ssh -i ~/.ssh/yamale_oracle ubuntu@92.4.151.72 "/opt/yamale/bin/blockchaind tx gov vote <id> yes --from alice --keyring-backend test --home /opt/yamale/node --chain-id yamale-devnet-2 --node tcp://127.0.0.1:26657 --gas 200000 --fees 1000uyml -y"
```

State plainly:
- the 30-minute window;
- that alice's share (from Step 2) carries quorum;
- that an unvoted proposal does nothing and costs only the deposit.

Watch the tally (`query gov tally <id>`) without voting.

- [ ] **Step 6: Watch the crossing**

At the halt height, each host's job swaps its binary and restarts. Watch with `systemctl status yamale-upgrade-jewel-register` and `journalctl -u yamale-upgrade-jewel-register -n 50` (bounded).
- yamale-us must leave `H` on its own.
- The Pi and the VM follow. The VM may be jailed after restarting; its auto-unjail handles that after the 10-minute jail.

If a job reports a rollback, stop and diagnose (systematic-debugging). The chain stays halted at `H`, which is safe.

- [ ] **Step 7: Verify on the live chain**

```bash
blockchaind query upgrade applied jewel-register --node tcp://127.0.0.1:26657
blockchaind query circuit disabled-list --node tcp://127.0.0.1:26657 -o json
blockchaind query jewel params --node tcp://127.0.0.1:26657 -o json
journalctl -u yamale-devnet -n 300 --no-pager | grep -E "register of jewels arrived|applying upgrade"
```

Expected:
- applied at `H`;
- the disabled list holds `/cosmos.nft.v1beta1.MsgSend`;
- the office is empty, with default params;
- the log line is present.

Also check that all three validators are signing (the monitor's chain set, or `/validators` and recent commits).

---

### Task 11: Appoint the office (controller, with the user)

- [ ] **Step 1: Create the three member keys on yamale-us**

The mnemonics go to `/dev/null`: these are devnet office keys, recoverable only through the keyring, which Task 8's nightly backup covers from the night after it exists.

```bash
sudo install -d -m 700 /opt/yamale/jewel-office
for k in jewel-office-1 jewel-office-2 jewel-office-3; do
  sudo /opt/yamale/bin/blockchaind keys add $k --keyring-backend test --home /opt/yamale/jewel-office >/dev/null 2>&1
done
sudo /opt/yamale/bin/blockchaind keys list --keyring-backend test --home /opt/yamale/jewel-office --list-names
```

Fund each through the local faucet (`curl -X POST http://127.0.0.1:8085/ -d '{"address":"…"}'`), then confirm each balance.

- [ ] **Step 2: Create the 2-of-3 group with its policy**

`/tmp/members.json`:

```json
{ "members": [
  {"address": "<jewel-office-1>", "weight": "1", "metadata": "jewel office member 1"},
  {"address": "<jewel-office-2>", "weight": "1", "metadata": "jewel office member 2"},
  {"address": "<jewel-office-3>", "weight": "1", "metadata": "jewel office member 3"} ] }
```

`/tmp/policy.json`:

```json
{ "@type": "/cosmos.group.v1.ThresholdDecisionPolicy", "threshold": "2",
  "windows": { "voting_period": "86400s", "min_execution_period": "0s" } }
```

```bash
sudo /opt/yamale/bin/blockchaind tx group create-group-with-policy <jewel-office-1 address> "Registre des bijoux — Yamale" "jewel office" /tmp/members.json /tmp/policy.json --group-policy-as-admin --from jewel-office-1 --keyring-backend test --home /opt/yamale/jewel-office --chain-id yamale-devnet-2 --node tcp://127.0.0.1:26657 --gas auto --gas-adjustment 1.5 -y -o json
```

Confirm in the block. Find the group by walking `query group groups` for that metadata. `--group-policy-as-admin` means `group-policies-by-admin` finds nothing; see the group-proposal-seeder-mechanics memory. Then read the policy address from `query group group-policies-by-group <id>`.

- [ ] **Step 3: Propose the appointment, then hand over the vote**

Ask the user in chat before submitting. The proposal carries the full params: start from the live `query jewel params` and set `office`.

```json
{
  "messages": [{
    "@type": "/blockchain.jewel.v1.MsgUpdateParams",
    "authority": "yml10d07y265gmmuvt4z0w9aw880jnsr700jz5s386",
    "params": { "office": "<policy address>", "offer_ttl": "1209600", "voucher_max_ttl": "300", "reassignment_window": "2592000" }
  }],
  "deposit": "1000000uyml",
  "title": "Appoint the register of jewels' office",
  "summary": "Appoints the 2-of-3 group \"Registre des bijoux — Yamale\" as x/jewel's office: it admits and suspends jewelers. Settings unchanged."
}
```

Use the field names and values exactly as `query jewel params -o json` prints them. `Params.Validate()` and the group-account check run at execution, so a wrong shape passes the vote and fails as `PROPOSAL_STATUS_FAILED`.

Submit from `alice` on the VM as in Task 10, Step 4. Hand over the same vote command with the new id. Then wait for `PROPOSAL_STATUS_PASSED`, and confirm the office in `query jewel params`.

---

### Task 12: Deploy and verify loyalty.yamaletech.com (controller)

- [ ] **Step 1: Install, in order**

```bash
deploy/yamale-us/gate/deploy-gate.sh
deploy/yamale-us/loyalty/install-loyalty.sh
(cd /d/Sermium/Yamale/yamale-platform && ./deploy.sh)
```

Expected:
- the gate's `nginx -t` passes, and its existing probes are unchanged (rpc 200, rest 200, unlisted path 401/403);
- the loyalty probes answer 200 once the installer has run;
- Caddy reloads and its post-reload probe passes.

- [ ] **Step 2: DNS and certificate**

`loyalty.yamaletech.com` is covered by the proxied wildcard `*` record at Cloudflare, so no DNS change is needed. Check:
- `curl -sI https://loyalty.yamaletech.com/` answers through Cloudflare (`server: cloudflare`) with 200;
- Caddy obtained its own certificate (no TLS error in `journalctl -u caddy -n 100`).

If the edge certificate is missing, stop; the order is in the cloudflare-in-front memory.

- [ ] **Step 3: Verify from the public hostname**

From this machine, which is not on yamale-us's loopback:

```bash
for p in / /j/1/1 /claim /api/rpc/status /api/loyalty/v1/health /api/custodian/v1/health /api/recovery/v1/health; do
  curl -s -o /dev/null -w "%{http_code}  $p\n" "https://loyalty.yamaletech.com$p"; done
curl -sI https://loyalty.yamaletech.com/ | grep -iE "content-security-policy|referrer-policy|x-frame-options|strict-transport"
curl -s -o /dev/null -w "%{http_code}  sponsor health from outside (must be 404)\n" https://loyalty.yamaletech.com/api/loyalty/v1/health/sponsor
curl -s -o /dev/null -w "%{http_code}  application list from outside (must be 404)\n" https://loyalty.yamaletech.com/api/loyalty/v1/applications
```

Expected:
- 200 on every path in the loop;
- CSP present, with `connect-src 'self'`;
- a Referrer-Policy header present;
- 404 on the last two.

Then the monitor: `systemctl status yamale-monitor` is active, and status.yamaletech.com lists the three loyalty probes as up.

- [ ] **Step 4: The walk-through, with the user**

- Open `https://loyalty.yamaletech.com` in the built-in browser. Check it at 390 px and at 1600 px, in both themes, and in Arabic, which has not yet been seen rendering (RTL). Take screenshots, and report anything broken as a defect rather than fixing it inline.
- The user creates a business account in the app (Account tab) and gives the account code in chat. Hand over:

  ```
  ssh -t yamale-us sudo /opt/yamale/bin/jewel-admit <Name words> <account code>
  ```

  The office's two votes then run on yamale-us; the user runs the command.
- With the user, on their devices:
  - register a ring (staff = the business account, or a staff member it adds);
  - offer it by QR;
  - claim it on a phone as a second person;
  - check that the public page `/j/<jeweler>/<jewel>` shows one owner and the declared sentence.

  The claim key must never appear in any server log. Check this with `sudo grep -c "claim#" /var/log/caddy/loyalty.yamaletech.com.access.log`: expect 0, since fragments are never sent.

---

### Task 13: Record it

- [ ] **Step 1: gaps.md**

In `docs/scope/gaps.md`:
- **Update the `x/jewel` row:**
  - applied on devnet-2 at height `H`, on the date;
  - the proposal ids;
  - the office's group and policy address;
  - live at https://loyalty.yamaletech.com;
  - commits `426586c..<head>`.
- **Move into the row, or strike** with a date, the items this plan settled:
  - the Ledger names;
  - the claim signature;
  - suspended stock;
  - the upgrade checklist;
  - `JEWEL_GAS`;
  - the Plan 3 deployment list entries that are done.
- **Record who holds x/circuit permissions:** nobody but the gov authority.
- **Leave open, with the date seen:**
  - the pre-public items not settled here;
  - the backup being on the same machine (no off-server copy);
  - any defect found in Task 12, Step 4.

- [ ] **Step 2: README**

`clients/README.md`: under the loyalty section, add "Public: https://loyalty.yamaletech.com (devnet-2). Deploy with `deploy/yamale-us/loyalty/install-loyalty.sh` (`--app` for the app alone); the office admits with `jewel-admit` on yamale-us."

- [ ] **Step 3: Commit, and update memory**

```bash
git add docs/scope/gaps.md clients/README.md
git commit -m "gaps: x/jewel live on devnet-2; loyalty.yamaletech.com deployed"
```

Update the `jewel-module-design` memory:
- Plan 3 done;
- the upgrade height;
- the office;
- where the office keys live (`/opt/yamale/jewel-office` on yamale-us, root 700, backed up nightly);
- the `jewel-admit` command.

Update `chain-on-yamale-us` with the new units (`yamale-loyalty`, the backup timer) and port 8082.

---

## Self-review (done while writing)

- **Decisions:** 1 → Task 1, 2 → Task 2, 3 → Task 3, 4 → Task 11.
- **Upgrade checklist from gaps.md:**
  - store added (Task 4);
  - `fromVM["jewel"]` not pre-set (the Task 4 test asserts the default params, which only InitGenesis writes);
  - circuit line (in the handler, Task 4; and in `init-devnet.sh`);
  - office by `MsgUpdateParams` (Task 11);
  - x/nft store present (measured: x/nft is in the module versions);
  - circuit permissions recorded (Task 13).
- **Plan 3 deployment list:**
  - loyalty-api and app: Tasks 8, 12;
  - Caddy routes: Tasks 7, 9;
  - rate limits: Task 7;
  - sponsor key funded, with a balance alert: Tasks 6, 8, 9;
  - operator token: Task 8;
  - nightly backup: Task 8 (same machine, recorded);
  - SPA fallback: Task 7;
  - `VITE_*` settings: Task 8;
  - health check: Task 9;
  - production flags: Task 8;
  - CSP and Referrer-Policy: the gate's `yamale-headers.conf`, Task 7, checked in Task 12;
  - RTL seen in a browser: Task 12;
  - pagination, proof forwarding, staff codes and the threat model stay open (noted above).
- **Signatures used across tasks:**
  - `ClaimSignBytes(chainID, jewelID, offerExpiresAt, claimer)`: Go, Task 1;
  - `claimSignBytes(chainId, jewelId, offerExpiresAt, claimer)` and `claimJewelOffer(claimer, jewelId, offerExpiresAt, key, chainId)`: TS, Task 1; used in Task 1, Step 7;
  - `jewelsign claim <priv> <chain> <jewel> <expires> <claimer>`: Task 1, used by `seed-jewel.sh`;
  - `stockOfferBlocked(offer, business, active)`: Task 2;
  - `OFFICE_VOTES` and `FUNDER`: Task 5, used by `jewel-admit` in Task 8;
  - `await-halt-then-swap.sh <name>`: Task 5, used in Task 10;
  - `/v1/health/sponsor`: Task 6, routed in Task 7 (404 outside), probed in Task 9.
