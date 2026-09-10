// Protobuf, by hand, because this console has no build step.
//
// WHY THIS FILE EXISTS, measured rather than assumed:
//
//   $ curl -si https://pay.yamalelegal.com/api/rest/yamale/blockchain/land/v1/params
//     HTTP/1.1 401 Unauthorized
//     WWW-Authenticate: Basic realm="Yamale — supervisor access"
//
//   $ curl -s 'https://pay.yamalelegal.com/api/rpc/abci_query?path="/blockchain.land.v1.Query/Params"&data=0x'
//     {"result":{"response":{"code":0,"value":"CgYIAxCA6kk=","height":"94464"}}}
//
// That was measured against x/land, which this console read at the time. The
// deployment's proxy allowlists REST paths per module and denies by default,
// so every read returned a browser login box with no credentials to type into
// it — on a register whose entire premise is that reading it is public. The
// identical queries answer unauthenticated over the node's ABCI interface,
// which the proxy does publish; it is the same discovery
// clients/app/src/standing.ts records for x/paymsg.
//
// The console has since moved to x/herd, and the finding moved with it rather
// than expiring: a new module's REST prefix is not on that allowlist either,
// and being new is exactly why. ABCI speaks protobuf and nothing else, so this
// file is what makes the register readable.
//
// It is small on purpose. Nothing here is a general protobuf implementation:
// there are no groups, no packed repeated scalars (x/herd has none), no maps,
// no float or fixed64 fields (x/herd has none of those either). Every codec
// below names the .proto file and field numbers it was written from, and
// proto.test.js round-trips each one, so drift shows up as a failing test
// rather than as a certificate that renders with the sire in the dam's field.

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
 * a parcel's encumbrances arrive as twelve separate field-7 entries and a
 * reader that kept only the last one would show a title carrying one mortgage
 * when it carries twelve. That is the failure mode this shape rules out.
 *
 * Unknown fields are kept rather than skipped. A node running newer code than
 * this page emits fields this file has never heard of, and the correct
 * behaviour is to ignore them, not to fail to parse the parcel around them.
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
      case 5: i += 4; break;   // fixed32 — none in x/herd, skipped rather than refused
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
 * A string and not a Number, deliberately. Heights and ids fit in a double
 * today; account numbers and Unix seconds are the same shape and will not
 * always. More immediately, and this is the one that bites: an id of zero is
 * meaningful in x/herd. `sire_id` and `dam_id` are 0 when the parentage is
 * unknown, which is the ordinary state of a founder, and the keeper never
 * issues animal 0 so that the two can be told apart. As a string, `'0'` is
 * truthy and forces the comparison to be written out; as a Number it is falsy,
 * and every `if (animal.sire_id)` on the page would quietly agree that no
 * founder has ever been recorded.
 */
export const u64 = (f, num) => String(first(f, num) ?? 0n);

/** uint32 and enums, where a Number is the natural type and the range is safe. */
export const u32 = (f, num) => Number(first(f, num) ?? 0n);

export const bool = (f, num) => Boolean(first(f, num) ?? 0n);

export const bytes = (f, num) => first(f, num) ?? new Uint8Array();

/** A nested message, decoded by `fn`, or `null` when the field is absent. */
export const sub = (f, num, fn) => {
  const v = first(f, num);
  return v === undefined ? null : fn(read(v));
};

/** Every entry of a repeated message field, in wire order. */
export const subs = (f, num, fn) => all(f, num).map((v) => fn(read(v)));

/** Every entry of a repeated string field, in wire order. */
export const strs = (f, num) => all(f, num).map((v) => decoder.decode(v));

/* -------------------------------------------------------------------- base64 */
/* Kept here rather than reached for from the page, because ABCI hands back
   base64 and the wallet protocol takes it — two places, one implementation. */

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
   x/herd — proto/blockchain/herd/v1/herd.proto, params.proto, query.proto.

   This console used to read x/land, and why it no longer does is worth
   recording rather than quietly dropping. A parcel and an animal do have the
   same shape — a uniquely identified high-value thing, registered by an
   admitted office, carrying documents, transferred under supervision — and
   that correspondence carried the demonstrator a long way. What x/land could
   not hold was PARENTAGE AS A RELATION. Ancestry lived in deed text, the
   pedigree graph was reassembled in this browser, and so the one claim a
   Ladoum certificate actually makes was checked by the page rendering it. A
   check that lives in a page is a display convention: it refuses nothing, and
   anybody sending a transaction straight to the chain never meets it.

   x/herd holds that line in consensus. A sire that is not male, a dam that
   gave birth a fortnight earlier, an animal declared its own ancestor — the
   chain refuses those now, so this file no longer has to pretend to.

   Field numbers are transcribed from the .proto rather than from the generated
   Go, because the .proto is the source both of them come from. Each decoder
   lists them so a reviewer can check this file against that one without
   holding a schema in their head.
   ============================================================================ */

/** Level — herd.proto. The whole design: a record costs what it proves. */
export const LEVEL = ['LEVEL_UNSPECIFIED', 'LEVEL_HERD', 'LEVEL_INDIVIDUAL',
  'LEVEL_CERTIFIED'];

/** Species — herd.proto. */
export const SPECIES = ['SPECIES_UNSPECIFIED', 'SPECIES_CATTLE', 'SPECIES_SHEEP',
  'SPECIES_GOAT', 'SPECIES_HORSE', 'SPECIES_CAMEL'];

/** Sex — herd.proto. The pedigree refusals turn on this. */
export const SEX = ['SEX_UNSPECIFIED', 'SEX_MALE', 'SEX_FEMALE'];

/** Status — herd.proto. A record that is not ACTIVE cannot begin a transfer. */
export const STATUS = ['STATUS_UNSPECIFIED', 'STATUS_ACTIVE',
  'STATUS_TRANSFER_PENDING', 'STATUS_DISPUTED', 'STATUS_FROZEN', 'STATUS_RETIRED'];

/** TransferState — herd.proto. */
export const TRANSFER_STATE = ['TRANSFER_STATE_UNSPECIFIED',
  'TRANSFER_STATE_PROPOSED', 'TRANSFER_STATE_ACCEPTED', 'TRANSFER_STATE_ATTESTED',
  'TRANSFER_STATE_OBJECTED', 'TRANSFER_STATE_COMPLETED', 'TRANSFER_STATE_CANCELLED'];

/**
 * Params: challenge_window=1, attestation_quorum=2,
 * same_authority_attestation=3, min_birth_interval_days=4,
 * require_dna_for_certified=5.
 *
 * Note the field order against x/land's, which put the quorum first. Carrying
 * the old decoder across unchanged would have read the challenge window as the
 * quorum: both are numbers, neither is absent, nothing would have thrown, and
 * the page would have rendered "604800 attestations required".
 */
export const decodeParams = (f) => ({
  challenge_window: u64(f, 1),
  attestation_quorum: String(u32(f, 2)),
  same_authority_attestation: bool(f, 3),
  min_birth_interval_days: String(u32(f, 4)),
  require_dna_for_certified: bool(f, 5),
});

/**
 * Herd: id=1, reference=2, keeper=3, authority=4, species=5, declared_size=6,
 * locality=7, location_hash=8, jurisdiction=9, status=10, registered_at=11.
 */
export const decodeHerd = (f) => ({
  id: u64(f, 1),
  reference: str(f, 2),
  keeper: str(f, 3),
  authority: str(f, 4),
  species: SPECIES[u32(f, 5)] ?? 'SPECIES_UNSPECIFIED',
  declared_size: String(u32(f, 6)),
  locality: str(f, 7),
  location_hash: str(f, 8),
  jurisdiction: str(f, 9),
  status: STATUS[u32(f, 10)] ?? 'STATUS_UNSPECIFIED',
  registered_at: u64(f, 11),
});

/**
 * Animal: id=1, herd_id=2, national_id=3, tag=4, level=5, name=6, sex=7,
 * coat=8, born_on=9, birth_type=10, sire_id=11, dam_id=12, dna_hash=13,
 * photo_hash=14, holder=15, authority=16, status=17, registered_at=18.
 *
 * `sire_id` and `dam_id` come back as strings, and `'0'` means unknown
 * parentage — the honest and common case for a founder. The keeper never
 * issues animal 0 precisely so zero can carry that meaning, so any test for
 * "has a recorded sire" must compare against `'0'`: the string is truthy.
 */
export const decodeAnimal = (f) => ({
  id: u64(f, 1),
  herd_id: u64(f, 2),
  national_id: str(f, 3),
  tag: str(f, 4),
  level: LEVEL[u32(f, 5)] ?? 'LEVEL_UNSPECIFIED',
  name: str(f, 6),
  sex: SEX[u32(f, 7)] ?? 'SEX_UNSPECIFIED',
  coat: str(f, 8),
  born_on: str(f, 9),
  birth_type: str(f, 10),
  sire_id: u64(f, 11),
  dam_id: u64(f, 12),
  dna_hash: str(f, 13),
  photo_hash: str(f, 14),
  holder: str(f, 15),
  authority: str(f, 16),
  status: STATUS[u32(f, 17)] ?? 'STATUS_UNSPECIFIED',
  registered_at: u64(f, 18),
});

/** HealthEvent: id=1, herd_id=2, disease=3, batch=4, treated=5, agent=6,
 *  recorded_at=7. */
export const decodeHealthEvent = (f) => ({
  id: u64(f, 1), herd_id: u64(f, 2), disease: str(f, 3), batch: str(f, 4),
  treated: String(u32(f, 5)), agent: str(f, 6), recorded_at: u64(f, 7),
});

/** Attestation: attestor=1, authority=2, evidence_hash=3, attested_at=4. */
const decodeAttestation = (f) => ({
  attestor: str(f, 1), authority: str(f, 2),
  evidence_hash: str(f, 3), attested_at: u64(f, 4),
});

/**
 * Transfer: id=1, animal_id=2, from=3, to=4, price=5, state=6,
 * attestations=7, attested_at=8, objected_by=9, objection_reason=10,
 * proposed_at=11, resolved_by=12, resolution_reason=13, objection_upheld=14,
 * accepted_at=15.
 */
export const decodeTransfer = (f) => ({
  id: u64(f, 1),
  animal_id: u64(f, 2),
  from: str(f, 3),
  to: str(f, 4),
  price: str(f, 5),
  state: TRANSFER_STATE[u32(f, 6)] ?? 'TRANSFER_STATE_UNSPECIFIED',
  attestations: subs(f, 7, decodeAttestation),
  attested_at: u64(f, 8),
  objected_by: str(f, 9),
  objection_reason: str(f, 10),
  proposed_at: u64(f, 11),
  resolved_by: str(f, 12),
  resolution_reason: str(f, 13),
  objection_upheld: bool(f, 14),
  accepted_at: u64(f, 15),
});

/** Authority: address=1, name=2, jurisdiction=3, active=4. */
export const decodeAuthority = (f) => ({
  address: str(f, 1), name: str(f, 2), jurisdiction: str(f, 3), active: bool(f, 4),
});

/** PedigreeNode — query.proto: animal=1, generation=2, path=3. */
const decodePedigreeNode = (f) => ({
  animal: sub(f, 1, decodeAnimal),
  generation: u32(f, 2),
  path: str(f, 3),
});

/* --------------------------------------------------------- query envelopes */
/* query.proto. Each response wraps its payload, so the wrapper is decoded here
   and the page never sees a field number. */

export const QUERY = {
  Params: {
    path: '/blockchain.herd.v1.Query/Params',
    request: () => new Uint8Array(),
    response: (f) => ({ params: sub(f, 1, decodeParams) }),
  },
  Authorities: {
    path: '/blockchain.herd.v1.Query/Authorities',
    request: () => new Uint8Array(),
    response: (f) => ({ authorities: subs(f, 1, decodeAuthority) }),
  },
  Herd: {
    path: '/blockchain.herd.v1.Query/Herd',
    request: (id) => write((w) => w.num(1, id)),
    response: (f) => ({ herd: sub(f, 1, decodeHerd) }),
  },
  HerdByRef: {
    path: '/blockchain.herd.v1.Query/HerdByRef',
    request: (ref) => write((w) => w.string(1, ref)),
    response: (f) => ({ herd: sub(f, 1, decodeHerd) }),
  },
  Animal: {
    path: '/blockchain.herd.v1.Query/Animal',
    request: (id) => write((w) => w.num(1, id)),
    response: (f) => ({ animal: sub(f, 1, decodeAnimal) }),
  },
  AnimalByNationalID: {
    path: '/blockchain.herd.v1.Query/AnimalByNationalID',
    request: (nid) => write((w) => w.string(1, nid)),
    response: (f) => ({ animal: sub(f, 1, decodeAnimal) }),
  },
  AnimalByTag: {
    path: '/blockchain.herd.v1.Query/AnimalByTag',
    request: (tag) => write((w) => w.string(1, tag)),
    response: (f) => ({ animal: sub(f, 1, decodeAnimal) }),
  },
  /**
   * The endpoint the paper certificate cannot have.
   *
   * `truncated` is carried through rather than dropped. When the depth bound is
   * what stopped the walk, a page that showed only the nodes would render "no
   * further ancestors" where the truth is "we stopped looking" — and those are
   * opposite facts about a line.
   */
  Pedigree: {
    path: '/blockchain.herd.v1.Query/Pedigree',
    request: ({ animalId, depth = 0 }) =>
      write((w) => w.num(1, animalId).num(2, depth)),
    response: (f) => ({
      nodes: subs(f, 1, decodePedigreeNode),
      truncated: bool(f, 2),
    }),
  },
  Animals: {
    path: '/blockchain.herd.v1.Query/Animals',
    request: (herdId) => write((w) => w.num(1, herdId)),
    response: (f) => ({ animals: subs(f, 1, decodeAnimal) }),
  },
  HealthEvents: {
    path: '/blockchain.herd.v1.Query/HealthEvents',
    request: (herdId) => write((w) => w.num(1, herdId)),
    response: (f) => ({ health_events: subs(f, 1, decodeHealthEvent) }),
  },
  Transfer: {
    path: '/blockchain.herd.v1.Query/Transfer',
    request: (id) => write((w) => w.num(1, id)),
    response: (f) => ({ transfer: sub(f, 1, decodeTransfer) }),
  },
  TransfersByAnimal: {
    path: '/blockchain.herd.v1.Query/TransfersByAnimal',
    request: (animalId) => write((w) => w.num(1, animalId)),
    response: (f) => ({ transfers: subs(f, 1, decodeTransfer) }),
  },
};

/* ============================================================================
   The messages this console can compose — proto/blockchain/herd/v1/tx.proto.

   The type URL prefix is `blockchain.`, NOT `yamale.blockchain.`. The Go
   package path is `yamale/blockchain/x/herd/types` and the proto package is
   `blockchain.herd.v1`; only the second appears on the wire. Getting this
   wrong produces `unable to resolve type URL`, which reads like a chain fault
   and is a typo. Verified against x/herd/types/tx.pb.go.
   ============================================================================ */

export const TYPE = (name) => `/blockchain.herd.v1.${name}`;

/**
 * Every herd message this console can compose, by the name autocli gives its
 * subcommand.
 *
 * `encode` writes the protobuf; `signer` names the field the chain checks the
 * signature against (`cosmos.msg.v1.signer`), which decides who can possibly
 * send it — and therefore whether a browser can send it at all, or must put it
 * to an office's group policy instead.
 */
export const MSG = {
  MsgEnrolHerd: {
    // creator=1, reference=2, keeper=3, species=4, declared_size=5,
    // locality=6, location_hash=7
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).string(2, m.reference).string(3, m.keeper)
      .num(4, m.species).num(5, m.declared_size)
      .string(6, m.locality).string(7, m.location_hash)),
  },
  MsgIdentifyAnimal: {
    // creator=1, herd_id=2, national_id=3, tag=4, name=5, sex=6, coat=7,
    // born_on=8, birth_type=9
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).num(2, m.herd_id).string(3, m.national_id)
      .string(4, m.tag).string(5, m.name).num(6, m.sex).string(7, m.coat)
      .string(8, m.born_on).string(9, m.birth_type)),
  },
  MsgCertifyAnimal: {
    // creator=1, animal_id=2, sire_id=3, dam_id=4, dna_hash=5, photo_hash=6
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).num(2, m.animal_id).num(3, m.sire_id)
      .num(4, m.dam_id).string(5, m.dna_hash).string(6, m.photo_hash)),
  },
  MsgRecordHealthEvent: {
    // agent=1, herd_id=2, disease=3, batch=4, treated=5
    signer: 'agent',
    encode: (m) => write((w) => w
      .string(1, m.agent).num(2, m.herd_id).string(3, m.disease)
      .string(4, m.batch).num(5, m.treated)),
  },
  MsgProposeTransfer: {
    // creator=1, animal_id=2, to=3, price=4
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).num(2, m.animal_id).string(3, m.to).string(4, m.price)),
  },
  MsgAcceptTransfer: {
    // buyer=1, transfer_id=2
    signer: 'buyer',
    encode: (m) => write((w) => w.string(1, m.buyer).num(2, m.transfer_id)),
  },
  MsgAttestTransfer: {
    // attestor=1, transfer_id=2, evidence_hash=3
    signer: 'attestor',
    encode: (m) => write((w) => w
      .string(1, m.attestor).num(2, m.transfer_id).string(3, m.evidence_hash)),
  },
  MsgObjectTransfer: {
    // objector=1, transfer_id=2, reason=3
    //
    // The one message here any visitor can send, deliberately: the person best
    // placed to recognise a stolen animal is rarely a party to the sale.
    signer: 'objector',
    encode: (m) => write((w) => w
      .string(1, m.objector).num(2, m.transfer_id).string(3, m.reason)),
  },
  MsgCompleteTransfer: {
    // creator=1, transfer_id=2
    signer: 'creator',
    encode: (m) => write((w) => w.string(1, m.creator).num(2, m.transfer_id)),
  },
  MsgCancelTransfer: {
    // seller=1, transfer_id=2, reason=3
    signer: 'seller',
    encode: (m) => write((w) => w
      .string(1, m.seller).num(2, m.transfer_id).string(3, m.reason)),
  },
  MsgResolveObjection: {
    // authority=1, transfer_id=2, uphold=3, reason=4
    signer: 'authority',
    encode: (m) => write((w) => w
      .string(1, m.authority).num(2, m.transfer_id).bool(3, m.uphold)
      .string(4, m.reason)),
  },
  MsgRetireAnimal: {
    // creator=1, animal_id=2, reason=3
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).num(2, m.animal_id).string(3, m.reason)),
  },
};

/** One `google.protobuf.Any`: type_url=1, value=2. */
export const any = (typeUrl, value) =>
  write((w) => w.string(1, typeUrl).bytesField(2, value));

/** A herd message packed into an Any, ready for a Tx or a group proposal. */
export function herdAny(name, fields) {
  const spec = MSG[name];
  if (!spec) throw new Error(`no such herd message: ${name}`);
  return any(TYPE(name), spec.encode(fields));
}

/* ============================================================================
   x/group — the office's half.

   A registry office is a group account, so no browser can produce its
   signature. What a browser CAN do is submit a proposal to the office's policy
   in the registrar's own name, which is a different act by a different signer.
   See registrar.js for why that distinction is the whole design.
   ============================================================================ */

/** MsgSubmitProposal — cosmos/group/v1/tx.proto:
 *  group_policy_address=1, proposers=2, metadata=3, messages=4, exec=5. */
export const GROUP_SUBMIT_PROPOSAL = '/cosmos.group.v1.MsgSubmitProposal';

export const groupSubmitProposal = ({ policy, proposers, metadata, messages }) =>
  write((w) => {
    w.string(1, policy);
    for (const p of proposers) w.string(2, p);
    w.string(3, metadata);
    for (const m of messages) w.bytesField(4, m);
    // exec is left unset: EXEC_TRY would run the proposal immediately if the
    // proposer's own vote already met the threshold, which on a 1-of-N policy
    // turns "submit a proposal" into "act as the office" without anybody
    // seeing a proposal. The office's M-of-N is the point.
  });

/* ============================================================================
   The transaction envelope — cosmos/tx/v1beta1/tx.proto.
   ============================================================================ */

/** TxBody: messages=1, memo=2, timeout_height=3. */
export const txBody = (messages, memo = '') =>
  write((w) => {
    for (const m of messages) w.bytesField(1, m);
    w.string(2, memo);
  });

/**
 * AuthInfo for exactly one signer, signing in SIGN_MODE_DIRECT.
 *
 * AuthInfo: signer_infos=1, fee=2.
 * SignerInfo: public_key=1, mode_info=2, sequence=3.
 * ModeInfo: single=1. ModeInfo.Single: mode=1. SIGN_MODE_DIRECT = 1.
 * Fee: amount=1, gas_limit=2, payer=3, granter=4.
 *
 * `fee` is empty by default and that is a decision, not an omission. The
 * message this console most wants a stranger to be able to send is an
 * objection, and the person best placed to object to a fraudulent sale of
 * somebody's land is the family member who owns no tokens at all. This chain's
 * nodes run `minimum-gas-prices = "0uyml"`, so a fee-less transaction is
 * accepted; if a network ever starts charging, the caller passes coins here and
 * the page has to say who pays. Silently attaching a fee an objector cannot
 * afford would make the objection fail at CheckTx with a message about gas.
 */
export const authInfo = ({ pubkey, sequence, gasLimit, fee = [], granter = '' }) =>
  write((w) => {
    w.msg(1, (s) => {
      s.bytesField(1, any('/cosmos.crypto.secp256k1.PubKey',
        write((k) => k.bytesField(1, pubkey))));
      s.msg(2, (mi) => mi.msg(1, (single) => single.num(1, 1)));
      s.num(3, sequence);
    });
    w.msg(2, (f) => {
      for (const coin of fee) f.msg(1, (c) => c.string(1, coin.denom).string(2, coin.amount));
      f.num(2, gasLimit);
      f.string(4, granter);
    });
  });

/** TxRaw: body_bytes=1, auth_info_bytes=2, signatures=3. */
export const txRaw = (bodyBytes, authInfoBytes, signatures) =>
  write((w) => {
    w.bytesField(1, bodyBytes);
    w.bytesField(2, authInfoBytes);
    for (const s of signatures) w.bytesField(3, s);
  });

/* ------------------------------------------------- who is signing, and where */

/** cosmos.auth.v1beta1.QueryAccountRequest: address=1. */
export const ACCOUNT_QUERY = '/cosmos.auth.v1beta1.Query/Account';
export const accountRequest = (address) => write((w) => w.string(1, address));

/**
 * The account number and sequence a signature is bound to.
 *
 * QueryAccountResponse wraps a `google.protobuf.Any`, and what is inside it
 * depends on the account: a BaseAccount, or a vesting or module account that
 * *embeds* one. Every such wrapper puts its BaseAccount at field 1, so the
 * unwrap below is one level deep and then reads BaseAccount's own fields —
 * address=1, pub_key=2, account_number=3, sequence=4.
 *
 * Returning null rather than zeroes for an account the chain has never seen is
 * load-bearing: a never-funded account answers `key not found`, and signing
 * with account_number 0 and sequence 0 produces a signature the chain rejects
 * with a message about the signature, not about the empty account. The caller
 * says "this account has never held anything, so it cannot pay a fee or sign"
 * instead.
 */
export function decodeAccount(bytes) {
  const outer = read(bytes);
  const anyBytes = first(outer, 1);
  if (anyBytes === undefined) return null;
  const wrapper = read(anyBytes);
  const typeUrl = str(wrapper, 1);
  let account = read(bytes_(wrapper, 2));

  // A vesting or module account carries its BaseAccount at field 1; a
  // BaseAccount carries its own address there. Telling them apart by whether
  // field 1 parses as a nested message is guesswork, so the type URL decides.
  if (!typeUrl.endsWith('.BaseAccount')) {
    const inner = first(account, 1);
    if (inner !== undefined) account = read(inner);
  }
  return {
    address: str(account, 1),
    account_number: u64(account, 3),
    sequence: u64(account, 4),
  };
}

const bytes_ = (f, num) => first(f, num) ?? new Uint8Array();
