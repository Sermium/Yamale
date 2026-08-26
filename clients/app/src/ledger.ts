/**
 * Account history, per currency.
 *
 * Every figure here is derived from the chain, never stored. A statement kept
 * as a saved document is a document that can drift from the truth it claims to
 * summarise — and the whole argument for putting money on a ledger is that the
 * ledger is the record. So this reads transfers back and computes balances
 * forward; if the two ever disagree, the chain is right and this is a bug.
 */
import { currencyOf } from './money.ts';
import { decodeMemo, type Remittance } from './iso.ts';

export interface Movement {
  height: number;
  /** Unix ms. The block's timestamp, not the client's clock. */
  at: number;
  denom: string;
  /** Positive into the account, negative out of it. Base units. */
  amount: bigint;
  counterparty: string;
  hash: string;
  /**
   * What the payer wrote, decoded from the transaction memo.
   *
   * The reference was going out with every payment and never coming back: the
   * history showed a name, a figure and a date, which is a receipt rather than
   * a statement. Reconciliation joins on the reference, so a payments app that
   * cannot show it on the receiving side has only done half the job.
   *
   * A memo from an older build is a plain string with no structure, and
   * `decodeMemo` reads that as remittance information in full rather than as a
   * malformed record — so the references that already exist on this chain
   * appear too.
   */
  reference: Remittance;
}

export interface MonthlyStatement {
  denom: string;
  /** First day of the month, local. */
  from: Date;
  to: Date;
  opening: bigint;
  closing: bigint;
  paidIn: bigint;
  paidOut: bigint;
  movements: Movement[];
}

const REST = '/api/rest';

/**
 * Every transfer touching this account, newest first.
 *
 * Two queries rather than one: the chain indexes sender and recipient
 * separately, and asking for "either" is not something the transaction index
 * can answer. Merging here is cheaper than an indexer nobody has built yet.
 */
export async function movements(address: string, limit = 200): Promise<Movement[]> {
  const [received, sent] = await Promise.all([
    search(`transfer.recipient='${address}'`, limit),
    search(`transfer.sender='${address}'`, limit),
  ]);

  const out: Movement[] = [];
  for (const tx of received) out.push(...parse(tx, address, +1));
  for (const tx of sent) out.push(...parse(tx, address, -1));

  // A transfer where the account is both sides nets to nothing and would
  // otherwise appear twice, once in each direction.
  const seen = new Set<string>();
  return out
    .filter((m) => {
      const key = `${m.hash}:${m.denom}:${m.amount}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .sort((a, b) => b.height - a.height);
}

async function search(query: string, limit: number): Promise<unknown[]> {
  try {
    const url = `${REST}/cosmos/tx/v1beta1/txs?query=${encodeURIComponent(query)}`
      + `&order_by=ORDER_BY_DESC&limit=${limit}`;
    const res = await fetch(url);
    if (!res.ok) return [];
    const json = await res.json();
    return json.tx_responses ?? [];
  } catch {
    return [];
  }
}

function parse(tx: any, address: string, sign: 1 | -1): Movement[] {
  const out: Movement[] = [];
  const height = Number(tx.height ?? 0);
  const at = tx.timestamp ? Date.parse(tx.timestamp) : 0;
  const hash = tx.txhash ?? '';
  // The memo is on the transaction body, not on the response envelope. An
  // absent one decodes to an empty unstructured reference rather than throwing.
  const reference = decodeMemo(String(tx.tx?.body?.memo ?? ''));

  for (const event of tx.events ?? []) {
    if (event.type !== 'transfer') continue;
    const attrs: Record<string, string> = {};
    for (const a of event.attributes ?? []) attrs[a.key] = a.value;

    const mine = sign > 0 ? attrs.recipient === address : attrs.sender === address;
    if (!mine) continue;

    for (const part of (attrs.amount ?? '').split(',')) {
      const m = /^(\d+)([a-z\/0-9]+)$/.exec(part.trim());
      if (!m) continue;
      // Only currencies the app knows. Fee movements in uyml still show,
      // which is honest: they left the account.
      if (!currencyOf(m[2])) continue;
      out.push({
        height, at, hash, reference,
        denom: m[2],
        amount: BigInt(m[1]) * BigInt(sign),
        counterparty: (sign > 0 ? attrs.sender : attrs.recipient) ?? '',
      });
    }
  }
  return out;
}

/**
 * A month's statement for one currency.
 *
 * The opening balance is computed *backwards* from today's balance by undoing
 * every movement since the start of the month. That is the only way to get it
 * right without a historical-state query: asking the chain what somebody held
 * six weeks ago needs an archive node, and this app talks to an ordinary one.
 */
export function statement(
  denom: string,
  currentBalance: bigint,
  all: Movement[],
  monthsAgo = 0,
): MonthlyStatement {
  const now = new Date();
  const from = new Date(now.getFullYear(), now.getMonth() - monthsAgo, 1, 0, 0, 0, 0);
  const to = new Date(now.getFullYear(), now.getMonth() - monthsAgo + 1, 1, 0, 0, 0, 0);

  const mine = all.filter((m) => m.denom === denom);
  const inMonth = mine.filter((m) => m.at >= from.getTime() && m.at < to.getTime());
  const since = mine.filter((m) => m.at >= from.getTime());

  const net = (list: Movement[]) => list.reduce((sum, m) => sum + m.amount, 0n);

  const closing = currentBalance - net(mine.filter((m) => m.at >= to.getTime()));
  const opening = closing - net(inMonth);

  return {
    denom, from, to, opening, closing,
    paidIn: inMonth.filter((m) => m.amount > 0n).reduce((s, m) => s + m.amount, 0n),
    paidOut: -inMonth.filter((m) => m.amount < 0n).reduce((s, m) => s + m.amount, 0n),
    movements: inMonth,
  };
}

/** The months that actually have movements, newest first, capped. */
export function monthsWithActivity(all: Movement[], max = 12): number[] {
  const now = new Date();
  const out: number[] = [];
  for (let i = 0; i < max; i++) {
    const from = new Date(now.getFullYear(), now.getMonth() - i, 1).getTime();
    const to = new Date(now.getFullYear(), now.getMonth() - i + 1, 1).getTime();
    if (i === 0 || all.some((m) => m.at >= from && m.at < to)) out.push(i);
  }
  return out;
}
