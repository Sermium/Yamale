# Yamale

A sovereign settlement chain for institutions that have to answer to somebody.

Yamale is a permissioned Cosmos SDK network for national payments, land title and
real-world-asset vehicles. It is built for finance ministries, central banks and
registries — which means the interesting parts are not the features but the
refusals, and most of the design exists to constrain whoever operates it.

**Status: pre-testnet.** Not audited. Running on a two-validator development
network. Do not put money on it.

---

## What it refuses

Every chain lists what it can do. This one is easier to judge by what it will
not do, because that is where the design decisions are.

| | |
|---|---|
| **Payments** | An institution no national authority approved. A customer that institution never registered. |
| **Land** | A second title over the same survey. A transfer any stranger has objected to — no standing required, because the people this protects usually have none to prove. |
| **Vehicles** | Diluting a shareholder: the supply is fixed at issuance and there is no second issuance. A sale price attested by accounts the seller appointed. |
| **Seizure** | Anything less than two thirds of bonded validator power, to a destination fixed in the constitution and nowhere else. A freeze needs one authority and expires by itself. |
| **Consumer accounts** | The operator moving a customer's money. The key exists in three shares and any two sign, so this is arithmetic rather than policy. |
| **Governance** | Editing any of thirteen invariants. No proposal can reach them. |
| **Settlement** | A netted obligation whose sender has not already prefunded the reserve to cover it. Adequacy is checked when the obligation is submitted, not when the window closes. |

## What it is built on

Cosmos SDK v0.53 and CometBFT v0.38 — proof-of-stake BFT with instant finality,
so settlement is final in about five seconds with nothing probabilistic to wait
through. Validator admission is a governance vote rather than an auction; that
single deviation from stock Cosmos is what makes the rest possible.

IBC is compiled out by default and enabled with `-tags ibc`. A second profile,
`-tags settlement`, removes the native token's issuance and five modules for a
deployment that wants pure settlement with no token economics.

## Modules

`alias` identity and the jurisdictional perimeter · `paymsg` ISO 20022 payments ·
`netting` tiered settlement · `land` title registry · `tokenisation` closed-end
RWA vehicles · `treasury` M-of-N custody · `enforcement` freezing and seizure ·
`constitution` the invariants nothing can edit · `validatorgov` admission and
concentration caps · `oracle` rates and appointed valuers · `stablecoin` ·
`amm` · `custody` · `emission` · `builderfee`

## Building

```bash
go build ./cmd/blockchaind          # the node
go build -tags settlement ./...     # the settlement profile
go test ./...
```

The clients are a workspace under `clients/`:

```bash
cd clients && npm install && npm test --workspaces
```

Threshold-signing accounts have their own tool:

```bash
go run ./tools/mpc keygen --out ./account
go run ./tools/mpc pay --shares ./account --to yml1... --amount 1000000uyml
```

## Where to start reading

- **[docs/scope/gaps.md](docs/scope/gaps.md)** — what is built, what is merged
  but not running, what is designed and not built, and every known defect. It is
  checked against a running chain rather than a task list, and it is the most
  useful document here. Read it before believing anything else.
- **[docs/guides/](docs/guides/)** — one guide per module, hand-written, arguing
  the design rather than restating the API.
- **[docs/reference/](docs/reference/)** — generated from the protobuf
  definitions and CI-guarded against drift. Do not edit by hand; improve the
  proto comments instead.

## Honest limitations

- **No audit has been performed.** Two independent reviews with non-overlapping
  scopes gate any commercial deployment.
- **No account service.** Threshold key custody is designed, and the protocol and
  a custodian service exist; enrolment, recovery and second-factor are not built.
- **Two validators.** Neither holds two thirds, so losing one stops the chain.
- **No country enrolled**, so `x/paymsg` holds no approved participants and no
  institutional payment has ever been made on this network.
- **No licence yet.** See below.

## Licence

**None.** This repository carries no LICENSE file, which under default copyright
means no third party may use, modify or deploy this code. That is deliberate
only in the sense that the choice has not been made yet; it is not a position.
Until a licence is added, treat everything here as all rights reserved.
