import assert from 'node:assert/strict';
import { test } from 'node:test';
import { secp256k1 } from '@noble/curves/secp256k1';

import { addressOfPublicKey } from '../../../sdk/src/jewel.ts';
import { cardKeyOf, memberKey, openCard, plan, reconcile, sealCard, type VaultRead } from './vault.ts';

const b64 = (b: Uint8Array) => btoa(String.fromCharCode(...b));
function account(seed: number) {
  const priv = new Uint8Array(32).fill(seed);
  const pub = secp256k1.getPublicKey(priv, true);
  return { priv, pub, address: addressOfPublicKey(pub), member: { address: addressOfPublicKey(pub), key: b64(pub) } };
}
const card = (seed: number) => new Uint8Array(32).fill(seed);

const shop = account(1);
const staff = account(2);
const stranger = account(3);

test('a sealed card opens with the member key it was sealed for, and nothing else', () => {
  const secret = card(9);
  const box = sealCard(secret, shop.pub, '7', shop.address);
  assert.equal(atob(box).length, 93);
  assert.deepEqual(openCard(box, shop.priv, '7', shop.address), secret);
  assert.equal(openCard(box, staff.priv, '7', staff.address), null);
  // Bound to the piece and the member: moved to another piece it does not open.
  assert.equal(openCard(box, shop.priv, '8', shop.address), null);
  assert.equal(openCard('not a box', shop.priv, '7', shop.address), null);
});

test('a member key is used only if it is the key of the address it is listed for', () => {
  assert.deepEqual(memberKey(shop.member), shop.pub);
  assert.equal(memberKey({ address: shop.address, key: stranger.member.key }), null);
  assert.equal(memberKey({ address: shop.address, key: 'AAAA' }), null);
});

test('a card from the vault is kept only if it opens to its recorded key', () => {
  const secret = card(9);
  const read: VaultRead = { members: [], cards: { '7': { card_key: cardKeyOf(secret), box: 'x', holders: [] } } };
  const good = reconcile(read, ['7'], () => null, () => secret);
  assert.deepEqual(good.get('7'), secret);
  // A box that opens to some other secret is refused.
  const forged = reconcile(read, ['7'], () => null, () => card(5));
  assert.equal(forged.has('7'), false);
});

test('a newer card in the vault replaces the one kept here, unless this one was just issued', () => {
  const old = card(4);
  const newer = card(9);
  const read: VaultRead = { members: [], cards: { '7': { card_key: cardKeyOf(newer), box: 'x', holders: [] } } };
  assert.deepEqual(reconcile(read, ['7'], () => old, () => newer).get('7'), newer);
  assert.deepEqual(reconcile(read, ['7'], () => old, () => newer, new Set(['7'])).get('7'), old);
  // Kept here, vault holds a different card this member cannot open: neither is trusted.
  const sealedAway: VaultRead = { members: [], cards: { '7': { card_key: cardKeyOf(newer), holders: [] } } };
  assert.equal(reconcile(sealedAway, ['7'], () => old, () => null).has('7'), false);
});

test('a card kept here and absent from the vault is uploaded for every member whose key checks out', () => {
  const secret = card(9);
  const read: VaultRead = {
    members: [shop.member, staff.member, { address: stranger.address, key: shop.member.key }],
    cards: {},
  };
  const uploads = plan(read, new Map([['7', secret]]), sealCard);
  assert.equal(uploads.length, 1);
  assert.equal(uploads[0].body.card_key, cardKeyOf(secret));
  assert.deepEqual(Object.keys(uploads[0].body.boxes).sort(), [shop.address, staff.address].sort());
  assert.deepEqual(openCard(uploads[0].body.boxes[staff.address], staff.priv, '7', staff.address), secret);
});

test('only members who lack the card are sealed for, and nothing is sent when all hold it', () => {
  const secret = card(9);
  const key = cardKeyOf(secret);
  const some: VaultRead = { members: [shop.member, staff.member], cards: { '7': { card_key: key, holders: [staff.address] } } };
  const uploads = plan(some, new Map([['7', secret]]), sealCard);
  assert.deepEqual(Object.keys(uploads[0].body.boxes), [shop.address]);
  const all: VaultRead = { members: [shop.member, staff.member], cards: { '7': { card_key: key, holders: [staff.address, shop.address] } } };
  assert.deepEqual(plan(all, new Map([['7', secret]]), sealCard), []);
});

test('a card issued anew replaces the old record for everyone', () => {
  const fresh = card(9);
  const read: VaultRead = { members: [shop.member, staff.member], cards: { '7': { card_key: cardKeyOf(card(4)), holders: [shop.address, staff.address] } } };
  const uploads = plan(read, new Map([['7', fresh]]), sealCard);
  assert.equal(uploads[0].body.card_key, cardKeyOf(fresh));
  assert.equal(Object.keys(uploads[0].body.boxes).length, 2);
});
