# Set up a treasury

A treasury holds funds that more than one person is responsible for. This guide
opens one, funds it, commits part of it to somebody on a vesting schedule, and
shows what that commitment protects against.

**You need:** a running chain with two funded accounts — see
[Run a local chain](local-devnet.md). Substitute your own addresses throughout.

**You will end with:** a treasury holding 5 YML, of which 4 is committed to a
beneficiary and cannot be spent by anyone, including you.

---

## Open a treasury

```bash
./blockchaind tx treasury create-treasury "Ops Treasury" \
  --from alice --chain-id yamale-devnet-2 --keyring-backend test --fees 500uyml --yes
```

The admin defaults to the sender. Treasury ids start at `0`.

```bash
./blockchaind query treasury get-treasury 0
```

## Fund it

Depositing is permissionless — anyone may pay into any treasury, the way anyone
may pay an invoice. It confers no control.

```bash
./blockchaind tx treasury deposit 0 5000000uyml \
  --from alice --chain-id yamale-devnet-2 --keyring-backend test --fees 500uyml --yes
```

```bash
./blockchaind query treasury balances 0
```

```json
{"balances":[{"denom":"uyml","total":"5000000","locked":"0","available":"5000000"}]}
```

The funds are held by the treasury module, not by alice's account. That
indirection is what the rest of this guide depends on.

## Commit funds to someone

A lock moves funds from *available* into *locked*. Nothing is transferred; the
beneficiary claims what has vested, when it has vested.

```bash
NOW=$(date +%s)

./blockchaind tx treasury create-lock \
  0 \
  $(./blockchaind keys show bob -a --keyring-backend test) \
  uyml 4000000 \
  vesting \
  $NOW $((NOW + 60)) $((NOW + 300)) \
  0 true \
  --from alice --chain-id yamale-devnet-2 --keyring-backend test --fees 500uyml --yes
```

The positional arguments are treasury, beneficiary, denom, amount, type, start,
cliff, end, intervals, revocable. So this commits 4 YML to bob, vesting over
five minutes, with nothing claimable for the first minute, and cancellable.

> The type is `vesting`, `time` or `unspecified` — the short form. The full
> proto name `LOCK_TYPE_VESTING` is rejected by the CLI.

`0` intervals means continuous vesting. Pass `4` for quarterly tranches, where
nothing releases until each quarter completes.

```bash
./blockchaind query treasury balances 0
```

```json
{"balances":[{"denom":"uyml","total":"5000000","locked":"4000000","available":"1000000"}]}
```

## What the commitment protects

Try to spend more than is available, as the admin:

```bash
./blockchaind tx treasury spend 0 <recipient> 2000000uyml "over the limit" \
  --from alice --chain-id yamale-devnet-2 --keyring-backend test --fees 500uyml --yes
```

Look at the result with `query tx <txhash>`:

```
code: 1104
treasury 0 has 1000000uyml available (5000000 held, 4000000 locked), needs 2000000
```

Alice is the administrator and has complete control of this treasury. She still
cannot reach bob's 4 YML. Neither can a governance proposal, and neither could a
group of signers that cleared its approval threshold — the funds are not
reachable by any spending path, because the only paths out check *available*
rather than *total*.

That is the property to keep in mind when deciding what a treasury is for: a
commitment made here is a commitment, not a policy that an administrator can
change their mind about.

Spending the 1 YML that *is* available works normally:

```bash
./blockchaind tx treasury spend 0 <recipient> 1000000uyml "salary" \
  --from alice --chain-id yamale-devnet-2 --keyring-backend test --fees 500uyml --yes
```

## Claiming

The beneficiary can find what is owed to them without knowing any treasury id:

```bash
./blockchaind query treasury my-locks $(./blockchaind keys show bob -a --keyring-backend test)
```

And check what is available right now:

```bash
./blockchaind query treasury claimable 0
```

Before the cliff:

```json
{"claimable":"0","vested":"0","remaining":"4000000"}
```

After it, vesting accrues from `start_time`, so the cliff releases everything
earned up to that point at once rather than restarting the clock:

```json
{"claimable":"1106666","vested":"1106666","remaining":"4000000"}
```

```bash
./blockchaind tx treasury claim 0 \
  --from bob --chain-id yamale-devnet-2 --keyring-backend test --fees 500uyml --yes
```

Only bob can do this. Claiming before anything has vested fails with
`nothing has vested yet` rather than transferring zero.

## Sharing control

So far alice controls this treasury alone. To require several people to agree,
create an [`x/group`](https://docs.cosmos.network/main/build/modules/group)
policy with the members and threshold you want, then hand the treasury to it.

Members go in one file and the threshold in another:

```json
// members.json
{"members":[
 {"address":"<alice>","weight":"1","metadata":"alice"},
 {"address":"<bob>","weight":"1","metadata":"bob"},
 {"address":"<carol>","weight":"1","metadata":"carol"}
]}
```

```json
// policy.json — two of the three must agree, within a minute
{"@type":"/cosmos.group.v1.ThresholdDecisionPolicy",
 "threshold":"2",
 "windows":{"voting_period":"60s","min_execution_period":"0s"}}
```

```bash
./blockchaind tx group create-group-with-policy <alice> "3 signers, 2 must agree" "ops policy" \
  members.json policy.json --group-policy-as-admin \
  --from alice --chain-id yamale-devnet-2 --keyring-backend test --fees 500uyml --yes

./blockchaind query group group-policies-by-group 1   # prints the policy address
```

Hand the treasury over:

```bash
./blockchaind tx treasury set-admin 0 <group-policy-address> \
  --from alice --chain-id yamale-devnet-2 --keyring-backend test --fees 500uyml --yes
```

Nothing moves and no schedule changes. But alice acting alone is now refused:

```
yml1lykpc… may not spend from treasury 0: signer is not authorized
```

From here every payment is proposed, voted on and executed through the group.
The proposal carries the treasury message, with the **policy address as its
signer**:

```json
// proposal.json
{"group_policy_address": "<policy>",
 "messages": [{
   "@type": "/blockchain.treasury.v1.MsgSpend",
   "spender": "<policy>",
   "treasury_id": "0",
   "recipient": "<carol>",
   "amount": [{"denom":"uyml","amount":"3000000"}],
   "memo": "supplier invoice, approved by the group"
 }],
 "metadata": "pay supplier",
 "proposers": ["<alice>"]}
```

```bash
./blockchaind tx group submit-proposal proposal.json --from alice ...
./blockchaind tx group vote 1 <alice> VOTE_OPTION_YES "yes"     --from alice ...
./blockchaind tx group vote 1 <bob>   VOTE_OPTION_YES "agreed"  --from bob ...
./blockchaind tx group exec 1 --from bob ...
```

> **`exec` succeeds with `code: 0` even when the proposal has not passed.** It
> simply does nothing. After one of the two required votes, the exec above
> returned success and the treasury balance was unchanged; after the second, the
> funds moved. Never read a successful `exec` as evidence that the payment
> happened — check the balance, or the proposal's status.

Every approval and rejection is recorded on-chain against the member who cast
it, which is what the compliance trail is built from.

This is deliberately not built into the treasury. `x/group` already does
membership, thresholds and the approval trail properly; a second, competing
approval system on the same funds would be a liability rather than a feature.

## Roles

Not everybody who touches a treasury should be able to reconfigure it.

```bash
# Can pay out, within the spending policy. Cannot change anything.
./blockchaind tx treasury assign-role 0 <address> spender --from alice ...

# Can freeze the treasury in an emergency. Cannot move funds.
./blockchaind tx treasury assign-role 0 <address> pauser --from alice ...
```

Pauser is separate from admin on purpose: whoever is on call at three in the
morning should be able to stop a suspected compromise without also being able to
empty the account.

Bound what a spender can do with a spending policy — per-transaction and
per-period caps, and destination allow/block lists. See
[x/treasury](../reference/treasury.md) for the full shape.

```bash
./blockchaind query treasury capacity 0 uyml
```

reports the largest single payment that would be accepted right now, taking the
balance, the per-transaction cap and the remaining period allowance together —
so an interface never offers an amount the chain would refuse.
