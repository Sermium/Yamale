/**
 * A hand-written protobuf codec, and the ABCI transport it rides on.
 *
 * WHY THIS EXISTS RATHER THAN A REST CLIENT
 *
 * The obvious way to read a Cosmos module from a browser is the gRPC-gateway:
 * `/api/rest/yamale/blockchain/<module>/v1/...`, JSON in and JSON out, no
 * codec to write. This deployment's proxy allowlists that surface per module
 * path, and most paths are denied. Measured against the live node on
 * 2026-08-31:
 *
 *   /api/rest/yamale/blockchain/enforcement/v1/params → 200
 *   /api/rest/yamale/blockchain/netting/v1/params     → 401
 *   /api/rest/yamale/blockchain/land/v1/params        → 401
 *
 * A 401 arrives in a browser as a login box. It looks like a page asking for a
 * password it was never given, and the reflex is to hunt for credentials that
 * do not exist. There are none: the path is simply not on the list. Note that
 * even x/land — whose own console reads REST — answers 401 for params today,
 * so the allowlist is not a stable thing to build on.
 *
 * The ABCI query endpoint under `/api/rpc/` answers every one of these
 * unauthenticated, because it is the node's own query router rather than a
 * gateway in front of it. The cost is that it speaks protobuf, so this file
 * pays that cost once.
 *
 * The upside is worth naming: this console cannot be silently switched off by
 * an allowlist edit. An oversight surface that stops answering when somebody
 * adjusts a proxy config is not oversight.
 *
 * The decoder is deliberately partial. It reads the fields these two consoles
 * render and skips the rest, which is exactly what proto3's wire format is
 * designed to permit. It does not attempt to be a protobuf library.
 */

// ---------------------------------------------------------------------------
// Wire format: reading.
// ---------------------------------------------------------------------------

/** Reads a base-128 varint. Returns [BigInt, nextOffset]. */
export function readVarint(buf, off) {
  let result = 0n;
  let shift = 0n;
  for (;;) {
    if (off >= buf.length) throw new Error('varint runs past the end of the buffer');
    // A varint longer than ten bytes cannot be a 64-bit number; refusing here
    // stops a malformed field from spinning this loop forever.
    if (shift > 63n) throw new Error('varint wider than 64 bits');
    const b = buf[off++];
    result |= BigInt(b & 0x7f) << shift;
    if ((b & 0x80) === 0) break;
    shift += 7n;
  }
  return [result, off];
}

/**
 * A 64-bit integer as something JavaScript can hold honestly.
 *
 * Block heights, voting power and block counts all fit in a double, and
 * returning a Number for them keeps the arithmetic in the rest of this console
 * ordinary. Anything that does not fit comes back as a decimal string rather
 * than as a silently wrong Number — which is the failure this guards: 2^53 is
 * not a large number of base units of a currency, and a rounded balance on an
 * enforcement page is a rounded balance in somebody's seizure notice.
 *
 * The amounts themselves are not affected: cosmos.Int is carried on the wire
 * as a string in every message this file reads, so it never passes through
 * here at all.
 */
function toNumberOrString(v) {
  return v <= 9007199254740991n ? Number(v) : v.toString();
}

/** Zig-zag is only used by sint32/sint64; nothing here uses those. int64 on
 *  the wire is a plain varint, two's complement for negatives. */
function asSigned(v) {
  // Negative int64 values are encoded as ten-byte varints of the two's
  // complement. Heights are never negative in practice, but an unset
  // `closed_at_height` read from a corrupted store would otherwise render as
  // 18446744073709551615 — a number that means nothing to a reader.
  if (v >= 1n << 63n) v -= 1n << 64n;
  return toNumberOrString(v);
}

/**
 * Splits a message into its fields without interpreting them.
 *
 * Returns a Map from field number to an array of raw entries, so a repeated
 * field and a singular field are the same shape here and the schema decides
 * which it is. proto3 permits a singular field to appear more than once (last
 * one wins) and permits a repeated field to appear zero times, and this shape
 * survives both.
 */
export function splitFields(buf) {
  const out = new Map();
  let off = 0;
  while (off < buf.length) {
    let tag;
    [tag, off] = readVarint(buf, off);
    const field = Number(tag >> 3n);
    const wire = Number(tag & 7n);
    if (field === 0) throw new Error('field number 0 is not valid');
    let entry;
    switch (wire) {
      case 0: {
        let v;
        [v, off] = readVarint(buf, off);
        entry = { wire, varint: v };
        break;
      }
      case 2: {
        let len;
        [len, off] = readVarint(buf, off);
        const n = Number(len);
        if (off + n > buf.length) throw new Error('length-delimited field runs past the end');
        entry = { wire, bytes: buf.subarray(off, off + n) };
        off += n;
        break;
      }
      case 5:
        entry = { wire, bytes: buf.subarray(off, off + 4) };
        off += 4;
        break;
      case 1:
        entry = { wire, bytes: buf.subarray(off, off + 8) };
        off += 8;
        break;
      default:
        // Groups (3, 4) were removed in proto3. Anything else is a corrupt
        // buffer, and guessing past it would produce a plausible-looking
        // record that is not what the chain holds.
        throw new Error(`unsupported wire type ${wire} on field ${field}`);
    }
    const list = out.get(field);
    if (list) list.push(entry);
    else out.set(field, [entry]);
  }
  return out;
}

const utf8 = new TextDecoder('utf-8', { fatal: false });

/**
 * Decodes a message against a schema.
 *
 * A schema is `{ <fieldNumber>: [name, kind, opts?] }` where kind is one of
 * 'uint64' | 'int64' | 'bool' | 'string' | 'bytes' | 'enum' | a nested schema,
 * and opts may set `{ repeated: true }`.
 *
 * Defaults are filled in for every field the schema names, so a caller never
 * has to distinguish "absent" from "zero" by checking for undefined. That
 * distinction is real on the wire — proto3 does not transmit a zero — and
 * where it matters to a reader this console says so in words rather than
 * leaving a blank cell. See `oversight.js`: an id of 0 and an unset id are the
 * same bytes, which is why nothing here treats a zero id as "missing".
 */
export function decode(buf, schema) {
  const fields = splitFields(buf);
  const out = {};
  for (const [numStr, spec] of Object.entries(schema)) {
    const num = Number(numStr);
    const [name, kind, opts] = spec;
    const repeated = !!(opts && opts.repeated);
    const entries = fields.get(num) || [];
    if (repeated) {
      out[name] = entries.map((e) => readOne(e, kind));
    } else if (entries.length === 0) {
      out[name] = defaultFor(kind);
    } else {
      // proto3: last occurrence wins for a singular field.
      out[name] = readOne(entries[entries.length - 1], kind);
    }
  }
  return out;
}

function defaultFor(kind) {
  if (typeof kind === 'object') return null; // an absent submessage is null, not {}
  switch (kind) {
    case 'string': return '';
    case 'bool': return false;
    case 'bytes': return new Uint8Array(0);
    default: return 0;
  }
}

function readOne(entry, kind) {
  if (typeof kind === 'object') {
    if (entry.wire !== 2) throw new Error('expected a length-delimited submessage');
    return decode(entry.bytes, kind);
  }
  switch (kind) {
    case 'uint64':
    case 'enum':
      if (entry.wire !== 0) throw new Error('expected a varint');
      return toNumberOrString(entry.varint);
    case 'int64':
      if (entry.wire !== 0) throw new Error('expected a varint');
      return asSigned(entry.varint);
    case 'bool':
      if (entry.wire !== 0) throw new Error('expected a varint');
      return entry.varint !== 0n;
    case 'string':
      if (entry.wire !== 2) throw new Error('expected a length-delimited string');
      return utf8.decode(entry.bytes);
    case 'bytes':
      if (entry.wire !== 2) throw new Error('expected length-delimited bytes');
      return entry.bytes;
    default:
      throw new Error(`unknown kind ${kind}`);
  }
}

// ---------------------------------------------------------------------------
// Wire format: writing. Only what a query request needs.
// ---------------------------------------------------------------------------

function writeVarint(bytes, v) {
  let n = BigInt(v);
  if (n < 0n) throw new Error('negative varint');
  for (;;) {
    const b = Number(n & 0x7fn);
    n >>= 7n;
    if (n === 0n) { bytes.push(b); return; }
    bytes.push(b | 0x80);
  }
}

/** Encodes `{ <fieldNumber>: value }` where a number is a varint and a string
 *  is length-delimited UTF-8. A zero or empty value is omitted, as proto3
 *  requires — and as every server here expects, since a request carrying an
 *  explicit zero and one omitting it must decode identically. */
export function encode(obj) {
  const bytes = [];
  const enc = new TextEncoder();
  for (const num of Object.keys(obj).map(Number).sort((a, b) => a - b)) {
    const v = obj[num];
    if (v === undefined || v === null) continue;
    if (typeof v === 'string') {
      if (v === '') continue;
      const payload = enc.encode(v);
      writeVarint(bytes, (num << 3) | 2);
      writeVarint(bytes, payload.length);
      for (const b of payload) bytes.push(b);
    } else if (typeof v === 'boolean') {
      if (!v) continue;
      writeVarint(bytes, num << 3);
      bytes.push(1);
    } else if (v instanceof Uint8Array) {
      if (v.length === 0) continue;
      writeVarint(bytes, (num << 3) | 2);
      writeVarint(bytes, v.length);
      for (const b of v) bytes.push(b);
    } else {
      if (Number(v) === 0) continue;
      writeVarint(bytes, num << 3);
      writeVarint(bytes, v);
    }
  }
  return new Uint8Array(bytes);
}

/** A submessage field, for the one request that carries pagination. */
export function encodeSub(num, inner) {
  if (inner.length === 0) return new Uint8Array(0);
  const bytes = [];
  writeVarint(bytes, (num << 3) | 2);
  writeVarint(bytes, inner.length);
  for (const b of inner) bytes.push(b);
  return new Uint8Array(bytes);
}

export function concat(...parts) {
  const total = parts.reduce((n, p) => n + p.length, 0);
  const out = new Uint8Array(total);
  let off = 0;
  for (const p of parts) { out.set(p, off); off += p.length; }
  return out;
}

export function toHex(bytes) {
  let s = '';
  for (const b of bytes) s += b.toString(16).padStart(2, '0');
  return s;
}

export function fromBase64(b64) {
  if (!b64) return new Uint8Array(0);
  // atob exists in the browser; Buffer is the Node path the tests take.
  if (typeof atob === 'function') {
    const bin = atob(b64);
    const out = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i += 1) out[i] = bin.charCodeAt(i);
    return out;
  }
  return new Uint8Array(Buffer.from(b64, 'base64'));
}

// ---------------------------------------------------------------------------
// The schemas.
//
// Field numbers are copied from proto/blockchain/{enforcement,netting}/v1/.
// They are the load-bearing part of this file: a wrong number here does not
// throw, it renders a different field's value under the right label, which on
// an enforcement page means showing one account's freeze against another
// account's address. Each block below names its proto file so the numbers can
// be checked against the source rather than against memory.
// ---------------------------------------------------------------------------

/** cosmos/base/v1beta1/coin.proto */
export const Coin = {
  1: ['denom', 'string'],
  2: ['amount', 'string'], // cosmos.Int — a decimal string on the wire, never a varint
};

/** cosmos/base/query/v1beta1/pagination.proto */
export const PageResponse = {
  1: ['next_key', 'bytes'],
  2: ['total', 'uint64'],
};

// ------------------------------------------------------------- x/enforcement

/** enforcement/v1/enforcement.proto — LegalInstrument */
export const LegalInstrument = {
  1: ['issuing_authority', 'string'],
  2: ['reference', 'string'],
  3: ['kind', 'enum'],
  4: ['hash', 'string'],
  5: ['issued_at', 'int64'],
};

/** enforcement/v1/enforcement.proto — Case */
export const Case = {
  1: ['id', 'uint64'],
  2: ['target', 'string'],
  3: ['opener', 'string'],
  4: ['action', 'enum'],
  5: ['status', 'enum'],
  6: ['reason', 'string'],
  7: ['evidence_uri', 'string'],
  8: ['evidence_hash', 'string'],
  9: ['opened_at_height', 'int64'],
  10: ['voting_ends_at_height', 'int64'],
  11: ['resolved_at_height', 'int64'],
  12: ['total_power_at_open', 'int64'],
  13: ['yes_power', 'int64'],
  14: ['no_power', 'int64'],
  15: ['abstain_power', 'int64'],
  16: ['recovered', Coin, { repeated: true }],
  17: ['sweep_complete', 'bool'],
  18: ['emergency', 'bool'],
  19: ['legal_instrument', LegalInstrument],
  20: ['execute_at_height', 'int64'],
  21: ['assessed_value', Coin, { repeated: true }],
};

/** enforcement/v1/enforcement.proto — Vote */
export const Vote = {
  1: ['case_id', 'uint64'],
  2: ['validator', 'string'],
  3: ['option', 'enum'],
  4: ['power', 'int64'],
};

/** enforcement/v1/enforcement.proto — Freeze */
export const Freeze = {
  1: ['address', 'string'],
  2: ['case_id', 'uint64'],
  3: ['expires_at_height', 'int64'],
  4: ['frozen_at_height', 'int64'],
};

/** enforcement/v1/params.proto — SeizureDelayTier */
export const SeizureDelayTier = {
  1: ['threshold', Coin],
  2: ['delay_blocks', 'uint64'],
};

/** enforcement/v1/params.proto — Params.
 *  Field 8 is reserved: it held `emergency_authority` before that power became
 *  ROLE_ENFORCEMENT_AUTHORITY in x/alias. It is left out here rather than
 *  skipped silently, so nobody adds a field 8 to this schema and decodes an
 *  old chain's emergency authority as something else. */
export const EnforcementParams = {
  1: ['voting_period_blocks', 'uint64'],
  2: ['provisional_freeze_blocks', 'uint64'],
  3: ['threshold_bps', 'uint64'],
  4: ['recovery_destination', 'string'],
  5: ['max_reason_length', 'uint64'],
  6: ['max_evidence_uri_length', 'uint64'],
  7: ['seize_requires_evidence', 'bool'],
  9: ['seizure_delay_blocks', 'uint64'],
  10: ['seizure_delay_tiers', SeizureDelayTier, { repeated: true }],
  11: ['seizure_window_blocks', 'uint64'],
  12: ['seizure_window_cap', Coin, { repeated: true }],
  13: ['max_seizures_per_window', 'uint64'],
  14: ['ombudsman', 'string'],
};

/** enforcement/v1/query.proto */
export const QueryEnforcementParamsResponse = { 1: ['params', EnforcementParams] };
export const QueryOpenCasesResponse = { 1: ['case', Case, { repeated: true }] };
export const QueryHeldCasesResponse = { 1: ['case', Case, { repeated: true }] };
export const QueryListCaseResponse = {
  1: ['case', Case, { repeated: true }],
  2: ['pagination', PageResponse],
};
export const QueryListFreezeResponse = {
  1: ['freeze', Freeze, { repeated: true }],
  2: ['pagination', PageResponse],
};
export const QueryGetCaseResponse = {
  1: ['case', Case],
  2: ['votes', Vote, { repeated: true }],
};
export const QueryCaseVotesResponse = {
  1: ['votes', Vote, { repeated: true }],
  2: ['yes_power', 'int64'],
  3: ['no_power', 'int64'],
  4: ['abstain_power', 'int64'],
  5: ['total_power_at_open', 'int64'],
  6: ['required_power', 'int64'],
};
export const QueryFreezeStatusResponse = {
  1: ['frozen', 'bool'],
  2: ['freeze', Freeze],
  3: ['case', Case],
};
export const QueryRecoveredResponse = {
  1: ['total', Coin, { repeated: true }],
  2: ['cases_opened', 'uint64'],
  3: ['cases_passed', 'uint64'],
};
export const QuerySeizureWindowResponse = {
  1: ['window_start_height', 'int64'],
  2: ['current_height', 'int64'],
  3: ['seized', Coin, { repeated: true }],
  4: ['cap', Coin, { repeated: true }],
  5: ['remaining', Coin, { repeated: true }],
  6: ['seizure_count', 'uint64'],
  7: ['max_seizures', 'uint64'],
};

// ----------------------------------------------------------------- x/netting

/** netting/v1/params.proto */
export const DenomPolicy = {
  1: ['denom', 'string'],
  2: ['gross_threshold', 'string'],
};
export const NettingParams = {
  1: ['cycle_blocks', 'uint64'],
  2: ['denom_policies', DenomPolicy, { repeated: true }],
};

/** netting/v1/netting.proto */
export const DenomOutcome = {
  1: ['denom', 'string'],
  2: ['status', 'enum'],
  3: ['gross_amount', 'string'],
  4: ['net_amount', 'string'],
  5: ['obligation_count', 'uint64'],
  6: ['hold_reason', 'string'],
};
export const Cycle = {
  1: ['id', 'uint64'],
  2: ['opened_at_height', 'int64'],
  3: ['closed_at_height', 'int64'],
  4: ['status', 'enum'],
  5: ['outcomes', DenomOutcome, { repeated: true }],
};
export const Obligation = {
  1: ['cycle_id', 'uint64'],
  2: ['id', 'uint64'],
  3: ['from_participant', 'string'],
  4: ['to_participant', 'string'],
  5: ['denom', 'string'],
  6: ['amount', 'string'],
  7: ['batch_hash', 'bytes'],
  8: ['mode', 'enum'],
  9: ['submitted_at_height', 'int64'],
};

/** netting/v1/query.proto */
export const QueryNettingParamsResponse = { 1: ['params', NettingParams] };
export const QueryCurrentCycleResponse = {
  1: ['cycle', Cycle],
  2: ['closes_at_height', 'int64'],
};
export const DenomCompression = {
  1: ['denom', 'string'],
  2: ['compression_bps', 'uint64'],
};
export const QueryCycleResponse = {
  1: ['cycle', Cycle],
  2: ['compression', DenomCompression, { repeated: true }],
};
export const PositionEntry = {
  1: ['denom', 'string'],
  2: ['reserve', 'string'],
  3: ['locked', 'string'],
  4: ['available', 'string'],
  5: ['net_position', 'string'],
};
export const QueryPositionResponse = { 1: ['entries', PositionEntry, { repeated: true }] };
export const QueryParticipantObligationsResponse = {
  1: ['obligations', Obligation, { repeated: true }],
  2: ['pagination', PageResponse],
};
export const HeldSlice = {
  1: ['cycle_id', 'uint64'],
  2: ['denom', 'string'],
  3: ['reason', 'string'],
  4: ['held_since_height', 'int64'],
};
export const QueryHeldSlicesResponse = { 1: ['held', HeldSlice, { repeated: true }] };

// ---------------------------------------------------------------------------
// The enum tables.
//
// Numbers rather than names come off the wire, and a number the chain knows
// about but this table does not must render as an unknown value rather than as
// a blank or, worse, as the zero case. "Unspecified" and "a status this page
// was written before" are different facts about a seizure.
// ---------------------------------------------------------------------------

export const CASE_STATUS = {
  0: 'UNSPECIFIED', 1: 'VOTING', 2: 'PASSED', 3: 'REJECTED', 4: 'EXPIRED',
  5: 'WITHDRAWN', 6: 'REVERSED', 7: 'HELD', 8: 'VETOED',
};
export const CASE_ACTION = { 0: 'UNSPECIFIED', 1: 'FREEZE', 2: 'SEIZE' };
export const VOTE_OPTION = { 0: 'UNSPECIFIED', 1: 'YES', 2: 'NO', 3: 'ABSTAIN' };
export const LEGAL_INSTRUMENT_KIND = {
  0: 'UNSPECIFIED', 1: 'COURT_ORDER', 2: 'REGULATORY_DIRECTION', 3: 'WARRANT',
};
export const CYCLE_STATUS = { 0: 'UNSPECIFIED', 1: 'OPEN', 2: 'SETTLED', 3: 'HELD' };
export const DENOM_STATUS = { 0: 'UNSPECIFIED', 1: 'OPEN', 2: 'SETTLED', 3: 'HELD' };
export const SETTLEMENT_MODE = { 0: 'UNSPECIFIED', 1: 'GROSS', 2: 'NET' };

export function enumName(table, v) {
  const name = table[v];
  return name === undefined ? `UNKNOWN_${v}` : name;
}
