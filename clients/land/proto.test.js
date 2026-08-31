// Tests for the hand-written protobuf this console reads the register with.
//
// The reason these exist is that nothing else checks them. A generated codec is
// wrong only if the generator is; a hand-written one is wrong whenever somebody
// transcribes a field number off the .proto and slips a digit — and the symptom
// of that is not a crash. It is a title rendered with the holder's account in
// the jurisdiction field, which reads as a strange-looking record rather than
// as a bug, on the one page whose entire job is to be believed.
//
// So the tests below are mostly about the failures that look like data:
// round-trips through every land codec, a real byte string off the live chain,
// proto3 default omission, and the two id fields whose zero means opposite
// things.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  ACCOUNT_QUERY,
  GROUP_SUBMIT_PROPOSAL,
  MSG,
  QUERY,
  STATUS,
  TYPE,
  accountRequest,
  any,
  authInfo,
  bool,
  decodeAccount,
  decodeAuthority,
  decodeParams,
  decodeParcel,
  decodeTransfer,
  fromBase64,
  groupSubmitProposal,
  landAny,
  read,
  str,
  strs,
  sub,
  subs,
  toBase64,
  toHex,
  txBody,
  txRaw,
  u32,
  u64,
  write,
} from './proto.js';

/* ------------------------------------------------------------- primitives */

test('a varint round-trips through every boundary width', () => {
  for (const n of [0, 1, 127, 128, 300, 16383, 16384, 2 ** 31, 2 ** 53 - 1]) {
    const f = read(write((w) => w.num(1, n === 0 ? 1 : n)));
    assert.equal(u64(f, 1), String(n === 0 ? 1 : n), `varint ${n}`);
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

test('an absent field reads as its proto3 default, never as undefined', () => {
  const f = read(new Uint8Array());
  assert.equal(str(f, 1), '');
  assert.equal(u64(f, 1), '0');
  assert.equal(u32(f, 1), 0);
  assert.equal(bool(f, 1), false);
  assert.deepEqual(strs(f, 1), []);
  assert.deepEqual(subs(f, 1, () => ({})), []);
  assert.equal(sub(f, 1, () => ({})), null);
});

test('a repeated field keeps every entry, not the last one', () => {
  // The failure this rules out: a parcel carrying twelve encumbrances rendered
  // as a title carrying one.
  const bytes = write((w) => { w.string(8, 'a'); w.string(8, 'b'); w.string(8, 'c'); });
  assert.deepEqual(strs(read(bytes), 8), ['a', 'b', 'c']);
});

test('a field this build has never heard of is ignored, not fatal', () => {
  // A node running newer code than this page emits fields it does not know.
  // Refusing to parse the parcel around them would take the register offline
  // for every reader on the day the chain upgrades.
  const bytes = write((w) => { w.num(1, 7); w.string(99, 'from a later build'); });
  const f = read(bytes);
  assert.equal(u64(f, 1), '7');
});

test('a truncated message is refused rather than half-decoded', () => {
  const full = write((w) => w.string(1, 'a cadastral reference'));
  assert.throws(() => read(full.subarray(0, full.length - 4)));
});

test('base64 and hex round-trip the bytes ABCI and the RPC actually exchange', () => {
  const bytes = Uint8Array.from([0, 1, 127, 128, 255, 10, 13]);
  assert.deepEqual(fromBase64(toBase64(bytes)), bytes);
  assert.equal(toHex(bytes), '00017f80ff0a0d');
});

/* ------------------------------------------- against bytes from the chain */

test('Params decodes the exact bytes yamale-devnet-2 answered with', () => {
  // Captured 2026-08-27 at height 94,464:
  //   GET /api/rpc/abci_query?path="/blockchain.land.v1.Query/Params"&data=0x
  //   → {"response":{"code":0,"value":"CgYIAxCA6kk=","height":"94464"}}
  //
  // A fixture off the live chain rather than one this file made up. A codec
  // tested only against its own writer agrees with itself and with nothing
  // else, which is the exact way a hand-written decoder passes its tests and
  // still shows the wrong quorum.
  const { params } = QUERY.Params.response(read(fromBase64('CgYIAxCA6kk=')));
  assert.equal(params.attestation_quorum, '3');
  assert.equal(params.challenge_window, '1209600');       // fourteen days
  assert.equal(params.same_authority_attestation, false);
});

test('an empty ABCI value is an empty list, not a failure', () => {
  // Also measured: Authorities answered `"value":""` — zero registry offices.
  // "The register holds none" and "the register did not answer" are opposite
  // facts, and this is the decode half of keeping them apart.
  const { authorities } = QUERY.Authorities.response(read(fromBase64('')));
  assert.deepEqual(authorities, []);
});

/* ---------------------------------------------------------- land codecs */

test('a parcel round-trips with every repeated field populated', () => {
  const bytes = write((w) => {
    w.num(1, 7).string(2, '9f2b1ce04a').string(3, 'YM-KIN-2024-00187')
      .string(4, 'yml1holder').string(5, 'yml1office').num(6, 4)
      .msg(7, (e) => e.string(1, 'mortgage').string(2, 'yml1bank')
        .string(3, '12,000 USD').num(4, 900).bool(5, false))
      .msg(7, (e) => e.string(1, 'lien').string(2, 'yml1other').num(4, 950).bool(5, true))
      .num(8, 812)
      .msg(9, (d) => d.string(1, 'inheritance').string(2, '4c1f9ab2')
        .string(3, 'https://registry.example/deeds/4c1f').string(4, 'ACT-1974-221')
        .string(5, '1974-06-02').num(6, 813))
      .msg(10, (r) => r.string(1, 'heritage_protected').string(2, '').string(3, 'listed 1962')
        .string(4, 'yml1office').num(5, 880).bool(6, false))
      .num(11, 3)
      .msg(12, (fz) => fz.string(1, 'court order 2026/114').string(2, 'yml1office')
        .num(3, 1200).bool(4, false));
  });
  const p = decodeParcel(read(bytes));

  assert.equal(p.id, '7');
  assert.equal(p.cadastral_ref, 'YM-KIN-2024-00187');
  assert.equal(p.status, 'STATUS_FROZEN');
  assert.equal(p.encumbrances.length, 2);
  assert.equal(p.encumbrances[0].kind, 'mortgage');
  assert.equal(p.encumbrances[0].released, false);
  assert.equal(p.encumbrances[1].released, true);
  assert.equal(p.deeds[0].reference, 'ACT-1974-221');
  assert.equal(p.restrictions[0].kind, 'heritage_protected');
  assert.equal(p.restrictions[0].value, '');
  assert.equal(p.vehicle_id, '3');
  assert.equal(p.freezes[0].reason, 'court order 2026/114');
  assert.equal(p.freezes[0].lifted, false);
});

test('a parcel status is the name the page compares against, not a number', () => {
  // Every verdict on this console is written against 'STATUS_FROZEN' and its
  // siblings — the strings the REST gateway used to emit. Returning the enum
  // as a number would leave those comparisons silently false, and a frozen
  // title would render as clear.
  for (let i = 0; i < STATUS.length; i += 1) {
    assert.equal(decodeParcel(read(write((w) => w.num(6, i)))).status, STATUS[i]);
  }
  // An enum value from a later build must not become `undefined`.
  assert.equal(decodeParcel(read(write((w) => w.num(6, 99)))).status, 'STATUS_UNSPECIFIED');
});

test('transfer 0 is a real transfer and decodes as a truthy id', () => {
  // x/land/keeper/msg_server_parcel.go steps past parcel 0 deliberately;
  // msg_server_transfer.go does not, so the first transfer this registry ever
  // records has id 0. Decoded as a Number it would be falsy and every
  // `if (t.id)` on the page would decide it does not exist.
  const t = decodeTransfer(read(write((w) => w.string(3, 'yml1seller').string(4, 'yml1buyer'))));
  assert.equal(t.id, '0');
  assert.ok(t.id, 'transfer 0 must not be falsy');
  assert.equal(t.completed_at, '0');
});

test('a transfer keeps its attestors in order and its objection intact', () => {
  const bytes = write((w) => {
    w.num(1, 12).num(2, 7).string(3, 'yml1seller').string(4, 'yml1buyer')
      .string(5, '4200 USD').bool(6, true).string(7, 'yml1office')
      .string(8, 'yml1a').string(8, 'yml1b').string(8, 'yml1c')
      .num(9, 1790000000).string(10, 'yml1daughter')
      .string(11, 'the seller died in 2019')
      .num(12, 900).num(13, 0);
  });
  const t = decodeTransfer(read(bytes));
  assert.deepEqual(t.attestors, ['yml1a', 'yml1b', 'yml1c']);
  assert.equal(t.quorum_at, '1790000000');
  assert.equal(t.objection_reason, 'the seller died in 2019');
  assert.equal(t.completed_at, '0');
});

test('an authority round-trips, including an inactive one', () => {
  const a = decodeAuthority(read(write((w) => w
    .string(1, 'yml1office').string(2, 'Kinshasa Lands Office').string(3, 'CD').bool(4, false))));
  assert.deepEqual(a, {
    address: 'yml1office', name: 'Kinshasa Lands Office', jurisdiction: 'CD', active: false,
  });
});

/* -------------------------------------------------------- query requests */

test('a query for parcel 7 asks for 7, and a query for transfer 0 asks for nothing', () => {
  assert.deepEqual([...QUERY.Parcel.request(7)], [0x08, 0x07]);
  // Field 1 omitted, because a proto3 zero IS the empty message. The keeper
  // reads the absent field as 0 and answers for transfer 0 — which is why the
  // console must not refuse a "0" typed into the transfer box.
  assert.equal(QUERY.Transfer.request(0).length, 0);
});

test('a cadastral reference with slashes survives the request encoding', () => {
  // The reason this query moved into the request body at all: every real
  // reference contains slashes, and a REST route bound it to one path segment.
  const bytes = QUERY.ParcelByRef.request('ACC/GA/2019/00412');
  assert.equal(str(read(bytes), 1), 'ACC/GA/2019/00412');
});

/* ------------------------------------------------------ land tx messages */

test('the type URL prefix is blockchain, not yamale.blockchain', () => {
  // This has cost time twice. The Go import path is yamale/blockchain/x/land,
  // the proto package is blockchain.land.v1, and only the second appears on
  // the wire. The wrong one produces `unable to resolve type URL`, which reads
  // as a chain fault and is a typo.
  assert.equal(TYPE('MsgRegisterParcel'), '/blockchain.land.v1.MsgRegisterParcel');
  assert.ok(!TYPE('MsgObject').includes('yamale'));
});

test('every message x/land declares has a codec here', () => {
  // Twelve, and the console has to cover all of them in some usable form.
  // MsgUpdateParams is the thirteenth and is not a land action at all — it is
  // governance changing the quorum — so it is deliberately absent.
  assert.deepEqual(Object.keys(MSG).sort(), [
    'MsgAttachDeed', 'MsgAttestTransfer', 'MsgAuthoriseFractionalisation',
    'MsgCompleteTransfer', 'MsgFreezeParcel', 'MsgObject', 'MsgProposeTransfer',
    'MsgRecordEncumbrance', 'MsgRegisterAuthority', 'MsgRegisterParcel',
    'MsgSetRestriction', 'MsgValidateTransfer',
  ]);
});

test('an objection encodes its reason, and an objection to transfer 0 still names a signer', () => {
  const bytes = MSG.MsgObject.encode({
    creator: 'yml1daughter', transfer_id: 0, reason: 'succession HC/PR/2024/118',
  });
  const f = read(bytes);
  assert.equal(str(f, 1), 'yml1daughter');
  assert.equal(u64(f, 2), '0');
  assert.equal(str(f, 3), 'succession HC/PR/2024/118');
});

test('a fractionalisation authorisation keeps its ceiling and expiry', () => {
  const f = read(MSG.MsgAuthoriseFractionalisation.encode({
    creator: 'yml1office', parcel_id: 7, right: 'exploitation',
    max_share_bps: 6000, expires_at: 1790000000, withdraw: false,
  }));
  assert.equal(u32(f, 4), 6000);
  assert.equal(u64(f, 5), '1790000000');
  assert.equal(bool(f, 6), false);
});

test('a release names its index, and index 0 is a real entry', () => {
  // `query land parcel` numbers entries from zero, so releasing the first
  // encumbrance means index 0 — which proto3 omits. The keeper reads the
  // absent field as 0, so this is correct; the test is here because it looks
  // wrong and somebody will one day "fix" it.
  const f = read(MSG.MsgRecordEncumbrance.encode({
    creator: 'yml1office', parcel_id: 7, release: true, index: 0,
  }));
  assert.equal(bool(f, 6), true);
  assert.equal(u32(f, 7), 0);
});

test('an Any carries the type URL and the message body', () => {
  const packed = landAny('MsgCompleteTransfer', { creator: 'yml1anyone', transfer_id: 12 });
  const f = read(packed);
  assert.equal(str(f, 1), '/blockchain.land.v1.MsgCompleteTransfer');
  const inner = read(f.get(2)[0]);
  assert.equal(str(inner, 1), 'yml1anyone');
  assert.equal(u64(inner, 2), '12');
});

test('landAny refuses a message name it does not know', () => {
  assert.throws(() => landAny('MsgSeizeLand', {}), /no such land message/);
});

/* ------------------------------------------------------- group proposals */

test('a group proposal carries the office policy, the proposer and the land message', () => {
  const inner = landAny('MsgValidateTransfer',
    { creator: 'yml1office', transfer_id: 12 });
  const f = read(groupSubmitProposal({
    policy: 'yml1office', proposers: ['yml1registrar'],
    metadata: 'Validate transfer 12', messages: [inner],
  }));
  assert.equal(str(f, 1), 'yml1office');
  assert.deepEqual(strs(f, 2), ['yml1registrar']);
  assert.equal(str(f, 3), 'Validate transfer 12');
  assert.equal(read(f.get(4)[0]) && str(read(f.get(4)[0]), 1),
    '/blockchain.land.v1.MsgValidateTransfer');
});

test('a group proposal never sets exec', () => {
  // EXEC_TRY runs the proposal in the same block if the proposer's own vote
  // already meets the threshold. On a 1-of-N policy that turns "submit a
  // proposal" into "act as the office" with no proposal anybody could read,
  // which is precisely the M-of-N this module refuses to work without.
  const f = read(groupSubmitProposal({
    policy: 'yml1office', proposers: ['yml1registrar'], metadata: '', messages: [],
  }));
  assert.equal(f.has(5), false);
});

test('the group message type URL is the SDK one', () => {
  assert.equal(GROUP_SUBMIT_PROPOSAL, '/cosmos.group.v1.MsgSubmitProposal');
});

/* ---------------------------------------------------- the tx envelope */

test('a transaction body carries its messages and its memo', () => {
  const body = txBody([landAny('MsgObject',
    { creator: 'yml1x', transfer_id: 3, reason: 'disputed' })], 'objection');
  const f = read(body);
  assert.equal(f.get(1).length, 1);
  assert.equal(str(f, 2), 'objection');
});

test('AuthInfo signs in SIGN_MODE_DIRECT with no fee by default', () => {
  // The default matters: the person best placed to object to a fraudulent sale
  // is the family member who holds no tokens. This chain runs
  // minimum-gas-prices = "0uyml", so a fee-less transaction is accepted.
  const f = read(authInfo({
    pubkey: Uint8Array.from([2, 1, 2, 3]), sequence: 4, gasLimit: 250000,
  }));
  const signer = read(f.get(1)[0]);
  assert.equal(str(read(signer.get(1)[0]), 1), '/cosmos.crypto.secp256k1.PubKey');
  const modeInfo = read(signer.get(2)[0]);
  assert.equal(u32(read(modeInfo.get(1)[0]), 1), 1, 'SIGN_MODE_DIRECT');
  assert.equal(u64(signer, 3), '4');

  const fee = read(f.get(2)[0]);
  assert.equal(fee.has(1), false, 'no coins');
  assert.equal(u64(fee, 2), '250000');
});

test('AuthInfo carries coins and a granter when one is given', () => {
  const fee = read(read(authInfo({
    pubkey: Uint8Array.from([2]), sequence: 0, gasLimit: 200000,
    fee: [{ denom: 'uyml', amount: '2000' }], granter: 'yml1sponsor',
  })).get(2)[0]);
  const coin = read(fee.get(1)[0]);
  assert.equal(str(coin, 1), 'uyml');
  assert.equal(str(coin, 2), '2000');
  assert.equal(str(fee, 4), 'yml1sponsor');
});

test('a sequence of zero is omitted, which is what a fresh account has', () => {
  // A never-used account signs at sequence 0, and proto3 omits it. Writing an
  // explicit zero here would produce different bytes from the ones the chain
  // reconstructs to verify the signature.
  const signer = read(read(authInfo({
    pubkey: Uint8Array.from([2]), sequence: 0, gasLimit: 1,
  })).get(1)[0]);
  assert.equal(signer.has(3), false);
});

test('TxRaw carries exactly the bytes that were signed', () => {
  const body = Uint8Array.from([1, 2, 3]);
  const auth = Uint8Array.from([4, 5]);
  const sig = Uint8Array.from(new Array(64).fill(9));
  const f = read(txRaw(body, auth, [sig]));
  assert.deepEqual(f.get(1)[0], body);
  assert.deepEqual(f.get(2)[0], auth);
  assert.equal(f.get(3)[0].length, 64);
});

/* -------------------------------------------------------- who is signing */

test('an account request names the address, and the path is the SDK one', () => {
  assert.equal(ACCOUNT_QUERY, '/cosmos.auth.v1beta1.Query/Account');
  assert.equal(str(read(accountRequest('yml1abc')), 1), 'yml1abc');
});

test('a BaseAccount yields its number and sequence', () => {
  const base = write((w) => w.string(1, 'yml1abc').num(3, 41).num(4, 7));
  const response = write((w) => w.bytesField(1,
    any('/cosmos.auth.v1beta1.BaseAccount', base)));
  assert.deepEqual(decodeAccount(response),
    { address: 'yml1abc', account_number: '41', sequence: '7' });
});

test('an account that embeds a BaseAccount is unwrapped one level', () => {
  // Vesting and module accounts put their BaseAccount at field 1. Reading
  // their own field 3 instead would return whatever that account type happens
  // to keep there, and sign against a number the chain never issued.
  const base = write((w) => w.string(1, 'yml1vest').num(3, 12).num(4, 3));
  const vesting = write((w) => w.bytesField(1, base).num(2, 500));
  const response = write((w) => w.bytesField(1,
    any('/cosmos.vesting.v1beta1.ContinuousVestingAccount', vesting)));
  assert.deepEqual(decodeAccount(response),
    { address: 'yml1vest', account_number: '12', sequence: '3' });
});

test('an account the chain has never seen decodes as null, not as zeroes', () => {
  // Signing with account_number 0 and sequence 0 against an account that does
  // not exist produces a rejection about the signature rather than about the
  // empty account, and the person reading it concludes the console is broken.
  assert.equal(decodeAccount(new Uint8Array()), null);
});
