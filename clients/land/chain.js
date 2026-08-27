// Reading the register, and sending what this page is allowed to send.
//
// ===========================================================================
// WHY THE READS MOVED, MEASURED RATHER THAN ASSUMED
// ===========================================================================
//
// Every read on this page used to go to `/api/rest/yamale/blockchain/land/v1/…`.
// On the live deployment, on 2026-08-27:
//
//   $ curl -si https://pay.yamalelegal.com/api/rest/yamale/blockchain/land/v1/params
//     HTTP/1.1 401 Unauthorized
//     WWW-Authenticate: Basic realm="Yamale — supervisor access"
//
// Not one query answered. The proxy in front of the node allowlists REST paths
// per module and denies by default, and x/land's prefix is not on that list —
// so the register whose entire premise is that reading it is public presented a
// browser login box with no credentials to type into it, on every screen.
//
// The same queries answer unauthenticated over the node's ABCI interface,
// which the proxy does publish:
//
//   $ curl -s '…/api/rpc/abci_query?path="/blockchain.land.v1.Query/Params"&data=0x'
//     {"result":{"response":{"code":0,"value":"CgYIAxCA6kk=","height":"94464"}}}
//
// That is the same discovery clients/app/src/standing.ts records for x/paymsg,
// arrived at the same way — by pointing the client at the running chain rather
// than by reading the nginx config. So everything below speaks ABCI, and the
// REST surface is used for exactly one thing that ABCI cannot do: transaction
// search, which is discovery rather than fact, and which degrades to an honest
// blank when it is gated.
//
// A consequence worth stating: every answer carries the **height it was read
// at**. "There is no objection against this title" is a claim about a moment,
// and a page that shows it without the moment is asking to be believed about a
// fact it cannot date.

import {
  ACCOUNT_QUERY,
  QUERY,
  accountRequest,
  authInfo,
  decodeAccount,
  fromBase64,
  read,
  toBase64,
  toHex,
  txBody,
  txRaw,
  write,
} from './proto.js';
import { CHAIN, GAS } from './registrar.js';

const RPC = '/api/rpc';
const REST = '/api/rest';

/** Long enough for a one-bar connection, short enough to fail rather than hang. */
const TIMEOUT_MS = 12000;

/**
 * A call that did not answer, named.
 *
 * "Something went wrong" tells a clerk nothing, and tells somebody standing in
 * front of a seller even less: they need to know whether the register said "no
 * objection" or whether the register did not answer, because those are opposite
 * facts about whether it is safe to pay.
 */
export class CallFailed extends Error {
  constructor(what, status, detail) {
    super(detail ? `${what} → ${status}: ${detail}` : `${what} → ${status}`);
    this.what = what;
    this.status = status;
  }
}

/* ========================================================================= */
/*  ABCI                                                                     */
/* ========================================================================= */

/**
 * One ABCI query, returning the response bytes and the height they are true at.
 *
 * `data` is the hex-encoded protobuf request. CometBFT's URI layer takes a
 * `0x…` string for a byte parameter, which avoids the quoting the base64 form
 * needs — and quoting inside a query string is where this kind of call usually
 * goes wrong.
 */
export async function abci(path, requestBytes = new Uint8Array()) {
  const url = `${RPC}/abci_query?path=${encodeURIComponent(`"${path}"`)}`
    + `&data=0x${toHex(requestBytes)}`;

  let res;
  try {
    res = await fetch(url, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  } catch (e) {
    throw new CallFailed(path, 'unreachable', e.message);
  }
  if (!res.ok) throw new CallFailed(path, res.status, '');

  const json = await res.json();
  if (json.error) throw new CallFailed(path, 'rpc error', json.error.message ?? '');

  const response = json.result?.response;
  if (!response) throw new CallFailed(path, 'malformed', 'no response body');

  // A non-zero ABCI code is the node refusing the query — an unknown path, a
  // pruned height, a key that does not exist. It is not an empty result, and
  // conflating the two is how "no such parcel" comes to look like "the parcel
  // has no encumbrances".
  if (response.code) {
    throw new CallFailed(path, `abci ${response.code}`, response.log || '');
  }
  return {
    bytes: fromBase64(response.value ?? ''),
    height: Number(response.height ?? '0'),
  };
}

/** The height the last answer was true at, for the line under the verdict. */
export let lastHeight = 0;

/**
 * One of x/land's queries, decoded.
 *
 * The keeper's "not found" comes back as a non-zero ABCI code, so the two
 * shapes are separated here once: `landQuery` throws for a call that did not
 * complete, and `landOrNull` turns a genuine "no such thing" into null. Every
 * screen depends on telling those apart.
 */
export async function landQuery(name, arg) {
  const q = QUERY[name];
  if (!q) throw new Error(`no such land query: ${name}`);
  const { bytes, height } = await abci(q.path, q.request(arg));
  if (height > lastHeight) lastHeight = height;
  return { ...q.response(read(bytes)), height };
}

/** Returns null when the register answered "there is no such thing". */
export async function landOrNull(name, arg) {
  try {
    return await landQuery(name, arg);
  } catch (e) {
    // The keeper returns collections.ErrNotFound, which the gRPC layer maps to
    // a NotFound status and ABCI reports as a non-zero code with the reason in
    // the log. Matched on the log rather than on the number, because the number
    // is the SDK's and moves between versions.
    const text = String(e.message ?? '').toLowerCase();
    if (text.includes('not found') || text.includes('no such') || text.includes('key not found')) {
      return null;
    }
    throw e;
  }
}

/* ------------------------------------------------------------ the chain head */

/**
 * Where the chain is, read from /status.
 *
 * `catchingUp` is surfaced rather than hidden. A node replaying history answers
 * every query correctly for a height that is hours old, and a register that
 * says "no objection" from four hours ago is worse than one that says nothing.
 */
export async function head() {
  const res = await fetch(`${RPC}/status`, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  if (!res.ok) throw new CallFailed('/status', res.status, '');
  const info = (await res.json())?.result?.sync_info;
  if (!info) throw new CallFailed('/status', 'malformed', 'no sync_info');
  return {
    height: Number(info.latest_block_height ?? '0'),
    at: new Date(String(info.latest_block_time ?? '')),
    catchingUp: Boolean(info.catching_up),
  };
}

/**
 * The time a block was written, from the RPC rather than from an average.
 *
 * An estimated date on a land record is a date somebody will one day cite in
 * court. Where the node no longer serves a block this returns null and the
 * caller shows the height instead of a guess.
 *
 * Held to three at a time: a parcel with a long history asks for a dozen blocks
 * at once, and firing them all is fine on a desk and miserable on the
 * connection this page is actually opened on, where the browser's own limit
 * turns a burst into a queue with nothing rendering behind it.
 */
const blockTimes = new Map();
let inFlight = 0;
const waiting = [];
const slot = () => (inFlight < 3
  ? (inFlight += 1, Promise.resolve())
  : new Promise((r) => waiting.push(r)));
const release = () => { const next = waiting.shift(); if (next) next(); else inFlight -= 1; };

export function blockTime(height) {
  const h = String(height);
  if (h === '0') return Promise.resolve(null);
  if (blockTimes.has(h)) return blockTimes.get(h);
  const p = (async () => {
    await slot();
    try {
      const res = await fetch(`${RPC}/block?height=${encodeURIComponent(h)}`,
        { signal: AbortSignal.timeout(TIMEOUT_MS) });
      if (!res.ok) return null;
      return (await res.json())?.result?.block?.header?.time ?? null;
    } catch {
      return null;   // pruned, or the node will not serve it
    } finally {
      release();
    }
  })();
  blockTimes.set(h, p);
  return p;
}

/* ------------------------------------------------------------- who is who */

/**
 * A user ID for an account, from x/alias.
 *
 * Over ABCI for the same reason as everything else. Failure is silent by
 * design: a missing user ID means the page shows an account reference, which is
 * a worse label but not a wrong one, and a register that refuses to draw a
 * title because a name lookup timed out has failed at its actual job.
 */
export async function aliasOf(address) {
  try {
    const { bytes } = await abci('/blockchain.alias.v1.Query/AliasOf',
      write((w) => w.string(1, address)));
    return aliasField(bytes, 1);   // Alias.id
  } catch {
    return null;
  }
}

/** The account a user ID resolves to, for the search box. Null if none does. */
export async function addressOfAlias(id) {
  try {
    const { bytes } = await abci('/blockchain.alias.v1.Query/Alias',
      write((w) => w.string(1, id)));
    return aliasField(bytes, 2);   // Alias.address
  } catch {
    return null;
  }
}

/** QueryAlias*Response { Alias alias = 1 }; Alias { id = 1, address = 2 }. */
function aliasField(bytes, num) {
  const alias = read(bytes).get(1)?.[0];
  if (!alias) return null;
  const value = read(alias).get(num)?.[0];
  return value ? new TextDecoder().decode(value) || null : null;
}

/* ---------------------------------------------------------- governance */

/**
 * x/gov's own module account, read from the chain.
 *
 * The only `authority` MsgRegisterAuthority is accepted from. Read rather than
 * hard-coded because the value is derived from the module name and the address
 * prefix, and a proposal naming the wrong one is the worst failure available
 * here: it passes its vote — a vote costs a deposit and days — and then fails
 * to execute, with the office still not admitted and nobody at fault.
 *
 * Measured on 2026-08-27: ModuleAccountByName("gov") answers
 * yml10d07y265gmmuvt4z0w9aw880jnsr700jz5s386 at account number 10.
 */
export async function govAuthority() {
  const { bytes } = await abci('/cosmos.auth.v1beta1.Query/ModuleAccountByName',
    write((w) => w.string(1, 'gov')));
  // QueryModuleAccountByNameResponse { Any account = 1 };
  // ModuleAccount { BaseAccount base_account = 1, string name = 2, … }
  const anyBytes = read(bytes).get(1)?.[0];
  if (!anyBytes) throw new CallFailed('the gov module account', 'empty', '');
  const module = read(read(anyBytes).get(2)?.[0] ?? new Uint8Array());
  const base = read(module.get(1)?.[0] ?? new Uint8Array());
  const address = base.get(1)?.[0];
  if (!address) throw new CallFailed('the gov module account', 'malformed', '');
  return new TextDecoder().decode(address);
}

/**
 * What a governance proposal must be deposited with, as the chain has it now.
 *
 * Returned as a coin string ready for the proposal document. A deposit typed
 * from memory is a proposal that sits below the threshold and never enters
 * voting, which looks from outside exactly like a proposal nobody supported.
 */
export async function govMinDeposit() {
  const { bytes } = await abci('/cosmos.gov.v1.Query/Params', write((w) => w.string(1, 'deposit')));
  // QueryParamsResponse { …, Params params = 4 }; Params { repeated Coin min_deposit = 1, … }
  const params = read(read(bytes).get(4)?.[0] ?? new Uint8Array());
  const coin = read(params.get(1)?.[0] ?? new Uint8Array());
  const denom = coin.get(1)?.[0];
  const amount = coin.get(2)?.[0];
  if (!denom || !amount) throw new CallFailed('the governance deposit', 'malformed', '');
  const d = new TextDecoder();
  return `${d.decode(amount)}${d.decode(denom)}`;
}

/* --------------------------------------------------------- transaction log */

/**
 * Transaction search, for the two things the register's state cannot answer:
 * which titles were registered recently, and which parcels have ever had
 * fractionalisation authorised.
 *
 * A different *kind* of source from a state query — the record of what was
 * sent, not of what is held — so every panel fed by it says so and degrades to
 * an honest blank rather than to an empty-looking success. It goes over REST
 * because tx search is not an ABCI query; on this deployment that means it sits
 * behind the operator's gate and answers 401, which is a legitimate
 * configuration and is reported as one.
 */
export async function txsOf(msgType, limit = 30) {
  const url = `${REST}/cosmos/tx/v1beta1/txs`
    + `?query=${encodeURIComponent(`message.action='${msgType}'`)}`
    + `&limit=${limit}&order_by=ORDER_BY_DESC`;
  const res = await fetch(url, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  if (!res.ok) throw new CallFailed('transaction search', res.status, '');
  const data = await res.json();
  const out = [];
  (data.tx_responses || []).forEach((r, i) => {
    const body = (data.txs || [])[i]?.body;
    (body?.messages || []).forEach((m) => {
      if (m['@type'] === msgType) {
        out.push({ msg: m, height: r.height, code: r.code, hash: r.txhash });
      }
    });
  });
  return out;
}

/* ========================================================================= */
/*  The wallet                                                               */
/* ========================================================================= */

/**
 * The Yamale wallet, spoken to over postMessage.
 *
 * This is the client half of `clients/connect/src/protocol.ts`, reimplemented
 * in plain JavaScript. Not copied out of laziness — that package is TypeScript
 * with a CosmJS dependency, and this console is deliberately one file with no
 * build step, because it gets opened when something is wrong, from a phone, on
 * one bar of signal. A page that needs `npm install` before it can say "there
 * is an objection against this title" is a page that will not be open at the
 * moment it matters.
 *
 * The two rules from that file hold here exactly:
 *
 *   1. Every message is origin-checked. The wallet answers only the origin that
 *      opened it; this end ignores anything that did not come from the wallet's
 *      origin. Skipping either check turns this into a signing oracle for
 *      whatever page is open in another tab.
 *
 *   2. Binary crosses as base64, never as a typed array.
 *
 * A popup, not an iframe. A page cannot be trusted to render the approval
 * dialog for a document it also composed — the person must see the wallet's own
 * window with the wallet's own address bar, or "approve" means nothing.
 */
export const WALLET_URL = `${window.location.origin}/wallet`;

export class WalletRefused extends Error {
  constructor(code, message) { super(message); this.code = code; }
  get declined() { return this.code === 'rejected'; }
}

function walletRequest(payload, { timeoutMs = 180000 } = {}) {
  return new Promise((resolve, reject) => {
    const id = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    const origin = new URL(WALLET_URL).origin;
    const popup = window.open(`${WALLET_URL}/connect`, 'yamale-wallet',
      'width=420,height=680,resizable=yes,scrollbars=yes');
    if (!popup) {
      reject(new WalletRefused('internal',
        'The wallet window was blocked. Allow pop-ups for this site and try again.'));
      return;
    }

    let settled = false;
    const finish = (fn) => {
      if (settled) return;
      settled = true;
      window.removeEventListener('message', onMessage);
      clearInterval(closedCheck);
      clearTimeout(timer);
      fn();
    };

    const onMessage = (event) => {
      if (event.origin !== origin) return;
      // And from the window we opened, not merely from a window at that origin.
      // `event.source` is stamped by the browser and cannot be forged. Without
      // it, any frame or tab on the wallet's origin can answer a request this
      // page made — and, found by driving this in a browser, a message this
      // page posts to the popup is also delivered back to this listener when
      // the two share an origin, so the page can answer itself with the echo of
      // its own request and read a request as a result.
      if (event.source !== popup) return;
      const data = event.data;
      if (!data || data.channel !== 'yamale.connect' || typeof data.id !== 'string') return;
      // The wallet announces itself when its listener is attached. Sending
      // before that would post into a window with nothing listening, and the
      // message would be lost with no error at either end.
      if (data.payload?.kind === 'ready') {
        popup.postMessage({ channel: 'yamale.connect', version: 1, id, payload }, origin);
        return;
      }
      if (data.id !== id) return;
      if (data.version !== 1) {
        finish(() => reject(new WalletRefused('unsupported',
          'The wallet speaks a different version of this protocol.')));
        return;
      }
      const result = data.payload;
      // Belt to the braces above: a reply must be a *result*, and the result
      // asked for. The source check is what stops an echo; this stops a reply
      // to a different question arriving under a matching id, which would
      // resolve the caller with an object whose fields it never checks.
      if (!result || !['connect', 'accounts', 'signDirect', 'error'].includes(result.kind)) return;
      if (result.kind !== 'error' && result.kind !== payload.kind) return;
      finish(() => {
        popup.close();
        if (result.kind === 'error') reject(new WalletRefused(result.code, result.message));
        else resolve(result);
      });
    };

    window.addEventListener('message', onMessage);

    // A closed window is a refusal. Without this the promise hangs and the page
    // sits on a spinner with nothing to cancel it.
    const closedCheck = setInterval(() => {
      if (popup.closed) {
        finish(() => reject(new WalletRefused('rejected', 'You closed the wallet window.')));
      }
    }, 400);

    const timer = setTimeout(() => finish(() => {
      popup.close();
      reject(new WalletRefused('rejected', 'The wallet did not answer in time.'));
    }), timeoutMs);
  });
}

/** Ask the wallet who is signing. Opens the wallet's own window. */
export async function connectWallet() {
  const result = await walletRequest({
    kind: 'connect', chainId: CHAIN.id, appName: 'Land title register',
  });
  if (result.kind !== 'connect') throw new WalletRefused('internal', 'Unexpected reply.');
  const account = result.accounts?.[0];
  if (!account) throw new WalletRefused('locked', 'The wallet holds no account for this chain.');
  return account;   // { address, pubkey (base64), algo }
}

/* ========================================================================= */
/*  Sending                                                                  */
/* ========================================================================= */

/** What the chain knows about an account, or null if it has never seen it. */
export async function accountOf(address) {
  try {
    const { bytes } = await abci(ACCOUNT_QUERY, accountRequest(address));
    return decodeAccount(bytes);
  } catch (e) {
    const text = String(e.message ?? '').toLowerCase();
    if (text.includes('not found')) return null;
    throw e;
  }
}

/**
 * Sign one or more messages with the connected account and put them in a block.
 *
 * The three steps are separated in what is returned, because they fail
 * differently and a person needs to know which one they are looking at:
 *
 *   `signed`     the wallet produced a signature, or the person declined
 *   `accepted`   the node put the transaction in its mempool (CheckTx)
 *   `delivered`  the transaction ran in a block, with the code it ran with
 *
 * The last distinction has cost this project four bugs. A `code: 0` reply to a
 * broadcast means the mempool accepted it, not that it worked; a message can be
 * accepted and then refused in the block for exactly the reason the person
 * needed to know. So this waits for inclusion and reports the **delivered**
 * code, never the broadcast one.
 */
export async function signAndSend(account, messageAnys, memo = '') {
  const onChain = await accountOf(account.address);
  if (!onChain) {
    throw new CallFailed('the account', 'unknown',
      'the chain has never seen this account, so it cannot sign anything yet');
  }

  const body = txBody(messageAnys, memo);
  const auth = authInfo({
    pubkey: fromBase64(account.pubkey),
    sequence: onChain.sequence,
    gasLimit: GAS,
    // No fee. See proto.js: the person best placed to object to a fraudulent
    // sale is the family member who holds no tokens, and this chain's nodes run
    // minimum-gas-prices = "0uyml".
  });

  const result = await walletRequest({
    kind: 'signDirect',
    chainId: CHAIN.id,
    signerAddress: account.address,
    bodyBytes: toBase64(body),
    authInfoBytes: toBase64(auth),
    accountNumber: onChain.account_number,
  });
  if (result.kind !== 'signDirect') throw new WalletRefused('internal', 'Unexpected reply.');

  // Broadcast what was signed, not what was sent. The wallet is permitted to
  // have changed the document — a fee the person edited — and signing over one
  // set of bytes while broadcasting another fails verification in a way that
  // reads as a chain fault.
  const raw = txRaw(fromBase64(result.bodyBytes), fromBase64(result.authInfoBytes),
    [fromBase64(result.signature)]);

  return broadcast(raw);
}

/** Put a signed transaction in the mempool, then wait for the block it runs in. */
export async function broadcast(txBytes) {
  const res = await fetch(`${RPC}/broadcast_tx_sync?tx=0x${toHex(txBytes)}`,
    { signal: AbortSignal.timeout(TIMEOUT_MS) });
  if (!res.ok) throw new CallFailed('broadcast', res.status, '');
  const json = await res.json();
  if (json.error) throw new CallFailed('broadcast', 'rpc error', json.error.data ?? json.error.message);

  const check = json.result;
  if (!check) throw new CallFailed('broadcast', 'malformed', 'no result');
  if (check.code) {
    // Rejected before it ever reached a block. Nothing was recorded, and saying
    // so matters: "your objection was refused" and "your objection is pending"
    // are opposite facts for somebody watching a sale go through.
    return {
      accepted: false, delivered: false, hash: check.hash,
      codespace: check.codespace, code: check.code, log: check.log ?? '',
    };
  }
  return awaitDelivery(check.hash);
}

/**
 * Wait for a transaction to run in a block, and report what it did.
 *
 * Polled rather than subscribed: a websocket through the proxy is one more
 * thing to be blocked, and this page's whole design premise is that it works on
 * a connection that barely does.
 */
export async function awaitDelivery(hash, { attempts = 20, everyMs = 1500 } = {}) {
  for (let i = 0; i < attempts; i += 1) {
    await new Promise((r) => setTimeout(r, everyMs));
    let json;
    try {
      const res = await fetch(`${RPC}/tx?hash=0x${hash}`,
        { signal: AbortSignal.timeout(TIMEOUT_MS) });
      json = await res.json();
    } catch {
      continue;   // the block has not been indexed yet, or the connection blinked
    }
    const result = json?.result;
    if (!result) continue;
    const r = result.tx_result ?? {};
    return {
      accepted: true,
      delivered: true,
      hash,
      height: Number(result.height ?? '0'),
      code: r.code ?? 0,
      codespace: r.codespace ?? '',
      log: r.log ?? '',
      // The message response bytes, so a caller can read the id the chain
      // assigned — the transfer number a seller then has to give their buyer.
      data: r.data ?? '',
    };
  }
  // Not a failure. The transaction is signed, broadcast and accepted; the page
  // simply stopped watching. Saying "it failed" here would be a lie that leads
  // somebody to send it a second time.
  return { accepted: true, delivered: false, hash };
}

/**
 * The id x/land assigned, read out of the transaction's response bytes.
 *
 * TxMsgData { repeated MsgResponse responses = 2 }, MsgResponse { type_url = 1,
 * value = 2 }, and the value is the message's own response — MsgObjectResponse
 * is empty, MsgProposeTransferResponse carries `transfer_id = 1`. Returned as a
 * string, because transfer 0 is real and a Number 0 would be falsy.
 *
 * The input is base64: CometBFT marshals `tx_result.data` as base64 in its JSON
 * like every other byte field, not as the hex the `/tx?hash=` parameter takes.
 * Mixing those up yields a decode failure that looks like the chain returning
 * nothing, which on this page reads as "the transfer has no number".
 */
export function idFromResponse(dataBase64) {
  if (!dataBase64) return null;
  try {
    const responses = read(fromBase64(dataBase64)).get(2) ?? [];
    for (const entry of responses) {
      const f = read(entry);
      const value = f.get(2)?.[0];
      if (!value || value.length === 0) continue;
      const inner = read(value);
      if (inner.has(1)) return String(inner.get(1)[0]);
    }
    // Every response field was absent, which for a uint64 means zero — and
    // zero is a real transfer id.
    return responses.length ? '0' : null;
  } catch {
    return null;
  }
}
