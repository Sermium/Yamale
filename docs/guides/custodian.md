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

**Two deployments, never one.** Enrolment needs three parties and the device is
only one of them, so this binary runs twice — once as the custodian, once as the
recovery holder — with separate sealing keys, separate directories and, if the
deployment is serious, separate operators:

```bash
custodian --role custodian \
          --dir /var/lib/yamale/custodian \
          --sealing-key-file /etc/yamale/custodian.key \
          --pepper-file /etc/yamale/custodian.pepper \
          --listen 127.0.0.1:8099 \
          --second-factor-above 0

custodian --role recovery \
          --dir /var/lib/yamale/recovery \
          --sealing-key-file /etc/yamale/recovery.key \
          --pepper-file /etc/yamale/recovery.pepper \
          --listen 127.0.0.1:8100
```

One process able to hold both shares would be one process able to sign, and the
whole design is the claim that no such process exists. So the store enforces it
rather than trusting the configuration: it is constructed for exactly one role,
refuses to *write* a share of any other, and refuses to *hand back* one it finds
on disk — which is what catches a directory shared by mistake or a backup
restored to the wrong service. `--role device` is refused outright, because the
device's share belongs on the device.

The sealing key and the pepper **must not live in `--dir`**, and the service
refuses to start if either does. That is the whole point of both: a stolen
directory yields ciphertext and a set of unguessable blind indexes, and neither
can be attacked without a file that was never in it.

The reason it is checked rather than documented: *"back up
`/var/lib/yamale/custodian`"* is the one instruction an operator will certainly
follow, and if the sealing key is in there the backup is a plaintext copy of
every share the service holds.

`--import` still exists, for moving an account produced by a ceremony:

```bash
custodian --dir ... --sealing-key-file ... --pepper-file ... \
          --import ./account/custodian.json \
          --import-email person@example.org \
          --import-password '...'
```

It prints the account's bech32 address and exits, and refuses a share that is
not this deployment's role.

## Enrolment

A member of the public creating an account, without any single service ever
holding two shares.

**The device coordinates.** It runs its own key-generation party — in the
browser, via `mpc/wasm` — and speaks to both services. Each service runs its own
party, computes its own share, and transmits none of it. The device is the only
participant that talks to both, and the only one that ever holds its own share.

```
POST /v1/enrol/start     email + password         -> session, first messages
POST /v1/enrol/message   session + one message    -> replies, and whether done
POST /v1/enrol/finish    session + the address    -> committed, or refused
```

`/v1/enrol/message` with no message is a **poll**, and it is not optional: a
round's messages can become available after the response that triggered them was
already written, and a client that never asks again stalls one round short with
nothing logged. Poll until `done` is true **for every party** — the device
knowing its own share says nothing about whether its peers know theirs, and
`/finish` refuses a generation that has not completed.

### What this can and cannot verify

It cannot verify that the person enrolling is who they say. Nobody can, over
HTTP, without a document or an agent, and pretending otherwise would be worse
than saying so. Identity belongs to enrolment policy — an agent network, a
document check, an invitation — and is deliberately not decided here.

What it does verify:

- **The email is not already enrolled.** Checked before anything expensive
  happens *and* again at commit, because two enrolments for one email can begin
  before either finishes. Without it, anybody could enrol over an existing
  account and replace the share that moves its money — which steals nothing and
  destroys the ability to ever move it again. The email is normalised first, so
  case and stray whitespace cannot open a second account for one person.
- **That everybody generated the same key.** The device sends the address *it*
  computed and the service refuses if it differs from its own. This is the only
  check either side gets that one key was generated rather than two, and it is
  what a connection rewriting the protocol fails on. Nothing is written when it
  fails.
- **That the password is long enough.** Twelve characters, counted in runes so
  twelve characters of Amharic is twelve characters. Length only — no
  composition rules, which push people toward `Password1!` and a smaller search
  space than a phrase they can remember. Twelve rather than eight because a
  thief holding the phone already has the device share: the password is not one
  factor among several here, it is the factor.

Nothing is written to disk until all of that holds. A generation that is
abandoned, fails, or is finished with a mismatched address leaves no account
behind — the password verifier is computed at `start` and held in memory until
`finish`.

### The pre-parameter pool

A party's Paillier key needs two safe primes and finding them takes minutes.
Generating them inside the request would give a person a four-minute spinner and
give an attacker an unauthenticated endpoint costing minutes of CPU per call —
a handful of requests pins every core and nobody can enrol or sign.

So they are made in advance, in the background, and enrolment either takes one
from the shelf immediately or is refused immediately with `503`. Both answers
are fast, and *"come back in a minute"* is a far better failure than a service
that becomes slow for everybody.

**Do not make `Take` block when the shelf is empty.** It is the obvious
improvement and it restores the whole problem behind a nicer error message: an
unbounded queue whose length is set by whoever is attacking. `--preparam-pool`
sets how many are kept ready; the default is 4.

## Recovery

The path these systems are actually robbed through, so this is the most
conservative thing in the repository. Support-desk social engineering is the
most reliable attack on consumer finance there is, and it needs no
cryptographic weakness at all.

```
POST /v1/recovery/initiate    operator, team, proof  -> the clock starts
POST /v1/recovery/approve     a second operator, a different team
POST /v1/recovery/complete    after 72 hours and two approvals
POST /v1/recovery/cancel      anybody, any time before completion
GET  /v1/recovery/statistics  counts and durations, never who
```

These are **operator** endpoints, not customer ones, and that asymmetry is
deliberate: a customer cannot recover their own account, because an attacker
holding a customer's email could then do it too. What a customer can do is
**cancel**, which needs no authority at all — the worst a malicious cancellation
achieves is that a real recovery has to start again.

### The five rules, and why each one is load-bearing

**Two approvers, from different teams, neither of them the initiator.** One
approver is one person to deceive. Two from the same team are two people with
one manager and one set of pressures, so the same team as the *initiator* is
excluded too.

**A 72-hour delay.** This is the attacker's real cost: they must keep the owner
unaware for three days. Shorten it and a weekend covers it. It is a constant,
not a setting.

**Notice at initiation — and if notice fails, nothing starts.** The delay
protects an account only if its owner is told the clock is running. A recovery
nobody was notified of is not a slow recovery, it is a silent one. So
`--notify-command` is required, and a service without one **refuses to start a
recovery** rather than running the process correctly and quietly, which is the
single most dangerous configuration this code can be in.

**Outbound payments frozen for 24 hours afterwards.** Recovery restores access,
not immediate spending. Somebody who did get through the process wrongly still
cannot convert it to money that afternoon, and the owner gets another window to
object. This is checked on **every signature**, not at sign-in, so a session
opened before the freeze cannot outlive it.

**A recorded standard of proof.** The service cannot check "they named their
last three payments" — no service can. What it can do is refuse to proceed
without one and make it a thing a named operator put their name to. Not name,
date of birth and address: that is precisely what social engineering runs on.

### What completing a recovery does not do

**It does not re-key anything.** Completion marks the account recoverable and
freezes outbound spending; the actual reshare — `mpc.Reshare`, which produces
new shares under the *same* public key, so the address and the `x/alias`
identifier survive — is driven by the device once the customer is back. Putting
the reshare in here would mean this service generating a share on behalf of
somebody who was not present, which is exactly the property the whole design
refuses.

### Published in aggregate

`GET /v1/recovery/statistics` returns counts, the median hours from initiation
to completion, and how many completed in the last 30 days. No email, no blind
index, no operator name. From the design: an unusual rate should be visible
without exposing who — and a number nobody looks at protects nobody, so it is an
endpoint rather than a log line.

### Notice, and one real constraint

`--notify-command` is run as `<command> <event> <address> [detail]`. A command
rather than built-in SMTP, because every deployment already has a way to send
mail and none of them agree on it — and because a subprocess is auditable by
somebody who does not read Go.

**The command is given the account's chain address, not their email.** This
service stores a blind index and never the address itself, which is what makes a
stolen store much less useful. So the deployment must resolve address to person
its own way. That is a genuine constraint rather than an oversight, and it is
the price of the store not holding a list of everybody's email.

The subprocess does not inherit this process's environment, because this
process's environment is where the paths to the sealing key and the pepper are.

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

## Operating it

`GET /v1/health` is written for whoever is on call, and answers three questions
nothing else in this service would:

```json
{
  "role": "custodian",
  "accounts": 0,
  "preparams_ready": 1,
  "enrolment": "ready",
  "recovery": "disabled: no notify command, and a recovery nobody is told about is worse than none",
  "open_sessions": 0,
  "open_enrolments": 0
}
```

**`enrolment`** is the one to alert on. An empty pre-parameter pool means new
accounts have been refused for as long as it has been empty — and everything
else looks perfectly healthy while it happens. Existing accounts sign, the store
is fine, and the only symptom is people who cannot sign up, which nobody
operating the service ever sees.

**`recovery`** says whether a recovery could run at all. A deployment without
`--notify-command` refuses to start one, by design. Better read on a health
check than discovered the first time somebody needs one.

**`role`** catches the misconfiguration that produces no error: two deployments
both set to `custodian` will happily enrol an account, and it will have no
recovery share, and nothing will say so until the day somebody needs it. The
enrolment client checks this too.

### Two things learned deploying it, so they are not learned twice

**`ReadWritePaths` does not expand `EnvironmentFile` variables.** systemd
resolves the sandbox before the environment file is read, so `${CUSTODIAN_DIR}`
silently becomes nothing and the service starts with its own store read-only. It
then fails at the first write with `read-only file system`, which reads like a
disk fault rather than a unit fault. The unit uses a literal parent path.

**`MemoryDenyWriteExecute` cannot be used**, and it is exactly the flag you want
on a process holding key material. `github.com/cloudwego/base64x`, pulled in
transitively, writes AVX2 dispatch code at init and executes it, so the process
panics with `permission denied` before it opens a socket. The unit says so
rather than simply omitting it — setting it, seeing the crash, and quietly
dropping it is how that becomes folklore.

### Timeouts, which are not one number

Signing is milliseconds of arithmetic. One round of key generation is tens of
seconds of Paillier and range-proof verification, **inside the handler**. Go's
`Server` timeouts are per-connection and global, so a single `WriteTimeout` that
suits signing cuts key generation mid-round — and because the handler has not
failed, nothing is logged and the client sees a bare `EOF` with no status and no
message. That is how the first live enrolment died.

The socket bound therefore fits the slowest legitimate handler, and the fast
routes get a tight bound back individually through `http.TimeoutHandler`, which
answers `503` rather than dropping the connection. A client driving enrolment
needs a matching allowance: 60 seconds reproduces the same symptom from the
other end.

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

- **No identity check at enrolment.** Anybody who can reach the endpoint can
  create an account for any email that has none. That is a policy gap rather
  than a protocol one — see above — but it is the first thing a deployment has
  to answer.
- **Recovery does not reach every enrolled device.** The design says notice goes
  to the registered email *and* every enrolled device; device enrolment does not
  exist, so today it reaches one address. Email is also where password resets
  land, which makes it one factor wearing two hats.
- **The reshare after a recovery is not automated.** Completion restores
  eligibility and freezes spending; the customer's device still has to drive
  `mpc.Reshare` to get working shares back. Deliberate — see above — but it
  means a recovery is not finished when the endpoint says completed.
- **No second factor.** The `second_factor` field is a placeholder, there is no
  enrolment, and above the threshold the service refuses rather than proceeds.
- **No notification of a SIGNATURE.** Recovery notifies; ordinary signing does
  not. Nothing tells a person their account signed something, or that somebody
  tried, and it is the cheapest defence against a stolen device there is.
- **No rate limit and no lockout.** Argon2id makes each guess cost about 90 ms
  and that is all that stands between an attacker and an online password
  attack.
- **It cannot refuse a payment.** It signs 32 opaque bytes. It refuses the
  signer; it cannot refuse the transaction.
- **No transport security of its own.** It listens on loopback by default and
  expects to sit behind something that terminates TLS. It authenticates the
  account, not the network.
- **No notification of an enrolment either.** Somebody enrolling an email that
  its owner has not yet used will not be noticed by that owner.

## Where else to read

- [mpc.md](mpc.md) — the protocol, the three shares, and what has been verified
  on the live chain.
- [accounts.md](accounts.md) — the design this is a fragment of, including
  everything above that is unbuilt.
- [key-ceremony.md](key-ceremony.md) — the foundation's 3-of-5, which holds the
  third share and is deliberately not the operator's.
