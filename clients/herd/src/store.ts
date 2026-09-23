/**
 * Where the herd lives when there is no signal: IndexedDB, on the device.
 *
 * Not `localStorage`. A herd of several hundred animals with iris templates
 * attached is megabytes of binary, `localStorage` is a synchronous string store
 * with a quota measured in single-digit megabytes, and serialising a
 * `Uint8Array` into it through JSON quadruples it. IndexedDB stores the typed
 * array as a typed array, asynchronously, and is the only store on the platform
 * that does.
 *
 * # Two implementations, and why the second is not just for tests
 *
 * `memoryStore()` exists because IndexedDB is absent or refused more often than
 * anybody expects: private browsing in some engines, a locked-down managed
 * device, an in-app browser, and Node — where these tests run. When it is
 * unavailable this application still has to work for the length of a session,
 * and it must SAY that what it holds will not survive being closed. `open()`
 * reports which store it got, and the Sync screen prints it. Falling back
 * silently would lose a day's captures the first time a tablet was restarted.
 *
 * # Nothing here decides anything
 *
 * This is storage. It does not merge, does not de-duplicate and does not
 * resolve. The rules about two devices disagreeing live in `outbox.ts` and are
 * pure; this puts records in and takes them out.
 */

import type { Animal, HealthEvent, Herd } from './herd.ts';
import type { Enrolled, IrisTemplate } from './iris.ts';
import type {
  Descriptor as MuzzleDescriptor,
  Enrolled as MuzzleEnrolled,
} from './muzzle.ts';
import type { Entry } from './outbox.ts';
import type { Replica, ReplicaMeta } from './replica.ts';

const DB_NAME = 'yamale.herd';
const DB_VERSION = 1;

const S_META = 'meta';
const S_ANIMALS = 'animals';
const S_HEALTH = 'health';
const S_ENROLMENTS = 'enrolments';
const S_OUTBOX = 'outbox';

/**
 * An enrolment as it is stored.
 *
 * `iris.ts`'s `Enrolled` with the herd attached, so a device that has held two
 * herds over its life cannot compare a probe against the wrong one. The
 * template's arrays are stored as `Uint8Array` and come back as `Uint8Array`;
 * the structured clone algorithm preserves them, which is the whole reason this
 * is not JSON.
 */
export interface StoredEnrolment {
  enrolmentId: string;
  animalId: string;
  herdId: string;
  capturedAt: number;
  /**
   * Which biometric this is: 'iris', 'muzzle' or 'face'.
   *
   * It decides which of the two payloads below is present, and which matcher
   * can read it. An enrolment whose modality disagrees with its payload is not
   * a degraded record, it is a record that will be compared with the wrong
   * arithmetic — so `toEnrolled` and `toMuzzleEnrolled` each return null rather
   * than guess.
   */
  modality: string;
  /** As the capture device scored it, or null when it does not score. */
  quality: number | null;
  deviceId: string;
  agent: string;
  /** Present for 'iris'. */
  template?: IrisTemplate;
  /**
   * Present for 'muzzle' and 'face'.
   *
   * Stored as the DESCRIPTOR rather than the photograph, because comparison
   * happens on the device against this replica and a descriptor is what can be
   * compared. The photograph goes to the outbox as a capture, where it is
   * evidence rather than an index.
   */
  descriptor?: MuzzleDescriptor;
}

export interface HerdStore {
  /** Which implementation this is, so a screen can say what survives a close. */
  readonly durable: boolean;
  readonly why: string;

  meta(): Promise<ReplicaMeta | null>;
  putReplica(replica: Replica): Promise<void>;
  /** Everything, with no filter: a herd is hundreds, not millions. */
  animals(): Promise<Animal[]>;
  herd(): Promise<Herd | null>;
  healthEvents(): Promise<HealthEvent[]>;

  enrolments(herdId: string): Promise<StoredEnrolment[]>;
  putEnrolment(e: StoredEnrolment): Promise<void>;

  outbox(): Promise<Entry[]>;
  putEntries(entries: readonly Entry[]): Promise<void>;

  /** Remove the replica and its enrolments. Never touches the outbox. */
  forgetReplica(): Promise<void>;
}

/**
 * An enrolment in the shape `iris.ts` searches over, or null.
 *
 * Null when this enrolment is not an iris. It used to return unconditionally,
 * reading `template` off whatever it was given — which was safe while iris was
 * the only modality and becomes a way to hand `undefined` to a matcher the
 * moment a second one exists.
 */
export function toEnrolled(e: StoredEnrolment): Enrolled | null {
  if (e.modality !== 'iris' || !e.template) return null;
  return {
    animalId: e.animalId,
    enrolmentId: e.enrolmentId,
    capturedAt: e.capturedAt,
    template: e.template,
  };
}

/**
 * An enrolment in the shape `muzzle.ts` searches over, or null.
 *
 * Faces are included: `muzzle.ts` compares them with the same arithmetic and
 * refuses to compare a face with a muzzle, so the kind travels on the
 * descriptor and the matcher enforces it rather than this function.
 */
export function toMuzzleEnrolled(e: StoredEnrolment): MuzzleEnrolled | null {
  if (!e.descriptor) return null;
  if (e.modality !== 'muzzle' && e.modality !== 'face') return null;
  return {
    animalId: e.animalId,
    enrolmentId: e.enrolmentId,
    capturedAt: e.capturedAt,
    descriptor: e.descriptor,
  };
}

/* ----------------------------------------------------------- in memory */

/**
 * The session-only store.
 *
 * `durable` is false and every screen that shows the replica prints it. An
 * operator who has spent a morning enrolling animals is entitled to know
 * before they close the tab, not after.
 */
export function memoryStore(why = 'in memory only'): HerdStore {
  let meta: ReplicaMeta | null = null;
  let herd: Herd | null = null;
  let animals: Animal[] = [];
  let health: HealthEvent[] = [];
  const enrolments = new Map<string, StoredEnrolment>();
  const outbox = new Map<string, Entry>();

  return {
    durable: false,
    why,
    async meta() { return meta; },
    async putReplica(replica) {
      meta = replica.meta;
      herd = replica.herd;
      animals = [...replica.animals];
      health = [...replica.healthEvents];
    },
    async animals() { return [...animals]; },
    async herd() { return herd; },
    async healthEvents() { return [...health]; },
    async enrolments(herdId) {
      return [...enrolments.values()].filter((e) => e.herdId === herdId);
    },
    async putEnrolment(e) { enrolments.set(e.enrolmentId, e); },
    async outbox() { return [...outbox.values()]; },
    async putEntries(entries) { for (const e of entries) outbox.set(e.id, e); },
    async forgetReplica() {
      meta = null; herd = null; animals = []; health = [];
      enrolments.clear();
    },
  };
}

/* --------------------------------------------------------- in IndexedDB */

function promised<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error('idb request failed'));
  });
}

function done(tx: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error ?? new Error('idb transaction failed'));
    tx.onabort = () => reject(tx.error ?? new Error('idb transaction aborted'));
  });
}

function openDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      // `meta` holds exactly one row under a fixed key. A store rather than a
      // loose value because a transaction that spans the meta and the animals
      // is what makes a download atomic: a replica whose meta says nine animals
      // and whose animal store holds four is worse than no replica.
      if (!db.objectStoreNames.contains(S_META)) db.createObjectStore(S_META);
      if (!db.objectStoreNames.contains(S_ANIMALS)) db.createObjectStore(S_ANIMALS, { keyPath: 'id' });
      if (!db.objectStoreNames.contains(S_HEALTH)) db.createObjectStore(S_HEALTH, { keyPath: 'id' });
      if (!db.objectStoreNames.contains(S_ENROLMENTS)) {
        db.createObjectStore(S_ENROLMENTS, { keyPath: 'enrolmentId' });
      }
      if (!db.objectStoreNames.contains(S_OUTBOX)) db.createObjectStore(S_OUTBOX, { keyPath: 'id' });
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error ?? new Error('indexedDB.open failed'));
    req.onblocked = () => reject(new Error('indexedDB blocked by another tab'));
  });
}

function indexedDbStore(db: IDBDatabase): HerdStore {
  return {
    durable: true,
    why: 'IndexedDB',

    async meta() {
      const tx = db.transaction(S_META, 'readonly');
      const value = await promised<ReplicaMeta | undefined>(tx.objectStore(S_META).get('replica'));
      return value ?? null;
    },

    async putReplica(replica) {
      // One transaction over four stores. A download that half-applied would
      // leave a meta claiming a herd whose animals are somebody else's.
      const tx = db.transaction([S_META, S_ANIMALS, S_HEALTH, S_ENROLMENTS], 'readwrite');
      tx.objectStore(S_ANIMALS).clear();
      tx.objectStore(S_HEALTH).clear();
      for (const a of replica.animals) tx.objectStore(S_ANIMALS).put(a);
      for (const h of replica.healthEvents) tx.objectStore(S_HEALTH).put(h);
      tx.objectStore(S_META).put(replica.meta, 'replica');
      tx.objectStore(S_META).put(replica.herd, 'herd');
      await done(tx);
    },

    async animals() {
      const tx = db.transaction(S_ANIMALS, 'readonly');
      return promised<Animal[]>(tx.objectStore(S_ANIMALS).getAll());
    },

    async herd() {
      const tx = db.transaction(S_META, 'readonly');
      const value = await promised<Herd | undefined>(tx.objectStore(S_META).get('herd'));
      return value ?? null;
    },

    async healthEvents() {
      const tx = db.transaction(S_HEALTH, 'readonly');
      return promised<HealthEvent[]>(tx.objectStore(S_HEALTH).getAll());
    },

    async enrolments(herdId) {
      const tx = db.transaction(S_ENROLMENTS, 'readonly');
      const all = await promised<StoredEnrolment[]>(tx.objectStore(S_ENROLMENTS).getAll());
      return all.filter((e) => e.herdId === herdId);
    },

    async putEnrolment(e) {
      const tx = db.transaction(S_ENROLMENTS, 'readwrite');
      tx.objectStore(S_ENROLMENTS).put(e);
      await done(tx);
    },

    async outbox() {
      const tx = db.transaction(S_OUTBOX, 'readonly');
      return promised<Entry[]>(tx.objectStore(S_OUTBOX).getAll());
    },

    async putEntries(entries) {
      const tx = db.transaction(S_OUTBOX, 'readwrite');
      for (const e of entries) tx.objectStore(S_OUTBOX).put(e);
      await done(tx);
    },

    async forgetReplica() {
      // The outbox is deliberately absent from this list. Discarding a replica
      // is housekeeping; discarding unsent observations is destroying evidence,
      // and a person who pressed "download again" did not ask for that.
      const tx = db.transaction([S_META, S_ANIMALS, S_HEALTH, S_ENROLMENTS], 'readwrite');
      tx.objectStore(S_META).clear();
      tx.objectStore(S_ANIMALS).clear();
      tx.objectStore(S_HEALTH).clear();
      tx.objectStore(S_ENROLMENTS).clear();
      await done(tx);
    },
  };
}

/**
 * Get a store, and say which one.
 *
 * Never throws. A device that cannot persist is still a device an operator is
 * standing on a hillside with, and refusing to start would be the wrong
 * trade — but `durable: false` travels with the store and reaches the screen.
 */
export async function open(): Promise<HerdStore> {
  if (typeof indexedDB === 'undefined') {
    return memoryStore('this browser has no IndexedDB');
  }
  try {
    return indexedDbStore(await openDatabase());
  } catch (err) {
    return memoryStore(err instanceof Error ? err.message : String(err));
  }
}
