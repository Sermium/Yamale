/**
 * The field kit: the application's `Device` built out of real capture sources.
 *
 * `ports.ts` says what the screens need — a tag reader, an iris camera, a
 * tattoo reader, a position — in the shape a screen asks questions in.
 * `device.ts` says what the hardware offers: a uniform `CaptureSource` with
 * `connect`, `capture`, `disconnect`, and an honest register of everything
 * about the protocol nobody has confirmed. This joins the two.
 *
 * It is its own file because the two halves were written against each other's
 * descriptions rather than each other's code, and this is where their
 * differences live. Folding it into either would hide a seam worth seeing.
 *
 * # Two words called "identity"
 *
 * `ports.ts` has an `Identity` that SIGNS — an operator and their key.
 * `identity.ts` is about IDENTIFIERS — the national number and the transponder
 * code. Same word, unrelated things. Noted because it is exactly the sort of
 * collision that gets miswired once and then believed; this file touches only
 * the second, and only through `device.ts`.
 *
 * # A source with an unconfirmed protocol is not constructed at all
 *
 * `IriShieldUSB` and `BluetoothLFReader` both REQUIRE their protocol — no
 * default, because there is no honest default for a USB endpoint or a GATT
 * UUID. So when one has not been supplied, this holds no source rather than a
 * source that pretends, and the capability reports `unsupported` with the list
 * of what the vendor still has to confirm.
 *
 * That is the same reasoning as `NO_DEVICE` in ports.ts: a reader that invented
 * a plausible code would put a fabricated identity on the one screen built to
 * refuse exactly that, and the fabrication would survive until somebody trusted
 * it.
 */

import {
  BluetoothLFReader,
  IriShieldUSB,
  PhoneCamera,
  UNCONFIRMED_CONSTANTS,
  type Capture as DeviceCapture,
  type CaptureSource,
  type FrameGrabber,
  type IriShieldProtocol,
  type LFReaderProtocol,
  type Outcome,
  type Unconfirmed,
} from './device.ts';
import { formatTagCode } from './identity.ts';
import type {
  CapabilityState,
  Capture as PortCapture,
  Device,
  IrisCamera,
  Positioner,
  TagReader,
  TattooReader,
} from './ports.ts';

/**
 * What the kit was built with.
 *
 * Every field is optional and absent is the normal state today. Absent means a
 * capability that says why it cannot work, not one that fails at a chooser.
 */
export interface KitOptions {
  /** The IriShield's USB protocol, once the vendor has confirmed it. */
  irisProtocol?: IriShieldProtocol;
  /** The stick reader's BLE GATT service, once the vendor has confirmed it. */
  tagProtocol?: LFReaderProtocol;
  /**
   * The camera, which this file does not implement.
   *
   * `PhoneCamera` takes a `FrameGrabber` — a `getUserMedia` wrapper that starts
   * a stream and hands over frames. Supplying it is the application's job
   * because it owns the video element the operator is looking at.
   */
  grabber?: FrameGrabber;
  /** Overridden in tests. */
  deviceIdFor?: () => string;
}

const UNSUPPORTED_DETAIL = (source: string, missing: readonly Unconfirmed[]): CapabilityState => ({
  kind: 'unsupported',
  detail: missing.length > 0
    ? `${source}: ${missing.length} protocol detail(s) unconfirmed — ${
      missing.map((u) => u.name).join('; ')
    }`
    : `${source} is not available in this browser`,
});

/** A source wrapped so a screen never has to think about connecting. */
class Attached {
  private connected = false;
  private readonly listeners = new Set<(s: CapabilityState) => void>();

  constructor(
    private readonly source: CaptureSource | null,
    private readonly label: string,
    private readonly missing: readonly Unconfirmed[],
  ) {}

  state(): CapabilityState {
    if (!this.source) return UNSUPPORTED_DETAIL(this.label, this.missing);
    if (!this.source.available()) {
      return UNSUPPORTED_DETAIL(this.label, this.source.unconfirmed());
    }
    return this.connected ? { kind: 'ready' } : { kind: 'disconnected' };
  }

  watch(listener: (s: CapabilityState) => void): () => void {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }

  private announce() {
    const s = this.state();
    for (const l of this.listeners) l(s);
  }

  /**
   * Connect if necessary, then capture.
   *
   * Folded together because a screen's "read the tag" button is the user
   * gesture, and `requestDevice` needs one. Making the operator press "connect"
   * and then "read" spends the gesture on the wrong button.
   */
  async capture(): Promise<Outcome<DeviceCapture>> {
    if (!this.source) {
      const state = this.state();
      return {
        ok: false,
        reason: 'unsupported',
        detail: state.kind === 'ready' ? this.label : (state.detail ?? this.label),
      };
    }
    if (!this.connected) {
      const opened = await this.source.connect();
      if (!opened.ok) return opened;
      this.connected = true;
      this.announce();
    }
    const got = await this.source.capture();
    if (!got.ok && got.reason === 'unreachable') {
      // The device went away mid-capture: a stick reader out of range, a cable
      // pulled. Marked disconnected so the screen stops claiming it is ready.
      this.connected = false;
      this.announce();
    }
    return got;
  }

  async release(): Promise<void> {
    if (!this.source || !this.connected) return;
    await this.source.disconnect();
    this.connected = false;
    this.announce();
  }

  unconfirmed(): readonly Unconfirmed[] {
    return this.source ? this.source.unconfirmed() : this.missing;
  }
}

/**
 * Reject rather than resolve on failure.
 *
 * `ports.ts` types these as promises that reject and the outbox records why.
 * Flattening a failure into a resolved value would make every caller check a
 * field the type does not mention.
 */
async function demand<T>(what: string, outcome: Promise<Outcome<T>>): Promise<T> {
  const got = await outcome;
  if (got.ok) return got.value;
  throw new Error(`${what}: ${got.reason} — ${got.detail}`);
}

/**
 * The two `Capture` types are not the same, and this is the only place that
 * knows it.
 *
 * `device.ts` carries the payload bytes and the source's own identifier
 * parsing; `ports.ts` carries what an enrolment record needs. The narrowing is
 * deliberate: a screen has no business with raw payload bytes, and giving it
 * some would invite a screen to re-parse what the device already parsed.
 */
function toPortCapture(c: DeviceCapture, deviceId: string): PortCapture {
  return {
    template: c.template ?? null,
    quality: c.quality,
    capturedAt: c.capturedAt,
    deviceId,
  };
}

/** The kit, and what it is still missing. */
export interface Kit extends Device {
  /** Every protocol constant still unconfirmed, for the checklist screen. */
  unconfirmed(): { source: string; name: string; risk: string; where: string }[];
  /** Let go of every device. */
  release(): Promise<void>;
}

/**
 * Build the field kit for this browser.
 *
 * Nothing here prompts, opens a chooser or touches hardware. A kit is cheap to
 * construct and can say what it is able to do; the first capture is what asks.
 */
export function makeKit(options: KitOptions = {}): Kit {
  const id = (options.deviceIdFor ?? deviceId)();

  const missingFor = (where: string) =>
    UNCONFIRMED_CONSTANTS.filter((u) => u.where.includes(where));

  const tagSource = new Attached(
    options.tagProtocol ? new BluetoothLFReader(options.tagProtocol) : null,
    'the transponder reader',
    missingFor('BluetoothLFReader'),
  );

  const irisSource = new Attached(
    options.irisProtocol ? new IriShieldUSB(options.irisProtocol) : null,
    'the iris camera',
    missingFor('IriShield'),
  );

  // The camera needs a frame grabber the application owns, because the
  // application owns the video element the operator is looking at. Without one
  // there is no camera, which is honest rather than a black rectangle.
  const cameraSource = new Attached(
    options.grabber ? new PhoneCamera('muzzle', options.grabber) : null,
    'the camera',
    [],
  );

  const tag: TagReader = {
    state: () => tagSource.state(),
    // The RAW code, as the reader reported it. ports.ts is explicit that
    // normalising is the application's job — and it has to be, because the
    // chain indexes a canonical form and the two must agree. See
    // x/herd/keeper/identifiers.go, which exists because they did not.
    read: () => demand('tag', tagSource.capture()).then((c) => {
      // Narrowed on `authority`, not on `country`: a NationalIdentifier has a
      // country too, so `'country' in seen` admits the wrong shape and the
      // compiler said so.
      const seen = c.identifier;
      if (!seen || !('authority' in seen)) {
        throw new Error('the reader replied with something that is not a transponder code');
      }
      // identity.ts's own formatter, rather than padding by hand here. Two
      // places that format a transponder code are two places that can disagree
      // about it, and the chain indexes one of the two.
      return formatTagCode(seen);
    }),
    watch: (l) => tagSource.watch(l),
  };

  const iris: IrisCamera = {
    state: () => irisSource.state(),
    capture: () => demand('iris', irisSource.capture()).then((c) => toPortCapture(c, id)),
    watch: (l) => irisSource.watch(l),
  };

  /**
   * The tattoo reader is the camera, and there is NO character recognition in
   * this build.
   *
   * Inventing a `text` matters more here than anywhere else in the application:
   * the screen puts the result straight into a field a person then submits. So
   * it takes the photograph, returns an empty reading with a null confidence,
   * and the screen's own rule — never submit, always let a person correct —
   * does the rest. `ports.ts` requires confidence to be nullable for exactly
   * this case.
   */
  const tattoo: TattooReader = {
    state: () => cameraSource.state(),
    read: async () => {
      await demand('tattoo', cameraSource.capture());
      return { text: '', confidence: null };
    },
  };

  /**
   * Position, from the browser rather than from a capture source.
   *
   * Null on refusal, on timeout and on no fix. Never a stale reading and never
   * (0, 0): a position commitment is evidence about where an animal was seen,
   * and an invented one is worse than an absent one.
   */
  const position: Positioner = {
    state: () =>
      typeof navigator !== 'undefined' && 'geolocation' in navigator
        ? { kind: 'ready' }
        : { kind: 'unsupported', detail: 'this browser has no geolocation' },
    fix: (signal) =>
      new Promise((resolve) => {
        if (typeof navigator === 'undefined' || !('geolocation' in navigator)) {
          resolve(null);
          return;
        }
        let settled = false;
        const done = (v: { lat: number; lon: number; accuracyM: number } | null) => {
          if (settled) return;
          settled = true;
          resolve(v);
        };
        signal?.addEventListener('abort', () => done(null), { once: true });
        navigator.geolocation.getCurrentPosition(
          (p) => done({
            lat: p.coords.latitude,
            lon: p.coords.longitude,
            accuracyM: p.coords.accuracy,
          }),
          () => done(null),
          { enableHighAccuracy: true, timeout: 15_000, maximumAge: 0 },
        );
      }),
  };

  return {
    id: () => id,
    tag,
    iris,
    tattoo,
    position,
    unconfirmed: () =>
      [
        ...tagSource.unconfirmed(),
        ...irisSource.unconfirmed(),
        ...cameraSource.unconfirmed(),
      ].map((u) => ({ source: u.where, name: u.name, risk: u.risk, where: u.source })),
    release: async () => {
      await Promise.all([tagSource.release(), irisSource.release(), cameraSource.release()]);
    },
  };
}

const DEVICE_ID_KEY = 'yamale.herd.device';

/**
 * A stable name for this instrument, recorded on every capture.
 *
 * Random and local. Its only job is to be different from the other tablet's, so
 * that two devices which recorded the same animal offline can be told apart —
 * which is what `outbox.ts` needs to stop them collapsing into one row. It is
 * not an identity and authenticates nothing, and it is regenerated if storage
 * is cleared, which is acceptable for that job.
 */
export function deviceId(): string {
  try {
    const held = localStorage.getItem(DEVICE_ID_KEY);
    if (held) return held;
    const made = `dev-${crypto.randomUUID().slice(0, 8)}`;
    localStorage.setItem(DEVICE_ID_KEY, made);
    return made;
  } catch {
    // Private browsing, blocked storage, or no window at all. A per-session id
    // is worse than a persistent one and far better than every device calling
    // itself the same thing.
    return `dev-ephemeral-${Math.random().toString(36).slice(2, 10)}`;
  }
}
