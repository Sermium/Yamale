# Jewels that carry their own record

A platform at **loyalty.yamaletech.com** where jewelers register the pieces they
sell, each with a token on the Yamale chain that holds its origin and the chain
of its owners. Anyone can check a jewel's authenticity, and every change of
owner is followed by a transfer of its token. The benefits a jeweler attaches,
such as cleaning or resizing, travel with the jewel.

Written 2026-11-12, after a brainstorming session that settled six choices.
They are listed first, because the rest follows from them.

---

## 1. The choices, and what each rules out

| Question | Decision | What it rules out |
|---|---|---|
| Who is it for | **A platform for many jewelers**, each with their own brand and staff | A single-client build that would have to be reworked for the second jeweler |
| What ties the jewel to its record | **Laser inscription** (stone girdle or inside the band) **plus the jeweler's photos** | NFC chips and card-only QR codes as proof. The certificate card's QR is a link, not evidence |
| Who takes part in a resale | **The two owners alone**: the seller offers, the buyer accepts | An obligatory jeweler check. The record says plainly that a resale is *declared*, not inspected |
| What "loyalty" means | **Benefits attached to the jewel**, set by its jeweler, which pass to each new owner | Points held by a person. Those need a ledger, an expiry policy and accounting, and are a separate product |
| Where records live | **The Yamale chain first**, as a new module, with the signed history portable to another chain later | An EVM contract on day one |
| How the module is built | **A thin `x/jewel` on top of `x/nft`**, borrowing patterns from `x/herd` and `x/land` but not their code | Generalising `x/herd`, which is live on devnet-2 and carries animal-only rules. Also plain `x/nft` with the rules held by our backend, where the chain could refuse nothing |

**What the record proves, and what it doesn't.** The chain proves that a
record exists, who registered it, and which account holds it now. It does not
prove that the ring on the table is the ring in the record. Only the inscription
and the photos do that, and only to someone who looks. Every public page says
so.

## 2. Who is involved

- **The platform** (Yamale Tech) runs an `x/group` office. It admits and
  suspends jewelers, and can propose a reassignment (section 6). It holds no
  owner's key and cannot move a jewel.
- **A jeweler** is a business account, plus staff accounts that act for it.
  The jeweler adds and removes staff without the business key changing. One
  governance vote per jeweler is deliberately avoided: `x/herd` showed that
  putting each participant behind a vote makes the system unusable.
- **An owner** is a person with an account from the account service
  (`recovery/`, see section 5): email and password, no seed phrase.
- **Anyone** can verify a jewel without an account.

## 3. The life of a jewel

1. **Registered.** A staff member records:
   - the inscription code;
   - photos;
   - metal, stones and weights;
   - origin: workshop, stone source, any lab certificate number.

   The files go to storage, and their fingerprint goes on the chain. The
   jewel starts as the jeweler's stock. An inscription already registered by
   the same jeweler is refused.
2. **Sold.** The jeweler offers the jewel to the customer's email address. The
   customer claims it, creating an account if needed, and becomes the owner.
3. **Resold.** The owner offers it to a buyer's account or email, and the buyer
   accepts. The seller can cancel until then. An offer lapses after
   **14 days**.
4. **Benefits.** The jeweler attaches benefits, for example "cleaning, 2 per
   year, until 2029-12-31". Using one needs the owner's consent (section 4.3).
   Benefits stay with the jewel when it changes owner.
5. **Reported stolen.** The owner flags the jewel, and clears the flag when it
   is found. While it is flagged, the public page shows it and no offer can be
   made.
6. **Corrected.** The issuing jeweler can append an amendment, such as a
   corrected carat weight. Amendments never overwrite: the original and the
   correction both stay visible.

**What the public sees:**
- the jewel, its photos, origin, materials and jeweler;
- its benefits and any stolen flag;
- **the number of owners and the date of each change, but never who they
  were**. A private owner's name has no place on a public page.

An owner can share a link that proves they are the current owner.

## 4. The chain module, `x/jewel`

### 4.1 State

- **Jeweler**: id, display name, business address, staff addresses, status
  (`ACTIVE` or `SUSPENDED`), and the address of the office that admitted it.
- **Collection**: one `x/nft` class per jeweler, `jewel-<jeweler id>`.
- **Jewel**: one `x/nft` token. `x/jewel` adds:
  - the normalised inscription code (upper case, spaces and separators
    removed);
  - the fingerprint and location of the record file;
  - the registration height;
  - the owner count;
  - the stolen flag;
  - its amendments.
- **Inscription index**: `(jeweler id, normalised code) → jewel id`. A
  duplicate is refused in one lookup, as with `x/land`'s survey-hash index,
  because a check that gets slower as the registry grows eventually gets
  switched off. The public search on a code returns every match with its
  jeweler, since two jewelers may use the same code.
- **Offer**: jewel, sender, an expiry, and either a recipient address or the
  public key of a one-time **claim key**. One open offer per jewel. The email
  link carries the claim key's private half, never a bare secret: a secret
  submitted on chain sits in the mempool where anyone can copy it and claim
  first, whereas a signature by the claim key names the claimer's own address
  and is useless for any other.
- **Benefit**: id, jewel, issuing jeweler, kind, allowance per period, the
  period, an expiry, and the uses recorded so far.
- **Pending reassignment** (section 6): jewel, proposed new owner, the end of
  the challenge window.
- **Counters** are exported in genesis, never derived (see the note on derived
  state).

### 4.2 Messages

| Message | Signed by | Effect |
|---|---|---|
| `AdmitJeweler`, `SuspendJeweler`, `ReinstateJeweler` | The office | Creates the jeweler and its collection, or changes its status |
| `SetStaff` | The jeweler's business address | Adds or removes staff |
| `RegisterJewel` | Staff of an active jeweler | Mints the token to the jeweler and indexes the inscription |
| `AmendJewel` | Staff of the issuing jeweler | Appends an amendment |
| `OfferJewel` | The owner | Opens an offer to an address or a claim key |
| `CancelOffer` | The owner | Closes it |
| `AcceptOffer` | The named recipient | Moves the token and increments the owner count |
| `ClaimOffer` | Anyone holding the claim key, which signs for the claimer's address | Moves the token to the signer |
| `AttachBenefit` | Staff of the issuing jeweler | Adds a benefit |
| `RedeemBenefit` | Staff of the issuing jeweler, carrying the owner's signed voucher | Records one use |
| `ReportStolen`, `ClearStolen` | The owner | Sets or clears the flag |
| `ProposeReassignment` | The office | Opens a 30-day public window |
| `VetoReassignment` | The current owner | Closes it |
| `CompleteReassignment` | Anyone, once the window has passed | Moves the token |

Offers lapse by expiry, checked when they are accepted or claimed. Nothing
runs on a timer: there is no Begin or EndBlocker, for the same reason as in
`x/land`.

### 4.3 Benefit vouchers

The owner's app shows a QR code holding a voucher, signed by the owner's key,
that names:
- the benefit;
- the issuing jeweler;
- a one-time number;
- an expiry 5 minutes ahead.

The staff scan it and submit `RedeemBenefit`. The module checks the signature
against the jewel's current owner, refuses a one-time number it has already
seen, and refuses an expired voucher. Staff cannot use up a benefit without
the owner in front of them.

### 4.4 No NFT moves by a bare send

`x/nft` has its own `MsgSend`, which would skip the offer and acceptance, the
stolen flag and the owner count. A check in the transaction pipeline is not
enough, because authz and interchain accounts reach the message router another
way. The **circuit breaker** (`x/circuit`, already wired) disables
`/cosmos.nft.v1beta1.MsgSend` at the router. Nothing on the chain uses `x/nft`
today, so nothing breaks. The rule from now on: an NFT on this chain moves only
through the module that owns its rules. A future `x/tokenisation` title NFT
follows the same rule.

`x/jewel` checks this itself. It refuses to admit a jeweler or register a
jewel while the circuit breaker still allows the bare send. A genesis file or
an upgrade that forgets the setting then fails at the first jeweler, loudly,
rather than leaving every jewel quietly movable.

### 4.5 Fees

Owners never hold coins. The platform grants each account fees through
`x/feegrant`, with a spending cap and limited to `x/jewel` messages. A
jeweler's staff are granted fees by the jeweler's business account.

## 5. The app, accounts and backend

**One web app at loyalty.yamaletech.com**, mobile first and installable on a
phone, in English and French, with three faces:

- **Verify** (no sign-in). Anyone types an inscription code, or opens the
  link printed on the certificate card (`/j/<jeweler>/<jewel>`). The page
  shows:
  - large photos that can be zoomed, for comparing the inscription under a
    loupe;
  - origin, materials and benefits;
  - the number of owners, with dates;
  - any stolen flag;
  - the sentence: *"Changes of owner are declared by the owners; the
    inscription was checked by the jeweler at registration only."*

  Each jeweler's logo and colours appear on their own jewels' pages.
- **My jewels** (owners). The owner can:
  - see the jewels they hold;
  - make an offer, or accept and claim one;
  - show a benefit QR code;
  - report a jewel stolen;
  - share an "I own this" link.
- **Atelier** (jewelers' staff). Staff can:
  - register a jewel;
  - sell it to an email address;
  - attach benefits, and scan benefit QR codes;
  - manage staff.

The screens never show addresses, hashes or fees. The raw record of each
jewel sits behind an "Advanced" link.

**Accounts.** The account service runs as deployed today:
- signing up generates a secp256k1 key in the browser and seals it under the
  person's password;
- the *sealed* copy is split 2-of-3 between the device, the custodian and the
  recovery service;
- on a new phone, email plus password brings back the two services' shares,
  and neither service can open the key alone or together.

The app signs through the existing chain signer, which accepts any offline
signer. The same key signs on EVM chains, with a different address format.
That is what keeps the accounts independent of the chain.

**loyalty-api**, a small backend, does four things:
- stores photo and record files under their fingerprints;
- sends offer, claim and benefit emails from no-reply@yamaletech.com;
- indexes chain events for search;
- takes jeweler applications for the office to review.

**It never signs for anyone.** If it is down, photos and emails stop; the
record does not.

**Hosting.** yamale-us, next to the chain node, behind Caddy and Cloudflare
like the other yamaletech.com subdomains, and watched by the existing monitor.

**Portability.** Every event is a transaction signed by the account that made
it. Moving to another chain means replaying those signed transactions behind
an adapter. Nobody has to sign again. Until mainnet, this is also what makes
devnet-2 acceptable as the first home: it is a demonstrator, with two
validators under one owner and genesis resets in its past.

## 6. When things go wrong

- **An offer goes to the wrong person.** The seller cancels before it is
  accepted. After that, only the new owner can send it back. Neither the
  platform nor the jeweler can reverse a change of owner, and the app says so
  before the seller confirms.
- **A claim link is forwarded or leaks.** Whoever holds the claim key can
  claim. Copying a claim out of the mempool does not work, because the
  signature names the claimer's address.
  The app also asks the claimer to sign in with the email address the offer
  went to. That is a safeguard in the app, not a rule on the chain, and the
  app does not present it as one. The seller can cancel until the offer is
  claimed.
- **An owner loses both password and device.** The key cannot be recovered;
  that is the account service's promise. After checking identity, the office
  can propose reassigning the jewel. The proposal is public for 30 days, the
  old key can veto it, and once the window has passed anyone can complete it.
  This is the `x/land` challenge-window pattern: the platform cannot quietly
  take a jewel.
- **A jeweler is suspended or leaves.** Its jewels and their histories stay.
  It can no longer register jewels, and its benefits are shown as "no longer
  honoured by the issuer".
- **A duplicate inscription.** Refused, and the error names the existing
  jewel.
- **A benefit QR code is reused or old.** Refused by the one-time number or
  the 5-minute expiry.
- **The backend is down.** The verify page reads the chain directly by jewel
  number. The photos may be missing, but the record still answers.

## 7. Testing

- **Keeper tests on real keepers** (`testutil/integration`). They prove that:
  - a bare `x/nft` send is refused at the router, including inside an authz
    exec;
  - each of these is refused: a duplicate inscription, an expired offer, a
    wrong claim secret, a reused or expired voucher, an offer on a stolen
    jewel, registration by a suspended jeweler, a staff action for another
    jeweler, and reassignment before the window ends or after a veto;
  - amendments only append;
  - genesis export and import match byte for byte.
- **No simulation**, like `x/land` and `x/herd`. A registry's rules are
  refusals that random messages almost never reach, so the keeper tests and
  the devnet run carry the weight.
- **End to end on a local devnet**: a seeder script with real keys, signing
  through the chain signer. It runs admission, registration, sale by email
  claim, resale, a benefit use, a stolen report and a reassignment.
- **The app**, checked against a populated chain at phone width and at
  1600 px.

## 8. Delivery

1. `x/jewel`: protos, keeper, messages, genesis, tests, wired into the app.
2. The local devnet run with the seeder.
3. The app and `loyalty-api`, deployed on yamale-us.
4. A coordinated upgrade on devnet-2 that adds `x/jewel` and sets the circuit
   breaker on `x/nft` sends. Claude prepares the proposal; the user casts the
   vote.

## 9. Left out on purpose

- **Repair and consignment entries** (custody without a change of owner).
  Easy to add later as a separate record type.
- **Points held by a person.** A separate product.
- **NFC chips.** A jeweler who wants them can be offered them later, as a
  second way to bind a jewel; the record format leaves room for one.
- **An EVM adapter.** Designed for in section 5, not built.
