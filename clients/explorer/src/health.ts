/**
 * Is the chain alive?
 *
 * This is the first question a central bank or a finance ministry asks when it
 * opens an explorer, and until this module existed the answer was a 10px grey
 * line in the corner of one card. It is now the page's furniture.
 *
 * Two things make the answer harder than reading a height.
 *
 * The first is that a height alone cannot say whether the chain is *moving*.
 * `34,928` looks identical whether the last block arrived two seconds ago or
 * last Tuesday, and an explorer that prints it without an age is inviting
 * somebody to trust a stopped network.
 *
 * The second is that a halted Cosmos node does not serve stale data — it
 * refuses. Every query is executed against the state left by the last block the
 * node finalised, so a node that is running but has finalised none answers
 * *every* REST request with an error naming the height it is stuck at:
 *
 *     invalid height: context did not contain latest block height in either
 *     check state or finalize block state (2733)
 *
 * That is the exact moment somebody opens this page — a chain halts because a
 * validator is gone — and it is the moment a page that reads "could not load
 * activity" is worth nothing. The height in the parentheses is the last
 * committed block, and asking for it explicitly through the
 * `x-cosmos-block-height` header makes the same query answer perfectly.
 *
 * `clients/foundation` established this; the regex and its guards are the same
 * rule, ported rather than shared because the foundation console is plain
 * JavaScript served without a build and is not importable from here.
 */

/** What the chain is doing, in the order of how much it should worry a reader. */
export type ChainState =
  | 'unknown'
  | 'live'
  | 'catching-up'
  | 'slow'
  | 'stopped'
  | 'unreachable';

/** How loudly the interface should say it. */
export type Severity = 'ok' | 'warn' | 'bad' | 'mute';

export interface ChainStatusFacts {
  chainId: string;
  latestHeight: number;
  /** RFC3339, as the node reports it. */
  latestTime: string;
  catchingUp: boolean;
}

export interface HealthInput {
  /** The RPC's own answer. Null when the RPC could not be reached at all. */
  status: ChainStatusFacts | null;
  /** Measured median seconds between blocks, when a sample was available. */
  blockSeconds?: number | null;
  /** Height a refusing node named, from `stalledAtHeight`. */
  stalledAt?: number | null;
  now?: Date;
}

export interface ChainHealth {
  state: ChainState;
  severity: Severity;
  height: number | null;
  /** Seconds since the last block, or null when there is no timestamp to use. */
  ageSeconds: number | null;
  /** What the interval is expected to be — measured where possible. */
  expectedSeconds: number;
  stalledAt: number | null;
  /**
   * True when every state-reading query on the page is answering from a fixed
   * past height rather than from the tip. Distinct from `stopped`: a node can
   * be stopped and still serve state, and can refuse state while blocks resume.
   */
  readingHistory: boolean;
}

/**
 * The interval to judge an age against when nothing has been measured.
 *
 * Five seconds is this chain's configured block time. Guessing low would make a
 * healthy chain look slow on the first paint, before the measurement lands.
 */
export const ASSUMED_BLOCK_SECONDS = 5;

/**
 * The height a stalled node is stuck at, read out of its own refusal.
 *
 * Returns null for anything else, and null for height 0 — a node that has never
 * committed a block has no state to show, and `x-cosmos-block-height: 0` means
 * "latest", which is the request that just failed.
 */
export function stalledAtHeight(body: unknown): number | null {
  const text = typeof body === 'string' ? body : JSON.stringify(body ?? '');
  const match = /context did not contain latest block height[^)]*\((\d+)\)/.exec(text);
  if (!match) return null;
  const height = Number(match[1]);
  return Number.isSafeInteger(height) && height > 0 ? height : null;
}

/**
 * Turns what the node said into what to tell a reader.
 *
 * The thresholds are multiples of the *measured* interval rather than fixed
 * seconds, because "late" means something different on a chain with one-second
 * blocks and a chain with thirty-second blocks. The floors underneath them stop
 * a fast chain from being called stopped over a single missed proposer.
 */
export function assessHealth(input: HealthInput): ChainHealth {
  const now = input.now ?? new Date();
  const stalledAt = input.stalledAt ?? null;
  const measured = input.blockSeconds && input.blockSeconds > 0 ? input.blockSeconds : null;
  const expectedSeconds = measured ?? ASSUMED_BLOCK_SECONDS;

  // No answer from the RPC at all. If a REST refusal named a height, the node
  // is up and the chain is halted; if nothing named anything, we cannot tell a
  // stopped chain from a broken connection and must not claim either.
  if (!input.status) {
    if (stalledAt !== null) {
      return {
        state: 'stopped',
        severity: 'bad',
        height: stalledAt,
        ageSeconds: null,
        expectedSeconds,
        stalledAt,
        readingHistory: true,
      };
    }
    return {
      state: 'unreachable',
      severity: 'bad',
      height: null,
      ageSeconds: null,
      expectedSeconds,
      stalledAt: null,
      readingHistory: false,
    };
  }

  const then = new Date(input.status.latestTime);
  const ageSeconds = Number.isNaN(then.getTime())
    ? null
    : Math.max(0, Math.floor((now.getTime() - then.getTime()) / 1000));

  const height = input.status.latestHeight;
  const readingHistory = stalledAt !== null;

  // A node that refuses every state query has not finalised a block, whatever
  // its RPC says about a height. Saying "live" over a page of historical
  // figures would be the single most misleading thing this interface could do.
  if (readingHistory) {
    return {
      state: 'stopped',
      severity: 'bad',
      height,
      ageSeconds,
      expectedSeconds,
      stalledAt,
      readingHistory: true,
    };
  }

  if (ageSeconds !== null && ageSeconds > Math.max(30, expectedSeconds * 8)) {
    return {
      state: 'stopped',
      severity: 'bad',
      height,
      ageSeconds,
      expectedSeconds,
      stalledAt: null,
      readingHistory: false,
    };
  }

  // Catching up is checked after stopped, because a replaying node whose last
  // block is a day old is a chain nobody should read as merely "syncing".
  if (input.status.catchingUp) {
    return {
      state: 'catching-up',
      severity: 'warn',
      height,
      ageSeconds,
      expectedSeconds,
      stalledAt: null,
      readingHistory: false,
    };
  }

  if (ageSeconds !== null && ageSeconds > Math.max(15, expectedSeconds * 3)) {
    return {
      state: 'slow',
      severity: 'warn',
      height,
      ageSeconds,
      expectedSeconds,
      stalledAt: null,
      readingHistory: false,
    };
  }

  return {
    state: 'live',
    severity: 'ok',
    height,
    ageSeconds,
    expectedSeconds,
    stalledAt: null,
    readingHistory: false,
  };
}

/**
 * How many validators can drop before the chain stops committing.
 *
 * Measured on this network: a three-validator chain stops when one node goes
 * down, because two thirds of the *power* is what a BFT commit needs and two of
 * three does not clear it. That belongs on the face of the page rather than
 * left for a reader to derive from a validator count.
 *
 * Takes the actual voting-power shares rather than a count, because stake here
 * is not evenly held — one validator on this chain holds a third of it. A
 * headline computed from `n` alone would claim a resilience the set does not
 * have, and would be wrong in the direction that matters.
 *
 * `shares` are fractions of bonded stake, in any order.
 */
export function faultTolerance(shares: number[]): {
  /** How many of the largest holders can be lost while blocks still commit. */
  spare: number;
  /** True when losing any single validator stops the chain. */
  fragile: boolean;
} {
  const sorted = [...shares].filter((s) => s > 0).sort((a, b) => b - a);
  if (sorted.length === 0) return { spare: 0, fragile: true };

  const total = sorted.reduce((sum, s) => sum + s, 0);
  const quorum = (2 / 3) * total;

  // Shares arrive as decimals the chain computed, so three equal thirds sum to
  // 0.9999999999999999 or to 1.0000000000000002 depending on the rounding, and
  // a bare `>` comparison decides whether a three-validator chain is fragile on
  // the last bit of a double. The tolerance is what stops that: an epsilon
  // wider than any float error and far narrower than any real stake difference.
  const EPSILON = 1e-9;

  let remaining = total;
  let spare = 0;
  for (const share of sorted) {
    // Strictly greater: a commit needs more than two thirds, not two thirds.
    if (remaining - share <= quorum + EPSILON) break;
    remaining -= share;
    spare += 1;
  }

  return { spare, fragile: spare < 1 };
}
