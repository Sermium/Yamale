// chain-load.mjs - how many transactions the chain takes, measured on a real node.
//
//   node chain-load.mjs keys <accounts>                 write accounts.json (throwaway keys)
//   node chain-load.mjs run <perAccount> [concurrency]  sign, flood, and measure
//
// Each account signs its transfers OFFLINE with explicit sequence numbers, so
// signing never waits on the chain, then the transactions are broadcast straight
// to the node's RPC (broadcast_tx_sync), many accounts in parallel and each
// account in sequence order - a later sequence arriving first would be refused.
// The node's RPC is used directly, not the public gate: the gate limits each
// address to 600 RPC requests a minute, and a test from one machine would only
// measure that limit.
//
// What it reports is read back from the chain, not assumed: which block took
// how many of these transactions, and the time between blocks.
//
// Environment: RPC (default http://127.0.0.1:26657), REST (default
// http://127.0.0.1:1317), CHAIN_ID (yamale-devnet-2), FEE (2000uyml).

import { readFileSync, writeFileSync } from "node:fs";
import { DirectSecp256k1HdWallet } from "@cosmjs/proto-signing";
import { SigningStargateClient } from "@cosmjs/stargate";
import { TxRaw } from "cosmjs-types/cosmos/tx/v1beta1/tx.js";

const RPC = process.env.RPC ?? "http://127.0.0.1:26657";
const REST = process.env.REST ?? "http://127.0.0.1:1317";
const CHAIN_ID = process.env.CHAIN_ID ?? "yamale-devnet-2";
const FEE_AMOUNT = (process.env.FEE ?? "2000uyml").match(/^(\d+)(\w+)$/);
const fee = { amount: [{ denom: FEE_AMOUNT[2], amount: FEE_AMOUNT[1] }], gas: "120000" };

const [, , cmd, a1, a2] = process.argv;

async function rpc(method, params = {}) {
  const res = await fetch(RPC, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }),
  });
  return (await res.json()).result;
}

async function height() {
  return Number((await rpc("status")).sync_info.latest_block_height);
}

if (cmd === "keys") {
  const n = Number(a1 ?? 50);
  const accounts = [];
  for (let i = 0; i < n; i++) {
    const w = await DirectSecp256k1HdWallet.generate(12, { prefix: "yml" });
    const [acc] = await w.getAccounts();
    accounts.push({ mnemonic: w.mnemonic, address: acc.address });
  }
  writeFileSync("accounts.json", JSON.stringify(accounts));
  console.log(accounts.map((a) => a.address).join(" "));
  process.exit(0);
}

if (cmd !== "run") {
  console.error("usage: node chain-load.mjs keys <n> | run <perAccount> [concurrency]");
  process.exit(2);
}

const perAccount = Number(a1 ?? 10);
const concurrency = Number(a2 ?? 50);
const accounts = JSON.parse(readFileSync("accounts.json", "utf8"));

// 1. Sign everything first, offline, so the flood measures the chain and not
//    the signer.
const signStart = Date.now();
const streams = [];
for (const a of accounts) {
  const wallet = await DirectSecp256k1HdWallet.fromMnemonic(a.mnemonic, { prefix: "yml" });
  const client = await SigningStargateClient.offline(wallet);
  const info = await (await fetch(`${REST}/cosmos/auth/v1beta1/accounts/${a.address}`)).json();
  const base = info.account?.base_account ?? info.account;
  if (!base?.account_number) throw new Error(`account ${a.address} is not on chain (fund it first)`);
  const txs = [];
  for (let i = 0; i < perAccount; i++) {
    const msg = {
      typeUrl: "/cosmos.bank.v1beta1.MsgSend",
      value: { fromAddress: a.address, toAddress: a.address, amount: [{ denom: "uyml", amount: "1" }] },
    };
    const raw = await client.sign(a.address, [msg], fee, `load ${i}`, {
      accountNumber: Number(base.account_number),
      sequence: Number(base.sequence ?? 0) + i,
      chainId: CHAIN_ID,
    });
    txs.push(Buffer.from(TxRaw.encode(raw).finish()).toString("base64"));
  }
  streams.push(txs);
}
const total = streams.reduce((n, s) => n + s.length, 0);
const signSecs = (Date.now() - signStart) / 1000;
console.log(`signed ${total} transfers from ${accounts.length} accounts in ${signSecs.toFixed(1)} s`);

// 2. Flood. Each worker takes whole accounts and sends that account's
//    transactions in sequence order.
const h0 = await height();
const results = { accepted: 0, full: 0, other: 0, errors: {} };
const floodStart = Date.now();
let next = 0;
async function worker() {
  while (next < streams.length) {
    const txs = streams[next++];
    for (const tx of txs) {
      const r = await rpc("broadcast_tx_sync", { tx });
      if (r && r.code === 0) results.accepted++;
      else if (r && /mempool is full/i.test(r.log ?? "")) results.full++;
      else {
        results.other++;
        const k = (r?.log ?? "no result").slice(0, 80);
        results.errors[k] = (results.errors[k] ?? 0) + 1;
      }
    }
  }
}
await Promise.all(Array.from({ length: concurrency }, worker));
const floodSecs = (Date.now() - floodStart) / 1000;
console.log(
  `broadcast in ${floodSecs.toFixed(1)} s (${(total / floodSecs).toFixed(0)} tx/s into the node): ` +
    `${results.accepted} accepted, ${results.full} refused as mempool full, ${results.other} other`,
);
for (const [k, v] of Object.entries(results.errors)) console.log(`  ${v} x ${k}`);

// 3. Wait for the mempool to drain, then read the blocks back.
for (let i = 0; i < 60; i++) {
  const u = await rpc("num_unconfirmed_txs");
  if (Number(u.n_txs) === 0) break;
  await new Promise((r) => setTimeout(r, 2000));
}
await new Promise((r) => setTimeout(r, 6000));
const h1 = await height();
const meta = [];
for (let h = h0; h <= h1; h += 20) {
  const r = await rpc("blockchain", { minHeight: String(h), maxHeight: String(Math.min(h + 19, h1)) });
  meta.push(...r.block_metas);
}
meta.sort((x, y) => Number(x.header.height) - Number(y.header.height));
const rows = meta.map((m) => ({ h: Number(m.header.height), n: Number(m.num_txs), t: Date.parse(m.header.time) }));
const busy = rows.filter((r) => r.n > 0);
const committed = busy.reduce((n, r) => n + r.n, 0);
console.log(`\nblocks ${h0}..${h1}:`);
for (let i = 1; i < rows.length; i++) {
  console.log(`  ${rows[i].h}  ${String(rows[i].n).padStart(5)} txs  ${((rows[i].t - rows[i - 1].t) / 1000).toFixed(2)} s`);
}
if (busy.length) {
  const first = rows.find((r) => r.h === busy[0].h - 1) ?? busy[0];
  const span = (busy[busy.length - 1].t - first.t) / 1000;
  const peak = Math.max(...busy.map((r) => r.n));
  console.log(`\ncommitted ${committed} txs in ${busy.length} blocks over ${span.toFixed(1)} s`);
  console.log(`sustained ${(committed / span).toFixed(1)} tx/s; largest block ${peak} txs`);
}
