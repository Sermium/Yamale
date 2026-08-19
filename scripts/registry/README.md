# Registering Yamale with the ecosystem

**Chainlist does not apply.** `chainlist.org` renders `ethereum-lists/chains` and
keys every entry off `eth_chainId`. It lists EVM networks only. A Cosmos SDK
chain has nothing for it to read, and there is no submission path.

The equivalent — and the one that actually matters, because Keplr, Leap,
Mintscan, cosmos.directory and every IBC relayer read it — is the
**Cosmos Chain Registry**: <https://github.com/cosmos/chain-registry>.

Layout there:

```
testnets/yamaletestnet/chain.json      ← testnet goes under testnets/
testnets/yamaletestnet/assetlist.json
yamale/chain.json                      ← mainnet at the top level, later
yamale/assetlist.json
```

`chain.json` in this directory is a working draft with every value that comes
from the code already correct: `bech32_prefix: yml`, `slip44: 118`, `uyml` as
the fee and staking token, SDK 0.53.6, CometBFT 0.38.21. Every `REPLACE-ME` is a
thing that does not exist yet.

## What has to be true before submitting

The registry is a directory of endpoints other people's software will depend on.
A pull request is judged on whether those endpoints will still answer in six
months, not on whether the chain is interesting.

- **A domain, and stable RPC/REST/gRPC on it.** The current public URL is a
  Cloudflare quick tunnel whose hostname changes on every restart. That is
  disqualifying on its own: a registry entry pointing at a dead host breaks
  wallets for everyone who has the chain added.
- **A published `genesis.json` at a permanent URL**, so anyone can sync from
  block zero and verify they reached the same state.
- **A public git repository** at the version tag named in `codebase`.
- **More than one validator.** A single node is a chain that is offline whenever
  that node is, and reviewers do check.
- **Persistent peers or seeds**, so a new node can find the network.

Yamale meets none of these today, which is the honest reason not to open the
pull request yet rather than a process obstacle.

## Order to do it in

1. Move off the Pi and off the quick tunnel: a host with a domain, TLS, and
   RPC/REST/gRPC on stable names.
2. Launch `yamale-testnet-1` properly, with the validators from the genesis
   ceremony rather than one node.
3. Publish genesis, the repository, and a seed node.
4. Open the chain-registry pull request under `testnets/`.
5. Separately, submit to the **Keplr chain registry**
   (<https://github.com/chainapsis/keplr-chain-registry>) so the wallet offers
   the chain without `experimentalSuggestChain`. It has its own review and
   requires the chain to be in the Cosmos registry first.
6. Mainnet repeats all of it at the top level, and is the point at which the
   token's economics stop being a devnet detail.

## The one that is free today

`experimentalSuggestChain` — what the wallet's connect code already does — lets
any browser add the chain to Keplr or Leap without any registry at all. It is
how every chain works before it is listed, and it is enough for testers.
