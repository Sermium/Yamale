# Freezing a scammer, and getting the money back

Yamale can stop a stolen balance from moving, and — if the validators agree —
take it and hand it to the foundation. This page is how that works, what it
costs the people it is aimed at, and what stops it being aimed at you.

**You need:** a running chain ([local devnet](local-devnet.md) is enough), and
for the acting parts, a bonded validator's key — or an account holding
`ROLE_ENFORCEMENT_AUTHORITY` over the country the target is recorded in.

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
| **Freeze** | one bonded validator, or an enforcement authority for the target's country | the same block | the account cannot send anything |
| **Emergency freeze** | an enforcement authority for the target's country | the same block | the same, without opening an accusation as a validator |
| **Release** | an enforcement authority for the case target's country | the same block | lifts any freeze, whoever imposed it |
| **Seizure** | two thirds of bonded power | a vote, **then a delay** | the balance moves to the recovery destination |
| **Veto** | the ombudsman | the same block | stops any case that has not taken anything yet |

That asymmetry is the whole design. One signature, acting alone and under its own
name, can stop transfers for a day. Nothing else. The freeze **expires by
itself** if the set does not confirm it, the case is public from the block it
was opened in, and no case can ever send funds anywhere except the one address
governance has set.

Note which column the enforcement authority appears in and which it does not. It
can stop money in one block, inside one country; it has no vote on any case
unless it happens also to be a bonded validator, and there is no message that
lets it take anything.

Four things constrain a seizure specifically, because a seizure is the only
action here that cannot be undone:

1. it must name an **external legal instrument** — a court order, a regulator's
   direction, a warrant — by issuer, reference and hash;
2. it **waits** after the vote, for longer the more it would take;
3. an **ombudsman** appointed outside the validator set can stop it during that
   wait, and can never start one;
4. the chain will only carry out so much per **rolling window**, by value and by
   count.

## 1. Open a case

```bash
blockchaind tx enforcement open-case <your-account> <target> seize \
  --reason "drained the YML/EUR pool at block 812,400 and moved the proceeds here" \
  --evidence-uri "https://example.org/reports/2026-08-11.pdf" \
  --evidence-hash "$(sha256sum report.pdf | cut -d' ' -f1)" \
  --legal-instrument '{
    "issuing_authority": "High Court of Kenya at Nairobi",
    "reference": "HCCC/2026/0412",
    "kind": "LEGAL_INSTRUMENT_KIND_COURT_ORDER",
    "hash": "'"$(sha256sum order.pdf | cut -d' ' -f1)"'",
    "issued_at": "1754870400"
  }' \
  --from validator --chain-id yamale-testnet-1
```

`<your-account>` is the signer's own account. Two kinds of account may sign, and
the case records them differently on purpose:

- a **bonded validator** signs with its account key, and the chain works out the
  operator address and records **that** on the case, so the accusation carries a
  name people recognise;
- a holder of **`ROLE_ENFORCEMENT_AUTHORITY`** covering the target's country is
  recorded by its own address, because a group policy has no operator address and
  its own address is exactly what `query alias role-holders <CC>` is read against.

The validator path is tried first, so nothing a validator could do before has
changed shape — including the error it gets when it has unbonded, which stays
`ErrUnknownValidator` as long as it holds no grant. An account that is neither is
told both things it is not, because a signer told only "you hold no enforcement
grant" when they meant to sign as a validator would go looking for a proposal
they never needed.

Holding the role **somewhere** gets an account past that step and permits
nothing. What permits is the perimeter: the signer has to hold
`ROLE_ENFORCEMENT_AUTHORITY` covering the country the **target** is recorded in,
and that check runs on both kinds of opener. Being a bonded validator says you
are trusted to secure this chain; it does not say whose accounts you may stop.
This module used to answer only the first question, which is how a validator in
one jurisdiction could freeze an account in another.

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

`seize` freezes and, if the case passes and its delay expires, moves the
balance. Freezing on suspicion is defensible; taking somebody's assets with
nothing on the record is not.

### Evidence and legal authority are not the same thing

A seizure needs both, and they are separate fields because they answer separate
questions.

**Evidence** is why the chain believes the allegation — a URI and the SHA-256 of
what it pointed at. Governance can waive it with `seize_requires_evidence`.

**A legal instrument** is who, outside this chain, ordered that something be
done about it. It is required for every seizure, always, and there is no
parameter that turns it off. A requirement governance can vote away is a
default, and this one is meant to be a requirement — a validator set able to
remove its own need for a court order is a validator set that does not need one.

Note what the instrument does **not** have: a URI. That is the design rather
than an omission.

> A link is a document somebody controls. Whoever hosts it can change it, take
> it down, or never have had it, and a seizure whose only external anchor is a
> URL is anchored to whoever runs that web server.

What is stored instead is an identifier that names the instrument **in the
world** — the issuing body and its own reference number — plus a hash that pins
the content of what was served:

| Field | What it is for |
|---|---|
| `issuing_authority` | the body that issued it, as it names itself |
| `reference` | the issuer's own number for it, so it can be looked up in *their* register |
| `kind` | court order, regulatory direction, or warrant — a closed list, because "other" would let a case name its own paperwork |
| `hash` | SHA-256, lowercase hex, of the instrument as served |
| `issued_at` | when it was issued; an instrument dated after the current block is refused |

Somebody with the reference can find the instrument without this chain's help.
Somebody with the hash can prove the copy they were shown is the one the
validators voted on. Neither depends on anyone keeping a server up.

Folding this into the evidence fields would let a case satisfy its authority
requirement by attaching its own investigation report — the validator set
producing a document and then citing that document as its warrant to take
somebody's assets. That is the same body twice, not oversight.

A **freeze** needs no instrument. It takes nothing, and it has to be openable in
the minute a theft is noticed, which is not a minute in which anybody has a
court order. One may still be attached, and if it is, it is checked.

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
not one opener's suspicion, and lifting it takes another decision.

For a `seize` case, **nothing happens yet**. The case becomes `HELD`: agreed,
frozen, and waiting out a delay sized by what it would take.

```bash
blockchaind query enforcement held-cases
```

Nothing is moved and nothing is unbonded during the hold. That costs something —
a seizure against staked funds now waits the delay *and then* the unbonding
period, rather than running them together — and it buys the thing the delay is
for: a case stopped during the hold leaves its target exactly as they were,
still staked, still earning. Unbonding somebody's stake on an accusation that is
then withdrawn is a real harm, and the hold is where accusations get withdrawn.

## 4. The delay, and why it scales

Taking a market trader's float and taking a family's savings should not move at
the same speed. The cost of getting the second one wrong is not the same, and
the time somebody needs to notice and object is not the same either.

So the wait is a **schedule**, not a constant:

- `seizure_delay_blocks` is the floor every seizure waits, whatever its size;
- `seizure_delay_tiers` are steps, each a threshold amount and a longer wait.

A case takes the **longest** tier it matches — longest rather than first, so the
answer does not depend on the order governance happened to write the tiers in.
An ordering bug in a parameter list is invisible until the day it moves
somebody's savings at the speed meant for pocket change.

The amount a case is measured at is its **assessed value**: balance, stake and
unbonding together, recorded on the case.

> Counting only the liquid balance would let anybody holding their money in a
> validator have the largest seizure on the chain treated as the smallest, and
> the stake collected later anyway.

The schedule is a parameter and not a constant because there is no amount that
means "large" on every chain this is shipped to. It has **no default**: the
tiers are denominated, and no denomination compiled into the binary is anybody
else's currency. A genesis that does not state them does not start — the same
rule, for the same reason, as the recovery destination.

Sweeping cannot short-circuit the wait. `MsgSweep` is permissionless, so without
that refusal anyone could call it the moment a seizure passed and collect the
balance before the window in which it could still be stopped had opened.

## 5. What executing does

When the delay expires and the rolling cap allows it, in one block:

1. every delegation the target holds is unbonded;
2. everything spendable in the account is sent to the recovery destination;
3. the account **stays frozen**.

```bash
blockchaind query enforcement case 1
blockchaind query enforcement recovered
```

## 6. Sweeping, and why a seizure is not over when it executes

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

## The ombudsman

One address, `ombudsman`, appointed outside the validator set. It can stop a
case. It can do nothing else, ever.

```bash
blockchaind query enforcement held-cases
blockchaind tx enforcement veto <ombudsman> 3 \
  --reason "the order relied on names a different account; the reference does not match the register" \
  --from ombudsman
```

The asymmetry is the entire point:

> An office that can only refuse cannot be used to take anything from anybody.
> Appointing one adds a check without adding a power, which is the only kind of
> oversight worth having over a module like this one.

A veto works on a case still being voted on and on a seizure waiting out its
delay — both states in which nothing has moved. It does **not** work on a
seizure that has executed, and the refusal says so rather than pretending: a
veto cannot un-take money, and marking such a case vetoed would put a comforting
lie in the record.

### How the office is barred from initiating

Not by being asked nicely. There are exactly four ways a case can be opened or
moved forward, and the ombudsman is closed out of all of them:

| Path | Why it is closed |
|---|---|
| `OpenCase` | requires a bonded validator or a holder of `ROLE_ENFORCEMENT_AUTHORITY` in the target's country — **and** is refused to the ombudsman's address in the handler, on every call, even if that key *is* bonded or *does* hold the grant |
| `VoteCase` | requires a bonded validator, and holding the role confers no vote — **and** is refused to the ombudsman's address in the handler, including a vote of no, because an office that could vote at all would be inside the process it checks |
| `EmergencyFreeze` | requires `ROLE_ENFORCEMENT_AUTHORITY` over the target's country, and is refused to the ombudsman's address in the handler before the grant is read at all |
| `UpdateParams`, `ReverseCase` | governance authority only — and `UpdateParams` refuses an ombudsman that already holds `ROLE_ENFORCEMENT_AUTHORITY`, returning `ErrOmbudsmanCannotInitiate` |
| `Sweep` | permissionless, and refused to the ombudsman anyway |

`Params.Validate()` used to refuse parameters in which the ombudsman and the
emergency authority were the same address. That comparison is gone with the
field, and it could not have been kept: the authority is a grant in another
module now, so a method on a parameter struct cannot see it.

What replaces it is two checks in two places, which between them cover the two
orders the collision can arrive in. `UpdateParams` asks the registry before
writing an ombudsman, and refuses one that already holds the role — that catches
appointing an ombudsman over an existing grant, and it fails closed, so a chain
that cannot check this cannot appoint an ombudsman past it. The handlers refuse
the ombudsman outright wherever a case is opened or advanced — and that is the
check actually holding the property, because it catches the other order too: a
grant made to a sitting ombudsman *after* the parameters were written, which the
parameters could never have seen even when the field existed.

The same argument covers the older half of this. Whether the ombudsman's key is
*also* a bonded validator is a fact about chain state, not about a parameter
struct — a validator admitted with that key, an operator changing hands. Guarding
where the power is exercised is the only place the guard holds for every state the
chain can reach, and the field's disappearance is what makes that no longer belt
and braces but the whole belt.

And the positive half: `MsgOmbudsmanVeto` is the only message whose signer is
the ombudsman, and the only status it can write is a terminal one. There is no
branch in it that creates a case, sets one to voting, or increases any tally.
The office cannot open a case because there is no code that would let it — not
because there is code that stops it.

A test enumerates every message in the service by reflection and asserts the
ombudsman is refused by all but the veto, so a message added later fails until
somebody decides, deliberately, which side of the line it falls on.

### What a veto is not

It is not permanent protection, and it is not meant to be. Any validator may
open a fresh case against the same target in the next block. What the veto buys
is that doing so is a **new, public accusation** every time, and stopping it
again costs the ombudsman another signature on the record.

Two things the ombudsman must not also be, and they are now checked in two
different places because only one of them is still a fact about the parameters.
`Params.Validate()` refuses an ombudsman that is the **recovery destination**,
which receives what seizures take — both are fields, so a struct method can
compare them. `UpdateParams` refuses an ombudsman that holds
**`ROLE_ENFORCEMENT_AUTHORITY`**, which can open cases — that is a grant in
another module, so the check moved to where the state is. The office is meant to
be outside, and those are the two ways it would not be.

If `ombudsman` is unset there is no veto at all, which is the default. An
unappointed office means nobody, never anybody.

## The rolling cap on seizures

A chain that can seize a bounded amount per period cannot be used for mass
expropriation in one sitting, whoever has captured the validator set, because
the arithmetic refuses before anybody has to.

```bash
blockchaind query enforcement window
```

```json
{
  "window_start_height": "612480",
  "current_height": "733440",
  "seized": [{ "denom": "uyml", "amount": "40000000000" }],
  "cap": [{ "denom": "uyml", "amount": "100000000000" }],
  "remaining": [{ "denom": "uyml", "amount": "60000000000" }],
  "seizure_count": "2",
  "max_seizures": "5"
}
```

There are two caps and they cover different gaps:

- `seizure_window_cap` limits **value**, per denomination, within any window.
- `max_seizures_per_window` limits **count**, whatever the value.

The count cap is the half that cannot be walked around by choosing a currency
nobody thought to price. A value cap only binds the denominations it names; a
count cap binds everything, including one issued the day after the parameters
were last set.

### How the window works

Rolling, not periodic. A window that reset on a fixed boundary would let twice
the cap through by placing half the seizures either side of it, and whoever was
doing that would be doing it on purpose.

The state is one ledger keyed by `(height, case id)` and nothing else. Summing
the window is a range scan from its start height, which touches exactly the
records inside it. **The number of records inside a window is bounded by the cap
itself** — a chain that permits five seizures a week has at most five to add up.
The cap pays for its own enforcement.

Nothing is forgotten. The sum's lower bound is computed from the current height
every time it is asked, so correctness does not depend on pruning having run;
pruning exists only to stop the store growing forever.

There is deliberately **no division** anywhere in the window arithmetic — no
bucket index, no modulus. A bucketed window needs a divisor, and a divisor that
arrives as a zero from genesis halts the chain in an end blocker. Subtraction
cannot do that. What it can do is underflow, so both ends are guarded at the
point of use as well as in `Params.Validate`.

### When the cap refuses

The case is **not** cancelled and **not** lost. It stays held, its target stays
frozen, and it is re-queued for the height at which the window could next have
room — when the oldest seizure in it falls out. An `EventSeizureDeferred` is
emitted **every time** it is refused, carrying the retry height and a sentence
saying which limit refused it.

> A case quietly waiting is indistinguishable from a case that has been
> forgotten, and the difference matters most to the person still frozen.

A seizure larger than the whole window's budget will keep deferring, visibly,
once per window, forever. Governance raising the cap or reversing the case are
the two ways out, and both need somebody to know it is stuck.

## The emergency path

The validator process has two failure modes a real network cannot simply wait
out. A theft at three in the morning with nobody awake to open a case. And a
freeze that was plainly wrong, sitting on a business's payroll for the twelve
hours until the voting period ends — "wait a day, it expires by itself" is not
an answer anyone can give a customer.

So an **enforcement authority** can do two things immediately: **freeze** and
**release**. That means a holder of `ROLE_ENFORCEMENT_AUTHORITY` in `x/alias`'s
grant registry, and the role goes to an `x/group` account, so those actions still
take several people.

### It used to be one address, and the shape of that is what changed

The signer used to be `emergency_authority`: one address, named in this module's
parameters, with no territorial limit at all. It could freeze any account on this
chain.

That shape is worth naming because it is the one the perimeter exists to abolish.
The single fastest power in the module — the one path that acts on one signature,
without waiting for a validator — was also the one path no border bounded. What
replaces it is not slower: an office signs, in one block, exactly as before,
inside a country. `EmergencyFreeze` now requires the signer to hold the role
covering the country the **target** is recorded in, which is the same check every
other authority action on this chain routes through, and skipping it because the
situation is urgent would mean an authority that needs to stop an account outside
its perimeter never needs the authority of that perimeter.

**"Unset means nobody" survives the move, in a new form.** The parameter's
careful property was that an empty `emergency_authority` meant *nobody*, never
*anybody*, and there was a dedicated error for it. The grant carries the same
property by a shorter road: no grant is no emergency path, because there is
nothing to satisfy the check. A store failure resolving the registry refuses too,
and so does a target the chain cannot place — the perimeter refuses an
unplaceable target *before* any grant is read. There is no configuration of this
module in which the emergency path is open to whoever noticed first.

The error you get on an empty chain is `ErrOutOfScope`, from the perimeter, and
it names the country the signer's grant did not reach. Code **1114**,
`ErrNoEmergencyAuthority`, is retired and will never be reused: a client that
special-cased it should find that out, and a future error wearing that number
would tell every one of them something false.

### What happened to the address a running chain already had

The `roles-that-do-something` upgrade carries it across, and the grant it writes
is **chain-wide** rather than scoped to a country. That is the honest reading of
what the parameter was rather than a preference: `emergency_authority` had no
territorial limit, so choosing a country here would be an upgrade narrowing an
authority nobody voted to narrow, and picking the country on top of it.
Governance can revoke that grant and issue country ones the moment it wants to;
an upgrade that silently removed the emergency path is not something anybody can
undo before they notice.

The grant is written by the upgrade handler rather than by either module's
migration, and the split is deliberate. Grants live in `x/alias` and
`x/enforcement` has no edge into it — precisely so the perimeter cannot be
widened from inside the module it constrains — so the upgrade handler, which
holds both keepers and runs once at a height governance voted for, reads the old
address out of the raw parameter bytes *before* the module migrations discard it
and writes the grant afterwards. `granted_by` names governance, because
governance is what set the parameter.

### What an office may now do that it could not

Holding the role also lets that account **open an ordinary case** — including a
seizure accusation — without being a bonded validator. That is a real widening
and it should be read as one, because collapsing two mechanisms into one always
is: `emergency_authority` could freeze and release and nothing else. On a chain
that had one, the account that held it can now accuse.

What it does not let it do is **decide** one. A seizure still needs two thirds of
bonded voting power on `MsgVoteCase`, and an office that is not a validator has
no vote to cast. A national authority can stop money for a day; the validator set
decides whether it is taken.

It cannot seize directly, and there is no message that would let it. That is the
whole shape of the power:

> Stopping money is recoverable — release the account and nothing was lost but
> time. Taking it is not. So taking it stays with the validator supermajority,
> whoever is asking.

A freeze an office imposes is **provisional exactly like a validator's**. It
opens a real case, it goes into the same voting queue, it lapses if nobody
confirms it, and the validators can refuse it — which releases the account in
that same block. The office is faster than the set; it is not above it.

Both messages are signed by the group policy, so they go through an `x/group`
proposal rather than a key at a terminal:

```json
{
  "@type": "/blockchain.enforcement.v1.MsgEmergencyFreeze",
  "authority": "<the enforcement office's group policy address>",
  "target": "yml1...",
  "reason": "exchange reported the deposit as stolen at 03:14 UTC",
  "evidence_uri": "https://…/report.pdf",
  "evidence_hash": "<sha256>"
}
```

```bash
blockchaind tx group submit-proposal proposal.json --from officer-1
blockchaind tx group vote <proposal-id> <officer> VOTE_OPTION_YES --from officer-2
blockchaind tx group exec <proposal-id> --from officer-1
```

Releasing is the same shape:

```json
{
  "@type": "/blockchain.enforcement.v1.MsgEmergencyRelease",
  "authority": "<the enforcement office's group policy address>",
  "case_id": "4",
  "reason": "the counterparty was the exchange's own settlement account"
}
```

Release works on a case that is still being voted on, on one that already
passed, and on a seizure waiting out its delay. What it does **not** do is undo a
seizure: if the funds have moved, releasing gives the account back and nothing
else. Those funds are the recovery destination's to return.

**Release is scoped to the case's target, not to anything the message names.**
There is no country in `MsgEmergencyRelease` — only a case id — so the case is
loaded first and the signer is then checked against the country its **target** is
recorded in. That ordering is forced rather than chosen: taking a country from the
signer instead would let an actor name its own perimeter, which is the one claim
the perimeter design refuses.

Leaving release chain-wide was the defensible alternative, since it is the smaller
act and a wrong freeze is an emergency of its own. It was not taken, because an
office able to release anywhere could lift the freeze another country's authority
had just imposed, which is interference in that perimeter rather than mercy in its
own.

The cost of that is real and is better stated than discovered: **a target whose
jurisdiction the chain cannot resolve cannot be released by this message at all.**
The perimeter refuses an unplaceable target before any grant is read, so there is
no office that can act for it. What is left for that account is the provisional
freeze lapsing by itself, and governance reversing a case that passed with
`MsgReverseCase`. Both are slower and both exist.

Everything the emergency path does is marked as such — on the case, in the
events, and with its own badge in the explorer. "A validator saw this" and "an
authority acted directly" are different facts about how the chain is being run,
and only one of them is the normal case.

If nobody holds `ROLE_ENFORCEMENT_AUTHORITY` there is no emergency path at all,
which is the state a chain starts in. No grant means nobody, never anybody — and
`query alias role-holders <CC>` and `query alias chain-wide-grants` are how you
find out which it is.

## When it goes wrong

Three ways out, in increasing order of how much the chain has already done:

**Withdraw.** Whoever opened a case can take it back while it is still being
voted on, which releases the account immediately. The party trusted to impose a
freeze alone is trusted to admit alone that they were wrong.

```bash
blockchaind tx enforcement withdraw-case <your-account> 1 --from validator
```

Withdrawal matches on **identity and nothing else**. An office whose grant has
since been revoked, and a validator that has since unbonded, may both still
withdraw the case they opened — deliberately, because withdrawing is
de-escalation, and a rule that made it conditional on still holding a power would
leave somebody's account frozen precisely because the party that was wrong about
them lost its authority afterwards. The one consequence to know: an unbonded
validator renders as its own account address rather than its operator address, so
it no longer matches a case it opened under the operator name. The freeze it
imposed still lapses, the validators can still resolve the case, and governance
can still reverse one that passed.

**Let it lapse.** A case nobody votes on expires at the end of its voting
period, and the account is released. The status is `EXPIRED`, deliberately not
`REJECTED`: silence is not a finding, and the record should not claim it was.

**Veto.** The ombudsman can stop any case that has not taken anything yet (see
above), including a seizure the validators have already agreed to.

**Release or appeal.** An enforcement authority for the country the case's target
is recorded in can lift the freeze immediately (see above). Governance can also
overturn a case that already passed or is waiting out its delay, through a
proposal carrying `MsgReverseCase` — and that path asks nothing about the target's
country, which is what makes it the one route left when the chain cannot place
the target at all. Either way the freeze is lifted and the reversal is recorded
beside the original accusation.

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

### The exception is narrower than it looks, and that is intended

A frozen account cannot use the recovery-destination exception itself unless the
transaction carries a **zero fee**. Paying a fee is a bank send to the fee
collector, and `SendRestriction` blocks it before the exempt transfer is ever
reached.

This is worth stating because it looks like a bug and is not. The exception
exists so the *module* can move what a seizure seized; it is not there so a
frozen account can voluntarily hand money over. Exempting fee deduction as well
would let a frozen account burn its balance on fees — draining, through the fee
collector, exactly what the freeze was imposed to preserve. Somebody who wants
to make restitution to the foundation should do it from an account that is not
frozen, or ask the foundation to sweep.

### Freeze lapse: a backstop, not the ordinary path

`provisional_freeze_blocks` is documented as what limits a freeze one signature
imposed alone, whether a validator's or an office's. It does, but not by the
route the name suggests, and the
difference is worth knowing before anybody goes looking for a lapse on a running
chain and concludes the mechanism is broken.

`Params.Validate` requires `provisional_freeze_blocks >= voting_period_blocks`.
So the **vote always ends first**, and every outcome of a vote except passing
lifts the freeze on the spot. A provisional freeze therefore cannot normally
reach its own expiry height: the case is resolved before it gets there, and a
case that passed had its freeze made permanent.

What actually releases an unconfirmed freeze in practice is the case expiring at
`voting_ends_at_height`. The expiry queue is the **backstop** underneath that —
what stands between an account and a permanent freeze if a resolution never
comes, because a queue entry was lost to a bad import, a migration, or a bug.
It is tested against exactly that state, which has to be constructed because it
cannot be reached by using the module correctly.

## The parameters

| Parameter | Default | What it decides |
|---|---|---|
| `voting_period_blocks` | 8,640 (~12h) | how long validators have |
| `provisional_freeze_blocks` | 17,280 (~24h) | how long a freeze one signature imposed lasts unconfirmed |
| `threshold_bps` | 6,667 | the share of bonded power a case needs |
| `recovery_destination` | *(required, no default)* | the only address seized funds can go to |
| `seize_requires_evidence` | true | whether a seizure can be opened without a record |
| `seizure_delay_blocks` | 8,640 (~12h) | the floor every seizure waits after the vote |
| `seizure_delay_tiers` | *(required, no default)* | how much longer a larger seizure waits |
| `seizure_window_blocks` | 120,960 (~7d) | the period the caps below are measured over |
| `seizure_window_cap` | *(required, no default)* | the most that may be seized per window, by value |
| `max_seizures_per_window` | 5 | the most that may be seized per window, by count |
| `ombudsman` | *(unset)* | the office that can stop a case and never start one |

There is no parameter for the legal instrument, and that is deliberate. A
requirement governance can vote away is a default.

There is no parameter for the emergency authority either, and there used to be:
field 8 held `emergency_authority`, and it is now **reserved** — the number will
never be reused, because a chain that has run carries parameter bytes with a
field 8 in them and a new field wearing that number would decode an old
authority's address as whatever the new field is. Who may freeze in an emergency
is a grant in `x/alias` now, scoped to the target's country. See
[the emergency path](#the-emergency-path).

`recovery_destination` is the foundation account: the trust body administering
the chain, which holds what is recovered so it can be restituted to the people
it was taken from.

**It is not a key.** It is a 3-of-5 `x/group` policy account — five named
custodians, any three of whom can act, every signature attributable on chain.
That is not a stylistic preference: the devnet's foundation account was a single
unencrypted key in a `keyring-test` backend on a cloud VM, printed once and
written down nowhere, and losing that VM would have lost every asset the chain
had ever recovered. The constitution fixes the shape as well as the address —
exactly five custodians, exactly three signatures — and a departing custodian is
replaced in the same message or the update is refused. See
[the key ceremony](key-ceremony.md).

It has no default and it is not optional. No address compiled into the binary is
anybody's foundation, so genesis has to name one — and a genesis that does not
is refused at `InitGenesis`, which means the chain does not start rather than
starting wrong. That is deliberate, and it is a change: it used to be empty by
default, on the argument that a chain which can freeze but not seize is a safe
place to launch from. The devnet then ran for weeks in exactly that state, and
what it actually bought was a chain where two thirds of the validator set could
pass a seizure that had nowhere to send what it took. Nobody noticed until a
console printed the parameter.

Changing it afterwards is **not** an ordinary governance proposal, and that is a
correction to what this guide used to say. `recovery_destination`,
`threshold_bps`, `voting_period_blocks` and `provisional_freeze_blocks` are
invariants: they are fixed at genesis in `x/constitution`, and a
`MsgUpdateParams` that tries to move any of them is refused — as is a genesis
whose values disagree with the settlement, which stops the chain from starting
at all rather than leaving it with two answers.

Moving one takes an amendment: a governance proposal, a multi-week public delay,
and a supermajority of the validator set ratifying it separately. See
[what governance can and cannot change](constitution.md).

The reasoning is short. A chain that can vote to lower its own seizure threshold
does not have one — the threshold is a safeguard the validator set applies to
itself, and a safeguard the same body can remove by the same vote is a statement
of intent rather than a rule. The destination is the same argument one step
further: the threshold decides whether funds move, the destination decides who
ends up with them.

The rest of the table above is still ordinary. `max_reason_length` and
`seize_requires_evidence` move by proposal as before.

`threshold_bps` cannot be set at or below 5,000 by any route, amendment
included. There is no configuration of this module worth having in which a
minority of the validator set can take somebody's assets, so that is refused at
the settlement level rather than left to a vote that happens to read
reasonably.

`seizure_delay_tiers` and `seizure_window_cap` have no defaults for the same
reason `recovery_destination` has none, and it is worth being precise about
which reason that is. It is not that a sensible default is hard to pick. It is
that a default priced in `uyml` would be a **live** schedule on a chain issuing
Kenyan shillings — matching nothing, capping nothing, and satisfying every
check. An absent value that stops the chain is safer than a present one that
looks configured and is not.

`seizure_delay_blocks` cannot be zero: a veto with no window to be cast in is
not a veto. `max_seizures_per_window` cannot be zero either, but for the
opposite reason — it would leave every seizure the validators passed waiting
forever with its target still frozen, which is a broken chain rather than a safe
one.

Every denomination named in a delay tier must also be capped by
`seizure_window_cap`. Individually legal and collectively wrong is the failure
this module's genesis keeps hitting, and a chain that delays large seizures in a
currency it does not cap has two safeguards on paper and one in fact.

### Going to the constitutional layer

`threshold_bps`, `recovery_destination`, `seizure_delay_blocks`,
`seizure_delay_tiers`, `seizure_window_blocks`, `seizure_window_cap`,
`max_seizures_per_window` and `ombudsman` are all intended to become
genesis-fixed invariants that `MsgUpdateParams` refuses to change, amendable
only by supermajority plus a multi-week public delay.

Until that lands they are ordinary parameters, changeable by an ordinary
governance proposal. A chain that can vote to shorten its own seizure delays on
a Tuesday afternoon has not really got them.

## What this costs

Be clear about what has been built here. The validator set can stop any account
on this chain from spending, within one block, on one member's say-so; and with
two thirds it can take what that account holds. That is a real power over other
people's money, and no amount of process makes it not one.

A country's enforcement authority can stop an account in its own country on its
own signature too, and can accuse one without holding a validator's stake —
though it can never take anything, has no vote on whether anything is taken, and
any account it stops is released the moment the validators say so. That last set
of powers used to belong to one address with no country attached to it at all,
which was the fastest power in the module and also the widest.

What is offered against all that is not a promise but a shape: every case is
public from the block it opens, carries its author's name and its grounds,
expires unless confirmed, can only ever move funds to one governance-set
address, and leaves a record — including of the cases that were rejected,
withdrawn, vetoed, released or overturned — that is never deleted.

And around a seizure specifically, four constraints that are arithmetic rather
than good intentions. It cannot be opened without naming an instrument some
court or regulator issued outside this chain. It cannot be carried out in the
block it is agreed, and the more it takes the longer it waits. It can be stopped
in that window by an office that has no power to start one. And the chain will
only do so much of it per week, by value and by count, before refusing itself.

None of that makes the power safe. What it does is make each use of it slow
enough to argue with, attributable to somebody, and bounded in aggregate — so
that the question "could this chain be turned on its own users" has an answer
that is checkable rather than reassuring.

Whether that trade is the right one is a question for the people running the
chain, not for the code. What the code can do is make sure the trade is visible.
