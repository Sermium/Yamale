# Accounts without a wallet

**This is the design document. The key it specifies is now built.**

> The split key of Part 1 exists as `mpc` — 2-of-3 threshold ECDSA — and has
> signed two payments on the live chain, the second after a password reset that
> did not move the address. See **[mpc.md](mpc.md)** for what was built and
> **[custodian.md](custodian.md)** for the service holding the operator's share.
>
> **Everything else here is still a specification.** No enrolment, no recovery
> workflow, no second factor, no notification. Part 5 — the part that actually
> gets robbed — is written and unbuilt. This page argues the design; those two
> say what exists.

**The goal:** a person signs up with a name, an email and a password, adds a
second factor, and uses the network without ever seeing a seed phrase, an
address, or the word "wallet". The chain is plumbing.

This is achievable. It is also the single largest decision in the product,
because of one fact that cannot be designed around:

> **A password cannot authorise a transaction. Only a key can.**

Every arrangement below is an answer to *where the key lives and who can use
it*. Everything else — the login screen, the 2FA, the encrypted profile — is
consequence.

---

## Part 1 — where the key lives

### The three real options

**A. Server-held keys (fully custodial).** The institution holds keys in an HSM.
Login authenticates you *to the server*, which then signs on your behalf.

- Simplest to build, and the only one where a forgotten password is genuinely
  recoverable.
- The operator can move customer funds. In most jurisdictions that makes them a
  regulated custodian, not a software vendor. This is a licensing decision
  before it is an engineering one.

**B. Password-wrapped keys (encrypted at rest, decrypted in the browser).** The
key is generated on the device, encrypted with a key derived from the user's
password, and the *ciphertext* is stored server-side. Login fetches the blob;
the browser decrypts it and signs locally.

- The server never sees a usable key. A database breach yields ciphertext.
- Feels exactly like a Web2 login.
- **A forgotten password is a lost account**, unless you add a recovery path —
  and the recovery path is then the weakest link (see Part 4).
- This is what Magic.link and Coinbase Wallet's cloud backup do.

**C. Threshold / MPC.** The key is split; the server holds one share, the device
another, and neither alone can sign.

- No single party can move funds, and password reset is possible without the
  server ever holding a whole key.
- Materially more complex, and usually means a vendor (Turnkey, Privy,
  Web3Auth) rather than something built in-house.

### Decided: C — threshold keys, and the operator cannot sign alone

Model A was chosen first and then reversed, for a reason worth recording: the
operator should **not** be able to move a user's funds. On a state-operated
system "the authority can spend any citizen's balance" is a very different
political object from "the authority runs the payment rails", and the second is
the one being sold.

That rules out A. It also rules out B on its own, because B's honest
consequence is that a forgotten password is a permanently lost account — not
viable for a national system serving people who lose phones.

**Threshold keys are the only arrangement that gives both.** The key is split;
the server holds one share and the device another. Neither alone can sign, so
the operator genuinely cannot move funds. And password reset works, because the
server's share can be re-wrapped without any party ever reconstructing a whole
key.

The cost, stated plainly: this is materially more complex than either
alternative, and in practice it means a vendor (Turnkey, Privy, Web3Auth) or a
serious in-house cryptography effort.

**That build-versus-buy question is answered: built in house.** The result is
`mpc`, and it differs from the sketch above in one way worth carrying back here.
This section says "the server holds one share and the device another" — two
shares. Two gives the security property and nothing else: if the password is
gone the device share is gone, and the account is dead with the money in it.
The reset specified below is impossible with two. So there are **three** —
device, custodian, and a recovery share held by the foundation's offline 3-of-5,
deliberately not by the operator — and any two of them sign.

Why threshold ECDSA rather than a 2-of-3 `x/group` account, which would have
needed no new cryptography: a multisig address derives from its member keys, so
rotating a share changes the address, and on this chain that retires the
account's `x/alias` identifier and breaks every saved payee. Resharing rotates
every share under the same public key. [mpc.md](mpc.md) has the argument and the
two transactions that demonstrate it.

Institutions keep self-custody, and treasuries keep multi-signature through
`x/group`. A single credential must never move institutional funds. Custody is a
property of the account; the interface is identical either way.

### Recovery: the design, since it is where these systems are actually robbed

Not the login. Recovery is used rarely, tested less, and grants exactly what an
attacker wants. So it is specified rather than left to a support process:

- **Two approvers minimum**, from different teams. Nobody may both initiate and
  approve.
- **72-hour delay**, with notice at initiation to the registered email *and*
  every enrolled device. A legitimate owner can cancel; an attacker must keep
  them unaware for three days.
- **Outbound payments frozen for 24 hours** after completion. Recovery restores
  access, not immediate spending.
- **Proof that is not public knowledge.** Not name, date of birth and address —
  that is precisely what social engineering runs on. A prior device, a
  transaction the user can name, or in-person verification at an agent.
- **Recoveries published in aggregate** — how many, how long they took — so an
  unusual rate is visible without exposing who.

---

## Part 2 — three encryption problems, not one

"Store everything encrypted so a breach leaks nothing" is the right instinct and
the wrong single mechanism. These are three different problems with three
different answers, and using one for all three produces a system that looks
encrypted and is not.

### 1. The password: hashed, never encrypted

Encryption is reversible. Anything reversible means a breach that takes the key
takes every password — and people reuse passwords, so that breach spends beyond
this system.

**Argon2id**, per-user salt, tuned to a real cost. Never store the password,
never store anything that can be turned back into it, never log it, never email
it.

### 2. The profile: encrypted at rest, but email must stay findable

Name and surname encrypt cleanly with a service key held in a KMS, ideally
envelope-encrypted per record.

Email is the awkward one: login has to *find* a user by it, and you cannot
search a randomised ciphertext. The usual mistake is deterministic encryption —
which makes equal emails produce equal ciphertext and hands an attacker a
frequency-analysable index.

The right shape is a **blind index**: store `HMAC(email, pepper)` for lookup,
where the pepper lives outside the database, and store the email itself under
normal randomised encryption for display. A dump of the database alone yields
neither the addresses nor a way to test guesses at them.

### 3. The key material: split, so no single party holds it

Under threshold keys there is no whole key to protect, anywhere, at any moment.
The server holds a share and the device holds a share; a signature is produced
jointly and neither party ever sees the other half.

That changes what a breach means. A stolen server database yields shares that
cannot sign. A stolen device yields the same. An attacker needs both, at once,
which is a materially harder position than compromising one system — and it is
the property that makes "the operator cannot move your money" a statement about
mathematics rather than about policy.

The server share is still protected in an HSM or KMS, because defence in depth
costs little here. But the argument does not rest on it.

---

## Part 3 — what must never go on the chain

The chain is permanent, public, and cannot be corrected. So:

| | Where |
|---|---|
| Name, surname, email, phone, document numbers | **Off-chain only**, encrypted |
| User ID (`K3M9-7QRT-B`) | On-chain — resolves to an address and nothing else |
| Balances, payments, votes | On-chain, and permanently public |

`x/alias` was built to hold an address and nothing else precisely so this line
is easy to hold. The temptation once accounts have profiles is to put "just the
email" on-chain for convenience. That is the decision that made PIX's leaks
damaging: the keys were tied to names and document numbers. Ours resolve to a
bech32 string.

**A GDPR consequence worth stating plainly:** personal data written to a chain
cannot be erased on request. Keeping it off-chain is not tidiness, it is what
makes the erasure right satisfiable at all.

---

## Part 4 — the second factor, and where it actually has to sit

Ranked by what they survive:

1. **Passkeys / WebAuthn** — phishing-resistant by construction, and now
   well-supported. This is where to aim.
2. **TOTP authenticator** — the working default. The secret never leaves the
   device and there is no carrier to socially engineer.
3. **Email code** — acceptable as backup. Compromises when the mailbox does,
   which is also where password resets land, so it is one factor wearing two
   hats.
4. **SMS — not as the primary factor.** SIM-swap is not theoretical against a
   payments network; it is the attack, it is cheap, and it is run at scale
   against exactly the people moving money.

**Google sign-in** is fine as an identity provider, and it moves the security
boundary to the Google account — which for most people is stronger than a
password they chose. Under model B it needs care: an OAuth login yields no
password to derive a key from, so either the key wraps under a separate
user-chosen PIN, or the account moves to model A or C. This is the detail that
most often forces a rushed architecture change late.

**Gate the operations, not just the login.** A second factor at sign-in and none
at payment protects the session rather than the money. Require it for: changing
the password, adding a device, rotating the user ID, and any payment over a
threshold.

---

## Part 5 — the part that actually gets robbed

Not the login. **Account recovery.**

Every custodial system's real attack surface is the path that exists for people
who lost their password. It is used rarely, tested less, and it grants exactly
what an attacker wants. Support-desk social engineering is the most reliable
attack on consumer finance in general.

So decide it deliberately, and write it down before building it:

- Who can approve a recovery, and does it need two people?
- Is there a mandatory delay, with notice to the account's email and every
  registered device, so a legitimate owner can cancel it?
- Are payments frozen for a period after a recovery?
- What is the standard of proof, and can it be defeated with information found
  on a social network?

A recovery flow with no delay and one approver is a system where a convincing
phone call is worth every account it holds.

---

## What this changes about the interfaces

The existing self-custody wallet stays — institutions and treasuries need it,
and multi-signature through `x/group` must never reduce to a single credential.

The consumer interface becomes: sign in, see a balance, send to a user ID or a
name from your contacts, confirm with a second factor. No phrase, no address, no
"connect wallet". The transfer app already resolves user IDs and names, so most
of that screen exists; what does not exist is the account service behind it.

**The key is built; the service is not.** `mpc` is the split key and
`tools/custodian` is authentication plus the decision to co-sign — see
[mpc.md](mpc.md) and [custodian.md](custodian.md). What is still needed is an
enrolment path a member of the public can use, an encrypted profile store, a 2FA
enrolment flow, notification, and the recovery process specified in Part 5 —
none of which is on the chain, and all of which is the larger half of the product
from here.

The blind index of Part 2 is worth a specific note. `tools/custodian` implements
it as specified: `HMAC(email, pepper)` with the pepper held outside the store and
refused if it is short or if it sits inside the directory it protects.
`clients/app` does not — it uses a bare `SHA-256` and calls the result a blind
index. That divergence is recorded in
[gaps.md](../scope/gaps.md#the-account-model---the-key-is-built-the-service-around-it-is-not).
