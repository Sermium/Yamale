import assert from 'node:assert/strict';
import { test } from 'node:test';

import { applicationProblem, canSaveLook, kindProblem, mayApplyLoad } from './rules.ts';

test('kind: trimmed, non-empty, at most 80 bytes (not characters)', () => {
  assert.equal(kindProblem('   '), 'empty');
  assert.equal(kindProblem('a'.repeat(80)), null);
  assert.equal(kindProblem('a'.repeat(81)), 'long');
  assert.equal(kindProblem('é'.repeat(40)), null);
  assert.equal(kindProblem('é'.repeat(41)), 'long');
});

const ok = { name: 'Maison', contact: 'Ab', email: 'a@b', phone: '', country: 'sn', message: '' };
test('application limits mirror the API', () => {
  assert.equal(applicationProblem(ok), null);
  assert.equal(applicationProblem({ ...ok, name: ' a ' }), 'name');
  assert.equal(applicationProblem({ ...ok, contact: 'é'.repeat(61) }), 'contact');
  assert.equal(applicationProblem({ ...ok, email: 'nope' }), 'email');
  assert.equal(applicationProblem({ ...ok, phone: '1'.repeat(41) }), 'phone');
  assert.equal(applicationProblem({ ...ok, country: 'SEN' }), 'country');
  assert.equal(applicationProblem({ ...ok, message: 'x'.repeat(2001) }), 'message');
});

test('Save is allowed only once the stored look has been read, or found absent', () => {
  assert.equal(canSaveLook('loading', false), false);
  assert.equal(canSaveLook('failed', false), false);
  assert.equal(canSaveLook('none', false), true);
  assert.equal(canSaveLook('loaded', true), false);
  assert.equal(mayApplyLoad(true), false);
  assert.equal(mayApplyLoad(false), true);
});
