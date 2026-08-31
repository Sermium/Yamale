// Threshold accounts, shown rather than described.
//
// The claim this page exists to make is one sentence: the operator cannot move
// a customer's money. Every payments system says something like it, and almost
// all of them mean "we have a policy about that". Here it is a statement about
// arithmetic — the key exists in three shares and no one of them can produce a
// signature — and the difference between those two things is the entire
// product. So the page does not assert it. It runs the protocol in front of
// you, refuses in front of you, and reads the consequences off a public chain.
//
// # What is real here and what is staged
//
// Being exact about this matters more than looking impressive.
//
// REAL: the account on the chain, its transactions, their heights, and the fact
// that the second was signed by shares that did not exist when the first was —
// created in a password reset that did not move the address. Anybody can check
// those against a block explorer.
//
// STAGED: the live demonstration below runs BOTH parties inside this one page.
// In production they are a phone and a server that never meet, exchanging the
// messages you can see in the transcript. Running them together is what lets
// you watch the traffic; it is also precisely the arrangement the design
// forbids, so the shares used for it belong to a throwaway account that holds
// nothing and whose share files are published on purpose.

const RPC = '/api/rpc';
const TIMEOUT_MS = 15000;

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
    if (!res.ok) throw new Unreachable(`${path} returned ${res.status}`);
    const body = await res.json();
    if (body.error) throw new Unreachable(`${path}: ${body.error.message || 'error'}`);
    return body.result;
  } catch (err) {
    if (err instanceof Unreachable) throw err;
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

// ------------------------------------------------------------------ wasm

let wasmReady = null;

/**
 * Load the protocol.
 *
 * Deferred until somebody asks for it. The module is several megabytes because
 * threshold ECDSA carries Paillier and a pile of zero-knowledge proofs, and
 * making every visitor pay for that before they have decided to look would be
 * charging them for a demonstration they did not ask for.
 *
 * The size is a browser problem and not a product one: the same Go package
 * compiles for android/arm64 and ios/arm64, where it is part of an app binary
 * and downloaded once from a store.
 */
export function loadProtocol(url = './mpc.wasm', onProgress = () => {}) {
  if (wasmReady) return wasmReady;
  wasmReady = (async () => {
    if (typeof Go === 'undefined') {
      throw new Error('the Go runtime shim did not load');
    }
    onProgress('fetching');
    const response = await fetch(url);
    if (!response.ok) throw new Error(`the protocol module is not deployed (${response.status})`);
    const bytes = await response.arrayBuffer();
    onProgress('instantiating');
    const go = new Go();
    const result = await WebAssembly.instantiate(bytes, go.importObject);
    go.run(result.instance);
    // go.run resolves only when main returns, and main blocks forever on
    // purpose, so the exports are polled for instead of awaited.
    for (let i = 0; i < 200 && typeof globalThis.yamaleMPC === 'undefined'; i += 1) {
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
    if (typeof globalThis.yamaleMPC === 'undefined') {
      throw new Error('the protocol module loaded but exported nothing');
    }
    onProgress('ready');
    return globalThis.yamaleMPC;
  })();
  return wasmReady;
}

/** Turns {error} into a thrown error, so callers stop checking two things. */
function unwrap(result, what) {
  if (!result || typeof result !== 'object') throw new Error(`${what}: no result`);
  if (result.error) throw new Error(result.error);
  return result;
}

/**
 * Run one signature between two parties, recording every message.
 *
 * The transcript is the point. A signature that simply appears is a black box
 * and asks to be trusted; a signature you watched being assembled out of
 * messages neither party could have produced alone is an argument.
 *
 * Both parties run here, which production never does. See the note at the top
 * of this file — that is why the shares are throwaway.
 */
export async function demonstrate(mpc, digestB64, deviceShare, custodianShare, onRound = () => {}) {
  const signers = 'custodian,device';
  const sessions = {};
  const transcript = [];

  // deviceShare and custodianShare are the share files' RAW TEXT. See the note
  // in main.js: parsing them in JavaScript silently destroys every big integer.
  const device = unwrap(mpc.startSign(digestB64, deviceShare, signers), 'device');
  sessions.device = device.session;
  const custodian = unwrap(mpc.startSign(digestB64, custodianShare, signers), 'custodian');
  sessions.custodian = custodian.session;

  let pending = [
    ...JSON.parse(device.outbound || '[]'),
    ...JSON.parse(custodian.outbound || '[]'),
  ];

  let round = 0;
  const deadline = Date.now() + 120000;
  for (;;) {
    const done = {};
    for (const role of ['device', 'custodian']) {
      const state = unwrap(mpc.signature(sessions[role]), role);
      done[role] = !state.pending;
    }
    if (done.device && done.custodian) break;
    if (Date.now() > deadline) throw new Error('the two parties never finished');

    if (pending.length === 0) {
      // Poll both parties for anything they have produced since we last looked.
      //
      // Under WebAssembly a Go goroutine only runs when JavaScript yields, so
      // at the moment startSign returns neither party has done any work and
      // neither has a message to give. A loop that waited to be handed the
      // first one would wait forever — which is exactly what the first browser
      // run did, for two minutes, with an empty transcript.
      await new Promise((resolve) => setTimeout(resolve, 15));
      for (const role of ['device', 'custodian']) {
        const more = unwrap(mpc.outbound(sessions[role]), role);
        pending.push(...JSON.parse(more.outbound || '[]'));
      }
      continue;
    }

    round += 1;
    const carrying = pending;
    pending = [];
    for (const message of carrying) {
      const to = message.from === 'device' ? 'custodian' : 'device';
      if (!message.broadcast && !(message.to || []).includes(to)) continue;
      transcript.push({
        round,
        from: message.from,
        to,
        broadcast: !!message.broadcast,
        bytes: (message.wire || '').length,
      });
      onRound(transcript[transcript.length - 1]);
      const reply = unwrap(mpc.handle(sessions[to], JSON.stringify(message)), to);
      pending.push(...JSON.parse(reply.outbound || '[]'));
    }
  }

  const deviceSig = unwrap(mpc.signature(sessions.device), 'device').signature;
  const custodianSig = unwrap(mpc.signature(sessions.custodian), 'custodian').signature;
  for (const role of ['device', 'custodian']) mpc.close(sessions[role]);

  return {
    signature: deviceSig,
    // Both parties compute identical bytes. That is what lets a device check
    // what it is about to broadcast rather than trusting the custodian to hand
    // back the signature it helped make.
    agreed: deviceSig === custodianSig,
    transcript,
  };
}

/**
 * Ask one share, alone, to sign — and report the refusal.
 *
 * The refusal is the product. It is run rather than described because a
 * sentence claiming the operator cannot sign is exactly as convincing as every
 * other such sentence, and this one can be checked.
 */
export function refuseWithOneShare(mpc, digestB64, shareText) {
  // The role is read off the text with a narrow match rather than by parsing,
  // for the same reason: JSON.parse would ruin the share on the way past.
  const role = (/"role"\s*:\s*"([a-z]+)"/.exec(shareText) || [])[1] || 'device';
  const result = mpc.startSign(digestB64, shareText, role);
  if (result && result.error) return result.error;
  if (result && result.session) mpc.close(result.session);
  return null;
}

/** A digest of arbitrary text, which is what the protocol signs. */
export async function digestOf(text) {
  const bytes = new TextEncoder().encode(text);
  const hash = await crypto.subtle.digest('SHA-256', bytes);
  let binary = '';
  for (const byte of new Uint8Array(hash)) binary += String.fromCharCode(byte);
  return btoa(binary);
}
