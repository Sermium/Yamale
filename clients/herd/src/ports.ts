/**
 * What this application needs from the device, and from whoever is holding it.
 *
 * ────────────────────────────────────────────────────────────────────────────
 * EVERYTHING IN THIS FILE IS ANOTHER AGENT'S WORK.
 *
 * `device.ts` and `identity.ts` are being written separately. This file is the
 * shape this half of the application compiles against until they land, written
 * down in one place so that reconciling the two is a single file's worth of
 * edits rather than a search across fifteen screens.
 *
 * Nothing here is implemented here. `NO_DEVICE` and `NO_IDENTITY` below are
 * absences, not stubs: every one of their members reports that the capability
 * is missing, and every screen is written to render that absence honestly. A
 * stub that returned a plausible tag read would be the single worst thing this
 * file could contain — the whole application exists to avoid asserting an
 * identity it does not have, and a fake reader would assert one before the
 * hardware ever arrived.
 * ────────────────────────────────────────────────────────────────────────────
 *
 * # Why these capabilities and not others
 *
 * §1.1 of the design settles the hardware: no phone reads an ISO 11784/11785
 * transponder, because that standard is 134.2 kHz and every phone radio is
 * 13.56 MHz. So the tag reader is an external Bluetooth device that may or may
 * not be paired, and `TagReader.state` has to carry "no reader paired" as a
 * first-class answer rather than as a failed read.
 *
 * §3 settles the biometrics: a cattle iris is dark brown and needs
 * near-infrared, which means a clip-on module. So the iris camera is also an
 * accessory that may be absent, and `IrisCamera.state` says so the same way.
 *
 * Both of those are why the Identify screen leads with the tattoo and the
 * keyboard: those two paths work on a bare phone, today, and an app whose
 * primary action is greyed out because an accessory is not paired is an app
 * nobody carries into a pen.
 */

import type { IrisTemplate } from './iris.ts';
import type { Descriptor as MuzzleDescriptor } from './muzzle.ts';

/* ------------------------------------------------------------- the device */

/**
 * Whether a capability can be used right now, and why not when it cannot.
 *
 * Four states rather than a boolean, because a screen says something different
 * about each one and "unavailable" would collapse a missing accessory into a
 * denied permission. The operator's next move differs: pair a reader, grant a
 * permission, plug the thing in, or give up and type.
 */
export type CapabilityState =
  /** The hardware is present and usable. */
  | { kind: 'ready' }
  /** This build, browser or device has no such thing. Nothing to fix here. */
  | { kind: 'unsupported'; detail?: string }
  /** Present, but nothing is paired or connected — a Bluetooth stick reader. */
  | { kind: 'disconnected'; detail?: string }
  /** Present and refused: camera permission, Bluetooth permission. */
  | { kind: 'denied'; detail?: string };

/** An ISO 11784/11785 stick or panel reader, over Bluetooth. See §1.1. */
export interface TagReader {
  state(): CapabilityState;
  /**
   * Read one transponder.
   *
   * Resolves with the raw code as the reader reported it — normalisation is
   * this application's job, in `herd.ts`, because the reader does not know
   * what a national numbering scheme looks like. Rejects when the read failed;
   * it never resolves with a guess.
   */
  read(signal?: AbortSignal): Promise<string>;
  /** Subscribe to connection changes, so a screen can stop lying about it. */
  watch(listener: (state: CapabilityState) => void): () => void;
}

/** What a capture attempt produced, before anything is claimed about it. */
export interface Capture {
  /** The finished template, when the capture was good enough to make one. */
  template: IrisTemplate | null;
  /**
   * The quality the capture achieved, 0 to 1, as the capture device scored it.
   *
   * Null when the device does not score — and null is rendered as "not scored",
   * never as zero and never as a bar at the left-hand end. §2 records this on
   * the enrolment precisely because a bad capture is the common failure and has
   * to be attributable.
   */
  quality: number | null;
  /** Device clock, milliseconds. The capture's own time, not the sync's. */
  capturedAt: number;
  /** Which instrument took it, for the enrolment record §2 wants. */
  deviceId: string;
  /** Why no template, when there is none. */
  refusal?: 'too-blurred' | 'too-dark' | 'eye-not-found' | 'moved' | 'cancelled';
}

/** A near-infrared iris module — the clip-on of §3. */
export interface IrisCamera {
  state(): CapabilityState;
  capture(signal?: AbortSignal): Promise<Capture>;
  watch(listener: (state: CapabilityState) => void): () => void;
}

/**
 * The muzzle, which is the one cattle biometric with a literature behind it.
 *
 * Separate from `IrisCamera` because they are different instruments answering
 * different questions. An iris needs near-infrared and an accessory; a muzzle
 * needs an ordinary camera held close, which every phone in the field already
 * has — so this capability is `ready` on a bare handset where the iris is not,
 * and the screens should prefer it for exactly that reason.
 *
 * Returns a DESCRIPTOR rather than an image. The comparison is done on the
 * device against the offline replica, so what a screen needs is the thing that
 * can be compared; the photograph itself goes to the outbox as a capture.
 */
export interface MuzzleCamera {
  state(): CapabilityState;
  /**
   * Photograph the muzzle and describe it.
   *
   * `quality` is the capture's own score and is nullable for the same reason it
   * is everywhere else: an instrument that does not score has not said the
   * capture was bad.
   */
  capture(signal?: AbortSignal): Promise<{
    descriptor: MuzzleDescriptor;
    quality: number | null;
    capturedAt: number;
    deviceId: string;
  }>;
  watch(listener: (state: CapabilityState) => void): () => void;
}

/** Optical character recognition over the camera, for a tattooed ear. */
export interface TattooReader {
  state(): CapabilityState;
  /**
   * Read a tattoo.
   *
   * Returns what it thinks it saw AND its own confidence, because OCR of a
   * dusty ear at dusk is wrong often enough that presenting its output as an
   * identifier would be a lie with a camera behind it. The screen puts the
   * result in the text field for a person to correct; it never submits it.
   */
  read(signal?: AbortSignal): Promise<{ text: string; confidence: number | null }>;
}

/** Where the device is, for the position commitment §2 puts on an enrolment. */
export interface Positioner {
  state(): CapabilityState;
  /** Null when there is no fix — never a stale one, and never (0, 0). */
  fix(signal?: AbortSignal): Promise<{ lat: number; lon: number; accuracyM: number } | null>;
}

/** Everything the field kit might have. Any of it may be missing. */
export interface Device {
  /** A stable identifier for this instrument, recorded on every capture. */
  id(): string;
  tag: TagReader;
  iris: IrisCamera;
  muzzle: MuzzleCamera;
  tattoo: TattooReader;
  position: Positioner;
}

/* ----------------------------------------------------------- the operator */

/**
 * Who is holding the device.
 *
 * An agent, in `x/herd`'s sense: a person with a plain key, accredited by an
 * office. Not an `Authority` — the module's own comment explains at length why
 * a field veterinarian cannot be an M-of-N group with a governance proposal
 * behind them.
 */
export interface Operator {
  /** The bech32 address that signs. Empty when nobody is signed in. */
  address: string;
  /** What they call themselves on this device. The chain's `Agent.name` wins. */
  label: string;
}

/** Signing, which this application does not implement and must not. */
export interface Identity {
  operator(): Operator | null;
  /**
   * Sign and broadcast one message.
   *
   * Takes the `{ typeUrl, value }` pair the chain registry understands and
   * returns the transaction hash. Rejects on refusal, on a failed broadcast,
   * and on an absent signer; the outbox in `outbox.ts` keeps the entry and
   * records why, so nothing is ever silently dropped.
   */
  submit(messages: { typeUrl: string; value: unknown }[], memo?: string): Promise<string>;
  watch(listener: () => void): () => void;
}

/* ---------------------------------------------------------- the absences */

const UNSUPPORTED: CapabilityState = { kind: 'unsupported', detail: 'device.ts not wired' };
const noWatch = () => () => {};
const refuse = (what: string) => () => Promise.reject(new Error(`${what}: no device attached`));

/**
 * A field kit with nothing in it.
 *
 * Deliberately not a simulator. A tag reader that invented a plausible code
 * would put a fabricated identity on a screen that exists to refuse exactly
 * that, and the fabrication would survive right up until somebody trusted it.
 */
export const NO_DEVICE: Device = {
  id: () => 'unattached',
  tag: { state: () => UNSUPPORTED, read: refuse('tag'), watch: noWatch },
  iris: { state: () => UNSUPPORTED, capture: refuse('iris'), watch: noWatch },
  muzzle: { state: () => UNSUPPORTED, capture: refuse('muzzle'), watch: noWatch },
  tattoo: { state: () => UNSUPPORTED, read: refuse('tattoo') },
  position: { state: () => UNSUPPORTED, fix: () => Promise.resolve(null) },
};

/** Nobody signed in, and nothing that can sign. */
export const NO_IDENTITY: Identity = {
  operator: () => null,
  submit: () => Promise.reject(new Error('no signer attached')),
  watch: noWatch,
};
