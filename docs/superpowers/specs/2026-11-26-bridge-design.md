# Money in and out of Yamale: fiat and external chains

**Status:** design, 2026-11-26; decisions taken the same day (see "Decided"). Nothing here is built except what the
"Already on the chain" section lists. Devnet keeps minting test money in the
meantime (the faucet, and the 10,000 USDC each new traceability account receives).

## What it is for

A buyer on the jewel marketplace, or anybody on pay.yamaletech, needs money on
their Yamale account. A seller needs to turn what they received into money they
can spend. Concretely:

- **In:** pay by card, by mobile money (M-Pesa, Orange Money, MTN MoMo), by bank
  transfer, or by sending USDC (or another asset) from another chain.
- **Out:** receive a bank transfer or a mobile-money payment, or send USDC to
  another chain.
- **Where:** pay.yamaletech is the one place money enters and leaves. Every
  other app (the jewel marketplace first) sends the person there with the
  amount already filled in, and gets them back when the money has arrived.

The person sees "Add money" and "Cash out", amounts in their currency, and a
status that ends in "arrived". They never see a bridge, a mint or a hash unless
they open the details.

## Rules this design keeps

These are on file already and the design does not reopen them, except the one
marked as a decision below.

1. **No wrapped bridge.** Bridges secured by a handful of keys are the largest
   category of loss in the industry ([custody.md](../../guides/custody.md)).
   Outside assets are held by a custodian on their own chain and a claim is
   issued here, one for one.
2. **USDC is the exception.** Circle issues native USDC on Noble, which reaches
   Cosmos chains over IBC. One connection, no custodian, no reserve to attest.
3. **One issuer per currency, approved by governance** (`x/stablecoin`), and
   every unit traceable to whoever issued it.
4. **Redemption can never be paused by the operator** (`x/custody`): an issuer
   who can stop you leaving is not a custodian.
5. **Yamalé does not hold customers' money.** Licensed partners do. Whether the
   platform may touch money at all, even for a moment at checkout, is a
   question for counsel before the first partner is signed.

## Already on the chain

| Piece | What it does today |
|---|---|
| `x/custody` | Governance registers an asset and appoints attestors. A deposit is credited when **2** distinct attestors agree on the same denom, recipient, amount and **external reference**; an asset with no attested reserve mints nothing, and issuance is checked against the attested reserve. Redemption burns the claim and is payable after **720 blocks** (about an hour); an attestor settles it with the payout's reference. Fee 0.1%, kept in the module and paid out only by governance. Live on devnet-2 with **no assets registered**. |
| `x/stablecoin` | Governance approves one issuer per currency; mint is bounded by a supply ceiling; an issuer can be revoked. `MsgMintCoin` carries **no reference**, so a retried credit can mint twice. |
| `x/paymsg` | Approved participants (banks, e-money institutions) and the customers they register. YAMALE-ONBOARD is the first, on devnet. |
| `x/alias` | Identifiers whose country is recorded by the participant that onboarded the account: on mainnet, whoever performed the identity check. |
| IBC (ibc-go v10) | Compiled into an opt-in build profile, **off** in the devnet-2 binary. Never exercised: no client, connection, channel or relayer ([ibc.md](../../guides/ibc.md)). |

From SettlX, which already runs a bank-wire rail and stablecoin rails on Stellar
and Ethereum (Sepolia), these carry over as code or as pattern:

- the expected-wire desk: one reference per expected transfer, arrivals unique
  on the bank's own reference, unmatched wires returned;
- issuance that refuses a reused deposit reference, so a retry is harmless;
- deposit watchers that keep a cursor (block and hash), wait for confirmations
  and rewind on a reorganisation;
- one deposit address per deposit, derived by HKDF from a seed;
- reserves spread into vaults that need 2 of 3 keys and a 24-hour delay, with a
  hot wallet capped at 5% of reserves;
- payouts that burn first and pay second, with held, returned and stuck states;
- screening both parties again before any money is credited.

Not built anywhere: card and mobile-money collection, processor webhooks, and a
bank API in place of a person confirming arrivals.

## The design: three rails, one rule

**The rule:** every unit that arrives from outside is minted by `x/custody`
against two attestations of an external reference, and leaves through
`x/custody`'s redemption queue. That gives every rail the properties
`MsgMintCoin` lacks: a reference that cannot be credited twice, two
independent parties instead of one key, issuance capped by an attested reserve,
and a public solvency query.

### Rail 1: fiat, through licensed partners

For each currency, one partner holds the money: a bank or e-money institution
licensed where the currency is used.

- **Registered on the chain as a custody asset** whose source is the partner,
  e.g. `source_chain: "bank:<partner>"`, with the partner and Yamalé operations
  as the two attestors. (A third, independent attestor is better once there is
  one: an auditor or a second partner.)
- **In by bank transfer:** pay.yamaletech shows the partner's account and a
  reference unique to this deposit (`YML-7K2Q-9M4D`). When the partner's
  statement shows it, the partner's attestor and Yamalé's attestor each confirm
  arrival with the bank's own reference. Two attestations, one mint, to the
  account named by the deposit.
- **In by card or mobile money:** the payment processor collects and settles
  **directly into the partner's account**; Yamalé is never the merchant of
  record and never holds the money, even for a moment. The processor's webhook
  only opens the deposit. Nothing is credited until the money has settled in the
  partner's account and both attestors have confirmed it: a processor's
  "succeeded" can still be reversed, and tokens minted against money that never
  arrives are tokens with nothing behind them.
- **Cards are reversible even after settlement.** A cardholder can dispute for
  around 120 days. Card payments therefore require 3-D Secure (fraud disputes
  then fall on the card issuer), a per-payment and per-account limit, and the
  partner's rolling reserve with the processor covers what disputes remain. Bank
  transfers and mobile money are final once settled.
- **Reserve:** the partner reports what it holds for the currency at least daily
  (`ReportReserve`); issuance stops if the report is older than
  `reserve_report_max_age_blocks` (today 0, meaning not enforced: set it).
- **Out:** the person asks to cash out to a bank account or a mobile-money
  number they verified. `RequestRedemption` burns the tokens and queues the
  payout; after the delay the partner pays and an attestor settles with the
  payout's reference. A returned payout is credited back by a new attestation.

This replaces partners minting through `x/stablecoin` directly. A partner that
wants to run its own issuance can still be approved as an `x/stablecoin` issuer,
but `MsgMintCoin` should gain an external reference refused on reuse first.

### Rail 2: USDC from any chain, through Noble

- Build the chain with the IBC profile, open **one** connection to Noble, and
  admit only the Noble transfer channel and only native USDC on it
  (governance-approved channel and denom allow-list).
- USDC arrives as a voucher (`ibc/…`). The apps show it as **USDC**; the
  hash stays behind "details".
- **From Ethereum, Solana, Base, Arbitrum and the other chains Circle's CCTP
  serves:** USDC is burned there and minted on Noble, then forwarded here over
  IBC. Yamalé runs no bridge and holds no reserve for this rail. pay.yamaletech
  shows a deposit address per source chain, using Noble's forwarding accounts,
  so the person sends from their usual wallet and the USDC lands on their Yamale
  account.
- **Out:** the reverse. IBC transfer to Noble, then CCTP to the chosen chain, or
  plain USDC on Noble.
- **Devnet's test USDC (`uusdc`, minted by the foundation) is a different
  token.** On mainnet `uusdc` is not registered in `x/stablecoin`; "USDC" means
  the Noble voucher and nothing else, so there is never a question of which USDC
  a price is in.

### Rail 3: other assets (ETH, SOL, USDT on Tron…), custodial

Later, and only where no native path like Noble exists.

- A custody asset per asset (`yeth`, `ysol`…), attested by **two watchers run by
  different parties on different hosts with different RPC providers**, so one
  compromised node or one lying provider cannot mint.
- SettlX's machinery: one deposit address per deposit, confirmations and reorg
  rewind, sweeping into vaults (2 of 3, 24-hour delay), a hot wallet capped at a
  share of reserves, burn-first withdrawals.
- These assets are not money for the marketplace: a price is in a currency, so
  a holder swaps on `x/amm` first.

## The service: yamale-ramp

One off-chain service, next to the traceability API on yamale-us at first and
on its own host before mainnet. It holds no customer money and no minting key;
it holds Yamalé's attestor key (one of two) and orchestrates.

- **Intents:** `POST /v1/deposits {rail, currency, amount, account, return_url}`
  returns what the person must do: bank details and reference, a processor
  checkout link, a mobile-money push, or a deposit address. `GET
  /v1/deposits/{id}` reports `waiting → seen → credited` (or `expired`,
  `returned`). Cash-out is the mirror: `POST /v1/payouts`.
- **Evidence in:** processor webhooks (Lomi and Flutterwave already take live
  payments for yamalelegal, and Flutterwave covers mobile money), the partner's
  statement feed or desk, chain watchers.
- **Attestation:** Yamalé's attestor signs `AttestDeposit` only when its own
  evidence matches the intent (amount, currency, reference, account). The
  partner's attestor signs from the partner's side. Neither can credit alone.
- **Screening:** sanctions and PEP screening of the account before the first
  credit and on every list change, carried over from SettlX's KYC module.
- **State:** Postgres, every transition logged, every reference unique.

## pay.yamaletech, and the hand-off from other apps

- **Add money / Cash out** screens: choose how (card, mobile money, bank, USDC
  from another chain), see fees and the final amount before confirming, then a
  status page that updates itself.
- **The hand-off:** the jewel marketplace's "Add money" opens
  `pay.yamaletech.com/add?amount=1250&currency=USDC&for=<app>&return=<url>`.
  When the money is credited, pay.yamaletech returns the person to the piece,
  and the purchase continues.
- **Prerequisite: one account per person across the apps.** Today the pay app
  keeps its own wallet derived from the password, while traceability enrols its
  key with the custodian and recovery services; the same email gives two
  addresses, and money added on pay.yamaletech would land in an account that
  does not buy. The fix, phase 0:
  1. **One account package** in `clients/shared`: traceability's sign-in,
     enrolment, recovery and session, used by every app. pay.yamaletech stops
     deriving its own wallet; the wallet app already enrols with the same two
     services.
  2. **One sign-in page**, `accounts.yamaletech.com`. Browser storage is per
     subdomain, so each app would otherwise ask for the sign-in again. The page
     hands the sealed account to the app that sent the person there, in the
     URL fragment, as the loyalty → traceability move page already does.
  3. **One accounts API** for what follows the address: the profile (name,
     date of birth) and the identifier lookups, moved out of the traceability
     API so every app reads the same record.

## Compliance

- **Identity tiers.** A small daily amount with an email and a phone; more with
  an identity check by the onboarding participant, which on mainnet also
  records the country behind the person's identifier. Limits per tier and per
  rail, enforced by yamale-ramp before it attests.
- **Jewellery is a regulated trade.** Dealers in precious metals and stones are
  obliged entities under the FATF recommendations; sales above the threshold
  need both parties identified. The marketplace should refuse a purchase above
  it from an unverified account, as a chain rule (`x/syndicate` already gates on
  KYC with a send restriction; the same pattern applies).
- **Travel rule** for crypto rails above threshold: carried with the intent,
  disclosed to the counterparty VASP on request.
- **Freeze reaches cash-out.** `x/custody` does not consult `x/enforcement`
  today, so a frozen account could still redeem. Redemption by a frozen account
  must be refused on the chain.

## Fees

- Custody fee: 0.1% on credit (`fee_bps` 10), already in `x/custody`.
- Processor and partner fees: passed through and shown before the person
  confirms.
- The marketplace's 1% is separate and unchanged.

## Phasing

| Phase | What | Chain change | Who decides |
|---|---|---|---|
| 0 (now) | One account across pay.yamaletech and the other apps; devnet faucet stays | none | |
| 1 | Fiat, one partner, one currency, bank transfer and card/mobile money; yamale-ramp; pay.yamaletech Add money / Cash out; marketplace hand-off | register the asset and attestors (governance); set `reserve_report_max_age_blocks`; freeze check in `x/custody` (upgrade) | partner, currency, counsel |
| 2 | USDC through Noble, including CCTP chains | IBC build profile on (upgrade); channel and denom allow-list (governance); a relayer | reverse the 2026-08-20 decision to keep IBC out |
| 3 | ETH, SOL, USDT custodial | register assets and two independent watcher-attestors | which assets, who runs the second watcher |

## Decided (2026-11-26, by the owner)

1. **Fiat tokens are `x/custody` claims**, not partners minting through
   `x/stablecoin`.
2. **IBC is admitted for Noble USDC**, reversing the 2026-08-20 decision to keep
   it compiled out: one channel to Noble, USDC only.
3. **Several partners, by rail and currency**: a card acquirer for card
   payments, mobile-money operators, banks for transfers; each a custody asset
   source with its own attestor.
4. **Attestors: the partner and Yamalé.**
5. **Credit only when the money is ours**: settled in the partner's account and
   attested by both, never on a processor's confirmation alone.
6. **Yamalé touches no money.** On the chain it can do nothing to a balance
   except the freeze governance votes; off the chain, processors settle into
   partners' accounts and Yamalé is never merchant of record.

Still open: which partners first, and counsel's view on the licences each
partner needs.

## Not in this design

- An exchange or market-making desk: conversion between currencies stays on
  `x/amm`.
- Loans, interest or yield on balances.
- Any bridge where Yamale validators sign for assets on another chain.
