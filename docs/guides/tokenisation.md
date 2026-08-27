# Tokenisation and crowdfunding

An issuer opens an offering, subscribers commit funds, and if the round
succeeds the chain mints a token representing the thing that was funded. The
chain's job is narrow: hold the money honestly until the outcome is known, then
either mint or refund. Everything about *what was funded* — the business, the
building, the harvest, the bond — lives off-chain in an agreement the chain
never sees.

## What the chain enforces

Three things, and nothing else:

1. **Subscribed funds are held by the module, not the issuer.** They move to the
   issuer only at settlement, and only if settlement succeeded. This is the same
   invariant `x/treasury` depends on: locked-versus-available is only real
   because the funds genuinely live in the module account.
2. **The outcome is deterministic.** Given the offering record and the block
   time, any node computes the same settlement. Nobody decides it.
3. **Approval gates issuance.** Anyone may propose an offering; only governance
   may let one take money.

Valuation, due diligence, whether the warehouse exists, whether the business is
solvent — none of that is consensus. It is what the approval step is *for*, and
the approval is a human judgement recorded on-chain, not a computed one.

## The offering lifecycle

    CreateOffering  ->  PENDING
                          |  ApproveOffering (gov only)
                          v
                        OPEN  ---- Subscribe / WithdrawSubscription
                          |
                          |  close_time reached
                          v
                       CLOSED  ---- Settle (permissionless crank)
                          |
              +-----------+-----------+
              v                       v
          SETTLED                  REFUNDED

`CreateOffering` is permissionless and writes a `PENDING` record that can hold
no funds. `ApproveOffering` accepts only the gov module account as signer.
Keeping those in separate messages is deliberate: an approval path reachable by
the applicant is the critical bug in this module, and one message with an
internal branch is exactly how that bug gets written.

Settlement is a permissionless crank rather than an `EndBlocker` sweep. An
`EndBlocker` that iterates every closed offering is a denial-of-service surface
that grows with usage; a crank puts the cost on whoever wants the outcome, and
anyone can pay it. The offering is fully determined at `close_time`, so nothing
depends on when the crank runs.

## Two raise modes

Declared per offering, scrutinised at approval:

**`RAISE_MODE_ALL_OR_NOTHING`** — if `raised < target` at `close_time`, every
subscriber is refunded in full and nothing is minted. This is the model retail
participants can be defended with, and the one a regulator recognises.

**`RAISE_MODE_KEEP_WHAT_YOU_RAISE`** — the issuer takes whatever was raised and
tokens mint pro-rata. Appropriate for an issuer funding a divisible thing (ten
hectares planted instead of thirty), dangerous for an indivisible one. Half a
bridge is not half as useful as a bridge, and approval is where that gets
caught.

The mode is on the record because both are legitimate and the difference is
about the *asset*, not the platform. Every client must state which mode applies
in plain words before a user commits funds — "you get your money back if this
does not reach its goal" or "you do not". That sentence is most of the
protection.

## The token

Settlement mints a plain bank denom, `tok/{offering_id}/{symbol}`. It transfers
freely and can be pooled on `x/amm` like any other coin. The offering id is in
the denom because symbols are not unique and never will be — two issuers will
both want `SOLAR`.

Free transfer is a deliberate choice with a real cost, recorded here so nobody
has to re-derive it. A freely tradable token representing SME equity or a
municipal bond is a bearer instrument that can reach anyone with a wallet,
including people who never signed the agreement that gives it value and
jurisdictions where offering it is an offence. The chain cannot fix this; the
approval step and the issuer's own terms have to. If that proves untenable for
a given asset class, the retrofit is a send restriction on the `tok/` prefix —
which is consensus-breaking and therefore a decision to make before mainnet,
not after.

## NFTs are minted by a declared authority, never by anyone

Fungible offering tokens are minted by settlement, so their supply is bounded by
what was actually subscribed — the maths is the permission. Non-fungible assets
have no such bound. A title deed, a warehouse receipt, a vehicle registration:
each is a claim that someone with standing has to *make*, and if anyone can mint
one then the token means nothing. A registry that will attribute a deed to
whoever asks is not a registry.

This is not a user-facing feature. There is no `MsgCreateCollection` that a
subscriber, an issuer, or an application can send — collections are chain-level
constructs and they come into existence only by governance. That is the
difference from the fungible path above, where anyone may *apply* to run an
offering and governance merely approves it. Here there is no application step to
approve, because a registry of deeds is not something a chain grants on request.

So minting is two-tier, the same shape as `x/custody`'s attestors and
`x/oracle`'s appointed valuers:

**Governance appoints the authority.** `MsgSetCollectionAuthority` accepts only
the gov module account as signer, and binds an address to a collection: the
lands ministry to `deeds/ci`, the licensed warehouse operator to their own
receipts, the vehicle registry to theirs. Appointment is a public, revocable,
on-chain act.

**The authority mints, and only into its own collection.** `MsgMintAsset`
checks the signer against the collection's authority and rejects everything
else. A mint names its recipient, so the asset is attributed to a wallet at
creation and never exists unattributed. There is no self-mint-then-transfer
path, because that path is where an authority laundering assets to itself
becomes indistinguishable from an authority doing its job.

Three rules that follow, and are easier to hold than to retrofit:

- **An empty authority is not a collection.** A collection whose authority is
  unset or revoked accepts no mints. Not "falls back to governance" — refuses.
  The failure mode of a permissive default here is unlimited issuance, which is
  precisely what `x/custody` refuses when it rejects an attestation threshold
  below two.
- **Revocation does not burn.** Removing an authority stops future mints and
  touches nothing already issued. Deciding an existing asset was wrongly issued
  is a seizure, and seizures go through `x/enforcement`, where two thirds decide
  — not through a registry keeper.
- **The authority cannot move what it minted.** Once attributed, the asset is
  the holder's. An authority that can move assets after issuance is a custodian
  with unlimited power over everything it has ever touched, which is the thing
  the user explicitly ruled out for funds and applies with equal force here.

Whether these NFTs transfer freely is a separate question from the fungible
tokens above and should not inherit that answer by default. A deed that trades
without the registry knowing is a deed the registry cannot honour.

## The instrument: a closed-end vehicle with a defined life

An RWA is tokenised to raise money against it, not to trade its title forever.
The shape is:

    raise  ->  ACTIVE  ---- income distributed to holders ----+
                  |                                           |
                  |  the asset is sold in the real world      |
                  v                                           |
               REALISED  ---- proceeds distributed the same way
                  |
                  v
               CLOSED   (supply zero, NFT burned)

**The NFT is the asset's identity and its encumbrance record.** It is not a
thing that trades. It exists so the chain has one object to attach the
shareholding and the obligation to, and it is burned when the vehicle closes.

**The tokens are the shareholding**, and they represent a *fixed percentage* of
the asset, not all of it. A raise that funds 40% of a warehouse mints tokens
carrying 40% of its economics; the sponsor keeps the rest. That share,
`holder_share_bps`, is fixed at settlement and can never be edited — an issuer
who could revise it after the raise could dilute every shareholder without
minting a single token.

Income during `ACTIVE` and proceeds at `REALISED` are the same mechanism: an
amount arrives, `holder_share_bps` of it enters the vault, and
`cumulative_per_token` rises. The sale is not a special case in the accounting.
It is the last distribution, and it is larger.

### Burning is how you claim

The tokens are not destroyed by the chain at the end. **Burning a token is the
act of claiming its share** — `MsgRedeem` burns and pays out in one step.
Supply falls to zero as holders claim, and the NFT burns when it reaches zero.

This ordering matters. A design that burns tokens and *then* expects holders to
claim strands the money of everyone who was slow, asleep, or dead, and leaves
the chain holding funds it can no longer attribute. Making the burn the claim
means an unclaimed share is still a live token with a live entitlement, visible
to its owner in any wallet, indefinitely.

### Trading must stop at realisation

The moment the asset is sold, each token becomes a claim on a known, fixed pot.
An AMM pool still holding tokens will keep quoting a price from its reserves,
which is now simply wrong — and a wrong price next to a known redemption value
is a free lunch that gets taken within a block.

So `REALISED` makes the token non-transferable except along the redemption path.
Pools stop; liquidity providers withdraw and redeem like everyone else. This is
the one place the free-transfer rule is deliberately suspended, and it is
suspended to protect the people it would otherwise rob.

### The sale price is the attack

Everything above assumes the chain learns what the asset sold for. It cannot
know. Somebody reports it, and **an issuer who under-reports the sale price
steals the difference from every shareholder** — in one transaction, at the
moment of maximum value, with no further obligations to anyone afterwards.

This is the largest number in the vehicle's life and the last one anybody can
contest, so a self-reported figure is not acceptable. It needs the shape already
used elsewhere on this chain:

- **an attestation threshold**, as `x/custody` requires for crediting a deposit,
  where one attestor is not a threshold but a single point of unlimited theft; or
- **the appointed independent valuer** from `x/oracle`, which exists for exactly
  this class of judgement; or
- **governance**, where the figure is voted with the evidence attached.

**Decided: the issuer alone can never set it.** The verification mode is a
property of the collection, chosen by governance when the collection is created,
because a deeds registry and a farm co-operative do not warrant the same
ceremony:

    VERIFY_VALUER      the appointed independent valuer signs the figure
    VERIFY_ATTESTORS   m-of-n attestors agree, m >= 2
    VERIFY_GOVERNANCE  voted, with the evidence attached to the proposal

`VERIFY_ATTESTORS` with `m = 1` is refused at validation, for the same reason
`x/custody` refuses an attestation threshold below two: one attestor is not a
threshold, it is a single point of unlimited theft.

### A reported price needs a challenge window

Verification alone is not enough, because **redemption is irreversible**. Once
holders start burning tokens against a figure, the money is gone; discovering
afterwards that the warehouse sold for twice the reported price leaves nothing
to correct with.

So a verified sale price does not become claimable immediately. It is recorded,
it is public, and it sits for a challenge window before `REALISED` opens the
redemption path. During that window any token holder may raise a dispute, which
suspends redemption and refers the figure to governance.

The window is the cheapest protection in the module. It costs a few days at the
end of a vehicle that ran for years, and it converts the one irreversible
mistake into a recoverable one. Without it, verification only means the theft
needed two signatures instead of one.

### The window and the mode follow the asset, not a constant

How hard a sale price is to check, and how much is lost if it is wrong, differ
by an order of magnitude across asset classes. One constant would be either
useless for a building or intolerable for a harvest.

| Asset class | Verification | Window | Why |
|---|---|---|---|
| Government / municipal bond | none needed | 2 days | The amount was fixed at issuance |
| Agricultural / commodity | `VALUER` on quantity, oracle on price | 7 days | Short cycle; price is observable |
| Real estate / infrastructure | `VALUER` | 30 days | Unique asset, no comparable, largest sum |
| SME equity / revenue share | `GOVERNANCE` | 30 days | Earnouts and non-cash consideration |

**A bond needs no oracle at all**, and this is worth dwelling on because it is
the sovereign case. The redemption schedule — par plus coupon, on a date — was
fixed when the vehicle was created. There is nothing to appraise. Verification
degenerates to a question the chain can answer by itself: *did the expected
amount arrive in the vault?* No valuer, no attestors, no vote, no trusted party
between the issuer and the holders. A state issuing a bond on this chain is
asking its citizens to trust arithmetic rather than a person, and that is the
strongest thing the module can offer anybody.

The commodity case leans on machinery that already exists: `x/oracle` carries
validator-voted rates, so the price side is a read. Only the *quantity* — how
many tonnes actually sold — needs a human, which is a far narrower thing to
attest than a valuation.

Real estate gets the longest window for the same reason it gets the valuer: the
asset is unique, there is no comparable to check the figure against, and the sum
is the largest in the vehicle's life. Thirty days is short next to the years the
vehicle ran, and it is the only period in which a wrong number can still be
caught.

### The dispute bond

A challenge window with no cost invites nuisance disputes; a flat bond is
trivial for a large fraud to post and prohibitive for a small holder to raise.
So the bond scales with the vehicle, not with the disputer: a fixed basis-point
fraction of the reported sale price, refunded in full if the dispute succeeds.

A failed dispute forfeits the bond **to the vault**, never to the issuer.
Paying it to the issuer would give them a reason to provoke weak challenges, and
an incentive to be opaque enough to attract them.


### The vault holds, the title records

Payments land in a vault keyed to the asset, never in the NFT holder's account.
That separation is what keeps a change of sponsor, custodian or operator a
non-event for shareholders: whoever holds title inherits an obligation to pay
in, not a balance they could walk away with.

## State

    Offerings      Map[uint64, Offering]              // id -> record
    Subscriptions  Map[Pair[uint64, string], Coin]    // (offering, subscriber)
    Distributions  Map[Pair[uint64, uint64], Dist]    // (offering, height)
    Claims         KeySet[Triple[uint64, uint64, string]]
    NextOfferingID Sequence

Offering ids start at 1. Zero is indistinguishable from an unset proto field,
and that has already cost this project once.

`Subscriptions` is keyed by a pair so a subscriber's commitment to one offering
is a single read. Listing every subscriber to an offering is a prefix scan;
listing every offering one account joined is not, and belongs in the indexer
rather than in a second index nobody maintains.

## Messages

| Message | Signer | Effect |
|---|---|---|
| `MsgCreateOffering` | anyone | Writes a `PENDING` record. Holds no funds. |
| `MsgApproveOffering` | gov only | `PENDING` -> `OPEN`. |
| `MsgRejectOffering` | gov only | `PENDING` -> `REJECTED`, terminal. |
| `MsgSubscribe` | anyone | Moves funds to the module account. |
| `MsgWithdrawSubscription` | subscriber | Only while `OPEN`. |
| `MsgSettleOffering` | anyone | Crank. Mints or refunds per mode. |
| `MsgDistribute` | issuer | Funds a distribution at the current height. |
| `MsgClaim` | holder | Claims one distribution. |

`MsgSubscribe` carries `min_tokens_out`. The price is fixed at creation so the
computation is not state-dependent, but pro-rata allocation under
keep-what-you-raise is, and a user signs against a state that has moved.

## Dependencies

- **`x/amm`** — pools for `tok/` denoms follow automatically from free transfer.
  No new work, but the distribution exclusion above depends on the pool being a
  module account.
- **`x/oracle`** — the appointed-valuer path already built for NFT appraisal is
  what real estate and infrastructure offerings need for periodic valuation.
  Nothing new to build; it needs wiring.
- **`x/alias`** — issuers should be identifiable by user ID, not raw address,
  everywhere a subscriber sees them.
- **No lending, no liquidation.** Nothing here is collateralised. If that
  changes, liquidation is a separate module with its own keeper incentives and
  bad-debt policy, and it should not be smuggled in here.

## What the three open questions turned out to be

This section used to list three things to decide before building. The module is
built, so here is what each became — kept rather than deleted, because the one
that is still open is easier to see beside the two that closed.

**The distribution snapshot mechanism became no snapshot at all.** The worry was
that paying holders from a snapshot lets somebody buy just before it and sell
just after, collecting a whole period's income for a moment's holding. Rather
than trying to place the snapshot where that is hardest, the vault carries a
cumulative-income-per-token index, and each holder carries the index as of the
last time their balance moved. What accrues to them is the difference multiplied
by what they held across it, settled on every transfer. Two blocks of holding
earn two blocks of income. There is no moment to time, so there is nothing to
snipe, and `Query/Entitlement` can answer exactly what one account may take right
now — which a snapshot scheme cannot do between snapshots.

A holder seen for the first time starts at the *current* index, not at zero.
Starting them at zero would credit them the asset's entire history, paying them
for a period in which they held nothing.

**`MaxHolders` became nothing.** It is not a chain parameter and not a
per-offering field, because neither answer was any good: a chain-wide cap is a
number no country agreed to, and a per-offering cap is one the issuer picks,
which makes it a cap on nobody. Where a jurisdiction limits how many people may
hold a private offering, that limit belongs to the registry that admitted the
vehicle, expressed as a restriction on the parcel, and x/land already carries
restrictions as data precisely because this kind of rule differs by country.

**The issuer bond is still open, and it is still the gap it always was.** The
`dispute_bond_bps` in a collection is staked by a *challenger*, not by the
issuer: it makes a frivolous challenge expensive, which is the opposite end of
the problem. There is nothing an issuer forfeits for taking the money and
delivering nothing, so the chain's guarantee still ends at settlement.

What partly covers it today, and it is worth being precise about how much:
`ReportSale` is checked by the collection's attestation threshold and challenge
window, so an issuer who *reports* a false price can be caught and disputed. An
issuer who reports nothing at all, and simply never realises the vehicle, is
caught by neither — the holders keep their tokens, the vault keeps whatever was
paid into it, and no message on this module compels an exit. That is a real
limitation and it should be read as one, not as an oversight.

## Open

1. The issuer bond above. Unresolved, and the only on-chain lever against an
   issuer who takes the money and does nothing.
2. Nothing compels realisation. A closed-end vehicle with no end date is an
   open-ended one with worse liquidity, and the module does not currently carry
   a defined life even though this guide's own heading promises one.
