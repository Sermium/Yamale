# The custodian: one part, and the decision to release it

The service that holds one of the three parts of a consumer account's sealed key.
What it does, what it refuses, and — the longer list — what it deliberately does
not do.

**Status: built, tested end to end, and not deployed anywhere.** Enrolment over
HTTP and the recovery workflow both exist; an end-to-end test enrols an account
against two live services, destroys the device's part, drives a full recovery
through both and signs with the rebuilt key. No member of the public has ever
used it. Read [what it does not do](#what-it-does-not-do) before deploying it
anywhere near a person's money.

**It does not co-sign anything.** Since the decision of 2026-09-18 the device
signs alone. This service exists for recoveries, and every section below that
talks about refusing is talking about refusing a recovery.

**Read first:** [recovery.md](recovery.md), for the three parts and why any two recover.

**Not to be confused with [custody.md](custody.md)**, which is `x/custody` —
issuing on-chain claims against outside assets somebody else is holding. This
page is `tools/custodian`, a service holding one part of a sealed consumer key.
The two share a word and nothing else.

---

## What it is actually protecting

Not the share.

The share alone is worthless, and saying so is not modesty — it is the property
that makes the rest tractable. One share signs nothing. A breach of this service
that took every share it holds would not yield a single signing key.

What the custodian protects is the **decision** — to release its part, so that
the account holder can rebuild their key on a new device. Somebody who has taken
over an email address wants exactly that, and a custodian that releases for
whoever asks has handed them the account.

It no longer protects a payment. A thief holding the phone and the password does
not need this service at all, and nothing here can stop them; see
[it no longer sees a payment at all](#it-no-longer-sees-a-payment-at-all).

Everything below exists because of that sentence. The endpoints are deliberately
boring and the refusals are the product.

## What it refuses

| | |
|---|---|
| **Opening what it holds** | Its part is a share of a ciphertext sealed under a password it has never seen. This is arithmetic, not configuration. |
| **Importing any part but its own** | *"one service holding two of the three is one service that can attempt a recovery on its own"*. |
| **Starting with its secrets inside the store** | The sealing key and the pepper must live outside `--dir`, or the service exits. |
| **A world-readable secret** | A key file with any group or other permission bit set is refused: `chmod 600 it`. |
| **A pepper under 32 bytes** | A pepper an attacker can guess is a bare hash, which is the defect this replaces. |
| **A password under 10 characters** | A length floor, and no composition rules — those measurably push people towards `Passw0rd!` and away from length. |
| **Saying whether an account exists** | Wrong password and no such account are the same refusal, with the same status and the same wording, and the unknown case still spends the Argon2 time so the timing does not leak what the wording would not. |
| **A frozen account** | A freeze blocks a recovery outright, and it is what a reported theft turns on. |
| **A deposit that is not signed by the key it names** | The signature covers the holding, the account and this service's role, so nothing in the middle can substitute any of them. |
| **A holding meant for the other service** | The device posts a different part to each. One that posted the same to both would leave the account recoverable from one breach and unrecoverable from the intended pair — and neither symptom appears until somebody tries. |
| **Enrolling over an existing account** | Checked before the expensive work and again after it, because two deposits for one email can both pass the first check. |
| **Releasing a part before the waiting period** | 72 hours of notice, two approvals from different teams, then a further 24 hours in which the account holder can still cancel. |
| **A recovery nobody was told about** | Without `--notify-command` the service refuses to start one at all. A process running correctly and silently is the worst configuration this can be in. |

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
          --notify-command /usr/local/bin/yamale-notify

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

Write each secret as **hex**, on the host that will use it, so it never
crosses a network:

```bash
umask 077
openssl rand -hex 32 > /etc/yamale/custodian.key
openssl rand -hex 32 > /etc/yamale/custodian.pepper
```

A file of exactly 32 raw bytes is also accepted and used exactly as stored, so
keys written that way keep working. What is refused is anything in between — a
typed secret plus an editor's newline, say — because it cannot be told apart
from a random key that happens to end in a newline byte, and the earlier habit
of trimming it made about one correctly generated raw key in twenty come out a
byte short.

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
holding two of the three parts.

**The device does all of it.** It generates an ordinary secp256k1 key, seals it
under the account holder's password, splits the sealed blob three ways, keeps one
part and posts one to each service. It is the only participant that speaks to
both, and the only one that ever holds its own part.

```
POST /v1/enrol   email + password + pubkey + holding + signature -> address
```

One request, no session, no state between calls. There is no longer any such
thing as an abandoned enrolment: a deposit either commits or leaves nothing
behind, so there is nothing to reap and no sweeper that can quietly stop running.

This replaced three endpoints, a ten-minute session TTL, a pool of pre-computed
safe primes, and about nine and a half minutes of a phone's CPU.

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

### The expensive step is Argon2id, and it runs last

There is no pre-parameter pool any more — no Paillier key, no safe primes,
nothing to pre-compute. What enrolment costs this service is one Argon2id
derivation at 128 MiB, for the password verifier.

The order of checks is deliberate and every cheap one runs first: the
already-enrolled check and the deposit signature are both verified **before**
Argon2id is asked to do any work. An unauthenticated caller cannot spend this
service's memory by posting rubbish.

The rate limit in front of it is what bounds the rest; see
`deploy/nginx/yamale-account.conf`.

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

**It does not touch what it stores.** Completion marks a decision and starts a
clock; the part it later releases is the same bytes that were deposited at
enrolment. It must be — a part that changed would rebuild a ciphertext that does
not open, and the account holder has no way to tell that from having mistyped
their password.

**It does not re-key anything either.** The key that comes back is the key that
went in, so the address and the `x/alias` identifier survive untouched. Re-keying
here would mean this service generating a key on behalf of
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
| `POST /v1/enrol` | verify a deposit and store one part |
| `POST /v1/recovery/initiate` | operator, **with the account's email**; the 72-hour clock starts |
| `POST /v1/recovery/approve` | a second operator, from a different team |
| `POST /v1/recovery/complete` | **with the email again**, after the notice period and two approvals |
| `POST /v1/recovery/cancel` | anybody, until the part is actually released |
| `POST /v1/recovery/claim` | **customer**; email + password -> this service's part |
| `POST /v1/freeze` | mark an account as reported stolen |
| `GET /v1/health` | account count, uptime, whether recovery can run |
| `POST /v1/sign/*` | `410 Gone`, with the reason |

`/v1/health` reports what it holds as a sentence rather than a number: *"one
holding per account: a share of a ciphertext this service cannot open"*.

The `/v1/sign/*` routes answer `410` rather than `404` on purpose. A client built
against the old exchange is in somebody's browser cache, and the difference
between *"this service is broken"* and *"this client is too old"* costs an
afternoon to work out from a `404`.

### Only one endpoint emits key material

`/v1/recovery/claim`, and it is a **customer** endpoint while the rest of the
recovery flow is an operator one. That split is the point: an operator who drives
a recovery end to end still finishes holding nothing they did not start with.

The password is required there — not to authorise the recovery, which the notice
period and two approvals did, but because the holder needs it anyway to open the
seal, so demanding it costs them nothing and stops the endpoint handing a
ciphertext part to anybody who learned a recovery identifier.

### It no longer sees a payment at all

This used to sign a 32-byte digest it could not read, which was a stated
limitation: a custodian that cannot read what it signs can refuse the **signer**
but never the payment.

That limitation is now total. The device signs alone, so this service does not
see a payment before, during or after — and the honest consequence is worth
writing down rather than leaving for somebody to discover:

**This service can no longer stop a payment, and a frozen account can still
spend.** Nothing in this codebase can stop a thief who holds both the phone and
the password. That is the shape of the architecture rather than an oversight in
it, and it is the price of a deal being between a buyer and a seller.

### The freeze is judged leniently, on purpose

`/v1/freeze` needs only the password, and the reasoning is worth keeping:
somebody reporting a stolen phone is often distressed, sometimes on a borrowed
device, and rarely able to produce a second factor that was on the phone.

What a freeze now does is **block a recovery**, which is narrower than what it
used to do and is still the right thing to stop: somebody who has taken over an
email address uses a recovery to move the account to a device they hold.

Making a freeze hard to request protects nobody. The cost of a false freeze is an
inconvenience; the cost of a slow one is the account.

A freeze is not a deletion. An account that vanishes takes its own audit trail
with it.

### The second factor moved to recovery, because that is where this service still is

The rule most systems get wrong is gating the **login** rather than the action:
an attacker holding the password does not need to log in again.

That rule was right and this service can no longer apply it to a payment, because
it never sees one. `--second-factor-above` is gone with the signing path. What
remains is `factors.go`, which decides what a **recovery** must produce — a
passkey if one is registered, else a TOTP code, else one sent by email — and
that is the moment this service is still present at.

The endpoints to enrol those factors are **not built**, and neither is a mailer.

## Operating it

`GET /v1/health` is written for whoever is on call, and answers three questions
nothing else in this service would:

```json
{
  "role": "custodian",
  "accounts": 0,
  "uptime_seconds": 41,
  "holds": "one holding per account: a share of a ciphertext this service cannot open",
  "enrolment": "ready",
  "recovery": "disabled: no notify command, and a recovery nobody is told about is worse than none"
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
| The store **and** the sealing key | Every part this service holds. Still not a key, and not even a ciphertext: one part of a 2-of-3 is information-theoretically nothing. |
| **Both services**, and both sealing keys | A ciphertext per account, and the ability to guess at passwords offline. This is the case the seal exists for, and the only one where the password is what stands there. |
| The store **and** the pepper | The ability to test a guess at which email addresses are enrolled. |

Encrypting at rest is not the last line of defence here — the seal is, and
before that the fact that one part alone is nothing. What encrypting at rest buys
is that a stolen disk is not also a stolen queue of half-compromised accounts.

The `Account` record is worth reading for what is **not** in it: no name, no
email, no phone, no document number. The custodian needs to find an account and
decide whether to release its part; it does not need to know who anybody is, and
personal data it never holds is data it cannot leak or be compelled to produce.

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
  exist, so today it would reach one address. Email is also where password
  resets land, which makes it one factor wearing two hats.
- **There is no mailer.** `--notify-command` names a program and no such program
  is written, so a recovery refuses to start. That is the correct default and it
  means recovery is off wherever it has not been configured, which is
  everywhere. What the command is *given* is now correct — a JSON notice on
  stdin carrying the address and the proved email — so writing one is a small
  job rather than a blocked one.
- **Second-factor enrolment is not built.** `factors.go` decides what a recovery
  must produce — a passkey, else TOTP, else an emailed code — and there is no
  endpoint to register any of them, so the ladder falls through to the factor
  that needs the mailer above.
- **It cannot refuse a payment, at all.** It does not see one. This is not a
  gap to be closed later; it is what the architecture decided. Nothing here can
  stop a thief holding both the phone and the password.
- **Nothing notifies a person that their account signed something**, or that
  somebody tried. It is the cheapest defence against a stolen device there is,
  and with no service in the signing path there is now nowhere in this codebase
  to put it — it has to come from the chain or from the device.
- **Both services are run by the same operator today.** That is exactly the case
  sealing before splitting exists for. It is also the case in which a breach of
  both exposes the account to offline password guessing, which the scheme this
  replaced did not allow.
- **No rate limit and no lockout of its own.** Argon2id at 128 MiB makes each
  guess expensive and that is all that stands between an attacker and an online
  password attack; `deploy/nginx/yamale-account.conf` is where the limit lives
  and it is not installed.
- **Accounts enrolled under the old scheme cannot be read.** They would have to
  enrol again. A migration is possible in principle and is not built.
- **No transport security of its own.** It listens on loopback by default and
  expects to sit behind something that terminates TLS. It authenticates the
  account, not the network.
- **No notification of an enrolment either.** Somebody enrolling an email that
  its owner has not yet used will not be noticed by that owner.

## Where else to read

- [recovery.md](recovery.md) — the scheme, the three parts, and what has been verified
  on the live chain.
- [accounts.md](accounts.md) — the design this is a fragment of, including
  everything above that is unbuilt.
- [key-ceremony.md](key-ceremony.md) — the foundation's 3-of-5, which holds the
  third share and is deliberately not the operator's.
