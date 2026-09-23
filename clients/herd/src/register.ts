/**
 * Registration: putting an animal into the register for the first time.
 *
 * Every other screen in this application ANSWERS a question — which animal is
 * this? This one makes an assertion, and it is the only screen that can create
 * something the register did not have before. That difference is the whole
 * design of this file.
 *
 * # What registration must not do
 *
 * Create a second record for an animal that already has one.
 *
 * The register makes exactly one promise — `ByNationalID makes an identifier
 * mean one animal` — and the way that promise dies is not fraud. It is a
 * registration agent in a pen with two hundred head, a phone, and no way to
 * tell whether the beast in front of them was registered last season by
 * somebody else. They register it again, in good faith. There are now two
 * records, two histories, and two futures for one animal, and nothing anywhere
 * looks wrong.
 *
 * So `check` runs before anything is created, against every identifier AND
 * every biometric this device holds, and `Blocked` is a first-class outcome
 * rather than a warning. The chain enforces uniqueness of identifiers too — see
 * x/herd/keeper/identifiers.go — but it cannot see a muzzle, and it cannot see
 * anything at all while the device is offline in a pen, which is most of when
 * this screen is used.
 *
 * # The scan number
 *
 * A registration is bound to a REFERENCE the operator scans or types: the
 * number pre-printed on the paper form, the ear tag applicator's docket, the
 * campaign's own sheet. It is not the national identifier and it is not the
 * transponder code.
 *
 * It exists because a registration is a claim about a physical event — this
 * animal, in this crush, on this morning — and the reference is what ties the
 * record back to the paper that a person can be asked about later. Without it,
 * a disputed registration is one agent's word and nothing else; with it, there
 * is a form in a box with the same number on it.
 *
 * # The hereditary link
 *
 * Registering is level 2 and takes no parents. Pedigree is level 3, a separate
 * assertion carrying record IDS rather than names, and the reason for that
 * split is in the proto: "A line of names can be neither walked nor
 * contradicted; a line of references can be both."
 *
 * `withParents` therefore produces a SECOND entry rather than folding the
 * parents into the first. An agent who knows the dam and not the sire can
 * register today and certify when they find out, and a parent that turns out to
 * be wrong can be contradicted without unmaking the animal.
 */

import {
  canonicaliseNationalId,
  formatNationalId,
  formatTagCode,
  parseNationalId,
  parseTagCode,
} from './identity.ts';
import { ambiguous, muzzleVerdict, search, type Descriptor, type Enrolled } from './muzzle.ts';

/* ---------------------------------------------------------- what is known */

/** An identifier the operator has for the animal in front of them. */
export interface Claimed {
  /** The registration reference: the paper this is being recorded against. */
  reference: string;
  /** The national identifier, if the animal already has one issued. */
  nationalId?: string;
  /** The ISO 11784/11785 transponder, if one has been implanted. */
  tag?: string;
}

/** What the register already holds, as far as this device knows. */
export interface Known {
  /** Animals this device has, by canonical national identifier. */
  byNationalId: ReadonlyMap<string, string>;
  /** Animals this device has, by canonical transponder code. */
  byTag: ReadonlyMap<string, string>;
  /** References already used, so one form cannot register two animals. */
  usedReferences: ReadonlySet<string>;
  /** Biometric enrolments to compare a fresh capture against. */
  enrolments: readonly Enrolled[];
}

export const EMPTY_KNOWN: Known = {
  byNationalId: new Map(),
  byTag: new Map(),
  usedReferences: new Set(),
  enrolments: [],
};

/* ------------------------------------------------------------- the answer */

/**
 * Whether this animal may be registered.
 *
 * Three outcomes and not two. `review` is the important one: a biometric match
 * is EVIDENCE that this animal is already registered, and evidence is not proof
 * — the thresholds in muzzle.ts have never seen a real animal, and a matcher
 * that stopped a legitimate registration outright would be worse than one that
 * asked. So a biometric hit sends the operator to look, and an identifier
 * collision stops them.
 *
 * The asymmetry is deliberate: an identifier is issued and unique by
 * construction, and a muzzle is a measurement.
 */
export type Check =
  | { verdict: 'ready' }
  | {
      verdict: 'review';
      /** What matched, so the screen can show the animal rather than a score. */
      animalIds: string[];
      reason: string;
    }
  | { verdict: 'blocked'; reason: string; animalId?: string };

export interface CheckInput {
  claimed: Claimed;
  known: Known;
  /** A muzzle capture, when one was taken. Registration does not require it. */
  muzzle?: Descriptor;
}

/**
 * Everything that must be true before a registration is created.
 *
 * Order matters and it is cheapest-first only by accident; what it is really
 * ordered by is how certain each refusal is. A malformed identifier is certain.
 * A collision is certain. A biometric match is not, so it comes last and
 * produces a question rather than a refusal.
 */
export function check({ claimed, known, muzzle }: CheckInput): Check {
  const reference = claimed.reference.trim();
  if (reference === '') {
    return {
      verdict: 'blocked',
      reason:
        'a registration needs the reference from the form it is being recorded against, ' +
        'because that is what ties this record to a piece of paper somebody can be asked about',
    };
  }
  if (known.usedReferences.has(reference)) {
    return {
      verdict: 'blocked',
      reason: `reference ${reference} has already been used for a registration`,
    };
  }

  if (claimed.nationalId !== undefined && claimed.nationalId.trim() !== '') {
    const canonical = canonicalNationalId(claimed.nationalId);
    if (!canonical.ok) {
      return { verdict: 'blocked', reason: canonical.detail };
    }
    const existing = known.byNationalId.get(canonical.value);
    if (existing !== undefined) {
      return {
        verdict: 'blocked',
        reason: `${canonical.value} is already registered`,
        animalId: existing,
      };
    }
  }

  if (claimed.tag !== undefined && claimed.tag.trim() !== '') {
    const parsed = parseTagCode(claimed.tag);
    if (!parsed.ok) {
      return { verdict: 'blocked', reason: parsed.detail };
    }
    const canonical = formatTagCode(parsed.value);
    const existing = known.byTag.get(canonical);
    if (existing !== undefined) {
      return {
        verdict: 'blocked',
        reason: `transponder ${canonical} is already registered`,
        animalId: existing,
      };
    }
  }

  if (muzzle) {
    const hits = search(muzzle, known.enrolments).filter((c) => c.verdict === 'match');
    if (hits.length > 0) {
      const animalIds = [...new Set(hits.map((h) => h.animalId))];
      return {
        verdict: 'review',
        animalIds,
        reason:
          animalIds.length > 1
            ? 'this muzzle matches more than one animal already registered, which has to be ' +
              'resolved by a person before anything is created'
            : 'this muzzle matches an animal already registered; check it is not the same beast',
      };
    }
  }

  return { verdict: 'ready' };
}

/* ------------------------------------------------------- what is produced */

export type Sex = 'female' | 'male' | 'unknown';

export interface Particulars {
  name?: string;
  sex?: Sex;
  coat?: string;
  /** ISO 8601 date, as the chain stores it. */
  bornOn?: string;
  birthType?: string;
}

/**
 * A registration, ready for the outbox.
 *
 * Shaped as the fields of `MsgIdentifyAnimal` and nothing else. The biometric
 * captures are recorded as their own outbox entries — `capture` — because the
 * chain has nowhere to put a muzzle template today and pretending otherwise
 * would queue a message that can never be sent.
 */
export interface Registration {
  reference: string;
  herdId: string;
  nationalId: string;
  tag: string;
  particulars: Particulars;
}

export class RegisterError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'RegisterError';
  }
}

/**
 * Build the registration, refusing if the check did not pass.
 *
 * `check` is taken as an argument rather than re-run, so the screen cannot show
 * one answer and submit against another. A caller that wants to proceed past a
 * `review` has to say so explicitly, which is a decision somebody made rather
 * than a branch that was never taken.
 */
export function register(
  input: CheckInput & { herdId: string; particulars?: Particulars },
  checked: Check,
  overrideReview = false,
): Registration {
  if (checked.verdict === 'blocked') {
    throw new RegisterError(checked.reason);
  }
  if (checked.verdict === 'review' && !overrideReview) {
    throw new RegisterError(
      `${checked.reason} — if it is a different animal, say so deliberately before registering`,
    );
  }
  if (input.herdId.trim() === '') {
    throw new RegisterError('a registration belongs to a herd');
  }

  let nationalId = '';
  if (input.claimed.nationalId && input.claimed.nationalId.trim() !== '') {
    const canonical = canonicalNationalId(input.claimed.nationalId);
    if (!canonical.ok) throw new RegisterError(canonical.detail);
    nationalId = canonical.value;
  }

  let tag = '';
  if (input.claimed.tag && input.claimed.tag.trim() !== '') {
    const parsed = parseTagCode(input.claimed.tag);
    if (!parsed.ok) throw new RegisterError(parsed.detail);
    tag = formatTagCode(parsed.value);
  }

  if (nationalId === '' && tag === '') {
    // A record with neither identifier can never be found again by anybody
    // holding a certificate or a reader, which makes it a row rather than a
    // registration.
    throw new RegisterError(
      'a registration needs a national identifier or a transponder code; without one the ' +
        'animal cannot be looked up again by anybody',
    );
  }

  return {
    reference: input.claimed.reference.trim(),
    herdId: input.herdId.trim(),
    nationalId,
    tag,
    particulars: input.particulars ?? {},
  };
}

/**
 * The one spelling a national identifier is stored and compared under.
 *
 * Parsed, then canonicalised, then formatted — in that order and never
 * shortcut. `221/682/1029/L` and `221/682/001029/L` are ONE animal, and
 * comparing the strings as typed is exactly the bug that
 * x/herd/keeper/identifiers.go exists to fix. Doing it here as well is not
 * belt and braces: the chain cannot be asked while this device is in a pen
 * with no signal, which is most of when this screen is used.
 */
function canonicalNationalId(
  raw: string,
): { ok: true; value: string } | { ok: false; detail: string } {
  const parsed = parseNationalId(raw);
  if (!parsed.ok) return { ok: false, detail: parsed.detail };
  const canonical = canonicaliseNationalId(parsed.value);
  if (!canonical.ok) return { ok: false, detail: canonical.detail };
  return { ok: true, value: formatNationalId(canonical.value) };
}

/* --------------------------------------------------------- the pedigree */

/** A parent, by the register's own record id. */
export interface Pedigree {
  /** The record id of the dam, or empty when it is not known. */
  dam: string;
  /** The record id of the sire, or empty when it is not known. */
  sire: string;
}

export interface Certification {
  animalId: string;
  pedigree: Pedigree;
}

/**
 * Link an animal to its parents.
 *
 * A SEPARATE assertion, and the separation is the point. Registration says this
 * animal exists; certification says where it came from, and those are believed
 * on different evidence by different people at different times.
 *
 * Parents are record ids, never names: "A line of names can be neither walked
 * nor contradicted; a line of references can be both."
 */
export function withParents(animalId: string, pedigree: Pedigree): Certification {
  const dam = pedigree.dam.trim();
  const sire = pedigree.sire.trim();

  if (dam === '' && sire === '') {
    throw new RegisterError('a certification with neither parent asserts nothing');
  }
  if (dam !== '' && dam === animalId) {
    throw new RegisterError('an animal cannot be its own dam');
  }
  if (sire !== '' && sire === animalId) {
    throw new RegisterError('an animal cannot be its own sire');
  }
  if (dam !== '' && dam === sire) {
    // Not a loop check — one device cannot walk the whole line offline — but
    // the one case it CAN see, and the one that is certainly a mistyped field
    // rather than an unusual pedigree.
    throw new RegisterError('the same record has been given as both dam and sire');
  }

  return { animalId, pedigree: { dam, sire } };
}

/**
 * Whether a capture is worth enrolling alongside a registration.
 *
 * Registration is the one moment the animal is certainly in front of the
 * operator and certainly being handled, so it is the best chance the register
 * will ever get at a good biometric. A poor capture taken now is still worth
 * keeping — it can be compared against and superseded — but the screen should
 * say what it got, which is why this returns a reason rather than a boolean.
 */
export function captureAdvice(quality: number | null): { keep: boolean; reason: string } {
  if (quality === null) {
    return { keep: true, reason: 'this instrument does not score its own captures' };
  }
  if (quality >= 0.7) return { keep: true, reason: 'good' };
  if (quality >= 0.4) {
    return {
      keep: true,
      reason: 'usable, and worth taking again while the animal is still held',
    };
  }
  return {
    keep: true,
    reason:
      'poor — keep it, but take another: this is the best access to this animal the ' +
      'register is likely to get',
  };
}

/**
 * Both outcomes of a muzzle search at registration, for a screen.
 *
 * Exposed so the review screen does not re-derive `ambiguous` and reach a
 * different conclusion from `check`.
 */
export function reviewIsAmbiguous(muzzle: Descriptor, known: Known): boolean {
  return ambiguous(search(muzzle, known.enrolments));
}

/** Re-exported so a screen can render a distance with the same words. */
export { muzzleVerdict };
