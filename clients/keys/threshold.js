// Sealed accounts, shown rather than described.
//
// The claim this page exists to make is one sentence: the operator cannot move
// a customer's money. Every payments system says something like it, and almost
// all of them mean "we have a policy about that". Here it is a statement about
// arithmetic, and the difference between those two things is the entire
// product. So the page does not assert it. It runs the arithmetic in front of
// you and reads the consequences off a public chain.
//
// # What the claim is now, because the mechanism changed
//
// It used to be threshold signing: the key existed in three shares, no one of
// them could produce a signature, and any two signed together. The decision of
// 2026-09-18 — a deal is between a buyer and a seller, and the services appear
// only in a recovery — removed the reason for that, and the device now signs
// alone with an ordinary secp256k1 key.
//
// The sentence survives, and the arithmetic behind it changed:
//
//   the key is SEALED under the account holder's password, and only then split
//   three ways. Two services run by the same operator can put their two parts
//   together and what they get is a ciphertext they cannot open.
//
// Sealing BEFORE splitting is the whole thing. Split the raw key and those two
// services reconstruct it between them — so "no single service can move your
// money" would be true while "not even both of them can" was false, and the
// second is the one that matters when one company runs both.
//
// # What is real here and what is staged
//
// Being exact about this matters more than looking impressive.
//
// REAL: the account on the chain, its transactions and their heights — made
// under the threshold scheme, which is why they are labelled as history rather
// than as a demonstration of what runs today. The sealed values in the
// demonstration are real too: they were produced by the Go implementation and
// are read from vectors.json, the same file its tests answer to.
//
// STAGED: the split and the reconstruction below run in this one page. In
// production the three parts are a phone and two independent services that
// never meet. Running them together is what lets you watch it; it is also
// precisely the arrangement the design forbids, so everything used for it is a
// throwaway whose values are published on purpose.
//
// NOT RUN AT ALL: the sealing itself. It is Argon2id at 128 MiB and would mean
// shipping libsodium to every visitor. This page used to load a 19 MB
// WebAssembly module and that is the mistake not to repeat, so the sealed blob
// is shown rather than computed, and says so where it is shown.

const RPC = '/api/rpc';
const TIMEOUT_MS = 15000;

/**
 * The chain answered, and the answer is that it does not have this.
 *
 * Distinct from Unreachable on purpose. The public node this page reads is
 * state-synced, so it holds no block before its snapshot and no index of the
 * transactions in them — asking for a hash older than that is answered "not
 * found", correctly, by a node that is working perfectly. Collapsing the two
 * into one error made a node doing exactly what it was configured to do look
 * like a node that was down, on the page whose entire job is to show that a
 * claim can be checked.
 */
export class NotHeld extends Error {
  constructor(what) {
    super(`the chain does not hold ${what}`);
    this.name = 'NotHeld';
  }
}

/** A call that did not answer, named — so a caller can tell "no" from "unknown". */
export class Unreachable extends Error {
  constructor(what, cause) {
    super(`could not reach the chain: ${what}`);
    this.name = 'Unreachable';
    this.cause = cause;
  }
}

async function rpc(path) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), TIMEOUT_MS);
  try {
    const res = await fetch(`${RPC}/${path}`, {
      signal: controller.signal,
      headers: { accept: 'application/json' },
    });
    // The body is read BEFORE the status is judged, because Tendermint answers
    // "this transaction is not in my index" with an HTTP 500 carrying a
    // JSON-RPC error — so a guard on res.ok alone turns the node's clearest
    // possible answer into "the node did not answer". That is the whole
    // difference this page has to be able to draw.
    let body = null;
    try { body = await res.json(); } catch { /* not json; the status decides */ }

    if (body?.error) {
      const detail = String(body.error.data || body.error.message || '');
      if (/not found/i.test(detail)) throw new NotHeld(detail.replace(/^tx\s*/, 'transaction '));
      throw new Unreachable(`${path}: ${body.error.message || 'error'}`);
    }
    if (!res.ok) throw new Unreachable(`${path} returned ${res.status}`);
    return body.result;
  } catch (err) {
    if (err instanceof Unreachable || err instanceof NotHeld) throw err;
    throw new Unreachable(path, err);
  } finally {
    clearTimeout(timer);
  }
}

/**
 * One transaction, read by hash.
 *
 * Deliberately reports the height and the delivered code together. A hash on
 * its own is the commonest way a failed transaction gets read as a successful
 * one — it lands in a block either way — and this page is making a claim about
 * transactions that succeeded, so it has to show the half that says so.
 */
export async function transaction(hash) {
  const result = await rpc(`tx?hash=0x${hash}&prove=false`);
  return {
    hash,
    height: Number(result.height),
    code: result.tx_result?.code ?? 0,
    gasUsed: Number(result.tx_result?.gas_used ?? 0),
  };
}

/** The chain's current height, so every claim on this page can be dated. */
export async function height() {
  const status = await rpc('status');
  return Number(status.sync_info.latest_block_height);
}

/**
 * The oldest block this node still holds.
 *
 * A state-synced node starts from a snapshot and keeps nothing before it, so
 * everything earlier — including the index that answers a query by transaction
 * hash — is simply not there. That is a deployment decision rather than a
 * fault, and it is the whole explanation for a transaction this page knows
 * happened coming back "not found". Reported so the page can say so instead of
 * leaving a reader to conclude the claim was never true.
 */
export async function earliestBlock() {
  const status = await rpc('status');
  return Number(status.sync_info.earliest_block_height);
}

// --------------------------------------------------------------- protobuf

/** Minimal varint writer — enough for the one query this page makes. */
export function varint(n) {
  const out = [];
  let value = BigInt(n);
  do {
    let byte = Number(value & 0x7fn);
    value >>= 7n;
    if (value > 0n) byte |= 0x80;
    out.push(byte);
  } while (value > 0n);
  return out;
}

/** A length-delimited string in field `field`. */
export function stringField(field, text) {
  const bytes = new TextEncoder().encode(text);
  return [...varint((field << 3) | 2), ...varint(bytes.length), ...bytes];
}

export function toHex(bytes) {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

function fromBase64(b64) {
  const binary = atob(b64);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) out[i] = binary.charCodeAt(i);
  return out;
}

/**
 * Every coin an account holds.
 *
 * Hand-decoded rather than pulled through a generated client, because this
 * console has no build step — the same reason clients/land carries its own
 * codec. The response is repeated Coin { denom = 1, amount = 2 }.
 */
export async function balances(address) {
  const request = new Uint8Array(stringField(1, address));
  const result = await rpc(
    `abci_query?path=%22/cosmos.bank.v1beta1.Query/AllBalances%22&data=0x${toHex(request)}`,
  );
  const response = result.response;
  if (response.code !== 0) {
    throw new Unreachable(`bank query: ${response.log || `code ${response.code}`}`);
  }
  const value = response.value ? fromBase64(response.value) : new Uint8Array();
  return { coins: decodeBalances(value), height: Number(response.height) };
}

/** Decodes the repeated Coin messages of an AllBalances response. */
export function decodeBalances(bytes) {
  const coins = [];
  let i = 0;
  const readVarint = () => {
    let result = 0n;
    let shift = 0n;
    for (;;) {
      const byte = bytes[i];
      i += 1;
      result |= BigInt(byte & 0x7f) << shift;
      if ((byte & 0x80) === 0) break;
      shift += 7n;
    }
    return result;
  };
  while (i < bytes.length) {
    const tag = Number(readVarint());
    const field = tag >> 3;
    const wire = tag & 7;
    if (field !== 1 || wire !== 2) {
      // Not a coin — skip it rather than guessing. A decoder that assumes the
      // shape of a message it does not recognise is one that reports a balance
      // nobody holds.
      if (wire === 2) {
        i += Number(readVarint());
      } else if (wire === 0) {
        readVarint();
      } else {
        break;
      }
      continue;
    }
    const length = Number(readVarint());
    const end = i + length;
    let denom = '';
    let amount = '';
    while (i < end) {
      const innerTag = Number(readVarint());
      const innerField = innerTag >> 3;
      const innerLength = Number(readVarint());
      const text = new TextDecoder().decode(bytes.slice(i, i + innerLength));
      i += innerLength;
      if (innerField === 1) denom = text;
      if (innerField === 2) amount = text;
    }
    coins.push({ denom, amount });
  }
  return coins;
}

// ------------------------------------------------------- the door out

/**
 * The key-disclosure pathway, read from the chain.
 *
 * This page's whole claim is that a key exists in three shares and in no other
 * form. The obvious question is then "so can I ever have it?", and a page that
 * did not answer would be making the claim and hiding the cost.
 *
 * The answer lives in x/selfcustody, on the chain, so that the terms are the
 * ones actually enforced rather than a paragraph somebody wrote here. The
 * waiting period is a chain parameter the operator cannot shorten for a
 * favoured customer or lengthen for an inconvenient one; the warning text is
 * pinned by HASH, so what a person agreed to can be checked rather than
 * asserted.
 *
 * Params { disclosure_delay = 1 (int64), acknowledgement_version = 2 (string),
 *          acknowledgement_hash = 3 (string), request_expiry = 4 (int64) },
 * wrapped in QueryParamsResponse at field 1.
 */
export async function disclosureTerms() {
  const result = await rpc(
    'abci_query?path=%22/blockchain.selfcustody.v1.Query/Params%22&data=0x',
  );
  const response = result.response;
  if (response.code !== 0) {
    // A chain built without this module answers with a non-zero code rather
    // than with empty params, and the two mean opposite things: "no such
    // query" is a deployment that has no pathway at all, which this page must
    // say plainly rather than render as a pathway with blank terms.
    throw new Unreachable(`selfcustody params: ${response.log || `code ${response.code}`}`);
  }
  const outer = response.value ? fromBase64(response.value) : new Uint8Array();
  const params = fieldsOf(outer).get(1)?.[0];
  if (!params) throw new Unreachable('selfcustody params: empty response');

  const f = fieldsOf(params);
  const text = (n) => (f.get(n) ? new TextDecoder().decode(f.get(n)[0]) : '');
  const num = (n) => Number(f.get(n) ? f.get(n)[0] : 0n);

  return {
    delaySeconds: num(1),
    version: text(2),
    hash: text(3),
    expirySeconds: num(4),
    height: Number(response.height),
  };
}

/**
 * How many accounts have taken their keys, and how many are inside the wait.
 *
 * A count and never a list. A public roll of accounts whose private keys are
 * known to be sitting in somebody's downloads folder is a target list, ordered
 * by how easy each one is to rob — so x/selfcustody publishes no query that
 * returns one. What the count buys is that the RATE stays visible: an operator
 * quietly pushing customers out of custody, or a sudden spike meaning an attack
 * campaign is under way, are both observable without anybody being named.
 *
 * QueryCountResponse { disclosed = 1 (uint64), pending = 2 (uint64) }.
 */
export async function disclosureCount() {
  const result = await rpc(
    'abci_query?path=%22/blockchain.selfcustody.v1.Query/Count%22&data=0x',
  );
  const response = result.response;
  if (response.code !== 0) {
    throw new Unreachable(`selfcustody count: ${response.log || `code ${response.code}`}`);
  }
  const f = fieldsOf(response.value ? fromBase64(response.value) : new Uint8Array());
  return {
    disclosed: Number(f.get(1)?.[0] ?? 0n),
    pending: Number(f.get(2)?.[0] ?? 0n),
  };
}

/**
 * Every field of a protobuf message, keyed by field number.
 *
 * A map of arrays rather than an object, so a repeated field keeps all of its
 * entries. Unknown fields are kept rather than refused: a node running newer
 * code than this page emits fields it has never heard of, and the right
 * behaviour is to ignore them and parse the message around them.
 */
export function fieldsOf(bytes) {
  const out = new Map();
  const put = (n, v) => { const l = out.get(n); if (l) l.push(v); else out.set(n, [v]); };
  let i = 0;
  const readVarint = () => {
    let result = 0n;
    let shift = 0n;
    for (;;) {
      if (i >= bytes.length) throw new Error('truncated varint');
      const byte = bytes[i];
      i += 1;
      result |= BigInt(byte & 0x7f) << shift;
      if ((byte & 0x80) === 0) return result;
      shift += 7n;
    }
  };
  while (i < bytes.length) {
    const tag = Number(readVarint());
    const field = tag >> 3;
    switch (tag & 7) {
      case 0: put(field, readVarint()); break;
      case 2: {
        const length = Number(readVarint());
        put(field, bytes.slice(i, i + length));
        i += length;
        break;
      }
      case 5: i += 4; break;
      case 1: i += 8; break;
      default: return out;
    }
  }
  return out;
}

// ---------------------------------------------------- what used to be here
//
// Everything below this line was the threshold protocol: loading a 19 MB
// WebAssembly module, driving a GG18 signing party across two simulated
// participants, and rendering the messages they exchanged.
//
// It is gone because the thing it demonstrated is gone. The architecture
// decision of 2026-09-18 — a deal is between a buyer and a seller, and the
// services appear only in a recovery — removed the reason for threshold
// signing, and the device now signs alone with an ordinary secp256k1 key.
//
// What replaced it is in sharing.js, and it is a better demonstration for
// exactly the reason the old one was hard to love: it costs nothing to load, it
// runs instantly rather than over several seconds of proofs, and it shows the
// claim a reader is entitled to disbelieve — that two parts held by two
// services run by the same operator rebuild something that is not the key.
//
// digestOf went with it. It hashed arbitrary text for the protocol to sign, and
// it used crypto.subtle, which is undefined on this site because plain HTTP is
// not a secure context. Nothing here needs a digest any more.
