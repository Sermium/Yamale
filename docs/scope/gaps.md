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

**There is no such thing today. Every parameter is an ordinary mutable gov
param.** The proof is already on the record: `recovery_destination` is a plain
`string` in `blockchain.enforcement.v1.Params`, and it was found **empty on the
running chain**. A seizure carried by two thirds of validators would have had
nowhere to send the funds. Nobody noticed until a console printed it.

Needed:

- a genesis-fixed invariant set — concentration caps, `threshold_bps`,
  `recovery_destination`, the delays;
- `MsgUpdateParams` refusing to change any of them;
- ~~validation at `InitGenesis` that refuses a genesis leaving one unset~~ —
  done for x/enforcement: `Params.Validate()` now refuses an empty or malformed
  `recovery_destination` as a bech32 address, and `InitGenesis` validates the
  whole genesis state before writing any of it, so a chain cannot start in that
  condition. The other modules still do not validate at `InitGenesis`;
- amendment only by supermajority *and* delay, if at all.

This is the same irreversibility class as the genesis-counter and
uniqueness-at-import problems already solved in x/land: cheap now, unfixable
after a deployment holds real value.

### 2. Concentration caps

**`blockchain.validatorgov.v1.Params` is an empty message.** There is nowhere to
put a cap, and no entity, beneficial-owner or jurisdiction attribute on a
validator application, so no cap could be computed even if there were.

Needed: those attributes on the application record; the three caps as invariants
(§1); and — the part that matters — **enforcement in an EndBlocker that can
demote, every epoch**, not an ante gate at admission. Admission-time-only
enforcement is precisely the hole x/land has: the cross-office quorum protects
the moment of transfer and leaves the standing state unguarded. A cap checked
only when a validator joins is not a cap.

Whatever divides in that epoch check must guard its divisor. A zero from genesis
in a Begin/EndBlocker halts the chain.

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

1. Payment confidentiality (schema) and the constitutional layer — both are
   impossible to retrofit.
2. Concentration caps with epoch enforcement; the two profile build tags.
3. Finish x/land: autocli, client, and the x/tokenisation enforcement half.
4. Replace the signing service; implement rotation; test the enforcement paths.
5. Everything in §5, if and only if open decision four says vendor.
