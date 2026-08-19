# Gaps against the revised scope

What is unfinished, and what the revised scope newly requires. Every claim here
was checked against the tree rather than against the task list — task #48 was
marked complete while its artifact did not exist, so the task list is not
evidence of what is built.

---

## A. Carried over — started, not finished

**x/land has no user surface.** `x/land/module/` contains `depinject.go` and
`module.go` only: no `autocli.go`, so there are no CLI commands, and there is no
`clients/land`. The keeper, the genesis round-trip and 14 tests are done.

**x/tokenisation cannot enforce anything x/land decides.** `Fractionalise`
(`x/tokenisation/keeper/msg_server.go:127`) contains no reference to x/land. A
parcel carrying `RestrictionNoFractionalisation` can be fractionalised today,
and nothing checks a `MsgAuthoriseFractionalisation` or a `max_share_bps`. This
is a correctness hole, not a missing feature: the land module's restrictions are
currently decorative.

**The governance console's write paths are untested.** `/vote` has been
exercised end to end. `/freeze` and `/case` have not.

**Operator key rotation is documented but not implemented.**
`docs/guides/validator.md` describes `MsgRotateOperator`, the pause, the
challenge window and the veto-by-signing rule. None of it exists in code. Docs
ahead of code is worse than no docs.

**The signing service is a devnet crutch.** `opsd.py` signs with the node's own
keyring on behalf of a browser. It must be replaced by client-side signing
through `@yamale/connect`, after which `/api/ops/`, both htpasswd files and both
copies of `opsd.py` are deleted and the consoles move onto the tailnet.

~~**Devnet `voting_period` is 180s**, so proposals expire while they are being
debugged.~~ Fixed: the devnet setup and upgrade scripts now write 1800s (30
minutes), with a 900s expedited period. A chain already running still carries
the old value in its state — that takes a governance proposal, or the next
`upgrade.sh`.

Also open and untouched: #51 bridge strategy, #65 browser extension wallet.

---

## B. New — required by the revised scope, nothing exists yet

### 1. The constitutional layer

~~There is no such thing today.~~ Built. `x/constitution` holds eleven
genesis-fixed invariants: the three concentration ceilings, the epoch the
ceilings are enforced at, the floor below which enforcement reports instead of
acting, `x/enforcement`'s `threshold_bps`, `recovery_destination`,
`voting_period_blocks` and `provisional_freeze_blocks`, and the delay and
threshold an amendment to any of them must clear.

- **`MsgUpdateParams` refuses them.** `Params.AssertConstitutional` is checked on
  x/enforcement's update path and again at its `InitGenesis`, so the two copies
  of those four values cannot diverge rather than merely being unlikely to. The
  values stay in x/enforcement's own store because that is where they are read
  at speed; the constitution is the authority on what they may be.
- **`InitGenesis` refuses an unset invariant.** Every field, including the three
  ceilings and the epoch length. `DefaultGenesis` is deliberately *not*
  startable: it leaves `enforcement_recovery_destination` empty, on the same
  reasoning x/enforcement already used — no address compiled into a binary is
  the foundation on somebody else's network, and a default that was merely valid
  would satisfy the check while pointing every seizure at whoever generated it.
- **Amendment is possible, and slow.** Not "not at all", because that would be a
  lie: a chain can be hard-forked and an upgrade handler can rewrite any store,
  so a constitution with no amendment path relocates its amendments into a
  binary release — a change with *less* public notice than a proposal. The path
  is a governance proposal, then a three-week public delay, then a separate
  ratification by four fifths of the voting power recorded when the amendment
  opened. The effective height is computed from the delay in force when it
  opened, so an amendment cannot shorten its own; no amendment may set the delay
  below seven days, a floor compiled into the binary because a floor that can
  itself be amended is not one; and the ratification threshold must exceed the
  seizure threshold, because changing the rule must never be easier than using
  it.

**Where they live, and why a module.** A new module rather than a shared store,
because the dependency has to run one way and only one way: x/validatorgov and
x/enforcement consult x/constitution, and x/constitution depends on x/staking
and on nothing else in this repository. Anything it consulted back would be a
cycle depinject cannot wire — and, more to the point, a constitution that read a
value out of the module it constrains would not be constraining it. A shared
store would have given every module write access to the values by construction.

Open: a chain that already holds value adopts this through the `constitution`
upgrade, whose handler takes the four enforcement values in force and leaves the
ceilings at their shipped defaults, because nothing on a running chain implies
them. The first act of governance after that upgrade should be to amend them
deliberately.

### 2. Concentration caps

~~`blockchain.validatorgov.v1.Params` is an empty message.~~ Built, and the
shape changed with the owner's decision that validator power is equal seats set
by governance rather than stake.

- **The register.** `MsgApplyValidator` now carries a legal entity identifier, an
  ultimate beneficial owner identifier and an ISO 3166-1 alpha-2 jurisdiction,
  all three required and the country checked against the assigned list
  `x/alias/types/iso3166.go` owns. The declaration is copied onto the approval at
  approval and kept current by `MsgAttestOwnership`, which restates the whole
  declaration rather than bumping a date — an operator whose owner changed and
  who re-attests the old values has signed a false statement, which is a fact a
  supervisor can act on. A declaration older than the interval is published as
  `EventDeclarationStale` at every epoch and nothing else is done about it: the
  chain cannot verify a declaration, and turning an operator's inattention into
  a consensus event would be a worse failure than the one it prevents.
- **Enforcement demotes, every epoch.** `ConcentrationEndBlocker` runs on epoch
  boundaries, restores first and then demotes. A breach is corrected by jailing
  from the largest member of the group downward until the group is inside its
  ceiling; nothing is slashed and no delegation unbonds. A demotion is undone
  automatically at the first epoch the breach has cleared, and an ante decorator
  refuses `MsgUnjail` from a demoted validator — without it the ceiling would be
  one transaction per block away from being advisory.
- **Governance-set power is bound too.** `MsgSetValidatorPower` deliberately does
  *not* check the ceilings. A power granted above one is accepted and trimmed at
  the next epoch like any other breach, because a ceiling only tested where power
  is granted leaves growth, merger and nationalisation unguarded — and because
  refusing at the message would have made the test that proves the real
  mechanism impossible to write.
- **Divisors are guarded at the point of use.** The epoch modulus, the
  basis-point shares and the ceiling arithmetic all guard a zero where they are
  read, not only where they are validated.
- **A ceiling that cannot be satisfied is reported, not enforced.** Below
  `min_active_validators` the breach is emitted as an event and nothing is
  demoted. Three validators under three owners cannot all sit below a fifth of
  the power, and a check that kept demoting until the arithmetic worked would
  demote the chain into a halt.

Still open: a bonded validator with no approval record — a genesis validator
from the gentx ceremony — is counted in the denominator and belongs to no group,
so it can never be demoted. Genesis now refuses an `ApprovedValidator` without a
declaration, which is how a founding set is brought inside the ceilings; a chain
whose gentx validators were never added to the allowlist has validators no
ceiling reaches.

### 3. Payment confidentiality

Highest irreversibility, so first. `MsgPayment`
(`proto/blockchain/paymsg/v1/tx.proto:72-80`) carries `debtor`, `creditor`,
`denom`, `amount`, `purpose_code` and `remittance_information` — all plaintext,
all in the transaction, all in state forever. Under the revised positioning
(institutions, capital-control compliance, supervisors) this is the blocker.

The decision to take now is the shape, not the implementation: commitment
on-chain with detail off-chain, or an encrypted payload with viewing keys for
the supervisor. Either way the field numbers must be reserved before a pilot
writes a single payment.

### 4. Profiles

**The build has no concept of a profile.** One binary wires all thirteen modules
unconditionally in `app/app_config.go`. The settlement profile needs no native
token, so `x/emission` must compile out — it is referenced at six points in
`app_config.go`, including the BeginBlocker ordering. The registry profile needs
land without payments. And IBC is wired unconditionally in `app/ibc.go` with no
build tag anywhere in `app/`, so "excluded by build tag" is not currently
possible.

These two — emission compile-out and the IBC tag — are small and should land
**before audit scope is fixed**. A module absent from the binary is absent from
the review; a module merely disabled is not.

### 5. The obligations of being a distribution

None of this exists: LTS branches, a backport policy, upgrade tooling beyond the
single handler from #38, a deployment certification procedure, or a legal entity
able to sign a warranty. Whether any of it is needed depends on open decision
four (vendor versus reference implementation), which is why that decision gates
the rest.

### 6. The document itself

`docs/scope/README.md` names `revised-scope.md` as authoritative. That file does
not exist in the tree. Everything in `docs/guides/` still assumes a single chain
carrying every module.

---

## Order

1. ~~Payment confidentiality (schema) and the constitutional layer~~ — the
   constitutional layer is built; payment confidentiality is still open and is
   now the only remaining item that is impossible to retrofit.
2. ~~Concentration caps with epoch enforcement~~; the two profile build tags.
3. Finish x/land: autocli, client, and the x/tokenisation enforcement half.
4. Replace the signing service; implement rotation; test the enforcement paths.
5. Everything in §5, if and only if open decision four says vendor.
