/**
 * The capture layer, tested as far as it can be tested without hardware.
 *
 * Which is further than it looks. Everything in device.ts that decides
 * something — the record parse, the quality gate, the reassembly of BLE
 * notifications, the state machine — is pure or takes its device as an
 * argument, so all of it runs here against fixtures and fakes.
 *
 * What is NOT tested here, and cannot be, is whether the constants are right:
 * whether that vendor id is IriTech's, whether that UUID is the reader's,
 * whether the 19794-6 offsets match the standard. The fakes answer with
 * whatever the fixtures say, so a test passing proves the code handles a
 * conforming device and proves nothing whatever about the device.
 *
 * Two tests below exist to keep that honest: one asserts that no usable
 * protocol constant is exported anywhere, and one asserts that an unconfirmed
 * protocol makes the source refuse to open a chooser rather than open an empty
 * one.
 */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  BluetoothLFReader,
  CaptureSession,
  IRIS_HEADER_BYTES,
  IriShieldUSB,
  LineAssembler,
  ManualEntry,
  NORDIC_UART,
  PhoneCamera,
  QUALITY_GATE,
  REPRESENTATION_FIXED_AFTER_QUALITY,
  REPRESENTATION_FIXED_BEFORE_QUALITY,
  UNCONFIRMED_CONSTANTS,
  assessFrame,
  encodeYamaleIrisCode,
  irisTemplateFrom,
  isAnswer,
  outcomeFromError,
  parseIrisRecord,
  protocolProblems,
  readerLineToTag,
  step,
  yamaleIrisCodec,
  type BluetoothLike,
  type Capture,
  type CaptureSource,
  type DeviceDescriptor,
  type Frame,
  type FrameGrabber,
  type GattCharacteristicLike,
  type IriShieldProtocol,
  type Outcome,
  type SessionEvent,
  type SessionState,
  type UsbDeviceLike,
  type UsbLike,
} from './device.ts';
import type { IrisTemplate } from './iris.ts';

function value<T>(got: Outcome<T>): T {
  assert.equal(got.ok, true, got.ok ? '' : `${got.reason}: ${got.detail}`);
  return (got as { ok: true; value: T }).value;
}

function refusal<T>(got: Outcome<T>): { reason: string; detail: string } {
  assert.equal(got.ok, false, 'expected a failure');
  return got as { ok: false; reason: string; detail: string };
}

/* ============================================ ISO/IEC 19794-6 fixtures */

interface RepSpec {
  image: Uint8Array;
  eye?: number;
  quality?: number[];
  width?: number;
  height?: number;
  imageFormat?: number;
  /** Write a length other than the true one, to exercise the arithmetic check. */
  forceLength?: number;
  /** Write a quality-block count other than the true one. */
  forceQualityCount?: number;
}

/**
 * Build a record by hand, from the layout device.ts documents.
 *
 * Every offset here is written independently of the parser's constants — as
 * literal field-by-field appends — so that the two agreeing means something. A
 * fixture built by calling the parser's own offsets would agree with any
 * layout at all, including a wrong one.
 */
function buildRecord(
  reps: RepSpec[],
  options: { magic?: number[]; version?: string; forceRecordLength?: number; forceCount?: number } = {},
): Uint8Array {
  const bodies = reps.map(buildRepresentation);
  const total = IRIS_HEADER_BYTES + bodies.reduce((n, b) => n + b.length, 0);

  const out = new Uint8Array(options.forceRecordLength ?? total);
  const view = new DataView(out.buffer);
  out.set(options.magic ?? [0x49, 0x49, 0x52, 0x00], 0);
  out.set(new TextEncoder().encode((options.version ?? '020').padEnd(4, '\0')).subarray(0, 4), 4);
  view.setUint32(8, options.forceRecordLength ?? total, false);
  view.setUint16(12, options.forceCount ?? reps.length, false);
  out[14] = 0; // certification flag
  out[15] = reps.length; // eyes represented

  let at = IRIS_HEADER_BYTES;
  for (const body of bodies) {
    if (at + body.length > out.length) break;
    out.set(body, at);
    at += body.length;
  }
  return out;
}

function buildRepresentation(spec: RepSpec): Uint8Array {
  const quality = spec.quality ?? [];
  const declaredQualityCount = spec.forceQualityCount ?? quality.length;
  const length =
    spec.forceLength ??
    REPRESENTATION_FIXED_BEFORE_QUALITY +
      quality.length * 5 +
      REPRESENTATION_FIXED_AFTER_QUALITY +
      spec.image.length;

  const out = new Uint8Array(
    REPRESENTATION_FIXED_BEFORE_QUALITY +
      quality.length * 5 +
      REPRESENTATION_FIXED_AFTER_QUALITY +
      spec.image.length,
  );
  const view = new DataView(out.buffer);

  view.setUint32(0, length, false);
  view.setUint16(4, 2026, false); // year
  out[6] = 9; // month
  out[7] = 22; // day
  out[8] = 14; // hour
  out[9] = 30; // minute
  out[10] = 15; // second
  view.setUint16(11, 250, false); // milliseconds
  out[13] = 3; // capture device technology
  view.setUint16(14, 0x1234, false); // capture device vendor
  view.setUint16(16, 0x0002, false); // capture device type
  out[18] = declaredQualityCount;

  quality.forEach((score, i) => {
    const at = REPRESENTATION_FIXED_BEFORE_QUALITY + i * 5;
    out[at] = score;
    view.setUint16(at + 1, 0x1234, false);
    view.setUint16(at + 3, 0x0001, false);
  });

  const after = REPRESENTATION_FIXED_BEFORE_QUALITY + quality.length * 5;
  view.setUint16(after + 0, 1, false); // representation number
  out[after + 2] = spec.eye ?? 1;
  out[after + 3] = 3; // image type
  out[after + 4] = spec.imageFormat ?? 10; // image format
  out[after + 5] = 1; // horizontal orientation
  out[after + 6] = 1; // vertical orientation
  out[after + 7] = 0; // compression history
  view.setUint16(after + 8, spec.width ?? 640, false);
  view.setUint16(after + 10, spec.height ?? 480, false);
  out[after + 12] = 8; // bit depth
  view.setUint16(after + 13, 0, false); // range
  view.setUint16(after + 15, 0, false); // roll angle
  view.setUint16(after + 17, 0, false); // roll uncertainty
  view.setUint16(after + 19, 100, false); // iris centre smallest X
  view.setUint16(after + 21, 540, false); // largest X
  view.setUint16(after + 23, 80, false); // smallest Y
  view.setUint16(after + 25, 400, false); // largest Y
  view.setUint32(after + 27, spec.image.length, false);
  out.set(spec.image, after + REPRESENTATION_FIXED_AFTER_QUALITY);

  return out;
}

const IMAGE = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]);

test('a record built from the documented layout parses back to what went in', () => {
  const record = value(parseIrisRecord(buildRecord([{ image: IMAGE, quality: [88] }])));
  assert.equal(record.version, '020');
  assert.equal(record.eyesRepresented, 1);
  assert.equal(record.representations.length, 1);

  const rep = record.representations[0]!;
  assert.equal(rep.eye, 1);
  assert.equal(rep.width, 640);
  assert.equal(rep.height, 480);
  assert.equal(rep.bitDepth, 8);
  assert.equal(rep.captureDeviceVendor, 0x1234);
  assert.deepEqual(rep.quality, [{ score: 88, algorithmVendor: 0x1234, algorithm: 1 }]);
  assert.deepEqual(rep.image, IMAGE);
});

test('the capture time is read as UTC, not as the tablet local time', () => {
  // Design §4 makes the interval between enrolments load-bearing, and it is
  // compared against a chain block time. A record time in the reader's local
  // zone would drift by hours and the drift would be invisible.
  const rep = value(parseIrisRecord(buildRecord([{ image: IMAGE }]))).representations[0]!;
  assert.equal(rep.capturedAt.toISOString(), '2026-09-22T14:30:15.250Z');
});

test('two representations both parse, and the second starts where the first ended', () => {
  const second = new Uint8Array([9, 9, 9]);
  const record = value(
    parseIrisRecord(buildRecord([{ image: IMAGE, eye: 1 }, { image: second, eye: 2, quality: [10, 20] }])),
  );
  assert.equal(record.representations.length, 2);
  assert.deepEqual(record.representations[0]!.image, IMAGE);
  assert.deepEqual(record.representations[1]!.image, second);
  assert.equal(record.representations[1]!.eye, 2);
  assert.equal(record.representations[1]!.quality.length, 2);
});

test('a reply that is not a record says so, with the bytes that came back', () => {
  // The most useful error in the file. If the USB constants are wrong this is
  // where it surfaces, and the hex is what tells a developer that the device
  // answered with something rather than not answering.
  const reply = new Uint8Array(24);
  reply.set([0xde, 0xad, 0xbe, 0xef]);
  const got = refusal(parseIrisRecord(reply));
  assert.equal(got.reason, 'refused');
  assert.match(got.detail, /not an ISO 19794-6 record/);
  assert.match(got.detail, /de ad be ef/);
});

test('a truncated transfer is caught by the declared record length', () => {
  const full = buildRecord([{ image: IMAGE }]);
  const got = refusal(parseIrisRecord(full.subarray(0, full.length - 4)));
  assert.match(got.detail, /declares \d+ bytes and \d+ were read/);
});

test('bytes left over after the representations are a disagreement, not padding', () => {
  const full = buildRecord([{ image: IMAGE }]);
  const padded = new Uint8Array(full.length + 8);
  padded.set(full);
  new DataView(padded.buffer).setUint32(8, padded.length, false);
  const got = refusal(parseIrisRecord(padded));
  assert.match(got.detail, /bytes left after 1 representations/);
});

test('a representation whose length arithmetic does not add up names the arithmetic', () => {
  // THE test that protects against a wrong header offset. If
  // REPRESENTATION_FIXED_AFTER_QUALITY is off by even one, a real device's
  // record fails here with a message that contains every term of the sum — so
  // the failure is a diagnosis rather than "could not parse".
  // Four bytes short of the truth: inside the record, so the bounds checks pass
  // and only the arithmetic catches it. That is exactly what an offset constant
  // that is off by four would look like against a real device.
  const honest = REPRESENTATION_FIXED_BEFORE_QUALITY + REPRESENTATION_FIXED_AFTER_QUALITY + IMAGE.length;
  const got = refusal(parseIrisRecord(buildRecord([{ image: IMAGE, forceLength: honest - 4 }])));
  assert.match(got.detail, /length arithmetic disagrees/);
  assert.match(got.detail, /ISO\/IEC 19794-6:2011/);
});

test('quality blocks that cannot fit inside the declared length are refused', () => {
  const record = buildRecord([{ image: IMAGE, quality: [50], forceQualityCount: 200 }]);
  const got = refusal(parseIrisRecord(record));
  assert.equal(got.reason, 'refused');
  assert.match(got.detail, /quality blocks/);
});

test('a header too short to be a header does not read past the end', () => {
  assert.equal(refusal(parseIrisRecord(new Uint8Array(4))).reason, 'refused');
  assert.equal(refusal(parseIrisRecord(new Uint8Array(0))).reason, 'refused');
});

test('a record with no representations is well-formed and empty', () => {
  const record = value(parseIrisRecord(buildRecord([])));
  assert.equal(record.representations.length, 0);
});

/* ---------------------------------------------------------- the template */

function template(bits = 256, fill = 0xa5): IrisTemplate {
  return {
    code: new Uint8Array(bits / 8).fill(fill),
    mask: new Uint8Array(bits / 8).fill(0xff),
    bits,
  };
}

test('without a codec, a representation yields no template and says why', () => {
  // The honesty this whole module turns on. ISO/IEC 19794-6 carries an IMAGE.
  // The iris code iris.ts compares is not in it and is not in any standard.
  const rep = value(parseIrisRecord(buildRecord([{ image: IMAGE }]))).representations[0]!;
  const got = refusal(irisTemplateFrom(rep));
  assert.equal(got.reason, 'refused');
  assert.match(got.detail, /does not carry an iris code/);
});

test('our own container round-trips through a record and into a template', () => {
  const original = template();
  const record = buildRecord([{ image: encodeYamaleIrisCode(original) }]);
  const rep = value(parseIrisRecord(record)).representations[0]!;
  const got = value(irisTemplateFrom(rep, yamaleIrisCodec));
  assert.equal(got.bits, original.bits);
  assert.deepEqual(got.code, original.code);
  assert.deepEqual(got.mask, original.mask);
});

test('the codec refuses anything that is not its own container', () => {
  const rep = value(parseIrisRecord(buildRecord([{ image: new Uint8Array(24).fill(7) }]))).representations[0]!;
  assert.match(refusal(yamaleIrisCodec(rep)).detail, /not a YIC1 template/);

  const stub = value(parseIrisRecord(buildRecord([{ image: IMAGE }]))).representations[0]!;
  assert.match(refusal(yamaleIrisCodec(stub)).detail, /too short for a YIC1 template/);
});

test('a code and mask of different lengths are refused, not quietly compared', () => {
  // iris.ts reads a short array as zeroes, and an all-zero mask means "nothing
  // comparable" — which surfaces as every comparison being inconclusive rather
  // than as an error, and is the kind of fault that takes a week to find.
  const bad = encodeYamaleIrisCode({ code: new Uint8Array(32), mask: new Uint8Array(16), bits: 256 });
  const rep = value(parseIrisRecord(buildRecord([{ image: bad }]))).representations[0]!;
  assert.match(refusal(yamaleIrisCodec(rep)).detail, /code is 32 bytes and mask is 16/);
});

test('a template claiming more bits than it carries is refused', () => {
  const bad = encodeYamaleIrisCode({ code: new Uint8Array(8), mask: new Uint8Array(8), bits: 2048 });
  const rep = value(parseIrisRecord(buildRecord([{ image: bad }]))).representations[0]!;
  assert.match(refusal(yamaleIrisCodec(rep)).detail, /declares 2048 bits but carries only 64/);
});

/* ================================================== the quality gate */

function frame(
  width: number,
  height: number,
  at: (x: number, y: number) => number,
): Pick<Frame, 'width' | 'height' | 'luma'> {
  const luma = new Uint8Array(width * height);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) luma[y * width + x] = Math.max(0, Math.min(255, Math.round(at(x, y))));
  }
  return { width, height, luma };
}

/** A checkerboard well inside the range, so sharpness is tested and not clipping. */
const sharp = frame(64, 64, (x, y) => ((x >> 1) + (y >> 1)) % 2 === 0 ? 80 : 176);
/** The same picture through 3x3 box blurs, which is what missing focus looks like. */
function boxBlur(
  src: Pick<Frame, 'width' | 'height' | 'luma'>,
  passes: number,
): Pick<Frame, 'width' | 'height' | 'luma'> {
  let current = src;
  for (let pass = 0; pass < passes; pass++) {
    const previous = current;
    current = frame(previous.width, previous.height, (x, y) => {
      let sum = 0;
      let n = 0;
      for (let dy = -1; dy <= 1; dy++) {
        for (let dx = -1; dx <= 1; dx++) {
          const px = x + dx;
          const py = y + dy;
          if (px < 0 || py < 0 || px >= previous.width || py >= previous.height) continue;
          sum += previous.luma[py * previous.width + px]!;
          n++;
        }
      }
      return sum / n;
    });
  }
  return current;
}

test('a sharp, well-exposed, contrasty capture passes', () => {
  const report = assessFrame(sharp);
  assert.equal(report.pass, true, JSON.stringify(report));
  assert.deepEqual(report.failures, []);
});

test('a flat grey card fails however clean it is', () => {
  // Nothing is in focus because nothing is there. A gate that passed this would
  // pass a lens cap.
  const report = assessFrame(frame(32, 32, () => 128));
  assert.equal(report.pass, false);
  assert.equal(report.sharpness, 0);
  assert.equal(report.contrast, 0);
  assert.equal(report.score, 0);
  assert.ok(report.failures.includes('herd.captureBlurred'));
  assert.ok(report.failures.includes('herd.captureFlat'));
});

test('a uniform frame scores zero sharpness, which proves the border is excluded', () => {
  // If the Laplacian included the border and treated the missing neighbours as
  // zero, a uniform frame would have a bright false edge all the way round and
  // would score as sharp — and a completely defocused muzzle would pass.
  assert.equal(assessFrame(frame(16, 16, () => 200)).sharpness, 0);
  assert.equal(assessFrame(frame(16, 16, () => 30)).sharpness, 0);
});

test('blur lowers the sharpness score, monotonically', () => {
  // The absolute numbers are arbitrary until they are calibrated against field
  // captures. The ORDERING is not, and the ordering is what the gate relies on.
  const scores = [0, 1, 2, 3].map((passes) => assessFrame(boxBlur(sharp, passes)).sharpness);
  for (let i = 1; i < scores.length; i++) {
    assert.ok(scores[i]! <= scores[i - 1]!, `pass ${i}: ${scores[i]} is not below ${scores[i - 1]}`);
  }
  assert.ok(scores[3]! < QUALITY_GATE.sharpness, 'a thoroughly defocused frame must fail the gate');
});

test('a defocused capture is refused, on one measure or another', () => {
  // Worth stating as its own test because the FIRST measure to catch a soft
  // capture is not always sharpness. One box-blur pass leaves the Laplacian
  // high — a blurred hard edge still has a strong second derivative — while the
  // amplitude collapses, so contrast is what fails. Both are real properties of
  // an out-of-focus muzzle, and the gate is a set of measures rather than one.
  for (const passes of [1, 2, 3]) {
    const report = assessFrame(boxBlur(sharp, passes));
    assert.equal(report.pass, false, `${passes} passes of blur should not be enrollable`);
  }
  assert.ok(assessFrame(boxBlur(sharp, 3)).failures.includes('herd.captureBlurred'));
});

test('a blown-out capture fails on clipping and on exposure', () => {
  // The wet muzzle under a single LED: specular highlight over the detail.
  const report = assessFrame(frame(32, 32, (x, y) => (x + y) % 2 === 0 ? 255 : 250));
  assert.equal(report.pass, false);
  assert.ok(report.failures.includes('herd.captureClipped'));
  assert.ok(report.failures.includes('herd.captureTooBright'));
  assert.ok(!report.failures.includes('herd.captureTooDark'), 'the direction must be right');
});

test('a dark capture is named as dark and not merely as badly exposed', () => {
  const report = assessFrame(frame(32, 32, (x, y) => ((x + y) % 2 === 0 ? 0 : 6)));
  assert.equal(report.pass, false);
  assert.ok(report.failures.includes('herd.captureTooDark'));
  assert.ok(!report.failures.includes('herd.captureTooBright'));
});

test('the score is the worst component and not the average', () => {
  // A razor-sharp, high-contrast, completely blown-out capture. Averaged it
  // looks fine; the blown-out part is where the muzzle detail was.
  const report = assessFrame(frame(32, 32, (x, y) => ((x + y) % 2 === 0 ? 255 : 0)));
  assert.equal(report.score, Math.min(report.sharpness, report.exposure, report.clipping, report.contrast));
  assert.equal(report.clipping, 0);
  assert.equal(report.score, 0);
  assert.ok(report.sharpness > 0.9, 'it really is sharp, which is the point');
});

test('a malformed frame is refused rather than measured', () => {
  const report = assessFrame({ width: 10, height: 10, luma: new Uint8Array(4) });
  assert.equal(report.pass, false);
  assert.deepEqual(report.failures, ['herd.frameMalformed']);
  assert.equal(assessFrame({ width: 1, height: 1, luma: new Uint8Array(1) }).pass, false);
});

test('every gate threshold is between zero and one', () => {
  for (const [name, threshold] of Object.entries(QUALITY_GATE)) {
    assert.ok(threshold > 0 && threshold < 1, `${name} is ${threshold}`);
  }
});

/* ============================================= BLE line reassembly */

test('a line split across notification packets is reassembled', () => {
  // The bug this prevents: a BLE notification carries 20 bytes at the default
  // MTU, and `LA982000000000003\r\n` is 19 characters plus a terminator. A
  // handler that parses each notification alone sees a fragment and throws away
  // a good read — intermittently, whenever the line happens to fit.
  const lines = new LineAssembler();
  const bytes = new TextEncoder().encode('LA982000000000003\r\n');
  // Split inside the digits, which is where a 20-byte MTU actually cuts it.
  assert.deepEqual(lines.push(bytes.subarray(0, 12)), [], 'a fragment is not a read');
  assert.deepEqual(lines.push(bytes.subarray(12)), ['LA982000000000003']);
});

test('CR, LF and CRLF all end a line, because readers disagree', () => {
  const enc = new TextEncoder();
  for (const terminator of ['\r', '\n', '\r\n']) {
    const lines = new LineAssembler();
    assert.deepEqual(lines.push(enc.encode(`982000000000003${terminator}`)), ['982000000000003']);
  }
});

test('several lines in one packet all come out, in order', () => {
  const lines = new LineAssembler();
  assert.deepEqual(lines.push(new TextEncoder().encode('READY\r\n982000000000003\r\n')), [
    'READY',
    '982000000000003',
  ]);
});

test('a line with no terminator is held, and flush gives it up', () => {
  const lines = new LineAssembler();
  assert.deepEqual(lines.push(new TextEncoder().encode('982000000000')), []);
  assert.deepEqual(lines.push(new TextEncoder().encode('003')), []);
  assert.deepEqual(lines.flush(), ['982000000000003']);
  assert.deepEqual(lines.flush(), [], 'flush empties');
});

test('a reader that never terminates a line cannot grow the buffer forever', () => {
  const lines = new LineAssembler(64);
  for (let i = 0; i < 50; i++) lines.push(new TextEncoder().encode('0123456789'));
  assert.ok(lines.flush()[0]!.length <= 64);
});

/* ------------------------------------------------- the line to a tag code */

test('a prefixed reader line yields the transponder code', () => {
  assert.equal(value(readerLineToTag('LA982000000000003')).country, '982');
  assert.equal(value(readerLineToTag('  982000000000003  ')).national, '000000000003');
  assert.equal(value(readerLineToTag('982000000000003,A,1')).country, '982');
});

test('a status line is not a read, and is not an error either', () => {
  // Readers emit battery lines, ready lines and echoes of their own commands.
  // Reporting the first one as a bad read would abort a scan that is working.
  assert.equal(refusal(readerLineToTag('READY')).reason, 'not-found');
  assert.equal(refusal(readerLineToTag('BATT 87%')).reason, 'not-found');
  assert.equal(refusal(readerLineToTag('')).reason, 'not-found');
});

test('sixteen digits is not a fifteen-digit code with an extra character', () => {
  // It is a format this parser does not understand, and slicing fifteen out of
  // it would enrol a tag nobody read.
  assert.equal(refusal(readerLineToTag('9820000000000031')).reason, 'not-found');
  assert.equal(refusal(readerLineToTag('19820000000000 3')).reason, 'not-found');
});

test('fifteen digits that are not a valid code stop the scan rather than being skipped', () => {
  // The reader read something and this did not understand it. That is a
  // protocol problem and must be loud, unlike a status line.
  const got = refusal(readerLineToTag('982999999999999'));
  assert.equal(got.reason, 'refused');
  assert.match(got.detail, /herd.tagNationalRange/);
});

/* ================================================ the IriShield over USB */

const UNFILLED: IriShieldProtocol = {
  vendorId: 0,
  productId: 0,
  configurationValue: 1,
  interfaceNumber: 0,
  endpointIn: 0,
  endpointOut: 0,
  captureCommand: new Uint8Array(0),
  maxReplyBytes: 0,
  captureTimeoutMs: 0,
};

/** A protocol shaped like a real one. Every number here is made up, for a fake. */
const FAKE_PROTOCOL: IriShieldProtocol = {
  vendorId: 0xf00d,
  productId: 0xbeef,
  configurationValue: 1,
  interfaceNumber: 0,
  endpointIn: 1,
  endpointOut: 2,
  captureCommand: new Uint8Array([0x01]),
  maxReplyBytes: 65536,
  captureTimeoutMs: 5000,
};

function fakeUsb(device: Partial<UsbDeviceLike> & { transferIn: UsbDeviceLike['transferIn'] }): {
  usb: UsbLike;
  calls: string[];
} {
  const calls: string[] = [];
  const full: UsbDeviceLike = {
    vendorId: FAKE_PROTOCOL.vendorId,
    productId: FAKE_PROTOCOL.productId,
    manufacturerName: 'IriTech',
    productName: 'IriShield MK 2120U',
    serialNumber: 'SN-1',
    opened: false,
    open: async () => void calls.push('open'),
    close: async () => void calls.push('close'),
    selectConfiguration: async () => void calls.push('selectConfiguration'),
    claimInterface: async () => void calls.push('claimInterface'),
    releaseInterface: async () => void calls.push('releaseInterface'),
    transferOut: async () => ({ status: 'ok', bytesWritten: 1 }),
    controlTransferOut: async () => ({ status: 'ok' }),
    ...device,
  };
  return {
    calls,
    usb: {
      requestDevice: async () => {
        calls.push('requestDevice');
        return full;
      },
      getDevices: async () => [full],
    },
  };
}

function asDataView(bytes: Uint8Array): DataView {
  return new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
}

test('an unconfirmed protocol makes the source unavailable and opens no chooser', async () => {
  // The rule the whole file is built around. Filtering on a placeholder vendor
  // id shows an EMPTY chooser, and an empty chooser is indistinguishable from a
  // device that is not plugged in — so it must not get that far.
  const { usb, calls } = fakeUsb({ transferIn: async () => ({ status: 'ok' }) });
  const source = new IriShieldUSB(UNFILLED, usb);

  assert.equal(source.available(), false);
  const got = refusal(await source.connect());
  assert.equal(got.reason, 'unsupported');
  assert.match(got.detail, /not confirmed/);
  assert.deepEqual(calls, [], 'requestDevice must not be reached');
});

test('the protocol validator names every hole rather than the first one', () => {
  const problems = protocolProblems(UNFILLED);
  assert.ok(problems.length >= 5, problems.join('; '));
  assert.ok(problems.some((p) => p.includes('vendorId')));
  assert.ok(problems.some((p) => p.includes('endpointIn')));
  assert.ok(problems.some((p) => p.includes('captureCommand')));
  assert.deepEqual(protocolProblems(FAKE_PROTOCOL), []);
});

test('no protocol supplied at all is its own complaint', () => {
  assert.match(protocolProblems(undefined)[0]!, /no protocol supplied/);
});

test('a bulk endpoint of zero is caught: zero is the control endpoint', () => {
  // The signature of a placeholder somebody meant to come back to.
  assert.ok(protocolProblems({ ...FAKE_PROTOCOL, endpointOut: 0 }).some((p) => p.includes('endpointOut')));
  assert.ok(protocolProblems({ ...FAKE_PROTOCOL, endpointIn: 16 }).some((p) => p.includes('endpointIn')));
});

test('a device that returns a record yields a capture with a template', async () => {
  const original = template();
  const record = buildRecord([{ image: encodeYamaleIrisCode(original), quality: [77] }]);
  const { usb, calls } = fakeUsb({
    transferIn: async () => ({ status: 'ok', data: asDataView(record) }),
  });
  const source = new IriShieldUSB(FAKE_PROTOCOL, usb, yamaleIrisCodec);

  assert.equal(source.available(), true);
  const descriptor = value(await source.connect());
  assert.equal(descriptor.handle, 'SN-1');
  assert.match(descriptor.label, /IriShield/);
  assert.deepEqual(calls, ['requestDevice', 'open', 'selectConfiguration', 'claimInterface']);

  const capture = value(await source.capture());
  assert.equal(capture.modality, 'iris');
  assert.deepEqual(capture.payload, record, 'the whole record is what gets committed');
  assert.equal(capture.quality, 0.77, "the device's own quality block, normalised");
  assert.deepEqual(capture.template?.code, original.code);

  await source.disconnect();
  assert.ok(calls.includes('releaseInterface') && calls.includes('close'));
});

test('without a codec the capture still succeeds, and says it carries no template', async () => {
  // Design §2 puts templates in the biometric service anyway. An enrolment of
  // the record is worth recording; pretending a template came back is not.
  const record = buildRecord([{ image: IMAGE, quality: [90] }]);
  const { usb } = fakeUsb({ transferIn: async () => ({ status: 'ok', data: asDataView(record) }) });
  const source = new IriShieldUSB(FAKE_PROTOCOL, usb);
  await source.connect();

  const capture = value(await source.capture());
  assert.equal(capture.template, undefined);
  assert.match(capture.note!, /no template/);
});

test('an empty reply is "no iris was captured", not a broken device', async () => {
  // The commonest outcome in a pen. It means hold still and try again, and the
  // state machine keeps the device connected for exactly that reason.
  const { usb } = fakeUsb({ transferIn: async () => ({ status: 'ok' }) });
  const source = new IriShieldUSB(FAKE_PROTOCOL, usb);
  await source.connect();
  assert.equal(refusal(await source.capture()).reason, 'not-found');
});

test('a stalled endpoint is reported as the device rejecting the command', async () => {
  // Which is what a guessed captureCommand looks like from the outside.
  const { usb } = fakeUsb({ transferIn: async () => ({ status: 'stall' }) });
  const source = new IriShieldUSB(FAKE_PROTOCOL, usb);
  await source.connect();
  const got = refusal(await source.capture());
  assert.equal(got.reason, 'refused');
  assert.match(got.detail, /stalled/);
});

test('a reply that is not a record is refused with the bytes, not accepted', async () => {
  const { usb } = fakeUsb({
    transferIn: async () => ({ status: 'ok', data: asDataView(new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16])) }),
  });
  const source = new IriShieldUSB(FAKE_PROTOCOL, usb);
  await source.connect();
  assert.match(refusal(await source.capture()).detail, /not an ISO 19794-6 record/);
});

test('a transferIn that never resolves times out instead of hanging', async () => {
  // A wrong endpoint number does not throw. It waits, forever, for bytes that
  // are not coming, and the bug report says "it hangs".
  // 1000ms rather than something quicker, because `protocolProblems` refuses a
  // shorter capture timeout than an animal takes to hold still — and a test
  // that reached in past the validator would be testing a protocol that cannot
  // be configured.
  const { usb } = fakeUsb({ transferIn: () => new Promise(() => {}) });
  const source = new IriShieldUSB({ ...FAKE_PROTOCOL, captureTimeoutMs: 1000 }, usb);
  await source.connect();
  const got = refusal(await source.capture());
  assert.equal(got.reason, 'unreachable');
  assert.match(got.detail, /timed out after 1000ms/);
});

test('an interface the operating system holds gets the sentence that explains it', async () => {
  // The failure that decides whether a browser can drive this device at all,
  // rather than one that can be fixed in code.
  const claimFailure = new Error('Unable to claim interface.');
  claimFailure.name = 'InvalidStateError';
  const { usb } = fakeUsb({
    transferIn: async () => ({ status: 'ok' }),
    claimInterface: async () => {
      throw claimFailure;
    },
  });
  const got = refusal(await new IriShieldUSB(FAKE_PROTOCOL, usb).connect());
  assert.equal(got.reason, 'refused');
  assert.match(got.detail, /already bound/);
  assert.match(got.detail, /WebUSB/);
});

test('no WebUSB at all is unsupported, and names iOS', async () => {
  const got = refusal(await new IriShieldUSB(FAKE_PROTOCOL, undefined).connect());
  assert.equal(got.reason, 'unsupported');
  assert.match(got.detail, /iOS/);
});

test('capturing before connecting is unreachable, not a crash', async () => {
  const { usb } = fakeUsb({ transferIn: async () => ({ status: 'ok' }) });
  assert.equal(refusal(await new IriShieldUSB(FAKE_PROTOCOL, usb).capture()).reason, 'unreachable');
});

/* ==================================================== the phone camera */

function goodFrame(): Frame {
  return { ...sharp, encoded: new Uint8Array([0xff, 0xd8, 0xff]), mime: 'image/jpeg' };
}
function badFrame(): Frame {
  return { ...frame(32, 32, () => 128), encoded: new Uint8Array([0xff]), mime: 'image/jpeg' };
}

function fakeGrabber(frames: Outcome<Frame>[]): FrameGrabber & { stopped: number } {
  const descriptor: DeviceDescriptor = { kind: 'phone-camera', label: 'back camera', handle: 'cam-0' };
  const state = { stopped: 0 };
  return {
    get stopped() {
      return state.stopped;
    },
    start: async () => ({ ok: true, value: descriptor }),
    grab: async () => frames.shift() ?? { ok: false, reason: 'not-found', detail: 'no more frames' },
    stop: async () => void state.stopped++,
  };
}

test('a good frame becomes a capture whose quality is the gate score', async () => {
  const camera = new PhoneCamera('muzzle', fakeGrabber([{ ok: true, value: goodFrame() }]));
  await camera.connect();
  const capture = value(await camera.capture());
  assert.equal(capture.modality, 'muzzle');
  assert.equal(capture.quality, assessFrame(sharp).score);
  assert.deepEqual(capture.payload, new Uint8Array([0xff, 0xd8, 0xff]));
  assert.match(capture.note!, /64x64 image\/jpeg/);
});

test('a bad capture is refused, and the refusal lists every reason', async () => {
  // Design §7 step one: an app that scores a capture and REFUSES a bad one
  // replaces a paper photograph with something checkable, before any matching
  // exists at all. `refused` and not `not-found`: the camera worked.
  const camera = new PhoneCamera('muzzle', fakeGrabber([{ ok: true, value: badFrame() }]));
  await camera.connect();
  const got = refusal(await camera.capture());
  assert.equal(got.reason, 'refused');
  assert.match(got.detail, /herd.captureBlurred/);
  assert.match(got.detail, /herd.captureFlat/);
  assert.match(got.detail, /sharpness 0.00/);
});

test('capturing before starting the camera is unreachable', async () => {
  const camera = new PhoneCamera('face', fakeGrabber([]));
  assert.equal(refusal(await camera.capture()).reason, 'unreachable');
});

test('disconnecting stops the grabber, so the camera light goes out', async () => {
  const grabber = fakeGrabber([]);
  const camera = new PhoneCamera('coat', grabber);
  await camera.connect();
  await camera.disconnect();
  assert.equal(grabber.stopped, 1);
  assert.equal(refusal(await camera.capture()).reason, 'unreachable');
});

/* ============================================== the LF reader over BLE */

class FakeCharacteristic implements GattCharacteristicLike {
  value?: DataView;
  written: Uint8Array[] = [];
  notifying = false;
  private listeners: ((event: Event) => void)[] = [];

  async startNotifications() {
    this.notifying = true;
    return this;
  }
  async stopNotifications() {
    this.notifying = false;
    return this;
  }
  async writeValue(v: Uint8Array) {
    this.written.push(v);
  }
  addEventListener(_type: string, listener: (event: Event) => void) {
    this.listeners.push(listener);
  }
  removeEventListener(_type: string, listener: (event: Event) => void) {
    this.listeners = this.listeners.filter((l) => l !== listener);
  }
  get listenerCount() {
    return this.listeners.length;
  }

  /** What a notification looks like from the page's side. */
  notify(text: string): void {
    const bytes = new TextEncoder().encode(text);
    this.value = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    for (const l of [...this.listeners]) l({ target: { value: this.value } } as unknown as Event);
  }
}

function fakeBluetooth(characteristic: FakeCharacteristic, options: { serviceError?: Error } = {}): BluetoothLike {
  return {
    requestDevice: async () => ({
      id: 'abc123',
      name: 'Stick Reader',
      gatt: {
        connected: true,
        connect: async function () {
          return this;
        },
        disconnect: () => {},
        getPrimaryService: async () => {
          if (options.serviceError) throw options.serviceError;
          return { getCharacteristic: async () => characteristic };
        },
      },
    }),
  };
}

const READER = {
  serviceUuid: NORDIC_UART.serviceUuid,
  notifyUuid: NORDIC_UART.notifyUuid,
  writeUuid: NORDIC_UART.writeUuid,
  readTimeoutMs: 2000,
};

test('a tag arriving across two notifications is read', async () => {
  const characteristic = new FakeCharacteristic();
  const reader = new BluetoothLFReader(READER, fakeBluetooth(characteristic));

  const descriptor = value(await reader.connect());
  assert.equal(descriptor.handle, 'bt:abc123');
  assert.equal(characteristic.notifying, true);

  const pending = reader.capture();
  characteristic.notify('LA98200000');
  characteristic.notify('0000003\r\n');

  const capture = value(await pending);
  assert.equal(capture.modality, 'tag');
  assert.deepEqual(capture.payload, new TextEncoder().encode('982000000000003'));
  assert.equal(capture.quality, null, 'a transponder read has no degraded form');
  assert.match(capture.note!, /manufacturer/);
});

test('status lines before the read are stepped over, not treated as failures', async () => {
  const characteristic = new FakeCharacteristic();
  const reader = new BluetoothLFReader(READER, fakeBluetooth(characteristic));
  await reader.connect();

  const pending = reader.capture();
  characteristic.notify('READY\r\nBATT 92%\r\n');
  characteristic.notify('686000000000042\r\n');
  const capture = value(await pending);
  assert.deepEqual(capture.payload, new TextEncoder().encode('686000000000042'));
});

test('a test transponder is refused at the reader, not enrolled', async () => {
  // Country code 999. They are handed out at trade shows and fitted to
  // demonstration animals, and one in a national register is a record pointing
  // at a chip that was never meant to identify anything.
  const characteristic = new FakeCharacteristic();
  const reader = new BluetoothLFReader(READER, fakeBluetooth(characteristic));
  await reader.connect();

  const pending = reader.capture();
  characteristic.notify('999000000000001\r\n');
  const got = refusal(await pending);
  assert.equal(got.reason, 'refused');
  assert.match(got.detail, /test transponder/);
});

test('no tag in range is not-found, with advice rather than an error code', async () => {
  const characteristic = new FakeCharacteristic();
  const reader = new BluetoothLFReader({ ...READER, readTimeoutMs: 30 }, fakeBluetooth(characteristic));
  await reader.connect();
  const got = refusal(await reader.capture());
  assert.equal(got.reason, 'not-found');
  assert.match(got.detail, /hold the reader closer/);
});

test('the listener is removed after every capture, however it ended', async () => {
  // A listener left attached per capture accumulates over a day's work and
  // resolves an old promise with a new animal's tag.
  const characteristic = new FakeCharacteristic();
  const reader = new BluetoothLFReader({ ...READER, readTimeoutMs: 20 }, fakeBluetooth(characteristic));
  await reader.connect();
  await reader.capture();
  await reader.capture();
  assert.equal(characteristic.listenerCount, 0);
});

test('a scan command is written when the protocol has one', async () => {
  const characteristic = new FakeCharacteristic();
  const reader = new BluetoothLFReader(
    { ...READER, readTimeoutMs: 20, scanCommand: new Uint8Array([0x53]) },
    fakeBluetooth(characteristic),
  );
  await reader.connect();
  await reader.capture();
  assert.deepEqual(characteristic.written, [new Uint8Array([0x53])]);
});

test('a missing service says the reader may be Bluetooth Classic, which is unreachable', async () => {
  // The procurement decision, surfaced at the moment somebody hits it. A wrong
  // UUID and a serial-port reader look identical from here, and both need a
  // person rather than a retry.
  const error = new Error('No Services matching UUID found in Device.');
  error.name = 'NotFoundError';
  const reader = new BluetoothLFReader(READER, fakeBluetooth(new FakeCharacteristic(), { serviceError: error }));
  const got = refusal(await reader.connect());
  assert.equal(got.reason, 'unsupported');
  assert.match(got.detail, /Bluetooth Classic serial/);
});

test('no Web Bluetooth at all is unsupported, and names iOS', async () => {
  const reader = new BluetoothLFReader(READER, undefined);
  assert.equal(reader.available(), false);
  const got = refusal(await reader.connect());
  assert.equal(got.reason, 'unsupported');
  assert.match(got.detail, /iOS/);
});

test('the Nordic UUIDs are the published ones and are not silently reused', () => {
  // They are real, published by Nordic Semiconductor, and a great many BLE
  // serial bridges use them. Whether YOUR reader does is a different question
  // and is in UNCONFIRMED_CONSTANTS.
  assert.equal(NORDIC_UART.serviceUuid, '6e400001-b5a3-f393-e0a9-e50e24dcca9e');
  assert.equal(NORDIC_UART.notifyUuid, '6e400003-b5a3-f393-e0a9-e50e24dcca9e');
  assert.equal(NORDIC_UART.writeUuid, '6e400002-b5a3-f393-e0a9-e50e24dcca9e');
  assert.notEqual(NORDIC_UART.notifyUuid, NORDIC_UART.writeUuid, 'notify and write are not the same one');
});

/* ==================================================== manual entry */

test('manual entry is always available, in every configuration there is', () => {
  // Design §1.1: a tattoo in a dusty ear at dusk is sometimes simply
  // unreadable. An application that can reach a state where an operator cannot
  // record what they are looking at gets closed and replaced with a notebook.
  assert.equal(new ManualEntry('tattoo', async () => null).available(), true);
  assert.equal(new ManualEntry('tag', async () => null).available(), true);
  assert.equal(new ManualEntry('national-id', async () => null).available(), true);
  assert.deepEqual(new ManualEntry('tag', async () => null).unconfirmed(), []);
});

test('a typed national identifier becomes a capture with no quality score', async () => {
  const entry = new ManualEntry('national-id', async () => ' 221/682/021029/l ');
  await entry.connect();
  const capture = value(await entry.capture());
  assert.equal(capture.modality, 'national-id');
  assert.equal(capture.quality, null, 'there is no such thing as a good typed identifier');
  assert.deepEqual(capture.payload, new TextEncoder().encode('221/682/021029/L'));
  assert.equal(capture.device.kind, 'manual');
});

test('a typed transponder code records that it was typed', async () => {
  // The same field filled by the reader means something stronger, and the
  // enrolment record has to be able to tell them apart.
  const entry = new ManualEntry('tag', async () => '982 000 000 000 003');
  await entry.connect();
  const capture = value(await entry.capture());
  assert.deepEqual(capture.payload, new TextEncoder().encode('982000000000003'));
  assert.match(capture.note!, /no checksum/);
});

test('a tattoo is taken as written, because no format is agreed', async () => {
  const entry = new ManualEntry('tattoo', async () => '  AB   123 ');
  await entry.connect();
  const capture = value(await entry.capture());
  assert.deepEqual(capture.payload, new TextEncoder().encode('AB 123'));
  assert.deepEqual(capture.identifier, { tattoo: 'AB 123' });
});

test('cancelling is not-found, so pressing cancel does not raise an error', async () => {
  const entry = new ManualEntry('tag', async () => null);
  await entry.connect();
  assert.equal(refusal(await entry.capture()).reason, 'not-found');

  const blank = new ManualEntry('tag', async () => '   ');
  await blank.connect();
  assert.equal(refusal(await blank.capture()).reason, 'not-found');
});

test('a malformed typed identifier is refused, with the parser key in the detail', async () => {
  const entry = new ManualEntry('national-id', async () => '221/682/021029');
  await entry.connect();
  const got = refusal(await entry.capture());
  assert.equal(got.reason, 'refused');
  assert.match(got.detail, /herd.idFieldCount/);

  const tag = new ManualEntry('tag', async () => '999000000000001');
  await tag.connect();
  assert.match(refusal(await tag.capture()).detail, /test transponder/);
});

/* ================================================= the state machine */

test('a failed capture goes somewhere different depending on WHY it failed', () => {
  // The whole reason the Outcome has four reasons rather than one. Collapse
  // them and you get an app that either makes an operator re-pair a reader
  // after every missed tag, or leaves them tapping capture against a stick
  // that is in the truck.
  assert.equal(step('capturing', { type: 'capture-failed', reason: 'not-found' }), 'ready');
  assert.equal(step('capturing', { type: 'capture-failed', reason: 'refused' }), 'ready');
  assert.equal(step('capturing', { type: 'capture-failed', reason: 'unreachable' }), 'idle');
  assert.equal(step('capturing', { type: 'capture-failed', reason: 'unsupported' }), 'failed');
});

test('a failed connection returns to idle unless it can never work', () => {
  assert.equal(step('connecting', { type: 'connect-failed', reason: 'not-found' }), 'idle');
  assert.equal(step('connecting', { type: 'connect-failed', reason: 'refused' }), 'idle');
  assert.equal(step('connecting', { type: 'connect-failed', reason: 'unreachable' }), 'idle');
  assert.equal(step('connecting', { type: 'connect-failed', reason: 'unsupported' }), 'failed');
});

test('a second tap while connecting or capturing changes nothing', () => {
  // Gloves, sun, an animal moving. Two requestDevice calls or two overlapping
  // GATT connections is a real bug with a confusing symptom.
  assert.equal(step('connecting', { type: 'connect' }), 'connecting');
  assert.equal(step('capturing', { type: 'capture' }), 'capturing');
});

test('capturing from idle is a no-op rather than a failure', () => {
  // A mistimed tap must not look like a broken device.
  assert.equal(step('idle', { type: 'capture' }), 'idle');
  assert.equal(step('idle', { type: 'captured' }), 'idle');
});

test('the happy path runs idle to ready to capturing and back', () => {
  let state: SessionState = 'idle';
  state = step(state, { type: 'connect' });
  assert.equal(state, 'connecting');
  state = step(state, { type: 'connected' });
  assert.equal(state, 'ready');
  state = step(state, { type: 'capture' });
  assert.equal(state, 'capturing');
  state = step(state, { type: 'captured' });
  assert.equal(state, 'ready');
  state = step(state, { type: 'capture' });
  state = step(state, { type: 'captured' });
  assert.equal(state, 'ready', 'a second capture needs no reconnection');
});

test('failed needs a retry, and disconnect always returns to idle', () => {
  assert.equal(step('failed', { type: 'capture' }), 'failed');
  assert.equal(step('failed', { type: 'connect' }), 'failed');
  assert.equal(step('failed', { type: 'retry' }), 'idle');
  for (const from of ['connecting', 'ready', 'capturing', 'failed'] as SessionState[]) {
    assert.equal(step(from, { type: 'disconnect' }), 'idle', from);
  }
});

test('closed is terminal, from every state and against every event', () => {
  const events: SessionEvent[] = [
    { type: 'connect' },
    { type: 'connected' },
    { type: 'capture' },
    { type: 'captured' },
    { type: 'retry' },
    { type: 'disconnect' },
    { type: 'connect-failed', reason: 'refused' },
    { type: 'capture-failed', reason: 'not-found' },
  ];
  for (const event of events) assert.equal(step('closed', event), 'closed', event.type);
});

test('the machine is total: every state and every event has an answer', () => {
  // A transition table with a hole in it throws in the field, in a pen, on a
  // combination nobody tried. Enumerated rather than trusted.
  const states: SessionState[] = ['idle', 'connecting', 'ready', 'capturing', 'failed', 'closed'];
  const events: SessionEvent[] = [
    { type: 'connect' },
    { type: 'connected' },
    { type: 'capture' },
    { type: 'captured' },
    { type: 'retry' },
    { type: 'disconnect' },
    { type: 'close' },
    ...(['not-found', 'refused', 'unreachable', 'unsupported'] as const).flatMap(
      (reason): SessionEvent[] => [
        { type: 'connect-failed', reason },
        { type: 'capture-failed', reason },
      ],
    ),
  ];
  for (const state of states) {
    for (const event of events) {
      assert.ok(states.includes(step(state, event)), `${state} + ${event.type}`);
    }
  }
});

/* ================================================= the capture session */

class ScriptedSource implements CaptureSource {
  readonly kind = 'scripted';
  readonly modalities = ['tag'] as const;
  disconnected = 0;

  private readonly connectResult: Outcome<DeviceDescriptor>;
  private readonly scripted: Outcome<Capture>[];

  constructor(connectResult: Outcome<DeviceDescriptor>, scripted: Outcome<Capture>[]) {
    this.connectResult = connectResult;
    this.scripted = scripted;
  }

  available() {
    return true;
  }
  unconfirmed() {
    return [];
  }
  async connect() {
    return this.connectResult;
  }
  async capture() {
    return this.scripted.shift() ?? ({ ok: false, reason: 'not-found', detail: 'script ended' } as const);
  }
  async disconnect() {
    this.disconnected++;
  }
}

const DESCRIPTOR: DeviceDescriptor = { kind: 'scripted', label: 'fake', handle: 'h' };
function aCapture(): Capture {
  return {
    modality: 'tag',
    payload: new TextEncoder().encode('982000000000003'),
    capturedAt: 1,
    device: DESCRIPTOR,
    quality: null,
  };
}

test('a session refuses to capture before a device is connected, and says so', async () => {
  const session = new CaptureSession(new ScriptedSource({ ok: true, value: DESCRIPTOR }, []));
  const got = refusal(await session.capture());
  assert.equal(got.reason, 'refused');
  assert.match(got.detail, /connect a device first/);
  assert.equal(session.state, 'idle');
});

test('a session collects captures and reports every state change once', async () => {
  const seen: SessionState[] = [];
  const session = new CaptureSession(
    new ScriptedSource({ ok: true, value: DESCRIPTOR }, [{ ok: true, value: aCapture() }]),
    (s) => seen.push(s),
  );

  value(await session.connect());
  assert.equal(session.state, 'ready');
  value(await session.capture());
  assert.equal(session.state, 'ready');
  assert.equal(session.captures.length, 1);
  assert.deepEqual(seen, ['connecting', 'ready', 'capturing', 'ready']);
});

test('a session that loses its device drops to idle and keeps what it collected', async () => {
  const session = new CaptureSession(
    new ScriptedSource({ ok: true, value: DESCRIPTOR }, [
      { ok: true, value: aCapture() },
      { ok: false, reason: 'unreachable', detail: 'unplugged' },
    ]),
  );
  await session.connect();
  await session.capture();
  await session.capture();
  assert.equal(session.state, 'idle', 'ready would be a lie');
  assert.equal(session.captures.length, 1, 'the good capture is not thrown away');
});

test('a refused capture leaves the device connected and ready to try again', async () => {
  const session = new CaptureSession(
    new ScriptedSource({ ok: true, value: DESCRIPTOR }, [
      { ok: false, reason: 'refused', detail: 'blurred' },
      { ok: true, value: aCapture() },
    ]),
  );
  await session.connect();
  refusal(await session.capture());
  assert.equal(session.state, 'ready', 'standing closer fixes this, reconnecting does not');
  value(await session.capture());
  assert.equal(session.captures.length, 1);
});

test('a session refuses a second connect while the first chooser is open', async () => {
  let resolve: ((got: Outcome<DeviceDescriptor>) => void) | undefined;
  const slow: CaptureSource = {
    kind: 'slow',
    modalities: ['tag'],
    available: () => true,
    unconfirmed: () => [],
    connect: () => new Promise((r) => (resolve = r)),
    capture: async () => ({ ok: false, reason: 'not-found', detail: '' }),
    disconnect: async () => {},
  };
  const session = new CaptureSession(slow);

  const first = session.connect();
  assert.equal(session.state, 'connecting');
  const second = refusal(await session.connect());
  assert.match(second.detail, /already connecting/);

  resolve!({ ok: true, value: DESCRIPTOR });
  value(await first);
  assert.equal(session.state, 'ready');
});

test('a failed connection leaves the session where it started', async () => {
  const session = new CaptureSession(
    new ScriptedSource({ ok: false, reason: 'not-found', detail: 'nothing chosen' }, []),
  );
  refusal(await session.connect());
  assert.equal(session.state, 'idle', 'the operator can just press the button again');
});

test('closing a session releases the device and is terminal', async () => {
  const source = new ScriptedSource({ ok: true, value: DESCRIPTOR }, []);
  const session = new CaptureSession(source);
  await session.connect();
  await session.close();
  assert.equal(session.state, 'closed');
  assert.equal(source.disconnected, 1);
  assert.equal(refusal(await session.capture()).reason, 'refused');
});

/* ============================================ outcomes and honesty */

test('a DOMException name decides the outcome, rather than everything being unreachable', () => {
  // "The operator pressed cancel" must not become "check the cable".
  const named = (name: string) => {
    const err = new Error('x');
    err.name = name;
    return refusal(outcomeFromError(err, 'ctx')).reason;
  };
  assert.equal(named('NotFoundError'), 'not-found');
  assert.equal(named('NotAllowedError'), 'refused');
  assert.equal(named('SecurityError'), 'refused');
  assert.equal(named('InvalidStateError'), 'refused');
  assert.equal(named('NetworkError'), 'unreachable');
  assert.equal(named('AbortError'), 'unreachable');
  assert.equal(named('NotSupportedError'), 'unsupported');
  assert.equal(named('SomethingNobodyHasSeen'), 'unreachable');
});

test('a thrown non-Error still produces an outcome and keeps the context', () => {
  const got = refusal(outcomeFromError('a string', 'connecting to the reader'));
  assert.equal(got.reason, 'unreachable');
  assert.match(got.detail, /connecting to the reader/);
});

test('isAnswer separates what the device said from silence', () => {
  assert.equal(isAnswer({ ok: true, value: 1 }), true);
  assert.equal(isAnswer({ ok: false, reason: 'not-found', detail: '' }), true);
  assert.equal(isAnswer({ ok: false, reason: 'refused', detail: '' }), true);
  assert.equal(isAnswer({ ok: false, reason: 'unreachable', detail: '' }), false);
  assert.equal(isAnswer({ ok: false, reason: 'unsupported', detail: '' }), false);
});

test('every unconfirmed constant says what goes wrong if it is guessed', () => {
  // The list is data so the application can render it as a procurement
  // checklist, and a checklist entry that does not say what the risk is will be
  // ticked off without being checked.
  assert.ok(UNCONFIRMED_CONSTANTS.length >= 10);
  for (const entry of UNCONFIRMED_CONSTANTS) {
    assert.ok(entry.where.length > 0, JSON.stringify(entry));
    assert.ok(entry.name.length > 0, entry.where);
    assert.ok(entry.risk.length > 40, `${entry.name} needs a real consequence, not a label`);
    assert.ok(entry.source.length > 0, entry.name);
  }
});

test('each hardware source reports its own unknowns, and manual entry has none', () => {
  const { usb } = fakeUsb({ transferIn: async () => ({ status: 'ok' }) });
  assert.ok(new IriShieldUSB(FAKE_PROTOCOL, usb).unconfirmed().length >= 4);
  assert.ok(new BluetoothLFReader(READER, undefined).unconfirmed().length >= 3);
  assert.ok(new PhoneCamera('muzzle', fakeGrabber([])).unconfirmed().length >= 2);
  assert.deepEqual(new ManualEntry('tattoo', async () => null).unconfirmed(), []);
});

test('the device protocols carry no usable defaults anywhere', async () => {
  // The rule stated as an executable check. If somebody later adds a
  // DEFAULT_IRISHIELD_PROTOCOL with a plausible vendor id in it, this is what
  // notices — because a fabricated constant that looks right is worse than an
  // obvious hole. Somebody will believe it.
  const module = (await import('./device.ts')) as Record<string, unknown>;
  for (const [name, exported] of Object.entries(module)) {
    if (!exported || typeof exported !== 'object') continue;
    assert.ok(
      !('vendorId' in exported) && !('productId' in exported),
      `${name} exports a USB identifier: no protocol constants may be shipped unconfirmed`,
    );
  }
});
