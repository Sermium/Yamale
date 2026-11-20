import assert from 'node:assert/strict';
import { test } from 'node:test';

import { listPriceDay, listPriceText } from './listprice.ts';

const lp = { amount: '17678.00', currency: 'USD', at: '2026-11-16', label: 'list price at registration' as const };

test('a list price is the amount, two decimals, and its currency code', () => {
  assert.equal(listPriceText(lp, 'en'), '17,678.00 USD');
  assert.equal(listPriceText({ ...lp, amount: '480' }, 'en'), '480.00 USD');
  assert.equal(listPriceText(lp, 'fr'), '17 678,00 USD');
});

test('a list price is dated by its day alone, whatever the timezone', () => {
  assert.equal(listPriceDay('2026-11-16', 'en-GB'), '16 Nov 2026');
  assert.equal(listPriceDay('2026-11-16', 'en'), 'Nov 16, 2026');
  assert.equal(listPriceDay('2026-11-16', 'fr'), '16 nov. 2026');
});
