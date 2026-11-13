import assert from 'node:assert/strict';
import { test } from 'node:test';

import { protectionOutcome } from './protection.ts';

test('both services taking the deposit is protection', () => {
  assert.equal(protectionOutcome({ alreadyEnrolled: [] }), 'enrolled');
});

test('a 409 from either service is not protection', () => {
  assert.equal(protectionOutcome({ alreadyEnrolled: ['custodian'] }), 'email-in-use');
  assert.equal(protectionOutcome({ alreadyEnrolled: ['recovery'] }), 'email-in-use');
  assert.equal(protectionOutcome({ alreadyEnrolled: ['custodian', 'recovery'] }), 'email-in-use');
});
