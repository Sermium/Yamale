/**
 * Reading the chain.
 *
 * A thin, typed wrapper over the node's REST and RPC endpoints. It exists so
 * that no interface has to know the shape of a Cosmos REST response, and so the
 * one place that does can be changed when the chain's API does.
 *
 * Everything here returns already-interpreted values where interpretation is
 * cheap and unambiguous — decoded messages, resolved denom metadata — and
 * leaves the raw payload attached for the expert view.
 */

import {
  decodeMessage,
  describeProposalAction,
  summariseTransaction,
  type DecodeContext,
  type DecodedMessage,
} from './decode.ts';
import {
  toPool,
  toProposal,
  toValidator,
  withTally,
  type Pool,
  type Proposal,
  type StakingOverview,
  type Validator,
} from './staking.ts';
import { KNOWN_DENOMS, type Coin, type DenomInfo } from './denom.ts';
import { normaliseUserId, validUserId } from './alias.ts';
import { toEnforcementCase, type EnforcementCase, type EnforcementVote } from './enforcement.ts';
import { measure, type BlockSample, type Performance } from './performance.ts';
import {
  toRoleAssignment,
  toTreasury,
  toTreasuryBalance,
  type RoleAssignment,
  type Treasury,
  type TreasuryBalance,
} from './treasury.ts';
import { describeTxResult, type TranslatedError } from './errors.ts';
import { DEFAULT_GOV_PARAMS, toGovParams, type GovParams } from './gov.ts';
import {
  DEFAULT_MAX_APPRAISAL_AGE,
  DEFAULT_MAX_RATE_AGE,
  toAppraisal,
  toRate,
  type Appraisal,
  type Rate,
} from './prices.ts';

export interface ChainClientOptions {
  /** Base URL of the node's REST (gRPC-gateway) API, e.g. http://localhost:1317 */
  restUrl: string;
  /** Base URL of the CometBFT RPC, e.g. http://localhost:26657 */
  rpcUrl: string;
  /** Overrides the global fetch, for tests. */
  fetchImpl?: typeof fetch;
}

export interface Transaction {
  hash: string;
  height: number;
  timestamp: string;
  /** 0 means success. */
  code: number;
  succeeded: boolean;
  error?: TranslatedError;
  gasUsed: number;
  gasWanted: number;
  fee: Coin[];
  memo: string;
  signers: string[];
  messages: DecodedMessage[];
  /** One-line description of the whole transaction. */
  summary: string;
  /** The untouched API payload, for the expert view. */
  raw: unknown;
}

/** Funds a treasury has committed to somebody, and what has become theirs. */
export interface Commitment {
  id: string;
  treasuryId: string;
  denom: string;
  /** Everything committed, in base units. */
  total: string;
  released: string;
  /** What could be claimed right now. */
  claimable: string;
  /** What has not yet been released, claimed or not. */
  remaining: string;
  vesting: boolean;
  /** True when the treasury may still cancel the unvested part. */
  revocable: boolean;
  active: boolean;
  cliffTime: number;
  endTime: number;
}

export interface Block {
  height: number;
  timestamp: string;
  hash: string;
  proposer: string;
  txCount: number;
  raw: unknown;
}

export interface ChainStatus {
  chainId: string;
  latestHeight: number;
  latestTime: string;
  /** Seconds between the two most recent blocks, when derivable. */
  blockTime?: number;
  nodeVersion: string;
  catchingUp: boolean;
}

export class ChainError extends Error {
  // Declared as plain fields rather than constructor parameter properties, so
  // the SDK stays runnable by Node's type-stripping without a build step.
  readonly status: number;
  readonly url: string;

  constructor(message: string, status: number, url: string) {
    super(message);
    this.name = 'ChainError';
    this.status = status;
    this.url = url;
  }
}

export class ChainClient {
  private readonly rest: string;
  private readonly rpc: string;
  private readonly fetchImpl: typeof fetch;
  private denomCache: Record<string, DenomInfo> | null = null;
  /** address -> user ID, or null for "asked, has none". See userIdOf. */
  private readonly userIds = new Map<string, string | null>();

  constructor(options: ChainClientOptions) {
    this.rest = options.restUrl.replace(/\/$/, '');
    this.rpc = options.rpcUrl.replace(/\/$/, '');
    this.fetchImpl = options.fetchImpl ?? globalThis.fetch.bind(globalThis);
  }

  private async get<T>(base: string, path: string): Promise<T> {
    const url = `${base}${path}`;
    const response = await this.fetchImpl(url);
    if (!response.ok) {
      throw new ChainError(`Request failed with ${response.status}`, response.status, url);
    }
    return (await response.json()) as T;
  }

  /** Chain identity and tip, for the header and health checks. */
  async status(): Promise<ChainStatus> {
    const data = await this.get<any>(this.rpc, '/status');
    const sync = data.result.sync_info;
    return {
      chainId: data.result.node_info.network,
      latestHeight: Number(sync.latest_block_height),
      latestTime: sync.latest_block_time,
      nodeVersion: data.result.node_info.version,
      catchingUp: Boolean(sync.catching_up),
    };
  }

  /**
   * Denom metadata from x/bank, layered over the built-in registry.
   *
   * x/stablecoin publishes metadata when governance approves an issuer, so a
   * currency registered after launch becomes readable without a client update.
   */
  async denomRegistry(): Promise<Record<string, DenomInfo>> {
    if (this.denomCache) return this.denomCache;

    const registry: Record<string, DenomInfo> = { ...KNOWN_DENOMS };
    try {
      const data = await this.get<any>(this.rest, '/cosmos/bank/v1beta1/denoms_metadata?pagination.limit=200');
      for (const meta of data.metadatas ?? []) {
        const display = (meta.denom_units ?? []).find((u: any) => u.denom === meta.display);
        registry[meta.base] = {
          base: meta.base,
          symbol: meta.symbol || meta.display || meta.base,
          exponent: Number(display?.exponent ?? 0),
          name: meta.name || meta.base,
        };
      }
    } catch {
      // Metadata is an enhancement; the built-in registry covers genesis denoms.
    }

    this.denomCache = registry;
    return registry;
  }

  async block(height: number | 'latest' = 'latest'): Promise<Block> {
    const path =
      height === 'latest'
        ? '/cosmos/base/tendermint/v1beta1/blocks/latest'
        : `/cosmos/base/tendermint/v1beta1/blocks/${height}`;
    const data = await this.get<any>(this.rest, path);
    return toBlock(data);
  }

  /** The most recent `count` blocks, newest first. */
  async recentBlocks(count = 12): Promise<Block[]> {
    const latest = await this.block('latest');
    const heights: number[] = [];
    for (let h = latest.height; h > Math.max(0, latest.height - count) ; h--) heights.push(h);

    const blocks = await Promise.all(
      heights.map((h) => (h === latest.height ? Promise.resolve(latest) : this.block(h).catch(() => null))),
    );
    return blocks.filter((b): b is Block => b !== null);
  }

  async transaction(hash: string, ctx?: DecodeContext): Promise<Transaction> {
    const data = await this.get<any>(this.rest, `/cosmos/tx/v1beta1/txs/${hash.toUpperCase()}`);
    const registry = ctx?.registry ?? (await this.denomRegistry());
    return toTransaction(data.tx, data.tx_response, { ...ctx, registry });
  }

  /**
   * Transactions matching a query, newest first.
   *
   * `events` uses the chain's own query syntax, e.g.
   * `message.sender='yml1…'`. The helpers below cover the common cases so
   * callers rarely have to write one.
   */
  async searchTransactions(events: string, limit = 25, ctx?: DecodeContext): Promise<Transaction[]> {
    const params = new URLSearchParams({
      query: events,
      'pagination.limit': String(limit),
      order_by: 'ORDER_BY_DESC',
    });
    const data = await this.get<any>(this.rest, `/cosmos/tx/v1beta1/txs?${params}`);
    const registry = ctx?.registry ?? (await this.denomRegistry());

    const responses: any[] = data.tx_responses ?? [];
    const txs: any[] = data.txs ?? [];
    return responses.map((response, i) => toTransaction(txs[i], response, { ...ctx, registry }));
  }

  /** Everything an account sent. */
  transactionsSentBy(address: string, limit = 25, ctx?: DecodeContext): Promise<Transaction[]> {
    return this.searchTransactions(`message.sender='${address}'`, limit, ctx);
  }

  /** Everything an account received, which is a separate index on the chain. */
  transactionsReceivedBy(address: string, limit = 25, ctx?: DecodeContext): Promise<Transaction[]> {
    return this.searchTransactions(`transfer.recipient='${address}'`, limit, ctx);
  }

  /** Transactions in a given block. */
  transactionsInBlock(height: number, limit = 50, ctx?: DecodeContext): Promise<Transaction[]> {
    return this.searchTransactions(`tx.height=${height}`, limit, ctx);
  }

  async balances(address: string): Promise<Coin[]> {
    const data = await this.get<any>(this.rest, `/cosmos/bank/v1beta1/balances/${address}`);
    return data.balances ?? [];
  }

  async totalSupply(): Promise<Coin[]> {
    const data = await this.get<any>(this.rest, '/cosmos/bank/v1beta1/supply');
    return data.supply ?? [];
  }

  async validators(): Promise<any[]> {
    const data = await this.get<any>(
      this.rest,
      '/cosmos/staking/v1beta1/validators?pagination.limit=200&status=BOND_STATUS_BONDED',
    );
    return data.validators ?? [];
  }

  /**
   * The validator set, interpreted: voting power as a share, commission as a
   * fraction, and a flag on any validator holding enough stake to stall the
   * chain alone.
   */
  async stakingOverview(): Promise<StakingOverview> {
    const [raw, pool, params, supply] = await Promise.all([
      this.validators(),
      this.get<any>(this.rest, '/cosmos/staking/v1beta1/pool').catch(() => null),
      this.get<any>(this.rest, '/cosmos/staking/v1beta1/params').catch(() => null),
      this.totalSupply().catch(() => [] as Coin[]),
    ]);

    const bonded = String(pool?.pool?.bonded_tokens ?? '0');
    const bondedInt = BigInt(bonded);
    const validators = raw
      .map((v) => toValidator(v, bondedInt))
      .sort((a, b) => b.votingPower - a.votingPower);

    const totalSupply = supply.find((c) => c.denom === (params?.params?.bond_denom ?? 'uyml'));
    const supplyInt = totalSupply ? BigInt(totalSupply.amount) : 0n;

    return {
      validators,
      bonded,
      bondedRatio: supplyInt > 0n ? Number((bondedInt * 10000n) / supplyInt) / 10000 : 0,
      unbondingSeconds: parseDuration(params?.params?.unbonding_time),
      inflationRate: null,
    };
  }

  /** Governance proposals, newest first, with their payloads described. */
  async proposals(limit = 50, ctx?: DecodeContext): Promise<Proposal[]> {
    const registry = ctx?.registry ?? (await this.denomRegistry());
    const data = await this.get<any>(
      this.rest,
      `/cosmos/gov/v1/proposals?pagination.limit=${limit}&pagination.reverse=true`,
    );
    const proposals: Proposal[] = (data.proposals ?? []).map((p: any) =>
      toProposal(p, (msg) => describeProposalAction(msg, { ...ctx, registry })),
    );

    // Only proposals still open need their tally fetched separately, and only
    // those cost an extra request. A closed proposal already carries its final
    // result, so re-querying every one of them would make the list slower the
    // longer the chain has been alive.
    return Promise.all(
      proposals.map(async (p) => {
        if (p.status !== 'voting') return p;
        try {
          const tally = await this.get<any>(this.rest, `/cosmos/gov/v1/proposals/${p.id}/tally`);
          return withTally(p, tally);
        } catch {
          return p;
        }
      }),
    );
  }

  async proposal(id: string, ctx?: DecodeContext): Promise<Proposal> {
    const registry = ctx?.registry ?? (await this.denomRegistry());
    const data = await this.get<any>(this.rest, `/cosmos/gov/v1/proposals/${id}`);
    return toProposal(data.proposal, (msg) => describeProposalAction(msg, { ...ctx, registry }));
  }

  /** The rules a proposal has to clear, so the interface can show progress toward them. */
  async govParams(): Promise<GovParams> {
    try {
      const data = await this.get<any>(this.rest, '/cosmos/gov/v1/params/tallying');
      const deposit = await this.get<any>(this.rest, '/cosmos/gov/v1/params/deposit').catch(() => null);
      return toGovParams({ params: { ...(data?.params ?? {}), ...(deposit?.params ?? {}) } });
    } catch {
      return DEFAULT_GOV_PARAMS;
    }
  }

  /** Every liquidity pool, with the price its reserves imply. */
  async pools(): Promise<Pool[]> {
    const registry = await this.denomRegistry();
    const data = await this.get<any>(this.rest, '/yamale/blockchain/amm/v1/pool');
    return (data.pool ?? []).map((p: any) => toPool(p, registry));
  }

  /**
   * Every agreed exchange rate, each with its age and whether the chain still
   * considers it usable.
   *
   * `now` comes from the chain's own latest block rather than the browser's
   * clock. A client whose clock has drifted would otherwise disagree with the
   * chain about whether a price is stale, and would do so silently.
   */
  async exchangeRates(): Promise<Rate[]> {
    const [data, params, status] = await Promise.all([
      this.get<any>(this.rest, '/yamale/blockchain/oracle/v1/rate?pagination.limit=100'),
      this.oracleParams(),
      this.status().catch(() => null),
    ]);

    const now = status ? Math.floor(new Date(status.latestTime).getTime() / 1000) : Math.floor(Date.now() / 1000);
    return (data.rates ?? []).map((r: any) => toRate(r, now, params.maxRateAgeSeconds));
  }

  // ------------------------------------------------------------ fee grants

  /**
   * Who is paying this account's network fees, if anyone.
   *
   * Worth showing in a wallet rather than leaving to a failed transaction. An
   * account holding only naira can transact if its institution sponsors it and
   * cannot if the allowance has lapsed, and those two states look identical
   * from a balance alone.
   */
  async feeAllowances(address: string): Promise<
    Array<{ granter: string; spendLimit: Coin[]; expiration: string | null }>
  > {
    try {
      const data = await this.get<any>(
        this.rest,
        `/cosmos/feegrant/v1beta1/allowances/${address}`,
      );
      return (data.allowances ?? []).map((a: any) => ({
        granter: a.granter ?? '',
        spendLimit: (a.allowance?.spend_limit ?? []).map((c: any) => ({
          denom: c.denom,
          amount: c.amount,
        })),
        expiration: a.allowance?.expiration ?? null,
      }));
    } catch {
      return [];
    }
  }

  // ------------------------------------------------------------- performance

  /**
   * Recent blocks, for measuring what the chain is actually doing.
   *
   * One RPC call rather than one per block: CometBFT's /blockchain endpoint
   * returns a run of block metas, and fetching sixty blocks individually would
   * take sixty round trips to answer a question about the last five minutes.
   * It returns at most 20 per call, so a wider window is stitched from a few.
   */
  async blockSamples(count = 20): Promise<BlockSample[]> {
    const status = await this.status();
    const latest = status.latestHeight;
    const samples: BlockSample[] = [];

    for (let top = latest; top > 1 && samples.length < count; top -= 20) {
      const min = Math.max(1, top - 19);
      const data = await this.get<any>(this.rpc, `/blockchain?minHeight=${min}&maxHeight=${top}`);
      for (const meta of data.result?.block_metas ?? []) {
        samples.push({
          height: Number(meta.header?.height ?? 0),
          time: new Date(meta.header?.time ?? 0),
          transactions: Number(meta.num_txs ?? 0),
        });
      }
    }

    return samples.slice(0, count);
  }

  /**
   * What the chain is doing right now, measured from those blocks.
   *
   * Twenty by default because that is exactly one call: CometBFT returns at
   * most twenty block metas per request, so sixty blocks cost three round trips
   * to make a median of nineteen gaps into a median of fifty-nine — a
   * steadiness nobody can see, paid for on every refresh.
   */
  async performance(count = 20): Promise<Performance | null> {
    return measure(await this.blockSamples(count));
  }

  /**
   * What this chain natively supports, with the live figure for each.
   *
   * Counts rather than claims. A capability with nothing behind it is reported
   * as zero rather than hidden — "no pools yet" is a true statement about a new
   * network and a more useful one than a feature list that cannot be checked.
   *
   * Every field falls back to zero on error instead of failing the page: a
   * chain missing one module should show the rest, not a blank screen.
   */
  async capabilities(): Promise<{
    currencies: number;
    pricedCurrencies: number;
    validators: number;
    treasuries: number;
    pools: number;
    participants: number;
    enforcementCases: number;
    proposals: number;
  }> {
    // Counting without fetching. Asking for 200 records to learn how many
    // there are pulled 12 KB of denom metadata to produce the number 42; asking
    // for one record with count_total pulls 350 bytes for the same answer.
    // Across the eight queries this page makes that is roughly 20 KB down to
    // under 2 KB per refresh, on a node that may be somebody's laptop.
    //
    // With one caveat, which is why this is not simply limit=1: a handler that
    // ignores count_total would report zero, and the page would confidently
    // show a chain with no currencies. So a total of zero *alongside a record
    // that exists* is treated as the handler failing to count rather than as an
    // empty set, and only then does it pay for the full fetch.
    const count = async (base: string, path: string, key: string): Promise<number> => {
      const listed = (data: any): number => (Array.isArray(data[key]) ? data[key].length : 0);

      try {
        const head = await this.get<any>(base, `${path}?pagination.limit=1&pagination.count_total=true`);
        const total = Number(head.pagination?.total ?? 0);

        if (Number.isFinite(total) && total > 0) return total;
        if (listed(head) === 0) return 0;

        // A record exists but the count came back zero: this handler does not
        // count, so fall back to counting them ourselves.
        const all = await this.get<any>(base, `${path}?pagination.limit=1000`);
        return listed(all);
      } catch {
        return 0;
      }
    };

    const [
      currencies,
      rates,
      validators,
      treasuries,
      pools,
      participants,
      cases,
      proposals,
    ] = await Promise.all([
      count(this.rest, '/cosmos/bank/v1beta1/denoms_metadata', 'metadatas'),
      count(this.rest, '/yamale/blockchain/oracle/v1/rate', 'rates'),
      count(this.rest, '/cosmos/staking/v1beta1/validators', 'validators'),
      count(this.rest, '/yamale/blockchain/treasury/v1/treasury', 'treasury'),
      count(this.rest, '/yamale/blockchain/amm/v1/pool', 'pool'),
      count(this.rest, '/yamale/blockchain/paymsg/v1/approved_participant', 'approved_participant'),
      count(this.rest, '/yamale/blockchain/enforcement/v1/case', 'case'),
      count(this.rest, '/cosmos/gov/v1/proposals', 'proposals'),
    ]);

    return {
      currencies,
      pricedCurrencies: rates,
      validators,
      treasuries,
      pools,
      participants,
      enforcementCases: cases,
      proposals,
    };
  }

  // -------------------------------------------------------------- treasuries
  //
  // The safe interface reads these. Balances come back split into total,
  // committed and available, because a treasurer shown one number will
  // eventually propose a payment out of funds that were already promised to
  // somebody else — and find out after collecting the approvals.

  /** Every treasury on the chain. */
  async treasuries(): Promise<Treasury[]> {
    const data = await this.get<any>(this.rest, '/yamale/blockchain/treasury/v1/treasury?pagination.limit=200');
    return (data.treasury ?? []).map(toTreasury);
  }

  /** One treasury, or null if there is no such id. */
  async treasury(id: string): Promise<Treasury | null> {
    try {
      const data = await this.get<any>(this.rest, `/yamale/blockchain/treasury/v1/treasury/${id}`);
      return toTreasury(data.treasury);
    } catch {
      return null;
    }
  }

  /** What a treasury holds, split into total, committed and available. */
  async treasuryBalances(id: string): Promise<TreasuryBalance[]> {
    const data = await this.get<any>(this.rest, `/yamale/blockchain/treasury/v1/treasury/${id}/balances`);
    return (data.balances ?? []).map(toTreasuryBalance);
  }

  /** Who holds which role in a treasury. */
  async treasuryRoles(id: string): Promise<RoleAssignment[]> {
    // The REST field is `role`, singular — the gateway names a repeated field
    // after the message it repeats, not after the list it produces.
    const data = await this.get<any>(this.rest, `/yamale/blockchain/treasury/v1/treasury/${id}/roles`);
    return (data.role ?? data.roles ?? []).map(toRoleAssignment);
  }

  /** The locks a treasury has created — its vesting schedules and escrows. */
  async treasuryLocks(id: string): Promise<Commitment[]> {
    const data = await this.get<any>(this.rest, `/yamale/blockchain/treasury/v1/treasury/${id}/locks?pagination.limit=200`);
    return (data.lock ?? data.locks ?? []).map((lock: any) => ({
      id: String(lock.id ?? '0'),
      treasuryId: String(lock.treasury_id ?? '0'),
      beneficiary: lock.beneficiary ?? '',
      denom: lock.denom ?? '',
      // total_amount and released_amount, not amount and claimed: the chain
      // names them for what they are, and a client that guessed would show
      // every commitment as zero.
      amount: String(lock.total_amount ?? '0'),
      claimed: String(lock.released_amount ?? '0'),
      claimable: '0',
      lockType: lock.lock_type ?? '',
      startTime: Number(lock.start_time ?? 0),
      cliffTime: Number(lock.cliff_time ?? 0),
      endTime: Number(lock.end_time ?? 0),
      revocable: Boolean(lock.revocable),
      // `active` is the chain's field; a revoked lock is an inactive one.
      revoked: lock.active === false,
    }));
  }

  /** How much of this period's spending allowance is left for a denom. */
  async spendCapacity(id: string, denom: string): Promise<{ remaining: string; limit: string } | null> {
    try {
      const data = await this.get<any>(
        this.rest,
        `/yamale/blockchain/treasury/v1/treasury/${id}/capacity?denom=${encodeURIComponent(denom)}`,
      );
      return { remaining: String(data.remaining ?? '0'), limit: String(data.limit ?? '0') };
    } catch {
      return null;
    }
  }

  // ------------------------------------------------------------ enforcement
  //
  // The freeze-and-seize power is only defensible if it is visible, so these
  // reads exist to put it on a public page rather than leaving it to whoever
  // thinks to run a CLI query.

  /** Every enforcement case ever opened, newest first. */
  async enforcementCases(): Promise<EnforcementCase[]> {
    const data = await this.get<any>(
      this.rest,
      '/yamale/blockchain/enforcement/v1/case?pagination.limit=200&pagination.reverse=true',
    );
    return (data.case ?? []).map(toEnforcementCase);
  }

  /** One case, with every vote cast on it. */
  async enforcementCase(id: string): Promise<{ case: EnforcementCase; votes: EnforcementVote[] } | null> {
    try {
      const data = await this.get<any>(this.rest, `/yamale/blockchain/enforcement/v1/case/${id}`);
      return {
        case: toEnforcementCase(data.case),
        votes: (data.votes ?? []).map((v: any) => ({
          validator: v.validator ?? '',
          option: v.option ?? '',
          power: Number(v.power ?? 0),
        })),
      };
    } catch {
      return null;
    }
  }

  /**
   * Whether an address may send, and if not, why.
   *
   * Returns null when the module is absent rather than throwing: an explorer
   * pointed at a chain without it should show an account normally, not an error
   * where the balance goes.
   */
  async freezeStatus(address: string): Promise<{ frozen: boolean; case: EnforcementCase | null } | null> {
    try {
      const data = await this.get<any>(this.rest, `/yamale/blockchain/enforcement/v1/freeze/${address}`);
      const frozen = Boolean(data.frozen);
      return { frozen, case: frozen && data.case?.id ? toEnforcementCase(data.case) : null };
    } catch {
      return null;
    }
  }

  /**
   * The user ID an address holds, or null.
   *
   * Null rather than a throw when there is none: "this account has no
   * identifier" is the ordinary case on a chain where registering is optional,
   * and a rejected promise would make every caller wrap a try/catch around a
   * fact.
   *
   * Cached for the life of the client. A binding is permanent by design — an
   * identifier is retired, never repointed — so re-asking the chain on every
   * render would be one request per address per row for an answer that cannot
   * change.
   */
  async userIdOf(address: string): Promise<string | null> {
    const hit = this.userIds.get(address);
    if (hit !== undefined) return hit;
    try {
      const data = await this.get<any>(this.rest, `/yamale/blockchain/alias/v1/alias_of/${address}`);
      const id = data?.alias?.id ?? null;
      this.userIds.set(address, id);
      return id;
    } catch {
      // Not registered, or the module is absent on this chain. The miss is
      // cached too, or an unregistered address is re-queried forever.
      this.userIds.set(address, null);
      return null;
    }
  }

  /**
   * Resolve a user ID to an address. The transfer app's hot path.
   *
   * Normalised and check-tested here rather than at each call site. Callers
   * were passing whatever was typed — lower case, hyphens, an I where a 1
   * belongs — and a mistyped ID reached the node and came back not-found, which
   * a person reads as "that account does not exist" rather than "you typed it
   * wrong". Anything that fails its own check character never leaves the
   * device.
   */
  async addressOfUserId(id: string): Promise<string | null> {
    if (!validUserId(id)) return null;
    try {
      const data = await this.get<any>(
        this.rest,
        `/yamale/blockchain/alias/v1/alias/${encodeURIComponent(normaliseUserId(id))}`,
      );
      return data?.alias?.address ?? null;
    } catch {
      return null;
    }
  }

  /**
   * The country recorded against an account, or null.
   *
   * Null is the ordinary answer for an account nobody has placed — which is
   * also an account that holds no user ID, because the chain refuses to issue
   * one without a jurisdiction.
   */
  async jurisdictionOf(address: string): Promise<string | null> {
    try {
      const data = await this.get<any>(
        this.rest,
        `/yamale/blockchain/alias/v1/jurisdiction/${address}`,
      );
      return data?.jurisdiction?.country ?? null;
    } catch {
      return null;
    }
  }

  /**
   * The accounts recorded in one country — an authority's own perimeter, and
   * nobody else's.
   *
   * Returns the jurisdiction records, not user IDs: the chain deliberately
   * offers no directory of identifiers, and this endpoint is not a way around
   * that.
   */
  async perimeter(
    country: string,
    limit = 100,
  ): Promise<{ address: string; country: string; recordedBy: string }[]> {
    try {
      const data = await this.get<any>(
        this.rest,
        `/yamale/blockchain/alias/v1/perimeter/${encodeURIComponent(country.toUpperCase())}?pagination.limit=${limit}`,
      );
      return (data?.jurisdictions ?? []).map((j: any) => ({
        address: j.address,
        country: j.country,
        recordedBy: j.recorded_by ?? '',
      }));
    } catch {
      return [];
    }
  }

  /** What the module has taken in total, and how often it has been used. */
  async enforcementTotals(): Promise<{ total: Coin[]; casesOpened: number; casesPassed: number } | null> {
    try {
      const data = await this.get<any>(this.rest, '/yamale/blockchain/enforcement/v1/recovered');
      return {
        total: (data.total ?? []).map((c: any) => ({ denom: c.denom, amount: c.amount })),
        casesOpened: Number(data.cases_opened ?? 0),
        casesPassed: Number(data.cases_passed ?? 0),
      };
    } catch {
      return null;
    }
  }

  /** The oracle's parameters, falling back to the module's own defaults. */
  async oracleParams(): Promise<{
    maxRateAgeSeconds: number;
    maxAppraisalAgeSeconds: number;
    quoteSymbol: string;
    votePeriod: number;
    acceptedDenoms: string[];
  }> {
    try {
      const data = await this.get<any>(this.rest, '/yamale/blockchain/oracle/v1/params');
      const p = data.params ?? {};
      return {
        maxRateAgeSeconds: Number(p.max_rate_age_seconds ?? DEFAULT_MAX_RATE_AGE),
        maxAppraisalAgeSeconds: Number(p.max_appraisal_age_seconds ?? DEFAULT_MAX_APPRAISAL_AGE),
        quoteSymbol: p.quote_symbol ?? 'USD',
        votePeriod: Number(p.vote_period ?? 0),
        acceptedDenoms: p.accepted_denoms ?? [],
      };
    } catch {
      return {
        maxRateAgeSeconds: DEFAULT_MAX_RATE_AGE,
        maxAppraisalAgeSeconds: DEFAULT_MAX_APPRAISAL_AGE,
        quoteSymbol: 'USD',
        votePeriod: 0,
        acceptedDenoms: [],
      };
    }
  }

  /** How reliably each validator has been reporting prices. */
  async missCounters(): Promise<Array<{ validator: string; misses: number; windows: number }>> {
    try {
      const data = await this.get<any>(this.rest, '/yamale/blockchain/oracle/v1/miss?pagination.limit=200');
      return (data.counters ?? []).map((c: any) => ({
        validator: c.validator ?? '',
        misses: Number(c.misses ?? 0),
        windows: Number(c.windows ?? 0),
      }));
    } catch {
      return [];
    }
  }

  /** The current valuation of one tokenised asset. */
  async appraisal(classId: string, nftId: string): Promise<Appraisal | null> {
    try {
      const [data, params, status] = await Promise.all([
        this.get<any>(this.rest, `/yamale/blockchain/oracle/v1/appraisal?class_id=${encodeURIComponent(classId)}&nft_id=${encodeURIComponent(nftId)}`),
        this.oracleParams(),
        this.status().catch(() => null),
      ]);

      const now = status ? Math.floor(new Date(status.latestTime).getTime() / 1000) : Math.floor(Date.now() / 1000);
      return toAppraisal(data.appraisal, now, params.maxAppraisalAgeSeconds, Boolean(data.appraiser_still_approved));
    } catch {
      return null;
    }
  }

  /** Every valuer the chain knows about, approved or not. */
  async appraisers(): Promise<any[]> {
    try {
      const data = await this.get<any>(this.rest, '/yamale/blockchain/oracle/v1/appraiser?pagination.limit=100');
      return data.appraisers ?? [];
    } catch {
      return [];
    }
  }

  /**
   * Funds committed to an address by a treasury, with what each would release
   * right now.
   *
   * Worth a separate query because these do not appear in a balance. Money
   * committed to somebody has left the treasury's spendable balance but has not
   * arrived in theirs, so an interface that only reads balances shows a person
   * with a vesting grant exactly nothing — which is the one case where they most
   * need to see something.
   */
  async commitmentsTo(address: string): Promise<Commitment[]> {
    try {
      const data = await this.get<any>(
        this.rest,
        `/yamale/blockchain/treasury/v1/beneficiary/${address}/locks?pagination.limit=100`,
      );

      const locks: any[] = data.lock ?? [];
      return Promise.all(
        locks.map(async (lock) => {
          let claimable = '0';
          let remaining = String(lock.total_amount ?? '0');
          try {
            const amounts = await this.get<any>(
              this.rest,
              `/yamale/blockchain/treasury/v1/lock/${lock.id}/claimable`,
            );
            claimable = String(amounts.claimable ?? '0');
            remaining = String(amounts.remaining ?? remaining);
          } catch {
            // The lock is still worth showing without its live figures.
          }

          return {
            id: String(lock.id ?? '0'),
            treasuryId: String(lock.treasury_id ?? '0'),
            denom: lock.denom ?? '',
            total: String(lock.total_amount ?? '0'),
            released: String(lock.released_amount ?? '0'),
            claimable,
            remaining,
            vesting: lock.lock_type === 'LOCK_TYPE_VESTING',
            revocable: Boolean(lock.revocable),
            active: Boolean(lock.active),
            cliffTime: Number(lock.cliff_time ?? 0),
            endTime: Number(lock.end_time ?? 0),
          };
        }),
      );
    } catch {
      return [];
    }
  }

  /** What an account has staked, and with whom. */
  async delegations(address: string): Promise<Array<{ validator: string; amount: Coin }>> {
    const data = await this.get<any>(this.rest, `/cosmos/staking/v1beta1/delegations/${address}`);
    return (data.delegation_responses ?? []).map((d: any) => ({
      validator: d.delegation?.validator_address ?? '',
      amount: d.balance,
    }));
  }

  /** Validator monikers keyed by operator address, for naming in decoded text. */
  async validatorNames(): Promise<Record<string, string>> {
    try {
      const list = await this.validators();
      const names: Record<string, string> = {};
      for (const v of list) {
        if (v.operator_address && v.description?.moniker) {
          names[v.operator_address] = v.description.moniker;
        }
      }
      return names;
    } catch {
      return {};
    }
  }
}

/** Parses a protobuf duration string such as "1814400s" into seconds. */
function parseDuration(value: string | undefined): number {
  if (!value) return 0;
  const seconds = Number(String(value).replace(/s$/, ''));
  return Number.isFinite(seconds) ? seconds : 0;
}

function toBlock(data: any): Block {
  const header = data.block?.header ?? data.sdk_block?.header ?? {};
  return {
    height: Number(header.height ?? 0),
    timestamp: header.time ?? '',
    hash: data.block_id?.hash ?? '',
    proposer: header.proposer_address ?? '',
    txCount: (data.block?.data?.txs ?? []).length,
    raw: data,
  };
}

function toTransaction(tx: any, response: any, ctx: DecodeContext): Transaction {
  const messages = (tx?.body?.messages ?? []).map((m: any) => decodeMessage(m, ctx));
  const code = Number(response?.code ?? 0);
  const result = describeTxResult(code, response?.raw_log);

  return {
    hash: response?.txhash ?? '',
    height: Number(response?.height ?? 0),
    timestamp: response?.timestamp ?? '',
    code,
    succeeded: result.ok,
    error: result.error,
    gasUsed: Number(response?.gas_used ?? 0),
    gasWanted: Number(response?.gas_wanted ?? 0),
    fee: tx?.auth_info?.fee?.amount ?? [],
    memo: tx?.body?.memo ?? '',
    signers: messages.map((m: DecodedMessage) => m.actor).filter((a: unknown): a is string => typeof a === 'string'),
    messages,
    summary: summariseTransaction(messages),
    raw: { tx, tx_response: response },
  };
}
