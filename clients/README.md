# Yamale clients

Everything user-facing, built on one shared abstraction of the chain.

```
clients/
├── sdk/        @yamale/chain — the abstraction layer
├── explorer/   @yamale/explorer — block explorer, simple and detailed views
├── wallet/     watching an account, and creating one
├── rwa/        the investor-facing app for tokenised vehicles
├── keys/       threshold accounts, running the protocol in a browser
├── markets/    the oracle, the AMM and the stablecoin registry
├── oversight/  enforcement cases and netting cycles
└── demo/       an eighteen-mechanism guided tour
```

All of the above are deployed under `https://pay.yamalelegal.com/<name>/`, along
with `app/`, `foundation/` and `land/`. Every one answered 200 when checked on
2026-08-31.

## The abstraction layer

`@yamale/chain` is the only thing that talks to a node. Every interface
consumes it rather than the REST API directly, so the judgement calls — what a
message means, how an amount reads, what an error tells somebody to do next —
live in one place instead of being re-derived, slightly differently, by each
frontend.

It covers four things:

- **Amounts.** `12500000uyml` → `12.5 YML`. All arithmetic in base units with
  BigInt, conversion only at the display boundary, and truncation rather than
  rounding so a shown balance is never larger than the real one.
- **Messages.** Every message type the chain can carry decodes to a sentence,
  plus a judgement (`everyday`) about whether a non-technical person needs to
  see it. Unknown types degrade to a readable row rather than raw JSON.
- **Errors.** Chain errors are written for node operators. Each one a user can
  hit is translated into what happened, why, and the single next action, with
  the raw text preserved behind a disclosure.
- **Reading.** A typed client over REST and RPC that returns already-decoded
  transactions with the untouched payload attached.

```bash
npm test --workspace @yamale/chain          # unit tests, no node required
node --experimental-strip-types sdk/src/smoke.ts   # against a running node
```

## The explorer

Two explorers over the same data, chosen by a switch in the header.

**Simple** answers *"did the money move, and where is mine?"* It shows only
everyday activity as sentences, with human amounts and relative times. No
hashes, gas, type URLs or JSON. Somebody who does not know what a validator is
should be able to find their payment here.

**Detailed** answers *"what exactly happened?"* Chain stats, block stream, every
message including the ones the simple view filters out, gas, fees, signatures
and the raw payload.

The same URL works in both. A person who found their payment in the simple view
can send that link to an engineer, who sees the full transaction at the same
address — that hand-off is when an explorer earns its keep. `?view=simple` or
`?view=expert` forces a mode for sharing; otherwise the last choice is
remembered, defaulting to simple.

```bash
npm run dev --workspace @yamale/explorer    # http://localhost:5173
```

The dev server proxies the node at `/api/rest` and `/api/rpc`, so development
does not depend on the node having CORS opened up. Point it elsewhere with
`YAMALE_REST` and `YAMALE_RPC`; in production set `VITE_REST_URL` and
`VITE_RPC_URL` at build time.

## Running a chain to develop against

```bash
go build -o blockchaind ./cmd/blockchaind
blockchaind init dev --chain-id yamale-devnet-2 --default-denom uyml
# fund accounts, gentx, collect-gentxs, then enable the API in app.toml:
#   [api] enable = true, minimum-gas-prices = "0uyml"
blockchaind start
```

Note that `config.toml` and `app.toml` must be written **without a UTF-8 BOM** —
the TOML parser rejects it, and some editors add one silently.

## Signing

The SDK can sign and broadcast, not only read. `ChainSigner` takes any CosmJS
`OfflineSigner` — a browser extension's, or a key held in a script — which is
what lets the signing path be tested against a running chain without a browser
in the loop. It waits for inclusion and reports what the transaction did *in the
block*, because a transaction the node accepts into its mempool can still fail
when the block runs it.

```ts
const signer = new ChainSigner(offlineSigner, { rpcUrl, chainId, gasPrice });
const result = await signer.submit([send(me, them, [{ denom: 'uyml', amount: '1250000' }])]);
result.succeeded; // what the chain did, not what the broadcast returned
```

`wallet.ts` connects Keplr or Leap and describes the chain to them — without
that description a wallet fails with "chain not found", because Yamale is not
one of the networks these extensions ship with.

### What can be signed

The message constructors cover the standard Cosmos actions — transfers,
staking, unstaking, claiming rewards, governance votes — and this chain's own:
payments, treasury creation, deposits and spends, lock claims, swaps, price
submissions, and the enforcement actions that freeze an account or sweep a
passed seizure.

The chain's own messages work because `registry.ts` registers the protobuf
types generated by `make proto-ts` into `src/generated/`. Signing a message
means encoding it as protobuf, and CosmJS encodes only types it holds an
encoder for; the default registry knows the standard Cosmos modules and nothing
else, which is why these actions used to be CLI-only.

`src/generated/` is generated and must not be hand-edited. It also carries the
`x/feegrant` messages, which are Cosmos's rather than this chain's: CosmJS omits
them from its default registry, and they are how an institution pays the network
fee for a customer who holds only their own currency. Without them the answer to
"must my customers hold YML?" is yes, at least from a browser. Anything else the
chain accepts can be signed without a constructor by passing a
`{ typeUrl, value }` pair straight to `submit` — the registry covers every
message in every module, including the authority-gated ones, so an interface
can assemble the contents of a governance proposal rather than telling somebody
to go and use the CLI.

### What has been verified

The signing path was exercised against a running devnet with a key held in the
test rather than in an extension: a transfer and a delegation both landed
(`code 0`, and the delegation is visible in `query staking delegations`), and a
vote on a proposal that does not exist was reported as the failure it is rather
than as success.

This chain's own messages were exercised the same way, on a devnet of their
own: a treasury was created, funded and spent out of — the recipient's balance
moved — and a swap against a pool that does not exist and a payment from an
account that banks with nobody both came back as failures with the chain's own
reasons attached (`Not found`, `Not an approved participant`) rather than as
silent successes. That last part matters more than it looks: a message that
encoded to nothing at all would also have failed, so the check is that the
chain rejected it *for the right reason*. The browser-extension plumbing on top of that — detecting
Keplr, suggesting the chain, requesting a signature — has not been exercised
here, because it needs an extension installed in a real browser.

## The wallet

`clients/wallet` — watching an account, and creating one.

Two screens kept deliberately apart. **Watching** needs no key and carries no
risk, so it is the default and works from a link: balances across every
currency the chain issues, whether the account is frozen and by which
enforcement case, and who is paying its network fees. **Creating** produces a
recovery phrase that must never leave the page, so it is a separate route with
its own warnings.

The usual "connect or create" landing page puts the dangerous action one click
from the harmless one, and shows a recovery phrase to people who came to check
a balance.

The phrase is generated in the browser by CosmJS and never transmitted —
verified by watching the network panel while generating one: nothing but local
module loads. It matches what `yamale-wallet` produces on the command line,
because both derive at `m/44'/118'/0'/0/0` with coin type 118.

The fee panel is not decoration. Fees are payable in YML, so an account holding
only naira with no sponsor cannot move a single unit, and a wallet showing a
healthy balance alone would hide the one fact that matters.

## Vehicles

`clients/rwa` — the investor-facing app for the real-world-asset vehicles
x/tokenisation issues.

It answers three questions in the order somebody deciding actually asks them.
*What is this?* — the collection it was issued under, the title, the underlying,
and where the underlying is registered land, the parcel with its restrictions,
its encumbrances and whether the registry's fractionalisation authorisation is
still live. *What do I own?* — a percentage of the asset rather than a token
balance, and an entitlement read from `Query/Entitlement`, which is the only
correct source: it is not derivable from a balance. *What could go wrong?*

The third is the one the app is really for. A closed-end vehicle cannot dilute
its holders — the supply is fixed at fractionalisation and there is no second
issuance — and the app says so, because it is unusual and it is true. What can
go wrong is the sale price: every holder is paid out against a number the
sponsor reports, and the only things between that number and a lie are the
collection's attestation threshold and its challenge window. So every collection
carries a graded protection, computed as its *worst* finding rather than as a
score. A ninety-day window does not offset the absence of any verification: a
figure nobody checks is not made true by being contestable for a month. A
collection with a threshold of zero and a window of zero grades as `none` and
leads with the sentence saying why.

Four actions, and they are treated differently on purpose. Claiming income
changes nothing about what is owned and asks once. Redeeming destroys shares —
the burn *is* the payment, with no later step — so it states what is destroyed
before what is received and requires the word typed. Disputing takes the
collection's bond out of the challenger's account in the same block, and that
bond appears nowhere in `MsgDisputeSale`, so it is stated as a figure above the
button. Each one shows the exact message the chain will receive, field by field,
with the consequences the signed bytes do not mention marked as such.

```bash
YAMALE_RPC=http://<node>:26657 \
YAMALE_REST=https://<host>/api/rest \
npm run dev --workspace @yamale/rwa          # http://localhost:5378/rwa/
```

The two upstreams are not interchangeable. `/api/rpc` carries every read of
x/tokenisation and x/land, because the proxy's REST allowlist does not cover
those module paths and a browser asking for a vehicle over REST gets a 401 — which
it renders as a login box for a password nobody has. The same queries answer
unauthenticated over ABCI, which is what x/land's own service comment intends:
*a citizen must be able to check a title before paying anybody, without an
account and without an official's permission.* `/api/rest` carries only the two
bank reads, a balance and a supply, which the allowlist does serve.

`fixtures.mjs` encodes the protobuf responses a populated chain would return.
It is a harness, not app code and not imported by it: the chain has held zero
collections throughout this work, and an app that only looks right once
populated is the failure this one was written against.
## Threshold accounts

`clients/keys` — the page that runs the protocol in front of you.

The claim it exists to make is one sentence: the operator cannot move a
customer's money. Every payments system says something like it and almost all of
them mean *we have a policy about that*. Here it is a statement about arithmetic
— the key exists in three shares and no one of them can produce a signature —
and the difference between those two things is the entire product. So the page
does not assert it. It runs the protocol, refuses in front of you, and reads the
consequences off a public chain.

`mpc/wasm` compiled to WebAssembly is what runs. The page is explicit about
which half of what you are watching is real:

- **Real:** the account on the chain, its transactions, their heights, and the
  fact that the second was signed by shares that did not exist when the first
  was — created in a password reset that did not move the address. Checkable
  against any block explorer.
- **Staged:** the live demonstration runs **both** parties inside this one page.
  In production they are a phone and a server that never meet, exchanging the
  messages you can see in the transcript. Running them together is what lets you
  watch the traffic; it is also precisely the arrangement the design forbids, so
  it uses a throwaway account that holds nothing and whose share files are
  published on purpose.

Being exact about that matters more than looking impressive. See
[docs/guides/mpc.md](../docs/guides/mpc.md).

## Markets

`clients/markets` — the oracle, the AMM and the stablecoin registry.

Three modules on this chain put a price on something and none of them had an
interface. `markets.js` is everything the console decides; `index.html` is
everything it draws — and the split is not tidiness. Every function in the first
file is a number somebody is about to act on, and a rendered page cannot be
checked.

Everything is `BigInt`. Reserves on this chain are already 3×10^10 base units and
a product of two reserves passes 2^53 immediately, so a swap quote computed in
doubles is wrong before anybody has done anything unusual.

The swap quote is the keeper's arithmetic byte for byte, including the rounding
direction, which is the protection: truncating leaves the fractional remainder in
the pool, and the algebraically equivalent subtraction form rounds the output
*up* and bleeds the pool one unit at a time. A test named `swap output never
exceeds the curve` is what stops somebody "simplifying" it.

The oracle panel shows how long a price has **left** rather than how long ago it
was set, because `max_rate_age_seconds` is not a display hint: past it, a value
too old to trust is not a value, and consumers must stop rather than proceed on
the last number anybody reported. It shows the same fact as a bar that empties,
because expiring is a thing that should look like it is happening.

**On this chain that panel is currently empty, and correctly so.** No rate has
ever been agreed on `yamale-devnet-2` — both validators have missed every oracle
window and no feeder is running. See
[gaps.md](../docs/scope/gaps.md#operational-loose-ends).

## Oversight

`clients/oversight` — enforcement cases and netting cycles.

Every function in `oversight.js` is a pure function of chain state, because the
console makes a claim — one validator can stop money in a block, and taking it
needs two thirds — and a claim like that has to be checkable. Where it
reimplements a keeper rule it names the file it is mirroring, and the test
asserts the mirror. A console that computes its own answer and quietly disagrees
with the chain is worse than one that says nothing: the moment it disagrees is
the moment somebody acts on the wrong number.

Three judgements worth naming:

- **The seizure threshold rounds up**, and it is measured against the power
  bonded when the case *opened* — not against turnout, which two validators pass
  on a quiet night, and not against the live set, which moves under the vote as
  validators unbond.
- **A case is shown as reachable or not**, because the keeper rejects eagerly the
  moment `total - no` falls below the bar. A progress bar alone would keep
  filling for a case that is already lost.
- **A freeze's clock is block height, never wall time**, and a permanent freeze
  is a zeroed field rather than a distant one.

It also detects the netting state this project has
[an open decision about](../docs/scope/gaps.md#open-decisions): `cycle_blocks = 0`
returns before closing anything, so the open window never settles. An end height
in the past is not the test — that is an ordinary window waiting for its
EndBlocker — so the disabled marker is what separates a stalled window from a
slow one.

## The guided tour

`clients/demo` — the page that makes the rest of this demonstrable in a room.

Eighteen mechanisms in five acts. Each one gets a sentence on what it does, the
thing it **refuses** to do, a query against the running chain that would come
back different if the refusal were decorative, and a link into the surface where
you can see it working. The refusal leads, because a finance ministry is not
asking whether this is clever — it is asking what the operator of it cannot do
to a citizen's money.

It is buildless: one HTML file, four ES modules, no npm install. It gets opened
in front of people, and a page that needs a toolchain to render a paragraph is a
page that will not be open at the moment it matters.

```bash
npm test --workspace @yamale/demo    # 71 tests, no node required
node clients/demo/serve.mjs          # http://localhost:5180, /api proxied
```

Two things it is arranged around.

**A proof that could not be read must say so.** Not zero, not blank, not a dash.
"Nought approved participants" and "the chain did not answer" are opposite facts
about whether an institution can move money. So a proof is a tagged value and
there is no path in `format.js` from an error to a numeral, and the catalogue is
tested against the four ways this deployment actually fails — a timeout, the
gateway's 401, the 503 a halted node returns, and an HTML error page where JSON
was expected. Every mechanism must come back unread from all four.

**The gateway rate-limits `/api/rpc/` and does not rate-limit `/api/rest/`.**
Measured: twelve concurrent RPC requests all return 503, and the limiter stays
tripped for sequential requests afterwards, while twelve concurrent REST
requests all succeed. This page feels it more than any other client, because the
four modules the REST allowlist denies — land, paymsg, netting, builderfee — can
only be read over ABCI. RPC therefore goes through a serialised queue at one
request every 500ms; REST keeps a plain concurrency gate. Without it the tour
opened, on a chain answering perfectly, showing six of its mechanisms as "cannot
reach the chain".
