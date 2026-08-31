# The custodian: one share, and the decision to use it

The service that holds the operator's share of a consumer account. What it does,
what it refuses, and — the longer list — what it deliberately does not do.

**Status: the service is built and is not an account service.** There is no
enrolment over HTTP, no recovery workflow and no second factor. Read
[what it does not do](#what-it-does-not-do) before deploying it anywhere near a
person's money.

**Read first:** [mpc.md](mpc.md), for the three shares and why any two sign.

**Not to be confused with [custody.md](custody.md)**, which is `x/custody` —
issuing on-chain claims against outside assets somebody else is holding. This
page is `tools/custodian`, a service holding one share of a consumer key. The two
share a word and nothing else.

---

## What it is actually protecting

Not the share.

The share alone is worthless, and saying so is not modesty — it is the property
that makes the rest tractable. One share signs nothing. A breach of this service
that took every share it holds would not yield a single signing key.

What the custodian protects is the **decision**. A person's phone can be stolen,
and the thief then holds a device share and needs exactly one further thing to
empty the account: a custodian willing to co-sign. A custodian that co-signs for
whoever asks has turned a two-of-three into a one-of-one held by the attacker.

Everything below exists because of that sentence. The endpoints are deliberately
boring and the refusals are the product.

## What it refuses

| | |
|---|---|
| **Signing alone** | It holds one share of three. This is arithmetic, not configuration. |
| **Importing any share but the custodian's** | *"importing any other would give this service two, and two shares sign"*. |
| **Starting with its secrets inside the store** | The sealing key and the pepper must live outside `--dir`, or the service exits. |
| **A world-readable secret** | A key file with any group or other permission bit set is refused: `chmod 600 it`. |
| **A pepper under 32 bytes** | A pepper an attacker can guess is a bare hash, which is the defect this replaces. |
| **A password under 10 characters** | A length floor, and no composition rules — those measurably push people towards `Passw0rd!` and away from length. |
| **Saying whether an account exists** | Wrong password and no such account are the same refusal, with the same status and the same wording, and the unknown case still spends the Argon2 time so the timing does not leak what the wording would not. |
| **A second signature while one is in flight** | One open session per account, and the answer is `busy` rather than a queue. Concurrency here is the difference between paying somebody and paying them twice. |
| **A frozen account** | A freeze stops co-signing entirely, and it is what a reported theft turns on. |
| **A stale session** | Two minutes. An abandoned session holds a party mid-protocol with the share loaded, and a service accumulating those is one whose memory is a pile of live key material waiting for a heap dump. |
| **Replaying a finished session** | Once a signature exists the session is done and closed, on the happy path too. |
| **A message claiming to be from itself** | The only peer in a signing committee is the device. Anything else is a bug or somebody probing. |
| **An amount over the second-factor threshold** | Refused outright, *because* second-factor enrolment is not built. A service that skips a check because the feature behind it is unfinished is a service that ships with the check missing. |

## Running it

```bash
custodian --dir /var/lib/yamale/custodian \
          --sealing-key-file /etc/yamale/custodian.key \
          --pepper-file /etc/yamale/custodian.pepper \
          --listen 127.0.0.1:8099 \
          --second-factor-above 0
```

The sealing key and the pepper **must not live in `--dir`**, and the service
refuses to start if either does. That is the whole point of both: a stolen
directory yields ciphertext and a set of unguessable blind indexes, and neither
can be attacked without a file that was never in it.

The reason it is checked rather than documented: *"back up
`/var/lib/yamale/custodian`"* is the one instruction an operator will certainly
follow, and if the sealing key is in there the backup is a plaintext copy of
every share the service holds.

Enrolment is an operator action, not an endpoint:

```bash
custodian --dir ... --sealing-key-file ... --pepper-file ... \
          --import ./account/custodian.json \
          --import-email person@example.org \
          --import-password '...'
```

It prints the account's bech32 address and exits.

## The endpoints

| | |
|---|---|
| `POST /v1/sign/start` | authenticate, open a session, return the custodian's first protocol messages |
| `POST /v1/sign/message` | feed one device message in, get the custodian's replies |
| `POST /v1/sign/result` | the signature, or `{"pending": true}` |
| `POST /v1/freeze` | stop this account co-signing |
| `GET /v1/health` | account count, open sessions, uptime |

Five, and one of them is a health check. The service is small on purpose.

`/v1/health` reports what it holds as a sentence rather than a number: *"one
share per account, which signs nothing alone"*.

### It signs 32 bytes it cannot read

`/v1/sign/start` takes a digest. The custodian does not build the transaction and
does not parse it, and it has no way to recover what those bytes commit to.

**This is a limitation and not a design win**, and it is stated that way in the
code. A custodian that cannot read what it signs cannot refuse a payment on its
merits — it can only refuse the **signer**. It can tell a wrong password from a
right one and a frozen account from a live one; it cannot tell a rent payment
from an account being drained.

The amount is passed alongside the digest, for the second-factor rule, and it is
**asserted by the caller** — which means a thief asserts zero. That is named
honestly in the request type rather than dressed up. Making the amount visible to
the custodian, so the threshold applies to something the custodian can verify, is
the next piece and it is not built.

### The freeze is judged differently from signing, on purpose

`/v1/freeze` needs only the password. That is a deliberately different bar from
signing, and the reasoning is worth keeping: somebody reporting a stolen phone is
often distressed, sometimes on a borrowed device, and rarely able to produce a
second factor that was on the phone.

Making a freeze hard to request protects nobody. The cost of a false freeze is an
inconvenience; the cost of a slow one is the account.

A freeze is not a deletion. An account that vanishes takes its own audit trail
with it.

### The second factor is demanded at signing, not at login

The rule most systems get wrong: gating the **login** and not the **payment**
protects the session rather than the money. An attacker holding the password and
the device share does not need to log in again — they need to sign.

So the threshold is on the amount, at the moment of signing, and not on the
session's age. `--second-factor-above 0` means every signature needs one, which
is a legitimate setting for an institution and a miserable one for somebody
buying bread.

## What a breach yields

Stated first because it is the question an auditor asks first.

| What leaks | What the attacker gets |
|---|---|
| The store alone | Ciphertext, and a set of blind indexes. No email addresses, no names, no signing key. |
| The store **and** the sealing key | Every custodian share. Still not a key: they then need a device share per account, one at a time, from the people holding them. |
| The store **and** the pepper | The ability to test a guess at which email addresses are enrolled. |

Encrypting at rest is not the last line of defence here — the threshold is. What
it buys is that a stolen disk is not also a stolen queue of half-compromised
accounts.

The `Account` record is worth reading for what is **not** in it: no name, no
email, no phone, no document number. The custodian needs to find an account and
decide whether to co-sign; it does not need to know who anybody is, and personal
data it never holds is data it cannot leak or be compelled to produce.

Passwords are **hashed, never encrypted** — Argon2id, per-user salt, tuned to
roughly 90 ms rather than copied from an example. Encryption is reversible, so a
breach that takes the key takes every password, and people reuse passwords, so
that breach spends beyond this system entirely.

Verification is constant-time. A comparison that returns early tells an attacker
how many leading bytes they got right, and enough of those answers is the hash.

## The blind index, and the defect this fixes

An account is found by email without the email being stored. The wrong answers,
in the order people reach for them:

1. **Store the email.** A dump is then a mailing list of everybody who uses a
   national payments system, which is a different kind of disaster from a
   financial one.
2. **Encrypt it deterministically.** Equal emails produce equal ciphertext, so
   the column is a frequency-analysable index of the same thing.
3. **Hash it.** This is the one that feels safe and is not. Email addresses are
   low-entropy and enumerable, so an attacker with a list and a CPU tests every
   guess offline and recovers exactly the membership the hash was meant to hide.

The answer is a keyed hash whose **pepper lives outside the store**. A dump alone
then yields neither the addresses nor any way to test a guess at them, because
testing requires a file that was never in the thing that leaked.

**`clients/app` does number 3 today.** `emailKey()` is a bare `SHA-256` of the
address and the code calls the result a blind index; it is not one. Local-only
storage makes it less severe than the same mistake server-side — it does not make
the claim true. The custodian is the first place in this codebase that does it
correctly, and the divergence is recorded in
[gaps.md](../scope/gaps.md#known-defects).

Emails are lower-cased and trimmed before hashing. `Alice@Example.COM` and
`alice@example.com` are one person and two keys, and the second one silently
becomes an account nobody can sign in to.

## What it does not do

This list is longer than the feature list, and that is the honest shape of it.

- **No enrolment over HTTP.** An account is enrolled by an operator running
  `--import` against a share file produced by the ceremony. That is defensible
  for a service holding nobody's money and would not be acceptable once it does.
- **No recovery workflow.** `mpc.Reshare` is the mechanism and nothing calls it
  here. Part 5 of [accounts.md](accounts.md) — two approvers from different
  teams, a 72-hour delay, notice to the registered email and every enrolled
  device, outbound payments frozen for 24 hours afterwards, and a standard of
  proof that is not public knowledge — is specified and unbuilt. That is the path
  these systems are actually robbed through.
- **No second factor.** The `second_factor` field is a placeholder, there is no
  enrolment, and above the threshold the service refuses rather than proceeds.
- **No notification.** Nothing tells a person their account signed something,
  or that somebody tried. It is the cheapest defence against a stolen device
  there is.
- **No rate limit and no lockout.** Argon2id makes each guess cost about 90 ms
  and that is all that stands between an attacker and an online password
  attack.
- **It cannot refuse a payment.** It signs 32 opaque bytes. It refuses the
  signer; it cannot refuse the transaction.
- **No transport security of its own.** It listens on loopback by default and
  expects to sit behind something that terminates TLS. It authenticates the
  account, not the network.
- **No pre-parameter pool.** [mpc.md](mpc.md) explains why one is needed for
  enrolment and reset to be seconds rather than minutes. It is not built, and it
  does not bite yet because neither enrolment nor reset runs here.

## Where else to read

- [mpc.md](mpc.md) — the protocol, the three shares, and what has been verified
  on the live chain.
- [accounts.md](accounts.md) — the design this is a fragment of, including
  everything above that is unbuilt.
- [key-ceremony.md](key-ceremony.md) — the foundation's 3-of-5, which holds the
  third share and is deliberately not the operator's.
