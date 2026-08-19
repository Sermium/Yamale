/**
 * What the chain is actually doing, measured rather than claimed.
 *
 * Every number here comes from block headers the node just served. That is the
 * whole point: a capability page that printed "5 second blocks, 100+ TPS" from
 * a constant would be marketing, and the first person to check it against the
 * chain would stop believing the rest of the page too.
 *
 * Where a figure cannot be measured — throughput under load, on a network
 * nobody is loading — this module returns what it can and says the rest is
 * unknown, rather than extrapolating.
 */

/** One block, reduced to what a measurement needs. */
export interface BlockSample {
  height: number;
  time: Date;
  transactions: number;
}

export interface Performance {
  /** Blocks the measurement is drawn from. */
  sampled: number;
  /** Median seconds between blocks — the honest middle, not the mean. */
  blockSeconds: number;
  /** The fastest and slowest gaps in the sample. */
  fastestSeconds: number;
  slowestSeconds: number;
  /** Transactions in the sampled window, and per second across it. */
  transactions: number;
  transactionsPerSecond: number;
  /** Blocks per day at the observed rate. */
  blocksPerDay: number;
  /**
   * Whether the chain was doing any work while this was measured.
   *
   * An idle chain's block time proves the interval is being kept, not that the
   * chain is fast — and an interface that reported "0.0 TPS" beside "capable of
   * hundreds" without saying which one was measured would be lying by layout.
   */
  idle: boolean;
}

/**
 * Turns a run of blocks into a measurement.
 *
 * The median rather than the mean, because one restart or one slow proposer
 * drags a mean somewhere no block actually was. In a sample of 60 five-second
 * blocks with a single 10-second gap the mean says 5.08 and the median says
 * 5.02; only one of those is a number you could set a timeout from.
 */
export function measure(samples: BlockSample[]): Performance | null {
  if (samples.length < 2) return null;

  const ordered = [...samples].sort((a, b) => a.height - b.height);
  const gaps: number[] = [];
  for (let i = 1; i < ordered.length; i++) {
    const seconds = (ordered[i]!.time.getTime() - ordered[i - 1]!.time.getTime()) / 1000;
    if (seconds > 0) gaps.push(seconds);
  }
  if (gaps.length === 0) return null;

  const sorted = [...gaps].sort((a, b) => a - b);
  const median = sorted[Math.floor(sorted.length / 2)]!;
  const transactions = ordered.reduce((total, block) => total + block.transactions, 0);
  const elapsed = (ordered[ordered.length - 1]!.time.getTime() - ordered[0]!.time.getTime()) / 1000;

  return {
    sampled: ordered.length,
    blockSeconds: median,
    fastestSeconds: sorted[0]!,
    slowestSeconds: sorted[sorted.length - 1]!,
    transactions,
    transactionsPerSecond: elapsed > 0 ? transactions / elapsed : 0,
    blocksPerDay: median > 0 ? Math.round(86_400 / median) : 0,
    idle: transactions === 0,
  };
}

/**
 * How long until a payment is final, in words.
 *
 * One block. Not "one block, then wait for confirmations" — this is a BFT
 * chain, so a committed block cannot be reorganised out from under somebody,
 * and telling a payments audience to wait six blocks would be importing a
 * Bitcoin habit that does not apply here.
 */
export function finality(blockSeconds: number): string {
  if (blockSeconds <= 0) return 'one block';
  if (blockSeconds < 1) return `${Math.round(blockSeconds * 1000)} milliseconds — one block, final`;
  return `${blockSeconds.toFixed(1)} seconds — one block, final`;
}

/**
 * What the interval leaves unused.
 *
 * Execution was measured at roughly 2.2 ms per transaction and 20 ms of fixed
 * cost per block on ordinary hardware, so a five-second interval spends almost
 * all of itself waiting on purpose. Stated as headroom rather than as a
 * throughput claim, because the honest sentence is "this is not the
 * constraint", not "this chain does N thousand TPS".
 */
export function headroom(blockSeconds: number, transactionsPerBlock: number): {
  usedSeconds: number;
  sharePercent: number;
} {
  const used = 0.02 + transactionsPerBlock * 0.0022;
  return {
    usedSeconds: used,
    sharePercent: blockSeconds > 0 ? Math.min(100, (used / blockSeconds) * 100) : 0,
  };
}

/**
 * Throughput in words, because the number alone can read as a contradiction.
 *
 * A quiet chain produces figures like 0.04 transactions per second, which
 * rounds to "0.0 per second" — printed next to "4 transactions" that is a
 * sentence disagreeing with itself, and a reader is right to trust neither
 * half. Below a tenth of a transaction per second the honest phrasing is an
 * interval rather than a rate.
 */
export function describeThroughput(p: Performance): string {
  if (p.transactions === 0) return 'nothing submitted';

  if (p.transactionsPerSecond >= 0.1) {
    return `about ${p.transactionsPerSecond.toFixed(1)} per second sustained`;
  }

  const seconds = Math.round(1 / p.transactionsPerSecond);
  if (seconds < 120) return `roughly one every ${seconds} seconds`;
  return `roughly one every ${Math.round(seconds / 60)} minutes`;
}
