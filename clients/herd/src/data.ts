/**
 * Reading the chain and the device into the screens, with the silences intact.
 *
 * Every hook returns a `Load<T>` whose failure keeps the four reasons `abci.ts`
 * draws — answered-and-absent, refused, unreachable, offline — because on this
 * surface those need four different sentences and a null return would collapse
 * them into "nothing here".
 *
 * # The local copy is a first-class source, not a cache
 *
 * A cache is something you fall back to when the real thing is slow. The
 * replica is the thing this application is FOR: §5 scopes it to one herd
 * precisely so that a keeper in a pastoral zone works against it all day and
 * the chain is what they reconcile with in the evening. So the herd screen
 * reads the replica first and does not go to the network at all, and the
 * identify screen says which of the two answered — because "found in the copy
 * on this device" and "found in the national register" are different claims and
 * only the second is current.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { head, online, type Head, type Outcome } from './abci.ts';
import {
  agents as fetchAgents,
  animal as fetchAnimal,
  animalByNationalId,
  animalByTag,
  animals as fetchAnimals,
  healthEvents as fetchHealthEvents,
  herd as fetchHerd,
  pedigree as fetchPedigree,
  transfersByAnimal,
} from './chain.ts';
import {
  classify,
  type Agent,
  type Animal,
  type HealthEvent,
  type Herd,
  type Line,
  type Transfer,
  asHeight,
} from './herd.ts';
import {
  biometricScope,
  freshness,
  index,
  type BiometricScope,
  type Freshness,
  type Index,
  type Replica,
  type ReplicaMeta,
} from './replica.ts';
import { merge, type Entry } from './outbox.ts';
import { open, toEnrolled, type HerdStore, type StoredEnrolment } from './store.ts';

/** A read in flight, or its result. */
export type Load<T> = { loading: true } | { loading: false; outcome: Outcome<T> };

function useRead<T>(run: () => Promise<Outcome<T>>, deps: unknown[]): [Load<T>, () => void] {
  const [state, setState] = useState<Load<T>>({ loading: true });
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    let live = true;
    setState({ loading: true });
    run().then((outcome) => { if (live) setState({ loading: false, outcome }); });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, nonce]);

  return [state, useCallback(() => setNonce((n) => n + 1), [])];
}

/* ----------------------------------------------------------- the clocks */

/**
 * The chain's head, refreshed while the app is open.
 *
 * A minute rather than thirty seconds, because this runs on a phone battery in
 * a field and nothing on these screens changes within a minute. When the device
 * is offline the poll stops entirely: nine seconds of failing fetch every
 * minute is a measurable share of a day's charge and buys nothing.
 */
export function useHead(): Head {
  const [state, setState] = useState<Head>({ known: false });
  const connected = useOnline();

  useEffect(() => {
    if (!connected) { setState({ known: false }); return; }
    let live = true;
    const tick = () => { head().then((h) => { if (live) setState(h); }); };
    tick();
    const timer = setInterval(tick, 60_000);
    return () => { live = false; clearInterval(timer); };
  }, [connected]);

  return state;
}

/**
 * Whether the device thinks it has a connection.
 *
 * `navigator.onLine` is optimistic — it reports the interface, not the internet
 * — so this is trusted in the negative and merely believed in the positive.
 * That asymmetry is why a screen never says "connected", only "no connection"
 * or nothing at all.
 */
export function useOnline(): boolean {
  const [state, setState] = useState(online);
  useEffect(() => {
    const on = () => setState(true);
    const off = () => setState(false);
    window.addEventListener('online', on);
    window.addEventListener('offline', off);
    return () => {
      window.removeEventListener('online', on);
      window.removeEventListener('offline', off);
    };
  }, []);
  return state;
}

/* ------------------------------------------------------------ the store */

/**
 * The device's own database, opened once for the life of the application.
 *
 * Null while it is opening. Never throws: a device that cannot persist is still
 * a device somebody is standing on a hillside with, and `store.durable` carries
 * the bad news to the screen instead of a crash carrying it to nobody.
 */
export function useStore(): HerdStore | null {
  const [store, setStore] = useState<HerdStore | null>(null);
  useEffect(() => {
    let live = true;
    open().then((s) => { if (live) setStore(s); });
    return () => { live = false; };
  }, []);
  return store;
}

/** Everything the device holds about the herd it is carrying. */
export interface LocalHerd {
  meta: ReplicaMeta | null;
  herd: Herd | null;
  animals: Animal[];
  healthEvents: HealthEvent[];
  enrolments: StoredEnrolment[];
  index: Index | null;
  scope: BiometricScope;
  outbox: Entry[];
  durable: boolean;
  why: string;
}

const EMPTY_LOCAL: LocalHerd = {
  meta: null, herd: null, animals: [], healthEvents: [], enrolments: [],
  index: null,
  scope: { enrolled: [], withTemplate: 0, withoutTemplate: 0 },
  outbox: [], durable: false, why: 'opening',
};

/**
 * The replica, read whole.
 *
 * A herd is hundreds of records, so reading all of it is cheaper than the
 * bookkeeping of not doing. The index is memoised against the animal array
 * rather than rebuilt per keystroke, because an operator typing a fifteen-digit
 * tag would otherwise rebuild it fifteen times.
 */
export function useLocalHerd(store: HerdStore | null): [LocalHerd, () => void] {
  const [raw, setRaw] = useState<Omit<LocalHerd, 'index' | 'scope'>>(EMPTY_LOCAL);
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    if (!store) return;
    let live = true;
    (async () => {
      const meta = await store.meta();
      const [herd, animals, healthEvents, outbox] = await Promise.all([
        store.herd(), store.animals(), store.healthEvents(), store.outbox(),
      ]);
      const enrolments = meta ? await store.enrolments(meta.herdId) : [];
      if (!live) return;
      setRaw({
        meta, herd, animals, healthEvents, enrolments, outbox,
        durable: store.durable, why: store.why,
      });
    })();
    return () => { live = false; };
  }, [store, nonce]);

  const idx = useMemo(
    () => (raw.animals.length ? index(raw.animals) : null),
    [raw.animals],
  );
  const scope = useMemo(
    // Iris enrolments only: toEnrolled returns null for a muzzle, and the
    // scope is a statement about what the IRIS matcher can compare. Passing
    // the nulls through would have counted muzzle enrolments as iris coverage.
    () =>
      biometricScope(
        raw.animals,
        raw.enrolments.map(toEnrolled).filter((e): e is NonNullable<typeof e> => e !== null),
      ),
    [raw.animals, raw.enrolments],
  );

  return [{ ...raw, index: idx, scope }, useCallback(() => setNonce((n) => n + 1), [])];
}

export function useFreshness(meta: ReplicaMeta | null, head: Head): Freshness | null {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 60_000);
    return () => clearInterval(timer);
  }, []);
  if (!meta) return null;
  return freshness(meta, now, head.known ? head.height : null);
}

/* ------------------------------------------------------- the download */

export type DownloadState =
  | { phase: 'idle' }
  | { phase: 'working'; what: string }
  | { phase: 'done'; animals: number }
  | { phase: 'failed'; detail: string };

/**
 * Pull one herd onto the device.
 *
 * Three queries, then one atomic write. The herd is fetched first because it is
 * what names the thing — a download that produced forty animals and no herd
 * record would leave a screen listing beasts under no heading — and because its
 * failure is the cheapest one to report.
 *
 * The animals query pages to exhaustion in `chain.ts`. A replica that quietly
 * held the first hundred of two hundred animals would answer "not one of yours"
 * about the second hundred, which is the worst failure this application has.
 */
export function useDownload(
  store: HerdStore | null,
  onDone: () => void,
): [DownloadState, (herdId: string) => void] {
  const [state, setState] = useState<DownloadState>({ phase: 'idle' });
  const running = useRef(false);

  const start = useCallback((herdId: string) => {
    if (!store || running.current) return;
    running.current = true;
    setState({ phase: 'working', what: herdId });

    (async () => {
      try {
        const herd = await fetchHerd(herdId);
        if (!herd.ok) {
          setState({ phase: 'failed', detail: `${herd.reason}: ${herd.detail}` });
          return;
        }
        const [animals, events] = await Promise.all([
          fetchAnimals(herdId), fetchHealthEvents(herdId),
        ]);
        if (!animals.ok) {
          setState({ phase: 'failed', detail: `${animals.reason}: ${animals.detail}` });
          return;
        }

        const replica: Replica = {
          meta: {
            herdId,
            chainId: '',
            height: asHeight(animals.height || herd.height),
            downloadedAt: Date.now(),
            animals: animals.value.length,
          },
          herd: herd.value,
          animals: animals.value,
          // A failed health read does not fail the download. The animals are
          // what the app is for; the campaigns are a panel on one screen, and
          // an empty one reads correctly because the screen distinguishes it.
          healthEvents: events.ok ? events.value : [],
        };
        await store.putReplica(replica);
        setState({ phase: 'done', animals: animals.value.length });
        onDone();
      } catch (err) {
        setState({ phase: 'failed', detail: err instanceof Error ? err.message : String(err) });
      } finally {
        running.current = false;
      }
    })();
  }, [store, onDone]);

  return [state, start];
}

/* ------------------------------------------------------------- one animal */

/**
 * One animal, from wherever it can be got.
 *
 * The local copy answers first and the chain is asked anyway when there is a
 * connection, because a record that has been frozen or disputed since the
 * download is exactly the fact an operator must not miss. The screen renders
 * the local copy immediately and replaces it when the chain answers, and says
 * which it is showing.
 */
export interface AnimalView {
  animal: Animal;
  from: 'chain' | 'replica';
  /** Set when the chain was asked and did not answer, so the local copy stands. */
  chainSilent: boolean;
}

export function useAnimal(id: string, local: LocalHerd): [Load<AnimalView>, () => void] {
  const fromReplica = local.index?.byId.get(id) ?? null;

  return useRead(async (): Promise<Outcome<AnimalView>> => {
    const got = await fetchAnimal(id);
    if (got.ok) {
      return { ok: true, value: { animal: got.value, from: 'chain', chainSilent: false }, height: got.height };
    }
    if (fromReplica) {
      return {
        ok: true,
        value: { animal: fromReplica, from: 'replica', chainSilent: got.reason !== 'not-found' },
        height: local.meta?.height ?? 0,
      };
    }
    return got;
  }, [id, fromReplica?.id ?? '']);
}

export function usePedigree(animalId: string, depth = 4): [Load<Line>, () => void] {
  return useRead(() => fetchPedigree(animalId, depth), [animalId, depth]);
}

export function useTransfers(animalId: string): [Load<Transfer[]>, () => void] {
  return useRead(() => transfersByAnimal(animalId), [animalId]);
}

export function useAgents(): [Load<Agent[]>, () => void] {
  return useRead(() => fetchAgents(''), []);
}

/* ------------------------------------------------- resolving an identifier */

/** Where an identifier lookup got its answer. */
export interface Resolved {
  animal: Animal;
  from: 'chain' | 'replica';
  by: 'tag' | 'national-id';
}

/**
 * Resolve what was typed, against the chain when there is one and against the
 * copy on the device when there is not.
 *
 * The replica is tried FIRST and its answer is returned immediately, then the
 * chain is asked. On a device in a pen that is the difference between an answer
 * in a hundred milliseconds and an answer in four seconds, and the local answer
 * is right almost always — it is the operator's own herd.
 *
 * When the two disagree the chain wins and the screen says the copy is stale.
 * That is the one place a "winner" is chosen in this application, and it is
 * chosen because one of the two is definitionally authoritative. It is not the
 * §5 case: that is two OBSERVATIONS, and neither of those is authoritative.
 */
export async function resolveIdentifier(
  raw: string,
  local: LocalHerd,
): Promise<
  | { kind: 'resolved'; value: Resolved }
  | { kind: 'not-found'; by: 'tag' | 'national-id'; value: string; scope: 'herd' | 'national' }
  | { kind: 'unrecognised'; raw: string }
  | { kind: 'collision'; animalIds: string[] }
  | { kind: 'silent'; reason: 'offline' | 'unreachable' | 'refused'; detail: string }
> {
  const entry = classify(raw);
  if (entry.kind === 'empty' || entry.kind === 'unrecognised') {
    return { kind: 'unrecognised', raw };
  }
  const by = entry.kind;

  const collision = local.index?.collisions.find(
    (c) => c.value === entry.value && c.kind === (by === 'tag' ? 'tag' : 'national-id'),
  );
  if (collision) return { kind: 'collision', animalIds: collision.animalIds };

  const localHit = by === 'tag'
    ? local.index?.byTag.get(entry.value)
    : local.index?.byNationalId.get(entry.value);

  const got = by === 'tag'
    ? await animalByTag(entry.value)
    : await animalByNationalId(entry.value);

  if (got.ok) return { kind: 'resolved', value: { animal: got.value, from: 'chain', by } };

  if (got.reason === 'not-found') {
    // The chain answered. A local hit against a chain miss means the replica is
    // holding a record the registry no longer has, which is a finding rather
    // than a fallback — surfaced as not-found so the screen can say so.
    return { kind: 'not-found', by, value: entry.value, scope: 'national' };
  }

  if (localHit) return { kind: 'resolved', value: { animal: localHit, from: 'replica', by } };

  if (local.index) {
    return { kind: 'not-found', by, value: entry.value, scope: 'herd' };
  }
  return { kind: 'silent', reason: got.reason, detail: got.detail };
}

/* ------------------------------------------------------------- the outbox */

/**
 * The queue, and the one way anything is added to it.
 *
 * Every write goes through `merge`, which is union-by-id and has no branch
 * capable of dropping an entry. Nothing in this hook deletes; `advance` in
 * `outbox.ts` turns a refusal into a refused entry that stays.
 */
export function useOutbox(
  store: HerdStore | null,
  entries: readonly Entry[],
  reload: () => void,
): (incoming: readonly Entry[]) => Promise<void> {
  return useCallback(async (incoming) => {
    if (!store) return;
    await store.putEntries(merge(entries, incoming));
    reload();
  }, [store, entries, reload]);
}
