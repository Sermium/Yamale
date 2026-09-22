/**
 * Talking to the hardware.
 *
 * Four ways to get an identifier or a biometric out of an animal and into the
 * application, behind one interface, so that nothing above this file knows
 * which one was used. An iris camera over WebUSB, the phone's own camera, a
 * low-frequency transponder reader over Bluetooth, and a person typing.
 *
 * ============================================================================
 * READ THIS BEFORE BUYING ANYTHING
 * ============================================================================
 *
 * None of this hardware exists here. Not one line below has been run against an
 * IriShield, a stick reader, or a phone camera. That has consequences, and the
 * consequences are stated here rather than discovered in a pen in Kaolack.
 *
 * VERIFIED — compiles, and tested against fixtures written by hand:
 *
 *   - the ISO/IEC 19794-6 container parse, including every consistency check
 *     that makes a wrong offset fail loudly instead of returning a plausible
 *     image (`parseIrisRecord`)
 *   - the quality gate on a raster: sharpness, exposure, clipping, contrast,
 *     and the rule that a capture scores as its worst property (`assessFrame`)
 *   - reassembly of BLE notification chunks into lines (`LineAssembler`), which
 *     is where a 15-digit code split across two 20-byte packets gets lost
 *   - the reader line to transponder code path (`readerLineToTag`)
 *   - the capture session's state machine (`step`), including the part that
 *     depends on WHICH failure happened
 *   - the `Outcome` mapping for every failure this file can name
 *
 * UNVERIFIED — written from documentation, inference and the shape of the APIs,
 * and wrong until somebody plugs a device in:
 *
 *   - every USB constant for the IriShield: vendor id, product id, interface,
 *     endpoints, and the command that makes it capture. We have none of them.
 *   - whether the IriShield can be reached from WebUSB AT ALL. See the note on
 *     `IriShieldUSB` — if the device enumerates as HID, WebUSB cannot claim it
 *     on Windows or macOS and no amount of code changes that.
 *   - the byte offsets of the ISO/IEC 19794-6:2011 record. The standard is a
 *     paid document nobody here has. The layout below is reconstructed from its
 *     published structure and the parser is built to fail loudly if it is
 *     wrong — but "fails loudly" is not "is right".
 *   - every GATT identifier for the LF reader, and more fundamentally whether
 *     the reader speaks BLE at all rather than Bluetooth Classic serial, which
 *     Web Bluetooth cannot reach.
 *   - the wire format of whatever the reader emits on that characteristic.
 *
 * Every unconfirmed constant is also listed in `UNCONFIRMED_CONSTANTS` below,
 * as data, so the application can render it as a procurement checklist and
 * nobody has to trust a comment.
 *
 * ============================================================================
 * WHY THERE ARE NO PLAUSIBLE-LOOKING DEFAULTS
 * ============================================================================
 *
 * There is no `DEFAULT_IRISHIELD_PROTOCOL` with a vendor id in it. A fabricated
 * constant that looks right is worse than an obvious hole, because somebody
 * will believe it. Any four-digit hex number reads exactly like a vendor id, it
 * will survive review, and it will fail on a bench three months from now as
 * "the device isn't detected" — which is indistinguishable from a broken cable.
 * No such number is written anywhere below, not even as an illustration.
 *
 * So the protocol is a required constructor argument with no default, the
 * constructor validates it, and the source reports itself unavailable until it
 * is filled in from the vendor's documentation.
 */

import type { IrisTemplate } from './iris.ts';
import {
  parseNationalId,
  parseTagCode,
  type Modality,
  type NationalIdentifier,
  type TagCode,
} from './identity.ts';

export type { Modality };

/* ========================================================== the outcome */

/**
 * What an attempt to talk to a device produced.
 *
 * Four failures rather than one, for the reason `clients/rwa/src/abci.ts` gives
 * for its three: a screen has to say different things about them, and a null
 * return conflates them all into "nothing happened". Here the distinction does
 * more than word a message — `step` below routes on it, because they need
 * different recoveries:
 *
 *   - `not-found`   the device answered, and the answer is that there is
 *                   nothing there: no transponder in range, no iris in the
 *                   frame. Try again. The device is fine.
 *   - `refused`     the device or the operator answered no: permission denied,
 *                   a command rejected, a capture that failed the quality gate.
 *                   Something must change before trying again.
 *   - `unreachable` no answer: unplugged, out of range, timed out, disconnected
 *                   mid-read. Reconnect.
 *   - `unsupported` this platform cannot do this at all, and never will on this
 *                   device. Get a different browser, or a different phone.
 *
 * The fourth is the one `abci.ts` does not need and this does. "Reader not
 * found" sends an operator looking for the reader; "this browser has no
 * Bluetooth" sends them to a different browser. Collapsing those two wastes a
 * morning in a place where a morning is a day's travel.
 */
export type Outcome<T> =
  | { ok: true; value: T }
  | { ok: false; reason: 'not-found'; detail: string }
  | { ok: false; reason: 'refused'; detail: string }
  | { ok: false; reason: 'unreachable'; detail: string }
  | { ok: false; reason: 'unsupported'; detail: string };

export type FailureReason = 'not-found' | 'refused' | 'unreachable' | 'unsupported';

export function fail<T>(reason: FailureReason, detail: string): Outcome<T> {
  return { ok: false, reason, detail };
}

/** True when the failure is the device's answer rather than the absence of one. */
export function isAnswer<T>(outcome: Outcome<T>): boolean {
  return outcome.ok || (outcome.reason !== 'unreachable' && outcome.reason !== 'unsupported');
}

/**
 * A thrown error, mapped to an outcome.
 *
 * WebUSB, Web Bluetooth and `getUserMedia` all report through `DOMException`
 * names, and those names carry the distinction this file needs — but only if
 * they are read. A blanket catch would turn "the operator pressed cancel" into
 * "the device is unreachable", and the operator would be told to check a cable
 * they did not touch.
 *
 * UNVERIFIED in one respect: which of these names each API actually throws in
 * each situation is documented behaviour that browsers do not implement
 * uniformly. The mapping is tested; the browsers' fidelity to it is not.
 */
export function outcomeFromError<T>(err: unknown, context: string): Outcome<T> {
  const name = err instanceof Error ? err.name : '';
  const message = err instanceof Error ? err.message : String(err);
  const detail = `${context}: ${name || 'error'}: ${message}`;

  switch (name) {
    // requestDevice with no device chosen, getUserMedia denied. The operator
    // said no, or there was nothing in the chooser to say yes to.
    case 'NotFoundError':
      return fail('not-found', detail);
    // Permission denied, or a device the OS will not release. Both need a
    // person to do something before a retry can work.
    case 'NotAllowedError':
    case 'SecurityError':
      return fail('refused', detail);
    // claimInterface against a kernel-driver-bound interface; a GATT
    // characteristic already in use.
    case 'InvalidStateError':
    case 'InvalidAccessError':
      return fail('refused', detail);
    // The device went away mid-conversation.
    case 'NetworkError':
    case 'NotReadableError':
      return fail('unreachable', detail);
    case 'AbortError':
    case 'TimeoutError':
      return fail('unreachable', detail);
    case 'NotSupportedError':
      return fail('unsupported', detail);
    default:
      return fail('unreachable', detail);
  }
}

/* ============================================ the unconfirmed constants */

/**
 * One thing we do not know, as data.
 *
 * As data rather than a comment so the application can render it — a screen
 * that lists what is unconfirmed is a screen nobody can ship past by accident,
 * and a comment is a screen nobody ever reads.
 */
export interface Unconfirmed {
  /** Which source needs it. */
  where: string;
  /** What it is. */
  name: string;
  /** What goes wrong if it is guessed, rather than what it is for. */
  risk: string;
  /** Where the real answer lives. */
  source: string;
}

/**
 * Everything in this file that must be checked against a vendor's
 * documentation before a device is bought or a field trial is booked.
 *
 * The ISO entries are here too, even though a standard is not a vendor: a byte
 * offset reconstructed from a structure description is exactly as unconfirmed
 * as an endpoint number, and the consequence is the same shape.
 */
export const UNCONFIRMED_CONSTANTS: readonly Unconfirmed[] = [
  {
    where: 'IriShieldUSB',
    name: 'USB vendor id and product id for the IriTech IriShield MK 2120U',
    risk:
      'requestDevice filters on these. Wrong values show an empty chooser, ' +
      'which looks exactly like a broken cable and will be diagnosed as one.',
    source: "IriTech's SDK documentation, or `lsusb` against a real unit",
  },
  {
    where: 'IriShieldUSB',
    name: 'configuration value, interface number, bulk IN and OUT endpoint numbers',
    risk:
      'claimInterface against the wrong interface throws; the wrong endpoint ' +
      'number either throws or reads nothing forever, and a transferIn that ' +
      'never resolves looks like a device that is thinking.',
    source: "the device's USB descriptors, read from a real unit",
  },
  {
    where: 'IriShieldUSB',
    name: "the vendor's command to capture and return a template",
    risk:
      'the whole conversation. There is no generic USB way to ask a biometric ' +
      'device for a template; this is proprietary to IriTech and cannot be guessed.',
    source: "IriTech's IriShield SDK / protocol documentation, under NDA if necessary",
  },
  {
    where: 'IriShieldUSB',
    name: 'whether the device enumerates as a vendor-specific class or as HID',
    risk:
      'if it is HID, WebUSB cannot claim it on Windows or macOS at all — the OS ' +
      'driver holds the interface. This is not a code problem and it decides ' +
      'whether a browser-based capture app is possible. Establish it FIRST.',
    source: "the device's interface descriptors; IriTech support",
  },
  {
    where: 'parseIrisRecord',
    name: 'the byte offsets of the ISO/IEC 19794-6:2011 record and representation headers',
    risk:
      'a wrong offset slices the image out of the wrong place and produces a ' +
      'template that compares beautifully against other templates from the same ' +
      'wrong parser and means nothing. The consistency checks below are designed ' +
      'to catch it, and they are a net rather than a guarantee.',
    source: 'ISO/IEC 19794-6:2011 §5, which is a purchased document',
  },
  {
    where: 'parseIrisRecord',
    name: 'the image format and image type code values',
    risk:
      'they are reported, not interpreted, precisely because we do not know ' +
      'them. Any code that branches on them is making them up.',
    source: 'ISO/IEC 19794-6:2011, Tables 3 and 4',
  },
  {
    where: 'IriShieldUSB / irisTemplateFrom',
    name: "the encoding of IriTech's iris template, if it emits one",
    risk:
      'ISO/IEC 19794-6 standardises iris IMAGES. The compact iris code that ' +
      'iris.ts compares is not in that standard and is vendor-proprietary. ' +
      'Without the vendor codec this layer can hand over a record and not a ' +
      'template, which changes what the biometric service has to do.',
    source: "IriTech's SDK documentation",
  },
  {
    where: 'BluetoothLFReader',
    name: 'whether the chosen stick reader speaks BLE GATT or Bluetooth Classic SPP',
    risk:
      'Web Bluetooth speaks BLE GATT ONLY. A reader that exposes a serial port ' +
      'profile — which many livestock readers do — is unreachable from a browser ' +
      'on any platform. This decides the procurement, not the code.',
    source: "the reader's datasheet; ask the vendor for 'BLE GATT' explicitly",
  },
  {
    where: 'BluetoothLFReader',
    name: 'the GATT service and characteristic UUIDs the reader exposes',
    risk:
      'Web Bluetooth cannot discover a service that was not named up front in ' +
      'filters or optionalServices. A wrong UUID is not a degraded connection, ' +
      'it is no connection, reported as "service not found".',
    source: "the reader's BLE documentation, or a GATT explorer against a real unit",
  },
  {
    where: 'BluetoothLFReader',
    name: 'the line format the reader emits when it reads a transponder',
    risk:
      'the default parser takes the first 15-digit run in the line, which works ' +
      'for the common prefixed-ASCII readers and silently mis-parses anything ' +
      'binary or checksummed. Supply a parser rather than hoping.',
    source: "the reader's protocol documentation",
  },
  {
    where: 'PhoneCamera',
    name: 'whether the target handset can focus at 8 cm with a camera getUserMedia can select',
    risk:
      'design §6 makes close focus the disqualifying specification, and the web ' +
      'platform can neither command it nor measure it. It must be verified by ' +
      'taking a picture on the actual handset, as §6 says.',
    source: 'a real handset and a real muzzle',
  },
  {
    where: 'QUALITY_GATE',
    name: 'every threshold in the quality gate',
    risk:
      'they are placeholders, in the same sense as iris.ts THRESHOLDS. Too ' +
      'strict and an operator cannot enrol an animal in the rain; too loose and ' +
      'the register fills with captures that will never match anything.',
    source: 'a few hundred field captures, scored and looked at',
  },
];

/* ================================================== what a source produces */

/** What took the capture, recorded because the enrolment record names it. */
export interface DeviceDescriptor {
  /** Which implementation: `irishield-usb`, `phone-camera`, `bt-lf-reader`, `manual`. */
  kind: string;
  /** Something a human can read on a screen. May be empty; devices lie about this. */
  label: string;
  /**
   * Stable enough to group captures by device, and NOT a device serial number
   * unless the device volunteered one. WebUSB exposes `serialNumber` only when
   * the descriptor carries it; Web Bluetooth's `device.id` is an opaque
   * per-origin identifier that is not stable across browser profiles.
   */
  handle: string;
}

/**
 * One capture, whatever produced it.
 *
 * `payload` is what gets committed — see `commitCapture` in identity.ts — and
 * is the ISO record for an iris, the encoded still for a camera, and the
 * identifier's UTF-8 bytes for a typed entry. The application never inspects
 * it; it hashes it and hands it to the biometric service.
 */
export interface Capture {
  modality: Modality;
  payload: Uint8Array;
  /** Milliseconds since the epoch, from the device's clock, which may be wrong. */
  capturedAt: number;
  device: DeviceDescriptor;
  /**
   * 0 to 1, or null when the source cannot score its own capture.
   *
   * Null is not zero and must not be rendered as one. A typed identifier has no
   * quality; the enrolment record says so rather than claiming a bad score.
   */
  quality: number | null;
  /** Present only when the source produced something `iris.ts` can compare. */
  template?: IrisTemplate;
  /** Present only when the capture is an identifier rather than a biometric. */
  identifier?: TagCode | NationalIdentifier | { tattoo: string };
  /** Why the payload has the bytes it has. For a log and for a bug report. */
  note?: string;
}

/**
 * The abstraction the application sees.
 *
 * `available` is synchronous and must not prompt, because it is called to
 * decide whether to draw a button. `connect` prompts, and in every one of these
 * APIs it MUST be called from inside a user gesture — a `requestDevice` in a
 * `useEffect` is rejected by the browser, and the rejection reads as a
 * permission error.
 */
export interface CaptureSource {
  readonly kind: string;
  readonly modalities: readonly Modality[];
  /** Could this platform ever run this source? No prompting, no I/O. */
  available(): boolean;
  /** Everything this source needs confirmed before it can work. Empty is good. */
  unconfirmed(): readonly Unconfirmed[];
  /** Ask for a device. Must be called from a user gesture. */
  connect(): Promise<Outcome<DeviceDescriptor>>;
  capture(): Promise<Outcome<Capture>>;
  disconnect(): Promise<void>;
}

/* ================================================ ISO/IEC 19794-6 records */

/**
 * The general record header, as reconstructed.
 *
 * `IIR\0` then an ASCII version then the total length. The magic is the one
 * part of this that is certain, and it is also the part that makes a wrong
 * reply obvious: if the IriShield's USB protocol constants are wrong, what
 * comes back will not start `IIR\0`, and the failure will say so.
 */
export const IRIS_RECORD_MAGIC = new Uint8Array([0x49, 0x49, 0x52, 0x00]); // "IIR\0"

/** 16 bytes: magic 4, version 4, record length 4, representations 2, flag 1, eyes 1. */
export const IRIS_HEADER_BYTES = 16;

/**
 * The fixed parts of a representation header, either side of the quality blocks.
 *
 *   before: length 4, datetime 9, device technology 1, vendor 2, type 2,
 *           quality block count 1                                     = 19
 *   each quality block: score 1, algorithm vendor 2, algorithm 2       =  5
 *   after:  representation number 2, eye 1, image type 1, image format 1,
 *           horizontal orientation 1, vertical orientation 1,
 *           compression history 1, width 2, height 2, bit depth 1,
 *           range 2, roll 2, roll uncertainty 2,
 *           iris centre smallest X 2, largest X 2, smallest Y 2,
 *           largest Y 2, image length 4                               = 31
 *
 * UNVERIFIED, and the single most likely thing in this file to be wrong. These
 * three numbers are why `parseIrisRecord` checks that the declared
 * representation length equals `19 + 5N + 31 + imageLength` and refuses rather
 * than guessing when it does not: a real device's output will not satisfy that
 * equation if any of them is off, and the error names the arithmetic.
 */
export const REPRESENTATION_FIXED_BEFORE_QUALITY = 19;
export const QUALITY_BLOCK_BYTES = 5;
export const REPRESENTATION_FIXED_AFTER_QUALITY = 31;

export interface IrisQualityBlock {
  score: number;
  algorithmVendor: number;
  algorithm: number;
}

export interface IrisRepresentation {
  /** 0 undefined, 1 right, 2 left, per the standard. Reported, not interpreted. */
  eye: number;
  representationNumber: number;
  /** From the record's own 9-byte timestamp, in UTC. The device's clock. */
  capturedAt: Date;
  captureDeviceTechnology: number;
  captureDeviceVendor: number;
  captureDeviceType: number;
  quality: readonly IrisQualityBlock[];
  /** Code values are UNCONFIRMED — carried through, never branched on. */
  imageType: number;
  imageFormat: number;
  width: number;
  height: number;
  bitDepth: number;
  /** The representation's payload, exactly as it sat in the record. */
  image: Uint8Array;
}

export interface IrisRecord {
  /** The four ASCII version bytes, trailing NUL trimmed. `020` for the 2011 edition. */
  version: string;
  certificationFlag: number;
  eyesRepresented: number;
  representations: readonly IrisRepresentation[];
}

/**
 * Parse an ISO/IEC 19794-6 record.
 *
 * # This is a container parse and not a template parse
 *
 * The statement that matters, and it is the one most likely to disappoint
 * somebody: ISO/IEC 19794-6 standardises iris **images** — rectilinear and
 * polar rasters with metadata. The compact iris **code** that `iris.ts`
 * compares, the Gabor phase bits with their mask, is not in that standard and
 * is proprietary to whoever extracted it. A record parsed here therefore yields
 * an image and metadata, and yields an `IrisTemplate` only through a codec
 * somebody supplies. `irisTemplateFrom` below says so in its return value
 * rather than inventing one.
 *
 * # Why it is this suspicious
 *
 * A biometric parser that is slightly wrong does not crash. It returns a
 * template, the template compares against other templates from the same wrong
 * parser, and everything looks like it works until a device from a different
 * vendor arrives. So every field that can be checked against another field is
 * checked, and any disagreement is a refusal:
 *
 *   - the magic, exactly
 *   - the declared record length against the actual buffer length
 *   - each declared representation length against the arithmetic of its parts
 *   - the image length against what is left in the representation
 *   - the representation count against how many were actually found
 */
export function parseIrisRecord(bytes: Uint8Array): Outcome<IrisRecord> {
  if (bytes.length < IRIS_HEADER_BYTES) {
    return fail('refused', `too short for a record header: ${bytes.length} bytes`);
  }

  for (let i = 0; i < IRIS_RECORD_MAGIC.length; i++) {
    if (bytes[i] !== IRIS_RECORD_MAGIC[i]) {
      // The most valuable error message in this file. If the USB protocol
      // constants are guesses, this is where the guess surfaces, and the hex
      // makes the difference between "no device" and "wrong endpoint" visible.
      return fail(
        'refused',
        `not an ISO 19794-6 record: expected magic 49 49 52 00, got ` +
          `${hexPreview(bytes, 4)} (${bytes.length} bytes in total)`,
      );
    }
  }

  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const version = new TextDecoder('ascii').decode(bytes.subarray(4, 8)).replace(/\0+$/, '');
  const declared = view.getUint32(8, false);

  if (declared !== bytes.length) {
    return fail(
      'refused',
      `record declares ${declared} bytes and ${bytes.length} were read — ` +
        `a truncated transfer, or the header layout is wrong`,
    );
  }

  const count = view.getUint16(12, false);
  const certificationFlag = bytes[14]!;
  const eyesRepresented = bytes[15]!;

  const representations: IrisRepresentation[] = [];
  let at = IRIS_HEADER_BYTES;

  for (let n = 0; n < count; n++) {
    const got = parseRepresentation(bytes, view, at);
    if (!got.ok) return fail(got.reason, `representation ${n}: ${got.detail}`);
    representations.push(got.value.representation);
    at = got.value.next;
  }

  if (at !== bytes.length) {
    return fail(
      'refused',
      `${bytes.length - at} bytes left after ${count} representations — ` +
        `the count or the representation lengths disagree with the record`,
    );
  }

  return { ok: true, value: { version, certificationFlag, eyesRepresented, representations } };
}

function parseRepresentation(
  bytes: Uint8Array,
  view: DataView,
  start: number,
): Outcome<{ representation: IrisRepresentation; next: number }> {
  if (start + REPRESENTATION_FIXED_BEFORE_QUALITY > bytes.length) {
    return fail('refused', `runs off the end of the record at offset ${start}`);
  }

  const length = view.getUint32(start, false);
  if (length < REPRESENTATION_FIXED_BEFORE_QUALITY + REPRESENTATION_FIXED_AFTER_QUALITY) {
    return fail('refused', `declared length ${length} is shorter than a header`);
  }
  if (start + length > bytes.length) {
    return fail(
      'refused',
      `declared length ${length} from offset ${start} exceeds the ${bytes.length}-byte record`,
    );
  }

  // The 9-byte capture time. Kept in UTC: a capture time rendered in the
  // tablet's local zone would drift against the chain's block time, and §4 of
  // the design makes the interval between enrolments load-bearing.
  const year = view.getUint16(start + 4, false);
  const capturedAt = new Date(
    Date.UTC(
      year,
      bytes[start + 6]! - 1,
      bytes[start + 7]!,
      bytes[start + 8]!,
      bytes[start + 9]!,
      bytes[start + 10]!,
      view.getUint16(start + 11, false),
    ),
  );

  const captureDeviceTechnology = bytes[start + 13]!;
  const captureDeviceVendor = view.getUint16(start + 14, false);
  const captureDeviceType = view.getUint16(start + 16, false);
  const qualityCount = bytes[start + 18]!;

  const qualityBytes = qualityCount * QUALITY_BLOCK_BYTES;
  const afterQuality = start + REPRESENTATION_FIXED_BEFORE_QUALITY + qualityBytes;
  if (afterQuality + REPRESENTATION_FIXED_AFTER_QUALITY > start + length) {
    return fail(
      'refused',
      `${qualityCount} quality blocks do not fit inside the declared ${length} bytes`,
    );
  }

  const quality: IrisQualityBlock[] = [];
  for (let q = 0; q < qualityCount; q++) {
    const at = start + REPRESENTATION_FIXED_BEFORE_QUALITY + q * QUALITY_BLOCK_BYTES;
    quality.push({
      score: bytes[at]!,
      algorithmVendor: view.getUint16(at + 1, false),
      algorithm: view.getUint16(at + 3, false),
    });
  }

  const representationNumber = view.getUint16(afterQuality, false);
  const eye = bytes[afterQuality + 2]!;
  const imageType = bytes[afterQuality + 3]!;
  const imageFormat = bytes[afterQuality + 4]!;
  const width = view.getUint16(afterQuality + 8, false);
  const height = view.getUint16(afterQuality + 10, false);
  const bitDepth = bytes[afterQuality + 12]!;
  const imageLength = view.getUint32(afterQuality + REPRESENTATION_FIXED_AFTER_QUALITY - 4, false);

  // The arithmetic check. This is what catches a wrong offset constant, and it
  // is the reason the three constants are exported and named rather than
  // buried: when this fires, the message is enough to fix them.
  const expected =
    REPRESENTATION_FIXED_BEFORE_QUALITY +
    qualityBytes +
    REPRESENTATION_FIXED_AFTER_QUALITY +
    imageLength;
  if (expected !== length) {
    return fail(
      'refused',
      `length arithmetic disagrees: header declares ${length} bytes, but ` +
        `${REPRESENTATION_FIXED_BEFORE_QUALITY} + ${qualityBytes} (${qualityCount} quality blocks) + ` +
        `${REPRESENTATION_FIXED_AFTER_QUALITY} + ${imageLength} (image) = ${expected}. ` +
        `Either the record is malformed or the header offsets in device.ts are wrong — ` +
        `check them against ISO/IEC 19794-6:2011 §5 before trusting this parser.`,
    );
  }

  const imageStart = afterQuality + REPRESENTATION_FIXED_AFTER_QUALITY;
  const image = bytes.slice(imageStart, imageStart + imageLength);

  return {
    ok: true,
    value: {
      representation: {
        eye,
        representationNumber,
        capturedAt,
        captureDeviceTechnology,
        captureDeviceVendor,
        captureDeviceType,
        quality,
        imageType,
        imageFormat,
        width,
        height,
        bitDepth,
        image,
      },
      next: start + length,
    },
  };
}

function hexPreview(bytes: Uint8Array, n: number): string {
  return Array.from(bytes.subarray(0, n), (b) => b.toString(16).padStart(2, '0')).join(' ');
}

/**
 * Turn a representation into something `iris.ts` can compare — if anything can.
 *
 * A codec is REQUIRED and there is no default that guesses. Without one this
 * returns a refusal whose detail explains why, because the alternative is a
 * function that appears to work and returns a template made of image pixels.
 */
export type TemplateCodec = (representation: IrisRepresentation) => Outcome<IrisTemplate>;

export function irisTemplateFrom(
  representation: IrisRepresentation,
  codec?: TemplateCodec,
): Outcome<IrisTemplate> {
  if (!codec) {
    return fail(
      'refused',
      `this representation holds an image (${representation.width}x${representation.height}, ` +
        `format ${representation.imageFormat}), and ISO/IEC 19794-6 does not carry an iris ` +
        `code at all. Supply the vendor's template codec, or enrol the record and let the ` +
        `biometric service extract the template.`,
    );
  }
  return codec(representation);
}

/**
 * Our own template container, for templates WE store.
 *
 * Named after us because it is ours. It is not a standard, it is not IriTech's,
 * and it exists so that the path from a record to an `IrisTemplate` can be
 * exercised end to end in a test rather than asserted in a comment. When the
 * vendor's codec arrives, this stays, because the local herd replica of §5 has
 * to write templates back somewhere and the format may as well be one we can
 * read.
 *
 *     'Y' 'I' 'C' '1'   magic
 *     uint32            bit count
 *     uint32            code length, then that many bytes
 *     uint32            mask length, then that many bytes
 */
export const YAMALE_IRIS_CODE_MAGIC = new Uint8Array([0x59, 0x49, 0x43, 0x31]); // "YIC1"

export function encodeYamaleIrisCode(template: IrisTemplate): Uint8Array {
  const out = new Uint8Array(4 + 4 + 4 + template.code.length + 4 + template.mask.length);
  const view = new DataView(out.buffer);
  out.set(YAMALE_IRIS_CODE_MAGIC, 0);
  view.setUint32(4, template.bits, false);
  view.setUint32(8, template.code.length, false);
  out.set(template.code, 12);
  view.setUint32(12 + template.code.length, template.mask.length, false);
  out.set(template.mask, 16 + template.code.length);
  return out;
}

/** The codec for the container above. Loud about every disagreement. */
export const yamaleIrisCodec: TemplateCodec = (representation) => {
  const bytes = representation.image;
  if (bytes.length < 16) return fail('refused', `too short for a YIC1 template: ${bytes.length} bytes`);
  for (let i = 0; i < 4; i++) {
    if (bytes[i] !== YAMALE_IRIS_CODE_MAGIC[i]) {
      return fail('refused', `not a YIC1 template: magic is ${hexPreview(bytes, 4)}`);
    }
  }

  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const bits = view.getUint32(4, false);
  const codeLength = view.getUint32(8, false);
  if (12 + codeLength + 4 > bytes.length) {
    return fail('refused', `code length ${codeLength} runs off the end`);
  }
  const code = bytes.slice(12, 12 + codeLength);
  const maskLength = view.getUint32(12 + codeLength, false);
  if (16 + codeLength + maskLength !== bytes.length) {
    return fail(
      'refused',
      `mask length ${maskLength} leaves ${bytes.length - 16 - codeLength - maskLength} bytes unaccounted for`,
    );
  }
  const mask = bytes.slice(16 + codeLength, 16 + codeLength + maskLength);

  // iris.ts indexes both arrays by bit and silently treats a short array as
  // zeroes — a mask of zeroes is "nothing comparable", which shows up as every
  // comparison being inconclusive rather than as an error. Caught here instead.
  if (code.length !== mask.length) {
    return fail('refused', `code is ${code.length} bytes and mask is ${mask.length}`);
  }
  if (bits > code.length * 8) {
    return fail('refused', `declares ${bits} bits but carries only ${code.length * 8}`);
  }

  return { ok: true, value: { code, mask, bits } };
};

/* ================================================== 1. the iris camera */

/**
 * Everything `IriShieldUSB` needs and cannot find out for itself.
 *
 * There is no default value for this interface anywhere in this file. See the
 * header: a plausible vendor id is worse than none.
 */
export interface IriShieldProtocol {
  /** USB vendor id. UNKNOWN — see UNCONFIRMED_CONSTANTS. */
  vendorId: number;
  /** USB product id for the MK 2120U. UNKNOWN. */
  productId: number;
  /** The configuration value to select. Usually 1, and "usually" is not "is". */
  configurationValue: number;
  /** The interface to claim. UNKNOWN. */
  interfaceNumber: number;
  /** Bulk IN endpoint number, 1–15. UNKNOWN. */
  endpointIn: number;
  /** Bulk OUT endpoint number, 1–15. UNKNOWN. */
  endpointOut: number;
  /** The vendor's "capture and return a template" command. UNKNOWN, and proprietary. */
  captureCommand: Uint8Array;
  /** How many bytes to ask for in one transferIn. */
  maxReplyBytes: number;
  /** How long to wait for a beast to hold still. */
  captureTimeoutMs: number;
}

/**
 * The problems with a protocol description, as sentences a developer can act on.
 *
 * Empty means every field is at least structurally possible. It does NOT mean
 * the values are right — nothing here can know that — and the name says
 * `problems` rather than `isValid` for exactly that reason.
 */
export function protocolProblems(p: Partial<IriShieldProtocol> | undefined): string[] {
  const out: string[] = [];
  if (!p) return ['no protocol supplied: every constant for this device is unknown'];

  const u16 = (n: unknown) => typeof n === 'number' && Number.isInteger(n) && n >= 0 && n <= 0xffff;
  if (!u16(p.vendorId) || p.vendorId === 0) out.push('vendorId is not a USB vendor id');
  if (!u16(p.productId)) out.push('productId is not a USB product id');
  if (typeof p.configurationValue !== 'number' || p.configurationValue < 1) {
    out.push('configurationValue must be at least 1');
  }
  if (typeof p.interfaceNumber !== 'number' || p.interfaceNumber < 0) {
    out.push('interfaceNumber must be zero or more');
  }
  // Endpoint numbers are 1–15. Zero is the control endpoint and is never a bulk
  // endpoint; a zero here is the signature of an unfilled placeholder.
  for (const [name, value] of [
    ['endpointIn', p.endpointIn],
    ['endpointOut', p.endpointOut],
  ] as const) {
    if (typeof value !== 'number' || value < 1 || value > 15) {
      out.push(`${name} must be a bulk endpoint number 1-15 (0 is the control endpoint)`);
    }
  }
  if (!(p.captureCommand instanceof Uint8Array) || p.captureCommand.length === 0) {
    out.push('captureCommand is empty: there is no generic way to ask for a template');
  }
  if (typeof p.maxReplyBytes !== 'number' || p.maxReplyBytes < 64) {
    out.push('maxReplyBytes is too small to hold a record');
  }
  if (typeof p.captureTimeoutMs !== 'number' || p.captureTimeoutMs < 1000) {
    out.push('captureTimeoutMs is too short for an animal to hold still');
  }
  return out;
}

/* The slice of WebUSB this file uses. Declared here rather than depended on,
 * because @types/w3c-web-usb would be a new dependency and because writing out
 * exactly the surface we touch is the honest amount of API to claim. */
interface UsbSetup {
  requestType: 'standard' | 'class' | 'vendor';
  recipient: 'device' | 'interface' | 'endpoint' | 'other';
  request: number;
  value: number;
  index: number;
}
interface UsbTransferIn {
  status?: string;
  data?: DataView;
}
interface UsbTransferOut {
  status?: string;
  bytesWritten?: number;
}
export interface UsbDeviceLike {
  vendorId: number;
  productId: number;
  productName?: string;
  manufacturerName?: string;
  serialNumber?: string;
  opened: boolean;
  open(): Promise<void>;
  close(): Promise<void>;
  selectConfiguration(value: number): Promise<void>;
  claimInterface(n: number): Promise<void>;
  releaseInterface(n: number): Promise<void>;
  transferOut(endpoint: number, data: Uint8Array): Promise<UsbTransferOut>;
  transferIn(endpoint: number, length: number): Promise<UsbTransferIn>;
  controlTransferOut(setup: UsbSetup, data?: Uint8Array): Promise<UsbTransferOut>;
}
export interface UsbLike {
  requestDevice(options: {
    filters: { vendorId?: number; productId?: number }[];
  }): Promise<UsbDeviceLike>;
  getDevices(): Promise<UsbDeviceLike[]>;
}

/**
 * The IriTech IriShield MK 2120U over WebUSB.
 *
 * ============================================================================
 * WHAT THIS DEVICE IS FOR, AND WHY IT IS WORTH CARRYING
 * ============================================================================
 *
 * It has its own near-infrared illumination, and it does segmentation and
 * template extraction in firmware. Design §3 explains why the NIR matters:
 * cattle irises are dark brown, melanin is far more transparent around
 * 700-900 nm, and a visible-light sensor behind an IR-cut filter sees a dark
 * disc with almost no structure. A phone cannot do this and the IriShield can.
 *
 * The firmware extraction is the other half. `iris.ts` says at length that it
 * does not extract templates, and that getting extraction wrong produces
 * templates which compare beautifully to each other and mean nothing. A device
 * that hands back a finished record removes that entire class of mistake. None
 * of it is reimplemented here and none of it should be.
 *
 * ============================================================================
 * THE THING TO ESTABLISH BEFORE ANYTHING ELSE
 * ============================================================================
 *
 * **WebUSB may not be able to reach this device at all.**
 *
 * WebUSB claims a whole USB interface. On Windows and on macOS, an interface
 * the operating system has already bound to a class driver — HID above all —
 * cannot be claimed by a browser, and `claimInterface` throws. Many biometric
 * peripherals enumerate as HID precisely so they work without a driver install,
 * and if the IriShield is one of them then a browser-based capture app is not
 * possible on those platforms and the answer is a native host or WebHID, not a
 * change to this file.
 *
 * On Android — which design §6 makes the target platform — Chrome supports
 * WebUSB and there is no equivalent HID binding in the way, so the picture may
 * be better. It is still not known.
 *
 * The other platform facts, which are certain:
 *
 *   - WebUSB is Chromium only. Not Firefox, not Safari, and in practice
 *     nothing on iOS, where the browsers that ship are WebKit-based.
 *   - It requires a secure context and `requestDevice` requires a user gesture.
 *   - Permission is per-origin and per-device, and the operator re-grants it
 *     the first time on each new device. There is no fleet provisioning.
 *
 * ============================================================================
 * VERIFIED / UNVERIFIED
 * ============================================================================
 *
 * VERIFIED: the shape of the conversation (open, select configuration, claim
 * interface, write a command, read a reply); the mapping of each failure to an
 * `Outcome`; the parse and every consistency check on what comes back; that a
 * protocol with holes in it makes the source report itself unavailable rather
 * than fail at the device. All tested against a fake `UsbDeviceLike`.
 *
 * UNVERIFIED: every constant in `IriShieldProtocol`, the fact that the device
 * speaks a request/response protocol over bulk endpoints at all, and whether a
 * single `transferIn` returns a whole record or the first packet of one.
 */
export class IriShieldUSB implements CaptureSource {
  readonly kind = 'irishield-usb';
  readonly modalities: readonly Modality[] = ['iris'];

  private device: UsbDeviceLike | null = null;
  private descriptor: DeviceDescriptor | null = null;
  private readonly problems: string[];
  private readonly protocol: IriShieldProtocol;
  private readonly usb: UsbLike | undefined;
  private readonly codec: TemplateCodec | undefined;

  constructor(
    protocol: IriShieldProtocol,
    usb: UsbLike | undefined = (globalThis.navigator as unknown as { usb?: UsbLike })?.usb,
    codec?: TemplateCodec,
  ) {
    this.protocol = protocol;
    this.usb = usb;
    this.codec = codec;
    this.problems = protocolProblems(protocol);
  }

  available(): boolean {
    return this.usb !== undefined && this.problems.length === 0;
  }

  unconfirmed(): readonly Unconfirmed[] {
    return UNCONFIRMED_CONSTANTS.filter((u) => u.where.includes('IriShield'));
  }

  async connect(): Promise<Outcome<DeviceDescriptor>> {
    if (!this.usb) {
      return fail(
        'unsupported',
        'navigator.usb is absent: WebUSB needs a Chromium browser in a secure context, ' +
          'and is unavailable on iOS entirely',
      );
    }
    if (this.problems.length > 0) {
      // Deliberately not a connection attempt. Filtering on a placeholder
      // vendor id shows an empty chooser, and an empty chooser is
      // indistinguishable from a device that is not plugged in — which is the
      // exact confusion the whole no-fabricated-constants rule exists to stop.
      return fail(
        'unsupported',
        `the IriShield protocol is not confirmed, so no device can be requested: ` +
          this.problems.join('; '),
      );
    }

    try {
      const device = await this.usb.requestDevice({
        filters: [{ vendorId: this.protocol.vendorId, productId: this.protocol.productId }],
      });
      await device.open();
      await device.selectConfiguration(this.protocol.configurationValue);
      await device.claimInterface(this.protocol.interfaceNumber);

      this.device = device;
      this.descriptor = {
        kind: this.kind,
        label: [device.manufacturerName, device.productName].filter(Boolean).join(' ').trim(),
        handle: device.serialNumber ?? `usb:${device.vendorId}:${device.productId}`,
      };
      return { ok: true, value: this.descriptor };
    } catch (err) {
      const mapped = outcomeFromError<DeviceDescriptor>(err, 'IriShield connect');
      if (!mapped.ok && mapped.reason === 'refused' && /claim/i.test(mapped.detail)) {
        // Worth its own sentence, because it is the failure mode that decides
        // whether this approach works at all rather than one that can be fixed.
        return fail(
          'refused',
          mapped.detail +
            ' — an interface the operating system has already bound (HID, for instance) ' +
            'cannot be claimed by WebUSB. If this is what the IriShield does, a browser ' +
            'cannot drive it on this platform.',
        );
      }
      return mapped;
    }
  }

  async capture(): Promise<Outcome<Capture>> {
    const device = this.device;
    const descriptor = this.descriptor;
    if (!device || !descriptor) return fail('unreachable', 'not connected');

    let reply: Uint8Array;
    try {
      const written = await device.transferOut(this.protocol.endpointOut, this.protocol.captureCommand);
      if (written.status && written.status !== 'ok') {
        return fail('refused', `the device rejected the capture command: status ${written.status}`);
      }

      const got = await withTimeout(
        device.transferIn(this.protocol.endpointIn, this.protocol.maxReplyBytes),
        this.protocol.captureTimeoutMs,
        'waiting for the device to return a capture',
      );
      if (!got.ok) return got;

      const transfer = got.value;
      if (transfer.status === 'stall') {
        return fail(
          'refused',
          'the IN endpoint stalled — the usual cause is a command the device did not ' +
            'understand, which is what a guessed captureCommand looks like',
        );
      }
      if (transfer.status && transfer.status !== 'ok') {
        return fail('unreachable', `transfer status ${transfer.status}`);
      }
      if (!transfer.data || transfer.data.byteLength === 0) {
        // Not a failure of the device. An iris the firmware could not segment
        // is the commonest outcome in a pen, and it means "hold the camera
        // still and try again", not "the camera is broken".
        return fail('not-found', 'the device returned nothing: no iris was captured');
      }
      reply = new Uint8Array(
        transfer.data.buffer,
        transfer.data.byteOffset,
        transfer.data.byteLength,
      );
    } catch (err) {
      return outcomeFromError(err, 'IriShield capture');
    }

    const record = parseIrisRecord(reply);
    if (!record.ok) return record;

    const representation = record.value.representations[0];
    if (!representation) {
      return fail('not-found', 'the record carries no representations: no iris was captured');
    }

    // The template is attached when a codec can produce one, and the capture is
    // returned WITHOUT one otherwise. An enrolment of the record is still worth
    // recording — design §2 puts the templates in the biometric service anyway,
    // and the commitment is over the payload either way.
    const template = irisTemplateFrom(representation, this.codec);

    return {
      ok: true,
      value: {
        modality: 'iris',
        payload: reply,
        capturedAt: Date.now(),
        device: descriptor,
        // The record's own quality block, 0-100 in the standard, normalised.
        // Reported only when the device gave one: a missing quality block and a
        // quality of zero are different statements.
        quality:
          representation.quality.length > 0
            ? Math.min(1, Math.max(0, representation.quality[0]!.score / 100))
            : null,
        ...(template.ok ? { template: template.value } : {}),
        note: template.ok
          ? 'ISO 19794-6 record with an extracted template'
          : `ISO 19794-6 record, no template: ${template.detail}`,
      },
    };
  }

  async disconnect(): Promise<void> {
    const device = this.device;
    this.device = null;
    this.descriptor = null;
    if (!device) return;
    // Best effort. A device that has already been unplugged throws on both of
    // these, and an unplugged device is exactly when disconnect gets called.
    try {
      await device.releaseInterface(this.protocol.interfaceNumber);
    } catch {
      /* already gone */
    }
    try {
      await device.close();
    } catch {
      /* already gone */
    }
  }
}

/**
 * A promise with a deadline, as an outcome rather than a rejection.
 *
 * `transferIn` against a wrong endpoint does not fail — it waits, forever, for
 * bytes that are never coming. Without this the application shows a spinner
 * until the operator gives up, and the bug report says "it hangs".
 */
export async function withTimeout<T>(
  promise: Promise<T>,
  ms: number,
  what: string,
): Promise<Outcome<T>> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const deadline = new Promise<'timeout'>((resolve) => {
    timer = setTimeout(() => resolve('timeout'), ms);
  });
  try {
    const result = await Promise.race([promise, deadline]);
    if (result === 'timeout') return fail('unreachable', `timed out after ${ms}ms ${what}`);
    return { ok: true, value: result as T };
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }
}

/* ================================================== 2. the phone camera */

/**
 * One frame, already reduced to what the gate needs.
 *
 * `luma` is one byte per pixel. Separating it from the encoded image is what
 * makes the gate testable without a camera, a canvas or a DOM: a test builds a
 * raster directly and asks what the gate thinks of it.
 */
export interface Frame {
  width: number;
  height: number;
  /** Row-major, one byte per pixel, length must be width * height. */
  luma: Uint8Array;
  /** The image as it will be stored and committed. */
  encoded: Uint8Array;
  mime: string;
}

/** Whatever can produce a frame. Injected so the gate can be tested. */
export interface FrameGrabber {
  start(): Promise<Outcome<DeviceDescriptor>>;
  grab(): Promise<Outcome<Frame>>;
  stop(): Promise<void>;
}

/**
 * What the gate measured, and what it thinks.
 *
 * Every component is 0 to 1 and higher is better, INCLUDING the ones derived
 * from a quantity where higher is worse — `clipping` is one minus the clipped
 * fraction. Mixing the two directions inside one report is how a score ends up
 * inverted, and an inverted score passes exactly the captures it should refuse.
 */
export interface QualityReport {
  /** Normalised variance of the Laplacian. The blur measure. */
  sharpness: number;
  /** 1 at the middle of the range, falling off toward black and toward white. */
  exposure: number;
  /** 1 when nothing is at 0 or 255. */
  clipping: number;
  /** Spread of the histogram. A flat grey card scores 0 however sharp it is. */
  contrast: number;
  /** The LOWEST component, not the mean. See the note below. */
  score: number;
  pass: boolean;
  /** Catalogue keys, not sentences: this layer is not user-facing. */
  failures: string[];
}

/**
 * The gate.
 *
 * WRITTEN TO BE CALIBRATED, NOT BELIEVED — the same warning `iris.ts` puts on
 * its thresholds, and for the same reason. Design §3 is blunt about it: the
 * published figures are laboratory figures with a restrained animal, even
 * light, a clean dry muzzle and a cooperative handler, and the field has dust,
 * rain, a wet muzzle, a moving head and the sun behind the operator. These
 * numbers are a starting point for the first few hundred captures and nothing
 * more.
 *
 * Getting them wrong is expensive in both directions. Too strict and an
 * operator cannot enrol an animal in the rain, so they stop using the
 * application — design §3 again: a modality an operator will actually use twice
 * a day beats a better one they avoid. Too loose and the register fills with
 * captures that will never match anything, and the failure appears a year later
 * as the system rejecting legitimate beasts.
 */
export const QUALITY_GATE = {
  sharpness: 0.3,
  exposure: 0.5,
  clipping: 0.9,
  contrast: 0.25,
} as const;

/**
 * The Laplacian variance that counts as fully sharp.
 *
 * Arbitrary, and the single most calibration-hungry number in this file. It
 * scales with sensor, resolution and subject, so a value tuned on a muzzle at
 * 12 MP means nothing for a face at 2 MP.
 */
export const SHARPNESS_FULL_SCALE = 24;

/**
 * Score a frame.
 *
 * # The score is the minimum, and that is the design
 *
 * Averaging the components is the obvious thing and it is wrong. A capture that
 * is razor sharp, beautifully contrasty and completely blown out averages to
 * "fine", and the blown-out part is where the muzzle detail was. A capture is
 * as good as its worst property, so the score is the worst one, and the report
 * lists every component that failed rather than only the first.
 *
 * Pure. No DOM, no camera. Tested against rasters built in the test file.
 */
export function assessFrame(
  frame: Pick<Frame, 'width' | 'height' | 'luma'>,
  gate: typeof QUALITY_GATE = QUALITY_GATE,
): QualityReport {
  const { width, height, luma } = frame;
  if (width < 3 || height < 3 || luma.length !== width * height) {
    return {
      sharpness: 0,
      exposure: 0,
      clipping: 0,
      contrast: 0,
      score: 0,
      pass: false,
      failures: ['herd.frameMalformed'],
    };
  }

  let sum = 0;
  let clipped = 0;
  for (const p of luma) {
    sum += p;
    if (p === 0 || p === 255) clipped++;
  }
  const mean = sum / luma.length;

  let variance = 0;
  for (const p of luma) variance += (p - mean) * (p - mean);
  const stddev = Math.sqrt(variance / luma.length);

  // The 4-neighbour Laplacian, over the interior only: the border has no
  // neighbours and treating the missing ones as zero puts a bright false edge
  // all the way round, which inflates the sharpness of a completely blurred
  // frame by exactly the amount that would let it through.
  let lapSum = 0;
  let lapSumSq = 0;
  let n = 0;
  for (let y = 1; y < height - 1; y++) {
    for (let x = 1; x < width - 1; x++) {
      const i = y * width + x;
      const l =
        4 * luma[i]! - luma[i - 1]! - luma[i + 1]! - luma[i - width]! - luma[i + width]!;
      lapSum += l;
      lapSumSq += l * l;
      n++;
    }
  }
  const lapMean = lapSum / n;
  const lapStd = Math.sqrt(Math.max(0, lapSumSq / n - lapMean * lapMean));

  const sharpness = clamp01(lapStd / SHARPNESS_FULL_SCALE);
  // A triangle peaking at mid-grey. Dark and blown out both fall off, and they
  // fall off symmetrically because a wet muzzle over-exposes as easily as a
  // black coat under-exposes.
  const exposure = clamp01(1 - Math.abs(mean - 127.5) / 127.5);
  const clipping = clamp01(1 - clipped / luma.length);
  // 64 rather than 128 as full scale: a real capture never uses the whole
  // range, and requiring it would fail everything.
  const contrast = clamp01(stddev / 64);

  const failures: string[] = [];
  if (sharpness < gate.sharpness) failures.push('herd.captureBlurred');
  if (exposure < gate.exposure) {
    failures.push(mean < 127.5 ? 'herd.captureTooDark' : 'herd.captureTooBright');
  }
  if (clipping < gate.clipping) failures.push('herd.captureClipped');
  if (contrast < gate.contrast) failures.push('herd.captureFlat');

  return {
    sharpness,
    exposure,
    clipping,
    contrast,
    score: Math.min(sharpness, exposure, clipping, contrast),
    pass: failures.length === 0,
    failures,
  };
}

function clamp01(n: number): number {
  return Number.isFinite(n) ? Math.min(1, Math.max(0, n)) : 0;
}

/**
 * Stills from the phone's own camera, for muzzle, face and coat.
 *
 * ============================================================================
 * WHAT THE WEB PLATFORM CANNOT DO HERE, STATED RATHER THAN WORKED AROUND
 * ============================================================================
 *
 * Design §6 makes **close focus** the single most important specification and
 * the one that disqualifies a device: a muzzle print needs sharp detail at
 * 10-20 cm, and a fixed-focus 2 MP macro camera produces an unusable image.
 *
 * `getUserMedia` cannot command close focus and cannot measure it.
 *
 *   - `focusMode` and `focusDistance` exist in the MediaStreamTrack image
 *     capture specification, and are implemented in Chrome on Android and
 *     essentially nowhere else. A phone that does not implement them ignores
 *     the constraint silently.
 *   - There is no way to ask for "the macro camera". `enumerateDevices`
 *     returns empty labels before permission is granted and vendor strings
 *     afterwards, and nothing in them says which lens focuses closest.
 *   - `getUserMedia` delivers a preview stream. Full-sensor stills need
 *     `ImageCapture.takePhoto`, which is Chromium-only and on many devices
 *     returns the same resolution as the video track anyway.
 *   - The torch is a Chrome-on-Android constraint. §6's ring light is an
 *     accessory, not an API.
 *
 * So §6's instruction stands exactly as written: **verify by taking a picture,
 * not by reading a spec sheet.** This class cannot do it for you. What it can
 * do is refuse the resulting capture, which is what `assessFrame` is for, and
 * a device that cannot focus close will fail the sharpness component on every
 * muzzle — which is a usable acceptance test for a handset.
 *
 * VERIFIED: the quality gate, the refusal path, the state around start and
 * stop, and every `Outcome` mapping. Tested against an injected grabber.
 * UNVERIFIED: whether any given handset honours any constraint requested.
 */
export class PhoneCamera implements CaptureSource {
  readonly kind = 'phone-camera';
  readonly modalities: readonly Modality[] = ['muzzle', 'face', 'coat'];

  private descriptor: DeviceDescriptor | null = null;
  private readonly modality: 'muzzle' | 'face' | 'coat';
  private readonly grabber: FrameGrabber;
  private readonly gate: typeof QUALITY_GATE;

  constructor(
    modality: 'muzzle' | 'face' | 'coat',
    grabber: FrameGrabber,
    gate: typeof QUALITY_GATE = QUALITY_GATE,
  ) {
    this.modality = modality;
    this.grabber = grabber;
    this.gate = gate;
  }

  available(): boolean {
    return typeof globalThis.navigator?.mediaDevices?.getUserMedia === 'function';
  }

  unconfirmed(): readonly Unconfirmed[] {
    return UNCONFIRMED_CONSTANTS.filter(
      (u) => u.where === 'PhoneCamera' || u.where === 'QUALITY_GATE',
    );
  }

  async connect(): Promise<Outcome<DeviceDescriptor>> {
    const got = await this.grabber.start();
    if (got.ok) this.descriptor = got.value;
    return got;
  }

  async capture(): Promise<Outcome<Capture>> {
    const descriptor = this.descriptor;
    if (!descriptor) return fail('unreachable', 'the camera is not started');

    const got = await this.grabber.grab();
    if (!got.ok) return got;
    const frame = got.value;

    const quality = assessFrame(frame, this.gate);
    if (!quality.pass) {
      // `refused` and not `not-found`: the camera worked and the picture is not
      // good enough, which is a thing the operator can fix in ten seconds by
      // moving closer or turning round. Design §7 puts this gate in step one
      // for that reason — refusing a bad capture is most of the value before
      // any matching exists at all.
      return fail(
        'refused',
        `the capture failed the quality gate [${quality.failures.join(', ')}] ` +
          `sharpness ${quality.sharpness.toFixed(2)} exposure ${quality.exposure.toFixed(2)} ` +
          `clipping ${quality.clipping.toFixed(2)} contrast ${quality.contrast.toFixed(2)}`,
      );
    }

    return {
      ok: true,
      value: {
        modality: this.modality,
        payload: frame.encoded,
        capturedAt: Date.now(),
        device: descriptor,
        quality: quality.score,
        note: `${frame.width}x${frame.height} ${frame.mime}`,
      },
    };
  }

  async disconnect(): Promise<void> {
    this.descriptor = null;
    await this.grabber.stop();
  }
}

/**
 * The real grabber: `getUserMedia` into a video element into a canvas.
 *
 * Isolated here so that everything above is testable without a DOM. Nothing in
 * this function has been run.
 *
 * The constraints are requested as `ideal` rather than `exact` throughout. An
 * `exact` constraint a handset cannot meet makes `getUserMedia` reject
 * outright, and a handset that cannot deliver 3840 wide should give what it has
 * rather than nothing — the quality gate is what decides whether that is good
 * enough, and it can say why.
 */
export function mediaFrameGrabber(
  options: { width?: number; height?: number; mime?: string; quality?: number } = {},
): FrameGrabber {
  const { width = 3840, height = 2160, mime = 'image/jpeg', quality = 0.92 } = options;
  let stream: MediaStream | null = null;
  let video: HTMLVideoElement | null = null;

  return {
    async start(): Promise<Outcome<DeviceDescriptor>> {
      if (typeof globalThis.navigator?.mediaDevices?.getUserMedia !== 'function') {
        return fail(
          'unsupported',
          'navigator.mediaDevices.getUserMedia is absent: this needs a secure context',
        );
      }
      try {
        stream = await navigator.mediaDevices.getUserMedia({
          video: {
            facingMode: { ideal: 'environment' },
            width: { ideal: width },
            height: { ideal: height },
            // UNVERIFIED and widely ignored. Requested because on the handsets
            // that do honour it, it is the difference between a sharp muzzle
            // and a soft one; it is `ideal` because on the ones that do not,
            // `exact` would reject the whole stream.
            ...({ focusMode: { ideal: 'continuous' } } as Record<string, unknown>),
          },
          audio: false,
        });
      } catch (err) {
        return outcomeFromError(err, 'getUserMedia');
      }

      video = document.createElement('video');
      video.srcObject = stream;
      video.playsInline = true;
      video.muted = true;
      try {
        await video.play();
      } catch (err) {
        return outcomeFromError(err, 'starting the preview');
      }

      const track = stream.getVideoTracks()[0];
      const settings = track?.getSettings() ?? {};
      return {
        ok: true,
        value: {
          kind: 'phone-camera',
          label: track?.label ?? '',
          // Not a device serial. `deviceId` is per-origin and resets when site
          // data is cleared, which is fine for grouping a day's captures and is
          // not an identity.
          handle: String(settings.deviceId ?? 'camera'),
        },
      };
    },

    async grab(): Promise<Outcome<Frame>> {
      if (!video) return fail('unreachable', 'the camera is not started');
      const w = video.videoWidth;
      const h = video.videoHeight;
      if (w === 0 || h === 0) {
        return fail('not-found', 'the preview has no frame yet');
      }

      const canvas = document.createElement('canvas');
      canvas.width = w;
      canvas.height = h;
      const context = canvas.getContext('2d');
      if (!context) return fail('unsupported', 'no 2d canvas context');
      context.drawImage(video, 0, 0, w, h);

      const pixels = context.getImageData(0, 0, w, h).data;
      const luma = new Uint8Array(w * h);
      for (let i = 0, p = 0; i < luma.length; i++, p += 4) {
        // Rec. 601 luma. Integer coefficients so the result is reproducible
        // across engines; the gate's thresholds were never going to be
        // sensitive to the third decimal place of a weighting.
        luma[i] = (299 * pixels[p]! + 587 * pixels[p + 1]! + 114 * pixels[p + 2]!) / 1000;
      }

      const blob = await new Promise<Blob | null>((resolve) =>
        canvas.toBlob(resolve, mime, quality),
      );
      if (!blob) return fail('unsupported', `the canvas would not encode ${mime}`);

      return {
        ok: true,
        value: {
          width: w,
          height: h,
          luma,
          encoded: new Uint8Array(await blob.arrayBuffer()),
          mime,
        },
      };
    },

    async stop(): Promise<void> {
      // Every track, explicitly. A stream whose tracks are left running holds
      // the camera open, the indicator light on, and the battery draining — and
      // on Android a second getUserMedia then fails with a device-in-use error
      // that reads like a hardware fault.
      for (const track of stream?.getTracks() ?? []) track.stop();
      stream = null;
      if (video) {
        video.srcObject = null;
        video = null;
      }
    },
  };
}

/* ============================================ 3. the LF transponder reader */

/**
 * A GATT description of a reader. Every field here is unknown per model.
 */
export interface LFReaderProtocol {
  /** The GATT primary service UUID. Must be named up front; cannot be discovered. */
  serviceUuid: string;
  /** The characteristic the reader notifies on. */
  notifyUuid: string;
  /** The characteristic to write commands to, when the reader takes commands. */
  writeUuid?: string;
  /** Advertised-name prefix, when filtering by service alone finds too much. */
  namePrefix?: string;
  /** Bytes that ask the reader to scan, for readers that need asking. */
  scanCommand?: Uint8Array;
  /** How long to wait for a tag to come into range. */
  readTimeoutMs: number;
  /** Turn one line from the reader into a code. Defaults to `readerLineToTag`. */
  parseLine?: (line: string) => Outcome<TagCode>;
}

/**
 * Nordic's UART Service, as a starting point and NOT as an answer.
 *
 * These three UUIDs are real and published by Nordic Semiconductor, and a large
 * number of BLE serial bridges use them because they ship on Nordic silicon
 * with Nordic's example firmware. That is why they are here.
 *
 * What is NOT known is whether the reader you buy uses them. Quoting a real
 * constant in the wrong place is still a fabrication, so: check the reader's
 * documentation, or point a GATT explorer at it, before assuming this preset.
 * A wrong service UUID does not degrade the connection, it prevents one, and
 * Web Bluetooth reports it as "service not found" — which reads exactly like a
 * reader that is switched off.
 */
export const NORDIC_UART = {
  serviceUuid: '6e400001-b5a3-f393-e0a9-e50e24dcca9e',
  /** Device to phone. Notify. */
  notifyUuid: '6e400003-b5a3-f393-e0a9-e50e24dcca9e',
  /** Phone to device. Write. */
  writeUuid: '6e400002-b5a3-f393-e0a9-e50e24dcca9e',
} as const;

/**
 * Notification chunks, reassembled into lines.
 *
 * ============================================================================
 * WHY THIS EXISTS AND IS TESTED
 * ============================================================================
 *
 * A BLE notification carries at most MTU minus three bytes, and the default MTU
 * is 23 — so twenty bytes per packet until something negotiates more, and
 * Android's negotiation is not something a web page controls. A reader that
 * emits `LA982000000000003\r\n` sends twenty bytes and then two, and a
 * handler that parses each notification on its own sees a seventeen-digit
 * fragment, decides it is not a tag, and throws away a perfectly good read.
 *
 * The symptom is a reader that works sometimes — specifically, whenever the
 * line happens to fit — which is the worst kind of intermittent.
 *
 * Handles CR, LF and CRLF, because readers disagree and the byte that ends a
 * line is not something anybody documents.
 */
export class LineAssembler {
  private buffer = '';
  private readonly decoder = new TextDecoder('ascii');
  private readonly maxBuffer: number;

  // Written out rather than declared as a constructor parameter property,
  // here and in every class below. Node's type stripping — which is what
  // `node --test src/*.test.ts` runs on — refuses parameter properties,
  // because erasing the type would erase the assignment with it.
  constructor(maxBuffer = 4096) {
    this.maxBuffer = maxBuffer;
  }

  /** Feed a notification. Returns whatever complete lines that completed. */
  push(chunk: Uint8Array): string[] {
    this.buffer += this.decoder.decode(chunk);

    // A reader that never sends a terminator would otherwise grow this string
    // until the tab dies. Dropping the oldest bytes keeps the newest read,
    // which is the one the operator is waiting for.
    if (this.buffer.length > this.maxBuffer) {
      this.buffer = this.buffer.slice(-this.maxBuffer);
    }

    const parts = this.buffer.split(/\r\n|\r|\n/);
    // The last element is whatever came after the final terminator: either an
    // incomplete line, or the empty string when the chunk ended cleanly.
    this.buffer = parts.pop() ?? '';
    return parts.filter((line) => line.trim() !== '');
  }

  /** Anything held back, for a reader that stops without a terminator. */
  flush(): string[] {
    const rest = this.buffer.trim();
    this.buffer = '';
    return rest === '' ? [] : [rest];
  }

  reset(): void {
    this.buffer = '';
  }
}

/**
 * The default line parser: the first fifteen-digit run in the line.
 *
 * UNVERIFIED as a match for any particular reader, and deliberately narrow. It
 * covers the common case — an ASCII line with a prefix, the code, and possibly
 * a trailing field — and it refuses anything where the fifteen digits sit
 * inside a longer run, because `9820000000000031` is either a different format
 * or a corrupted read and guessing which would be how a wrong tag gets
 * enrolled.
 *
 * Readers that emit binary, or that append a checksum without a separator,
 * need their own parser. Supply `parseLine` rather than hoping.
 */
export function readerLineToTag(line: string): Outcome<TagCode> {
  const trimmed = line.trim();
  if (trimmed === '') return fail('not-found', 'empty line');

  // Bounded by non-digits on both sides. A longer digit run is not a tag with
  // extra characters, it is something this parser does not understand.
  const match = /(?<![0-9])([0-9]{15})(?![0-9])/.exec(trimmed);
  if (!match) {
    return fail('not-found', `no 15-digit transponder code in ${JSON.stringify(trimmed)}`);
  }

  const parsed = parseTagCode(match[1]!);
  if (!parsed.ok) return fail('refused', `${parsed.reason}: ${parsed.detail}`);
  return { ok: true, value: parsed.value };
}

/* The slice of Web Bluetooth this file uses. Declared rather than depended on,
 * for the same reason as the WebUSB block above. */
export interface GattCharacteristicLike {
  value?: DataView;
  startNotifications(): Promise<GattCharacteristicLike>;
  stopNotifications(): Promise<GattCharacteristicLike>;
  writeValue(value: Uint8Array): Promise<void>;
  addEventListener(type: string, listener: (event: Event) => void): void;
  removeEventListener(type: string, listener: (event: Event) => void): void;
}
export interface GattServiceLike {
  getCharacteristic(uuid: string): Promise<GattCharacteristicLike>;
}
export interface GattServerLike {
  connected: boolean;
  connect(): Promise<GattServerLike>;
  disconnect(): void;
  getPrimaryService(uuid: string): Promise<GattServiceLike>;
}
export interface BluetoothDeviceLike {
  id: string;
  name?: string;
  gatt?: GattServerLike;
}
export interface BluetoothLike {
  getAvailability?(): Promise<boolean>;
  requestDevice(options: {
    filters?: { services?: string[]; namePrefix?: string }[];
    optionalServices?: string[];
  }): Promise<BluetoothDeviceLike>;
}

/**
 * An ISO 11784/11785 stick reader over Web Bluetooth.
 *
 * ============================================================================
 * WHY THIS CLASS EXISTS AT ALL — design §1.1
 * ============================================================================
 *
 * ISO 11784/11785 transponders work at **134.2 kHz**. Every NFC radio in every
 * phone and tablet is **13.56 MHz**. Different bands, different antennas,
 * different physics. No phone on the market reads an animal implant, and no
 * software fixes it. A field team issued with phones and told to scan the chip
 * finds this out on the first farm, which is why §1.1 puts it first.
 *
 * So the reader is a second object to carry and charge, and this class talks to
 * it.
 *
 * ============================================================================
 * THE PROCUREMENT DECISION THIS CODE CANNOT MAKE
 * ============================================================================
 *
 * **Web Bluetooth speaks BLE GATT and nothing else.** It cannot open a
 * Bluetooth Classic serial port. A great many livestock stick readers — the
 * established ones, the ones already in the field — expose exactly that: the
 * Serial Port Profile, over Bluetooth Classic, which is what their Windows and
 * Android native applications use.
 *
 * **A reader like that is unreachable from a browser on every platform there
 * is.** Not slow, not awkward: unreachable. If the chosen reader turns out to
 * be SPP, the options are a native Android application, a USB-OTG cable to a
 * reader that also does serial-over-USB, or a different reader — and that is a
 * purchasing decision, not a bug to be assigned.
 *
 * So before a reader is bought, ask the vendor, in these words: *does it expose
 * a BLE GATT service, and what are the service and characteristic UUIDs?* An
 * answer of "yes, Bluetooth" is not an answer to that question.
 *
 * The rest of the platform facts, which are certain:
 *
 *   - Web Bluetooth is unavailable on iOS in Safari and in every browser
 *     anybody will be issued. Treat it as absent there.
 *   - On Android it is Chromium only, needs a secure context, needs a user
 *     gesture, and the operating system requires location permission for a BLE
 *     scan — which operators find alarming and which somebody has to explain.
 *   - A service not named in `filters` or `optionalServices` **cannot be
 *     discovered**. There is no enumeration. Guessing a UUID is not a
 *     degraded experience, it is no connection.
 *
 * VERIFIED: line reassembly across notification boundaries, the line-to-tag
 * parse, the refusal of a test transponder, the timeout path, every `Outcome`
 * mapping, and the connect sequence against a fake GATT stack.
 * UNVERIFIED: the UUIDs, the wire format, and whether the device speaks BLE.
 */
export class BluetoothLFReader implements CaptureSource {
  readonly kind = 'bt-lf-reader';
  readonly modalities: readonly Modality[] = ['tag'];

  private device: BluetoothDeviceLike | null = null;
  private characteristic: GattCharacteristicLike | null = null;
  private descriptor: DeviceDescriptor | null = null;
  private readonly lines = new LineAssembler();
  private readonly protocol: LFReaderProtocol;
  private readonly bluetooth: BluetoothLike | undefined;

  constructor(
    protocol: LFReaderProtocol,
    bluetooth: BluetoothLike | undefined = (
      globalThis.navigator as unknown as { bluetooth?: BluetoothLike }
    )?.bluetooth,
  ) {
    this.protocol = protocol;
    this.bluetooth = bluetooth;
  }

  available(): boolean {
    return this.bluetooth !== undefined;
  }

  unconfirmed(): readonly Unconfirmed[] {
    return UNCONFIRMED_CONSTANTS.filter((u) => u.where === 'BluetoothLFReader');
  }

  async connect(): Promise<Outcome<DeviceDescriptor>> {
    if (!this.bluetooth) {
      return fail(
        'unsupported',
        'navigator.bluetooth is absent: Web Bluetooth needs a Chromium browser in a ' +
          'secure context and does not exist on iOS at all',
      );
    }

    try {
      const filters: { services?: string[]; namePrefix?: string }[] = [
        { services: [this.protocol.serviceUuid] },
      ];
      if (this.protocol.namePrefix) filters.push({ namePrefix: this.protocol.namePrefix });

      const device = await this.bluetooth.requestDevice({
        filters,
        // Named again here because a device matched by namePrefix would
        // otherwise have no permission to reach the service at all.
        optionalServices: [this.protocol.serviceUuid],
      });
      if (!device.gatt) {
        return fail('unsupported', 'the chosen device exposes no GATT server');
      }

      const server = await device.gatt.connect();
      const service = await server.getPrimaryService(this.protocol.serviceUuid);
      const characteristic = await service.getCharacteristic(this.protocol.notifyUuid);
      await characteristic.startNotifications();

      this.device = device;
      this.characteristic = characteristic;
      this.lines.reset();
      this.descriptor = {
        kind: this.kind,
        label: device.name ?? '',
        // Opaque and per-origin, not a serial number. It groups a day's reads
        // and it must never be presented as the reader's identity.
        handle: `bt:${device.id}`,
      };
      return { ok: true, value: this.descriptor };
    } catch (err) {
      const mapped = outcomeFromError<DeviceDescriptor>(err, 'LF reader connect');
      if (!mapped.ok && /no services matching|service.*not found/i.test(mapped.detail)) {
        return fail(
          'unsupported',
          mapped.detail +
            ` — service ${this.protocol.serviceUuid} was not found. Either the UUID is ` +
            `wrong, or the reader speaks Bluetooth Classic serial, which Web Bluetooth ` +
            `cannot reach on any platform.`,
        );
      }
      return mapped;
    }
  }

  /**
   * Wait for one transponder.
   *
   * The listener is attached before the scan command is written. A reader that
   * answers within a millisecond would otherwise notify into nothing, and the
   * read would be lost until the operator waved the stick again — which they
   * will do, and which will work, and which makes the bug look like the animal
   * moved.
   */
  async capture(): Promise<Outcome<Capture>> {
    const characteristic = this.characteristic;
    const descriptor = this.descriptor;
    if (!characteristic || !descriptor) return fail('unreachable', 'the reader is not connected');

    const parse = this.protocol.parseLine ?? readerLineToTag;
    this.lines.reset();

    let listener: ((event: Event) => void) | null = null;
    const waiting = new Promise<Outcome<TagCode>>((resolve) => {
      listener = (event: Event) => {
        const target = event.target as { value?: DataView } | null;
        const data = target?.value ?? characteristic.value;
        if (!data) return;
        const chunk = new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
        for (const line of this.lines.push(chunk)) {
          const got = parse(line);
          // A line that is not a tag is not a failure: readers emit status
          // lines, battery lines and echoes of their own commands. Keep
          // listening rather than reporting the first one as a bad read.
          if (got.ok) {
            resolve(got);
            return;
          }
          if (got.reason === 'refused') {
            // Fifteen digits that are not a valid code. That IS worth stopping
            // on: it means the reader read something and this did not
            // understand it, which is a protocol problem, not a quiet one.
            resolve(got);
            return;
          }
        }
      };
      characteristic.addEventListener('characteristicvaluechanged', listener);
    });

    try {
      if (this.protocol.scanCommand && this.protocol.writeUuid) {
        await characteristic.writeValue(this.protocol.scanCommand);
      }
      const got = await withTimeout(waiting, this.protocol.readTimeoutMs, 'for a transponder');
      if (!got.ok) {
        // A timeout here is almost always "no tag in range" rather than "the
        // reader is gone", and those need different sentences on a screen.
        return fail(
          'not-found',
          `no transponder read within ${this.protocol.readTimeoutMs}ms — ` +
            `hold the reader closer, or the animal has no chip`,
        );
      }
      if (!got.value.ok) return got.value;
      const tag = got.value.value;

      if (tag.authority === 'test') {
        return fail(
          'refused',
          `${tag.country}${tag.national} is a test transponder (country code 999) and ` +
            `must not be enrolled against a real animal`,
        );
      }

      const digits = `${tag.country}${tag.national}`;
      return {
        ok: true,
        value: {
          modality: 'tag',
          payload: new TextEncoder().encode(digits),
          capturedAt: Date.now(),
          device: descriptor,
          // A transponder read is right or it is not; there is no degraded
          // read. Null rather than 1, because claiming a perfect quality score
          // would put a number in the enrolment record that means nothing.
          quality: null,
          identifier: tag,
          note: `ISO 11784/11785, ${tag.authority} code ${tag.country}`,
        },
      };
    } catch (err) {
      return outcomeFromError(err, 'LF reader capture');
    } finally {
      if (listener) characteristic.removeEventListener('characteristicvaluechanged', listener);
    }
  }

  async disconnect(): Promise<void> {
    const characteristic = this.characteristic;
    const device = this.device;
    this.characteristic = null;
    this.device = null;
    this.descriptor = null;
    this.lines.reset();
    try {
      await characteristic?.stopNotifications();
    } catch {
      /* already gone */
    }
    try {
      device?.gatt?.disconnect();
    } catch {
      /* already gone */
    }
  }
}

/* ================================================== 4. manual entry */

/** What a person is typing. */
export type ManualKind = 'national-id' | 'tag' | 'tattoo';

/** How the application asks. Injected, so this is testable without a screen. */
export type ManualPrompt = (kind: ManualKind) => Promise<string | null>;

/**
 * A person, typing.
 *
 * ============================================================================
 * WHY THIS IS NOT A FALLBACK
 * ============================================================================
 *
 * Design §1.1, last paragraph: manual entry must ALWAYS be available, because a
 * tattoo in a dusty ear at dusk is sometimes simply unreadable. Not "if OCR
 * fails" — always. `available()` returns true unconditionally and there is no
 * configuration that can switch it off, because an application that can reach a
 * state where an operator cannot record what they are looking at is an
 * application that gets closed and replaced with a notebook.
 *
 * The cost is honestly recorded rather than hidden. `quality` is null: there is
 * no such thing as a good or bad typed identifier. The modality on the capture
 * says it was typed, so an enrolment sourced this way is visibly weaker
 * evidence than a read one, and the registry can see which is which.
 *
 * And a typed transponder number has no checksum to catch a transposition —
 * `parseTagCode`'s comment says so at length. Two independent markers, per
 * design §3, and a disagreement between them is a finding rather than an error.
 *
 * VERIFIED: everything. There is no hardware.
 */
export class ManualEntry implements CaptureSource {
  readonly kind = 'manual';
  readonly modalities: readonly Modality[] = ['national-id', 'tag', 'tattoo'];

  private readonly descriptor: DeviceDescriptor = {
    kind: 'manual',
    label: 'typed by the operator',
    handle: 'manual',
  };

  private readonly entryKind: ManualKind;
  private readonly prompt: ManualPrompt;

  constructor(entryKind: ManualKind, prompt: ManualPrompt) {
    this.entryKind = entryKind;
    this.prompt = prompt;
  }

  /** Always. See the class comment: this is the point of it. */
  available(): boolean {
    return true;
  }

  unconfirmed(): readonly Unconfirmed[] {
    return [];
  }

  async connect(): Promise<Outcome<DeviceDescriptor>> {
    return { ok: true, value: this.descriptor };
  }

  async capture(): Promise<Outcome<Capture>> {
    let typed: string | null;
    try {
      typed = await this.prompt(this.entryKind);
    } catch (err) {
      return outcomeFromError(err, 'manual entry');
    }

    // Cancelled. Not a failure of anything, and it must not be reported as one
    // or the operator gets an error for pressing the button marked "cancel".
    if (typed === null) return fail('not-found', 'the operator cancelled');
    if (typed.trim() === '') return fail('not-found', 'nothing was typed');

    const encoder = new TextEncoder();
    const base = { capturedAt: Date.now(), device: this.descriptor, quality: null as number | null };

    switch (this.entryKind) {
      case 'national-id': {
        const parsed = parseNationalId(typed);
        if (!parsed.ok) return fail('refused', `${parsed.reason}: ${parsed.detail}`);
        const text = `${parsed.value.country}/${parsed.value.flock}/${parsed.value.serial}/${parsed.value.breed}`;
        return {
          ok: true,
          value: {
            ...base,
            modality: 'national-id',
            payload: encoder.encode(text),
            identifier: parsed.value,
            note: 'typed national identifier',
          },
        };
      }
      case 'tag': {
        const parsed = parseTagCode(typed);
        if (!parsed.ok) return fail('refused', `${parsed.reason}: ${parsed.detail}`);
        if (parsed.value.authority === 'test') {
          return fail('refused', 'country code 999 is a test transponder');
        }
        const digits = `${parsed.value.country}${parsed.value.national}`;
        return {
          ok: true,
          value: {
            ...base,
            modality: 'tag',
            payload: encoder.encode(digits),
            identifier: parsed.value,
            // Said out loud in the record, because the same field filled by
            // BluetoothLFReader means something stronger.
            note: 'typed transponder code — no checksum protects this against a transposition',
          },
        };
      }
      case 'tattoo': {
        // No structure is imposed. A tattoo is whatever was inked into the ear,
        // by whoever inked it, and a client-side format rule here would refuse
        // real animals to satisfy a convention nobody agreed to. Whitespace is
        // squeezed because `AB 123` and `AB  123` are the same tattoo and the
        // chain would index them as two.
        const text = typed.trim().replace(/\s+/g, ' ');
        return {
          ok: true,
          value: {
            ...base,
            modality: 'tattoo',
            payload: encoder.encode(text),
            identifier: { tattoo: text },
            note: 'transcribed tattoo — no format is checked, because none is agreed',
          },
        };
      }
    }
  }

  async disconnect(): Promise<void> {
    /* nothing to release */
  }
}

/* ============================================== the capture session */

/**
 * Where a capture session is.
 *
 *   idle        nothing connected
 *   connecting  a chooser is open, or a GATT connection is being made
 *   ready       a device is connected and waiting
 *   capturing   a capture is in flight
 *   failed      something needs a person before anything else can happen
 *   closed      finished with, deliberately
 */
export type SessionState = 'idle' | 'connecting' | 'ready' | 'capturing' | 'failed' | 'closed';

export type SessionEvent =
  | { type: 'connect' }
  | { type: 'connected' }
  | { type: 'connect-failed'; reason: FailureReason }
  | { type: 'capture' }
  | { type: 'captured' }
  | { type: 'capture-failed'; reason: FailureReason }
  | { type: 'retry' }
  | { type: 'disconnect' }
  | { type: 'close' };

/**
 * The transition.
 *
 * ============================================================================
 * WHY THE FAILURE REASON IS AN ARGUMENT HERE
 * ============================================================================
 *
 * This is where the four-way `Outcome` earns itself. After a failed capture the
 * right next state depends entirely on WHICH failure it was:
 *
 *   - `not-found`  nothing was there. The device is fine. Back to `ready`, and
 *                  the operator tries again — which is the overwhelmingly
 *                  common case in a pen and must cost one tap.
 *   - `refused`    the device or the gate said no. Also back to `ready`: a
 *                  blurred muzzle is fixed by standing closer, not by
 *                  reconnecting a camera that is working perfectly.
 *   - `unreachable` the device has gone. Back to `idle`, because `ready` would
 *                  be a lie and the next capture would fail the same way.
 *   - `unsupported` this will never work here. `failed`, and a person has to
 *                  read something.
 *
 * Collapse those into one and you get an application that either makes an
 * operator re-pair a reader after every missed tag, or leaves them tapping
 * capture against a stick that is in the truck.
 *
 * ============================================================================
 * THE DOUBLE TAP
 * ============================================================================
 *
 * `connect` from `connecting` is ignored, and so is `capture` from `capturing`.
 * An operator in the sun with gloves on taps twice, and two `requestDevice`
 * calls or two overlapping GATT connections is a real bug with a confusing
 * symptom. The state machine is where that gets handled, once, rather than in
 * every button.
 *
 * Pure and total: every state times every event has an answer, and unknown
 * combinations stay put rather than throwing.
 */
export function step(state: SessionState, event: SessionEvent): SessionState {
  if (event.type === 'close') return 'closed';
  if (state === 'closed') return 'closed';
  if (event.type === 'disconnect') return 'idle';

  switch (state) {
    case 'idle':
      // A capture from idle is not an error state, it is a no-op: there is
      // nothing to capture with, and dropping into `failed` would make a
      // mistimed tap look like a broken device.
      return event.type === 'connect' ? 'connecting' : 'idle';

    case 'connecting':
      switch (event.type) {
        case 'connected':
          return 'ready';
        case 'connect-failed':
          // Nothing was chosen, or permission was declined. Both leave the
          // application exactly where it started, which is `idle` — the
          // operator can press the button again. `unsupported` is different:
          // pressing it again will never work.
          return event.reason === 'unsupported' ? 'failed' : 'idle';
        default:
          return 'connecting';
      }

    case 'ready':
      switch (event.type) {
        case 'capture':
          return 'capturing';
        case 'connect':
          return 'connecting';
        default:
          return 'ready';
      }

    case 'capturing':
      switch (event.type) {
        case 'captured':
          return 'ready';
        case 'capture-failed':
          switch (event.reason) {
            case 'not-found':
            case 'refused':
              return 'ready';
            case 'unreachable':
              return 'idle';
            case 'unsupported':
              return 'failed';
          }
          return 'ready';
        default:
          return 'capturing';
      }

    case 'failed':
      return event.type === 'retry' ? 'idle' : 'failed';
  }
}

/**
 * A source, driven through the state machine, collecting captures.
 *
 * Thin on purpose: the interesting logic is in `step`, which is pure and
 * tested, and this is the part that awaits things. Everything it does to the
 * state it does through `step`, so the two cannot drift.
 */
export class CaptureSession {
  private current: SessionState = 'idle';
  private readonly collected: Capture[] = [];
  private readonly source: CaptureSource;
  private readonly onChange: (state: SessionState) => void;

  constructor(source: CaptureSource, onChange: (state: SessionState) => void = () => {}) {
    this.source = source;
    this.onChange = onChange;
  }

  get state(): SessionState {
    return this.current;
  }

  get captures(): readonly Capture[] {
    return this.collected;
  }

  private move(event: SessionEvent): void {
    const next = step(this.current, event);
    if (next === this.current) return;
    this.current = next;
    this.onChange(next);
  }

  async connect(): Promise<Outcome<DeviceDescriptor>> {
    const from = this.current;
    // `step` treats a second `connect` while connecting as a no-op, which is
    // right for the state but wrong for the caller: a silent no-op would let
    // the second tap fall through and open a second chooser. Refused here,
    // loudly, and the state machine still holds the rule about the state.
    if (from === 'connecting') {
      return fail('refused', 'already connecting: a second chooser would open a second connection');
    }
    // Asked of `step` BEFORE moving, rather than moved and then inspected. The
    // state machine is the one place that decides what is legal, and a second
    // opinion here would be a second place for the rule to live.
    if (step(from, { type: 'connect' }) !== 'connecting') {
      return fail('refused', `cannot connect from ${from}`);
    }
    this.move({ type: 'connect' });

    const got = await this.source.connect();
    this.move(got.ok ? { type: 'connected' } : { type: 'connect-failed', reason: got.reason });
    return got;
  }

  async capture(): Promise<Outcome<Capture>> {
    if (this.current !== 'ready') {
      // Named states in the detail, because "cannot capture" with no reason is
      // the message that generates the support call.
      return fail('refused', `cannot capture from ${this.current}: connect a device first`);
    }
    this.move({ type: 'capture' });

    const got = await this.source.capture();
    if (got.ok) {
      this.collected.push(got.value);
      this.move({ type: 'captured' });
    } else {
      this.move({ type: 'capture-failed', reason: got.reason });
    }
    return got;
  }

  async disconnect(): Promise<void> {
    await this.source.disconnect();
    this.move({ type: 'disconnect' });
  }

  async close(): Promise<void> {
    await this.source.disconnect();
    this.move({ type: 'close' });
  }
}
