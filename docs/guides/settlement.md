# Netting and settlement

**You need:** a running chain, an account that is an approved participant in
`x/paymsg`, and a currency governance has given a netting policy.

**You get:** an understanding of what the chain settles on your behalf, what it
never settles, and what to do on the day a window does not clear.

Every command below was run against
[`scripts/devnet/netting-demo.sh`](../../scripts/devnet/netting-demo.sh), which
stands up a four-participant chain and clears a real multilateral cycle. The
figures quoted are that run's.

---

## What this layer is for

The chain does not carry your customers' payments. You match those on your own
books, and you tell the chain only what the day left you owing another
institution. That is the whole of the privacy design: a payment that was never
written to a ledger cannot be read off it by a competitor bank, by a node
operator, or by whoever compromises either of them in ten years. It is the
reason confidential amounts were assessed and deferred rather than built — see
[the confidentiality note](../scope/confidentiality.md).

What that buys in privacy it owes in risk management, and the rest of this guide
is that debt being paid.

## Prefund first, or nothing else works

```bash
blockchaind tx netting post-reserve $BANK_A 20000000ungn --from bank-a
```

The coins leave your account and enter the module's. They are yours — nobody
else can direct them, and `withdraw-reserve` brings back whatever is not
committed — but they are no longer spendable from your own balance, and that is
the point rather than a side effect. A balance you have merely promised not to
spend can be spent with an ordinary bank send, and the settlement system would
find out at the worst possible moment.

**Everything you are allowed to owe is bounded by what you have posted here.**
That single rule is what makes deferred settlement safe on this chain, and the
next section is what it prevents.

## Submitting what you owe

```bash
blockchaind tx netting submit-obligation $BANK_A $BANK_B ungn 4000000 \
  --batch-hash $(sha256 of your salted batch) --from bank-a
```

Three things are worth knowing about the response.

**You do not choose whether it nets.** The chain decides from the amount and the
currency's threshold. At or above the threshold it settles gross, in this block,
out of your own balance — the money is with the counterparty before the
transaction returns. Below it, the obligation joins the open window. An
institution that could choose would put its largest items into the deferred
window, which is exactly backwards.

**The cap is measured on your net position, not on the obligation.** If you are
owed 900 and you now owe 1000, your exposure is 100 and 100 is what has to be
collateralised. This is why netting saves liquidity rather than costing it, and
it is released the moment an offsetting obligation arrives, not at the close:

```
bank-a: reserve 20000000  locked 0        available 20000000  net  3400000
bank-b: reserve 20000000  locked  400000  available 19600000  net  -400000
bank-d: reserve 20000000  locked 3000000  available 17000000  net -3000000
```

Four institutions, 39,300,000 of obligations submitted, and the largest amount
of collateral any of them has tied up is 3,000,000.

**The batch hash is required, and it is required at 32 bytes.** It is the only
link from an interbank figure back to the items it summarises. Without it,
tiering trades away auditability as well as throughput, and neither party can
reconcile the figure months later. Salt the batch before hashing it: a small,
guessable payload hashed without a salt is not a fingerprint, it is a lookup
table.

## What the close looks like

The window closes in an end blocker, at the first height divisible by
`cycle_blocks`. There is no transaction, so there is no receipt — watch for
`EventCycleSettled`, or ask:

```bash
blockchaind query netting cycle 1
```

```
status: CYCLE_STATUS_SETTLED   closed at height 60
ungn: 16 obligations, gross 39300000, net funded 3400000
compression: 91.34%
```

**Nothing moved between accounts.** Settlement rearranges claims on reserve the
module already holds. That is what makes it unrefusable: there is no transfer
for a freeze, a blocked address or a participant whose approval lapsed this
morning to stop, because nobody is being asked for anything. Your credit lands
in your reserve, where it funds your own debits in the next window without a
round trip through your balance. Taking it out is an ordinary withdrawal.

**Currencies settle separately.** There is no cross-currency netting, because
offsetting a euro debit against a naira credit is an FX trade, and an FX trade
priced by a settlement system is a position that system is taking.

**Read the compression figure for what it is.** It is the share of submitted
value that never had to be funded, computed by the chain from its own records —
and it is a self-reported operational statistic, not an audited one. Two
participants exchanging offsetting obligations all day raise it while committing
almost no collateral, which is not an attack so much as a description of what
netting is. Use it to size liquidity, not to compare institutions.

## The day a window does not clear

```bash
blockchaind query netting held
```

Empty on a healthy chain. Anything in it is money participants expected to have
settled and do not have.

A held slice is **not** recomputed, reassigned or cancelled. There is no message
in this module that can do any of those, deliberately, and the absence is
enforced by there being no code for it. Every obligation in a held slice is
still owed, at its original amount, to its original counterparty, and it is
retried unchanged at every window boundary until it clears. That is what
avoiding unwinding risk costs: while a slice is stuck, the collateral behind it
stays committed and the institutions in it are carrying an exposure past the
moment they expected to be discharged.

What you do about one depends on why it happened, and the reason is on the
record:

- **`insufficient reserve`** — a participant's reserve does not cover a debit
  the cap should have prevented, which means the state was imported or migrated
  rather than built by the handlers. The remedy is money: whoever is short posts
  reserve, and the slice clears at the next boundary with no further action.
- **`net positions in a currency do not sum to zero`** — the module's books
  disagree with themselves. Nothing will fix that from outside; it needs a
  migration, and until then refusing to settle is the correct behaviour, because
  settling the part that looks consistent would hand one institution money
  another never owed.

New business is unaffected either way. A stuck slice belongs to a closed window;
the window that opened after it keeps taking traffic, which is the second reason
not to unwind.

## Two things governance can get wrong

**Enabling netting for a currency is a decision, and so is the threshold.** A
currency with no policy nets nothing and settles every obligation gross, in the
block it arrived in. That is the safe direction: a misconfiguration costs
liquidity rather than creating credit risk nobody agreed to. Netting arrives
currency by currency, never as a default that comes with a new denom.

**Do not switch netting off in the middle of a window.** Setting `cycle_blocks`
to zero stops the end blocker before it closes anything, so a window that is
already open never settles: the obligations stay owed, the collateral behind
them stays committed, and the participants holding it cannot withdraw. Held
slices stop being retried for the same reason. Nothing is lost — a later
proposal setting a positive value closes the window at the next boundary — but
in between, every participant in that window is carrying an exposure with no
settlement date, which is the state deferred net settlement exists to avoid.
Check `query netting current-cycle` shows no positions before proposing it.

## What the chain does not do about who can read this

Queries carry no signature. The chain does not know who is asking, so there is
nobody for it to authorise, and anybody running a node reads the state store
without reaching a query handler at all — which on a permissioned network is
every participant.

So the shaping in the query service is not a confidentiality control and is not
described as one. Every endpoint that returns obligations demands a participant,
and none enumerates the bilateral matrix; that keeps the graph out of the
indexers and explorers that consume whatever the REST gateway offers, which is
the difference between a graph that is technically public and one that is
published. Per-caller scoping is done by the authenticated proxy in front of the
REST gateway, because that is the only layer that knows who the caller is. It is
a deployment convention, not a chain-enforced guarantee.

The confidentiality that is real is upstream of all of it: the customer payments
this layer exists to keep off the chain are never written down.
