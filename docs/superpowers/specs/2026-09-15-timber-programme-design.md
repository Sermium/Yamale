# The timber programme: tokenising an operating business end to end

**Status:** programme design, 2026-09-15. Scope approved in full; sequence chosen
by me and stated below. Each phase gets its own implementation plan.

Source: `presentation.html` (SRM / Yamalé, September 2026, "From Timber to
Capital") plus the brief of 2026-09-15.

---

## The subject

A Congolese timber operation with a 250,000 mT/year production right. The
operation stays Congolese; a **UAE holding company** collects a management fee
and the proceeds, and **that company** is what gets tokenised. The deck is
explicit that the forest is not the asset being sold — the company that collects
its proceeds is.

### The figures the app must show

| | value | source |
|---|---|---|
| Tokens issued | 100,000,000 | brief |
| Price at issue | $0.60 | brief |
| Implied value of the vehicle | $60,000,000 | arithmetic |
| Retained by the owner | 75–80% | deck, "Before anything else" |
| Free float | 20–25%, so $12–15M | arithmetic |
| Year-1 distribution | ~$15M | brief; deck's $15.22M dividend line, after 25% reinvested on site |
| Operating result, year 1 | $28.18M | deck |
| Congo CIT | 28% (2026 Finance Law) | deck |
| Multiple ladder | 5.3× rising to 7.0×, base case | deck |

**One tension, stated rather than reconciled.** $15M across 100M tokens is
$0.15/token against a $0.60 price — a **25% gross yield** — while the deck values
the holding on a 5×+ multiple of earnings, which points nearer $100M than $60M.
Both can be true if $0.60 is a deliberate entry discount, but the investor
dashboard will display that yield in large type, so the number has to be the one
intended. Flagged for the owner; the app uses $0.60 and $15M as given.

### Naming

"Vehicle" becomes **"Tokenisation"** throughout the `/rwa/` console and the site
menu. The module keeps its name — `x/tokenisation` is already correct; it was
only the interface that said vehicle.

---

## What already exists, and what does not

`x/tokenisation` already carries the whole closed-end structure: collections, an
NFT as title, `Fractionalise` into a fixed shareholding that cannot be diluted,
a `Vault` with an income index, `Position`, `Claim`, `Redeem`, and sale
reporting with attestors and a challenge window. The deck's promises about
non-dilution and public distribution are **already properties of the code**.

Four things are not there.

1. **A marketplace.** `x/syndicate` has simple fixed-price listings;
   `x/tokenisation` has none, and nothing anywhere has an order book or a
   reverse auction.
2. **A dividend token.** Vault income is paid out in the denomination it arrived
   in. The brief wants a distinct dividend token, exchangeable for stables at a
   treasury.
3. **Pro-rata temporis.** See below — this is the consequential one.
4. **Covenants.** Financial statements, structure changes, GA notices. Same
   shape as the documents shipped to `x/syndicate` on 2026-09-14.

---

## The one that changes the module's core

`Vault.cumulative_per_token` pays **whoever holds at the moment of declaration**.
Its own comment defends that as anti-snipe: *"buy from the pool at H-1, be holder
of record at H, sell at H+1, and collect a quarter's rent for two blocks of
exposure"*.

The brief asks for something different: *"the date of token movement closes a %
of dividend for the initiator and opens position for the recipient"*. Hold
January to June, sell, and you still earn roughly half the year — even though the
cheque is declared in December.

That requires **share-time accounting**: the state accumulates *token-seconds*
rather than tokens, a transfer closes the seller's accrual and opens the buyer's,
and a distribution divides the pot by total token-seconds in the period rather
than by tokens outstanding.

Three things follow, and all three are arguments for doing it:

- It is how dividend accrual actually works in a private company, which is what
  this vehicle is.
- It is **more** snipe-resistant than the present index, not less: buying a block
  before declaration earns one block of share-time out of a year.
- It is nonetheless a rewrite of the mechanism every existing vehicle depends on,
  and a consensus-breaking one.

---

## Sequence, and the trade it makes

**Phase 1 — no chain change.** Rename to Tokenisation. Seed the timber vehicle on
the mechanism as it stands: mint the title, fractionalise 100M, fund a
distribution, attach specimen documents. Build the investor dashboard. Days, and
it puts the whole story on screen before any vote.

**Phase 2 — one upgrade.** Pro-rata temporis, the dividend token, and covenant
documents, bundled into a single halt. Three separate upgrades would stop the
chain three times.

**Phase 3 — one upgrade.** The marketplace: order book, fixed price, reverse
auction. Separate because it is the largest single build and independent of the
dividend mechanics.

**Phase 4.** The account service — email and OAuth enrolment, MPC signing, 2FA.
This is what the brief's *"account connection via the new email credentials"*
requires, and it does not exist: `tools/custodian` runs on the VM, but its own
startup log says enrolment needs a second deployment holding the recovery role,
and that was never stood up.

**The trade being made, stated plainly.** Phase 2 after Phase 1 means migrating
the positions Phase 1 creates. On a devnet that is a handful of accounts, and the
migration is cheap and testable. Doing it the other way round would be cleaner in
the state tree and would delay anything visible by weeks. I chose visible.

---

## Specimen documents

The brief asks for incorporation, share-creation and SPV papers. These are
produced as **clearly marked specimens** — the word SPECIMEN in the body of each,
the same plain-text-plus-digest approach as the syndicate papers — and attached
by digest exactly as real ones would be.

They describe the owner's own prospective structure for the owner's own
demonstration, which is legitimate. Marking them costs nothing and prevents a
demonstration artefact from ever being taken for a filing.

---

## What the investor sees

Two levels, per the brief.

**A general dashboard**: every vehicle held, the position in each, what it has
paid, and performance against entry.

**A per-project dashboard**: the vehicle's documents, its distribution history,
the covenants published against it, and the holder's own accrual — including,
after Phase 2, the share-time earned so far this period, which is the figure a
pro-rata temporis scheme makes meaningful and a declaration-time index cannot
show at all.
