<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen. Run `make docs` to regenerate.
-->

# Reference

Every message, query, state type, parameter and error code on the chain,
generated from the protobuf definitions and the modules' own source. If
something here is wrong, the code is wrong — these pages cannot drift from it.

For explanations and walkthroughs, start with the [guides](../guides/).

| Module | Purpose | Transactions | Queries |
| --- | --- | --- | --- |
| [x/alias](alias.md) |  | 10 | 12 |
| [x/amm](amm.md) | A constant-product automated market maker: permissionless liquidity pools, and swaps priced by the pool's own reserves. | 5 | 3 |
| [x/builderfee](builderfee.md) | Shares a governance-set portion of transaction fees with the developer whose message type was used. | 3 | 5 |
| [x/constitution](constitution.md) |  | 3 | 4 |
| [x/custody](custody.md) |  | 7 | 5 |
| [x/emission](emission.md) | Replaces the standard mint module with a fixed, decaying issuance schedule that converges on a capped supply. | 1 | 2 |
| [x/enforcement](enforcement.md) |  | 9 | 10 |
| [x/land](land.md) |  | 13 | 10 |
| [x/netting](netting.md) | The tiered settlement layer: participants settle retail activity on their own books and submit only what they owe each other, netted multilaterally against prefunded reserves, with high-value items settling gross. | 4 | 6 |
| [x/oracle](oracle.md) |  | 7 | 9 |
| [x/paymsg](paymsg.md) | ISO 20022-shaped credit transfers between institutions that governance has approved, each leaving a queryable statement entry. | 6 | 7 |
| [x/stablecoin](stablecoin.md) | Governance-approved issuers for fiat-referenced currencies, with minting and redemption restricted to the approved issuer of each denom. | 5 | 5 |
| [x/tokenisation](tokenisation.md) |  | 13 | 5 |
| [x/treasury](treasury.md) | Programmable custody: shared funds with roles, spending policies, time locks and vesting schedules, where committed funds cannot be spent by anyone. | 16 | 12 |
| [x/validatorgov](validatorgov.md) | Restricts the validator set to candidates that governance has admitted, enforced before a create-validator transaction is accepted. | 9 | 10 |

## Chain-wide conventions

**Amounts** are integers in the base unit, with no decimal point. `uyml` is the
base unit of YML at six decimal places, so `12500000uyml` is 12.5 YML. Clients
convert only when displaying.

**Addresses** are bech32 with the `yml` prefix; validator operator addresses use
`ymlvaloper`.

**Signers** are declared per message and enforced by the SDK. A message whose
signer is the governance module account cannot be sent directly — it has to be
the payload of a governance proposal.

**Errors** carry a module codespace and the code listed on each page, so a
failed transaction can be traced to exactly one registered error.
