# What governance can and cannot change

**You need:** nothing installed. This is about how this chain is bounded, and it
is worth reading before you rely on any of the numbers in the other guides.

**You get:** a clear line between the parameters a proposal can move and the
ones it cannot, and the procedure for the second kind.

---

## The problem this solves

Until recently, every number on this chain was an ordinary governance parameter.
That is the normal Cosmos arrangement and it sounds fine until you name the
numbers: the share of validators needed to seize somebody's assets, the single
address those assets may be sent to, and how long anyone gets to object before a
seizure completes.

A chain that can vote to lower its own seizure threshold does not have one. The
threshold is not a safeguard against the validator set — it is a safeguard the
validator set applies to itself, and a safeguard the same body can remove by the
same vote is a statement of current intent, not a rule.

This is not hypothetical here. `recovery_destination` — the only address seized
funds may go to — was **empty on the running devnet for weeks**. Two thirds of
the validator set could have passed a seizure that then had nowhere to send what
it took, and nobody noticed until a console happened to print the parameter.

## The two kinds of value

**Ordinary parameters** move by governance proposal, as before. Message size
limits, evidence requirements, rotation delays, the attestation interval — the
operational knobs. Nothing about how they work has changed.

**Invariants** live in `x/constitution`. They are set once, at genesis, and
`MsgUpdateParams` refuses to change them. There are thirteen:

| Invariant | What it decides |
|---|---|
| `max_entity_power_bps` | the share of voting power one admitted legal entity may hold |
| `max_beneficial_owner_power_bps` | the same, for the ultimate owner behind however many entities |
| `max_jurisdiction_power_bps` | the same, for everything answering to one national authority |
| `concentration_epoch_blocks` | how often those three are checked and breaches corrected |
| `min_active_validators` | the set size below which the check reports instead of acting |
| `enforcement_threshold_bps` | the share of power a seizure needs |
| `enforcement_recovery_destination` | the only address seized assets may go to |
| `enforcement_voting_period_blocks` | how long validators have to vote on a case |
| `enforcement_provisional_freeze_blocks` | how long one validator's freeze lasts unconfirmed |
| `amendment_delay_blocks` | how long a change to this table waits in public |
| `amendment_threshold_bps` | the share of power that must ratify such a change |
| `foundation_custodian_count` | how many people hold the account seizures are sent to |
| `foundation_signature_threshold` | how many of them must sign for it to act |

```bash
blockchaind query constitution invariants
```

The four `enforcement_*` values are also stored in `x/enforcement`'s own
parameters, because that is where they are read at speed. They cannot drift: a
`MsgUpdateParams` that disagrees with the constitution is refused, and so is a
genesis — the chain does not start.

The epoch length is in this table for the same reason as the ceilings it
governs. A ceiling enforced at an interval the same vote can lengthen to a
billion blocks has been repealed without appearing to have been touched.

## The foundation's shape

`enforcement_recovery_destination` names an `x/group` policy account, and the
last two rows say what that policy has to look like: exactly five custodians,
any three of whom can act. Together the three values describe the whole custody
arrangement rather than just its address. See
[the key ceremony](key-ceremony.md) for how it is built.

They are constitutional because the group is **its own admin** — which is what
stops any single key rewriting the membership, and also means the custodians
could otherwise vote to change their own rule. Three of them agreeing to make it
two of five would be an ordinary group proposal with a week's notice. Making it
an amendment puts it where it belongs: three weeks in public and a four-fifths
ratification, which is the right weight for "how many people it takes to move
seized property".

The count is exact, not a floor, and an ante decorator enforces it on every
route a change can arrive by — a direct message, a group proposal, the execution
of one submitted earlier, `x/authz`, or a governance proposal. A departing
custodian is replaced in the same message or the update is refused:

```
this update would leave the foundation group with 4 custodians; the
constitution fixes it at 5. A departing custodian is replaced in the same
message …
```

The reason it is exact is that the alternative drifts quietly. Five custodians
of whom three must sign is sixty per cent; four of whom three must sign is
seventy-five, so everyone who stayed holds more authority than the ceremony gave
them. At three it is unanimity and one unreachable custodian freezes the account
permanently, with the chain still sending seizures into it. Nobody decides on
that — it is reached by two reasonable decisions taken months apart.

`MsgLeaveGroup` is refused outright for this group, for the same reason:
leaving changes how much authority everybody else holds, so it is not a decision
one custodian takes alone.

## Why amendment is possible at all

Because "impossible" would be a lie.

A chain can be hard-forked, and an upgrade handler can rewrite any store. A
constitution with no amendment path does not become unamendable — it relocates
its amendments into a binary release, which is a change with less public notice
and fewer signatures than a proposal. So the path exists, on purpose, and it is
made slow and loud enough that taking it is a thing people find out about.

## Amending one

Two independent conditions, both required, weeks apart.

**1. A governance proposal** carrying `MsgProposeAmendment`. It supplies the
complete replacement table, not a delta — an amendment that meant to move one
ceiling cannot silently zero another by omitting it. It must state its grounds
in words.

When it passes, an amendment opens and its clock starts:

```bash
blockchaind query constitution list-amendment
```

The effective height is computed from the delay **in force when it opened**, not
from the delay being proposed. An amendment that shortens the delay does not
shorten its own, and no amendment may propose a delay below seven days — that
floor is compiled into the binary, because a floor that can itself be amended is
not a floor.

**2. A supermajority of validators ratifying it**, each signing with its own key:

```bash
blockchaind tx constitution ratify-amendment "$(blockchaind keys show carol -a)" 3 \
  --from carol --chain-id yamale-testnet-1 --fees 500uyml --yes
```

```bash
blockchaind query constitution ratifications 3
```

Ratification is measured against the voting power recorded **when the amendment
opened**, not against the power bonded when it comes due. A threshold measured
against the set that remains is passed by jailing everybody who would have
refused.

There is no un-ratifying. The protections are the delay and the threshold, not
the ability to run the vote backwards; a withdrawable ratification would let a
faction hold an amendment one vote short indefinitely.

**At the effective height** the chain enacts it if the ratified power has reached
the threshold, and lapses it if not. A lapsed amendment is kept on the record —
the fact that somebody tried to move a ceiling and the set declined is exactly
the history worth having.

Governance may withdraw a pending amendment with `MsgWithdrawAmendment` before
it takes effect.

The ratification threshold is deliberately higher than the seizure threshold.
Changing the rule must never be easier than using it, or the cheapest route past
a two-thirds seizure vote is a two-thirds vote to lower it.

## What this does not protect against

Everything above is a rule the software enforces on the validator set. It is not
a rule anyone enforces on a supermajority of validators willing to run a
different binary. What it buys is that doing so is a visible act — a fork, with
a different hash, that anyone comparing two nodes can see — rather than a
parameter change indistinguishable from housekeeping.

That is the whole of the claim, and it is worth being exact about it.

---

## Related

- [Freezing and recovering assets](enforcement.md) — the module three of these
  invariants bound.
- [Run a validator](validator.md) — the beneficial-ownership declaration and the
  concentration ceilings.
