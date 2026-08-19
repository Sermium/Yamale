# Run a validator

How to join the set of nodes that produce blocks on Yamale — which, unlike most
chains, requires a vote first.

**You need:** a Linux host, the `blockchaind` binary, and enough YML to
self-bond. Every command below was run against a real node.

**You will end with:** an approved candidacy and a bonded validator.

---

## Why there is an approval step

On a permissionless chain, anyone with enough stake validates. Yamale is
permissioned: the token is a payments instrument for institutions that have to
know who is processing their transactions, so the validator set is a decision
the chain makes rather than an auction it runs.

That decision lives in `x/validatorgov`. Its enforcement is not advisory — an
unapproved account's `create-validator` transaction is rejected before it is
even processed, by the ante handler.

**One exception:** the gate does not apply at genesis. That is what makes the
founding ceremony possible — the initial validators are established by the
genesis file itself, not by voting. See the
[deployment runbook](../../scripts/testnet/README.md).

## 1. Sync a node first

Before applying, run a full node and let it catch up. You are asking to be
trusted with block production; having demonstrably kept up is the argument.

```bash
blockchaind init <your-moniker> --chain-id yamale-testnet-1 --default-denom uyml
```

Replace `~/.blockchain/config/genesis.json` with the network's genesis, set
`persistent_peers` in `config.toml` to existing nodes, and start it. Confirm
`catching_up` has gone false:

```bash
blockchaind status
```

## 2. Apply

```bash
blockchaind tx validatorgov apply-validator "Carol Node" "carol@example.org" \
  "5493001KJTIIGC8Y1R12" "5493001KJTIIGC8Y1R12" "CH" \
  --from carol --chain-id yamale-testnet-1 --keyring-backend file --fees 500uyml --yes
```

A moniker and a way to reach you. Put something real in the second field — it is
how governance asks you questions before voting.

The last three are the declaration, and all three are required:

- **legal entity identifier** — your LEI if you have one, your national register
  number otherwise.
- **ultimate beneficial owner identifier** — whoever ultimately owns you. If
  nobody does, repeat your own identifier. "Nobody owns us" is a claim you sign,
  not a field you leave blank.
- **jurisdiction** — an ISO 3166-1 alpha-2 country code. It is checked against
  the assigned list; `QK` and `ZZ` are two letters and neither is a country.

They are required because the concentration ceilings are computed over declared
entities, owners and jurisdictions. A validator that declared none would belong
to no group and therefore sit under no ceiling, which is the one outcome a
beneficial-ownership register exists to prevent. They are also what the vote is
meant to be judging: a set asked to approve a candidate whose owner and
jurisdiction it cannot see is being asked to approve an address.

The chain cannot check any of this. What it can do is make it a signed statement
with a date on it, so that a false one is documented and actionable — which is
how the rest of the financial system handles beneficial ownership, and the most
an honest chain can offer.

```bash
blockchaind query validatorgov list-validator-application
```

```yaml
validator_application:
- candidate: yml1yu77rjnwumn4kr9jhezshhddujj2cugzy5ffwp
  status: pending
```

## 3. Governance decides

Somebody with a stake submits `MsgApproveValidator`, which only the governance
module account may sign:

```json
{
  "messages": [{
    "@type": "/blockchain.validatorgov.v1.MsgApproveValidator",
    "authority": "yml10d07y265gmmuvt4z0w9aw880jnsr700jrghjur",
    "candidate": "yml1yu77rjnwumn4kr9jhezshhddujj2cugzy5ffwp",
    "approve": true
  }],
  "metadata": "",
  "deposit": "10000000uyml",
  "title": "Admit Carol Node as a validator",
  "summary": "Carol Node has run a full node for three months without a missed block."
}
```

Get the `authority` value from `blockchaind query auth module-account gov` — it
is the gov module's address, not yours.

```bash
blockchaind tx gov submit-proposal proposal.json \
  --from alice --chain-id yamale-testnet-1 --keyring-backend file \
  --gas 400000 --fees 500uyml --yes
```

> `--gas 400000`: the 200,000 default is not enough for a proposal carrying a
> message, and it fails with `code: 11`, out of gas.

After the voting period:

```bash
blockchaind query validatorgov list-approved-validator
```

```yaml
approved_validator:
- approved: "true"
  candidate: yml1yu77rjnwumn4kr9jhezshhddujj2cugzy5ffwp
```

## 4. Create the validator

Now the standard staking transaction works. Write the description to a file:

```json
{
  "pubkey": {"@type":"/cosmos.crypto.ed25519.PubKey","key":"<from `blockchaind comet show-validator`>"},
  "amount": "100000000000uyml",
  "moniker": "Carol Node",
  "commission-rate": "0.10",
  "commission-max-rate": "0.20",
  "commission-max-change-rate": "0.01",
  "min-self-delegation": "1"
}
```

```bash
blockchaind tx staking create-validator validator.json \
  --from carol --chain-id yamale-testnet-1 --keyring-backend file --fees 500uyml --yes
```

Confirm you are bonded:

```bash
blockchaind query staking validators --output json | grep -c BOND_STATUS_BONDED
```

Or run the network's own health check, which also confirms every node agrees on
the same block:

```bash
./scripts/testnet/07-verify.sh http://<peer1>:26657 http://<peer2>:26657
```

## 5. Feed prices

**This is not optional in practice.** `x/oracle` needs a majority of *stake* to
report, so on a small validator set a couple of non-participating validators
leave the chain with no prices at all — measured on a three-validator network,
one reporter of three agrees nothing, two agree a rate.

Delegate to a hot key so the operator key stays offline, then submit once per
vote period:

```bash
blockchaind tx oracle delegate-feeder <your-valoper-address> <feeder-address> \
  --from carol --chain-id yamale-testnet-1 --keyring-backend file --fees 500uyml --yes
```

See [Price feeds and asset valuations](oracle.md) for the whole picture.

---

## Things worth knowing

**Keep the operator key offline.** It is the key that can move your stake and
change your commission. The feeder delegation above exists so that voting on
prices — the thing you must do every minute — does not require it.

**Losing your node stops more than your rewards.** With three equal validators
the network tolerates no failures; a fourth would let it survive one. Run under
systemd with `Restart=always`, as the [runbook](../../scripts/testnet/README.md)
does.

**Your reliability is on the chain.** `blockchaind query oracle misses` shows
how many vote periods each validator failed to report in. Nothing is slashed for
it — the record is the consequence.

## Peering privately over Tailscale

A validator does not need a public P2P port. The Yamale devnet peers its nodes
over a [Tailscale](https://tailscale.com) tailnet: every node joins the same
tailnet and sets the others as `persistent_peers` at their `100.x` addresses, so
consensus traffic never touches the public internet and no `26656` is exposed on
any host. This is how the second validator (a Raspberry Pi behind home NAT)
joins the cloud node — the home connection accepts no inbound, but the tailnet
does not need it to.

The steps, once the new node is synced as a full node (matching binary, same
genesis — a mismatched binary forks at the first block):

1. `apply-validator` from the node's operator key (an application).
2. A governance proposal carrying `MsgApproveValidator` for that candidate;
   it passes after the voting period and the ante gate then admits the operator.
3. `staking create-validator` with the node's consensus pubkey and a
   self-delegation.

Keep a joining validator's stake a **minority** of the set while you test it: if
the existing validators still hold more than two-thirds, the chain keeps
producing blocks even when the new node drops — which, for a node on a home
connection, it will.

**Full reference:** [x/validatorgov](../reference/validatorgov.md) — every
message, query, parameter and error code, generated from the source.

## Concentration ceilings

Your power is not only yours to keep. Three ceilings bound how much of the
validator set any one entity, owner or country may carry, and they are checked
at **every epoch** — not at admission.

```bash
blockchaind query validatorgov concentration
```

That is deliberate, and it is the part most easily got wrong. Admission is an
event; concentration is a state. Nothing applies to vote on when a state-owned
bank acquires a participant, when two members merge, when an operator is
nationalised, or when a proposal grants somebody more seats. Power concentrates
and there is no message for the chain to refuse. A ceiling that were only tested
when somebody joined would be a ceiling on joining.

**If your group goes over one**, the epoch check jails validators from that
group, largest first, until it is back inside. Nothing is slashed, no delegation
unbonds, and no vote is taken:

```bash
blockchaind query validatorgov list-demotion
```

**It is undone automatically.** At the first epoch where the breach has cleared,
the demotion record goes and the validator is unjailed. You do not have to ask
the validators who gained from your demotion for permission to come back — which
is the difference between a ceiling and an expulsion. You cannot `unjail`
yourself out of one, though: that path is closed while a demotion is in force,
because it would otherwise be a one-transaction way to ignore the ceiling
entirely.

**A ceiling that cannot be satisfied is reported, not enforced.** Three
validators under three owners cannot all sit below a fifth of the power, and a
check that kept demoting until the arithmetic worked would demote the chain into
a halt. Below `min_active_validators` the breach is published as an event every
epoch and nothing is done about it. Enforcement must never be the thing that
stops block production.

The ceilings themselves are invariants — see
[what governance can and cannot change](constitution.md).

## Re-attesting your declaration

The declaration you applied with goes stale. Re-sign for it:

```bash
blockchaind tx validatorgov attest-ownership \
  "5493001KJTIIGC8Y1R12" "5493001KJTIIGC8Y1R12" "CH" \
  --from carol --chain-id yamale-testnet-1 --keyring-backend file --fees 500uyml --yes
```

You restate the whole declaration, not just the date. That is the point: if your
owner has changed and you re-attest the old values, you have put a false
statement on the record under your own key, which is a fact a supervisor can act
on. A bare heartbeat would have let you keep a stale declaration fresh without
ever repeating it.

Nothing happens to you if you let it lapse. The chain emits a
`EventDeclarationStale` for you at every epoch and leaves your seats alone —
turning inattention into a consensus event would be worse than the problem, and
a chain cannot verify a declaration anyway. What it can do is publish the date
and say so loudly, which is enough for admission governance to act on.

## Rotating a validator's operator key

The operator address is a validator's identity, so it cannot simply be edited.
But keys are lost — a phone drowns, a person leaves — and a design with no
answer for that is a design that loses validators permanently.

There are two paths, and the difference between them is whether the current key
can still sign.

### Planned rotation — the operator still holds the key

`MsgRotateOperator`, signed by the **current** operator, naming the new address.
Possession of the old key is itself the proof of legitimacy, so this needs
nothing more: a short delay (`planned_rotation_delay_blocks`, one voting period
— 48 hours by default) and it takes effect. Cheap, because an attacker who could
sign this already controls everything it protects.

```sh
blockchaind tx validatorgov rotate-operator <new-operator-address> --from <operator>
```

Changed your mind during the delay? `cancel-operator-rotation <id>`, signed by
the same operator.

**This is the path operators should be pushed towards.** Rotate deliberately
when a person leaves or a device is replaced, rather than waiting for a loss.

### Recovery — the key is gone

The dangerous one, because it is what an attacker claims. It is deliberately
slow and public:

1. **Anyone may propose it** with `MsgProposeOperatorRecovery`, stating the
   grounds — but it takes effect only with the **same quorum that admitted the
   validator**, the independent offices under `validatorgov`, voting
   `MsgApproveOperatorRecovery`. Recovering a validator should be exactly as
   hard as admitting one.

   A proposal on its own does *nothing*: no pause, no clock. If it did, one
   transaction from any address on the chain would be enough to stop any
   validator on it.
2. **The validator is paused from the moment the quorum approves.** It is
   jailed: removed from the active set, so it stops signing blocks and its
   power stops counting — but nothing is slashed and no delegation unbonds. A
   validator whose control is disputed should not be exercising control.
3. **A long challenge window** — `recovery_challenge_window_blocks`, 7 days by
   default — measured in days, not minutes. It runs from the approval, which is
   also when the pause starts.
4. **The old key can veto by existing.** If the current operator signs *anything
   at all* while the recovery is open, it is cancelled automatically and the
   pause lifts in the same block. That is the cleanest possible disproof of "the
   key is lost", it costs the real owner one transaction, and an attacker cannot
   produce it. The veto is live from the moment the recovery is *proposed*, not
   just from the approval, so an operator who notices early does not have to
   wait to object.

   It is enforced in the ante handler, which is the only place the chain looks
   at who signed independently of what they signed — a rule that required a
   particular message would be invisible to exactly the operator it protects.
   Two consequences worth knowing: a simulated transaction does not count (its
   signature was never verified), and neither does one that fails the fee or
   signature checks. It has to be a transaction the chain actually executes.

Anybody can see what is pending, which is the point:

```sh
blockchaind query validatorgov pending-operator-rotation <operator-address>
```

### What does not change

Rotation moves **who signs**, and nothing else:

- The consensus key is untouched — the node keeps producing blocks with the same
  identity, so no re-peering and no downtime beyond the pause.
- The stake and the delegations stay exactly where they are.
- **Delegators did not consent to a new operator**, so the challenge window is
  also their window: they can undelegate before it completes. The pending
  rotation is published as a chain event and answerable by query, which is what
  makes that possible — the window is public rather than a quiet administrative
  action.

### What rotation cannot do

The Cosmos SDK keys a validator record — and every delegation pointing at it —
by the operator address the validator was created with, and offers no operation
that re-keys one. Moving the address itself would mean unwinding and rebuilding
every delegation behind it, which is the cost this whole design exists to avoid.

So a completed rotation does two things instead:

- The `validatorgov` allowlist entry **moves** to the new address. The old
  address is no longer an approved operator and cannot create a validator.
- The new address is **granted authorisation** (via `x/authz`) over the messages
  that operate the validator: `MsgEditValidator`, `MsgUnjail`, and
  `MsgWithdrawValidatorCommission`.

Two limits follow, and neither is a bug to be fixed later:

- **The validator's on-chain operator address stays the old one.** Block
  explorers and `staking` queries will keep showing it. What changed is who can
  act through it.
- **The old key is not revoked.** The SDK has no way to stop an address signing
  for its own validator record. For a genuinely lost key this costs nothing,
  because a key that can still sign would have vetoed the recovery — that is
  what the veto rule is for. But rotation is not a remedy for a *stolen* key,
  and it should not be reached for as one: a thief who still holds the key can
  veto every recovery attempt indefinitely. That case is an
  [`x/enforcement`](../reference/enforcement.md) matter, not a rotation.
- **Self-delegated stake stays with the old address**, along with anything else
  that account holds. `MsgUndelegate` and `MsgSetWithdrawAddress` are
  deliberately *not* granted: each would let the incoming operator move the
  outgoing one's money, and a recovery that could move somebody's funds would be
  a false claim worth filing. An operator whose key is genuinely lost loses
  their self-delegation with it.

### Why not simply re-admit as a new validator

Because the stake and the delegations would have to be unwound and rebuilt, and
delegators would bear the unbonding period for somebody else's lost phone. The
whole point of rotation is to move the key without moving the money.
