/**
 * What an operator has published against a tokenisation, in the terms a holder
 * decides in.
 *
 * Everything here is pure and takes the chain's own values. The one judgement
 * this file makes is the one the module's own proto comment insists on:
 *
 *   "A general assembly notice with a date is something a holder must ACT on
 *    and can miss; a set of accounts is something they read. A register that
 *    filed both as 'document' makes the one with a deadline as easy to overlook
 *    as the one without."
 *
 * So a covenant is sorted, toned and described by whether it carries a
 * deadline, not only by what kind of filing it is. Nothing here formats a date
 * or a duration — that is duration.ts, at the display boundary.
 */

/* ------------------------------------------------------------ chain shapes */

/** blockchain.tokenisation.v1.CovenantKind, as the wire carries it. */
export type CovenantKind =
  | 'COVENANT_KIND_UNSPECIFIED'
  | 'COVENANT_KIND_FINANCIAL_STATEMENTS'
  | 'COVENANT_KIND_STRUCTURE_CHANGE'
  | 'COVENANT_KIND_DIVIDEND_NOTICE'
  | 'COVENANT_KIND_GENERAL_ASSEMBLY'
  | 'COVENANT_KIND_RESOLUTION'
  | 'COVENANT_KIND_INDEPENDENT_REPORT'
  | 'COVENANT_KIND_MATERIAL_EVENT';

export interface Covenant {
  id: string;
  assetId: string;
  kind: CovenantKind;
  title: string;
  /** SHA-256, lower-case hex, 64 characters. Empty when the chain held none. */
  digest: string;
  uri: string;
  /** Unix seconds. Both zero for a filing that covers a moment, not a span. */
  periodStart: number;
  periodEnd: number;
  /** Unix seconds. Zero when nothing is required of a holder. */
  actsBy: number;
  publishedBy: string;
  publishedAt: number;
  /** Covenant ids. '0' means none — see isRealId in vehicle.ts. */
  supersedes: string;
  supersededBy: string;
}

/* ------------------------------------------------------------- the digest */

/**
 * The digest as lower-case hex.
 *
 * Written by hand rather than through a helper because the empty case has to
 * survive: a covenant the chain refused to write without 32 bytes can still
 * arrive here with none through a decode that dropped the field, and a helper
 * that threw would take the whole register down with it.
 */
export function toHex(bytes: Uint8Array | undefined | null): string {
  if (!bytes || bytes.length === 0) return '';
  let out = '';
  for (const b of bytes) out += b.toString(16).padStart(2, '0');
  return out;
}

/**
 * A digest short enough to sit on one line, with both ends kept.
 *
 * Both ends rather than a prefix: a prefix-only truncation is the one a forger
 * would attack, because matching the first eight characters of a hash is the
 * only part of it anybody ever eyeballs. Keeping the tail means the two strings
 * a reader compares differ in a place they are actually looking.
 *
 * This is a LABEL and never a value. The whole digest has to stay reachable
 * wherever this is used, because a truncated digest cannot be compared against
 * anything — see `verifyCommand`.
 */
export function shortDigest(hex: string, keep = 8): string {
  if (hex.length <= keep * 2 + 1) return hex;
  return `${hex.slice(0, keep)}…${hex.slice(-keep)}`;
}

/**
 * The command that actually checks a file against the chain's digest.
 *
 * `sha256sum --check` rather than a bare `sha256sum`, because a bare one prints
 * a hash and leaves the comparison to a human eye reading 64 hex characters —
 * which is the comparison nobody performs correctly and most people skip. This
 * form exits non-zero when the file is not the filing, and says so in words.
 *
 * The filename is a placeholder the reader replaces. It is deliberately taken
 * from the covenant's uri when there is one, so the command is closer to
 * runnable and the reader can see which file is meant.
 */
export function verifyCommand(hex: string, uri: string): string {
  const name = fileNameOf(uri) || 'document.pdf';
  return `echo "${hex}  ${name}" | sha256sum --check`;
}

/** The last path segment of a uri, when it has one that looks like a file. */
export function fileNameOf(uri: string): string {
  if (!uri) return '';
  // Query strings and fragments are not part of a name, and a trailing slash
  // means the uri names a directory rather than a document.
  const stripped = uri.split('#')[0]!.split('?')[0]!;
  const last = stripped.split('/').pop() ?? '';
  return /^[\w.\-+]{1,128}$/.test(last) ? last : '';
}

/* ------------------------------------------------------------ the deadline */

export type DeadlineState =
  /** Nothing is required of a holder. Most filings. */
  | 'none'
  /** A date is set and it has not arrived. This is the one that can be missed. */
  | 'open'
  /** The date has passed. Kept visible: it is why a holder lost a vote. */
  | 'passed';

export interface Deadline {
  state: DeadlineState;
  /** Seconds until `acts_by`; 0 once it has passed. */
  remainingSeconds: number;
  /** Unix seconds, as the chain holds it. 0 when there is no deadline. */
  at: number;
}

/**
 * Whether this filing is something a holder can miss.
 *
 * Keyed on `acts_by` rather than on the kind, because `acts_by` is the field
 * the chain provides for exactly this and the kind only correlates with it. An
 * assembly notice is the usual carrier, but a material event can set one too —
 * a tender with a closing date — and a resolution recorded after the fact
 * carries none even though it came out of an assembly.
 *
 * `now` is passed in so a caller can drive this from the chain's block time
 * rather than the reader's laptop. A deadline counted down against a clock four
 * minutes fast tells somebody they have missed a vote they have not missed.
 */
export function deadlineOf(covenant: Pick<Covenant, 'actsBy'>, nowSeconds: number): Deadline {
  const at = covenant.actsBy;
  if (!Number.isFinite(at) || at <= 0) return { state: 'none', remainingSeconds: 0, at: 0 };

  const remaining = at - nowSeconds;
  // Inclusive of its final instant, matching every other window on this
  // surface: closing a deadline a second early tells somebody they have missed
  // something they have not.
  if (remaining < 0) return { state: 'passed', remainingSeconds: 0, at };
  return { state: 'open', remainingSeconds: remaining, at };
}

/** True when this filing has been corrected by a later one. */
export function isSuperseded(covenant: Pick<Covenant, 'supersededBy'>): boolean {
  const id = (covenant.supersededBy ?? '0').trim();
  return /^\d+$/.test(id) && BigInt(id) > 0n;
}

/* ---------------------------------------------------------------- in words */

/** Catalogue key for what kind of filing this is. */
export function kindKey(kind: CovenantKind): string {
  switch (kind) {
    case 'COVENANT_KIND_FINANCIAL_STATEMENTS': return 'rwa.cov.kind.accounts';
    case 'COVENANT_KIND_STRUCTURE_CHANGE': return 'rwa.cov.kind.structure';
    case 'COVENANT_KIND_DIVIDEND_NOTICE': return 'rwa.cov.kind.dividend';
    case 'COVENANT_KIND_GENERAL_ASSEMBLY': return 'rwa.cov.kind.assembly';
    case 'COVENANT_KIND_RESOLUTION': return 'rwa.cov.kind.resolution';
    case 'COVENANT_KIND_INDEPENDENT_REPORT': return 'rwa.cov.kind.report';
    case 'COVENANT_KIND_MATERIAL_EVENT': return 'rwa.cov.kind.material';
    default: return 'rwa.cov.kind.unknown';
  }
}

/**
 * Catalogue key for what this filing ASKS OF THE READER.
 *
 * Separate from the kind, and this is the distinction the whole panel exists to
 * draw. "Annual accounts" says what a document is; "to read" and "you must act
 * by this date" say what happens if it is ignored, and only the second of those
 * costs a holder anything.
 */
export function obligationKey(state: DeadlineState): string {
  switch (state) {
    case 'open': return 'rwa.cov.mustAct';
    case 'passed': return 'rwa.cov.deadlinePassed';
    default: return 'rwa.cov.toRead';
  }
}

/**
 * The tone a filing carries.
 *
 * A live deadline is `warn` whatever it is about, and that is deliberate: the
 * colour is carrying "this one has a clock on it", not "this one is bad news".
 * A superseded filing is muted wherever it would otherwise have been loud,
 * because a corrected deadline is not a deadline.
 */
export function covenantTone(
  covenant: Pick<Covenant, 'supersededBy'>,
  deadline: Deadline,
): 'ok' | 'warn' | 'bad' | 'mute' {
  if (isSuperseded(covenant)) return 'mute';
  if (deadline.state === 'open') return 'warn';
  if (deadline.state === 'passed') return 'mute';
  return 'ok';
}

/* ----------------------------------------------------------------- the order */

/**
 * The register, ordered so the thing that can be missed cannot be.
 *
 * The chain returns filings oldest first, "which is the order the vehicle's
 * life happened in" — correct for an archive and wrong for a page somebody
 * opens to find out what is required of them. A holder scrolling a chronology
 * to discover an assembly is next Tuesday has been failed by the ordering.
 *
 * So: live deadlines first, soonest first, because those are the only items
 * here with a cost attached to not reading them. Everything else follows,
 * newest first, which is how a register is read. Superseded filings sink within
 * their group rather than being pulled out — they stay in the record, they just
 * stop competing for attention with the version that replaced them.
 *
 * Not filtered. `current_only` exists on the query and is deliberately not used:
 * a dispute is usually about what was said at a date, and a reader who can only
 * see the current version cannot answer that.
 */
export function orderCovenants(covenants: Covenant[], nowSeconds: number): Covenant[] {
  const rank = (c: Covenant) => {
    if (isSuperseded(c)) return 2;
    return deadlineOf(c, nowSeconds).state === 'open' ? 0 : 1;
  };

  return [...covenants].sort((a, b) => {
    const ra = rank(a);
    const rb = rank(b);
    if (ra !== rb) return ra - rb;
    // Soonest deadline first inside the live group; newest filing first in the
    // others. Ids are uint64 and exceed what a double holds exactly, so the
    // tie-break is on BigInt rather than on Number.
    if (ra === 0) return a.actsBy - b.actsBy;
    if (a.publishedAt !== b.publishedAt) return b.publishedAt - a.publishedAt;
    return BigInt(b.id) > BigInt(a.id) ? 1 : -1;
  });
}

/** Live deadlines, soonest first — what a banner at the top of the folder says. */
export function openDeadlines(covenants: Covenant[], nowSeconds: number): Covenant[] {
  return covenants
    .filter((c) => !isSuperseded(c) && deadlineOf(c, nowSeconds).state === 'open')
    .sort((a, b) => a.actsBy - b.actsBy);
}

/** True when this filing covers a span rather than a moment. */
export function hasPeriod(c: Pick<Covenant, 'periodStart' | 'periodEnd'>): boolean {
  return c.periodStart > 0 && c.periodEnd > 0;
}
