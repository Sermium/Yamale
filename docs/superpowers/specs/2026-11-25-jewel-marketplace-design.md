# The jewel marketplace

**Date:** 2026-11-25
**Status:** approved by the owner in conversation (payment option A, 1% to Yamalé, dedicated fee account).
**Builds on:** [the ownership card](2026-11-24-jewel-ownership-card-design.md), the first sale. This is every sale after it, and a shop may also sell its stock here.

## Decisions

| Question | Decision |
|---|---|
| How money and piece change hands | **One transaction on the chain.** The buyer's payment and the transfer of the piece happen together or not at all. Nobody holds the money; nobody has to step in. |
| Who takes part | The owner lists; any account buys. The shop is not asked. A shop may list its own stock the same way. |
| The price | One amount in one currency, chosen by the seller from the currencies governance allows (`sale_denoms`). Listing again changes it. No auctions, no counter-offers. |
| Who gets what | **99% to the seller, 1% to Yamalé** (`sale_fee_bps` = 100, governance may change it, at most 10%). No royalty to the jeweler. The fee is rounded down, in the seller's favour. |
| Where the 1% goes | **x/jewel's own module account**: no key exists for it, it is blocked from direct sends, and only governance pays out of it (`MsgWithdrawSaleFees`). |
| The buyer's guard | The buyer names the price and currency they saw; a different listing price refuses the purchase. |
| What ends a listing | The seller withdrawing it; any change of owner (a sale, a gift, a card claim, a reassignment, the shop moving its business account); a theft report. A listing is never moved to a new owner. |
| What refuses one | A stolen piece; a pending reassignment; a suspended jeweler's stock (`requireSellable`); a currency not in `sale_denoms`; a zero price. |

## Chain (x/jewel, upgrade `jewel-marketplace`)

State: `Listing` by jewel id — `seller`, `price` (a Coin), `listed_at`. In genesis as `listings`.

Messages:
- `MsgListJewel{owner, jewel_id, price}` — the owner, or staff for stock (`actsForOwner`). Replaces an earlier listing.
- `MsgUnlistJewel{owner, jewel_id}` — same signers.
- `MsgBuyJewel{buyer, jewel_id, price}` — `price` must equal the listing's. Pays `price − fee` to the seller and `fee` to the module account from the buyer's balance, then `transfer(kind "sale")`, which also spends any card and claim.
- `MsgWithdrawSaleFees{authority, to, amount}` — governance only.

Params: `sale_fee_bps` (default 100), `sale_denoms` (default `uusd`, `uusdc`, `ueurc`; the upgrade sets every stable currency issued on devnet-2).

Queries: `QueryJewelResponse.listing`; `Listings` (paginated, jewel id order) at `/yamale/blockchain/jewel/v1/listings`.

Events: `EventJewelListed`, `EventJewelUnlisted`, `EventJewelSold{jewel_id, seller, buyer, price, fee}`; the transfer itself emits `EventJewelTransferred{kind: "sale"}`.

## App (traceability.yamaletech.com)

- **Marché** tab (`/market`): every listing with photo, title, jeweler and price; filter by jeweler and currency; public.
- A jewel's public page shows the listing and, for a signed-in account that is not the seller, **Acheter**: the price, the buyer's balance in that currency, the fact that it cannot be undone, and the shortfall with a pointer to the pay app when the balance is short.
- **Mes bijoux → a piece**: "Vendre sur le marché" (price and currency), then "Modifier le prix" / "Retirer".
- **Atelier → a stock piece**: the same panel, for staff.
- The platform's fee sponsorship (`tools/traceabilityapi` ownerMessages) gains List, Unlist and Buy; list version 3.

## Out of scope

Auctions, offers below the price, paying in a currency other than the listing's (a later swap-then-buy), royalties.
