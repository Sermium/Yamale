import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { formatDate, formatDuration, splitDuration } from './duration.ts';

test('the largest whole unit is the one used', () => {
  assert.deepEqual(splitDuration(7 * 86400), { value: 7, unit: 'day' });
  assert.deepEqual(splitDuration(3600), { value: 1, unit: 'hour' });
  assert.deepEqual(splitDuration(90), { value: 1, unit: 'minute' });
  assert.deepEqual(splitDuration(20), { value: 20, unit: 'second' });
});

test('rounding is downward, because a deadline is not a suggestion', () => {
  // Fifty minutes left must not read as "1 hour": somebody planning around
  // that figure loses ten minutes of a window they cannot reopen.
  assert.deepEqual(splitDuration(50 * 60), { value: 50, unit: 'minute' });
  assert.deepEqual(splitDuration(86400 - 1), { value: 23, unit: 'hour' });
  assert.deepEqual(splitDuration(6 * 86400 + 23 * 3600), { value: 6, unit: 'day' });
});

test('a span under a minute never rounds to zero of a bigger unit', () => {
  // "0 minutes" reads as expired. Twenty seconds is not expired.
  assert.deepEqual(splitDuration(20), { value: 20, unit: 'second' });
  assert.deepEqual(splitDuration(59), { value: 59, unit: 'second' });
});

test('an expired or nonsensical span is zero seconds, not a negative one', () => {
  assert.deepEqual(splitDuration(0), { value: 0, unit: 'second' });
  assert.deepEqual(splitDuration(-500), { value: 0, unit: 'second' });
});

test('the phrase is in the reader\'s language, not in English with a translation beside it', () => {
  assert.equal(formatDuration(7 * 86400, 'en'), '7 days');
  assert.equal(formatDuration(1 * 86400, 'en'), '1 day', 'the singular has to agree');

  // French and Portuguese, which the catalogues also ship.
  assert.match(formatDuration(7 * 86400, 'fr'), /jours/);
  assert.match(formatDuration(3 * 3600, 'pt'), /horas/);

  // Arabic: the plural agreement and the numerals both come from the platform,
  // which is the whole reason this does not go through the catalogues.
  const ar = formatDuration(3 * 86400, 'ar');
  assert.doesNotMatch(ar, /day/, `Arabic still rendered an English unit: ${ar}`);
});

test('a nonsense locale falls back rather than throwing inside a render', () => {
  // A thrown formatter in a render path takes the page with it, and the page is
  // more useful than the phrase.
  assert.match(formatDuration(7 * 86400, 'not-a-locale'), /7/);
});

test('dates are written out, because a numeric one is two different days', () => {
  // 03/04/2026 is 3 April or 4 March depending on which side of an ocean it is
  // read, and every date here is a deadline somebody is planning around.
  const at = new Date('2026-04-03T09:30:00Z');
  const en = formatDate(at, 'en-GB');
  assert.match(en, /April/);
  assert.match(en, /2026/);
  assert.doesNotMatch(en, /^\d+\/\d+\/\d+/);

  assert.match(formatDate(at, 'fr'), /avril/);
});

test('an unset date renders as nothing rather than as 1970', () => {
  assert.equal(formatDate(new Date(NaN), 'en'), '');
});
