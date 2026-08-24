/**
 * The activity feed: what happened, to whose money, and whether it is finished.
 *
 * Before this module the feed was ten rows of identical weight, each carrying
 * the same icon, and the top of it read:
 *
 *     yml11d0dc0l…jka9 voted yes on shared-account request 2
 *     yml13duwtfp…utc2 voted yes on shared-account request 2
 *     yml15m9y2wd…wtfw voted yes on shared-account request 2
 *     Request 2 reached enough approvals and was carried out
 *
 * Four rows, one event, no amount, no names, and the only row that says what
 * actually changed says it least. Two things are wrong with that, and this
 * module fixes both.
 *
 * **A step is not an outcome.** A vote is a move inside a procedure that has
 * not concluded; an execution is the moment the thing became true. Presenting
 * them at the same weight is the same failure as printing a quote and a
 * settlement in the same typeface — the reader has to reconstruct the sequence
 * the interface already knew.
 *
 * **An execution knows more than its own message.** `MsgExec` carries a
 * proposal id and nothing else, so the SDK can only say "request 2 was carried
 * out". But the transaction's own events carry the tally and the executor's
 * result, and the request's *submission* — usually a few rows down the same
 * feed — carries the title and the inner messages that were run. Correlating
 * them turns that row into "Alice's country was recorded as KE — carried out
 * after 3 of 3 approvals", which is the sentence somebody actually opened the
 * page for.
 *
 * Everything here is a pure function of transactions the client already
 * fetched: no extra round trips, and no chain state that has since been pruned
 * (a group proposal is deleted the moment it executes, so its title is *only*
 * available this way).
 */

// Leaf imports rather than the `@yamale/chain` barrel: it re-exports two `.tsx`
// modules, which Node's type-stripping test runner cannot load. See amount.ts.
import {
  decodeMessage,
  shortTypeUrl,
  type DecodeContext,
  type DecodedDetail,
  type DecodedMessage,
  type MessageKind,
} from '../../sdk/src/decode.ts';
import { truncateAddress } from '../../sdk/src/format.ts';
import type { Coin } from '../../sdk/src/denom.ts';

/** Just enough of the SDK's Transaction for this module to work on. */
export interface FeedTransaction {
  hash: string;
  height: number;
  timestamp: string;
  succeeded: boolean;
  messages: DecodedMessage[];
  error?: { message?: string } | undefined;
  raw?: unknown;
}

/**
 * How finished a row is.
 *
 * `outcome` — something is now true that was not: money moved, an identity was
 * issued, an approved action ran. This is what the audience came for.
 *
 * `step`  — a move inside a procedure still in flight: a vote, a submission, an
 * application. Real, worth showing, but not an answer.
 *
 * `routine` — the chain administering itself: parameters, grants, feeder
 * delegations. Visible to an engineer, noise to a ministry.
 */
export type FeedTier = 'outcome' | 'step' | 'routine';

export interface Tally {
  yes: number;
  no: number;
  abstain: number;
  veto: number;
}

export interface ApprovalOutcome {
  proposalId: string;
  /** The request's own title, when its submission is in the same window. */
  title?: string;
  /** What it actually did, decoded from the submission's inner messages. */
  actions: string[];
  tally?: Tally;
  /** Whether the messages the proposal carried ran cleanly. */
  ran: 'success' | 'failure' | 'unknown';
}

export interface RequestReference {
  proposalId: string;
  title?: string;
  /** `yes`, `no`, `abstain`, `no with veto`. */
  option: string;
}

export interface FeedEntry {
  key: string;
  hash: string;
  height: number;
  timestamp: string;
  tier: FeedTier;
  kind: MessageKind;
  typeUrl: string;
  /** The sentence: who did what to how much, in human units. */
  headline: string;
  /** Value this message moved, in base units. Formatted at the display edge. */
  coins: Coin[];
  actor?: string;
  counterparty?: string;
  details?: DecodedDetail[];
  failed: boolean;
  /** Why it failed, in the chain's own words, for the disclosure. */
  failureReason?: string;
  /** Whether a non-technical reader has any use for this row. */
  everyday: boolean;
  /** Present on an executed shared-account request. */
  approval?: ApprovalOutcome;
  /** Present on a vote, naming what is being voted on. */
  request?: RequestReference;
  raw: unknown;
}

// --------------------------------------------------------------- decoders

/**
 * Message types the SDK's decoder does not cover yet.
 *
 * These are not a fork of it: `decode()` below calls the SDK first and only
 * consults this table for what came back as an undecoded fallback. Every entry
 * here is a module the live chain is using and the shared decoder has not
 * caught up with — the feed's top rows on this network are `x/alias` messages,
 * and "Set Jurisdiction on the alias module" is a type URL with the slashes
 * taken out, not a sentence.
 *
 * They belong upstream in `sdk/src/decode.ts` eventually. They are here because
 * that file is shared with five other live surfaces and this change is not.
 */
type LocalDecoder = (m: Record<string, any>, who: (a?: string) => string) => {
  kind: MessageKind;
  title: string;
  summary: string;
  everyday: boolean;
  actor?: string;
  counterparty?: string;
  details?: DecodedDetail[];
};

const LOCAL_DECODERS: Record<string, LocalDecoder> = {
  // ---- x/alias: who an account is, and which country holds it ------------
  '/blockchain.alias.v1.MsgRegisterAlias': (m, who) => ({
    kind: 'admin',
    title: 'Identity issued',
    summary: `${who(m.account)} was issued a Yamale user ID`,
    everyday: true,
    actor: m.account,
  }),
  '/blockchain.alias.v1.MsgRotateAlias': (m, who) => ({
    kind: 'admin',
    title: 'Identity replaced',
    summary: `${who(m.account)} replaced their user ID with a new one`,
    everyday: true,
    actor: m.account,
  }),
  '/blockchain.alias.v1.MsgSetJurisdiction': (m, who) => ({
    kind: 'governance',
    title: 'Country recorded',
    summary: `${who(m.account)}'s country was recorded as ${String(m.country ?? 'unknown')}`,
    everyday: true,
    actor: m.recorder,
    counterparty: m.account,
    details: [{ label: 'Recorded by', value: String(m.recorder ?? '—'), address: true }],
  }),
  '/blockchain.alias.v1.MsgAppointRegulator': (m, who) => ({
    kind: 'governance',
    title: 'Regulator appointed',
    summary: `${who(m.address)} was appointed regulator for ${String(m.country ?? 'a country')}`,
    everyday: false,
    counterparty: m.address,
  }),
  '/blockchain.alias.v1.MsgGrantAuditor': (m, who) => ({
    kind: 'governance',
    title: 'Auditor appointed',
    summary: `${who(m.address)} was granted audit access`,
    everyday: false,
    counterparty: m.address,
  }),
  '/blockchain.alias.v1.MsgRegisterViewingKey': (m, who) => ({
    kind: 'admin',
    title: 'Viewing key published',
    summary: `${who(m.account)} published a viewing key, so payment detail addressed to them can be encrypted`,
    everyday: false,
    actor: m.account,
  }),
  '/blockchain.alias.v1.MsgRevokeViewingKey': (m, who) => ({
    kind: 'admin',
    title: 'Viewing key withdrawn',
    summary: `${who(m.account)} withdrew a viewing key`,
    everyday: false,
    actor: m.account,
  }),

  // ---- x/enforcement: freezing and seizing --------------------------------
  '/blockchain.enforcement.v1.MsgOpenCase': (m, who) => ({
    kind: 'governance',
    title: 'Case opened',
    summary: `${who(m.opener)} opened an enforcement case against ${who(m.target)}`,
    everyday: true,
    actor: m.opener,
    counterparty: m.target,
    details: m.reason ? [{ label: 'Grounds given', value: String(m.reason) }] : undefined,
  }),
  '/blockchain.enforcement.v1.MsgVoteCase': (m, who) => ({
    kind: 'governance',
    title: 'Case vote',
    summary: `${who(m.voter)} voted on enforcement case ${String(m.id ?? '')}`.trim(),
    everyday: false,
    actor: m.voter,
  }),
  '/blockchain.enforcement.v1.MsgEmergencyFreeze': (m, who) => ({
    kind: 'governance',
    title: 'Account frozen',
    summary: `${who(m.target)}'s account was frozen immediately, pending a vote`,
    everyday: true,
    counterparty: m.target,
    details: m.reason ? [{ label: 'Grounds given', value: String(m.reason) }] : undefined,
  }),
  '/blockchain.enforcement.v1.MsgEmergencyRelease': (m, who) => ({
    kind: 'governance',
    title: 'Freeze lifted',
    summary: `An emergency freeze was lifted`,
    everyday: true,
    details: m.reason ? [{ label: 'Reason', value: String(m.reason) }] : undefined,
  }),
  '/blockchain.enforcement.v1.MsgSweep': (m, who) => ({
    kind: 'treasury',
    title: 'Funds seized',
    summary: `${who(m.sender)} carried out a seizure decided by an enforcement case`,
    everyday: true,
    actor: m.sender,
  }),
  '/blockchain.enforcement.v1.MsgOmbudsmanVeto': (m, who) => ({
    kind: 'governance',
    title: 'Case vetoed',
    summary: `The ombudsman vetoed an enforcement case`,
    everyday: true,
    details: m.reason ? [{ label: 'Reason', value: String(m.reason) }] : undefined,
  }),
  '/blockchain.enforcement.v1.MsgWithdrawCase': (m, who) => ({
    kind: 'governance',
    title: 'Case withdrawn',
    summary: `${who(m.opener)} withdrew their enforcement case`,
    everyday: false,
    actor: m.opener,
  }),

  // ---- validator housekeeping that reads as an outage to a reader ---------
  '/cosmos.slashing.v1beta1.MsgUnjail': (m, who) => ({
    kind: 'staking',
    title: 'Validator back in',
    summary: `${who(m.validator_addr)} returned to the validator set after being jailed`,
    everyday: false,
    actor: m.validator_addr,
  }),
};

/**
 * Whether the SDK produced a real interpretation or gave up.
 *
 * `fallbackDecode` marks its output with kind `other`, which is the signal —
 * there is no message on this chain whose genuine kind is "other".
 */
function undecoded(message: DecodedMessage): boolean {
  return message.kind === 'other';
}

/**
 * The SDK's decoder, with the explorer's own table behind it.
 *
 * Called on the raw payload rather than on the `DecodedMessage` the client
 * already produced, and that is deliberate: the client decodes on the way in,
 * before the page has had a chance to resolve any account to a name. Re-decoding
 * here — where the name map is complete — is what lets a sentence read
 * "KE-M1BM-Z66Y-P sent 5 YML to …" instead of naming one party by its bech32
 * prefix while a chip two lines down shows the same account's user ID.
 *
 * Cheap enough to do on every render: forty transactions of string
 * concatenation, against a network round trip saved.
 */
export function decode(raw: Record<string, any>, ctx: DecodeContext = {}): DecodedMessage {
  const message = decodeMessage(raw, ctx);
  if (!undecoded(message)) return message;

  const local = LOCAL_DECODERS[message.typeUrl];
  if (!local) return message;

  const who = (address?: string) =>
    !address ? 'someone' : (ctx.names?.[address] ?? truncateAddress(address));

  return { ...message, ...local(raw, who) };
}

// -------------------------------------------------------------- tiering

/**
 * Messages that move a procedure along without concluding it.
 *
 * Listed explicitly rather than inferred, because the distinction is a product
 * judgement and not a property of the proto. A `MsgVote` and a `MsgExec` have
 * the same shape, the same module and the same signer; only one of them means
 * anything happened.
 */
const STEP_TYPES = new Set([
  '/cosmos.group.v1.MsgVote',
  '/cosmos.group.v1.MsgSubmitProposal',
  '/cosmos.group.v1.MsgWithdrawProposal',
  '/cosmos.gov.v1.MsgVote',
  '/cosmos.gov.v1.MsgVoteWeighted',
  '/cosmos.gov.v1.MsgSubmitProposal',
  '/cosmos.gov.v1.MsgDeposit',
  '/blockchain.enforcement.v1.MsgVoteCase',
  '/blockchain.enforcement.v1.MsgOpenCase',
  '/blockchain.paymsg.v1.MsgApplyParticipant',
  '/blockchain.stablecoin.v1.MsgApplyIssuer',
  '/blockchain.validatorgov.v1.MsgApplyValidator',
  '/blockchain.oracle.v1.MsgApplyAppraiser',
  '/blockchain.builderfee.v1.MsgRegisterBuilder',
]);

/**
 * Where a row sits between "the chain is administering itself" and "somebody's
 * money moved".
 *
 * A failure is never an outcome whatever it attempted: nothing became true, and
 * the row's job is to say a thing was refused.
 */
export function tierOf(message: DecodedMessage, failed = false): FeedTier {
  if (STEP_TYPES.has(message.typeUrl)) return 'step';
  if (message.typeUrl.endsWith('.MsgUpdateParams')) return 'routine';
  if (failed) return 'step';
  if (message.kind === 'admin') return message.everyday ? 'outcome' : 'routine';
  if (message.coins && message.coins.length > 0) return 'outcome';
  if (message.everyday) return 'outcome';
  return 'routine';
}

// --------------------------------------------------------- chain events

interface RawEvent {
  type?: string;
  attributes?: Array<{ key?: string; value?: string }>;
}

function eventsOf(tx: FeedTransaction): RawEvent[] {
  const raw = tx.raw as { tx_response?: { events?: RawEvent[] } } | undefined;
  return raw?.tx_response?.events ?? [];
}

/**
 * One attribute out of one event.
 *
 * Attribute values arrive JSON-encoded — the id `2` comes over the wire as the
 * four characters `"2"` — so the quotes come off here rather than in four
 * call sites that each forget a different one.
 */
function attr(events: RawEvent[], type: string, key: string): string | undefined {
  for (const event of events) {
    if (event.type !== type) continue;
    for (const a of event.attributes ?? []) {
      if (a.key === key && typeof a.value === 'string') {
        return a.value.replace(/^"|"$/g, '');
      }
    }
  }
  return undefined;
}

function parseTally(value: string | undefined): Tally | undefined {
  if (!value) return undefined;
  try {
    const raw = JSON.parse(value) as Record<string, string>;
    const n = (k: string) => Number(raw[k] ?? 0) || 0;
    return {
      yes: n('yes_count'),
      no: n('no_count'),
      abstain: n('abstain_count'),
      veto: n('no_with_veto_count'),
    };
  } catch {
    return undefined;
  }
}

// ------------------------------------------------------------ the feed

interface SubmittedRequest {
  proposalId: string;
  title?: string;
  actions: string[];
  coins: Coin[];
}

/**
 * Requests submitted anywhere in this window, indexed by the id the chain gave
 * them.
 *
 * The id is not in the message — a submission does not know its own id — it is
 * in the transaction's `EventSubmitProposal`. This is the only place the title
 * of an executed request survives at all: `x/group` prunes a proposal the
 * moment it runs, so by the time the execution is on screen there is nothing
 * left to query.
 */
function indexRequests(txs: FeedTransaction[], ctx: DecodeContext): Map<string, SubmittedRequest> {
  const index = new Map<string, SubmittedRequest>();

  for (const tx of txs) {
    if (!tx.succeeded) continue;
    const id = attr(eventsOf(tx), 'cosmos.group.v1.EventSubmitProposal', 'proposal_id');
    if (!id) continue;

    for (const message of tx.messages) {
      if (message.typeUrl !== '/cosmos.group.v1.MsgSubmitProposal') continue;
      const raw = message.raw as Record<string, any>;
      const inner: Array<Record<string, any>> = Array.isArray(raw.messages) ? raw.messages : [];
      const decoded = inner.map((m) => decode(m, ctx));

      index.set(id, {
        proposalId: id,
        title: typeof raw.title === 'string' && raw.title ? raw.title : undefined,
        actions: decoded.map((d) => d.summary),
        coins: decoded.flatMap((d) => d.coins ?? []),
      });
    }
  }

  return index;
}

/**
 * A shared-account request, described by what it would do rather than by its
 * number.
 *
 * "Approval requested for an action on a shared account" is a sentence with no
 * information in it. The request carries a title its proposer wrote and the
 * messages it would run, and both are readable.
 */
function describeSubmission(request: SubmittedRequest | undefined, fallback: string): string {
  if (!request) return fallback;
  const what = request.title ?? request.actions[0];
  if (!what) return fallback;
  return `Approval requested: ${what}`;
}

/**
 * Turns transactions into rows, in the order the chain produced them.
 *
 * One row per message, because a transaction carrying three payments is three
 * things that happened to three people and collapsing them loses two.
 */
export function buildFeed(txs: FeedTransaction[], ctx: DecodeContext = {}): FeedEntry[] {
  const requests = indexRequests(txs, ctx);
  const entries: FeedEntry[] = [];

  for (const tx of txs) {
    const events = eventsOf(tx);

    tx.messages.forEach((incoming, index) => {
      const message = decode(incoming.raw as Record<string, any>, ctx);
      const failed = !tx.succeeded;
      const entry: FeedEntry = {
        key: `${tx.hash}-${index}`,
        hash: tx.hash,
        height: tx.height,
        timestamp: tx.timestamp,
        tier: tierOf(message, failed),
        kind: message.kind,
        typeUrl: message.typeUrl,
        headline: message.summary,
        coins: message.coins ?? [],
        actor: message.actor,
        counterparty: message.counterparty,
        details: message.details,
        failed,
        failureReason: tx.error?.message,
        everyday: message.everyday,
        raw: message.raw,
      };

      // A shared-account execution: the one row on this page that says a thing
      // became true, and the one the SDK could say least about.
      if (message.typeUrl === '/cosmos.group.v1.MsgExec' && !failed) {
        const id = String((message.raw as Record<string, any>).proposal_id ?? '');
        const request = requests.get(id);
        const result = attr(events, 'cosmos.group.v1.EventExec', 'result');
        const tally = parseTally(
          attr(events, 'cosmos.group.v1.EventProposalPruned', 'tally_result'),
        );

        entry.approval = {
          proposalId: id,
          title: request?.title,
          actions: request?.actions ?? [],
          tally,
          ran:
            result === 'PROPOSAL_EXECUTOR_RESULT_SUCCESS'
              ? 'success'
              : result === 'PROPOSAL_EXECUTOR_RESULT_FAILURE'
                ? 'failure'
                : 'unknown',
        };

        // Name the effect, not the procedure. Prefer what it did over what it
        // was called: a title is somebody's note to their co-signers, the
        // decoded action is what the chain executed.
        const what = request?.actions[0] ?? request?.title;
        if (what) entry.headline = what;
        if (request) entry.coins = request.coins;
        // An execution whose inner messages failed is not an outcome. The
        // approval passed and the action did not run.
        if (entry.approval.ran === 'failure') entry.tier = 'step';
      }

      if (message.typeUrl === '/cosmos.group.v1.MsgSubmitProposal') {
        const id = attr(events, 'cosmos.group.v1.EventSubmitProposal', 'proposal_id');
        const request = id ? requests.get(id) : undefined;
        entry.headline = describeSubmission(request, message.summary);
        if (request) {
          entry.coins = request.coins;
          entry.request = { proposalId: request.proposalId, title: request.title, option: '' };
        }
      }

      if (message.typeUrl === '/cosmos.group.v1.MsgVote') {
        const raw = message.raw as Record<string, any>;
        const id = String(raw.proposal_id ?? '');
        const request = requests.get(id);
        entry.request = {
          proposalId: id,
          title: request?.title ?? request?.actions[0],
          option: voteWord(raw.option),
        };
      }

      entries.push(entry);
    });
  }

  return entries;
}

/** The enum name as a word. `VOTE_OPTION_NO_WITH_VETO` → `no with veto`. */
export function voteWord(option: unknown): string {
  const map: Record<string, string> = {
    VOTE_OPTION_YES: 'yes',
    VOTE_OPTION_NO: 'no',
    VOTE_OPTION_ABSTAIN: 'abstain',
    VOTE_OPTION_NO_WITH_VETO: 'no with veto',
    VOTE_OPTION_ONE: 'yes',
    VOTE_OPTION_TWO: 'abstain',
    VOTE_OPTION_THREE: 'no',
    VOTE_OPTION_FOUR: 'no with veto',
  };
  return map[String(option)] ?? String(option ?? '').toLowerCase().replace(/_/g, ' ');
}

/**
 * The everyday feed: what a person who is not an engineer came to see.
 *
 * Routine rows go, and so do the steps of a procedure whose outcome is already
 * in the window — three "voted yes" rows above "the transfer was made" is the
 * procedure explaining itself to somebody who asked what happened to their
 * money. The outcome stays and carries the tally, so nothing is lost.
 */
export function everydayFeed(entries: FeedEntry[]): FeedEntry[] {
  const settled = new Set(
    entries.filter((e) => e.approval && !e.failed).map((e) => e.approval!.proposalId),
  );

  return entries.filter((entry) => {
    if (entry.tier === 'routine') return false;
    if (!entry.everyday && !entry.failed) return false;
    if (entry.request && settled.has(entry.request.proposalId)) return false;
    return true;
  });
}

/** `MsgSendPayment` → `Send Payment`, for the type badge in the expert view. */
export function messageLabel(typeUrl: string): string {
  return shortTypeUrl(typeUrl)
    .replace(/^Msg/, '')
    .replace(/([a-z])([A-Z])/g, '$1 $2');
}
