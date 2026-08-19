<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/tokenisation

## Transactions

### MsgAttestSale

`/blockchain.tokenisation.v1.MsgAttestSale`

Signed by the `attestor` field.

Permissionless.

| Field | Type | Description |
| --- | --- | --- |
| `attestor` | string |  |
| `asset_id` | uint64 |  |
| `price` | Coin |  |

### MsgClaim

`/blockchain.tokenisation.v1.MsgClaim`

Signed by the `holder` field.

MsgClaim withdraws accrued income without giving up the shareholding.

| Field | Type | Description |
| --- | --- | --- |
| `holder` | string |  |
| `asset_id` | uint64 |  |

### MsgCreateCollection

`/blockchain.tokenisation.v1.MsgCreateCollection`

Signed by the `authority` field.

MsgCreateCollection brings a registry into existence. Governance only.

There is deliberately no permissionless counterpart. Every other creation path on this chain is apply-then-approve, but here there is no application to approve: a registry of deeds is not something a chain grants on request.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `collection` | Collection |  |

### MsgDisputeSale

`/blockchain.tokenisation.v1.MsgDisputeSale`

Signed by the `challenger` field.

MsgDisputeSale suspends redemption and refers the figure to governance.

The bond is a basis-point fraction of the reported price -- scaled to the vehicle rather than flat, since a flat bond is trivial for a large fraud to post and prohibitive for a small holder to raise. Refunded if the dispute succeeds; otherwise forfeited to the vault, never to the issuer. Paying the issuer would reward provoking weak challenges.

| Field | Type | Description |
| --- | --- | --- |
| `challenger` | string |  |
| `asset_id` | uint64 |  |
| `reason` | string |  |

### MsgFractionalise

`/blockchain.tokenisation.v1.MsgFractionalise`

Signed by the `owner` field.

Title holder.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string |  |
| `asset_id` | uint64 |  |
| `symbol` | string |  |
| `supply` | string |  |
| `holder_share_bps` | uint32 | Share of the asset's economics the tokens carry. The sponsor keeps the rest. Never editable afterwards. For an asset over an x/land parcel this is the figure the registry's max_share_bps caps: it is what is being sold, not what is retained. |
| `income_denom` | string | Denomination income and proceeds will be paid in. |

### MsgFundVault

`/blockchain.tokenisation.v1.MsgFundVault`

Signed by the `funder` field.

MsgFundVault pays income in. holder_share_bps of it reaches the index; the remainder is the sponsor's and is never taken from them.

| Field | Type | Description |
| --- | --- | --- |
| `funder` | string |  |
| `asset_id` | uint64 |  |
| `amount` | Coin |  |

### MsgMintAsset

`/blockchain.tokenisation.v1.MsgMintAsset`

Signed by the `minter` field.

The collection's appointed authority only.

| Field | Type | Description |
| --- | --- | --- |
| `minter` | string |  |
| `collection_id` | string |  |
| `owner` | string |  |
| `uri` | string |  |
| `parcel_id` | uint64 | The x/land parcel this asset is title over, or 0 when it is not land. Checked here as well as at fractionalisation, because it is cheap: a mint naming a parcel that does not exist, or one held by somebody other than the named owner, is refused outright rather than left to fail later when shares are being sold and there is money on the other side of it. |

### MsgRedeem

`/blockchain.tokenisation.v1.MsgRedeem`

Signed by the `holder` field.

MsgRedeem burns tokens and pays their share in the same step.

The burn *is* the claim. A design that burns and then expects a later claim strands the money of everyone slow, asleep or dead, and leaves the chain holding funds it can no longer attribute. Supply falls to zero as holders claim, and title burns when it reaches zero.

| Field | Type | Description |
| --- | --- | --- |
| `holder` | string |  |
| `asset_id` | uint64 |  |
| `amount` | string |  |

### MsgReportSale

`/blockchain.tokenisation.v1.MsgReportSale`

Signed by the `reporter` field.

MsgReportSale states what the asset sold for. It does not open redemption.

The figure is recorded, is public, and sits for the collection's challenge window. Verification alone would not be enough: redemption is irreversible, so without a window verification only means the theft needed two signatures.

| Field | Type | Description |
| --- | --- | --- |
| `reporter` | string |  |
| `asset_id` | uint64 |  |
| `price` | Coin |  |
| `evidence_uri` | string |  |

### MsgResolveDispute

`/blockchain.tokenisation.v1.MsgResolveDispute`

Signed by the `authority` field.

MsgResolveDispute is governance deciding the contested figure.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `asset_id` | uint64 |  |
| `corrected_price` | Coin | Empty upholds the reported figure and forfeits the bond. |

### MsgSetCollectionAuthority

`/blockchain.tokenisation.v1.MsgSetCollectionAuthority`

Signed by the `authority` field.

MsgSetCollectionAuthority appoints or revokes who may mint into a collection.

Revocation stops future mints and touches nothing already issued. Deciding an existing asset was wrongly issued is a seizure, and seizures go through x/enforcement where two thirds decide -- not through a registry keeper.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `collection_id` | string |  |
| `new_authority` | string | Empty revokes. A collection with no authority refuses mints; it does not fall back to governance. |

### MsgTransferAsset

`/blockchain.tokenisation.v1.MsgTransferAsset`

Signed by the `owner` field.

MsgTransferAsset moves title. The shareholding is untouched.

A buyer takes an encumbered asset subject to its shareholders, and the obligation to fund the vault moves with title.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string |  |
| `asset_id` | uint64 |  |
| `recipient` | string |  |

### MsgUpdateParams

`/blockchain.tokenisation.v1.MsgUpdateParams`

Signed by the `authority` field.

Governance only.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `params` | Params |  |

## Queries

### Asset

`GET /yamale/blockchain/tokenisation/v1/assets/{asset_id}`

Request:

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `asset` | Asset |  |
| `vault` | Vault |  |
| `sale` | SaleReport |  |

### Assets

`GET /yamale/blockchain/tokenisation/v1/assets`

Request:

| Field | Type | Description |
| --- | --- | --- |
| `collection_id` | string |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `assets` | repeated Asset |  |
| `pagination` | PageResponse |  |

### Collections

`GET /yamale/blockchain/tokenisation/v1/collections`

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `collections` | repeated Collection |  |
| `pagination` | PageResponse |  |

### Entitlement

`GET /yamale/blockchain/tokenisation/v1/assets/{asset_id}/entitlement/{holder}`

What an account is owed right now, including income that has accrued since its balance last moved.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `holder` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `owed` | Coin |  |

### Params

`GET /yamale/blockchain/tokenisation/v1/params`

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params |  |

## State

### Asset

Asset is title to one real-world thing.

Title and shareholding are separate registers. This record says who owns the asset; the tokens say who holds the economic interest. They move independently, so an asset changing operator is a non-event for shareholders.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `collection_id` | string |  |
| `owner` | string | Holder of title. Whoever holds it carries the obligation to pay income into the vault -- an obligation, never a balance they could walk away with. |
| `uri` | string |  |
| `fraction_denom` | string | The encumbrance. Non-empty means shareholders exist against this asset. It lives on the asset record rather than in an index built beside it, so there is no path where a stale view sells somebody a warehouse that already has shareholders. That is the failure which costs the chain its credibility as a registry rather than merely costing money. |
| `holder_share_bps` | uint32 | Share of the asset's economics carried by the tokens, in basis points. The sponsor keeps the rest. Fixed at fractionalisation and never editable: an issuer who could revise it would dilute every holder without minting a single token. |
| `status` | Status |  |
| `parcel_id` | uint64 | The x/land parcel this vehicle is over, or 0 when the asset is not land. Naming the parcel is what makes the registry's authorisation enforceable: fractionalising an asset with a parcel id is refused unless the registry has a live permission for that parcel, the issued share is inside the ceiling it set, and the sponsor is still the parcel's holder. Zero means none of that applies, so a warehouse receipt or a bond is untouched. The parcel is referenced, never moved. Title stays in x/land held by the same account, which is what keeps a fractionalised parcel governed by the registry rather than by whoever accumulated the tokens. |

### Collection

Collection is a class of real-world asset the chain will record title for: a deeds registry, a warehouse operator's receipts, a vehicle register.

Collections are chain-level constructs. There is no permissionless message that creates one, because a registry of deeds is not something a chain grants on request -- governance creates it and appoints who may mint into it.

| Field | Type | Description |
| --- | --- | --- |
| `id` | string |  |
| `authority` | string | The only address permitted to mint assets into this collection. An unset or revoked authority does not fall back to governance: the collection refuses mints outright. The failure mode of a permissive default here is unlimited issuance of title to things nobody owns. |
| `verification` | VerificationMode | How a sale price reported against assets in this collection is checked. |
| `attestation_threshold` | uint32 | Attestor threshold when verification is VERIFY_ATTESTORS. Refused below 2: one attestor is not a threshold, it is a single point of unlimited theft. |
| `challenge_window_seconds` | int64 | How long a reported sale price sits before redemption opens. Calibrated to the asset class rather than a chain-wide constant -- a bond's redemption was fixed at issuance and needs days, a unique building needs a month. See docs/guides/tokenisation.md. |
| `dispute_bond_bps` | uint32 | Fraction of the reported sale price a challenger must bond, in basis points. Scales with the vehicle rather than being flat: a flat bond is trivial for a large fraud to post and prohibitive for a small holder. |

### Position

Position is one account's place in the index.

Settled whenever the balance moves, so what is owed is always balance * (cumulative_per_token - last_index).

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `holder` | string |  |
| `last_index` | string |  |
| `accrued` | string | Settled but not yet withdrawn. |

### SaleReport

SaleReport is a reported price inside or past its challenge window.

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `price` | Coin |  |
| `reporter` | string |  |
| `reported_at` | Timestamp |  |
| `claimable_at` | Timestamp | Opens redemption once passed, absent a dispute. |
| `attestors` | repeated string | Attestors who have signed this figure, when the mode requires them. |
| `disputed` | bool |  |

### Vault

Vault holds what is owed to shareholders and the running index that says how much each token has earned.

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `cumulative_per_token` | string | Income per token, accumulated over the vehicle's life, scaled by 1e18. An index rather than a snapshot because a snapshot is snipeable: buy from the pool at H-1, be holder of record at H, sell at H+1, and collect a quarter's rent for two blocks of exposure with no capital at risk. An index is time-weighted, so two blocks earn two blocks. It also removes a halt: there is no holder set to iterate. |
| `funded` | repeated Coin | Total ever paid in, for the solvency view. |
| `denom` | string | The denomination income arrives in. |

## Value types

### Status

Status is the vehicle's life. It ends; this is a closed-end instrument, not a perpetual registry.

| Value | Meaning |
| --- | --- |
| `STATUS_UNSPECIFIED` |  |
| `STATUS_HELD` | Title exists, no shareholders yet. |
| `STATUS_ACTIVE` | Shareholders exist. Income distributes as it arrives. |
| `STATUS_REPORTED` | A sale price has been reported and is inside its challenge window. Redemption is not open yet. |
| `STATUS_REALISED` | The price is final. Proceeds distribute exactly like income -- the sale is not a special case in the accounting, it is the last distribution and it is larger. Transfers of the token stop here. Each token is now a claim on a known fixed pot, and a pool still quoting from its reserves is a free lunch that gets taken within a block. |
| `STATUS_CLOSED` | Supply reached zero and the title was burned. |
| `STATUS_DISPUTED` | A challenge succeeded or the sale fell through. Back to ACTIVE by governance, never automatically. |

### VerificationMode

VerificationMode is how the chain learns what an asset sold for.

The issuer alone can never set it. An issuer who under-reports steals from every shareholder in one transaction, at the moment of maximum value, with no further obligation to anybody afterwards.

| Value | Meaning |
| --- | --- |
| `VERIFICATION_UNSPECIFIED` |  |
| `VERIFY_VALUER` | The appointed independent valuer signs the figure. x/oracle already holds this machinery for appraisals. |
| `VERIFY_ATTESTORS` | m-of-n attestors agree, m >= 2. Same shape as x/custody's deposits. |
| `VERIFY_GOVERNANCE` | Voted, with the evidence attached to the proposal. |
| `VERIFY_SCHEDULE` | The redemption amount was fixed when the vehicle was created -- par plus coupon, on a date -- so there is nothing to appraise and nobody to trust. The chain checks whether the expected amount arrived. This is the sovereign case: a state issuing a bond asks its citizens to trust arithmetic rather than a person. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 10 | `ErrInvalidShare` | holder share must be between 1 and 10000 basis points |
| 11 | `ErrAmountTooSmall` | amount is too small to divide across the shareholding |
| 12 | `ErrWrongStatus` | the asset is not in a state that allows this |
| 13 | `ErrTradingHalted` | this asset has been realised; the only remaining path is redemption |
| 14 | `ErrNotVerified` | the reported sale price has not met its verification requirement |
| 15 | `ErrStillInWindow` | the reported sale price is still inside its challenge window |
| 16 | `ErrAlreadyDisputed` | that sale is already under dispute |
| 17 | `ErrNotAttestor` | this account is not an appointed attestor for that collection |
| 18 | `ErrAlreadyAttested` | this attestor has already signed that figure |
| 19 | `ErrInvalidThreshold` | attestation threshold must be at least two |
| 2 | `ErrCollectionNotFound` | no such collection |
| 20 | `ErrInvalidParams` | invalid parameters |
| 21 | `ErrInvalidSigner` | expected the governance account as the only signer |
| 22 | `ErrInvalidAmount` | amount must be positive |
| 23 | `ErrNothingOwed` | nothing is owed to this account |
| 24 | `ErrNoSaleReported` | no sale has been reported for that asset |
| 25 | `ErrWrongDenom` | that is not the denomination this vault pays income in |
| 26 | `ErrNoParcel` | the land registry has no parcel with that id |
| 27 | `ErrNotParcelHolder` | the asset's owner is not the current holder of that parcel |
| 28 | `ErrNoLandAuthorisation` | the land registry has not authorised fractionalising that parcel |
| 29 | `ErrAuthorisationWithdrawn` | the land registry has withdrawn its authorisation to fractionalise that parcel |
| 3 | `ErrCollectionExists` | that collection already exists |
| 30 | `ErrAuthorisationExpired` | the land registry's authorisation to fractionalise that parcel has expired |
| 31 | `ErrShareCeilingExceeded` | the shares issued over that parcel would exceed the ceiling the land registry set |
| 32 | `ErrLandFractionalisationForbidden` | a restriction on that parcel forbids fractionalisation |
| 33 | `ErrNoLandRegistry` | this chain has no land registry, so an asset cannot name a parcel |
| 4 | `ErrNoAuthority` | this collection has no minting authority |
| 5 | `ErrNotAuthority` | this account is not the collection's minting authority |
| 6 | `ErrAssetNotFound` | no such asset |
| 7 | `ErrNotOwner` | this account does not hold title to that asset |
| 8 | `ErrAlreadyFractionalised` | that asset already has shareholders |
| 9 | `ErrNoShareholders` | that asset has no shareholders to credit |
