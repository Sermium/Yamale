# Freezing a scammer, and getting the money back

Yamale can stop a stolen balance from moving, and — if the validators agree —
take it and hand it to the foundation. This page is how that works, what it
costs the people it is aimed at, and what stops it being aimed at you.

**You need:** a running chain ([local devnet](local-devnet.md) is enough), and
for the acting parts, a bonded validator's key.

**You will end with:** an account frozen mid-theft, a case voted through, and
the funds in the recovery destination — plus the same account released again
when a case fails.

---

## The shape of it

A theft is drained in minutes. A vote takes hours. Any design where the money
only stops once everyone has agreed is a design that arrives after the money has
gone, so the two are deliberately split apart:

| | Who | How fast | What it does |
|---|---|---|---|
| **Freeze** | one bonded validator | the same block | the account cannot send anything |
| **Emergency freeze** | the founders' group | the same block | the same, without waiting for a validator |
| **Release** | the founders' group | the same block | lifts any freeze, whoever imposed it |
| **Seizure** | two thirds of bonded power | a vote | the balance moves to the recovery destination |

That asymmetry is the whole design. One validator, acting alone and under their
own name, can stop transfers for a day. Nothing else. The freeze **expires by
itself** if the set does not confirm it, the case is public from the block it
was opened in, and no case can ever send funds anywhere except the one address
governance has set.

## 1. Open a case

```bash
blockchaind tx enforcement open-case <your-account> <target> seize \
  --reason "drained the YML/EUR pool at block 812,400 and moved the proceeds here" \
  --evidence-uri "https://example.org/reports/2026-08-11.pdf" \
  --evidence-hash "$(sha256sum report.pdf | cut -d' ' -f1)" \
  --from validator --chain-id yamale-testnet-1
```

`<your-account>` is the validator's own account — the key it signs with. The
chain works out the operator address and records **that** on the case, so the
accusation carries a name people recognise.

The target is frozen in this block. Check it from the other side:

```bash
blockchaind query enforcement freeze-status <target>
```

That answers *why*, not just *whether* — it returns the case, its grounds and
its evidence alongside the freeze. An account that is frozen without being told
why is the failure this module has to avoid most, so the answer is one query.

A transfer from a frozen account now fails with:

```
account yml1... is frozen by enforcement case 1; query
enforcement/v1/freeze/yml1... for the case and its grounds
```

### Freeze or seize

`freeze` stops the account and takes nothing — the funds stay where they are and
stay the account's. Use it when something is wrong and you do not yet know what.

`seize` freezes and, if the case passes, moves the balance. It needs evidence:
a URI and the SHA-256 of what it pointed at. Freezing on suspicion is
defensible; taking somebody's assets with nothing on the record is not.

## 2. Vote

```bash
blockchaind query enforcement open-cases
blockchaind tx enforcement vote-case <your-account> 1 yes --from validator
```

Options are `yes`, `no` and `abstain`. Abstain counts towards nothing; it is
there so a validator can put on the record that they saw the case and declined
to judge it, which is a different statement from not voting.

Votes are weighed by consensus power, measured against the power bonded **when
the case was opened**. Power leaving the set mid-vote cannot lower the bar for a
decision already in progress.

The case resolves the moment the answer can no longer change — in either
direction. Enough yes power and it passes immediately; enough no power that the
threshold is unreachable and it is rejected immediately, and the account is
released in that same block.

> **At three validators, two thirds means all three.** With equal power, two of
> three carry 66.6% and the threshold is 66.67%. That is the same arithmetic
> that makes this chain stop producing blocks when one of three nodes is down,
> and it means a single validator being offline blocks enforcement entirely.
> Worth knowing before the set grows.

## 3. What passing does

For a `freeze` case: the freeze stops expiring. It is the set's decision now,
not one validator's suspicion, and lifting it takes another decision.

For a `seize` case, in the block it passes:

1. every delegation the target holds is unbonded;
2. everything spendable in the account is sent to the recovery destination;
3. the account **stays frozen**.

```bash
blockchaind query enforcement case 1
blockchaind query enforcement recovered
```

## 4. Sweeping, and why a seizure is not over when it passes

Staked funds do not come back instantly. Unbonding them starts a clock the
module does not shorten — nothing here overrides the chain's security
assumptions — so the stake arrives weeks later, in an account that is still
frozen.

Anyone can collect it:

```bash
blockchaind tx enforcement sweep <any-account> 1 --from anyone
```

Permissionless because the destination is fixed by the parameters: there is
nothing to gain by being the one who calls it, and nothing to lose by letting
somebody else. Repeatable because funds keep arriving — matured unbonding, a
late payment from an accomplice. The response says what it actually collected,
which may be nothing:

```json
{ "collected": [], "complete": false }
```

`complete` means nothing liquid, nothing staked, nothing unbonding. Even then
the case can be swept again if something new turns up; a case marked finished is
not sealed shut, because the account is still frozen and funds arriving into it
would otherwise be stuck forever.

## The founders' emergency path

The validator process has two failure modes a real network cannot simply wait
out. A theft at three in the morning with nobody awake to open a case. And a
freeze that was plainly wrong, sitting on a business's payroll for the twelve
hours until the voting period ends — "wait a day, it expires by itself" is not
an answer anyone can give a customer.

So one address, `emergency_authority`, can do two things immediately: **freeze**
and **release**. In practice it is a founders' M-of-N group policy from
`x/group`, so those actions still take several people.

It cannot seize, and there is no message that would let it. That is the whole
shape of the power:

> Stopping money is recoverable — release the account and nothing was lost but
> time. Taking it is not. So taking it stays with the validator supermajority,
> whoever is asking.

A freeze the founders impose is **provisional exactly like a validator's**. It
opens a real case, it goes into the same voting queue, it lapses if nobody
confirms it, and the validators can refuse it — which releases the account in
that same block. The founders are faster than the set; they are not above it.

Both messages are signed by the group policy, so they go through an `x/group`
proposal rather than a key at a terminal:

```json
{
  "@type": "/blockchain.enforcement.v1.MsgEmergencyFreeze",
  "authority": "<founders group policy address>",
  "target": "yml1...",
  "reason": "exchange reported the deposit as stolen at 03:14 UTC",
  "evidence_uri": "https://…/report.pdf",
  "evidence_hash": "<sha256>"
}
```

```bash
blockchaind tx group submit-proposal proposal.json --from founder-1
blockchaind tx group vote <proposal-id> <founder> VOTE_OPTION_YES --from founder-2
blockchaind tx group exec <proposal-id> --from founder-1
```

Releasing is the same shape:

```json
{
  "@type": "/blockchain.enforcement.v1.MsgEmergencyRelease",
  "authority": "<founders group policy address>",
  "case_id": "4",
  "reason": "the counterparty was the exchange's own settlement account"
}
```

Release works on a case that is still being voted on and on one that already
passed. What it does **not** do is undo a seizure: if the funds have moved,
releasing gives the account back and nothing else. Those funds are the recovery
destination's to return.

Everything the emergency path does is marked as such — on the case, in the
events, and with its own badge in the explorer. "A validator saw this" and "the
founders acted directly" are different facts about how the chain is being run,
and only one of them is the normal case.

If `emergency_authority` is unset there is no emergency path at all, which is
the default. An unset authority means nobody, never anybody.

## When it goes wrong

Three ways out, in increasing order of how much the chain has already done:

**Withdraw.** The validator who opened a case can take it back while it is still
being voted on, which releases the account immediately. The person trusted to
impose a freeze alone is trusted to admit alone that they were wrong.

```bash
blockchaind tx enforcement withdraw-case <your-account> 1 --from validator
```

**Let it lapse.** A case nobody votes on expires at the end of its voting
period, and the account is released. The status is `EXPIRED`, deliberately not
`REJECTED`: silence is not a finding, and the record should not claim it was.

**Release or appeal.** The founders' group can lift any freeze immediately (see
above). Governance can also overturn a case that already passed, through a
proposal carrying `MsgReverseCase`. Either way the freeze is lifted and the
reversal is recorded beside the original accusation.

It does **not** return what was already seized. Those funds are in the recovery
destination's hands, and only they can send them back. The module could pretend
otherwise, and the state machine would then be telling a comforting lie.

## What cannot be frozen

Module accounts. The bonded pool, the fee collector, the treasury and payments
custody accounts — freezing one would stop staking, distribution or every
payment on the chain for everybody, and there is nobody behind such an address
to accuse. The attempt is refused outright rather than left to whoever happens
to be voting.

## Where the freeze is enforced

On the bank keeper, as a send restriction — not in the ante chain.

An ante decorator only ever sees messages that arrive as transactions. A freeze
enforced there would be walked around by authz, by interchain accounts, by a
treasury spend, by a swap against a pool. Every one of those ends in a bank
transfer, and that is the one place all of them pass through.

There is exactly one exception, and it is one address wide: a frozen account can
still send **to the recovery destination**, because a seizure has to be able to
move what it seized. Since that address is fixed by governance, the exception
cannot be used to send anywhere else.

The freeze stops sending, not receiving. Refusing incoming funds would bounce
payments from people who have done nothing wrong, and would scatter the trail
rather than preserve it.

## The parameters

| Parameter | Default | What it decides |
|---|---|---|
| `voting_period_blocks` | 8,640 (~12h) | how long validators have |
| `provisional_freeze_blocks` | 17,280 (~24h) | how long one validator's freeze lasts unconfirmed |
| `threshold_bps` | 6,667 | the share of bonded power a case needs |
| `recovery_destination` | *(required, no default)* | the only address seized funds can go to |
| `seize_requires_evidence` | true | whether a seizure can be opened without a record |
| `emergency_authority` | *(unset)* | the founders' group, which can freeze and release at once |

`recovery_destination` is the foundation account: the trust body administering
the chain, which holds what is recovered so it can be restituted to the people
it was taken from.

It has no default and it is not optional. No address compiled into the binary is
anybody's foundation, so genesis has to name one — and a genesis that does not
is refused at `InitGenesis`, which means the chain does not start rather than
starting wrong. That is deliberate, and it is a change: it used to be empty by
default, on the argument that a chain which can freeze but not seize is a safe
place to launch from. The devnet then ran for weeks in exactly that state, and
what it actually bought was a chain where two thirds of the validator set could
pass a seizure that had nowhere to send what it took. Nobody noticed until a
console printed the parameter.

Changing it afterwards is a governance proposal, visible to everyone, like any
other parameter.

`threshold_bps` cannot be set at or below 5,000. There is no configuration of
this module worth having in which a minority of the validator set can take
somebody's assets, so that is refused at the parameter level rather than left to
a proposal that happens to read reasonably.

## What this costs

Be clear about what has been built here. The validator set can stop any account
on this chain from spending, within one block, on one member's say-so; and with
two thirds it can take what that account holds. That is a real power over other
people's money, and no amount of process makes it not one.

The founders' group can stop any account on its own signature too, though it
can never take anything and any account it stops is released the moment the
validators say so.

What is offered against all that is not a promise but a shape: every case is
public from the block it opens, carries its author's name and its grounds,
expires unless confirmed, can only ever move funds to one governance-set
address, and leaves a record — including of the cases that were rejected,
withdrawn, released or overturned — that is never deleted.

Whether that trade is the right one is a question for the people running the
chain, not for the code. What the code can do is make sure the trade is visible.
