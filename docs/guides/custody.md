# Bringing outside assets in

**Status: decided, not built.** This is the design agreed on 14 August 2026 and
the reasoning behind it. No code exists yet.

Yamale does not bridge. It **issues against custody** — the same thing it
already does for fiat, applied to crypto assets.

---

## The decision

A wrapped bridge was rejected outright. Bridges are the largest single category
of loss in this industry — Ronin $624M, BNB Bridge $586M, Wormhole $326M, Nomad
$190M — and a bridge secured by four validators is secured by four keys.

What replaces it: a custodian holds the asset on its own chain and an equivalent
claim is issued here. Deposit 1 ETH, receive 1 `yeth`. Return 1 `yeth`, receive
1 ETH.

## The one thing this design gets right and the obvious version gets wrong

The tempting shape is "deposit ETH, receive YML". It is the wrong shape, and the
difference is solvency.

Deposit 1 ETH at $3,000; mint 3,000 YML at $1. The treasury now holds ETH and
owes YML.

- **ETH falls 40%** → $1,800 of collateral against a 3,000 YML claim. Insolvent
  on that position, and nothing in the chain noticed.
- **YML doubles** → returning 3,000 YML costs $6,000 to reclaim $3,000 of ETH.
  Nobody would. **They only redeem when it hurts you.**

That asymmetry is a written option: the treasury would be short a put,
permanently, unhedged, for free. It is the Terra shape — a native token issued
against volatile collateral, redeemable at a floating rate — and it works right
up until the collateral falls.

Worse, if redemption is priced by oracle and YML's price comes from this chain's
own AMM pool, pushing the pool up and redeeming against it drains the reserve.
That is not hypothetical; it is how several protocols were emptied.

**The rule that avoids all of it: the asset and the liability must be the same
unit.** Deposit ETH, owe ETH. Deposit NGN, owe NGN. No price risk exists,
because there is no price in the arrangement.

## So it is two steps, not one

| | What it is | Who carries the risk |
|---|---|---|
| **1. Issuance** | ETH in → `yeth` out, 1:1, custodial | nobody — asset equals liability |
| **2. Exchange** | `yeth` → YML on the AMM | the liquidity providers, knowingly |

The user experience is unchanged: arrive with ETH, leave holding YML. What
changes is who carries the price risk — liquidity providers who chose to, rather
than the treasury silently.

And step 2 is a **real bid for YML**, at a visible price with slippage the buyer
accepts. Minting YML on deposit would have expanded supply exactly in step with
demand, which pins the price rather than raising it.

If the foundation wants to accumulate ETH by selling YML, it should do that
openly — seed a pool, or run an OTC desk. Just without a redemption promise
stapled to it. A sale is a sale; a redeemable claim is a liability.

## USDC is the exception: do not custody it

Noble already issues native USDC in the Cosmos ecosystem and it arrives over
IBC. No bridge, no wrapper, no reserve, no custodian, and it is the established
standard. One IBC connection gets real USDC.

Keep the custodial model for ETH, SOL and BNB, where no such path exists.

## Reserves

Yield on the reserve is allowed and must never become the business. Every
custodian failure ran the same way: a reserve pushed into riskier assets to fund
operations. **Fees on issuance and redemption are the revenue model.**

Three tiers:

1. **Instant** — plain, unencumbered, sized to cover normal redemption flow.
2. **Short duration** — tokenised T-bills and similar, T+1 rather than instant.
3. **Slower** — staked or lent, capped at the portion statistically unlikely to
   be called.

Liquid staking tokens are the trap. "Unlock" there means *sell*, at whatever the
market pays during the event that made you need the money.

Staked ETH takes days to weeks to exit, SOL a few days, BNB seven. A redemption
wave inside that window cannot be honoured: solvent on paper, unable to pay.
That is the duration mismatch that killed Celsius and BlockFi, and it has to be
designed against rather than discovered. **State the redemption delay out loud.**
"Instant" that sometimes isn't is far worse than "up to 7 days" that always is.

## Solvency must be a query

"Are we solvent?" should be a chain query returning issued-versus-held per
asset, not a spreadsheet somebody maintains.

If the number cannot be checked by anyone, the whole model rests on trusting the
foundation — and this is a chain whose entire purpose is not having to.

## What exists and what does not

| Piece | Status |
|---|---|
| Issue and redeem against custody | `x/stablecoin` — exists, and already allows exponent 18, which ETH needs |
| YML ↔ yETH market | `x/amm` — exists |
| Pricing both sides | `x/oracle` — exists, 48 denoms |
| Custody with roles and limits | `x/treasury` — exists |
| **Deposit attestation** — confirming an external deposit before minting | **new** |
| **Redemption queue and reserve accounting** | **new** |
| **Proof of reserves as a query** | **new** |

## Two problems to solve before building

**A bridged or issued asset has no approved issuer in the x/paymsg sense.**
Everything on this chain assumes permission: one issuer per currency, both
participants approved for a payment. An asset arriving from outside sits outside
that. The issuance model helps — every unit is minted by a named issuer — but
the participant gating still needs a decision.

**An enforcement freeze cannot reach the far side.** A scammer bridges out and
the case is moot. Custody helps here too: the custodian can refuse a redemption
to a frozen account, which a wrapped bridge could not. That refusal should be a
chain rule, not an operational promise.
