# Run a local chain

A single-node Yamale network on your machine, with the REST API open so you
can point the explorer at it. About two minutes.

**You need:** Go 1.25.10 or later. Nothing else — protobuf generation and linting
run through `go tool`.

**You will end with:** a chain producing blocks at `localhost:26657`, a REST API
at `localhost:1317`, and two funded accounts.

---

## 1. Build the node

```bash
go build -o blockchaind ./cmd/blockchaind
```

## 2. Initialise

```bash
./blockchaind init dev --chain-id yamale-devnet-1 --default-denom uyml
```

This writes `~/.blockchain`. Every command below takes `--home` if you would
rather keep it somewhere else.

## 3. Create and fund two accounts

```bash
./blockchaind keys add alice --keyring-backend test
./blockchaind keys add bob --keyring-backend test
```

```bash
./blockchaind genesis add-genesis-account $(./blockchaind keys show alice -a --keyring-backend test) 200000000000uyml --keyring-backend test
./blockchaind genesis add-genesis-account $(./blockchaind keys show bob -a --keyring-backend test) 100000000000uyml --keyring-backend test
```

Amounts are in `uyml`, the base unit: `200000000000uyml` is 200,000 YML.

The `test` keyring stores keys unencrypted on disk. That is fine for a throwaway
local chain and wrong for anything else — a real validator uses
`--keyring-backend file`.

## 4. Make alice a validator

```bash
./blockchaind genesis gentx alice 100000000000uyml --chain-id yamale-devnet-1 --keyring-backend test
./blockchaind genesis collect-gentxs
./blockchaind genesis validate-genesis
```

The last command should print that the genesis file is valid.

Validators normally need a governance vote to join this chain, but the gate does
not apply at genesis — that is what makes the founding ceremony possible. See
[x/validatorgov](../reference/validatorgov.md).

## 5. Open the API

Two settings in `~/.blockchain/config/app.toml`:

```toml
minimum-gas-prices = "0uyml"

[api]
enable = true
```

> **Write these files without a byte-order mark.** The TOML parser rejects a
> BOM with `invalid character at start of key`, and several editors add one
> silently when saving as UTF-8.

Without `minimum-gas-prices` the node refuses to start at all, with
`set min gas price in app.toml or flag or env variable`.

## 6. Start

```bash
./blockchaind start
```

Blocks begin immediately. Check it:

```bash
curl -s localhost:26657/status | grep -o '"latest_block_height":"[0-9]*"'
curl -s localhost:1317/cosmos/bank/v1beta1/supply
```

## 7. Send something

```bash
./blockchaind tx bank send \
  $(./blockchaind keys show alice -a --keyring-backend test) \
  $(./blockchaind keys show bob -a --keyring-backend test) \
  12500000uyml \
  --chain-id yamale-devnet-1 --keyring-backend test --fees 500uyml --yes
```

That is 12.5 YML. The command prints a `txhash`; the `code` in that output only
means the node accepted it for inclusion. To see whether it actually succeeded:

```bash
./blockchaind query tx <txhash>
```

`code: 0` there is the real success.

## 8. Point the explorer at it

```bash
cd clients && npm install && npm run dev
```

Open <http://localhost:5173>. The dev server proxies the node, so nothing needs
CORS opened up.

---

## Things worth knowing

**Block time.** The default is five seconds. One-second blocks are tempting for
development, but a chain left running at that rate writes its consensus WAL hard
enough to exhaust file resources on some machines — which shows up as a
`CONSENSUS FAILURE` in the log and a node that keeps producing blocks while
quietly including no transactions. If transactions stop landing, check the log
before suspecting your transaction.

**Supply grows fast.** The default emission schedule is tuned so its decay curve
is visible within a short run rather than over years. On a devnet that means
total supply climbs from the genesis allocation into the hundreds of millions of
YML within an hour. That is the schedule doing what it says; it is not what a
long-lived testnet should launch with. See
[x/emission](../reference/emission.md).

**Resetting.** `./blockchaind comet unsafe-reset-all` clears the chain data but
keeps your keys and genesis, which is usually what you want when experimenting.
