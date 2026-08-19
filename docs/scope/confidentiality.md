# Payment confidentiality

**Decided:** confidential amounts, using Pedersen commitments and range proofs,
with viewing keys granted to the parties and functions that need to see. This
note records what that means, what must deliberately stay public, and what has to
happen *now* versus what can follow.

---

## Hash and encryption are not the same tool

Worth stating first, because the two get conflated and they do opposite jobs.

A **hash** is a fingerprint. Putting one on-chain proves a payload existed and
has not been altered since. Nobody can read anything from it — not the parties,
not the regulator, not anyone. It is for integrity.

**Encryption with viewing keys** is what lets specific named parties read. That
is what "both ends plus the regulator can see it" requires.

The design uses both: a hash on-chain so the record is permanent and tamper
evident, and an encrypted payload — held off-chain — that the payer, the payee
and one regulator can decrypt.

## What is hidden

Participant-to-participant transfers of issued currency: the **amount** becomes a
commitment rather than a figure, and the ISO 20022 metadata (remittance
information, purpose codes, references) becomes an encrypted payload with a hash
on-chain.

The mechanism is standard and deployed elsewhere. A Pedersen commitment is
`C = aG + rH`, where `a` is the amount and `r` a blinding factor. Commitments add
up — `C₁ + C₂` commits to `a₁ + a₂` — so the chain can verify that what went in
equals what came out **without ever seeing either figure**. A range proof
accompanies it to prove the amount is not negative, which without it would be an
inflation bug rather than a privacy feature.

## What stays public, and why

This is the part that has to be got right, because "encrypt everything" produces
a ledger that cannot do its job. Each of these is public because a designed
function depends on it:

**AMM pool reserves.** The ratio of the reserves *is* the price. A pool with a
secret reserve cannot quote, and a chain that cannot quote cannot route a swap.
Pool operations are public, and that is not a compromise — a market price is
supposed to be public.

**Issuance and redemption.** Mint and burn by the issuer stay in the clear, so
total supply remains publicly provable. This directly answers the objection a
central bank will raise first: individual holdings are private, but *how much
currency exists* is arithmetic anyone can check. Confidentiality applies between
holders, never to the money's creation.

**Staking weights and voting power.** Consensus safety depends on knowing who
holds what weight, and the concentration caps are computed from exactly these
numbers. Hiding them would break both.

**Fees.** Every validator must verify the fee was paid.

**Oracle rates.** The whole point of them is publication.

**Treasury commitments.** Kept public by default. The reason to use `x/treasury`
at all is that a commitment provably left spendable balance and cannot be
redirected — for donor disbursement, subsidy programmes and public payroll, the
auditability *is* the product. A confidential option can exist later for
commercial escrow; it is not the default.

**Enforcement amounts.** A seizure has to be visible to the ombudsman and countable
against the rolling-window cap. A secret seizure is the thing the oversight design
exists to prevent.

So the line is: **confidential between holders, public at the edges.** That is
both defensible to a regulator and the only version that leaves the system able
to function.

## Who holds a viewing key

- **The payer and the payee** — over their own payments, derived from their
  account keys.
- **The regulator of the declared settlement jurisdiction** — over any payment
  that settles in their jurisdiction. This is the same field that decides which
  authority may act on a cross-border deal (see
  [roles-and-perimeter.md](roles-and-perimeter.md)), which is what makes it the
  right field: one declaration, two consistent consequences.
- **A time-boxed auditor role**, granted by governance and expiring by itself,
  for aggregate checks that cross accounts.

Because the payload is encrypted to the regulator's key at the moment of sending,
the settlement jurisdiction has to be declared then. It cannot be decided later.

## What this does not fix

Stated plainly, because overselling it is how a pilot ends badly.

**The graph survives.** Who paid whom, and when, remains visible even when
amounts and purpose are not. Identifying one address later still exposes its
counterparties across all of history. Only a fully shielded pool removes that,
and no financial regulator will accept one. This is a large reduction in
exposure, not an elimination, and it should be described that way in every
external document.

**Individual balances stop being enumerable.** The chain can prove supply is
conserved; it cannot list who holds what without the keys. That is the intent,
but it changes what an explorer can show and what a supervisor's tooling must do.

**A lost blinding factor makes funds unspendable.** If a wallet loses `r`, the
owner cannot prove the commitment is theirs. This is a genuine new way to lose
money that does not exist today, and it lands squarely on the account service and
the threshold-custody decision still open in §8 of the scope. It must be designed
before this ships, not after.

**This is the largest single item in the specification.** It is research-grade
work, it needs its own audit, and it touches the AMM, treasury and enforcement.
It should not be estimated as a sprint.

## Sequence

**Now, and genuinely urgent — reserve the proto field numbers.** Before a pilot
writes a single payment, `MsgPayment` gets reserved fields for the commitment,
the range proof, the encrypted payload hash and the declared settlement
jurisdiction. Field numbers cost nothing today and cannot be reclaimed once a
deployment holds real balances. This is the same irreversibility class as the
genesis-counter and import-uniqueness decisions already made in `x/land`.

**Immediately after — encrypt the metadata.** `remittance_information` and
`purpose_code` move to an encrypted payload with a hash on-chain. No new
cryptography, and it closes the concrete personal-data exposure: ISO 20022 free
text is where operators actually put names, and it is currently written to an
append-only ledger with no erasure path under the NDPA, the DPA, POPIA or GDPR.

**Then — commitments and range proofs**, as its own workstream with its own audit.
