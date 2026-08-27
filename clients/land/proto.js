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
// The whole of x/land's REST prefix sits behind the deployment's supervisor
// credential, so every read this console made returned a browser login box.
// The identical queries answer unauthenticated over the node's ABCI interface,
// which the proxy does publish — the same discovery clients/app/src/standing.ts
// records for x/paymsg. ABCI speaks protobuf and nothing else, so a register
// whose entire premise is that reading it is public could only be read by
// writing this.
//
// It is small on purpose. Nothing here is a general protobuf implementation:
// there are no groups, no packed repeated scalars (x/land has none), no maps,
// no float or fixed64 fields (x/land has none of those either). Every codec
// below names the .proto file and field numbers it was written from, and
// proto.test.js round-trips each one, so drift shows up as a failing test
// rather than as a title that renders with the holder in the wrong field.

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
      case 5: i += 4; break;   // fixed32 — none in x/land, skipped rather than refused
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
 * always. More immediately: the REST surface this console used to read renders
 * uint64 as a JSON string, so every comparison already written against these
 * values — `completed_at !== '0'` decides whether land has changed hands — is a
 * string comparison. Returning a Number here would make each of those silently
 * false.
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
   x/land — proto/blockchain/land/v1/parcel.proto and params.proto.

   Field numbers are transcribed from the .proto rather than from the generated
   Go, because the .proto is the source both of them come from. Each decoder
   below lists them so a reviewer can check this file against that one without
   holding a schema in their head.
   ============================================================================ */

/** Status enum — parcel.proto. 0 never appears on a stored parcel. */
export const STATUS = ['STATUS_UNSPECIFIED', 'STATUS_REGISTERED',
  'STATUS_TRANSFER_PENDING', 'STATUS_DISPUTED', 'STATUS_FROZEN'];

/** Params: attestation_quorum=1, challenge_window=2, same_authority_attestation=3. */
export const decodeParams = (f) => ({
  attestation_quorum: String(u32(f, 1)),
  challenge_window: u64(f, 2),
  same_authority_attestation: bool(f, 3),
});

/** Encumbrance: kind=1, holder=2, detail=3, recorded_at=4, released=5. */
const decodeEncumbrance = (f) => ({
  kind: str(f, 1), holder: str(f, 2), detail: str(f, 3),
  recorded_at: u64(f, 4), released: bool(f, 5),
});

/** Deed: kind=1, document_hash=2, uri=3, reference=4, issued_on=5, recorded_at=6. */
const decodeDeed = (f) => ({
  kind: str(f, 1), document_hash: str(f, 2), uri: str(f, 3),
  reference: str(f, 4), issued_on: str(f, 5), recorded_at: u64(f, 6),
});

/** Restriction: kind=1, value=2, detail=3, imposed_by=4, imposed_at=5, lifted=6. */
const decodeRestriction = (f) => ({
  kind: str(f, 1), value: str(f, 2), detail: str(f, 3),
  imposed_by: str(f, 4), imposed_at: u64(f, 5), lifted: bool(f, 6),
});

/** Freeze: reason=1, imposed_by=2, imposed_at=3, lifted=4, lifted_by=5,
 *  lift_reason=6, lifted_at=7. */
const decodeFreeze = (f) => ({
  reason: str(f, 1), imposed_by: str(f, 2), imposed_at: u64(f, 3),
  lifted: bool(f, 4), lifted_by: str(f, 5), lift_reason: str(f, 6),
  lifted_at: u64(f, 7),
});

/**
 * Parcel: id=1, geometry_hash=2, cadastral_ref=3, holder=4, authority=5,
 * status=6, encumbrances=7, registered_at=8, deeds=9, restrictions=10,
 * vehicle_id=11, freezes=12.
 *
 * `status` is turned back into its name because every existing comparison on
 * this page is against `'STATUS_FROZEN'` and the like — the strings the REST
 * gateway used to emit. Keeping the enum as a number here would mean rewriting
 * the verdict, and the verdict is the part of this console that must not move.
 */
export const decodeParcel = (f) => ({
  id: u64(f, 1),
  geometry_hash: str(f, 2),
  cadastral_ref: str(f, 3),
  holder: str(f, 4),
  authority: str(f, 5),
  status: STATUS[u32(f, 6)] ?? 'STATUS_UNSPECIFIED',
  encumbrances: subs(f, 7, decodeEncumbrance),
  registered_at: u64(f, 8),
  deeds: subs(f, 9, decodeDeed),
  restrictions: subs(f, 10, decodeRestriction),
  vehicle_id: u64(f, 11),
  freezes: subs(f, 12, decodeFreeze),
});

/**
 * Transfer: id=1, parcel_id=2, from=3, to=4, price=5, validated=6,
 * validated_by=7, attestors=8, quorum_at=9, objected_by=10,
 * objection_reason=11, proposed_at=12, completed_at=13.
 *
 * Note what `id` being a string buys here. Transfer ids come off
 * `collections.Sequence.Next()` with no zero skipped — unlike parcel ids, which
 * the keeper explicitly steps past — so **transfer 0 is a real transfer**, and
 * `u64` returns `'0'` for it, which is truthy. A Number would be falsy, and
 * every `if (transfer.id)` on the page would quietly decide the first transfer
 * this registry ever recorded does not exist.
 */
export const decodeTransfer = (f) => ({
  id: u64(f, 1),
  parcel_id: u64(f, 2),
  from: str(f, 3),
  to: str(f, 4),
  price: str(f, 5),
  validated: bool(f, 6),
  validated_by: str(f, 7),
  attestors: strs(f, 8),
  quorum_at: u64(f, 9),
  objected_by: str(f, 10),
  objection_reason: str(f, 11),
  proposed_at: u64(f, 12),
  completed_at: u64(f, 13),
});

/** Authority: address=1, name=2, jurisdiction=3, active=4. */
export const decodeAuthority = (f) => ({
  address: str(f, 1), name: str(f, 2), jurisdiction: str(f, 3), active: bool(f, 4),
});

/**
 * FractionalisationAuthority: parcel_id=1, right=2, max_share_bps=3,
 * expires_at=4, granted_by=5, granted_at=6, withdrawn=7, withdrawn_at=8.
 */
export const decodeFractionalisation = (f) => ({
  parcel_id: u64(f, 1), right: str(f, 2), max_share_bps: String(u32(f, 3)),
  expires_at: u64(f, 4), granted_by: str(f, 5), granted_at: u64(f, 6),
  withdrawn: bool(f, 7), withdrawn_at: u64(f, 8),
});

/* --------------------------------------------------------- query envelopes */
/* query.proto. Each response wraps its payload, so the wrapper is decoded here
   and the page never sees a field number. */

export const QUERY = {
  Params: {
    path: '/blockchain.land.v1.Query/Params',
    request: () => new Uint8Array(),
    response: (f) => ({ params: sub(f, 1, decodeParams) }),
  },
  Parcel: {
    path: '/blockchain.land.v1.Query/Parcel',
    request: (id) => write((w) => w.num(1, id)),
    response: (f) => ({ parcel: sub(f, 1, decodeParcel) }),
  },
  ParcelByRef: {
    path: '/blockchain.land.v1.Query/ParcelByRef',
    request: (ref) => write((w) => w.string(1, ref)),
    response: (f) => ({ parcel: sub(f, 1, decodeParcel) }),
  },
  ParcelByGeometry: {
    path: '/blockchain.land.v1.Query/ParcelByGeometry',
    request: (hash) => write((w) => w.string(1, hash)),
    response: (f) => ({ parcel: sub(f, 1, decodeParcel) }),
  },
  ParcelsByHolder: {
    path: '/blockchain.land.v1.Query/ParcelsByHolder',
    request: (holder) => write((w) => w.string(1, holder)),
    response: (f) => ({ parcels: subs(f, 1, decodeParcel) }),
  },
  Transfer: {
    path: '/blockchain.land.v1.Query/Transfer',
    // `num` omits a zero, which is exactly right: transfer 0 is real, and its
    // request is the empty message, because that is what a proto3 zero looks
    // like on the wire. The keeper reads an unset id as 0 and answers for it.
    request: (id) => write((w) => w.num(1, id)),
    response: (f) => ({ transfer: sub(f, 1, decodeTransfer) }),
  },
  TransfersByParcel: {
    path: '/blockchain.land.v1.Query/TransfersByParcel',
    request: (parcelId) => write((w) => w.num(1, parcelId)),
    response: (f) => ({ transfers: subs(f, 1, decodeTransfer) }),
  },
  PendingTransfers: {
    path: '/blockchain.land.v1.Query/PendingTransfers',
    request: () => new Uint8Array(),
    response: (f) => ({ transfers: subs(f, 1, decodeTransfer) }),
  },
  Authorities: {
    path: '/blockchain.land.v1.Query/Authorities',
    request: () => new Uint8Array(),
    response: (f) => ({ authorities: subs(f, 1, decodeAuthority) }),
  },
  FractionalisationAuthority: {
    path: '/blockchain.land.v1.Query/FractionalisationAuthority',
    request: (parcelId) => write((w) => w.num(1, parcelId)),
    response: (f) => ({
      authorisation: sub(f, 1, decodeFractionalisation),
      live: bool(f, 2),
    }),
  },
};

/* ============================================================================
   The messages this console can compose — proto/blockchain/land/v1/tx.proto.

   The type URL prefix is `blockchain.`, NOT `yamale.blockchain.`. The Go
   package path is `yamale/blockchain/x/land/types` and the proto package is
   `blockchain.land.v1`; only the second one appears on the wire. Getting this
   wrong produces `unable to resolve type URL`, which reads like a chain fault
   and is a typo. Verified against x/land/types/tx.pb.go.
   ============================================================================ */

export const TYPE = (name) => `/blockchain.land.v1.${name}`;

/**
 * Every land message, by the name autocli gives its subcommand.
 *
 * `encode` writes the protobuf; `signer` names the field the chain checks the
 * signature against (`cosmos.msg.v1.signer`), which is what decides who can
 * possibly send it and therefore which of the three shapes in registrar.js
 * this message takes.
 */
export const MSG = {
  MsgRegisterAuthority: {
    // authority=1, office=2, name=3, jurisdiction=4, active=5
    signer: 'authority',
    encode: (m) => write((w) => w
      .string(1, m.authority).string(2, m.office).string(3, m.name)
      .string(4, m.jurisdiction).bool(5, m.active)),
  },
  MsgRegisterParcel: {
    // creator=1, geometry_hash=2, cadastral_ref=3, holder=4
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).string(2, m.geometry_hash)
      .string(3, m.cadastral_ref).string(4, m.holder)),
  },
  MsgProposeTransfer: {
    // creator=1, parcel_id=2, to=3, price=4
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).num(2, m.parcel_id).string(3, m.to).string(4, m.price)),
  },
  MsgValidateTransfer: {
    // creator=1, transfer_id=2
    signer: 'creator',
    encode: (m) => write((w) => w.string(1, m.creator).num(2, m.transfer_id)),
  },
  MsgAttestTransfer: {
    // creator=1, transfer_id=2
    signer: 'creator',
    encode: (m) => write((w) => w.string(1, m.creator).num(2, m.transfer_id)),
  },
  MsgObject: {
    // creator=1, transfer_id=2, reason=3
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).num(2, m.transfer_id).string(3, m.reason)),
  },
  MsgCompleteTransfer: {
    // creator=1, transfer_id=2
    signer: 'creator',
    encode: (m) => write((w) => w.string(1, m.creator).num(2, m.transfer_id)),
  },
  MsgRecordEncumbrance: {
    // creator=1, parcel_id=2, kind=3, holder=4, detail=5, release=6, index=7
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).num(2, m.parcel_id).string(3, m.kind)
      .string(4, m.holder).string(5, m.detail).bool(6, m.release).num(7, m.index)),
  },
  MsgFreezeParcel: {
    // creator=1, parcel_id=2, reason=3, unfreeze=4
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).num(2, m.parcel_id).string(3, m.reason).bool(4, m.unfreeze)),
  },
  MsgAttachDeed: {
    // creator=1, parcel_id=2, kind=3, document_hash=4, uri=5, reference=6,
    // issued_on=7
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).num(2, m.parcel_id).string(3, m.kind)
      .string(4, m.document_hash).string(5, m.uri).string(6, m.reference)
      .string(7, m.issued_on)),
  },
  MsgSetRestriction: {
    // creator=1, parcel_id=2, kind=3, value=4, detail=5, lift=6, index=7
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).num(2, m.parcel_id).string(3, m.kind)
      .string(4, m.value).string(5, m.detail).bool(6, m.lift).num(7, m.index)),
  },
  MsgAuthoriseFractionalisation: {
    // creator=1, parcel_id=2, right=3, max_share_bps=4, expires_at=5, withdraw=6
    signer: 'creator',
    encode: (m) => write((w) => w
      .string(1, m.creator).num(2, m.parcel_id).string(3, m.right)
      .num(4, m.max_share_bps).num(5, m.expires_at).bool(6, m.withdraw)),
  },
};

/** One `google.protobuf.Any`: type_url=1, value=2. */
export const any = (typeUrl, value) =>
  write((w) => w.string(1, typeUrl).bytesField(2, value));

/** A land message packed into an Any, ready for a Tx or a group proposal. */
export function landAny(name, fields) {
  const spec = MSG[name];
  if (!spec) throw new Error(`no such land message: ${name}`);
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
