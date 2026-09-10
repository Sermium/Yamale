# Replacing GG18 with DKLs23

Decided 2026-09-09: move consumer-account threshold signing from tss-lib's GG18
to DKLs23, using Silence Laboratories' audited implementation behind a sidecar.
This records what the evaluation found, including the two things that make it
harder than a library swap.

---

## Where GG18 actually sits

`github.com/bnb-chain/tss-lib/v2 v2.0.2` is a **GG18-family** scheme
(Gennaro–Goldfeder, 2018) whose multiplicative-to-additive step runs on
**Paillier encryption with range proofs**. Everything painful about enrolment
descends from that one choice: the Paillier key needs two safe primes, the
search costs 44–54 seconds per party, and `tools/custodian/preparams.go` — the
pool, its background worker, and the denial-of-service argument that justified
it — exists solely to hide that cost.

It is not state of the art. It is the widely deployed default. There are two
frontiers and they are different frontiers:

| Line | Protocol | Best at |
|---|---|---|
| Paillier | **CGGMP21** | UC proof, identifiable aborts, first-class proactive refresh |
| Oblivious transfer | **DKLs23** | No Paillier at all — millisecond keygen, three-round signing, far smaller cryptographic surface |

### The version question matters more than the protocol question

GG18's real history is not a weak paper, it is implementations that shipped
without the required zero-knowledge proofs, or without checking them, and lost
whole private keys: Alpha-Rays (2021) and TSSHOCK (2023) both hit tss-lib and a
long list of wallets and custodians built on it.

So this repository's exposure was checked rather than assumed, on 2026-09-09:

- `modproof` (Paillier–Blum modulus), `facproof` (no small factors) and
  `dlnproof` are all **present** in v2.0.2;
- all three are **verified** in `ecdsa/keygen/round_3.go`;
- the compatibility escape hatch exists — `Parameters.NoProofMod()` and
  `NoProofFac()` skip verification for "old parties" — but it is **opt-in**,
  defaults to false, and `mpc/` never sets it.

**We are on the post-hardening path, not a known-broken version.** That is what
makes this a considered upgrade rather than an incident.

---

## The field, and the bind in it

| Option | Language | Licence | Audited | Maturity |
|---|---|---|---|---|
| tss-lib v2.0.2 *(current)* | Go | MIT | Hardened, proofs enforced | Very widely deployed |
| **silence-laboratories/dkls23** | Rust | **Non-commercial (SLL)** | **Trail of Bits**, Feb 2024 + 2025 review | `sl-dkls23` 1.0.0-beta, "powers the Silent Shard SDK" |
| 0xCarbon/DKLs23 | Rust | Apache-2.0 / MIT | None | 16 stars; README points elsewhere for production |
| f3rmion/fy | Go | MIT | None | **2 stars, 1 fork** |
| taurusgroup/multi-party-sig | Go | Apache-2.0 | None — *"needs further testing and auditing to be production-ready"* | 391 stars, CGGMP21 |

**The only audited, production-grade implementation in the field is the one that
cannot be used commercially without a licence.** Everything permissively
licensed is unaudited, the Go CGGMP21 option says so itself, and the only Go
DKLs23 is a two-star single-author project. Putting consumer custody on that
would be worse than staying on hardened tss-lib, and hand-rolling DKLs23 would
be worse again: threshold ECDSA is the exact category where "it compiles and the
chain accepted the transaction" is not evidence of correctness, and the GG18
attack history is what that looks like when it goes wrong.

### The licence, which is a question for counsel

Silence Laboratories' SLL permits **non-commercial use** and names nonprofits,
educational institutions and **government bodies** among permitted users, while
excluding "internal business purposes" and resale. Yamale is a state-operated
payments network, so it may qualify — and it may not. A commercial licence is
purchasable from Silence Laboratories either way.

The full text is printed as the first step of `.github/workflows/dkls-spike.yml`
so that it can be read without a Rust toolchain or a checkout.

**Engineering proceeds in parallel with that question on purpose.** The
integration work is identical under either answer, and it is the long pole.

---

## Why this is not a library swap

### It is a language decision

Both viable DKLs23 implementations are Rust. The agreed shape is a **sidecar**:
a Rust process holds the share and does the cryptography, while the Go custodian
keeps authentication, sessions, the freeze and the second-factor rule and talks
to it over a local socket. No CGO, so the arm64/amd64 cross-builds stay as
simple as they are today, and it matches the pattern `tools/rpcgate` already
established.

That split is also the right one on the merits: **policy in Go, cryptography in
audited Rust.** The existing seam holds — `SigningParty`, `KeygenParty`,
`Reshare` and `mpc/cosmos` are the boundary, and nothing above `mpc/` changes.

### The library authenticates its own parties, and the current design does not

This is the finding that adds real work. `sl-dkls23`'s `setup` module carries
"protocol id, participant information, **cryptographic keys**, and tailored per
MPC protocol parameters", and messages travel over a generic `relay` on which
parties *publish* rather than address one another.

Today, tss-lib parties are identified **positionally** by role name, and
authentication is entirely the transport's job in `tools/custodian/auth.go`.
Under DKLs23 there is protocol-level party authentication, which is *better*
security and also new state nobody has designed:

- a long-term identity keypair per party;
- distribution of the identity public keys, so each party knows the others;
- an enrolment step that establishes them before key generation can start;
- a decision about whether identities are per-account or long-lived per-party.

**Corrected 2026-09-10, and this is the good kind of correction.** The above was
written from the setup module's description and treated as a prerequisite. The
spike showed it is not one: the library's own examples construct their setups
with `NoSigningKey` and `NoVerifyingKey`, so party authentication is generic and
**opting out is supported**.

That means the migration can start under exactly the trust model already in
place — transport-authenticated, with `auth.go` doing the deciding — which is
the same model tss-lib runs under today. Real per-party signing keys become a
later hardening step rather than a blocking design question, and a whole phase
of work moves off the critical path.

### What the migration does not have to do

**Nothing has to be migrated.** DKLs23 shares are not GG18 shares, and
converting between them would mean either an unaudited import/export path — the
library marks import/export and quorum-change as outside the Trail of Bits
audit — or reconstructing a whole key, which is the one thing this design
refuses.

It does not matter, because devnet-2 was decided on 2026-09-06 to be a
demonstrator that will be relaunched from a fresh genesis. Fresh genesis means
fresh accounts. **That decision paid for itself here.**

---

## The API, as first read (superseded below by what the spike confirmed)

`sl-dkls23` 1.0.0-beta, published 2025-10-13. Async on **tokio**, curve
arithmetic from **`k256`** — which is RustCrypto's secp256k1, so the same curve
this chain uses.

```rust
use sl_dkls23::{keygen, sign};
use sl_mpc_mate::coord::SimpleMessageRelay;
use k256::ecdsa::{RecoveryId, VerifyingKey};

let coord  = SimpleMessageRelay::new();
let share  = keygen::run(setup, rng.gen(), coord.connect()).await?;  // t=2, n=3
let pubkey = share.public_key().to_bytes();                          // SEC1 compressed

let (sig, recid) = sign::run(setup_dsg(&shares[0..2], "m"), rng.gen(), relay).await?;
```

Two properties that matter for Cosmos, both confirmed from the examples:

- the digest is **pre-hashed, 32 bytes** — exactly what a Cosmos `SignDoc` hash is;
- the signature is a `k256::ecdsa::Signature`, i.e. **r‖s** — exactly what the
  ante handler expects, and `mpc/cosmos` already turns a compressed SEC1 key
  into a `secp256k1.PubKey` and a `yml1` address.

Signing also takes a `chain_path` (`"m"` at the root), so one keyshare can
derive child keys. Useful, and a footgun: signing under the wrong path yields a
valid signature for the wrong address.

**The exact `setup` constructors could not be read from outside** —
`raw.githubusercontent.com` answers 503 and the examples' helper is not at the
path the README implies — which is why the first CI run compiled nothing of
ours and printed the library's own source instead. They are recorded, confirmed,
in the section below.

---

## Where the build happens, and why not on a host

Measured 2026-09-09:

| Host | RAM | Available | Swap | Cores | Also runs |
|---|---:|---:|---:|---:|---|
| VM | 954 MB | 258 MB | 4 GB NVMe | 2 | majority validator (57.18%), nginx, faucet, feeder, custodian |
| Pi | 1837 MB | 261 MB | 1 GB | 4 | second validator (42.82%), the public web server |

The NVMe swap makes an actual OOM kill unlikely. It does not make a **stalled
validator** unlikely, and that is the hazard: both validators hold more than a
third of the voting power, so losing either one drops the chain below the
two-thirds it needs and block production stops. A Rust crypto build saturating
two cores and thrashing I/O for minutes can make a node miss consensus timeouts
with nothing killed and nothing alarming.

Amendment 1 also needs both validators alive to ratify before **2026-09-14**.

So the build runs in **GitHub Actions**, which also gives reproducible artefacts
for two architectures rather than whatever was on somebody's laptop.

---
## What the spike established, 2026-09-09 to 2026-09-10

Run through `.github/workflows/dkls-spike.yml`. Every figure below was measured
or asserted in CI, not read from documentation.

### It is faster by two orders of magnitude on the thing that hurts

| | GG18, measured 2026-08-31 | DKLs23 |
|---|---:|---:|
| Key generation, 2 of 3 | 65,000 ms | **~1 s** |
| Reshare / key refresh | 81,000 ms | **~1 s** |
| Signing | — | **< 1 s** |
| Cold build of the library | — | 39 s |

`tools/custodian/preparams.go` exists solely to hide GG18's safe-prime search.
On these numbers **it stops needing to exist**, and with it the background
worker and the denial-of-service argument that justified an unauthenticated
endpoint costing minutes of CPU per call.

### A signature from it is usable on this chain

Asserted by `spike/dkls23/verify_test.go` against the chain's own code:

- the joint key parses as a secp256k1 point;
- it derives a `yml1` address through `mpc/cosmos` — the same path the device
  and the custodian use;
- the signature verifies over the digest;
- **S is normalised.** 32 signatures over one key and one digest, every one in
  the lower half of the curve order. By chance that is one in four billion, so
  it is the library's behaviour and not luck.

That last one was the point of the exercise. Both `(r, s)` and `(r, n-s)`
verify, Cosmos rejects the high form, and one low sample would have proved
nothing while looking exactly like proof. **The sidecar should still normalise
unconditionally** — what the count buys is knowing that line is belt and braces
rather than the only thing standing between this design and a chain refusing
half its transactions.

### The API, confirmed rather than inferred

```rust
use k256::elliptic_curve::group::GroupEncoding;   // to_bytes() comes from here

gen_keyshares(t: u8, n: u8) -> Vec<Arc<Keyshare>>
setup_dsg(shares: &[Arc<Keyshare>], chain_path: &str) -> Vec<sign::SetupMessage>
sign::run(setup, seed, relay).await -> Result<(k256::ecdsa::Signature, RecoveryId)>
keyshare.public_key().to_bytes()  // SEC1, AsRef<[u8]>
```

The digest is **pre-hashed, 32 bytes** — which is what a Cosmos `SignDoc` hash
is — and the signature is **r‖s**, which is what the ante handler expects.

### Three things learned the hard way, recorded so they are not relearned

- **`to_bytes()` is a `GroupEncoding` method** and the trait must be in scope or
  it does not resolve. Two probes testing `as_slice()` and `as_ref()` failed
  *identically*, and that is what said the problem was the method rather than
  the accessor: both cannot be wrong on a type that has either.
- **Cargo.toml declares its examples explicitly**, which turns autodiscovery
  off — it must, because `examples/common.rs` has no `main()`. A copied file is
  not a target until the manifest names it.
- **`continue-on-error` rewrites a step's conclusion to success.** Five probes
  built to diagnose a compile failure all reported success while the thing they
  were diagnosing had never compiled. Job conclusions are truthful where step
  conclusions are not, so each probe is now a matrix leg. A check that passes
  regardless of what it is checking is worse than no check.

---

## The plan

Steps 1 and 2 are done, and they answered the question: this is worth doing.

3. **The sidecar** — `tools/dklsd`, Rust. Holds one share, seals it at rest the
   way `tools/custodian/store.go` already does, and implements a relay that is
   a local pipe to Go. **Normalises S unconditionally** on the way out.
   Deliberately a process and not a library linked into Go: no CGO keeps the
   arm64 and amd64 cross-builds as simple as they are today, it matches the
   pattern `tools/rpcgate` established, and the split is right on the merits —
   policy in Go, cryptography in audited Rust.
4. **Its own setup construction.** `setup_dsg` hardcodes `.with_hash([1; 32])`,
   so the sidecar has to build `sign::SetupMessage` itself before it can sign a
   real digest. This is the first genuinely new Rust and the first thing to test
   against an actual `SignDoc` hash.
5. **The Go client** — `mpc/dkls`, on the existing `SigningParty` /
   `KeygenParty` / `Reshare` seam, so `tools/custodian` changes as little as
   possible and `mpc/cosmos` does not change at all.
6. **Rewire the custodian** — enrol, sign, recovery. Sessions, freeze, the
   second-factor rule and the password rules are policy and do not move.
   `preparams.go` is deleted rather than ported.
7. **The device half** — Rust to WebAssembly, replacing `mpc/wasm`. Likely an
   improvement in its own right: Go's WebAssembly output is heavy for a page a
   consumer loads on a phone.
8. **Re-prove on chain** — the `tools/mpc pay` equivalent, a real payment from a
   threshold account on a running chain, before the GG18 path is retired.

Party identity is **not** a prerequisite. `NoSigningKey` and `NoVerifyingKey`
are supported, so the migration can start under the trust model already in
place — transport-authenticated, with `tools/custodian/auth.go` doing the
deciding — and real per-party signing keys become a later hardening step rather
than a blocking design question.

The licence question runs in parallel. It gates deployment, not engineering.
