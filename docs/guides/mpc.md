# A key that exists in three shares and nowhere else

How a consumer account on Yamale is held so that the operator of the network
cannot move the money in it — not as a policy, as arithmetic.

**Status: the protocol is built and has signed on the live chain.** The account
service around it is not. Read [what is not built](#what-is-not-built) before
believing anything here is a product.

**You need:** Go, and for the last section a funded account on a running chain.

**You will end with:** an account whose key never existed anywhere as a whole
key, a payment it signed, and the same account paying again after a password
reset that did not move its address.

---

## The fact everything else follows from

> A password cannot authorise a transaction. Only a key can.

[accounts.md](accounts.md) is the design document, and it works through the three
places a key can live. The short version, because the rest of this page is the
answer rather than the argument:

- **Server-held.** Simplest, recoverable, and the operator can spend any
  citizen's balance. On a state-operated payments network that is a different
  political object from "the authority runs the rails", and the second is the
  one being sold. Rejected.
- **Password-wrapped on the device.** The server never holds a usable key, and a
  forgotten password is a permanently lost account. Not viable for a national
  system serving people who lose phones. Rejected.
- **Split.** No whole key exists, and a password reset is possible without any
  party ever assembling one. Chosen, on 2026-08-20.

So the key is split three ways and any two shares sign.

| Share | Where it lives | Wrapped by |
|---|---|---|
| `device` | the customer's own hardware | their password |
| `custodian` | the operator's service — [custodian.md](custodian.md) | a sealing key held outside the store |
| `recovery` | the foundation's offline custody: the same 3-of-5 `x/group` that `x/constitution` pins as the seizure destination | the [key ceremony](key-ceremony.md) |

Ordinary signing is **device + custodian**. Recovery is **recovery + custodian**,
and only after the workflow in Part 5 of [accounts.md](accounts.md).

## What it refuses

| | |
|---|---|
| **One share signing** | `Sign` refuses fewer than two, and `NewSigningParty` refuses a committee that is not exactly two. The refusal is the product, not an error path. |
| **The operator signing alone** | The operator holds `custodian`. Two are needed and it has one. |
| **The operator holding two** | `recovery` is deliberately not the operator's. An operator holding both custodian and recovery is a custodian again, with an extra step. |
| **A share travelling** | Only protocol messages move. A `Share` is the thing that is sealed and stored, and the thing that is never transmitted. |
| **Inferring who signs** | `Sign` takes the signers by name. A function that quietly used whatever shares it could reach is one that signs with custodian + recovery on the day somebody leaves both on one machine — which is the arrangement the whole design exists to prevent. |
| **A high `S` value** | Normalised to the lower half of the curve order. A signature with two valid encodings is a transaction with two hashes. |
| **A reshare that moves the key** | Asserted, not assumed. A reshare that changed the public key would silently orphan every coin in the account. |

## Why three shares and not two

Two — device and custodian — gives the security property and nothing else. If
the password is gone the device share is gone, and the account is dead with the
money in it.

The specification requires that a password can be reset **without any party ever
reconstructing a whole key**. Two shares cannot do that: the two survivors of a
lost device share are one, and one is below the threshold.

Three shares, any two of which sign, is the smallest arrangement that gives both
properties at once.

## Why threshold ECDSA rather than a Cosmos multisig

This is the decision most worth arguing, because a 2-of-3 `x/group` or legacy
multisig account gives the same "no single party signs" property with no new
cryptography, and it was the first thing considered. Two things ruled it out.

**The address moves.** A multisig account's address is derived from the set of
member keys, so rotating a share changes the address — and rotating a share is
exactly what a password reset is. On this chain an address carries the account's
`x/alias` identifier, so changing it retires that identifier: the handle a person
has given to everyone who pays them stops resolving, and every saved payee
silently breaks. Threshold ECDSA re-shares under the **same public key**, so a
reset is invisible to everybody else.

**The consumer base becomes countable.** Every consumer account would carry the
operator's share pubkey inside its multisig, on a public chain, making the whole
customer base trivially enumerable and linkable by a stranger. A threshold
signature is an ordinary secp256k1 signature, and an account holding one is
indistinguishable from an account whose owner keeps a seed phrase in a drawer.

That second property is what `mpc/cosmos` exists to deliver: `CosmosPubKey`
returns a plain `secp256k1.PubKey`. Same address format, same ante-handler
verification, same signature. Nothing about the account announces what it is.

## The pieces

```
mpc/                the protocol. No authentication, no transport, no policy.
  mpc.go            Keygen, Sign, and the refusals
  party.go          SigningParty — ONE participant, holding one share
  reshare.go        Reshare — every share replaced, same public key
  cosmos/address.go the joint key as a Cosmos pubkey and a bech32 address
  wasm/main.go      the device's half, compiled for the browser
tools/mpc           the CLI: keygen, address, sign, reshare, pay
tools/custodian     the service that holds one share and decides whether to use it
clients/keys        the page that runs the protocol in front of you
```

**`mpc` is the protocol, not the service.** It performs no authentication,
decides nothing about who may hold a share, and moves no message between
parties. The caller supplies the transport, because the transport is where the
authentication lives and this package must not be the place that gets it wrong.

### `Sign` and `SigningParty` are not the same thing, and the difference is the security model

`Sign` runs every party in one process. That is correct for a test and
catastrophic anywhere else: a browser calling it would hold the device share
**and** the custodian share, which is a whole key in all but name, and "the
operator cannot move your money" would be false the moment anybody looked.

`SigningParty` is the type production uses. It holds one share, never sees
another, emits protocol messages and consumes the ones addressed to it. The
caller carries the bytes — over HTTPS to the custodian, over a QR code to an
air-gapped recovery holder, however the deployment does it.

An earlier WebAssembly build exposed `sign(digest, shares)`. It worked. It is
the reason `SigningParty` exists, and the file that made the mistake says so in
its own header.

### Pre-parameters, and why signing up does not mean watching a phone think

Key generation's expensive half is a safe-prime search for each party's Paillier
key — minutes per party, depending on nothing: not the other parties, not the
account, not the user. `GeneratePreParams` splits it out so a custodian can keep
a pool generated in advance. It is the single biggest reason a naive threshold
implementation feels unusable.

## Resharing: a password reset that does not move the account

The device share is wrapped by the user's password, so a forgotten password
destroys it. `recovery` and `custodian` — the other two — reshare, and the device
is handed a new share wrapped under the new password.

The old share is not revoked so much as made **arithmetically useless**: it
belongs to a sharing that no longer reconstructs anything.

What resharing does not do is change the account. The address, the public key
and the `x/alias` identifier are untouched, and nobody who has ever paid this
person needs to be told anything.

It is also proactive security independent of any incident. An attacker who has
patiently obtained one share and is waiting for a second has to start again after
every reshare, because shares from two different sharings do not combine. Run on
a schedule, it turns "one share stolen" from a permanent half-compromise into a
race with a deadline.

**Two things cost real time to learn and are worth carrying:**

- The old peer context must contain **only the parties actually taking part**,
  while the party count passed alongside it is the **original sharing's size**.
  Build the context from all three when two are present and the incoming
  committee waits forever for a third sender: no error, no timeout, a goroutine
  in `select`.
- `tss-lib` retires an outgoing party by zeroing its `Xi` **in place**, and the
  save data is full of pointers. Handing it the caller's share destroys the
  caller's share. `Reshare` deep-copies. A reshare that died halfway without that
  copy would leave the custodian holding a gutted share, and an account whose
  remaining shares cannot reach the threshold is an account nobody can ever spend
  from again.

## Verified on the live chain

Two payments from one threshold account on `yamale-devnet-2`, read back from the
node on 2026-08-31 rather than from a test log.

| | Height | Transaction | Code | Amount | Memo |
|---|---|---|---|---|---|
| Before the reset | 118,885 | `A8F18CAB…B15C3C` | 0 | 1,250,000 `uyml` | `threshold signed: device + custodian` |
| After the reset | 118,968 | `6C784D06…4AFCFE` | 0 | 750,000 `uyml` | `after a password reset: new shares, same address` |

Both are `/cosmos.bank.v1beta1.MsgSend` from
`yml1ael7jxwlvacc3daawzc2kpd6lst6w8nmml6a97`, sequence 0 and sequence 1.

**The claim that matters is the public key.** Both transactions carry the same
compressed secp256k1 key, byte for byte:

```
0320ddc3fc254d0c20994c0d9c59e42fb7b53a03d871f2a25909efd083c82a3c34
```

which derives to `yml1ael7jxwlvacc3daawzc2kpd6lst6w8nmml6a97`. The second
payment was signed by shares that did not exist when the first was signed, and
the account is the same account: same address, same alias, same saved payee for
anybody who had stored it. That is the property a multisig could not have given,
and it is checkable by anybody against a block explorer.

Two honest limits on that demonstration. The chain sees an ordinary
single-signature account and cannot tell you the signature was produced
jointly — that indistinguishability is the point, and it is also why the
transaction alone is not proof of the arrangement. And the shares in this
rehearsal were files on one machine, driven by `tools/mpc`, which is exactly the
arrangement the design exists to avoid; the CLI's own header says so.

## The CLI

```
mpc keygen  --out DIR                          create an account, three shares
mpc address --share FILE                       the account a share belongs to
mpc sign    --digest B64 --share A --share B   a signature from any two
mpc reshare --share A --share B --out DIR      rotate; the address does not move
mpc pay     --shares DIR --to ADDR --amount X  a real payment on a real chain
```

`pay` is the one that matters. Everything above it can be satisfied by a library
that is subtly wrong — a signature over the wrong bytes, a public key encoded the
way this codebase does not expect, an `S` value in the upper half of the curve —
and all of those look identical to a caller checking that `Sign` returned no
error. A chain either accepts the transaction or it does not.

It signs with **exactly two** shares and refuses more: *"more is not safer, it is
just more shares in one place"*.

```bash
go run ./tools/mpc keygen --out ./account
go run ./tools/mpc address --share ./account/device.json
go run ./tools/mpc pay --shares ./account \
  --to yml1... --amount 1250000uyml \
  --node http://localhost:26657 --chain-id yamale-devnet-2
```

`tools/mpc` was written because `mpc` had no caller at all, which is the same
shape as the two dead load-bearing functions found in `x/tokenisation` on
2026-08-27 — code that was written, commented, and invoked by nothing. See
[tokenisation.md](tokenisation.md) for what that cost.

## In the browser

`mpc/wasm` is the device's half compiled for WebAssembly, because the device
share must be **generated on the device and never leave it**. A share generated
server-side and sent down was known to the server for a moment, and "the operator
cannot move your money" stops being true of that account forever.

It exposes one object, `yamaleMPC`, and every call is local:

```
yamaleMPC.startSign(digestB64, shareJSON, signersCSV) -> {session, outbound}
yamaleMPC.handle(session, outboundJSON)               -> {outbound}
yamaleMPC.outbound(session)                           -> {outbound}
yamaleMPC.signature(session)                          -> {signature} | {pending}
yamaleMPC.publicKey(session)                          -> {x, y}
yamaleMPC.close(session)
```

Sessions are held in Go, not handed to JavaScript, so a share and a half-finished
protocol state never cross into a heap the page can read. JavaScript gets an
opaque handle.

Two things it deliberately does not do. It **does not store anything** — sealing
the share under the user's password belongs to the application, and a crypto
module that reached for `localStorage` is one nobody can test. It **does not do
key generation** — that is three parties and minutes of safe primes, and it needs
the custodian's pre-parameter pool and a session that survives a page reload.

`mpc/cosmos` is a separate package for one blunt reason: importing `cosmos-sdk`
drags in the whole store stack, none of which compiles for `GOOS=js`. Keeping the
protocol free of it is what makes the WebAssembly build possible at all.

`clients/keys` — live at `/keys/` — runs this in front of an audience. It is
explicit about what is real and what is staged: the account and its transactions
are real and checkable, while the live demonstration runs **both** parties inside
one page, which is precisely the arrangement the design forbids, so it uses a
throwaway account holding nothing whose share files are published on purpose.

## What is not built

The credibility of this page depends on this section being accurate.

- **No enrolment.** An account is created by `tools/mpc keygen` on one machine
  and imported into the custodian by an operator. There is no path by which a
  member of the public gets an account, and the ceremony arrangement is the thing
  the design exists to avoid.
- **No recovery workflow.** `Reshare` is the mechanism. The two approvers from
  different teams, the 72-hour delay, the notice to email and every enrolled
  device, the 24-hour outbound freeze afterwards and the standard of proof — all
  of Part 5 of [accounts.md](accounts.md) — are specified and none of it exists.
  `Reshare` decides nothing about authorisation; by the time it is called that
  question is already answered by something that has not been written.
- **No second factor.** The custodian refuses above its threshold rather than
  waving the request through, which is the right failure, but enrolment does not
  exist so the refusal is currently absolute.
- **No distributed key generation.** `Keygen` runs all three parties in one
  process. That is correct for a ceremony on one machine and wrong for
  production, where the device's share must be generated on the device.
  `SigningParty` has no keygen counterpart yet.
- **No notification.** Nothing tells a person that their account signed
  something, which is the cheapest defence against a stolen device that exists.
- **The device share's wrapping is the application's job** and no application
  does it yet. `clients/app` still ships the model the design rejected: a CosmJS
  password-wrapped key in `localStorage`, with a forgotten password a lost
  account.

## Where else to read

- [accounts.md](accounts.md) — the design document. This page is what was built
  from it; that one is the argument, the recovery specification, and the parts
  that are still only a specification.
- [custodian.md](custodian.md) — the service holding the second share, and what
  it refuses.
- [key-ceremony.md](key-ceremony.md) — the 3-of-5 that holds the recovery share.
- [wallet.md](wallet.md) — self-custody, which is unchanged. Institutions and
  treasuries keep their own keys and their `x/group` multi-signature; a single
  credential must never move institutional funds.
