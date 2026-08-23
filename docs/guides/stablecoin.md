# Issue a currency

How a bank or e-money institution gets permission to issue a fiat-referenced
token on Yamale, and what it can do once it has.

**You need:** a running chain and an account with some YML.
[Run a local chain](local-devnet.md) gets you both. Every command below was run
against a real node.

**You will end with:** a new currency on the chain, a supply of it you minted,
and a demonstration that nobody else can mint it.

---

## The rule this module exists to enforce

One currency, one issuer. Nobody may mint `ueur` except the party governance
approved to issue `ueur`, and that permission is recorded on the chain rather
than in a config file. It is per-denom, not global: being approved to issue
euros grants nothing over francs.

That is the whole model. Everything below is the mechanics of establishing it.

## 1. Register the currency

Registering *is* the application. There is no separate "apply" step — the
registration describes the currency you propose to issue, and it lands as a
pending application for governance to decide on:

```bash
blockchaind tx stablecoin register-currency \
  ueur EUR 6 "Euro" "EUR" "Euro issued by Alpine Bank" \
  --from bank --chain-id yamale-testnet-1 --keyring-backend test --fees 500uyml --yes
```

The arguments are the base denom, the display denom, the exponent between them,
then the name, symbol and description. `6` means one EUR is 1,000,000 ueur —
the same relationship YML has to uyml, and the convention the whole chain
assumes.

```bash
blockchaind query stablecoin list-issuer-application
```

```yaml
issuer_application:
- creator: yml1qmg7w7fn9qr5svggyf92qjgxsj46atny98jk07
  denom: ueur
  description: Euro issued by Alpine Bank
  display_denom: EUR
  exponent: "6"
  name: Euro
  status: pending
  symbol: EUR
```

`status: pending` grants nothing. Minting at this point fails.

## 2. Governance approves it

Approval is `MsgApproveIssuer`, which only the governance module account may
sign — so it happens through a proposal. Write it to a file:

```json
{
  "messages": [{
    "@type": "/blockchain.stablecoin.v1.MsgApproveIssuer",
    "authority": "yml10d07y265gmmuvt4z0w9aw880jnsr700jz5s386",
    "denom": "ueur",
    "approve": true
  }],
  "metadata": "",
  "deposit": "10000000uyml",
  "title": "Approve Alpine Bank as the issuer of ueur",
  "summary": "Alpine Bank is FINMA-licensed. Approving this lets them, and only them, mint and redeem ueur."
}
```

Two things about that payload:

- **The `authority` is the governance module account**, not your address. Get it
  with:
  ```bash
  blockchaind query auth module-account gov
  ```
  A wrong value here fails with `decoding bech32 failed`, which does not
  obviously point at the cause.
- **The approval keys on the denom, not on an address.** The chain already knows
  who applied for `ueur`; approving the denom approves that applicant. You
  cannot accidentally approve a different party for a currency somebody else
  registered.

Submit and vote:

```bash
blockchaind tx gov submit-proposal proposal.json \
  --from alice --chain-id yamale-testnet-1 --keyring-backend test \
  --gas 400000 --fees 500uyml --yes
```

```bash
blockchaind tx gov vote 3 yes --from alice \
  --chain-id yamale-testnet-1 --keyring-backend test --fees 500uyml --yes
```

> The default gas of 200,000 is not enough for a proposal carrying a message;
> it fails with `code: 11` (out of gas). `--gas 400000` covers it.

Once the voting period closes:

```bash
blockchaind query stablecoin list-approved-issuer
```

```yaml
approved_issuer:
- denom: ueur
  issuer: yml1qmg7w7fn9qr5svggyf92qjgxsj46atny98jk07
```

## 3. Mint

```bash
blockchaind tx stablecoin mint-coin ueur 50000000000 <recipient> \
  --from bank --chain-id yamale-testnet-1 --keyring-backend test --fees 500uyml --yes
```

That is 50,000 EUR in base units. Check what actually happened — the code
printed at broadcast only means the node accepted the transaction:

```bash
blockchaind query tx <txhash>
```

`code: 0` there is the real success.

And the property the module exists for, from the same run:

```
mint 50,000 EUR by the approved issuer  -> code 0
mint 1 EUR by an unrelated account      -> code 1104
```

Code 1104 is `address is not an approved issuer`. Holding the currency, or
having registered a different one, changes nothing.

## 4. Redeem

Burning is the mirror of minting and is restricted the same way:

```bash
blockchaind tx stablecoin burn-coin ueur 1000000000 \
  --from bank --chain-id yamale-testnet-1 --keyring-backend test --fees 500uyml --yes
```

The issuer burns from its own balance, so redeeming means the holder returns the
tokens to the issuer first, off-chain settlement happens, and the issuer burns
what it received. The chain does not model the fiat leg; it records the token
side of it accurately.

---

## Things worth knowing

**The currency is ordinary money to the rest of the chain.** Once minted it can
be sent with `bank send`, paid with `paymsg`, held by a treasury, and pooled on
the AMM — see [Trade and provide liquidity](amm.md), which uses the `ueur` from
this guide.

**Interfaces pick up the metadata automatically.** The name, symbol and exponent
you registered are published to x/bank, so the explorer and `@yamale/chain`
render `50000000000ueur` as `50,000 EUR` without a client update.

**Rejection is not permanent.** A refused application can be resubmitted; what
governance rejected was that proposal, not that party.

**Full reference:** [x/stablecoin](../reference/stablecoin.md) — every message,
query, parameter and error code, generated from the source.
