# Trade and provide liquidity

How to open a liquidity pool, swap against it, and take your share back out.

**You need:** a running chain and two currencies. [Run a local chain](local-devnet.md)
gives you YML; [Issue a currency](stablecoin.md) gives you the second one. This
guide continues from that one, using the `ueur` it created. Every command below
was run against a real node.

**You will end with:** a pool, a completed trade, and a withdrawal — with the
figures the chain actually produced.

---

## What a pool is

Two currencies held together, with the price set by their ratio rather than by
an order book. Anyone may open one and anyone may add to one; there is no
permission step, unlike issuing a currency or validating.

The price is a consequence of the reserves, so **every trade moves it against
the trader**, and the larger the trade relative to the pool, the further. That
is not a fee — it is the mechanism — and it is the single thing to understand
before trading on one.

## 1. Open a pool

```bash
blockchaind tx amm create-pool uyml 20000000000 ueur 8400000000 30 \
  --from bank --chain-id yamale-testnet-1 --keyring-backend test --fees 500uyml --yes
```

That is 20,000 YML against 8,400 EUR, with a 30 basis point (0.30%) swap fee.
The opening ratio is your declaration of the price — 8400/20000 = **0.42 EUR per
YML** — because nothing else defines it yet. Open a pool at a ratio the market
disagrees with and the first trader will correct it at your expense.

> **YML is the hub.** On the live network every currency is pooled against YML
> and nothing else, so a swap between two other currencies routes through YML as
> a *double hop* (sell A → YML, buy B ← YML). That is why the chain runs one
> pool per currency, not one per pair — see [Currencies Yamale carries](currencies.md).
> The wallet performs both hops as a single "change" action, so a person never
> sees the intermediate YML.

```bash
blockchaind query amm get-pool 1
```

```yaml
pool:
  denom_a: uyml
  denom_b: ueur
  id: "1"
  reserve_a: "20000000000"
  reserve_b: "8400000000"
  swap_fee_bps: "30"
  total_shares: "14248139292"
```

`total_shares` is your claim on the pool. It is a token like any other —
`amm/pool/1` — held in your balance, and it is what you hand back to withdraw.

## 2. Swap

```bash
blockchaind tx amm swap 1 uyml 100000000 ueur 1 \
  --from alice --chain-id yamale-testnet-1 --keyring-backend test --fees 500uyml --yes
```

The arguments are the pool id, what you are putting in and how much, what you
want out, and **the least you will accept**.

That last one is the important one. `1` means "any amount at all", which is fine
on a quiet devnet and reckless anywhere else: between your quote and your
transaction landing, somebody else can trade and move the price. The floor is
what protects you, and setting it is the difference between a bad fill and an
unbounded one.

To find the right number, ask before you sign. The
[explorer's Trade page](../../clients/explorer) quotes it using the chain's own
formula and rounding, so the figure it shows is the figure the chain produces —
verified by executing a swap with the quoted amount as the floor and receiving
exactly that. It also shows the minimum received at 0.5%, 1% or 5% tolerance;
put that number here.

Check what happened:

```bash
blockchaind query tx <txhash>
```

## 3. Add liquidity

```bash
blockchaind tx amm join-pool 1 1000000000 420000000 \
  --from bank --chain-id yamale-testnet-1 --keyring-backend test --fees 500uyml --yes
```

Both sides, in proportion to the current reserves — which are not the ratio you
opened at if anyone has traded since. Query the pool first and match what is
there, or the excess of one side is wasted.

You receive more `amm/pool/1` shares for it.

## 4. Withdraw

```bash
blockchaind tx amm exit-pool 1 1000000000 \
  --from bank --chain-id yamale-testnet-1 --keyring-backend test --fees 500uyml --yes
```

You hand back shares and receive both currencies in proportion to what the pool
holds *now*. From a real run, burning 1,000,000,000 of 14,248,139,292 shares:

| | before | after | returned |
| --- | --- | --- | --- |
| YML reserve | 22,200.000000 | 20,641.901813 | 1,558.098187 |
| EUR reserve | 9,144.836767 | 8,503.010029 | 641.826738 |
| shares | 14,248,139,292 | 13,248,139,292 | 1,000,000,000 |

Note what that says: 7% of the shares returned 7% of each reserve. You get back
a share of the pool, not the assets you put in.

## What you earn, and what it costs

Every swap leaves its fee in the pool, so the reserves grow slightly faster than
the shares. That is the yield — it accrues to the shares themselves rather than
being paid out, and it is why the withdrawal above returned more than a
proportional share of the original deposit.

Set against that: if the two currencies move apart in price, a pool
automatically sells the one that rose and buys the one that fell. Withdraw after
that and you hold less of the winner than if you had simply kept both. The fees
have to exceed that difference for providing liquidity to have been worth it,
and on a quiet pair they may not.

The chain does not estimate this for you, and any interface that shows a
headline yield without it is telling you half the story.

---

## Things worth knowing

**Rounding always favours the pool.** Any fraction of a base unit stays with the
liquidity providers rather than the trader. This is deliberate and it is
load-bearing: rounding the other way lets somebody drain a pool one unit at a
time with repeated tiny trades.

**The pool's price is not the chain's price.** Validators agree exchange rates
separately — see [Price feeds](oracle.md) — and a pool is free to differ. A wide
gap means either the pool is thin or it has drifted; the explorer's Trade page
flags it and says by how much.

**Pool shares are ordinary tokens.** They can be sent, and they appear in
balances as `amm/pool/<id>`. Sending them transfers the claim on the pool.

**Full reference:** [x/amm](../reference/amm.md) — every message, query,
parameter and error code, generated from the source.
