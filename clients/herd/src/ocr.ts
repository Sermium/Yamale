/**
 * Reading a tattooed ear.
 *
 * # What this is and what it deliberately is not
 *
 * A Senegalese ear tattoo is a short string — digits and a few letters — punched
 * into the inside of an ear, then rubbed with ink. By the time anybody wants to
 * read it, it has spent years in dust, sun and flies, on an animal that does not
 * want its ear held still.
 *
 * So this is NOT a general text recogniser and does not pretend to be. It is a
 * pipeline for one constrained problem: dark marks on a pale, curved, filthy
 * surface, in a known alphabet, photographed badly.
 *
 * The honest division of labour:
 *
 *   - the PREPARATION is the part that decides whether a read is possible at
 *     all, and it is ordinary image processing that can be written and tested
 *     precisely. Most of the work here is that.
 *   - the RECOGNITION is template matching over a known alphabet. It is
 *     adequate for a punched tattoo, which is a small set of shapes at a fixed
 *     size, and would be hopeless for handwriting.
 *   - the PLATFORM's own detector is used where it exists, because a browser
 *     that ships one has a far better model than anything that belongs in this
 *     file.
 *
 * # Nothing here ever submits
 *
 * `ports.ts` requires the confidence to be nullable and says why: "OCR of a
 * dusty ear at dusk is wrong often enough that presenting its output as an
 * identifier would be a lie with a camera behind it". That rule is enforced
 * here as well as at the screen — `read` returns what it saw and how sure it
 * is, `verdictOf` refuses to call anything certain, and a caller that wants to
 * skip the human has to do so deliberately rather than by accident.
 *
 * A wrong identifier that somebody accepted is worse than no identifier. It
 * attaches a real animal's history to a different animal, and nothing later
 * looks wrong.
 */

/* ------------------------------------------------------------ the picture */

/** A grayscale image: one byte per pixel, row-major. */
export interface Gray {
  data: Uint8ClampedArray;
  width: number;
  height: number;
}

/**
 * Luminance from RGBA.
 *
 * Rec. 601 weights rather than a plain mean. A tattoo is blue-black ink on pink
 * or brown skin, and the mean collapses exactly that contrast: blue is where
 * the difference lives and a mean gives it a third of the vote instead of the
 * eleven per cent that matches how it actually looks.
 */
export function toGray(rgba: Uint8ClampedArray, width: number, height: number): Gray {
  const out = new Uint8ClampedArray(width * height);
  for (let i = 0, p = 0; i < out.length; i++, p += 4) {
    out[i] = (rgba[p]! * 299 + rgba[p + 1]! * 587 + rgba[p + 2]! * 114) / 1000;
  }
  return { data: out, width, height };
}

/**
 * Otsu's threshold: the grey level that best splits the image into two.
 *
 * Chosen over a fixed threshold because there is no fixed threshold. The same
 * tattoo photographed in shade and in the sun differs by more than the ink
 * differs from the skin, so any constant is right for one photograph and wrong
 * for the next.
 */
export function otsu(gray: Gray): number {
  const histogram = new Uint32Array(256);
  for (const v of gray.data) histogram[v]!++;

  const total = gray.data.length;
  let sum = 0;
  for (let i = 0; i < 256; i++) sum += i * histogram[i]!;

  let sumBackground = 0;
  let weightBackground = 0;
  // The best threshold is usually a RANGE, not a point: when the two
  // populations are well separated, every level in the gap between them splits
  // the image identically and scores identically. Taking the first is what a
  // naive implementation does, and it puts the threshold hard against the ink,
  // where a pixel one level lighter than the darkest ink falls on the wrong
  // side. The midpoint of the plateau is the value a person would pick by
  // looking at the histogram.
  let firstBest = 0;
  let lastBest = 0;
  let bestVariance = -1;

  for (let t = 0; t < 256; t++) {
    weightBackground += histogram[t]!;
    if (weightBackground === 0) continue;
    const weightForeground = total - weightBackground;
    if (weightForeground === 0) break;

    sumBackground += t * histogram[t]!;
    const meanBackground = sumBackground / weightBackground;
    const meanForeground = (sum - sumBackground) / weightForeground;
    const between =
      weightBackground * weightForeground * (meanBackground - meanForeground) ** 2;
    if (between > bestVariance) {
      bestVariance = between;
      firstBest = t;
      lastBest = t;
    } else if (between === bestVariance) {
      lastBest = t;
    }
  }
  return Math.round((firstBest + lastBest) / 2);
}

/**
 * Local contrast, before thresholding.
 *
 * An ear is CURVED, so one side of the photograph is in shadow and the other is
 * not. A single global threshold then keeps the lit half and loses the shaded
 * half entirely, which reads as a tattoo with missing characters rather than as
 * a badly lit photograph.
 *
 * This normalises each pixel against the mean of its neighbourhood, which
 * flattens the lighting gradient and leaves the ink.
 *
 * # The radius is not a constant, and a constant is a bug
 *
 * It must be comfortably LARGER THAN A WHOLE CHARACTER. A radius near the size
 * of a stroke makes each stroke its own neighbourhood: the middle of a thick
 * stroke is then no darker than the average around it, so it survives
 * thresholding as an outline and a `1` arrives at the recogniser as two
 * disconnected pieces. That was the first version of this function, and it
 * failed on clean synthetic input before it ever saw a photograph.
 *
 * Defaulting from the image height is what makes it scale: a tattoo fills a
 * usable fraction of the frame at any resolution, so the height is a far better
 * guide to character size than any number written here.
 */
export function flatten(gray: Gray, radius = Math.max(8, Math.round(gray.height * 0.9))): Gray {
  const { width, height, data } = gray;
  const integral = new Float64Array((width + 1) * (height + 1));
  for (let y = 0; y < height; y++) {
    let rowSum = 0;
    for (let x = 0; x < width; x++) {
      rowSum += data[y * width + x]!;
      integral[(y + 1) * (width + 1) + x + 1] = integral[y * (width + 1) + x + 1] + rowSum;
    }
  }

  const area = (x0: number, y0: number, x1: number, y1: number) =>
    integral[y1 * (width + 1) + x1]! -
    integral[y0 * (width + 1) + x1]! -
    integral[y1 * (width + 1) + x0]! +
    integral[y0 * (width + 1) + x0]!;

  const out = new Uint8ClampedArray(width * height);
  for (let y = 0; y < height; y++) {
    const y0 = Math.max(0, y - radius);
    const y1 = Math.min(height, y + radius + 1);
    for (let x = 0; x < width; x++) {
      const x0 = Math.max(0, x - radius);
      const x1 = Math.min(width, x + radius + 1);
      const mean = area(x0, y0, x1, y1) / ((x1 - x0) * (y1 - y0));
      // Centred on 128 so the result is still an image rather than a mask, and
      // a later global threshold has something sensible to work on.
      out[y * width + x] = 128 + (data[y * width + x]! - mean);
    }
  }
  return { data: out, width, height };
}

/** Ink is 1. Everything darker than the threshold is ink. */
export function binarise(gray: Gray, threshold: number): Uint8Array {
  const out = new Uint8Array(gray.width * gray.height);
  for (let i = 0; i < out.length; i++) out[i] = gray.data[i]! < threshold ? 1 : 0;
  return out;
}

/**
 * Morphological closing: dilate, then erode.
 *
 * # What it is for
 *
 * A punched tattoo is dots, and years of wear take some of them. The ink also
 * dries, cracks and fills with dust. So a stroke that was solid when it was
 * made arrives at the camera as a broken line, and a broken line is not one
 * connected component — it is three, each too small to pass the character
 * filter, so the character vanishes rather than reading badly.
 *
 * Closing bridges gaps up to twice the radius and leaves everything larger
 * alone. It is the difference between "this digit is worn" and "this digit is
 * not here".
 *
 * # Why the radius has to stay small
 *
 * Too large and it bridges the gap BETWEEN characters, merging `1` and `0` into
 * one blob that matches nothing. The gap between punched characters is roughly
 * a stroke wide, so the radius must stay well under that — which is why it is
 * derived from the image height and kept to a couple of per cent of it, rather
 * than being tuned upward until a stubborn test passes.
 */
export function close(
  binary: Uint8Array,
  width: number,
  height: number,
  radius = Math.max(1, Math.round(height * 0.02)),
): Uint8Array {
  return erode(dilate(binary, width, height, radius), width, height, radius);
}

function dilate(binary: Uint8Array, width: number, height: number, radius: number): Uint8Array {
  return morph(binary, width, height, radius, true);
}

function erode(binary: Uint8Array, width: number, height: number, radius: number): Uint8Array {
  return morph(binary, width, height, radius, false);
}

/**
 * One morphological pass, separably: rows then columns.
 *
 * Separable because a square structuring element is the product of a horizontal
 * and a vertical one, and doing it in two passes is O(r) rather than O(r^2) per
 * pixel. On a phone photograph that is the difference between imperceptible and
 * noticeable.
 */
function morph(
  binary: Uint8Array,
  width: number,
  height: number,
  radius: number,
  grow: boolean,
): Uint8Array {
  const pass = (src: Uint8Array, horizontal: boolean) => {
    const out = new Uint8Array(src.length);
    const span = horizontal ? width : height;
    const other = horizontal ? height : width;
    for (let o = 0; o < other; o++) {
      for (let i = 0; i < span; i++) {
        let value = grow ? 0 : 1;
        for (let d = -radius; d <= radius; d++) {
          const at = i + d;
          if (at < 0 || at >= span) {
            // Outside the frame counts as background. For an erosion that
            // shrinks the edge, which is correct: a stroke running off the
            // photograph is a stroke whose extent is unknown.
            if (!grow) value = 0;
            continue;
          }
          const px = horizontal ? o * width + at : at * width + o;
          if (grow) {
            if (src[px] === 1) {
              value = 1;
              break;
            }
          } else if (src[px] !== 1) {
            value = 0;
            break;
          }
        }
        out[horizontal ? o * width + i : i * width + o] = value;
      }
    }
    return out;
  };
  return pass(pass(binary, true), false);
}

/* ------------------------------------------------------ finding characters */

export interface Box {
  x: number;
  y: number;
  width: number;
  height: number;
}

/**
 * Connected components, EIGHT-neighbour, iterative.
 *
 * # Eight, and four is a bug
 *
 * Four-neighbour connectivity treats pixels that touch only at a corner as
 * belonging to different shapes. That is exactly what a diagonal stroke is: the
 * body of a `2`, the leg of a `4`, the whole of a `7`. Under four-neighbour
 * rules a `2` arrives as four separate components, each of which is then
 * measured, filtered and recognised as if it were its own character — so a
 * six-character tattoo came out as eighteen.
 *
 * It matters more here than in most image processing, because a PUNCHED
 * character is not drawn: it is a row of dots, and dots on a diagonal touch at
 * their corners or not at all.
 *
 * Iterative rather than recursive on purpose: a photograph of a dirty ear can
 * produce a single connected blob of tens of thousands of pixels when the ink
 * has run, and a recursive flood fill overflows the stack on exactly the input
 * that was already going badly.
 */
export function components(binary: Uint8Array, width: number, height: number): Box[] {
  const seen = new Uint8Array(binary.length);
  const boxes: Box[] = [];
  const stack: number[] = [];

  for (let start = 0; start < binary.length; start++) {
    if (binary[start] !== 1 || seen[start]) continue;

    let minX = width;
    let maxX = -1;
    let minY = height;
    let maxY = -1;

    stack.push(start);
    seen[start] = 1;
    while (stack.length > 0) {
      const at = stack.pop()!;
      const x = at % width;
      const y = (at - x) / width;
      if (x < minX) minX = x;
      if (x > maxX) maxX = x;
      if (y < minY) minY = y;
      if (y > maxY) maxY = y;

      const left = x > 0;
      const right = x < width - 1;
      const up = y > 0;
      const down = y < height - 1;

      if (left) push(at - 1);
      if (right) push(at + 1);
      if (up) push(at - width);
      if (down) push(at + width);
      // The diagonals. Without these a punched character falls apart.
      if (up && left) push(at - width - 1);
      if (up && right) push(at - width + 1);
      if (down && left) push(at + width - 1);
      if (down && right) push(at + width + 1);
    }
    boxes.push({ x: minX, y: minY, width: maxX - minX + 1, height: maxY - minY + 1 });
  }

  function push(at: number) {
    if (binary[at] === 1 && !seen[at]) {
      seen[at] = 1;
      stack.push(at);
    }
  }

  return boxes;
}

/**
 * Which components are plausibly characters, left to right.
 *
 * A photograph of an ear contains the tattoo and a great deal else: hair, a
 * tag, a fly, the operator's thumb, and dirt. Filtering by shape removes most
 * of it without knowing anything about what the characters say.
 *
 * The filters are deliberately loose. A character that is wrongly discarded is
 * a missing digit in the middle of an identifier, which is worse than a piece
 * of dirt the recogniser then scores badly and reports as uncertain.
 */
export function characterBoxes(boxes: Box[], imageHeight: number): Box[] {
  const plausible = boxes.filter(
    (b) =>
      // Too small is dirt or a fly; too tall is the edge of the ear.
      b.height >= imageHeight * 0.08 &&
      b.height <= imageHeight * 0.9 &&
      // Punched characters are taller than they are wide, but a 0 or an 8 in a
      // squat font is close to square, and a 1 is very narrow.
      b.width >= 2 &&
      b.width <= b.height * 2.0 &&
      b.height <= b.width * 12,
  );
  if (plausible.length === 0) return [];

  // Keep only what sits on roughly the same line as the tallest component: a
  // tattoo is one row, and the second row in the picture is usually the tag.
  const tallest = plausible.reduce((a, b) => (b.height > a.height ? b : a));
  const centre = tallest.y + tallest.height / 2;
  const tolerance = tallest.height * 0.8;

  return plausible
    .filter((b) => Math.abs(b.y + b.height / 2 - centre) <= tolerance)
    .sort((a, b) => a.x - b.x);
}

/* --------------------------------------------------------- the recogniser */

/**
 * The alphabet, as 5x7 bitmaps.
 *
 * Small on purpose. These are templates for matching a PUNCHED character, which
 * is a handful of dots in a fixed arrangement rather than a typeface — so the
 * shape that matters is coarse, and a finer template would match the dirt
 * between the dots as confidently as the dots.
 *
 * Digits and the few letters the sector's tattoos use. Extending it is adding a
 * row here, and the recogniser needs no other change.
 */
const GLYPH_WIDTH = 5;
const GLYPH_HEIGHT = 7;

const GLYPHS: Record<string, string[]> = {
  '0': ['01110', '10001', '10011', '10101', '11001', '10001', '01110'],
  '1': ['00100', '01100', '00100', '00100', '00100', '00100', '01110'],
  '2': ['01110', '10001', '00001', '00010', '00100', '01000', '11111'],
  '3': ['11111', '00010', '00100', '00010', '00001', '10001', '01110'],
  '4': ['00010', '00110', '01010', '10010', '11111', '00010', '00010'],
  '5': ['11111', '10000', '11110', '00001', '00001', '10001', '01110'],
  '6': ['00110', '01000', '10000', '11110', '10001', '10001', '01110'],
  '7': ['11111', '00001', '00010', '00100', '01000', '01000', '01000'],
  '8': ['01110', '10001', '10001', '01110', '10001', '10001', '01110'],
  '9': ['01110', '10001', '10001', '01111', '00001', '00010', '01100'],
  A: ['01110', '10001', '10001', '11111', '10001', '10001', '10001'],
  B: ['11110', '10001', '10001', '11110', '10001', '10001', '11110'],
  C: ['01110', '10001', '10000', '10000', '10000', '10001', '01110'],
  L: ['10000', '10000', '10000', '10000', '10000', '10000', '11111'],
  M: ['10001', '11011', '10101', '10101', '10001', '10001', '10001'],
  N: ['10001', '11001', '10101', '10011', '10001', '10001', '10001'],
  S: ['01110', '10001', '10000', '01110', '00001', '10001', '01110'],
  Z: ['11111', '00001', '00010', '00100', '01000', '10000', '11111'],
};

const TEMPLATES: { char: string; bits: Float64Array }[] = Object.entries(GLYPHS).map(
  ([char, rows]) => {
    const bits = new Float64Array(GLYPH_WIDTH * GLYPH_HEIGHT);
    rows.forEach((row, y) => {
      for (let x = 0; x < GLYPH_WIDTH; x++) bits[y * GLYPH_WIDTH + x] = row[x] === '1' ? 1 : 0;
    });
    return { char, bits };
  },
);

/** Every character this can recognise. Exported so a screen can say so. */
export const ALPHABET = Object.keys(GLYPHS).join('');

/**
 * Resample one component to the template grid.
 *
 * Area averaging rather than nearest-neighbour: a punched character at
 * photograph resolution is a few hundred pixels, and nearest-neighbour throws
 * away most of them and turns a 6 into an 8 whenever the sample lands on a gap.
 */
function normalise(binary: Uint8Array, width: number, box: Box): Float64Array {
  const cell = new Float64Array(GLYPH_WIDTH * GLYPH_HEIGHT);
  for (let gy = 0; gy < GLYPH_HEIGHT; gy++) {
    const y0 = box.y + Math.floor((gy * box.height) / GLYPH_HEIGHT);
    const y1 = Math.max(y0 + 1, box.y + Math.floor(((gy + 1) * box.height) / GLYPH_HEIGHT));
    for (let gx = 0; gx < GLYPH_WIDTH; gx++) {
      const x0 = box.x + Math.floor((gx * box.width) / GLYPH_WIDTH);
      const x1 = Math.max(x0 + 1, box.x + Math.floor(((gx + 1) * box.width) / GLYPH_WIDTH));
      let ink = 0;
      let n = 0;
      for (let y = y0; y < y1; y++) {
        for (let x = x0; x < x1; x++) {
          ink += binary[y * width + x]!;
          n++;
        }
      }
      cell[gy * GLYPH_WIDTH + gx] = n === 0 ? 0 : ink / n;
    }
  }
  return cell;
}

/**
 * Normalised correlation between a cell and a template.
 *
 * Correlation rather than a count of matching pixels, because the count
 * rewards a blob: a solid black square matches every template about equally
 * well on a raw count and matches none of them under correlation.
 */
function correlate(cell: Float64Array, template: Float64Array): number {
  let meanA = 0;
  let meanB = 0;
  for (let i = 0; i < cell.length; i++) {
    meanA += cell[i]!;
    meanB += template[i]!;
  }
  meanA /= cell.length;
  meanB /= template.length;

  let num = 0;
  let devA = 0;
  let devB = 0;
  for (let i = 0; i < cell.length; i++) {
    const a = cell[i]! - meanA;
    const b = template[i]! - meanB;
    num += a * b;
    devA += a * a;
    devB += b * b;
  }
  if (devA === 0 || devB === 0) return 0;
  return num / Math.sqrt(devA * devB);
}

/** One recognised character and how well it matched. */
export interface Glyph {
  char: string;
  score: number;
  /** The runner-up's score. A close second is the real measure of doubt. */
  runnerUp: number;
  box: Box;
}

export function recogniseBox(binary: Uint8Array, width: number, box: Box): Glyph {
  const cell = normalise(binary, width, box);
  let best = { char: '?', score: -1 };
  let second = -1;
  for (const t of TEMPLATES) {
    const score = correlate(cell, t.bits);
    if (score > best.score) {
      second = best.score;
      best = { char: t.char, score };
    } else if (score > second) {
      second = score;
    }
  }
  return { char: best.char, score: best.score, runnerUp: Math.max(second, 0), box };
}

/* ------------------------------------------------------------- the answer */

export interface Reading {
  text: string;
  /**
   * How much of what was read had to be repaired, 0 to 1.
   *
   * The fraction of the final ink that closing INVENTED rather than saw. A
   * clean photograph repairs nothing; a worn tattoo is bridged back together
   * and this says how much of it is inference.
   *
   * It exists because template matching alone cannot tell. Closing is very
   * good at its job, so a badly worn character is repaired into something that
   * matches its template beautifully — and confidence measured on the match
   * alone was therefore HIGHER for a worn tattoo than a clean one, which is
   * exactly backwards and was caught by a test asserting the opposite.
   */
  repaired: number;
  /**
   * 0 to 1, or null when there was nothing to read.
   *
   * Null rather than zero, and the distinction is the same one `ports.ts` draws
   * about capture quality: "no characters were found" and "characters were
   * found and they are probably wrong" need different words on the screen and
   * lead to different actions — move the camera, or check the reading.
   */
  confidence: number | null;
  glyphs: Glyph[];
}

/**
 * How certain a reading is, per character and overall.
 *
 * MARGIN, not score. A character that matches its best template at 0.9 and its
 * second at 0.88 is not a confident read; it is a coin toss between two shapes
 * that both fit. Scoring by the best alone reports exactly that case as
 * near-certain, which is the failure mode that puts a 6 where an 8 belongs.
 *
 * The whole string is scored by its WEAKEST character, because an identifier is
 * only as good as its worst digit — an average lets nine confident characters
 * carry one that was a guess.
 */
export function confidenceOf(glyphs: Glyph[]): number | null {
  if (glyphs.length === 0) return null;
  let worst = 1;
  for (const g of glyphs) {
    const margin = Math.max(0, g.score - g.runnerUp);
    // Scaled so a margin of about 0.35 reads as confident. Chosen to be
    // conservative rather than fitted to anything: see the note on thresholds.
    const certainty = Math.min(1, Math.max(0, g.score) * Math.min(1, margin / 0.35));
    worst = Math.min(worst, certainty);
  }
  return worst;
}

/**
 * Thresholds, and a warning about them.
 *
 * WRITTEN TO BE CALIBRATED, NOT BELIEVED. No photograph of a real tattoo has
 * ever been through this code. These numbers exist so the pipeline has a shape
 * and the screens have three states to render; they must be re-derived from
 * field captures before anybody relies on them, exactly as iris.ts says of its
 * own.
 */
export const OCR_THRESHOLDS = { likely: 0.62, unsure: 0.35 };

export type OCRVerdict = 'likely' | 'unsure' | 'unreadable';

export function verdictOf(confidence: number | null): OCRVerdict {
  if (confidence === null) return 'unreadable';
  if (confidence >= OCR_THRESHOLDS.likely) return 'likely';
  if (confidence >= OCR_THRESHOLDS.unsure) return 'unsure';
  return 'unreadable';
}

/**
 * The whole pipeline: a photograph in, a reading out.
 *
 * Note what it does NOT do: decide. It returns a string and a number, and the
 * screen is where a person looks at both.
 */
export function readTattoo(
  rgba: Uint8ClampedArray,
  width: number,
  height: number,
): Reading {
  return readTattooGray(toGray(rgba, width, height));
}

/**
 * The same pipeline, for a source that already has luminance.
 *
 * `device.ts` hands out a `Frame` carrying `luma` — one byte per pixel, which
 * is what every camera produces before anything converts it to RGBA. Going
 * through RGBA to get back to grey would cost a full-frame copy and three
 * multiplies per pixel to recover a number the camera already had.
 */
export function readTattooLuma(luma: Uint8Array, width: number, height: number): Reading {
  return readTattooGray({ data: new Uint8ClampedArray(luma), width, height });
}

function readTattooGray(source: Gray): Reading {
  const { width, height } = source;
  const gray = flatten(source);
  const raw = binarise(gray, otsu(gray));
  // Closed before components are found, so a worn stroke is one character
  // rather than three fragments that each fail the size filter.
  const binary = close(raw, width, height);
  const boxes = characterBoxes(components(binary, width, height), height);

  const glyphs = boxes.map((b) => recogniseBox(binary, width, b));
  const repaired = repairFraction(raw, binary);
  return {
    text: glyphs.map((g) => g.char).join(''),
    // Discounted by the repair. Reading a tattoo that was half invented is not
    // as good as reading one that was there, and the match score cannot tell
    // the difference because the repair happened before the match.
    confidence: discount(confidenceOf(glyphs), repaired),
    repaired,
    glyphs,
  };
}

/** The share of the final ink that closing added rather than saw. */
function repairFraction(before: Uint8Array, after: Uint8Array): number {
  let added = 0;
  let total = 0;
  for (let i = 0; i < after.length; i++) {
    if (after[i] === 1) {
      total++;
      if (before[i] !== 1) added++;
    }
  }
  return total === 0 ? 0 : added / total;
}

/**
 * Lower the confidence in proportion to how much was inferred.
 *
 * Linear and deliberately blunt. A finer curve would be a claim about how
 * repair relates to error, and nothing here has the field data to make one —
 * see the note on the thresholds.
 */
function discount(confidence: number | null, repaired: number): number | null {
  if (confidence === null) return null;
  return Math.max(0, confidence * (1 - Math.min(1, repaired)));
}
