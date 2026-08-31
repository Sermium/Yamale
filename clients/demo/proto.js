// Protobuf, by hand, because this page has no build step.
//
// WHY IT IS NEEDED, measured on 2026-08-31 against the live deployment rather
// than read off the nginx config:
//
//   $ curl -so /dev/null -w '%{http_code}\n' \
//       https://pay.yamalelegal.com/api/rest/yamale/blockchain/land/v1/params
//     401
//   $ curl -so /dev/null -w '%{http_code}\n' \
//       https://pay.yamalelegal.com/api/rest/yamale/blockchain/enforcement/v1/params
//     200
//
// The proxy allowlists REST per module prefix and denies by default. x/land,
// x/paymsg, x/netting and x/builderfee are not on the list; the rest are. So a
// tour page that read only REST would show four of its mechanisms as a browser
// login box — and a login box is a 401, not a missing password.
//
// The same queries answer unauthenticated over ABCI at /api/rpc/abci_query,
// which the proxy does publish. ABCI speaks protobuf and nothing else, so this
// file exists.
//
// It is small on purpose. No groups, no maps, no packed repeated scalars, no
// floats — none of the messages read here contain any. Every decoder names the
// .proto file and the field numbers it was written from, and proto.test.js
// round-trips each one, so drift shows up as a failing test rather than as a
// parcel rendered with the holder in the authority's field.

/* ------------------------------------------------------------------ writing */

class Writer {
  constructor() { this.bytes = []; }

  /** Base-128 varint, little-endian groups of seven bits. */
  varint(value) {
    let v = BigInt(value);
    if (v < 0n) v += 1n << 64n;          // int64 negatives are ten-byte varints
    do {
      const byte = Number(v & 0x7fn);
      v >>= 7n;
      this.bytes.push(v > 0n ? byte | 0x80 : byte);
    } while (v > 0n);
    return this;
  }

  tag(field, wire) { return this.varint((field << 3) | wire); }

  bytesField(field, payload) {
    if (!payload || payload.length === 0) return this;   // proto3 omits defaults
    this.tag(field, 2).varint(payload.length);
    for (const b of payload) this.bytes.push(b);
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
 * A map of arrays rather than an object, because repeated fields are the whole
 * point here: a transfer's attestors arrive as three separate field-8 entries,
 * and a reader that kept only the last one would show a transfer attested by
 * one office when three signed it — which is exactly the guarantee this page
 * claims. Unknown fields are kept rather than refused: a node running newer
 * code emits fields this file has never heard of, and ignoring them is correct
 * where failing to parse the record around them is not.
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
      case 5: i += 4; break;    // fixed32 — none in what is read here
      case 1: i += 8; break;    // fixed64 — likewise
      default: throw new Error(`unsupported wire type ${key & 7}`);
    }
  }
  return fields;
}

const DECODER = new TextDecoder();

/** A string field, or '' when absent. proto3 cannot distinguish the two. */
export const str = (m, n) => {
  const v = m.get(n)?.[0];
  return v === undefined ? '' : DECODER.decode(v);
};

/**
 * A numeric field, or 0 when absent.
 *
 * Returned as a Number, which is exact to 2^53. Every field read through this
 * is a block height, a count, a basis-point value or a unix second — all far
 * inside that. Token amounts are NOT read through here: they are strings on
 * the wire precisely because they overflow a double, and they stay strings.
 */
export const num = (m, n) => {
  const v = m.get(n)?.[0];
  return v === undefined ? 0 : Number(v);
};

export const list = (m, n) => m.get(n) ?? [];

/* -------------------------------------------------------------- byte codings */

export const toHex = (bytes) =>
  Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');

export function fromBase64(text) {
  if (!text) return new Uint8Array();
  const binary = typeof atob === 'function'
    ? atob(text)
    : Buffer.from(text, 'base64').toString('binary');
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) out[i] = binary.charCodeAt(i);
  return out;
}

/* ========================================================================= */
/*  Decoders                                                                 */
/* ========================================================================= */

/**
 * proto/blockchain/land/v1/parcel.proto — Status.
 *
 * Indexed by the enum's own numbers, so a value this build has never seen
 * renders as its number rather than as REGISTERED. A parcel wrongly shown as
 * registered is a parcel somebody buys.
 */
export const PARCEL_STATUS = [
  'STATUS_UNSPECIFIED',
  'STATUS_REGISTERED',
  'STATUS_TRANSFER_PENDING',
  'STATUS_DISPUTED',
  'STATUS_FROZEN',
];

export const statusName = (n) => PARCEL_STATUS[n] ?? `STATUS_${n}`;

/** parcel.proto — Parcel. Fields 1..12. */
export function decodeParcel(bytes) {
  const m = read(bytes);
  return {
    id: num(m, 1),
    geometryHash: str(m, 2),
    cadastralRef: str(m, 3),
    holder: str(m, 4),
    authority: str(m, 5),
    status: num(m, 6),
    encumbrances: list(m, 7).length,
    registeredAt: num(m, 8),
    deeds: list(m, 9).length,
    restrictions: list(m, 10).map((r) => {
      const f = read(r);
      return { kind: str(f, 1), value: str(f, 2), lifted: num(f, 6) === 1 };
    }),
    vehicleId: num(m, 11),
    freezes: list(m, 12).length,
  };
}

/** parcel.proto — Transfer. Fields 1..13. */
export function decodeTransfer(bytes) {
  const m = read(bytes);
  return {
    id: num(m, 1),
    parcelId: num(m, 2),
    from: str(m, 3),
    to: str(m, 4),
    price: str(m, 5),
    validated: num(m, 6) === 1,
    validatedBy: str(m, 7),
    attestors: list(m, 8).map((a) => DECODER.decode(a)),
    quorumAt: num(m, 9),
    objectedBy: str(m, 10),
    objectionReason: str(m, 11),
    proposedAt: num(m, 12),
    completedAt: num(m, 13),
  };
}

/** parcel.proto — Authority. Fields 1..4. */
export function decodeAuthority(bytes) {
  const m = read(bytes);
  return {
    address: str(m, 1),
    name: str(m, 2),
    jurisdiction: str(m, 3),
    active: num(m, 4) === 1,
  };
}

/** proto/blockchain/land/v1/params.proto — Params. */
export function decodeLandParams(bytes) {
  const m = read(bytes);
  return { attestationQuorum: num(m, 1), challengeWindowSeconds: num(m, 2) };
}

/**
 * A response whose only field is a repeated message, and the total the
 * paginator reports.
 *
 * `total` is read separately because it is the number the page states out
 * loud. A list of length 3 in a page of 100 and a list of length 100 in a
 * corpus of 4,000 look identical from the array alone.
 */
export function decodeList(bytes, decoder, { itemField = 1, pageField = 2 } = {}) {
  const m = read(bytes);
  const items = list(m, itemField).map((b) => decoder(b));
  const page = m.get(pageField)?.[0];
  // cosmos.base.query.v1beta1.PageResponse { next_key = 1, total = 2 }
  const total = page ? num(read(page), 2) : 0;
  return { items, total: total || items.length };
}

/** A response wrapping exactly one message in field 1, or null if absent. */
export function decodeOne(bytes, decoder, field = 1) {
  const inner = read(bytes).get(field)?.[0];
  return inner === undefined ? null : decoder(inner);
}
