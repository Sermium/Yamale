# Placement handoff: the menu, the exemption, and the institution console

**Status:** design, approved 2026-09-14. Not yet implemented.

This covers pieces **B** and **C** of the account work. The account service
itself — OAuth and email enrolment, DKG, session, 2FA, background signing — is
piece **A** and gets its own spec. The key-export screen for `x/selfcustody` is
piece **D** and follows A.

---

## The problem

Four faults sit on one path: the moment a newly created account tries to become
usable.

**1. The wallet is offered to consumers.** `/wallet/` sits in the site menu
under *For anyone*, so a member of the public meets 24 words and the sentence
"whoever has it controls the money — there is no support desk that can restore
it". That is correct for a self-custody wallet and wrong as a platform's opening
proposition.

**2. The placement request is a dead end.** `clients/wallet/src/pages/Create.tsx`
ends by printing a text document the holder is told to hand to their institution.
Nothing anywhere consumes it. There is no institution-side screen in this
repository.

**3. The institution field is free text.** `MsgSetJurisdiction` says the first
recording is made by "the approved participant that onboarded the account,
because that is the party that performed the KYC and therefore the only one that
knows the answer." A name typed into a box is a name the chain will never
honour, so the field invites an answer that cannot be acted on.

**4. A foundation administrator cannot complete the form.** `role.proto` makes
`ROLE_FOUNDATION_ADMINISTRATOR` the single exception to "every account has a
jurisdiction" — it may "hold an identifier with no country behind it", and is
"CHAIN-WIDE OR NOTHING", a grant naming a country being refused outright. The
form nonetheless requires a country and tells the holder they will get no user
ID without one. Both statements are true of ordinary accounts and false of the
one group the chain exempts.

Faults 2, 3 and 4 are the same fault from three angles: the handoff from "I have
a key" to "I have a usable account" was specified on the chain and never built
in the interfaces.

---

## C — the menu and the exemption

### C1. Audience, stated rather than implied

The site menu already groups by audience: *For anyone*, *For institutions*, *For
supervisors and operators*. The change is which group each surface sits in.

| surface | today | after |
|---|---|---|
| `/app/` — Yamale Pay | For anyone | **For anyone** (unchanged) |
| `/wallet/` | For anyone | **For institutions** *and* **For supervisors and operators** |
| `/wallet/faucet` | For anyone | For anyone |

The wallet is listed **twice, deliberately**. Both audiences hold their own keys
and both need it: an institution's signer on a `x/group` policy, and a
supervisor or validator operator. A link is not a resource, and duplicating it
costs nothing next to sending one of those two audiences hunting.

The faucet stays under *For anyone*. It serves whoever is demonstrating the
chain, including somebody whose account came from `/app/`, and its being a route
inside the wallet bundle is an implementation detail rather than an audience
statement.

Nothing is deleted and nothing becomes unreachable. What changes is what the
platform **proposes**, which is the whole of the ask.

Copy for the wallet entry changes with it. "Create or load an account, see
balances" describes a consumer product. It should say what it is: the wallet for
people who hold a key on behalf of an institution, and who therefore accept that
losing it loses the account.

### C2. The foundation-administrator path

The placement step gains a fork before the country field, with two choices and
no default guessing:

- **Onboarded by an institution** — the ordinary path. Country required,
  institution chosen from the dropdown of B1 below.
- **Foundation administrator** — no placement request at all, and the reason is
  more specific than "no country". Corrected after reading the keeper on
  2026-09-15: such an account has **no jurisdiction record**, and
  `Keeper.CountryOf` falls through to `hasFoundationGrant` and returns
  `FoundationCountry` = `ZZ`, which is *derived from holding the role and never
  stored*. `MsgSetJurisdiction` refuses `ZZ` outright, precisely so the marker
  cannot be handed to an ordinary account.

  So the screen must not ask for a country, must not produce a participant
  command, and must not say "you will get no user ID" — which is the current
  copy and is false for them. Once governance grants the role, `CountryOf`
  answers `ZZ` and the ordinary Claim-user-ID button works, issuing an
  identifier with a `ZZ` prefix. What the screen owes them is that sequence, the
  cap (`MaxFoundationAdministrators`), and the fact that chain-wide scope is
  governance-only.

**Foundation support staff are not a category the chain has.** There is no
support role in `role.proto`. Somebody working at the foundation who is not an
administrator takes the ordinary path, with a country, holding no role at all.
The screen should say so, because the natural assumption is the opposite and
discovering it at a refused transaction is expensive.

---

## B — the placement handoff

### B1. The institution dropdown

The free-text field becomes a select fed by `x/paymsg`'s
`ListApprovedParticipant`, showing each participant's `name` and `code`.

The empty case is the one that must be designed, because it is the live case:
devnet-2 returns an empty list today. It renders as "no institution on this
chain can onboard you yet" with a word on what that means, never as an empty
select. An empty select reads as a loading failure; a sentence reads as a fact.

### B2. `clients/institution/` — the console

A new console beside the others, following the house pattern exactly: a single
page, no build step, **holds no key and composes no transaction**, prints the
command that the institution's own group signs.

Three screens.

**Take a request.** Paste the placement request (or scan its QR). The console
parses it, then **checks it against the chain rather than trusting the paste**:

- does the account exist;
- does it already hold a jurisdiction, and if so which — because a correction is
  a foundation administrator's act and not the participant's, and a participant
  who could rewrite a country it had already recorded could move a customer out
  from under an authority mid-investigation;
- is the named institution actually an approved participant;
- is the country one ISO has assigned — the reserved codes, `ZZ` among them, are
  refused precisely so the marker the foundation administrators carry cannot be
  handed to an ordinary account.

**Read over REST, not ABCI, and the measurement is the reason.** The other
consoles speak `abci_query` because the proxy denies their modules' REST
prefixes. Measured 2026-09-15, both endpoints this console needs answer `200`
over `/api/rest`:
`yamale/blockchain/paymsg/v1/approved_participant` and
`yamale/blockchain/alias/v1/jurisdiction/{address}`. REST returns JSON, so this
console transcribes no protobuf at all. Do not "fix" it to ABCI without
re-measuring.

**Sign it.** Shows what will be recorded, who signs it, and prints the
`MsgSetJurisdiction` for the participant's group.

**Confirm.** Look up the account afterwards and show the jurisdiction recorded
and the user ID issued, so the institution can hand the holder something that is
true.

### B3. Making the dropdown non-empty

Two governance steps, and the dependency is worth stating because it is not one
command:

1. Grant `ROLE_PAYMENTS_AUTHORITY` to a holder. devnet-2 today has exactly two
   chain-wide grants, both `ROLE_FOUNDATION_ADMINISTRATOR`, and no payments
   authority at all.
2. That holder admits a participant — a bank with a name and a code.

Approved: do both. Each is a proposal the user votes on; I submit and hand over
the vote command.

---

## What is deliberately not here

- **No chain change.** Every fault above is an interface fault against rules the
  chain already enforces. Nothing in B or C is consensus-breaking.
- **No queue on chain.** A holder submitting a placement request as a message
  would be an unauthenticated, spammable write in front of every institution,
  and would need an upgrade to add. Paste and QR are enough for a handoff that
  happens once per account.
- **The account service is not in this spec.** `/wallet/` keeps generating BIP39
  phrases for the audiences that want them. Consumers stop being sent there,
  which is a different statement from the wallet changing.

---

## Testing

`clients/institution/` follows `syndicate` and `livestock`: the logic is pure
functions in their own module, tested under `node --test`, and the page renders
them.

The parser and validator tests are the ones that matter, and each names a
specific way the screen could mislead somebody:

- a malformed paste, which must be refused rather than half-read;
- a request naming an institution that is not an approved participant;
- an account that already holds a jurisdiction — the console must say "this is a
  correction and not yours to make", not print a command that will be refused;
- a reserved country code (`ZZ`), refused for the reason `role.proto` gives;
- an account the chain has never seen, which is an absence and not a failure;
- an empty participant list, which must render as a sentence.

For C, a test that the site menu links only to directories the bundle actually
contains — `deploy/stage-site.sh` already carries that guard, and this change
adds a directory and moves two links, which is exactly the shape that broke it
last time.

---

## Sequence

1. C1 — menu and copy. No dependencies.
2. C2 — the foundation-administrator fork in `Create.tsx`.
3. B1 — the dropdown, with its empty state.
4. B2 — `clients/institution/`, tests first.
5. B3 — the two governance proposals, then a real placement performed end to end
   on devnet-2 and reported with what the chain said.
