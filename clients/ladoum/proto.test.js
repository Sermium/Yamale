// Tests for the hand-written protobuf this console reads the register with.
//
// The reason these exist is that nothing else checks them. A generated codec is
// wrong only if the generator is; a hand-written one is wrong whenever somebody
// transcribes a field number off the .proto and slips a digit — and the symptom
// is not a crash. It is a certificate rendered with the sire in the dam's
// field, which reads as a strange-looking record rather than as a bug, on the
// one page whose entire job is to be believed.
//
// These codecs were rewritten wholesale when this console moved from x/land to
// x/herd, which is the single most dangerous thing that can happen to a file
// like this: the shapes are similar enough that a stale decoder still returns
// an object with every field populated. The first test below is about exactly
// that.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  LEVEL,
  MSG,
  QUERY,
  SEX,
  SPECIES,
  STATUS,
  TRANSFER_STATE,
  TYPE,
  any,
  bool,
  decodeAnimal,
  decodeAuthority,
  decodeHealthEvent,
  decodeHerd,
  decodeParams,
  decodeTransfer,
  herdAny,
  read,
  str,
  sub,
  subs,
  toHex,
  u32,
  u64,
  write,
} from './proto.js';

/* ------------------------------------------------------------- primitives */

test('a varint round-trips through every boundary width', () => {
  for (const n of [1, 127, 128, 300, 16383, 16384, 2 ** 31, 2 ** 53 - 1]) {
    const f = read(write((w) => w.num(1, n)));
    assert.equal(u64(f, 1), String(n), `varint ${n}`);
  }
});

test('proto3 defaults are omitted rather than written', () => {
  // Not tidiness. The chain re-serialises a message it stores, so a client that
  // writes an explicit zero produces bytes that differ from the ones the chain
  // would produce — which for anything hashed or compared is a mismatch nobody
  // can see by reading either side.
  assert.equal(write((w) => w.num(1, 0)).length, 0);
  assert.equal(write((w) => w.string(1, '')).length, 0);
  assert.equal(write((w) => w.bool(1, false)).length, 0);
  assert.equal(write((w) => w.num(1, 1)).length, 2);
});

/* ------------------------------------------------------ the migration trap */

test('the params decoder reads herd field order, not land field order', () => {
  // The one that would have shipped silently. x/land's Params put the quorum
  // first and the challenge window second; x/herd puts them the other way
  // round. Both are numbers, neither is absent, and carrying the old decoder
  // across throws nothing — it renders "604800 attestations required" and a
  // challenge window of two seconds, on the page whose job is to be believed.
  const bytes = write((w) => w
    .num(1, 604800)      // challenge_window
    .num(2, 2)           // attestation_quorum
    .bool(3, false)      // same_authority_attestation
    .num(4, 140)         // min_birth_interval_days
    .bool(5, true));     // require_dna_for_certified

  const p = decodeParams(read(bytes));
  assert.equal(p.challenge_window, '604800');
  assert.equal(p.attestation_quorum, '2');
  assert.equal(p.same_authority_attestation, false);
  assert.equal(p.min_birth_interval_days, '140');
  assert.equal(p.require_dna_for_certified, true);
});

/* ------------------------------------------------------------ every record */

test('an animal round-trips through all eighteen fields', () => {
  const bytes = write((w) => w
    .num(1, 3).num(2, 1)
    .string(3, '221/682/021029/L').string(4, '982000000000003')
    .num(5, 3)                                   // LEVEL_CERTIFIED
    .string(6, 'Classico').num(7, 1)             // SEX_MALE
    .string(8, 'blanc tacheté').string(9, '2026-08-25').string(10, 'single')
    .num(11, 1).num(12, 2)
    .string(13, 'sha256:adn').string(14, 'sha256:photo')
    .string(15, 'yml1holder').string(16, 'yml1office')
    .num(17, 1)                                  // STATUS_ACTIVE
    .num(18, 94464));

  const a = decodeAnimal(read(bytes));
  assert.equal(a.id, '3');
  assert.equal(a.herd_id, '1');
  assert.equal(a.national_id, '221/682/021029/L');
  assert.equal(a.tag, '982000000000003');
  assert.equal(a.level, 'LEVEL_CERTIFIED');
  assert.equal(a.name, 'Classico');
  assert.equal(a.sex, 'SEX_MALE');
  assert.equal(a.coat, 'blanc tacheté');
  assert.equal(a.born_on, '2026-08-25');
  assert.equal(a.birth_type, 'single');
  assert.equal(a.sire_id, '1');
  assert.equal(a.dam_id, '2');
  assert.equal(a.dna_hash, 'sha256:adn');
  assert.equal(a.photo_hash, 'sha256:photo');
  assert.equal(a.holder, 'yml1holder');
  assert.equal(a.authority, 'yml1office');
  assert.equal(a.status, 'STATUS_ACTIVE');
  assert.equal(a.registered_at, '94464');
});

test('unknown parentage decodes to the string zero, which is truthy', () => {
  // The whole reason u64 returns a string. A founder has no sire and no dam,
  // which is honest and common, and the keeper never issues animal 0 so that
  // zero can carry that meaning. As a Number, `if (animal.sire_id)` would be
  // false for a founder AND false for animal 1's parent — and the page would
  // quietly agree that no founder has ever been recorded.
  const founder = decodeAnimal(read(write((w) => w.num(1, 7))));
  assert.equal(founder.sire_id, '0');
  assert.equal(founder.dam_id, '0');
  assert.ok(founder.sire_id, 'the string zero must be truthy so the test is written out');
});

test('a herd round-trips, and its species and status survive as names', () => {
  const bytes = write((w) => w
    .num(1, 1).string(2, 'SN-DK-H-000147').string(3, 'yml1keeper').string(4, 'yml1office')
    .num(5, 2).num(6, 40).string(7, 'Niacoulrab').string(8, 'sha256:geo')
    .string(9, 'SN').num(10, 1).num(11, 94000));

  const h = decodeHerd(read(bytes));
  assert.equal(h.reference, 'SN-DK-H-000147');
  assert.equal(h.species, 'SPECIES_SHEEP');
  assert.equal(h.declared_size, '40');
  assert.equal(h.locality, 'Niacoulrab');
  assert.equal(h.jurisdiction, 'SN');
  assert.equal(h.status, 'STATUS_ACTIVE');
});

test('a health event round-trips', () => {
  const e = decodeHealthEvent(read(write((w) => w
    .num(1, 4).num(2, 1).string(3, 'PPR').string(4, 'PPR-2026-0431')
    .num(5, 38).string(6, 'yml1agent').num(7, 94500))));
  assert.equal(e.disease, 'PPR');
  assert.equal(e.batch, 'PPR-2026-0431');
  assert.equal(e.treated, '38');
});

test('a transfer round-trips, attestations and resolution included', () => {
  const attestation = (who) => write((w) => w
    .string(1, who).string(2, 'yml1office2').string(3, 'sha256:tag-read').num(4, 1788000000));

  const bytes = write((w) => {
    w.num(1, 1).num(2, 3).string(3, 'yml1seller').string(4, 'yml1buyer')
      .string(5, '150000').num(6, 6);            // TRANSFER_STATE_CANCELLED
    w.bytesField(7, attestation('yml1vetA'));
    w.bytesField(7, attestation('yml1vetB'));
    w.num(8, 1788600000)
      .string(9, 'yml1neighbour').string(10, 'volé en juillet')
      .num(11, 1787000000)
      .string(12, 'yml1office').string(13, 'plainte 2026/431 au dossier')
      .bool(14, true)
      .num(15, 1787100000);
  });

  const t = decodeTransfer(read(bytes));
  assert.equal(t.state, 'TRANSFER_STATE_CANCELLED');
  assert.equal(t.attestations.length, 2);
  assert.equal(t.attestations[0].attestor, 'yml1vetA');
  assert.equal(t.attestations[1].evidence_hash, 'sha256:tag-read');
  assert.equal(t.objected_by, 'yml1neighbour');
  assert.equal(t.resolved_by, 'yml1office');
  assert.equal(t.objection_upheld, true);
  assert.equal(t.accepted_at, '1787100000');
});

test('an authority round-trips', () => {
  const a = decodeAuthority(read(write((w) => w
    .string(1, 'yml1office').string(2, "Direction de l'Élevage").string(3, 'SN').bool(4, true))));
  assert.equal(a.name, "Direction de l'Élevage");
  assert.equal(a.jurisdiction, 'SN');
  assert.equal(a.active, true);
});

/* -------------------------------------------------------- the enum tables */

test('every enum table is dense and starts at its unspecified value', () => {
  // A hole in one of these tables becomes `undefined`, and the `??` fallback
  // then reports UNSPECIFIED — which on `status` means a frozen animal would
  // render as an ordinary one.
  for (const [name, table] of [['LEVEL', LEVEL], ['SPECIES', SPECIES], ['SEX', SEX],
    ['STATUS', STATUS], ['TRANSFER_STATE', TRANSFER_STATE]]) {
    assert.ok(table.length > 1, `${name} is empty`);
    assert.ok(table[0].endsWith('UNSPECIFIED'), `${name}[0] is not the unspecified value`);
    table.forEach((v, i) => assert.equal(typeof v, 'string', `${name}[${i}] is a hole`));
    assert.equal(new Set(table).size, table.length, `${name} repeats a name`);
  }
});

test('the status table covers retired, which x/land had no equivalent of', () => {
  // x/land's STATUS ran to FROZEN at 4 and stopped. x/herd adds RETIRED at 5,
  // so a table copied across would render every dead animal as ACTIVE by way
  // of the unspecified fallback — on a register whose pedigrees deliberately
  // keep referring to dead ancestors.
  assert.equal(STATUS[5], 'STATUS_RETIRED');
  assert.equal(TRANSFER_STATE[6], 'TRANSFER_STATE_CANCELLED');
});

/* ------------------------------------------------------------ the queries */

test('every query names a herd path and encodes its request', () => {
  for (const [name, q] of Object.entries(QUERY)) {
    assert.ok(q.path.startsWith('/blockchain.herd.v1.Query/'), `${name}: ${q.path}`);
    assert.equal(typeof q.request, 'function', `${name} has no request encoder`);
    assert.equal(typeof q.response, 'function', `${name} has no response decoder`);
  }
});

test('the pedigree request carries the animal and the depth', () => {
  const f = read(QUERY.Pedigree.request({ animalId: '3', depth: 4 }));
  assert.equal(u64(f, 1), '3');
  assert.equal(u32(f, 2), 4);
});

test('a truncated pedigree says so, because silence would read as a founder', () => {
  const node = (id, generation, path) => write((w) => w
    .bytesField(1, write((v) => v.num(1, id).string(3, `221/682/00000${id}/L`)))
    .num(2, generation).string(3, path));

  const bytes = write((w) => {
    w.bytesField(1, node(3, 0, ''));
    w.bytesField(1, node(1, 1, 'sire'));
    w.bool(2, true);
  });

  const res = QUERY.Pedigree.response(read(bytes));
  assert.equal(res.nodes.length, 2);
  assert.equal(res.nodes[1].path, 'sire');
  assert.equal(res.truncated, true);
});

/* ----------------------------------------------------------- the messages */

test('every message names a signer field it actually encodes', () => {
  // A signer field the encoder never writes produces a transaction the chain
  // rejects for an empty signer, which surfaces as an opaque failure rather
  // than as the typo it is.
  const probe = {
    creator: 'yml1a', buyer: 'yml1a', attestor: 'yml1a', objector: 'yml1a',
    seller: 'yml1a', authority: 'yml1a', agent: 'yml1a',
  };
  for (const [name, spec] of Object.entries(MSG)) {
    assert.ok(spec.signer in probe, `${name}: unknown signer field ${spec.signer}`);
    const bytes = spec.encode({ ...probe, [spec.signer]: 'yml1signer' });
    assert.ok(toHex(bytes).includes(toHex(new TextEncoder().encode('yml1signer'))),
      `${name} does not encode its own signer field`);
  }
});

test('a transfer proposal encodes in field order', () => {
  const f = read(MSG.MsgProposeTransfer.encode({
    creator: 'yml1seller', animal_id: 3, to: 'yml1buyer', price: '150000',
  }));
  assert.equal(str(f, 1), 'yml1seller');
  assert.equal(u64(f, 2), '3');
  assert.equal(str(f, 3), 'yml1buyer');
  assert.equal(str(f, 4), '150000');
});

test('certifying with unknown parentage writes no parent fields at all', () => {
  // Zero is the wire's own absence, and it must stay that way: the keeper reads
  // an unset sire_id as 0 and treats it as unknown parentage, which is the
  // correct meaning. Writing an explicit zero would be a different byte string
  // for the same fact.
  const bytes = MSG.MsgCertifyAnimal.encode({
    creator: 'yml1office', animal_id: 5, sire_id: 0, dam_id: 0,
    dna_hash: 'sha256:adn', photo_hash: '',
  });
  const f = read(bytes);
  assert.equal(u64(f, 3), '0');
  assert.equal(u64(f, 4), '0');
  assert.equal(str(f, 5), 'sha256:adn');
  assert.equal(str(f, 6), '');
});

test('an objection carries its reason, which is the whole record of it', () => {
  const f = read(MSG.MsgObjectTransfer.encode({
    objector: 'yml1neighbour', transfer_id: 1, reason: 'ce bélier est le mien',
  }));
  assert.equal(str(f, 1), 'yml1neighbour');
  assert.equal(str(f, 3), 'ce bélier est le mien');
});

test('a herd message packs into an Any under the wire package name', () => {
  // `blockchain.herd.v1`, not `yamale.blockchain.herd.v1`. The Go module path
  // is not the proto package, and only the proto package appears on the wire.
  const f = read(herdAny('MsgAcceptTransfer', { buyer: 'yml1buyer', transfer_id: 1 }));
  assert.equal(str(f, 1), '/blockchain.herd.v1.MsgAcceptTransfer');
  assert.equal(TYPE('MsgAcceptTransfer'), '/blockchain.herd.v1.MsgAcceptTransfer');

  // `sub` already applies `read`, so it hands back a field map, not bytes.
  const inner = sub(f, 2, (x) => x);
  assert.equal(str(inner, 1), 'yml1buyer');
});

test('an unknown message name fails loudly rather than encoding nothing', () => {
  assert.throws(() => herdAny('MsgSellSheep', {}), /no such herd message/);
});

/* ------------------------------------------------- decoding what we encode */

test('a query response survives the full encode-decode loop', () => {
  // The end-to-end shape: build the bytes a node would return, and read them
  // back through the envelope the page actually calls.
  const animal = write((w) => w
    .num(1, 3).num(2, 1).string(3, '221/682/021029/L').num(5, 3).string(6, 'Classico').num(7, 1));
  const response = write((w) => w.bytesField(1, animal));

  const out = QUERY.AnimalByNationalID.response(read(response));
  assert.equal(out.animal.national_id, '221/682/021029/L');
  assert.equal(out.animal.name, 'Classico');
  assert.equal(out.animal.level, 'LEVEL_CERTIFIED');
});

test('an absent nested message decodes to null rather than an empty record', () => {
  // "The register has no such animal" and "the register returned a blank
  // animal" are opposite facts for somebody deciding whether to pay.
  assert.equal(sub(read(new Uint8Array()), 1, decodeAnimal), null);
  assert.deepEqual(subs(read(new Uint8Array()), 1, decodeAnimal), []);
  assert.equal(bool(read(new Uint8Array()), 2), false);
});
