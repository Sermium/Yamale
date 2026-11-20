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
export interface PieceOutcome { status: 'registered' | 'alreadyRegistered' | 'failed' | 'notRun'; jewelId: string }

/**
 * Messages per transaction. Measured 2026-11-20 with scripts/devnet/measure-register-batch-gas.sh
 * on a local devnet against JEWEL_GAS (1,650,000), keeping 25% headroom (at most 1,237,500):
 * 24 registrations 1,148,821 gas, 25 registrations 1,198,414 (fits), 26 registrations
 * 1,245,967 (over). About 47,000 gas a message plus 28,000 a transaction.
 */
export const REGISTER_BATCH = 25;

export async function registerPieces(pieces: Piece[], deps: RegisterDeps, size = REGISTER_BATCH):
  Promise<{ outcomes: Map<number, PieceOutcome>; stopped: { text: string; raw: string } | null }> {
  const outcomes = new Map<number, PieceOutcome>(pieces.map((p) => [p.id, { status: 'notRun', jewelId: '' }]));
  let done = 0;
  deps.progress(0, pieces.length);
  for (let i = 0; i < pieces.length; i += size) {
    const batch = pieces.slice(i, i + size);
    const taken = await deps.taken(batch.map((p) => p.inscription));
    if (!taken.ok) return { outcomes, stopped: { text: taken.text, raw: taken.raw } };
    const todo: Piece[] = [];
    for (const p of batch) {
      const id = taken.value.get(p.inscription);
      if (id) { outcomes.set(p.id, { status: 'alreadyRegistered', jewelId: id }); done++; } else todo.push(p);
    }
    const hashes: string[] = [];
    for (const p of todo) {
      const up = await deps.upload(p.record);
      if (!up.ok) {
        for (const q of todo) outcomes.set(q.id, { status: 'failed', jewelId: '' });
        return { outcomes, stopped: { text: up.text, raw: up.raw } };
      }
      hashes.push(up.value);
    }
    if (todo.length > 0) {
      const sent = await deps.submit(todo.map((p, j) => ({ inscription: p.inscription, hash: hashes[j]! })));
      if (!sent.ok) {
        for (const q of todo) outcomes.set(q.id, { status: 'failed', jewelId: '' });
        return { outcomes, stopped: { text: sent.text, raw: sent.raw } };
      }
      todo.forEach((p, j) => outcomes.set(p.id, { status: 'registered', jewelId: sent.value[j] ?? '' }));
      done += todo.length;
    }
    deps.progress(done, pieces.length);
  }
  return { outcomes, stopped: null };
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
