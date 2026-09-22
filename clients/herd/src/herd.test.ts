/**
 * The parts of the record model that can be wrong without a network.
 *
 * Almost all of this is about what an operator's thumbs produced. A national
 * identifier typed with dashes because the keyboard's slash is two taps away, a
 * tag read back from a stick reader with spaces in it, a serial that is one
 * digit short because the tattoo was dusty — each of those resolves to a
 * different animal or to none, and the difference between "repair this" and
 * "refuse this" is the entire correctness of the lookup.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  ageInMonths,
  asHeight,
  classify,
  isMovable,
  levelOf,
  lineGaps,
  normaliseNationalId,
  normaliseTag,
  looksLikeNationalId,
  looksLikeTag,
  speciesOf,
  statusOf,
  statusTone,
  toAnimal,
  transferStateOf,
  type Animal,
  type Line,
} from './herd.ts';

/* ------------------------------------------------------------------ enums */

describe('enums', () => {
  it('maps the values devnet-2 actually carries', () => {
    // Read off the live chain: animal 4 is level 3, animal 1 is level 2, herd 1
    // is species 2 (sheep), herd 2 is species 1 (cattle).
    assert.equal(levelOf(3), 'certified');
    assert.equal(levelOf(2), 'individual');
    assert.equal(speciesOf(2), 'sheep');
    assert.equal(speciesOf(1), 'cattle');
    assert.equal(statusOf(1), 'active');
    assert.equal(statusOf(2), 'transferPending');
    assert.equal(statusOf(3), 'disputed');
    assert.equal(transferStateOf(3), 'attested');
  });

  it('keeps an unknown number as unspecified rather than inventing a default', () => {
    // The keeper refuses a zero on purpose, so a zero arriving here is a record
    // this client does not understand. Defaulting it to `active` would be the
    // exact silent promotion the module declined to make.
    assert.equal(statusOf(0), 'unspecified');
    assert.equal(levelOf(0), 'unspecified');
    assert.equal(statusOf(99), 'unspecified');
    assert.equal(levelOf(-1), 'unspecified');
  });

  it('refuses to call a pending transfer movable', () => {
    assert.equal(isMovable('active'), true);
    assert.equal(isMovable('transferPending'), false);
    assert.equal(isMovable('frozen'), false);
    assert.equal(isMovable('unspecified'), false);
  });

  it('gives a pending transfer a warning tone, not a good one', () => {
    assert.equal(statusTone('active'), 'ok');
    assert.equal(statusTone('transferPending'), 'warn');
    assert.equal(statusTone('frozen'), 'bad');
    assert.equal(statusTone('retired'), 'mute');
  });
});

/* ------------------------------------------------------- the two clocks */

describe('the two clocks', () => {
  it('reads a height off a wire animal without treating it as a time', () => {
    // devnet-2, animal 1. 320898 is a BLOCK, and a Date built from it lands in
    // January 1970. The brand is what stops a screen passing it to a formatter.
    const a = toAnimal({
      id: '1', herdId: '1', nationalId: '221/682/000102/L', tag: '982000000000102',
      level: 2, name: 'Nianing', sex: 2, coat: 'fauve', bornOn: '2018-05-20',
      birthType: 'single', sireId: '0', damId: '0', dnaHash: '', photoHash: '',
      holder: 'yml1zu5', authority: 'yml14249', status: 1, registeredAt: '320898',
    } as never);
    assert.equal(a.registeredAt, asHeight(320898));
    assert.equal(a.id, '1');
    assert.equal(a.level, 'individual');
    assert.equal(a.sireId, '0');
  });
});

/* ----------------------------------------------------- what was typed in */

describe('normalising a national identifier', () => {
  it('accepts the separators a thumb produces', () => {
    const want = '221/682/021029/L';
    for (const typed of [
      '221/682/021029/L',
      '221-682-021029-L',
      '221 682 021029 L',
      '221.682.021029.l',
      '221\\682\\021029\\L',
      '  221/682/021029/l  ',
      '221//682//021029//L',
    ]) {
      assert.equal(normaliseNationalId(typed), want, typed);
    }
  });

  it('does not repair a digit', () => {
    // A short serial is the operator's mistake to correct. Padding it here
    // would resolve a certificate to a different animal and look like a
    // successful lookup while doing it.
    assert.equal(normaliseNationalId('221/682/1029/L'), '221/682/1029/L');
    assert.notEqual(normaliseNationalId('221/682/1029/L'), '221/682/021029/L');
  });

  it('recognises the live forms and rejects prose', () => {
    assert.equal(looksLikeNationalId('221/682/021029/L'), true);
    assert.equal(looksLikeNationalId('221/312/041101/B'), true);
    assert.equal(looksLikeNationalId('221/682/000102'), true);
    assert.equal(looksLikeNationalId('NIANING'), false);
    assert.equal(looksLikeNationalId(''), false);
  });
});

describe('normalising a transponder code', () => {
  it('strips what a reader and a label put between the digits', () => {
    assert.equal(normaliseTag('982 000 000 000 102'), '982000000000102');
    assert.equal(normaliseTag('982.000000000102'), '982000000000102');
    assert.equal(normaliseTag('982000000000102'), '982000000000102');
  });

  it('checks the length rather than assuming it', () => {
    // ISO 11784 is fifteen digits. Fourteen is a misread, and querying with it
    // would answer "no such animal" about a perfectly good beast.
    assert.equal(looksLikeTag('982000000000102'), true);
    assert.equal(looksLikeTag('98200000000010'), false);
    assert.equal(looksLikeTag('9820000000001020'), false);
    assert.equal(looksLikeTag(''), false);
  });
});

describe('deciding which query to run', () => {
  it('routes the two live forms', () => {
    assert.deepEqual(classify('982000000021029'), {
      kind: 'tag', value: '982000000021029', raw: '982000000021029',
    });
    assert.equal(classify('221/682/021029/L').kind, 'national-id');
    assert.equal(classify('221/682/021029/L').value, '221/682/021029/L');
  });

  it('routes a spaced tag as a tag', () => {
    const got = classify('982 000 000 021 029');
    assert.equal(got.kind, 'tag');
    assert.equal(got.value, '982000000021029');
  });

  it('refuses to guess when it cannot tell', () => {
    // Deliberate. Firing both queries and reporting whichever answered teaches
    // an operator to trust a lookup that was a coin toss.
    assert.equal(classify('Nianing').kind, 'unrecognised');
    assert.equal(classify('98200000').kind, 'unrecognised');
    assert.equal(classify('   ').kind, 'empty');
  });

  it('keeps the raw input for echoing back', () => {
    assert.equal(classify('  Nianing ').raw, '  Nianing ');
  });
});

/* ------------------------------------------------------------------ age */

describe('age', () => {
  const now = new Date('2026-09-22T00:00:00Z');

  it('counts whole months from a recorded day', () => {
    assert.equal(ageInMonths('2025-08-25', now), 12);
    assert.equal(ageInMonths('2026-09-22', now), 0);
    assert.equal(ageInMonths('2018-05-20', now), 100);
  });

  it('does not count a month that has not completed', () => {
    assert.equal(ageInMonths('2026-08-25', now), 0);
    assert.equal(ageInMonths('2026-08-22', now), 1);
  });

  it('returns null rather than zero when the day is absent', () => {
    // Zero would say the animal was born today, which is a claim. Null is the
    // absence of one, and the screen prints "not recorded".
    assert.equal(ageInMonths('', now), null);
    assert.equal(ageInMonths('2018', now), null);
    assert.equal(ageInMonths('not a date', now), null);
  });
});

/* -------------------------------------------------------------- the line */

describe('gaps in a line', () => {
  const animal = (id: string, sireId = '0', damId = '0'): Animal => ({
    id, herdId: '1', nationalId: `221/682/00000${id}/L`, tag: '', level: 'certified',
    name: '', sex: 'unspecified', coat: '', bornOn: '', birthType: '',
    sireId, damId, dnaHash: '', photoHash: '', holder: '', authority: '',
    status: 'active', registeredAt: asHeight(1),
  });

  it('tells a founder from a walk that stopped', () => {
    // devnet-2, animal 7 (Classico): sire 3, dam 6, and 6's dam is 1. Three
    // generations, all present, so nothing is unreached.
    const line: Line = {
      truncated: false,
      nodes: [
        { generation: 0, path: '', animal: animal('7', '3', '6') },
        { generation: 1, path: 'sire', animal: animal('3') },
        { generation: 1, path: 'dam', animal: animal('6', '0', '1') },
        { generation: 2, path: 'dam.dam', animal: animal('1') },
      ],
    };
    const gaps = lineGaps(line);
    assert.deepEqual(gaps.unreached, []);
    // 3 and 1 have neither parent; 6 has no sire. Five unrecorded parents.
    assert.equal(gaps.unrecorded, 5);
  });

  it('names a parent the walk did not reach', () => {
    const line: Line = {
      truncated: true,
      nodes: [{ generation: 0, path: '', animal: animal('7', '3', '6') }],
    };
    assert.deepEqual(lineGaps(line).unreached, ['3', '6']);
    assert.equal(lineGaps(line).unrecorded, 0);
  });
});
