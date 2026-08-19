# User IDs and the address book

**Status: designed, not built.** Two layers that together replace `yml1chmca667fk4wtsf47ghnrzvnfgw7kds4u97a8p` with something a person can
read, say aloud, and check.

They are deliberately separate, and the separation is the whole design:

| | What it is | Where it lives |
|---|---|---|
| **User ID** | `K3M9-7QRT-B` — one globally unique handle per account | **on-chain**, `x/alias` |
| **Address book** | *your* name for someone — "Mum", "Acme Ltd" | **only on your device**, never on-chain |

One needs consensus because two people must not hold the same ID. The other
needs no consensus at all, and putting it on-chain would publish the entire
social graph of everyone who uses the network.

---

## Part 1 — the user ID

### The format

```
K3M9-7QRT-B
└──────┘ └┘
 8 chars  check character
```

**Alphabet: Crockford Base32** — `0123456789ABCDEFGHJKMNPQRSTVWXYZ`. It drops
`I`, `L`, `O` and `U` from the 36 alphanumerics. The first three go because
`I`/`l`/`1` and `O`/`0` are the transcription errors people actually make; `U`
goes so a random draw cannot spell something obscene. Crockford also decodes
case-insensitively and maps `I`→`1`, `L`→`1`, `O`→`0` on the way in, so the most
common typo corrects itself instead of failing.

**Size: 32⁸ = 1,099,511,627,776.** One-point-one trillion — **11× the hundred
billion minimum**, from eight characters.

**The check character** is computed with **Luhn mod N** over base 32.

This was specified as the Damm algorithm and built as Luhn, and the difference
is worth stating rather than papering over. Damm would catch every adjacent
transposition; it needs a 32×32 totally anti-symmetric quasigroup, and a
hand-constructed one that is subtly wrong is worse than a standard algorithm
that is right. Luhn mod N is well defined for any base and easy to verify.

What it actually delivers, measured by
[`x/alias/types/id_test.go`](../../x/alias/types/id_test.go) rather than
asserted:

- **Every single-character substitution is caught.** Tested exhaustively —
  every position against all 32 symbols.
- **Adjacent transpositions: 4 missed out of ~1,600 tested.** Luhn's known gap
  is a swapped pair differing by exactly half the base; the test pins the number
  so a regression that breaks the check outright is caught.

That is a typo check, not a signature. It stops somebody paying the wrong
account because they misread a character down a phone line. It stops nothing
deliberate. If the remaining 0.25% ever matters, Damm is the upgrade and the
identifier format does not change.

The hyphens are presentation only. Input is stripped and upper-cased before
lookup, so `k3m97qrtb` and `K3M9-7QRT-B` are the same ID.

### Evolutive by construction

Payload length is a module parameter, defaulting to 8 and permitting 8–16. Each
alias stores its own length, and the check character is computed over whatever
length it has, so raising the parameter later issues longer IDs **without
invalidating a single existing one**. At 16 characters the space is 32¹⁶ ≈
1.2 × 10²⁴.

Nobody will exhaust 1.1 trillion. The parameter exists so that a future
requirement — a separate range for merchants, say — is a config change rather
than a migration.

### The IDs are assigned, not chosen

This is the most consequential decision here, and it goes against the instinct
to let people pick a vanity handle.

A chosen handle means squatting, a resale market, and — the one that actually
costs money — **impersonation**. `YAMALE-PAY` and `YAMALE-PAY1` look identical
in a payment confirmation to somebody in a hurry. Every phishing attack on a
name-based payment system starts here.

So the chain derives it: `alias = base32(truncate(sha256(address ‖ nonce)))`,
incrementing `nonce` on the vanishingly rare collision. Deterministic, so every
validator computes the same thing. Non-sequential, so the registry does not leak
how many users exist or in what order they joined. And unchooseable, so there is
no name to squat and no lookalike to register.

This is also what PIX does with its random keys, and for the same reason.

### Retired IDs are never reissued

An account whose key is compromised rotates to a new ID. The old one goes into a
tombstone set and is **never** issued again.

Reissuing would mean a payment sent to a handle somebody memorised last year
arrives in a stranger's account. That is a money-loss bug with no error message,
and the only defence is to never let the situation exist. Tombstones cost nine
bytes each.

### State and messages

```go
Aliases   collections.Map[string, string]        // alias -> address   0x01
Owners    collections.Map[sdk.AccAddress, string] // address -> alias  0x02
Retired   collections.KeySet[string]              // never reissue     0x03
Params    collections.Item[Params]                // 0x00
```

`Owners` is derived: `InitGenesis` rebuilds it from `Aliases` and `ExportGenesis`
does not emit it, so genesis round-trips byte-for-byte.

Two messages, and no more:

- **`MsgRegisterAlias{account}`** — claim an ID. The chain picks it; the response
  and the event carry it back.
- **`MsgRotateAlias{account}`** — retire the current ID and take a new one. One
  message rather than release-then-register, because that ordering must be
  atomic: an account between the two would be unreachable.

Queries are `Alias(id) → address`, `AliasOf(address) → id`, and `Params`. There
is deliberately **no list query** — enumerating the directory is an indexer's
job, not a chain endpoint's.

### What it does not change

An alias resolves to an address. **Every existing gate still applies to that
address**, and this is the property that must never be weakened:

- `x/paymsg` still requires both participants approved and the customer
  registered by the institution naming them. An ID is not a way around the
  participant gate.
- `x/enforcement`'s `SendRestriction` still refuses a frozen account. A frozen
  account's ID keeps resolving on purpose — somebody whose payment was refused
  needs to reach the case and read the grounds.

Adding `x/alias` changes the module set, the store keys and the genesis format,
which is consensus-breaking. On the devnet that is a restart, not an upgrade.

### The privacy limit, stated plainly

**A typable ID directory is enumerable, and no clever storage fixes that.**

Eight characters is 2⁴⁰ possibilities. Hashing the key before storing it would
force an attacker to brute-force rather than read — but 2⁴⁰ hashes is hours on
one GPU, so it raises the cost without changing the outcome. Making the ID long
enough to resist enumeration (13+ characters) makes it untypable, which defeats
the point.

So the design accepts it and constrains the damage: **the ID maps to an address
and to nothing else.** No name, no phone number, no email, no document number
lives in this module. Someone who walks the registry learns which addresses have
IDs — which they can already see on a public chain — and nothing about who owns
them.

That constraint is worth holding, because it is exactly the one PIX did not
hold: Brazil's leaks were damaging because the keys were tied to names and
document numbers. Ours resolve to a bech32 string.

---

## Part 2 — the address book

Everything above gives you `K3M9-7QRT-B`. That is checkable and safe to dictate,
but it is not *memorable*, and nobody wants a payment history that reads like
licence plates.

The address book is **your private names for people**, and it stays on your
device. Not on-chain, not on a server:

- It needs no consensus — nobody else has to agree that you call someone "Mum".
- On-chain it would publish who transacts with whom, permanently. The social
  graph is more sensitive than the payments.
- It changes constantly, and consensus is the most expensive place to store
  anything that changes constantly.

An entry is `{ address, userId, pseudonym, addedAt, note }`, held in browser
storage, exportable as a file the user controls.

### One display rule, everywhere

This is what "replace the wallets by the user ID or pseudos" means in practice.
It belongs in the SDK, written once, so the explorer, the wallet, the Safe and
the transfer app cannot disagree:

```
displayName(address):
  1. a pseudonym in my address book   →  "Acme Ltd"
  2. else a registered user ID        →  "K3M9-7QRT-B"
  3. else                             →  "yml1chm…7a8p"
```

Raw addresses stop being the default and become the fallback — visible when you
ask for it, in the expert view, and in any payload you are about to sign.

### Why a local pseudonym is safe to trust

A nickname you can set yourself is normally a phishing vector: attacker calls
themselves "Acme Ltd", you pay the wrong Acme.

Here it is not, and the two layers are why:

- **Pseudonyms are strictly local.** Nobody can push one to your device. There
  is no field on-chain for anyone to write a name into.
- **User IDs are assigned, not chosen.** There is no lookalike to register.

So the only way a wrong name gets into your book is if you put it there. The
apps defend that one moment: adding a contact confirms against the full user ID,
and the first payment to a new contact shows the ID in full alongside the
pseudonym rather than the pseudonym alone.

---

## Part 2b — claimed names: the ENS-style tier

A local pseudonym is private and a user ID is assigned. Neither gives a business
a name its customers can be *told*. That is the third tier: a chosen, owned,
transferable name, held as an NFT attached to the wallet.

It is worth building. It also reintroduces every risk `x/alias` was shaped to
avoid, so it has to be a **separate namespace with different rules**, not a
softening of the user ID.

### Three tiers, and why they must not blur

| | Chosen by | Transferable | Renders as |
|---|---|---|---|
| Pseudonym | you, privately | n/a — local | `Acme Ltd` |
| User ID | the chain | never | `K3M9-7QRT-B` |
| **Claimed name** | **the holder** | **no — soulbound** | **`@acme`** |

**Decided: names are free, and scarcity comes from the account.** No lease, no
renewal, no fee model — a name is issued on request. That works here for a
reason that does not hold on a public network: **one name per account, and
accounts are verified**. The scarce resource is the identity, not the name, so
squatting is bounded by how many accounts a person can legitimately hold rather
than by what they can afford.

It removes the whole pricing question, which was the hardest open decision in
this tier — priced too low and bots take the namespace before a real business
registers; too high and nobody claims one.

**The condition it rests on, stated so it is not lost:** if account creation is
ever unverified, this collapses. Anyone able to open unlimited accounts can take
unlimited names, and the namespace goes to whoever scripts fastest. Free names
and open registration cannot both be true.

**Decided: claimed names are not transferable.** They are an NFT for ownership
and enumeration, not for trade. That single choice removes most of what makes
ENS-style naming dangerous — there is no resale market, so no squatting for
profit; no change of owner, so no payment aimed at a remembered name landing
with a buyer; no cooldown machinery to build. On a network for regulated
institutions, a name is an identity rather than an asset, and identities are not
supposed to change hands.

The sections below on transfer are kept for the record of *why* the alternative
was rejected. They describe a model this chain is not adopting.

**The rendering difference is a security control, not decoration.** A claimed
name must never be able to look like a user ID, or somebody registers `@K3M9-7QRT-B`
and harvests payments meant for the account that was *assigned* it. Fixed
sigil, disjoint character sets, enforced by the chain at registration.

### The four rules that decide whether this is safe

**1. Confusables, or the whole thing is a phishing kit.** Lowercase `a–z`,
digits, and hyphen. Nothing else — no Unicode, so no Cyrillic `а` in `@аcme`,
no zero-width joiners, no right-to-left overrides. Additionally reject a name
whose *skeleton* collides with an existing one: `@rn` versus `@m`, `@acme` versus
`@acrne`, `0`/`o`, `1`/`l`. ENS shipped without this and homoglyph theft became
routine. It is far cheaper to enforce at registration than to arbitrate later.

**2. Transfer is the sharp edge, and it is the reason payments break.** A user
ID is safe partly because it never moves. A tradeable name means the handle
somebody memorised last year can be sold to an attacker, and every payment aimed
at the remembered name now lands with the buyer. So:

- a **transfer cooldown** — the name resolves to nothing for a period after it
  changes hands, rather than silently resolving to the new owner;
- **provenance in the resolution result**, so a payment screen can say *"this
  name changed owner 6 days ago"* — the client must be able to warn, which means
  the chain must expose when ownership last moved;
- a payment to a recently-transferred name shows the address in full, not the
  name alone.

**3. Expiry, or the good names are gone in a week.** Registration is a
time-limited lease with renewal, priced so squatting thousands of names costs
real money. A permanent free claim means the entire useful namespace is taken by
bots before a single real business registers.

**4. Reserved from genesis.** Currency codes, `@yamale`, the foundation,
approved participants' trading names. Held back and released only by governance
— on a payments network, `@safaricom` resolving to a stranger is not a naming
dispute, it is theft with extra steps.

### Verified is a different claim from claimed

`@acme` means somebody registered it. It does **not** mean they are Acme. On a
network for regulated institutions that distinction is the product: a claimed
name and a name verified against an approved `x/paymsg` participant must render
differently, and a payment interface should treat unverified claimed names with
the same caution as an unknown address.

Verification belongs to governance — the same mechanism that already approves
issuers and participants — not to whoever holds the NFT.

### The name binds to the user ID, not to the address

The NFT carries the **user ID of the account it was minted for**, and resolution
runs in two hops: `@acme` → user ID → address.

This is worth the extra indirection, because it turns rule 2's hardest problem
into a check anyone can make.

**A transfer becomes self-evident.** The name records the user ID it was issued
against. If the current holder's user ID no longer matches the one recorded on
the NFT, the name has changed hands — and any client can see that without
trusting an event log or a history index. A payment screen can then do what it
should:

> **@acme** — minted for `K3M9-7QRT-B`, currently held by `P70Q-2XVC-4`.
> This name has changed owner. Check the address before paying.

Compare that with binding straight to an address, where a transfer is invisible
in the resolution result and the only defence is remembering to query
provenance separately — which no client does reliably.

**It also survives key rotation.** `MsgRotateAlias` retires a user ID and issues
a new one to the same account, for a holder whose key was compromised. A name
bound to an *address* would break, or worse, keep resolving to an address the
attacker controls. Resolution therefore goes through the account: the recorded
ID is provenance, and the *current* ID of the holding account is what resolves.
Rotation carries the name with it and leaves a visible mark that it happened.

The rule that falls out of both: **the recorded ID and the live ID are separate
fields, and the query returns both.** Collapsing them to one loses exactly the
signal that makes the transfer visible.

### Several addresses under one name

Yes — an institution has a treasury, an operations account and a payroll
account, and wants all of them known as Acme. But the two directions of that
relationship have opposite risk profiles and must not share a rule.

**Decided: one name, one address. One user ID, one address. Neither is ever
repointed.**

This is the PIX arrangement, and it is simpler than everything below. A person
or institution with three accounts registers three names and holds three user
IDs — the accounts are distinguished by a modifier rather than by a subrecord:

```
chris-france    →  yml1chm…7a8p
chris-sa        →  yml13tt…mh9r
acme-treasury   →  yml1srx…frpm
```

Every ambiguity discussed further down disappears with it. There is no list to
resolve, no primary to preselect, no client rule about picking. `@chris-france`
means exactly one address, permanently, and a payment interface has nothing to
decide.

**The one consequence, stated plainly.** An immutable binding has no recovery
path: a lost key leaves the name pointing at an address nobody controls, and a
stolen key leaves it pointing at the thief — permanently, and with the name's
familiarity working for the attacker.

So "never repointed" must not mean "never ends". The escape is the same shape
`x/alias` already uses for user IDs:

> A name can be **retired**, never **repointed**.

Retiring kills the name and tombstones it, exactly as a rotated user ID is
tombstoned and never reissued. The holder then registers a new one against their
new account. Payments keep the property that matters — a name never silently
starts resolving somewhere else — while somebody who has been robbed is not left
with an attacker wearing their identity forever.

Note this is precisely why PIX lets you delete a key. It is not a weakening of
immutability; it is the difference between a binding that cannot change and a
binding that cannot be escaped.

**The modifier convention needs the confusable rules more, not less.**
`chris-france` and `chris-fronce` differ by one character, and a naming scheme
that encourages near-identical siblings is exactly where a lookalike hides. The
charset and skeleton-collision checks below are not optional under this scheme.

---

*The remainder of this section describes the many-addresses-per-name model that
was considered and rejected. Kept for the reasoning.*

**One identity, many addresses — the rejected alternative.**

An institution has a treasury, an operations account and a payroll account. All
of them are Acme. The name describes *who*, and the chain still moves money
between addresses; the name is a label over the top, never a substitute for the
address in the transaction.

The only place this can bite is the instant somebody types `@acme` into a send
field, and the fix is a client rule rather than a chain restriction:

> **Resolve to the list. Never pick for the user.**

```
@acme  →  treasury   yml1srx…frpm
          payroll    yml13tt…mh9r
          operations yml188p…3h0j
```

Show them, name them, let the person choose, and put the chosen address on the
confirmation screen. A client that silently resolved `@acme` to whichever entry
came first would be inventing an answer to a question the user did not ask —
that is the danger, and it lives entirely in the interface.

An optional **primary** is a convenience for the common case, not a licence to
skip the choice: if an identity marks one address as its default, a client may
preselect it, but it still shows which one it picked.

**Displaying is many-to-one, and that is safe.**

Any number of addresses may be *labelled* `@acme` in an explorer, because
display moves no money. This is what makes an institution's activity legible
instead of scattered across unrelated-looking accounts.

**But the link must be confirmed from both ends.**

This is the rule that matters. To show address X as `@acme`, two independent
facts must hold:

1. the holder of `@acme` lists X, **and**
2. X itself points back at `@acme`.

Either alone is forgeable and in opposite directions. Without (2), a name holder
labels the foundation's treasury as theirs and an explorer shows it as `@acme`.
Without (1), anybody points their own account at `@yamale` and is displayed as
the foundation. Requiring both means a display name is a claim two parties made
about each other, which is the only version worth trusting.

ENS learned this the hard way and calls it the reverse record; it is the reason
its forward and reverse resolutions are separate transactions.

**Payment interfaces should still show the primary.** If a payment is going to
`@acme/treasury`, say so and show the address — an interface that renders every
Acme account identically as "Acme" has recreated the ambiguity the one-to-one
rule just removed.

### What it is built on

Cosmos SDK ships `x/nft`, which gives ownership, transfer and enumeration for
free. The name module then holds the mapping and the rules — lease expiry,
confusable checks, transfer cooldown, reserved list — and reads ownership from
`x/nft` rather than reimplementing it.

**Sequencing:** `x/alias` first. It is smaller, it carries no economics, and
every account needs an identifier before any account wants a vanity one. Claimed
names are a product decision with a fee model and a governance surface attached;
they should not block the thing that simply makes accounts addressable.

## Part 3 — credentials, 2FA, and who actually holds the key

Once accounts are named by a user ID rather than an address, the obvious next
step is to let people **log in** — an identifier, a password, a second factor —
instead of guarding a recovery phrase. That is the right product instinct. It
also decides something much larger than a login screen, so it should be decided
deliberately.

**A password and a 2FA code cannot authorise a transaction. Only a key can.**
Every design therefore answers one question: where does the key live?

### Two models, and what 2FA is worth in each

**Self-custody.** The key lives on the user's device, encrypted under their
password. This is what `clients/wallet/src/vault.ts` implements today — PBKDF2
at 600,000 iterations, AES-GCM, unlocked in memory for the tab.

Here **2FA at login adds almost nothing**, and it is important not to pretend
otherwise. An attacker holding the device and the password already has the
ciphertext and the means to open it; there is no server in the loop to demand a
second factor from, so there is nothing for the code to gate. Self-custody's
real defence is that the key was never anywhere else, and its real failure mode
is that a forgotten password is a permanently lost account.

**Custodial.** The key lives on a server operated by an institution. Credentials
plus 2FA authenticate the user *to that server*, which then signs for them.

Here **2FA is doing genuine work** — it is the actual control protecting the
account, because the server is the thing that decides whether to sign. It also
buys what self-custody cannot: password reset, account recovery, a support desk,
transaction limits, and an audit trail of who authorised what.

The honest cost is that the institution can move the money, which makes it a
regulated custodian in most jurisdictions rather than a software vendor.

### Which one this chain is shaped for

Custodial, for consumers — and the chain is already built that way. `x/paymsg`
assumes institutions act for customers: a participant registers a customer, and
a payment names the institution that acts for the account paying. That is a
custody relationship expressed on-chain. `x/feegrant` already lets an
institution pay a customer's fees. The PIX comparison holds here too: PIX keys
are held by banks, not by individuals with seed phrases.

So the shape is **both**, chosen per account rather than for the network:

- **Institutions and treasuries** keep self-custody, and multi-signature through
  `x/group` — no single credential should ever move institutional funds.
- **Consumers** get an account custodied by their institution, reached with a
  user ID, a password and a second factor.

The wallet's own vault stays, for anyone who wants to hold their own key.

### If 2FA is the control, the choice of factor is the control

Ranked by what they survive:

1. **TOTP authenticator** — the working default. The secret never leaves the
   device and there is no carrier to socially engineer. (Passkeys/WebAuthn are
   stronger still and worth planning toward.)
2. **Email** — acceptable as a backup. Compromises when the mailbox does, which
   is also where password resets land, so it is one factor wearing two hats.
3. **SMS — do not use it as the primary factor.** SIM-swap is not a theoretical
   attack against a payments network; it is *the* attack, it is cheap, and it is
   run at scale precisely against people moving money. Offering it because users
   ask for it is defensible; making it the default is not.

A second factor must also gate the **operations that matter**, not just login:
changing the password, adding a new device, rotating the user ID, and any
payment above a threshold. A 2FA prompt at login and none at payment protects
the session rather than the money.

### What must be decided before this is built

- Who runs the custody service and under what licence — this is a legal question
  before it is an engineering one.
- Whether keys are held per-customer in an HSM, or one institutional key signs
  for many customers. The second is far simpler and makes the institution a
  single point of total failure.
- How a custodied account is recovered, and what stops that path from becoming
  the cheapest way to steal one. Account recovery is where custodial systems
  actually get robbed.

## Build order

1. `x/alias` — proto, keeper, the Damm check, genesis round-trip, simulation.
2. SDK — `resolveDisplayName()`, address-book storage, ID validation client-side
   so a mistyped ID never reaches the chain.
3. Every existing app switches to `displayName()` in place of `truncateAddress()`.
4. The transfer app, which is mostly the address book plus a confirm screen.
