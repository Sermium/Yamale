/**
 * A smoke check against a running node.
 *
 * Unit tests prove the decoders handle the shapes we think the chain produces.
 * This proves the chain actually produces them. Run it against a devnet:
 *
 *   node --experimental-strip-types src/smoke.ts
 *
 * It is not part of the test suite, because it needs a node.
 */

import { ChainClient } from './client.ts';
import { formatAmount, formatCoins } from './denom.ts';
import { formatNumber, timeAgo } from './format.ts';

const REST = process.env.YAMALE_REST ?? 'http://localhost:1317';
const RPC = process.env.YAMALE_RPC ?? 'http://localhost:26657';

const client = new ChainClient({ restUrl: REST, rpcUrl: RPC });

const status = await client.status();
console.log(`chain      ${status.chainId} @ height ${formatNumber(status.latestHeight)}`);
console.log(`node       ${status.nodeVersion}, ${timeAgo(status.latestTime)}`);

const supply = await client.totalSupply();
console.log(`supply     ${formatCoins(supply)}`);

const names = await client.validatorNames();
console.log(`validators ${Object.values(names).join(', ') || 'none'}`);

const blocks = await client.recentBlocks(5);
console.log(`\nrecent blocks`);
for (const b of blocks) {
  console.log(`  #${b.height}  ${b.txCount} tx  ${timeAgo(b.timestamp)}`);
}

// Every transaction the chain has seen, decoded.
const txs = await client.searchTransactions("tx.height>0", 20, { names });
console.log(`\n${txs.length} transactions decoded\n`);
for (const tx of txs) {
  const status = tx.succeeded ? 'ok  ' : 'FAIL';
  console.log(`  ${status} #${tx.height}  ${tx.summary}`);
  if (!tx.succeeded && tx.error) {
    console.log(`        → ${tx.error.message}: ${tx.error.reason ?? ''}`);
  }
  for (const m of tx.messages) {
    const flag = m.everyday ? 'simple' : 'expert';
    console.log(`        [${flag}] ${m.title} · fee ${formatCoins(tx.fee)}`);
  }
}

// Anything the decoder did not recognise is worth knowing about.
const unknown = txs.flatMap((t) => t.messages).filter((m) => m.kind === 'other');
if (unknown.length > 0) {
  console.log(`\nundecoded message types:`);
  for (const m of new Set(unknown.map((u) => u.typeUrl))) console.log(`  ${m}`);
} else {
  console.log(`\nevery message type on this chain decodes to a sentence.`);
}
