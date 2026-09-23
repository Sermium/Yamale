/**
 * The tattoo pipeline, tested against the things that make it wrong.
 *
 * # The trap this file is written to avoid
 *
 * The recogniser matches against templates. A test that renders a character
 * FROM those templates and checks it comes back is testing nothing at all — it
 * proves the file can compare a thing to itself, which it could do if every
 * other line were deleted.
 *
 * So every test here renders a character and then RUINS it in a way a real
 * photograph ruins one: a different size, a shifted position, uneven lighting,
 * speckled dirt, a broken stroke. The question is whether the pipeline survives
 * the damage, which is the only question that matters in a pen.
 */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  ALPHABET,
  binarise,
  characterBoxes,
  close,
  components,
  confidenceOf,
  flatten,
  otsu,
  readTattoo,
  toGray,
  verdictOf,
  OCR_THRESHOLDS,
  type Box,
} from './ocr.ts';

/* ------------------------------------------------------------- a canvas */

/** A page of "skin" to punch characters into. 0 is ink, 255 is skin. */
function page(width: number, height: number, skin = 210) {
  const rgba = new Uint8ClampedArray(width * height * 4);
  for (let i = 0; i < width * height; i++) {
    rgba[i * 4] = skin;
    rgba[i * 4 + 1] = skin - 40;
    rgba[i * 4 + 2] = skin - 60;
    rgba[i * 4 + 3] = 255;
  }
  return { rgba, width, height };
}

// The shapes drawn by the tests. Deliberately a SECOND copy of the alphabet
// rather than an import of the recogniser's own: if ocr.ts and this file drift,
// that is a disagreement worth failing on, and sharing the table would hide it.
const DRAW: Record<string, string[]> = {
  '0': ['01110', '10001', '10011', '10101', '11001', '10001', '01110'],
  '1': ['00100', '01100', '00100', '00100', '00100', '00100', '01110'],
  '2': ['01110', '10001', '00001', '00010', '00100', '01000', '11111'],
  '4': ['00010', '00110', '01010', '10010', '11111', '00010', '00010'],
  '7': ['11111', '00001', '00010', '00100', '01000', '01000', '01000'],
  L: ['10000', '10000', '10000', '10000', '10000', '10000', '11111'],
};

/** Punch a character at a scale, with an optional per-pixel corruption. */
function punch(
  img: { rgba: Uint8ClampedArray; width: number },
  char: string,
  atX: number,
  atY: number,
  scale: number,
  corrupt: (x: number, y: number) => boolean = () => false,
) {
  const rows = DRAW[char]!;
  for (let gy = 0; gy < rows.length; gy++) {
    for (let gx = 0; gx < 5; gx++) {
      if (rows[gy]![gx] !== '1') continue;
      for (let dy = 0; dy < scale; dy++) {
        for (let dx = 0; dx < scale; dx++) {
          const x = atX + gx * scale + dx;
          const y = atY + gy * scale + dy;
          if (corrupt(x, y)) continue;
          const p = (y * img.width + x) * 4;
          img.rgba[p] = 25;
          img.rgba[p + 1] = 20;
          img.rgba[p + 2] = 30;
        }
      }
    }
  }
}

function write(
  img: { rgba: Uint8ClampedArray; width: number },
  text: string,
  atX: number,
  atY: number,
  scale: number,
  corrupt?: (x: number, y: number) => boolean,
) {
  let x = atX;
  for (const ch of text) {
    punch(img, ch, x, atY, scale, corrupt);
    x += 6 * scale; // five columns and a gap
  }
}

/** Deterministic speckle, so a failure is reproducible. */
function speckle(seed: number, density: number) {
  return (x: number, y: number) => {
    let v = (seed * 2654435761 + x * 40503 + y * 15485863) >>> 0;
    v ^= v >>> 13;
    v = (v * 1274126177) >>> 0;
    return (v >>> 8) / 0xffffff < density;
  };
}

/* ----------------------------------------------------------- the pipeline */

test('a clean tattoo reads back exactly', () => {
  // The floor. If this fails nothing below is worth looking at.
  const img = page(400, 90);
  write(img, '221042', 30, 20, 8);
  const got = readTattoo(img.rgba, img.width, 90);
  assert.equal(got.text, '221042');
  assert.ok(got.confidence !== null && got.confidence > OCR_THRESHOLDS.likely,
    `a clean read should be confident, got ${got.confidence}`);
});

test('the same tattoo at a different size reads the same', () => {
  // The recogniser normalises every component to a fixed grid. If it did not,
  // it would work at exactly the scale the tests happen to draw and nowhere
  // else — which is the shape of a test suite that passes and an app that does
  // not work.
  const small = page(300, 60);
  write(small, '1042', 20, 10, 5);
  const large = page(700, 150);
  write(large, '1042', 40, 25, 13);

  assert.equal(readTattoo(small.rgba, small.width, 60).text, '1042');
  assert.equal(readTattoo(large.rgba, large.width, 150).text, '1042');
});

test('a tattoo photographed against a lighting gradient still reads', () => {
  // An ear is curved, so one side is in shadow. A single global threshold keeps
  // the lit half and loses the rest, which looks like missing characters rather
  // than like a badly lit photograph. flatten() is what stops that.
  const img = page(420, 90);
  write(img, '221042', 30, 20, 8);
  // Darken progressively from left to right: by the right-hand edge the "skin"
  // is darker than the ink was on the left.
  for (let y = 0; y < 90; y++) {
    for (let x = 0; x < 420; x++) {
      const p = (y * 420 + x) * 4;
      const fall = 1 - (x / 420) * 0.8;
      img.rgba[p] = img.rgba[p]! * fall;
      img.rgba[p + 1] = img.rgba[p + 1]! * fall;
      img.rgba[p + 2] = img.rgba[p + 2]! * fall;
    }
  }
  assert.equal(readTattoo(img.rgba, img.width, 90).text, '221042');
});

test('without flattening, that same photograph does not read', () => {
  // States what the previous test buys. A global threshold on a gradient is the
  // obvious implementation and it fails on the ordinary case, so the cost of
  // flatten() is worth paying.
  const img = page(420, 90);
  write(img, '221042', 30, 20, 8);
  for (let y = 0; y < 90; y++) {
    for (let x = 0; x < 420; x++) {
      const p = (y * 420 + x) * 4;
      const fall = 1 - (x / 420) * 0.8;
      img.rgba[p] = img.rgba[p]! * fall;
      img.rgba[p + 1] = img.rgba[p + 1]! * fall;
      img.rgba[p + 2] = img.rgba[p + 2]! * fall;
    }
  }
  const gray = toGray(img.rgba, 420, 90);
  const naive = binarise(gray, otsu(gray));
  const boxes = characterBoxes(components(naive, 420, 90), 90);
  assert.notEqual(boxes.length, 6,
    'a global threshold found all six characters, so this test no longer proves anything');
});

test('dirt in the ink does not change what it says', () => {
  // A punched character is dots, and dust fills them. Losing a fifth of the ink
  // must not change the answer.
  const img = page(400, 90);
  write(img, '221042', 30, 20, 8, speckle(7, 0.2));
  assert.equal(readTattoo(img.rgba, img.width, 90).text, '221042');
});

test('dirt on the skin is not read as characters', () => {
  // The other half: specks of dirt are components too, and a pipeline that
  // called them characters would insert digits into an identifier.
  const img = page(400, 90);
  write(img, '1042', 40, 20, 8);
  // Scatter small blobs above and below the line of text.
  for (let i = 0; i < 60; i++) {
    const x = 10 + ((i * 37) % 380);
    const y = i % 2 === 0 ? 4 : 84;
    const p = (y * 400 + x) * 4;
    img.rgba[p] = 20;
    img.rgba[p + 1] = 20;
    img.rgba[p + 2] = 20;
  }
  assert.equal(readTattoo(img.rgba, img.width, 90).text, '1042');
});

test('an empty photograph is unreadable rather than an empty string', () => {
  // "I found nothing" and "I found nothing legible" are different sentences and
  // the screen says different things about them.
  const img = page(300, 80);
  const got = readTattoo(img.rgba, img.width, 80);
  assert.equal(got.text, '');
  assert.equal(got.confidence, null);
  assert.equal(verdictOf(got.confidence), 'unreadable');
});

/* --------------------------------------------------------- the confidence */

test('confidence is a margin, not a score', () => {
  // The failure this guards: a character that matches its best template at 0.9
  // and its second at 0.88 is a coin toss, and scoring by the best alone
  // reports it as near-certain. That is how a 6 becomes an 8.
  const decisive = confidenceOf([
    { char: '1', score: 0.9, runnerUp: 0.2, box: {} as Box },
  ]);
  const tossUp = confidenceOf([
    { char: '8', score: 0.9, runnerUp: 0.88, box: {} as Box },
  ]);
  assert.ok(decisive! > 0.6, `a decisive match should be confident, got ${decisive}`);
  assert.ok(tossUp! < 0.2, `a coin toss should not be confident, got ${tossUp}`);
});

test('the weakest character decides the whole string', () => {
  // An identifier is only as good as its worst digit. An average would let nine
  // confident characters carry one that was a guess.
  const good = { char: '1', score: 0.95, runnerUp: 0.1, box: {} as Box };
  const guess = { char: '8', score: 0.7, runnerUp: 0.69, box: {} as Box };
  const all = confidenceOf([good, good, good, good, good, guess]);
  assert.ok(all! < 0.2, `one guess should sink the string, got ${all}`);
});

test('there are three verdicts and the middle one is the useful one', () => {
  assert.equal(verdictOf(0.9), 'likely');
  assert.equal(verdictOf(OCR_THRESHOLDS.likely), 'likely');
  assert.equal(verdictOf(0.5), 'unsure');
  assert.equal(verdictOf(OCR_THRESHOLDS.unsure), 'unsure');
  assert.equal(verdictOf(0.1), 'unreadable');
  assert.equal(verdictOf(null), 'unreadable');
});

test('the thresholds are ordered and neither is certainty', () => {
  assert.ok(OCR_THRESHOLDS.unsure < OCR_THRESHOLDS.likely);
  assert.ok(OCR_THRESHOLDS.likely < 1,
    'nothing this pipeline produces should ever be called certain');
});

/* ------------------------------------------------------------- the pieces */

test('Otsu finds a threshold between the two populations', () => {
  const img = page(100, 40);
  write(img, '1', 40, 12, 4);
  const t = otsu(toGray(img.rgba, 100, 40));
  assert.ok(t > 30 && t < 200, `threshold ${t} does not separate ink from skin`);
});

test('components are found without recursing', () => {
  // A ran-ink photograph is one huge blob, and a recursive flood fill overflows
  // the stack on exactly the input that was already going badly.
  const width = 300;
  const height = 200;
  const binary = new Uint8Array(width * height).fill(1);
  const boxes = components(binary, width, height);
  assert.equal(boxes.length, 1);
  assert.equal(boxes[0]!.width, width);
});

test('the alphabet is published so a screen can say what it can read', () => {
  for (const ch of '0123456789') assert.ok(ALPHABET.includes(ch), `${ch} is missing`);
  assert.ok(ALPHABET.includes('L'), 'the breed letter is missing');
});

test('flatten leaves a flat image alone', () => {
  // A correction that changes an image which needed no correction is a
  // correction that will damage a good photograph.
  const img = page(60, 40, 200);
  const before = toGray(img.rgba, 60, 40);
  const after = flatten(before, 8);
  for (let i = 0; i < after.data.length; i++) {
    assert.ok(Math.abs(after.data[i]! - 128) < 3,
      'a uniform image should flatten to uniform mid-grey');
  }
});

test('closing bridges a broken stroke without merging neighbours', () => {
  // Both halves of the same constraint. A radius large enough to repair a worn
  // stroke and small enough to leave the gap between characters alone is the
  // only radius worth having, and getting it wrong fails in opposite
  // directions: a vanished digit, or two digits read as one blob.
  const width = 40;
  const height = 40;

  // A vertical stroke with a two-pixel gap in it, and a second stroke well
  // clear of it.
  const broken = new Uint8Array(width * height);
  for (let y = 5; y < 35; y++) {
    if (y === 19 || y === 20) continue; // the break
    broken[y * width + 10] = 1;
    broken[y * width + 11] = 1;
  }
  for (let y = 5; y < 35; y++) {
    broken[y * width + 28] = 1;
    broken[y * width + 29] = 1;
  }

  assert.equal(components(broken, width, height).length, 3, 'the fixture is not broken as intended');

  const closed = close(broken, width, height, 2);
  const after = components(closed, width, height);
  assert.equal(after.length, 2, `closing should repair the break and keep two strokes, got ${after.length}`);
});

test('a worn tattoo reads, and the pipeline says it is less certain', () => {
  // The whole point of reporting confidence: the same identifier read off a
  // clean ear and a worn one should not be presented with equal authority.
  const clean = page(400, 90);
  write(clean, '221042', 30, 20, 8);
  const worn = page(400, 90);
  write(worn, '221042', 30, 20, 8, speckle(11, 0.18));

  const a = readTattoo(clean.rgba, clean.width, 90);
  const b = readTattoo(worn.rgba, worn.width, 90);
  assert.equal(a.text, '221042');
  assert.equal(b.text, '221042');
  assert.ok(a.confidence !== null && b.confidence !== null);
  assert.ok(b.confidence <= a.confidence,
    `a worn read should not be more confident than a clean one: ${b.confidence} vs ${a.confidence}`);
});
