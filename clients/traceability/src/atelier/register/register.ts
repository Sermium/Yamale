/**
 * Registering pieces in batches of several MsgRegisterJewel per transaction.
 *
 * The chain is the progress record. Before each batch its codes are checked
 * against the register again, so a run stopped halfway, a second tab or a
 * double click cannot register a code twice; running the same file again
 * registers only what is missing. Each batch's records are stored just before
 * the batch: the gate allows 30 writes a minute, and storing as we go puts
 * jewels on the register while the rest wait their turn.
 */

export type Step<T> = { ok: true; value: T } | { ok: false; text: string; raw: string };

export interface RegisterDeps {
  /** Which of these codes this jeweler already registered: code → jewel id. */
  taken(inscriptions: string[]): Promise<Step<Map<string, string>>>;
  upload(record: Uint8Array): Promise<Step<string>>;
  /** One transaction; the new jewel ids in message order. */
  submit(batch: { inscription: string; hash: string }[]): Promise<Step<string[]>>;
  progress(done: number, of: number): void;
}

export interface Piece { id: number; inscription: string; record: Uint8Array }
export interface PieceOutcome { status: 'registered' | 'alreadyRegistered' | 'failed' | 'unknown' | 'notRun'; jewelId: string }

/**
 * Messages per transaction. Measured 2026-11-20 with scripts/devnet/measure-register-batch-gas.sh
 * on a local devnet, worst case (40-character code, real record URI), against JEWEL_GAS
 * (1,650,000) keeping 25% headroom (at most 1,237,500): 22 registrations 1,155,481 gas,
 * 23 registrations 1,206,702 (fits), 24 registrations 1,257,923 (over).
 * About 51,200 gas a message plus 28,000 a transaction.
 */
export const REGISTER_BATCH = 23;

export interface RegisterResult {
  outcomes: Map<number, PieceOutcome>;
  stopped: { text: string; raw: string } | null;
  /** True when a dependency threw (typically the account locked itself) rather than answering "no". */
  interrupted: boolean;
}

/** A dependency that throws becomes a stop, never an exception: what is already registered must survive. */
type Threw = { threw: string };
async function guard<T>(call: () => Promise<T>): Promise<T | Threw> {
  try { return await call(); } catch (e) { return { threw: e instanceof Error ? e.message : String(e) }; }
}
const threw = <T>(r: T | Threw): r is Threw => typeof r === 'object' && r !== null && 'threw' in r;

export async function registerPieces(pieces: Piece[], deps: RegisterDeps, size = REGISTER_BATCH): Promise<RegisterResult> {
  const outcomes = new Map<number, PieceOutcome>(pieces.map((p) => [p.id, { status: 'notRun', jewelId: '' }]));
  const interrupt = (raw: string): RegisterResult => ({ outcomes, stopped: { text: '', raw }, interrupted: true });
  let done = 0;
  deps.progress(0, pieces.length);
  for (let i = 0; i < pieces.length; i += size) {
    const batch = pieces.slice(i, i + size);
    const taken = await guard(() => deps.taken(batch.map((p) => p.inscription)));
    if (threw(taken)) return interrupt(taken.threw);
    if (!taken.ok) return { outcomes, stopped: { text: taken.text, raw: taken.raw }, interrupted: false };
    const todo: Piece[] = [];
    for (const p of batch) {
      const id = taken.value.get(p.inscription);
      if (id) { outcomes.set(p.id, { status: 'alreadyRegistered', jewelId: id }); done++; } else todo.push(p);
    }
    const hashes: string[] = [];
    for (const p of todo) {
      const up = await guard(() => deps.upload(p.record));
      if (threw(up)) {
        outcomes.set(p.id, { status: 'failed', jewelId: '' });
        return interrupt(up.threw);
      }
      if (!up.ok) {
        outcomes.set(p.id, { status: 'failed', jewelId: '' });
        return { outcomes, stopped: { text: up.text, raw: up.raw }, interrupted: false };
      }
      hashes.push(up.value);
    }
    if (todo.length > 0) {
      const sent = await guard(() => deps.submit(todo.map((p, j) => ({ inscription: p.inscription, hash: hashes[j]! }))));
      if (threw(sent)) {
        // The transaction may or may not have been broadcast: the register is the only judge.
        for (const q of todo) outcomes.set(q.id, { status: 'unknown', jewelId: '' });
        return interrupt(sent.threw);
      }
      if (!sent.ok) {
        for (const q of todo) outcomes.set(q.id, { status: 'failed', jewelId: '' });
        return { outcomes, stopped: { text: sent.text, raw: sent.raw }, interrupted: false };
      }
      todo.forEach((p, j) => outcomes.set(p.id, { status: 'registered', jewelId: sent.value[j] ?? '' }));
      done += todo.length;
    }
    deps.progress(done, pieces.length);
  }
  return { outcomes, stopped: null, interrupted: false };
}

/** Retry while the answer says "busy" (the gate's 429, the API's 503), pausing longer each time. */
export async function paced<T>(attempt: () => Promise<T>, busy: (r: T) => boolean, sleep: (ms: number) => Promise<void>,
  waits: number[] = [2500, 2500, 5000, 5000, 10000, 10000, 20000, 20000]): Promise<T> {
  let r = await attempt();
  for (const ms of waits) {
    if (!busy(r)) return r;
    await sleep(ms);
    r = await attempt();
  }
  return r;
}
