// Protobuf, by hand, because this console has no build step.
//
// WHY THIS FILE EXISTS, and why it is a third copy of the same primitives:
//
//   $ curl -si https://pay.yamalelegal.com/api/rest/yamale/blockchain/land/v1/params
//     HTTP/1.1 401 Unauthorized
//     WWW-Authenticate: Basic realm="Yamale — supervisor access"
//
// The deployment's proxy allowlists REST paths per module and denies by
// default. That was measured against x/land on 2026-08-27 and the finding is
// not about x/land: a NEW module's prefix is not on that allowlist either, and
// being new is exactly why. x/syndicate is the newest module on the chain, so a
// console reading it over REST would present a browser login box on every
// screen, with no credentials to type into it.
//
// The same queries answer unauthenticated over the node's ABCI interface, which
// the proxy does publish. ABCI speaks protobuf and nothing else.
//
// ON THE DUPLICATION. The reader and writer below are close to the ones in
// clients/land/proto.js and clients/ladoum/proto.js, and copying them was the
// choice rather than the accident. Each console is a self-contained tree
// deployed to its own path with no build step and no shared runtime, so a
// common module would have to be a fourth deployed artefact that all three
// fetch — one more thing whose absence takes every page down, which is exactly
// the failure the Ladoum console shipped with. What is NOT copied is anything
// below the primitives: every codec here is transcribed from
// proto/blockchain/syndicate/v1/*.proto, names the field numbers it was written
// from, and is round-tripped in proto.test.js, so drift shows up as a failing
// test rather than as a book that renders the minimum in the target's place.

/* ------------------------------------------------------------------ writing */

/** A growable byte sink. Plain arrays: these messages are hundreds of bytes. */
class Writer {
  constructor() { this.bytes = []; }

  /** Base-128 varint, little-endian groups of seven bits. */
  varint(value) {
    let v = BigInt(value);
    if (v < 0n) v += 1n << 64n;   // int64 negatives are ten-byte varints
    do {
      const byte = Number(v & 0x7fn);
      v >>= 7n;
      this.bytes.push(v > 0n ? byte | 0x80 : byte);
    } while (v > 0n);
    return this;
  }

  tag(field, wire) { return this.varint((field << 3) | wire); }

  raw(bytes) { for (const b of bytes) this.bytes.push(b); return this; }

  /** Length-delimited: a varint length then the payload. */
  bytesField(field, payload) {
    if (!payload || payload.length === 0) return this;   // proto3 omits defaults
    this.tag(field, 2).varint(payload.length).raw(payload);
    return this;
  }

  string(field, value) {
    if (value === undefined || value === null || value === '') return this;
    return this.bytesField(field, new TextEncoder().encode(String(value)));
  }

  /** uint32, uint64, int64 and enums all encode as a varint. */
  num(field, value) {
    if (value === undefined || value === null) return this;
    const v = BigInt(value);
    if (v === 0n) return this;                            // proto3 omits defaults
    return this.tag(field, 0).varint(v);
  }

  bool(field, value) {
    if (!value) return this;                              // proto3 omits defaults
    return this.tag(field, 0).varint(1n);
  }

  /** A nested message, encoded by `fn` into its own writer. */
  msg(field, fn) {
    const inner = new Writer();
    fn(inner);
    return this.bytesField(field, inner.finish());
  }

  finish() { return Uint8Array.from(this.bytes); }
}

export const write = (fn) => { const w = new Writer(); fn(w); return w.finish(); };

/* ------------------------------------------------------------------ reading */

/**
 * Every field in a message, keyed by field number.
 *
 * A map of arrays rather than an object, because repeated fields are the point:
 * a deal's book arrives as forty separate field-1 entries and a reader that
 * kept only the last one would show a syndicate of one. That is the failure
 * mode this shape rules out.
 *
 * Unknown fields are kept rather than refused. A node running newer code than
 * this page emits fields this file has never heard of, and the correct
 * behaviour is to ignore them, not to fail to parse the deal around them.
 */
export function read(bytes) {
  const fields = new Map();
  const put = (num, value) => {
    const list = fields.get(num);
    if (list) list.push(value); else fields.set(num, [value]);
  };

  let i = 0;
  const varint = () => {
    let result = 0n;
    let shift = 0n;
    for (;;) {
      if (i >= bytes.length) throw new Error('truncated varint');
      const byte = bytes[i++];
      result |= BigInt(byte & 0x7f) << shift;
      if ((byte & 0x80) === 0) return result;
      shift += 7n;
      if (shift > 70n) throw new Error('varint too long');
    }
  };

  while (i < bytes.length) {
    const key = Number(varint());
    const field = key >>> 3;
    switch (key & 7) {
      case 0: put(field, varint()); break;
      case 2: {
        const len = Number(varint());
        if (i + len > bytes.length) throw new Error('truncated length-delimited field');
        put(field, bytes.subarray(i, i + len));
        i += len;
        break;
      }
      case 5: i += 4; break;   // fixed32 — none in x/syndicate
      case 1: i += 8; break;   // fixed64 — likewise
      default: throw new Error(`unsupported wire type ${key & 7}`);
    }
  }
  return fields;
}

const first = (f, num) => f.get(num)?.[0];
const all = (f, num) => f.get(num) ?? [];

const decoder = new TextDecoder();

/** proto3 semantics: an absent field is its default, never undefined. */
export const str = (f, num) => {
  const v = first(f, num);
  return v === undefined ? '' : decoder.decode(v);
};

/**
 * A 64-bit integer as a decimal string.
 *
 * A string and not a Number, and in this module that is not a precaution — it
 * is a correctness requirement. Every monetary field here is a `cosmos.Int`,
 * which is ARBITRARY precision and arrives as a decimal string on the wire; the
 * distribution index is carried at 1e18, so a single index value exceeds
 * Number.MAX_SAFE_INTEGER by eight orders of magnitude. Anything that touches
 * those goes through BigInt, never a double.
 */
export const u64 = (f, num) => String(first(f, num) ?? 0n);

/** uint32 and enums, where a Number is the natural type and the range is safe. */
export const u32 = (f, num) => Number(first(f, num) ?? 0n);

export const bool = (f, num) => Boolean(first(f, num) ?? 0n);

/** A nested message, decoded by `fn`, or `null` when the field is absent. */
export const sub = (f, num, fn) => {
  const v = first(f, num);
  return v === undefined ? null : fn(read(v));
};

/** Every entry of a repeated message field, in wire order. */
export const subs = (f, num, fn) => all(f, num).map((v) => fn(read(v)));

/* -------------------------------------------------------------------- base64 */

export function toBase64(bytes) {
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

export function fromBase64(value) {
  const binary = atob(value ?? '');
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) out[i] = binary.charCodeAt(i);
  return out;
}

export const toHex = (bytes) =>
  [...bytes].map((b) => b.toString(16).padStart(2, '0')).join('');

/* ============================================================================
   x/syndicate — proto/blockchain/syndicate/v1/syndicate.proto, params.proto,
   query.proto.

   A `cosmos.Int` is a STRING on the wire, not a varint. Every amount below is
   therefore read with `str()` and handed to the page as a decimal string, and
   the page does its arithmetic in BigInt. Reading one of these as a number is
   the single most likely way this file could be wrong while still parsing, so
   each amount field is named in the comment above its decoder.
   ============================================================================ */

/** DealState — syndicate.proto. */
export const DEAL_STATE = ['DEAL_STATE_UNSPECIFIED', 'DEAL_STATE_OPEN',
  'DEAL_STATE_CLOSED', 'DEAL_STATE_ABORTED', 'DEAL_STATE_REALISED'];

/** ListingState — syndicate.proto. */
export const LISTING_STATE = ['LISTING_STATE_UNSPECIFIED', 'LISTING_STATE_LIVE',
  'LISTING_STATE_FILLED', 'LISTING_STATE_CANCELLED'];

/** Params: max_entry_fee_bps=1, max_management_fee_bps=2, max_open_period=3,
 *  min_open_period=4. All four are plain integers, not Ints. */
export const decodeParams = (f) => ({
  max_entry_fee_bps: u32(f, 1),
  max_management_fee_bps: u32(f, 2),
  max_open_period: u64(f, 3),
  min_open_period: u64(f, 4),
});

/** FeeSchedule: sourcing_bps=1, syndication_bps=2, management_bps=3. */
const decodeFees = (f) => ({
  sourcing_bps: u32(f, 1),
  syndication_bps: u32(f, 2),
  management_bps: u32(f, 3),
});

/** cosmos.base.v1beta1.Coin: denom=1, amount=2. The amount is an Int, so a
 *  string — a distribution of ten million USDC does not fit in a double. */
const decodeCoin = (f) => ({ denom: str(f, 1), amount: str(f, 2) || '0' });

/**
 * Deal: id=1, reference=2, company=3, partner=4, drawdown_account=5,
 * operator=6, settlement_denom=7, target=8, minimum=9, minimum_ticket=10,
 * fees=11, closes_at=12, state=13, raised=14, denom=15, supply=16,
 * distribution_index=17, distributed=18, opened_at=19, closed_at=20.
 *
 * Ints, all read as strings: target, minimum, minimum_ticket, raised, supply,
 * distribution_index, distributed. `distribution_index` is carried at 1e18 and
 * is the one that would silently lose precision as a Number.
 *
 * `denom` is empty until the deal closes — a deal that has not closed has
 * issued no tokens — and the page uses that emptiness rather than the state
 * enum in one place, because a token that does not exist cannot be quoted.
 */
export const decodeDeal = (f) => ({
  id: u64(f, 1),
  reference: str(f, 2),
  company: str(f, 3),
  partner: str(f, 4),
  drawdown_account: str(f, 5),
  operator: str(f, 6),
  settlement_denom: str(f, 7),
  target: str(f, 8) || '0',
  minimum: str(f, 9) || '0',
  minimum_ticket: str(f, 10) || '0',
  fees: sub(f, 11, decodeFees) ?? { sourcing_bps: 0, syndication_bps: 0, management_bps: 0 },
  closes_at: u64(f, 12),
  state: DEAL_STATE[u32(f, 13)] ?? 'DEAL_STATE_UNSPECIFIED',
  raised: str(f, 14) || '0',
  denom: str(f, 15),
  supply: str(f, 16) || '0',
  distribution_index: str(f, 17) || '0',
  distributed: str(f, 18) || '0',
  opened_at: u64(f, 19),
  closed_at: u64(f, 20),
});

/** Commitment: deal_id=1, investor=2, amount=3 (Int), committed_at=4. */
export const decodeCommitment = (f) => ({
  deal_id: u64(f, 1),
  investor: str(f, 2),
  amount: str(f, 3) || '0',
  committed_at: u64(f, 4),
});

/** Investor: address=1, onboarded_by=2, reference=3, jurisdiction=4, active=5,
 *  onboarded_at=6. */
export const decodeInvestor = (f) => ({
  address: str(f, 1),
  onboarded_by: str(f, 2),
  reference: str(f, 3),
  jurisdiction: str(f, 4),
  active: bool(f, 5),
  onboarded_at: u64(f, 6),
});

/** Operator: address=1, name=2, jurisdiction=3, active=4, admitted_at=5. */
export const decodeOperator = (f) => ({
  address: str(f, 1),
  name: str(f, 2),
  jurisdiction: str(f, 3),
  active: bool(f, 4),
  admitted_at: u64(f, 5),
});

/** Listing: id=1, deal_id=2, seller=3, buyer=4, quantity=5 (Int), price=6
 *  (Coin), state=7, filled_by=8, listed_at=9, filled_at=10.
 *
 *  `buyer` is the NAMED counterparty of a private placement, and empty means
 *  the listing is open to anybody onboarded. `filled_by` is who actually took
 *  it. Two different fields, and collapsing them would hide the case the
 *  secondary market exists to make visible: a seller naming one buyer. */
export const decodeListing = (f) => ({
  id: u64(f, 1),
  deal_id: u64(f, 2),
  seller: str(f, 3),
  buyer: str(f, 4),
  quantity: str(f, 5) || '0',
  price: sub(f, 6, decodeCoin) ?? { denom: '', amount: '0' },
  state: LISTING_STATE[u32(f, 7)] ?? 'LISTING_STATE_UNSPECIFIED',
  filled_by: str(f, 8),
  listed_at: u64(f, 9),
  filled_at: u64(f, 10),
});

/* --------------------------------------------------------- query envelopes */
/* query.proto. Each response wraps its payload, so the wrapper is decoded here
   and the page never sees a field number. */

export const QUERY = {
  Params: {
    path: '/blockchain.syndicate.v1.Query/Params',
    request: () => new Uint8Array(),
    response: (f) => ({ params: sub(f, 1, decodeParams) }),
  },
  Operators: {
    path: '/blockchain.syndicate.v1.Query/Operators',
    request: () => new Uint8Array(),
    response: (f) => ({ operators: subs(f, 1, decodeOperator) }),
  },
  /**
   * Whether an address has completed onboarding.
   *
   * `onboarded` is its own boolean and is NOT the same question as whether the
   * investor record came back. The keeper answers rather than erroring for an
   * address it has never seen, and it reports `onboarded: false` for one whose
   * onboarding was withdrawn — so a page that inferred admission from the
   * record's presence would tell a struck-off investor they may still bid.
   */
  Investor: {
    path: '/blockchain.syndicate.v1.Query/Investor',
    request: (address) => write((w) => w.string(1, address)),
    response: (f) => ({
      onboarded: bool(f, 1),
      investor: sub(f, 2, decodeInvestor),
    }),
  },
  Deal: {
    path: '/blockchain.syndicate.v1.Query/Deal',
    request: (id) => write((w) => w.num(1, id)),
    response: (f) => ({ deal: sub(f, 1, decodeDeal) }),
  },
  DealByRef: {
    path: '/blockchain.syndicate.v1.Query/DealByRef',
    request: (reference) => write((w) => w.string(1, reference)),
    response: (f) => ({ deal: sub(f, 1, decodeDeal) }),
  },
  Deals: {
    path: '/blockchain.syndicate.v1.Query/Deals',
    request: () => new Uint8Array(),
    response: (f) => ({ deals: subs(f, 1, decodeDeal) }),
  },
  Commitments: {
    path: '/blockchain.syndicate.v1.Query/Commitments',
    request: (dealId) => write((w) => w.num(1, dealId)),
    response: (f) => ({ commitments: subs(f, 1, decodeCommitment) }),
  },
  /**
   * What one holder could claim right now.
   *
   * `claimable` is settled and unsettled TOGETHER, which is what a statement
   * shows. The settled part alone reads zero for a holder who has never
   * transferred — most of them, most of the time — and those are precisely the
   * people who would conclude the platform had lost their money.
   */
  Entitlement: {
    path: '/blockchain.syndicate.v1.Query/Entitlement',
    request: ({ dealId, holder }) => write((w) => w.num(1, dealId).string(2, holder)),
    response: (f) => ({
      balance: str(f, 1) || '0',
      claimable: sub(f, 2, decodeCoin) ?? { denom: '', amount: '0' },
      share_bps: u32(f, 3),
    }),
  },
  Listings: {
    path: '/blockchain.syndicate.v1.Query/Listings',
    request: ({ dealId, liveOnly = false }) =>
      write((w) => w.num(1, dealId).bool(2, liveOnly)),
    response: (f) => ({ listings: subs(f, 1, decodeListing) }),
  },
};

/* ---------------------------------------------------- balances, for a holder */

/**
 * cosmos.bank.v1beta1.Query/Balance — address=1, denom=2.
 *
 * Read directly rather than inferred from the Entitlement response, for the one
 * screen that has to show a holding in a deal the reader has not named. The
 * response is a single Coin at field 1.
 */
export const BALANCE_QUERY = '/cosmos.bank.v1beta1.Query/Balance';
export const balanceRequest = (address, denom) =>
  write((w) => w.string(1, address).string(2, denom));
export const decodeBalance = (bytes) =>
  sub(read(bytes), 1, decodeCoin) ?? { denom: '', amount: '0' };
