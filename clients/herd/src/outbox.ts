/**
 * What this device recorded while it had no signal, and what happens to it.
 *
 * §5: "Everything captured offline is held with its timestamp and position and
 * submitted when a connection returns." And, one line later, the rule that
 * decides the shape of this file:
 *
 *     "Two tablets that both recorded a treatment offline are reconciled by
 *     submitting both; the registry holds both records with their agents, and a
 *     contradiction is visible rather than silently resolved by whoever synced
 *     last."
 *
 * So there is no de-duplication here, no merge of two observations into one,
 * and no comparison of timestamps to decide a winner. Two entries about the
 * same animal from two agents are two entries, both submitted, and the chain
 * holds both. `disagreements()` below exists to make that visible on the
 * screen — not to resolve it.
 *
 * # Entry ids are device-local and that matters
 *
 * An entry's id is minted on the device, never by the chain, and is never
 * derived from the entry's content. A content hash would make two devices that
 * observed the same animal at the same second produce the same id, and a merge
 * keyed on it would silently drop one of the two observations — which is
 * precisely the last-write-wins this file exists to avoid. Minting from a
 * device id and a counter costs nothing and cannot collide across devices.
 *
 * # What can actually be submitted today
 *
 * `x/herd` has no `Enrolment` and no `IdentificationClaim` message. §8 proposes
 * both and neither exists on devnet-2 — the module's write side is registry,
 * identification, certification, health and transfer, and that is all. So a
 * queued capture is `held`, not `ready`, and the Sync screen says which. A
 * queue that showed a capture as "waiting to send" when there is nothing on the
 * chain to send it to would be a progress bar with no end, and the operator
 * would eventually stop believing any of it.
 *
 * What CAN be submitted is an attestation — `MsgAttestTransfer` carries an
 * `evidence_hash`, which is exactly "what the attestor actually checked". That
 * is the one path from a field capture to the chain that exists today, and
 * `SUBMITTABLE` below is the whole of it.
 */

/** What an entry records. */
export type EntryKind =
  /** A biometric capture, with its quality and the device that took it. §2. */
  | 'capture'
  /** "This capture was matched to this animal, by me, at this score." §2. */
  | 'identification-claim'
  /** An accredited agent swearing an animal is the animal. MsgAttestTransfer. */
  | 'attestation'
  /** A treatment administered. MsgRecordHealthEvent. */
  | 'health-event';

/**
 * Which kinds the chain has somewhere to put, today.
 *
 * Read off `proto/blockchain/herd/v1/tx.proto` at the commit this was written
 * against. When §8's messages land, this is the one line that changes.
 */
export const SUBMITTABLE: Record<EntryKind, boolean> = {
  capture: false,
  'identification-claim': false,
  attestation: true,
  'health-event': true,
};

/** Where an entry is in its life. */
export type EntryState =
  /** Recorded, and the chain has no message that carries it. Not a failure. */
  | 'held'
  /** Recorded, and waiting only for a connection. */
  | 'queued'
  /** A submission is in flight. */
  | 'sending'
  /** The chain accepted it. `txHash` says which transaction. */
  | 'sent'
  /** The chain refused it, or the signer did. `note` says what happened. */
  | 'refused';

export interface Position {
  lat: number;
  lon: number;
  accuracyM: number;
}

export interface Entry {
  /** Minted on the device. Never a chain id, never a content hash. */
  id: string;
  kind: EntryKind;
  /** The animal this is about, when it is about one. */
  animalId: string | null;
  /** The herd, which is what scopes the replica this device holds. */
  herdId: string;
  /**
   * The DEVICE's clock at the moment of observation, in milliseconds.
   *
   * Not the sync's clock, and not the chain's. This is the only record of when
   * the thing actually happened, and an entry that took its time from the
   * submission would date a Tuesday capture to Friday.
   */
  observedAt: number;
  /** Which instrument, and who was holding it. Both go on the record. §2. */
  deviceId: string;
  agent: string;
  /** Where, when there was a fix. Null is "no fix", never (0, 0). */
  position: Position | null;
  /** Kind-specific detail. Opaque here; the screens and `chain.ts` read it. */
  detail: Record<string, unknown>;
  state: EntryState;
  attempts: number;
  txHash: string | null;
  note: string | null;
}

/** A counter, so two entries minted in the same millisecond still differ. */
let minted = 0;

/**
 * Mint an entry.
 *
 * `state` is derived, never passed: a capture is `held` because the chain has
 * nowhere to put it, and a caller that could override that could queue a
 * capture forever.
 */
export function newEntry(input: {
  kind: EntryKind;
  animalId: string | null;
  herdId: string;
  observedAt: number;
  deviceId: string;
  agent: string;
  position?: Position | null;
  detail?: Record<string, unknown>;
}): Entry {
  minted += 1;
  return {
    id: `${input.deviceId}:${input.observedAt}:${minted}`,
    kind: input.kind,
    animalId: input.animalId,
    herdId: input.herdId,
    observedAt: input.observedAt,
    deviceId: input.deviceId,
    agent: input.agent,
    position: input.position ?? null,
    detail: input.detail ?? {},
    state: SUBMITTABLE[input.kind] ? 'queued' : 'held',
    attempts: 0,
    txHash: null,
    note: null,
  };
}

/* ------------------------------------------------------------- the merge */

/**
 * Fold one list of entries into another.
 *
 * Union by entry id, and nothing else. Two entries with different ids always
 * both survive, whatever they say, whoever made them and whenever they were
 * made — that is the §5 rule, expressed as code that has no branch capable of
 * dropping one.
 *
 * Where the SAME id appears twice, the later state wins, and "later" is by the
 * state machine rather than by a clock: `sent` beats `sending` beats `queued`.
 * That is not a reconciliation of two observations — it is the same entry, on
 * the same device, seen at two points in its own life, and a clock comparison
 * here would let a stale read resurrect an entry that has already been sent.
 */
export function merge(into: readonly Entry[], incoming: readonly Entry[]): Entry[] {
  const RANK: Record<EntryState, number> = {
    held: 0, queued: 1, sending: 2, refused: 3, sent: 4,
  };
  const byId = new Map<string, Entry>();
  for (const e of into) byId.set(e.id, e);
  for (const e of incoming) {
    const had = byId.get(e.id);
    if (!had) { byId.set(e.id, e); continue; }
    byId.set(e.id, RANK[e.state] >= RANK[had.state] ? e : had);
  }
  return [...byId.values()].sort((a, b) => a.observedAt - b.observedAt);
}

/* --------------------------------------------------- the visible conflict */

/**
 * Two observations of one animal that cannot both be the whole truth.
 *
 * NOT resolved. This produces the pairs a screen shows side by side, with both
 * agents named, so a person can see that two devices disagree. Everything in
 * every pair is still submitted.
 */
export interface Disagreement {
  animalId: string;
  kind: EntryKind;
  entries: Entry[];
  /** True when the entries came from different agents, which is the case §5
   *  is about. Same agent twice is usually a re-capture, not a contradiction. */
  differentAgents: boolean;
}

/**
 * Find them.
 *
 * Grouped by animal and kind, and reported whenever more than one entry exists
 * in a group — including when one agent recorded twice, because a second
 * capture minutes after the first is worth a glance even though it is usually
 * innocent. `differentAgents` is what the screen leads with.
 *
 * The window is deliberately not applied here. A contradiction between a
 * Tuesday capture and a Friday one is still a contradiction if both are
 * unsubmitted, and a time filter would hide exactly the pair that has been
 * sitting in a bag for three days.
 */
export function disagreements(entries: readonly Entry[]): Disagreement[] {
  // collections.Pair is not a map key in Go and a tuple is not one in
  // JavaScript either — a string key, built explicitly, with a separator that
  // cannot appear in either half.
  const groups = new Map<string, Entry[]>();
  for (const e of entries) {
    if (e.animalId === null) continue;
    if (e.state === 'sent') continue; // already on the chain; the chain shows it
    const key = `${e.animalId} ${e.kind}`;
    const got = groups.get(key);
    if (got) got.push(e); else groups.set(key, [e]);
  }

  const out: Disagreement[] = [];
  for (const [key, group] of groups) {
    if (group.length < 2) continue;
    const [animalId, kind] = key.split(' ');
    out.push({
      animalId,
      kind: kind as EntryKind,
      entries: [...group].sort((a, b) => a.observedAt - b.observedAt),
      differentAgents: new Set(group.map((e) => e.agent)).size > 1,
    });
  }
  return out.sort((a, b) => a.animalId.localeCompare(b.animalId));
}

/* ---------------------------------------------------------- what to show */

export interface Tally {
  held: number;
  queued: number;
  sending: number;
  sent: number;
  refused: number;
  /** The oldest thing on this device that has not reached the chain. */
  oldestUnsent: number | null;
}

export function tally(entries: readonly Entry[]): Tally {
  const out: Tally = {
    held: 0, queued: 0, sending: 0, sent: 0, refused: 0, oldestUnsent: null,
  };
  for (const e of entries) {
    out[e.state] += 1;
    if (e.state !== 'sent') {
      out.oldestUnsent = out.oldestUnsent === null
        ? e.observedAt
        : Math.min(out.oldestUnsent, e.observedAt);
    }
  }
  return out;
}

/** The entries a sync run would attempt, oldest first. */
export function sendable(entries: readonly Entry[]): Entry[] {
  return entries
    .filter((e) => e.state === 'queued' || e.state === 'refused')
    .filter((e) => SUBMITTABLE[e.kind])
    .sort((a, b) => a.observedAt - b.observedAt);
}

/**
 * Move an entry along, without ever losing it.
 *
 * A refusal keeps the entry and records why. Nothing in this module deletes an
 * entry: a capture whose submission the chain refused is still the only record
 * that the operator stood in front of that animal, and dropping it to keep a
 * queue tidy would destroy the evidence to improve the housekeeping.
 */
export function advance(
  entry: Entry,
  outcome: { ok: true; txHash: string } | { ok: false; note: string },
): Entry {
  if (outcome.ok) {
    return { ...entry, state: 'sent', txHash: outcome.txHash, note: null, attempts: entry.attempts + 1 };
  }
  return { ...entry, state: 'refused', note: outcome.note, attempts: entry.attempts + 1 };
}
