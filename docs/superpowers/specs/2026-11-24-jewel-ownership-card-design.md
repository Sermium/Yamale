# The jewel ownership card

**Status:** approved in conversation, 2026-11-24. Next: the implementation plan.
**Scope:** the first sale of a registered jewel. The marketplace for later, private sales is a separate design that builds on this one.

## Why

Passing a jewel from the shop to its buyer takes too many steps today. Staff open the piece, choose "by link" or "to an account", show a QR code or scan the buyer's account code, and the buyer then accepts. An offer also lapses after 14 days (`offer_ttl` on devnet-2), so it cannot be prepared ahead of the sale.

The owner wants three steps:

1. Each jewel comes with an ownership card carrying a QR code.
2. The buyer scans it and claims the jewel. The shop validates the claim.
3. The jewel is in the buyer's *My jewels*.

## Decisions

Taken one by one with the owner on 2026-11-24.

| Question | Decision |
|---|---|
| What the shop's validation is | The buyer's scan files a claim naming their account. Staff approve it once they know this person paid. A card photographed in the shop is harmless, because nobody approves that claim. |
| What the QR holds | A secret, one per card. It is printed only on the card; the chain keeps the card's public key. |
| When cards are printed | From stock, whenever needed: one piece, a design, or every piece without a card. Printing a piece's card again cancels the previous one. |
| Today's offers | In the Atelier, cards replace offers by link and to an account. In *My jewels*, an owner keeps one simple **Give** to a known account until the marketplace exists. The offer messages stay on the chain, unchanged. |
| Where the card and the claim live | On the chain, in x/jewel (approach A). Approval moves the jewel itself, so the buyer never has to accept anything. |

Rejected:

- **Keeping cards and claims in traceability-api.** It needs no upgrade, but the claim would exist only in our database, and the buyer would still have to accept an offer.
- **Printing today's claim link and raising `offer_ttl`.** It has no validation at all: whoever holds the card would take the jewel.

## The chain: x/jewel

### State

Two new maps, both keyed by jewel id:

- **`Card`**: `{ key, issued_by, issued_at }`. `key` is a compressed secp256k1 public key in hex, the same form as `Offer.claim_key`. A jewel has at most one card.
- **`Claim`**: `{ claimer, card_key, filed_at }`. A jewel has at most one pending claim. `card_key` records the card the claim was signed with.

A claim is listed per jeweler through a new index, `ClaimsByJeweler`: a key set of `(jeweler_id, jewel_id)`. It changes only together with `Claim`.

### Messages

Every address taken from a message goes through `k.canonical`, as every x/jewel handler already does.

| Message | Signer | Effect | Refused when |
|---|---|---|---|
| `MsgIssueCard {staff, jewel_id, card_key}` | the business account or its staff, while the jewel is stock (`actsForOwner`) | Sets the jewel's card, replacing any earlier one, and removes any pending claim made with the earlier card. | The jeweler is not active. The jewel is not stock (its owner is not the jeweler's business account). The jewel is reported stolen. `card_key` is not a valid compressed key. |
| `MsgFileClaim {claimer, jewel_id, card_pub_key, card_signature}` | the claimer | Records the pending claim. | The jewel has no card, or `card_pub_key` is not its card. The signature does not verify over `CardClaimSignBytes`. The jewel is not stock, or is stolen. A claim is already pending. The claimer is the jeweler's business account. The jeweler is not active. |
| `MsgApproveClaim {staff, jewel_id}` | the business account or its staff, while the jewel is stock (`actsForOwner`) | Moves the jewel to the claimer through `transfer`, with the new transfer kind `card`. | No claim is pending. The jeweler is not active. The jewel is no longer stock. |
| `MsgRejectClaim {staff, jewel_id}` | the business account or its staff, while the jewel is stock (`actsForOwner`) | Removes the claim. The card stays valid. | No claim is pending. |
| `MsgWithdrawClaim {claimer, jewel_id}` | the claimer | Removes their own claim. | No claim is pending, or it names someone else. |

What the card's secret signs:

```
CardClaimSignBytes(chainID, jewelID, cardKey, claimer) =
  "yamale-jewel-card/v1:<chainID>:<jewelID>:<cardKey hex>:<claimer>"
```

A claim copied from the network therefore names its claimer and cannot be refiled for anyone else. Because it also names the card key, a claim signed with a cancelled card fails.

### Rules that cut across messages

- **Who acts for the shop.** "Staff" below and in the app means the business account or a member of its staff, the same test `actsForOwner` applies to stock today.

- **A transfer spends the card.** `transfer` is still the only path that moves a token. It now also removes the jewel's `Card`, its `Claim` and the claim's `ClaimsByJeweler` entry, whatever caused the transfer: an approved claim, an accepted or claimed offer, or a completed reassignment. A card can never move a jewel twice.
- **A theft report closes the claim.** `ReportStolen` already closes the open offer; it now also removes a pending claim.
- **One claim at a time.** A stranger who files first only delays the real buyer: staff reject that claim and the buyer files with the same card.
- **A suspended jeweler cannot hand over stock.** This is today's rule for offers, applied to issuing and approving.

### Events

`EventCardIssued {jewel_id, by}`, `EventClaimFiled {jewel_id, claimer}`, `EventClaimApproved {jewel_id, claimer, by}`, `EventClaimRejected {jewel_id, by}` and `EventClaimWithdrawn {jewel_id, claimer}`. An approval also emits the existing `EventJewelTransferred`, with kind `card`.

### Queries

- **`Jewel`** (the verification page's query) also returns `has_card` and the pending `claim`, if any.
- **`ClaimsByJeweler {jeweler_id, pagination}`** returns the jeweler's pending claims, each with its jewel, for the Atelier.

### Genesis

`Card` and `Claim` are exported and imported. `ClaimsByJeweler` is derived state: `InitGenesis` rebuilds it from `Claim`, and the round-trip test checks byte equality (see the derived-state rule in memory). Validation refuses:

- a claim on a jewel without a card;
- a claim whose `card_key` is not that card's key;
- a card or claim on a jewel that does not exist.

### Signing hardware

The amino names stay within Ledger's limit, which an earlier review shortened two names for: `blockchain/x/jewel/MsgIssueCard`, `MsgFileClaim`, `MsgApproveClaim`, `MsgRejectClaim`, `MsgWithdrawClaim`.

## traceability-api

- **The sponsor pays claims.** `ownerMessages` in `sponsor.go` gains `MsgFileClaim` and `MsgWithdrawClaim`. An allowance is an `AllowedMsgAllowance` over a fixed list, so accounts granted before this change cannot file a claim with sponsored fees. When such an account asks for sponsorship, the API revokes the old allowance and grants the new one, instead of answering `errAlreadyGranted`. The daily cap on new accounts is unchanged.
- **Staff pay as today.** Staff messages are paid the way staff messages are paid now.

## The app (traceability.yamaletech.com)

### The card

The QR encodes:

```
https://traceability.yamaletech.com/card#<jewel id>.<secret hex>
```

The secret sits in the fragment, which browsers never send to a server, as in today's claim links.

The printed card shows:

- the shop's name and logo;
- the design name and the serial;
- the QR;
- one line: "Scan to register your ownership".

### Shop: printing cards (Atelier → En stock)

1. Each piece shows whether it has a card.
2. **Print ownership cards** opens a choice: one piece, a design, or every piece without a card.
3. The app generates one secret per card on the device.
4. It submits the `MsgIssueCard` messages in batches, as the CSV registration batches `MsgRegisterJewel`.
5. **Only once the transactions succeed** does it show the print sheet: A4, about 8 cards per page, through the browser's print dialog.
6. When the page closes, the secrets are gone. The app never stores them, and no server ever sees them.

Reprinting a card asks for confirmation first, because it cancels the card already in the box. If a batch fails, nothing is printed and the outcome is reported per piece, as the CSV registration does.

### Buyer: scanning the card (`/card`)

The page shows the jewel's public record (photos, design, serial), then depends on the account on this phone:

1. **Signed in:** "Claim into your account", with the account code shown, so the buyer can see where the jewel will land.
2. **An account on this phone, locked:** the usual unlock, then the claim.
3. **No account on this phone:** two equal choices.
   - **"I'm new: create my account"** runs the existing sign-up.
   - **"I already have an account"** runs the existing sign-in on another device (recovery).

   One line warns: "If you create a new account instead, the jewel goes there, not to your existing one."

   Either way, the page returns to the card on its own, with the claim button waiting.

**The secret survives the detour.** Sign-up and recovery navigate away, and the fragment would be lost. The app keeps `{jewel id, secret}` in this tab's `sessionStorage` until the claim is filed or the tab closes, then erases it. Scanning the card again resumes.

After the claim is filed, the page says: "Claimed. Maison Diop will confirm that you bought it." (The shop's name comes from the jeweler.)

**My jewels** gains a section, "Waiting for the shop", listing pending claims with **Withdraw**. After approval the jewel appears with the others the next time the page loads.

### Shop: validating (Atelier)

At the top of the Atelier, **Claims to validate (n)** lists each pending claim:

- the piece and its serial;
- the claimer's account code;
- how long ago they claimed;
- **Validate** and **Reject**.

### The verification page

While a claim is pending, the jewel's public page says "Ownership claimed, awaiting the shop". A buyer at the counter can see the claim worked.

### What goes away

- **The Atelier's "Le transmettre" panel** (by link, to an account): shops hand over through cards only.
- **The owner's "by link" option.** In *My jewels*, an owner keeps **Give**: the recipient shows their account code, the owner scans it and confirms, and the recipient accepts. These are today's offer-to-an-account messages.
- **`/claim` stays,** so links already sent keep working until their offers lapse.

## Rollout

1. **x/jewel:** messages, state, queries and genesis, with keeper tests.
2. **A local rehearsal** of the upgrade, with `scripts/devnet/rehearse-jewel-upgrade.sh` as the model. The upgrade adds the two maps and the index. It sets no state for existing jewels: the 8 jewels already registered on devnet-2 have no card, and the shop prints theirs from stock afterwards.
3. **traceability-api:** the sponsor's message list and re-grant.
4. **The app:** printing, `/card`, validation, *My jewels*, and the verification page.
5. **devnet-2, by coordinated upgrade** (memory: devnet2-upgrade-run-recipe). The proposal is submitted and the vote command handed to the owner, who casts it.
6. **Deploy** the API and the app, then walk it live:
   1. Print a card for a stock piece.
   2. Claim it from a new account.
   3. Approve; the jewel is in that account's *My jewels*.
   4. Print another, claim it from an existing account signed in on a second device, and approve.
   5. Reject a claim, and check the card still works.

## Testing

**Keeper:**

- every refusal in the message table;
- a reissued card drops the claim made with the old one;
- each kind of transfer (approved claim, accepted offer, claimed offer, reassignment) removes the card and the claim;
- a theft report closes the claim;
- genesis round-trip, byte for byte;
- genesis validation refuses the three inconsistencies listed above;
- simulation operations for the five messages. Cards are seeded at genesis, as approvals are, since gov-gated setup rarely passes in simulation.

**traceability-api:** a previously granted account is re-granted the widened list, once.

**App:**

- the card link parses and refuses malformed fragments;
- no print sheet unless every `MsgIssueCard` in the batch succeeded;
- the secret survives sign-up and recovery, and is erased after filing;
- the claim status reads correctly on `/card`, in *My jewels* and on the verification page.

**End to end:** the local devnet first, then the live walk in Rollout step 6.

## Not in this design

- **The marketplace.** An owner lists a jewel for sale, any registered user buys it with crypto (possibly through the pay app), and the shop is not involved. It will need its own design. A sale there moves the jewel through `transfer`, like an approved claim.
- **Notifications** when a claim arrives. There is no outbound email; the Atelier list is where claims appear.
