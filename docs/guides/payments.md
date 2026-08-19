# Send a payment

How money moves between institutions on Yamale, and how the statement entry
that records it is kept trustworthy.

**You need:** a running chain and an account with some YML.
[Run a local chain](local-devnet.md) gets you both. Every command below was run
against a real node.

**You will end with:** two approved institutions, a registered customer, a
settled payment, and the statement entry it produced.

---

## The shape of it

A payment is an ISO 20022 pacs.008 credit transfer: a real transfer of funds,
plus the information that makes it reconcilable — an end-to-end reference, a
purpose code, and remittance details. It settles in one block, and it leaves a
queryable record that plays the part of a camt.053 statement entry.

Three parties appear on every payment:

- the **debtor**, whose balance the money leaves, and who signs;
- the **instructing participant**, the debtor's institution;
- the **instructed participant**, the creditor's institution.

Both participants must be approved by governance. And — this is the part that
makes the record worth anything — **the debtor must actually bank with the
instructing participant it names.**

## 1. Both institutions apply

```bash
blockchaind tx paymsg apply-participant 00000011 "Bank A" \
  --from bank-a --chain-id yamale-testnet-1 --keyring-backend test --fees 500uyml --yes
```

The first argument is the participant code, the ISPB-equivalent identifier your
institution is known by. Applications sit pending:

```bash
blockchaind query paymsg list-participant-application
```

## 2. Governance approves them

`MsgApproveParticipant` is signed only by the governance module account, so it
goes through a proposal — one per institution:

```json
{
  "messages": [{
    "@type": "/blockchain.paymsg.v1.MsgApproveParticipant",
    "authority": "yml10d07y265gmmuvt4z0w9aw880jnsr700jrghjur",
    "participant": "yml1rhd9xfc0a6nhetjqqdep0u2r3p30qj8c9cuk0v",
    "approve": true
  }],
  "metadata": "",
  "deposit": "10000000uyml",
  "title": "Admit Bank A as a payment participant",
  "summary": "Licensed institution; approving lets it instruct and receive payments."
}
```

Get the `authority` from `blockchaind query auth module-account gov`, and submit
with `--gas 400000` — the 200,000 default is not enough for a proposal carrying
a message and fails with `code: 11`.

```bash
blockchaind query paymsg list-approved-participant
```

```yaml
approved_participant:
- code: "00000011"
  name: Bank A
  participant: yml1rhd9xfc0a6nhetjqqdep0u2r3p30qj8c9cuk0v
- code: "00000012"
  name: Bank B
  participant: yml1exg54yeg95hqt7u2gvmmzxey04qslusupk2v4q
```

## 3. The bank registers its customer

An approved institution is not yet entitled to appear on anybody's payment. It
has to claim the accounts it acts for:

```bash
blockchaind tx paymsg register-customer <customer-address> true \
  --from bank-a --chain-id yamale-testnet-1 --keyring-backend test --fees 500uyml --yes
```

Only the participant may sign this, and an account can bank with **one**
participant at a time — a second institution trying to claim it is refused.
Passing `false` ends the relationship, which the participant can do without
governance being involved.

**Why this step exists.** Without it, naming a participant on a payment was an
unverified claim. Any account could file an instruction attributing it to two
institutions that had never seen it, and those institutions would find payments
they never processed recorded against their name — in the ledger their customers
reconcile against. Nothing was stealable, because the transfer always came from
the signer's own balance. What was broken was the meaning of every record in the
module.

Skipping it produces exactly that refusal:

```
code 1107: ... does not bank with ..., so that participant may not be named as
instructing this payment
```

## 4. Send

```bash
blockchaind tx paymsg send-payment \
  "INV-2026-0001" \
  <instructing-participant> <instructed-participant> \
  <creditor> uyml 250000000 GDSV "Invoice 2026-0001" \
  --from customer --chain-id yamale-testnet-1 --keyring-backend test --fees 500uyml --yes
```

That is 250 YML with purpose code `GDSV` (goods and services). As always, check
what the transaction actually did rather than trusting the broadcast:

```bash
blockchaind query tx <txhash>
```

## 5. Read the statement entry

```bash
blockchaind query paymsg get-payment-record <instructing-participant> "INV-2026-0001"
```

```yaml
payment_record:
  amount: "250000000"
  block_height: "642"
  creditor: yml1t0nf0crxydpw3vvzvk27chxmuyfzma7v6ca8l2
  debtor: yml1t0nf0crxydpw3vvzvk27chxmuyfzma7v6ca8l2
  denom: uyml
  end_to_end_id: INV-2026-0001
  instructed_participant: yml1exg54yeg95hqt7u2gvmmzxey04qslusupk2v4q
  instructing_participant: yml1rhd9xfc0a6nhetjqqdep0u2r3p30qj8c9cuk0v
  purpose_code: GDSV
  remittance_information: Invoice 2026-0001
```

**The query takes the participant as well as the reference**, because that is how
the record is keyed. End-to-end ids are unique *per instructing party* in ISO
20022, not globally — so two banks can both use `INV-2026-0001` without
colliding, and neither can take a reference the other intends to use. Verified:
the same id was sent successfully under both banks in the run above.

## Paying the fee for your customers

A customer who holds naira and no YML **cannot send anything**. Network fees are
payable in YML, so their balance is stuck behind a currency they never asked
for:

```
spendable balance 0uyml is smaller than 200uyml: insufficient funds
```

That is the wrong shape for a payments rail. In ISO 20022 the institution bears
the cost of processing, not the customer, and this chain can work the same way —
a participant grants its customers a fee allowance:

```bash
blockchaind tx feegrant grant <participant> <customer>   --spend-limit 1000000uyml   --expiration 2026-12-31T23:59:59Z   --from participant
```

The customer then transacts normally, naming the sponsor:

```bash
blockchaind tx bank send <customer> <recipient> 1000000ungn   --from customer --fee-granter <participant>
```

Verified on a live chain: a customer holding **500 NGN and zero YML** was
refused, then granted an allowance, then moved the money — and their YML balance
was still exactly zero afterwards. The bank paid.

The allowance is a budget, not a blank cheque. It is consumed as it is spent
(1,000,000 → 999,800 uyml after one 200 uyml fee), it can carry an expiry, and
revoking it takes effect immediately:

```bash
blockchaind tx feegrant revoke <participant> <customer> --from participant
```

> The chain's own error when a grant is missing renders the customer's address
> as raw bytes, which tells a support desk nothing. The client SDK translates it
> into *"No fee allowance — this transaction asked another account to pay its
> network fee, but that account has not granted an allowance"*, so build against
> the SDK rather than parsing raw logs.

From an interface rather than the CLI, the SDK builds the same messages:

```ts
import { grantFeeAllowance, revokeFeeAllowance } from '@yamale/chain';

await signer.submit([
  grantFeeAllowance({
    granter: bank,
    grantee: customer,
    spendLimit: [{ denom: 'uyml', amount: '1000000' }],
    expiresAt: new Date('2026-12-31T23:59:59Z'),
  }),
]);
```

Both were signed from TypeScript against a live chain, and a customer with no
grant comes back as `succeeded: false` carrying *"No YML for the network fee"* —
not as a thrown exception, which is what CosmJS does on its own for anything
rejected before it reaches a block.

Grants are per-customer, which is deliberate: an institution sponsoring a
thousand customers issues a thousand grants and can revoke any one of them
without touching the rest.

## Field limits

Taken from ISO 20022 rather than invented, because they are what every system on
the other side of these payments already enforces:

| Field | Limit | |
| --- | --- | --- |
| `end_to_end_id` | 35 characters | Max35Text; also a store key |
| `purpose_code` | 4 characters | ExternalPurpose1Code |
| `remittance_information` | 140 characters | Max140Text, unstructured |

Exceeding one is refused with `code 1108`, naming the field and both lengths.

## Keeping the detail off the ledger

`purpose_code` and `remittance_information` are free text, and in practice they
are where an operator puts a customer's name. Written here they are public,
permanent and unerasable, which is the exposure under Nigeria's NDPA, Ghana's
DPA, POPIA and the GDPR — this chain has no erasure path, and adding one to an
append-only ledger is not a thing that can be done later.

So send a **hash** instead. The detail becomes a payload the parties hold, and
the chain records only SHA-256 over it:

```ts
import { metadataHash, newPaymentMetadata, payment, savePaymentMetadata } from '@yamale/chain';

const detail = newPaymentMetadata('GDSV', 'Invoice 2026-0001');
savePaymentMetadata(instructingParticipant, 'INV-2026-0001', detail);

const msg = payment({
  debtor, endToEndId: 'INV-2026-0001',
  instructingParticipant, instructedParticipant, creditor,
  denom: 'uyml', amount: '250000000',
  metadataHash: await metadataHash(detail),
  settlementJurisdiction: 'NG',
});
```

Three things to be clear about:

**The hash proves, it does not hide by itself.** Anyone holding the payload can
show it is the one this payment recorded — `verifyMetadata(payload, recorded)`
either matches the block or it does not, so a counterparty cannot produce an
edited remittance line and call it the record. Reading it still requires having
it. The viewing keys that let the payer, the payee and the regulator decrypt a
shared copy are the next workstream; see
[confidentiality](../scope/confidentiality.md).

**The payload carries a salt, and that is not optional.** A purpose code is four
characters from a published list. The unsalted hash of one is a lookup table,
not a fingerprint, and the ledger gives an attacker unlimited time to try.
`newPaymentMetadata` generates a fresh one per payment.

**Send the hash or the plaintext, never both.** A payment carrying both puts the
name on the ledger *and* the hash beside it — the privacy of neither. The chain
refuses it with `code 1110`.

## Settlement jurisdiction

```
--settlement-jurisdiction NG
```

An ISO 3166-1 alpha-2 country, and it answers two questions with one
declaration. A payment from Nigeria to Ghana touches two perimeters and both
authorities may *see* it, but only the declared one may *act* on it — otherwise
there is a contest over standing the chain cannot settle. The same declaration
names the regulator who will hold the third viewing key over the payload.

It is **optional today and validated when present**: `NG` is accepted, `nga`,
`NGA`, `N` and `ng` are refused with `code 1109`. It becomes mandatory when
governance turns on `require_settlement_jurisdiction`. It is a parameter rather
than a rule in the binary because a node syncing from block 0 re-executes every
historical payment, and payments made before the field existed named no country;
refusing them on replay would stop the node rather than improve anything.

---

## Things worth knowing

**The transfer is real.** This is not a message about a payment that settles
elsewhere — `SendPayment` moves the funds through x/bank in the same
transaction. If the debtor cannot cover it, nothing is recorded.

**The purpose code and remittance information are first-class.** They are the
reason this module exists rather than using a plain transfer with a memo: they
are the fields reconciliation actually needs. Send them as a hash rather than as
text — the detail stays reconcilable to whoever holds the payload, without the
customer's name going onto a public ledger that cannot forget it.

**A participant can pay from its own balance.** It is its own instructing agent,
so no registration is needed for that case.

**Full reference:** [x/paymsg](../reference/paymsg.md) — every message, query,
parameter and error code, generated from the source.
