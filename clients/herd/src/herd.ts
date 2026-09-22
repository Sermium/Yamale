/**
 * The registry's records, in the shapes these screens actually render.
 *
 * Between the wire and the screen, and pure, so the parts that can be wrong
 * without a network — what an operator's thumbs produced, what an enum means,
 * and which of the module's two incompatible time fields a number came from —
 * are tested rather than eyeballed.
 *
 * # The one that will bite somebody
 *
 * `x/herd` has two kinds of `int64` and they are not distinguished by name, by
 * type or by comment:
 *
 *     Animal.registered_at        BLOCK HEIGHT   keeper writes blockHeight(ctx)
 *     Herd.registered_at          BLOCK HEIGHT
 *     HealthEvent.recorded_at     BLOCK HEIGHT
 *     Transfer.proposed_at        UNIX SECONDS   keeper writes blockTime(ctx)
 *     Transfer.accepted_at        UNIX SECONDS
 *     Transfer.attested_at        UNIX SECONDS
 *     Attestation.attested_at     UNIX SECONDS
 *     Agent.accredited_at         UNIX SECONDS
 *
 * Read off devnet-2: animal 1 carries `registered_at: 320898` while transfer 1
 * carries `proposed_at: 1789310513`. Rendering the first as a date prints the
 * 4th of January 1970 on a certificate. So the two are different types here —
 * `Height` and `Seconds` — and a screen cannot pass one where the other is
 * expected without the compiler stopping it.
 */

import type {
  Agent as WireAgent,
  Animal as WireAnimal,
  Attestation as WireAttestation,
  Authority as WireAuthority,
  HealthEvent as WireHealthEvent,
  Herd as WireHerd,
  Transfer as WireTransfer,
} from '../../sdk/src/generated/blockchain/herd/v1/herd.ts';
import type { PedigreeNode as WirePedigreeNode } from '../../sdk/src/generated/blockchain/herd/v1/query.ts';

/* ------------------------------------------------------------ the two clocks */

/** A block height. Not a time, and never rendered as one. */
export type Height = number & { readonly __brand: 'height' };
/** Unix seconds, as the chain's own block time. */
export type Seconds = number & { readonly __brand: 'seconds' };

export function asHeight(v: string | number): Height {
  return (Number(v) || 0) as Height;
}
export function asSeconds(v: string | number): Seconds {
  return (Number(v) || 0) as Seconds;
}

/* ----------------------------------------------------------------- enums */

/**
 * Enums as words, not numbers.
 *
 * Every one of these keeps its `unspecified` member. The module refuses to
 * store a zero on purpose — "the keeper refuses it rather than defaulting,
 * because defaulting here would silently record a herd as a certified animal" —
 * so a zero arriving at a screen means a record this client does not
 * understand, and showing it as anything else would invent the very default the
 * chain declined to.
 */
export type Level = 'unspecified' | 'herd' | 'individual' | 'certified';
export type Species = 'unspecified' | 'cattle' | 'sheep' | 'goat' | 'horse' | 'camel';
export type Sex = 'unspecified' | 'male' | 'female';
export type Status =
  | 'unspecified' | 'active' | 'transferPending' | 'disputed' | 'frozen' | 'retired';
export type TransferState =
  | 'unspecified' | 'proposed' | 'accepted' | 'attested' | 'objected' | 'completed' | 'cancelled';

const LEVELS: Level[] = ['unspecified', 'herd', 'individual', 'certified'];
const SPECIES: Species[] = ['unspecified', 'cattle', 'sheep', 'goat', 'horse', 'camel'];
const SEXES: Sex[] = ['unspecified', 'male', 'female'];
const STATUSES: Status[] =
  ['unspecified', 'active', 'transferPending', 'disputed', 'frozen', 'retired'];
const TRANSFER_STATES: TransferState[] =
  ['unspecified', 'proposed', 'accepted', 'attested', 'objected', 'completed', 'cancelled'];

function at<T>(table: T[], n: number, fallback: T): T {
  return table[n] ?? fallback;
}

export const levelOf = (n: number): Level => at(LEVELS, n, 'unspecified');
export const speciesOf = (n: number): Species => at(SPECIES, n, 'unspecified');
export const sexOf = (n: number): Sex => at(SEXES, n, 'unspecified');
export const statusOf = (n: number): Status => at(STATUSES, n, 'unspecified');
export const transferStateOf = (n: number): TransferState =>
  at(TRANSFER_STATES, n, 'unspecified');

export type Tone = 'ok' | 'warn' | 'bad' | 'mute' | 'brass';

/**
 * The colour behind a status, which is always behind a word and never instead
 * of one.
 *
 * `transferPending` is amber rather than green because an animal in the middle
 * of a handover cannot start another one — that is a stop, not a note — and an
 * operator in a pen deciding whether to begin a sale needs to see it before
 * they read anything else.
 */
export function statusTone(s: Status): Tone {
  switch (s) {
    case 'active': return 'ok';
    case 'transferPending': return 'warn';
    case 'disputed': return 'bad';
    case 'frozen': return 'bad';
    case 'retired': return 'mute';
    default: return 'mute';
  }
}

export function transferTone(s: TransferState): Tone {
  switch (s) {
    case 'completed': return 'ok';
    case 'attested': return 'brass';
    case 'accepted': case 'proposed': return 'warn';
    case 'objected': return 'bad';
    case 'cancelled': return 'mute';
    default: return 'mute';
  }
}

/** True when a record is in a state that permits a new handover to start. */
export function isMovable(s: Status): boolean {
  return s === 'active';
}

/* --------------------------------------------------------------- records */

export interface Animal {
  id: string;
  herdId: string;
  nationalId: string;
  /** ISO 11784/11785 transponder. Empty when none is fitted. */
  tag: string;
  level: Level;
  name: string;
  sex: Sex;
  coat: string;
  /** A day, `YYYY-MM-DD`, or empty. Never a timestamp: a pastoral birth is
   *  known to the day at best and second precision would be invented. */
  bornOn: string;
  birthType: string;
  /** '0' means unknown, which is honest and common. It is not animal zero. */
  sireId: string;
  damId: string;
  dnaHash: string;
  photoHash: string;
  holder: string;
  authority: string;
  status: Status;
  registeredAt: Height;
}

export interface Herd {
  id: string;
  reference: string;
  keeper: string;
  authority: string;
  species: Species;
  /** DECLARED, not counted. The field name on the chain says so on purpose. */
  declaredSize: number;
  locality: string;
  locationHash: string;
  jurisdiction: string;
  status: Status;
  registeredAt: Height;
}

export interface HealthEvent {
  id: string;
  herdId: string;
  disease: string;
  batch: string;
  treated: number;
  agent: string;
  recordedAt: Height;
}

export interface Attestation {
  attestor: string;
  authority: string;
  evidenceHash: string;
  attestedAt: Seconds;
}

export interface Transfer {
  id: string;
  animalId: string;
  from: string;
  to: string;
  /** Free text. The chain never witnessed the money and does not pretend to. */
  price: string;
  state: TransferState;
  attestations: Attestation[];
  attestedAt: Seconds;
  objectedBy: string;
  objectionReason: string;
  proposedAt: Seconds;
  resolvedBy: string;
  resolutionReason: string;
  objectionUpheld: boolean;
  acceptedAt: Seconds;
}

export interface Agent {
  address: string;
  name: string;
  authority: string;
  jurisdiction: string;
  active: boolean;
  accreditedAt: Seconds;
}

export interface Authority {
  address: string;
  name: string;
  jurisdiction: string;
  active: boolean;
}

/** One ancestor, and where it sits in the line. */
export interface Ancestor {
  animal: Animal;
  generation: number;
  /** '', 'sire', 'dam', 'dam.dam' — the side each ancestor came from. */
  path: string;
}

export interface Line {
  nodes: Ancestor[];
  /**
   * True when the walk stopped at the depth bound rather than at a founder.
   *
   * Carried all the way to the screen. Without it a truncated line and a
   * founder render identically, and a buyer reads "no further ancestors" where
   * the truth is "we stopped looking".
   */
  truncated: boolean;
}

/* ------------------------------------------------------ out of the wire */

export function toAnimal(w: WireAnimal): Animal {
  return {
    id: String(w.id),
    herdId: String(w.herdId),
    nationalId: w.nationalId,
    tag: w.tag,
    level: levelOf(w.level),
    name: w.name,
    sex: sexOf(w.sex),
    coat: w.coat,
    bornOn: w.bornOn,
    birthType: w.birthType,
    sireId: String(w.sireId),
    damId: String(w.damId),
    dnaHash: w.dnaHash,
    photoHash: w.photoHash,
    holder: w.holder,
    authority: w.authority,
    status: statusOf(w.status),
    registeredAt: asHeight(w.registeredAt),
  };
}

export function toHerd(w: WireHerd): Herd {
  return {
    id: String(w.id),
    reference: w.reference,
    keeper: w.keeper,
    authority: w.authority,
    species: speciesOf(w.species),
    declaredSize: Number(w.declaredSize) || 0,
    locality: w.locality,
    locationHash: w.locationHash,
    jurisdiction: w.jurisdiction,
    status: statusOf(w.status),
    registeredAt: asHeight(w.registeredAt),
  };
}

export function toHealthEvent(w: WireHealthEvent): HealthEvent {
  return {
    id: String(w.id),
    herdId: String(w.herdId),
    disease: w.disease,
    batch: w.batch,
    treated: Number(w.treated) || 0,
    agent: w.agent,
    recordedAt: asHeight(w.recordedAt),
  };
}

export function toAttestation(w: WireAttestation): Attestation {
  return {
    attestor: w.attestor,
    authority: w.authority,
    evidenceHash: w.evidenceHash,
    attestedAt: asSeconds(w.attestedAt),
  };
}

export function toTransfer(w: WireTransfer): Transfer {
  return {
    id: String(w.id),
    animalId: String(w.animalId),
    from: w.from,
    to: w.to,
    price: w.price,
    state: transferStateOf(w.state),
    attestations: (w.attestations ?? []).map(toAttestation),
    attestedAt: asSeconds(w.attestedAt),
    objectedBy: w.objectedBy,
    objectionReason: w.objectionReason,
    proposedAt: asSeconds(w.proposedAt),
    resolvedBy: w.resolvedBy,
    resolutionReason: w.resolutionReason,
    objectionUpheld: Boolean(w.objectionUpheld),
    acceptedAt: asSeconds(w.acceptedAt),
  };
}

export function toAgent(w: WireAgent): Agent {
  return {
    address: w.address,
    name: w.name,
    authority: w.authority,
    jurisdiction: w.jurisdiction,
    active: Boolean(w.active),
    accreditedAt: asSeconds(w.accreditedAt),
  };
}

export function toAuthority(w: WireAuthority): Authority {
  return {
    address: w.address,
    name: w.name,
    jurisdiction: w.jurisdiction,
    active: Boolean(w.active),
  };
}

export function toAncestor(w: WirePedigreeNode): Ancestor {
  return {
    animal: toAnimal(w.animal!),
    generation: Number(w.generation) || 0,
    path: w.path,
  };
}

/* -------------------------------------------------------- what was typed */

/**
 * A national identifier as the sector issues it: `221/682/021029/L`.
 *
 * Country dialling code, flock, serial, breed letter. Typed on a phone, in
 * sunlight, with one thumb, by somebody wearing a glove — so a space, a dash, a
 * full stop and a backslash all become the separator, and the breed letter is
 * upper-cased. Nothing else is repaired: a digit the operator got wrong is
 * theirs to correct, and a client that "helpfully" padded a serial would
 * resolve a certificate to the wrong animal.
 */
export function normaliseNationalId(input: string): string {
  return input
    .trim()
    .replace(/[\s.\-\\_]+/g, '/')
    .replace(/\/+/g, '/')
    .replace(/^\/|\/$/g, '')
    .toUpperCase();
}

/** Loose, on purpose: the registry owns the format, this only routes a query. */
export function looksLikeNationalId(value: string): boolean {
  return /^[0-9]{1,4}\/[0-9A-Z]{1,8}\/[0-9A-Z]{1,10}(\/[A-Z]{1,3})?$/.test(value);
}

/**
 * A transponder code.
 *
 * ISO 11784 is 15 decimal digits. Readers and labels print them spaced or
 * dotted in groups, so everything that is not a digit goes; the length is then
 * checked rather than assumed, because a 14-digit string is a misread and
 * silently querying with it would answer "no such animal" about a perfectly
 * good beast.
 */
export function normaliseTag(input: string): string {
  return input.replace(/\D+/g, '');
}

export function looksLikeTag(value: string): boolean {
  return /^[0-9]{15}$/.test(value);
}

/** What an operator typed, and therefore which query to run. */
export type EntryKind = 'empty' | 'tag' | 'national-id' | 'unrecognised';

export interface Entry {
  kind: EntryKind;
  /** The normalised value, ready for the chain. */
  value: string;
  /** Exactly as typed, for echoing back in an error. */
  raw: string;
}

/**
 * Decide what was typed.
 *
 * Deliberately refuses to guess between the two when it cannot tell. A bare
 * `982000000000102` is fifteen digits and is a tag; `221/682/000102/L` has the
 * shape of a certificate. Anything else is `unrecognised`, and the screen says
 * what the two forms look like rather than firing both queries and presenting
 * whichever happened to answer — which is how an operator learns to trust a
 * lookup that was a coin toss.
 */
export function classify(raw: string): Entry {
  const trimmed = raw.trim();
  if (trimmed === '') return { kind: 'empty', value: '', raw };

  const tag = normaliseTag(trimmed);
  if (looksLikeTag(tag) && !/[^0-9\s.\-]/.test(trimmed)) {
    return { kind: 'tag', value: tag, raw };
  }

  const nid = normaliseNationalId(trimmed);
  if (looksLikeNationalId(nid)) return { kind: 'national-id', value: nid, raw };

  return { kind: 'unrecognised', value: trimmed, raw };
}

/* ------------------------------------------------------------ small things */

/**
 * Age from a birth day, in whole months.
 *
 * Null when the day is absent or unparseable, and null is rendered as "not
 * recorded" — never as zero, which would say the animal was born today. §4
 * turns on this number: a muzzle or face template stops matching the animal it
 * belongs to as the beast grows, so the age is what decides whether an
 * enrolment is due to be taken again.
 */
export function ageInMonths(bornOn: string, now: Date): number | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(bornOn)) return null;
  const [y, m, d] = bornOn.split('-').map(Number);
  if (!y || !m || !d) return null;
  const born = Date.UTC(y, m - 1, d);
  if (Number.isNaN(born)) return null;
  const months = (now.getUTCFullYear() - y) * 12 + (now.getUTCMonth() - (m - 1));
  return now.getUTCDate() < d ? months - 1 : months;
}

/**
 * Which animals in a line are missing, and therefore whether a pedigree can be
 * walked at all.
 *
 * A line of one node is a founder OR an animal whose parents were never
 * recorded, and the two are the same on the wire. `Animal.sire_id` and `dam_id`
 * distinguish them: zero is "not recorded", a non-zero id that the line does
 * not contain is an ancestor the query did not reach.
 */
export function lineGaps(line: Line): { unrecorded: number; unreached: string[] } {
  const present = new Set(line.nodes.map((n) => n.animal.id));
  let unrecorded = 0;
  const unreached: string[] = [];
  for (const node of line.nodes) {
    for (const parent of [node.animal.sireId, node.animal.damId]) {
      if (parent === '0' || parent === '') { unrecorded++; continue; }
      if (!present.has(parent)) unreached.push(parent);
    }
  }
  return { unrecorded, unreached };
}
