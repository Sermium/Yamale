# Working through the devnet-2 audit, on the chain

Everything the audit found that code could close is closed and committed. This
is the other half: the steps that need a transaction, a vote or a key, written
out so they can be checked before they are run rather than after.

Nothing here has been executed. The commands are the ones to run, not a record
of anything that happened — every figure was read from
`https://yamale.tail4355e8.ts.net` on **2026-09-05** at block **196,559**, and
should be re-read before acting, because some of them move.

The order matters and it is the audit's, not mine. Step 1 is what makes the rest
worth doing.

---

## 1. The 976 million YML that is not bonded

**Why first.** Governance quorum is 33.4%, the enforcement supermajority is
66.67%, and a constitutional amendment needs 80% — all measured against 174,900
YML of bonded stake. The two validator operator keys can withdraw 976,733,334
YML between them, and a delegation of any fraction of that clears every one of
those thresholds in a single block. Until this is dealt with, steps 2 to 6 are
protections that one signature can undo.

Read it again before acting:

```bash
blockchaind query distribution validator-outstanding-rewards ymlvaloper1m9xhc6zy7fxfax9t5fnykh9k2e29faj7p4h3kh --node https://yamale.tail4355e8.ts.net/api/rpc
```

| Holder | Rewards | Commission | Total YML |
|---|---:|---:|---:|
| `yml1m9xhc…htmqms` (operator of `pi`) | 799,146,452.40 | 88,794,050.27 | 887,940,502.67 |
| `yml1cgguvt0…v5see4` (operator of `pi-2`) | 79,913,548.25 | 8,879,283.18 | 88,792,831.43 |

Emission has already stopped — `current_provisions_per_block` reads `0`, and
`last_reduction_period` is 1965 — so the figure is fixed rather than growing.
That is the only reason this is not an emergency.

### The decision to make first

There are three honest answers and they are not equivalent:

**Burn it.** Withdraw, then send to a burn address or through a governance
proposal that removes it from supply. Leaves bonded stake at 174,900 YML of a
174,900 YML float, so every threshold means what it says. It also throws away
the entire premine, which is fine on a devnet and is not fine if this genesis is
meant to become anything.

**Bond it.** Withdraw and delegate it back, split across the validator set.
Thresholds then measure against a float that is almost entirely bonded. It does
not fix the concentration — two keys would hold 97.9% of the voting power
outright — so it is only an answer alongside step 5.

**Move it to the foundation group.** Withdraw into the 3-of-5 x/group account,
which is the arrangement the rest of this design assumes anyway. One key stops
being able to act alone without anything being destroyed.

I would take the third for a chain that is going anywhere and the first for one
that is not. It is not my call, and the difference is the whole premine.

### Withdrawing

Run on the host holding each operator key. `--commission` takes the commission
in the same transaction.

```bash
blockchaind tx distribution withdraw-rewards ymlvaloper1m9xhc6zy7fxfax9t5fnykh9k2e29faj7p4h3kh --commission --from pi-operator --chain-id yamale-devnet-2 --gas auto --gas-adjustment 1.4
```

```bash
blockchaind tx distribution withdraw-rewards ymlvaloper1cgguvt0hvdg2602flzan9shg0g56ruje62ug5j --commission --from pi2-operator --chain-id yamale-devnet-2 --gas auto --gas-adjustment 1.4
```

Then check that the balance landed where you expected before doing anything with
it:

```bash
blockchaind query bank balances yml1m9xhc6zy7fxfax9t5fnykh9k2e29faj7htmqms --node https://yamale.tail4355e8.ts.net/api/rpc
```

**Do not skip the check.** Withdrawing follows the withdraw address, which is
not necessarily the operator account, and x/enforcement now resets that address
on a freeze — so the two are not always the same thing any more.

---

## 2. Rotate the treasury admin off the oracle feeder key

`yml1vlukxvmeg6kjtu658sc7lvlu6uj7c4n4p0fmas` is the delegated oracle feeder for
`pi-2` **and** the admin of treasury 2, "Lagos Field Operations". The feeder
runs from a keyring the shipped unit file described as `test` — unencrypted, on
the validator host — under a trust model that says a compromised feeder "cannot
touch the stake". It cannot. It can rewrite the treasury's spend policy, because
`SetSpendPolicy` is admin-only, and then empty it.

The example file is fixed in the repo. The chain is not.

```bash
blockchaind query treasury treasury 2 --node https://yamale.tail4355e8.ts.net/api/rpc
```

Rotate the admin to an account that is not a hot key — ideally an x/group
account, which is what x/alias already requires for role grants:

```bash
blockchaind tx treasury set-admin 2 <new-admin-address> --from <current-admin> --chain-id yamale-devnet-2
```

Then rotate the feeder key itself, since it has been sitting in an unencrypted
keyring: generate a new one, `tx oracle delegate-feeder` it, and set
`FEEDER_KEYRING=file` in `/etc/yamale/feeder.env` on both hosts with the
passphrase supplied through a systemd credential rather than the env file.

While you are there: the operator passphrase for the majority validator is still
in `~/.bash_history` on the VM. That was reported earlier and never used; it has
also never been rotated or shredded.

---

## 3. Appoint an ombudsman

`MsgOmbudsmanVeto` is the only message that can stop a seizure that has passed
and is waiting out its execution delay. The parameter is empty, and
`assertOmbudsman` correctly refuses everybody when it is — an unset authority
means nobody, never anybody. So the safeguard the design describes as "frozen,
decided, and still stoppable for free" has no holder, and the only recourse left
is a governance `MsgReverseCase` from the same voter that could have passed the
seizure.

This is a `MsgUpdateParams` on x/enforcement, so it is a governance proposal.
**Read the current params first and change one field** — `MsgUpdateParams`
replaces the whole set, and an omitted field is a zero, and several of these
zeros are divisors:

```bash
blockchaind query enforcement params --node https://yamale.tail4355e8.ts.net/api/rpc -o json > enforcement-params.json
```

Build the proposal from that file with `ombudsman` set, and with the seizure cap
from step 4 in the same change — one proposal, one delay, one vote.

---

## 4. Extend the seizure cap past `uyml`

`seizure_window_cap` lists one denomination — `uyml` at 500 YML — out of the 48
on the chain. A seizure of any of the 43 fiat currencies, or of an
`amm/pool/*` or `tok/*` denom, is bounded only by `max_seizures_per_window`: 5
per 31.6 hours, at any size. The code comment anticipated exactly this case
("including a currency issued the day after the value cap was last set"); the
configuration never followed it.

List what actually exists before writing the cap, so this does not have to be
done again next month:

```bash
blockchaind query bank total --node https://yamale.tail4355e8.ts.net/api/rpc -o json | jq -r '.supply[].denom'
```

Same proposal as step 3.

---

## 5. The concentration ceilings — start this early, it takes 9.2 days

Two independent reasons the concentration system enforces nothing:

**The ceilings are 100%.** All three invariants read 10,000 basis points.
`Invariants.Validate()` refuses zero — "a ceiling of zero would demote every
validator" — and permits 10,000, so a genesis can declare ceilings that bind
nobody and still pass validation.

**They cover no validator.** `activeSeatHolders` needs an `ApprovedValidator`
declaration, and the live query returns an empty set: both validators were
onboarded through the gentx ceremony, and the founding set was never declared.
A validator with no record "is counted in the total and belongs to no group, so
it can never be demoted". The same emptiness disarms `assertWithinCaps`, which
x/enforcement runs before every freeze.

### 5a. Declare the founding set — do this today, it needs no delay

Each operator applies for itself, then governance approves. `pi` and `pi-2` are
the two.

```bash
blockchaind tx validatorgov apply-validator --moniker pi --description "founding validator, declared retrospectively" --legal-entity-id <entity> --beneficial-owner-id <owner> --jurisdiction <CC> --from pi-operator --chain-id yamale-devnet-2
```

The four identifiers are the whole point of the exercise — the ceilings group
validators by entity, by beneficial owner and by jurisdiction, so two validators
declaring the same `beneficial_owner_id` are one holder as far as the ceiling is
concerned. **Declaring them honestly is the difference between a concentration
ceiling and a form.** If `pi` and `pi-2` are in fact the same operator, say so;
the ceiling being violated is information, and hiding it in the declaration is
the one failure mode this system has no defence against.

Then approve each through governance (`MsgApproveValidator`, authority-gated).

### 5b. Propose the amendment — start it whenever, it lands 9.2 days later

```bash
blockchaind query constitution invariants --node https://yamale.tail4355e8.ts.net/api/rpc -o json
```

`MsgProposeAmendment` takes the **complete** replacement set, not a delta — the
proto says so explicitly, and an omitted field is a zero that becomes a divisor
somewhere. Copy the current set and change the three ceilings.

What to set them to is a real decision and depends on 5a: with two validators, a
ceiling below 5,000 bps demotes one of them immediately, and
`min_active_validators` is 1, so the chain would keep producing blocks with a
single validator and no fault tolerance at all. A ceiling that bites before the
set is large enough to absorb it is worse than none.

Ratification needs 80% of voting power, and the delay is 120,960 blocks.

---

## 6. The rest, in no particular order

- **Raise the oracle vote threshold.** `vote_threshold_bps` is 5,000 and the
  larger validator holds 5,717, so it meets the threshold alone and its own rate
  is the weighted median. On a two-validator set the threshold has to exceed
  5,717 for the median to mean anything — which effectively means it cannot be
  fixed by a parameter, only by a third validator.
- **Grant national authorities for NG, ZA and CI**, or accept that every
  message accepting "governance or a scoped authority" is governance-only
  outside CD. `AssertScope` fails closed, which is right, and the consequence is
  that three of the four countries have no local authority at all.
- **Approve a payment participant**, or the payments product the public hostname
  is named for has no rail behind it. `SendPayment` requires both participants
  approved, so `MsgSendPayment` cannot succeed for anybody today.
- **Include the security headers.** `deploy/nginx/yamale-headers.conf` is in the
  repo and `deploy/deploy.sh` copies it to both hosts, but it does nothing until
  each server block carries `include
  /etc/nginx/snippets/yamale-headers.conf;`. Deliberately not automated: a bad
  header on the only public hostname breaks every console at once. The deploy
  script checks and reports which headers are missing.
- **Set the stablecoin mint ceilings.** The new ceiling defaults to no minting
  when unset, so after the next upgrade no currency can be issued until
  governance states a figure. That is the intended direction and it will look
  like a bug to whoever hits it first.
- **Close the `abci_query` bypass** before any payment is ever made. See
  `docs/scope/gaps.md`; a path regex cannot do it.

---

## What I did not do, and why

Nothing on this page. Every item needs either a key I should not use or a vote
that is yours to cast — and after proposal 8 was voted without being handed over
first, the rule here is that governance gets proposed and handed over, never
completed.

The two code changes that belong to items on this page are also **not** made:

- `Invariants.Validate()` still permits a ceiling of exactly 10,000 bps. It
  should refuse it with the same directness it refuses zero. Left out because it
  belongs in the same change as the figures in 5b, and shipping the guard before
  the amendment would make the running chain's own genesis unloadable.
- Nothing enforces a minimum bonded fraction of supply. That is the structural
  answer to item 1 and it is a design decision, not a fix.
