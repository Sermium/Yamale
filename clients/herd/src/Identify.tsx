/**
 * The screen the application opens on: which beast is this.
 *
 * Two ways in, and they are a segmented control rather than four tabs because
 * they are not four questions — they are one question with two kinds of
 * evidence. An IDENTIFIER (a transponder read back, a number typed off a
 * certificate, a tattoo photographed) is exact and the registry either holds it
 * or does not. A BIOMETRIC is a comparison, it produces a ranked list, and the
 * list is the answer.
 *
 * Typing is always available and is never behind a mode. §1.1 settles why: no
 * phone reads an ISO 11784/11785 transponder, so the reader is an external
 * Bluetooth device that may be flat, unpaired or left in the vehicle — and an
 * application whose primary path depends on an accessory is an application
 * nobody carries into a pen. A tattoo in a dusty ear at dusk is sometimes
 * simply unreadable, which is the design's own sentence and the other half of
 * the same argument.
 *
 * # What this screen will not do
 *
 * It will not pick one animal out of two. It will not turn a distance into a
 * percentage. It will not report "no match" when it had nothing to compare
 * against, and it will not report a miss within one herd as a miss in the
 * national register. Each of those is a place where a confident wrong answer
 * about which beast this is would be easy to produce and worse than no answer,
 * and `identify.ts` makes each of them a different value that this file renders
 * differently.
 */
import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { t } from '@yamale/chain';

import { type LocalHerd, resolveIdentifier, useHead, useOnline } from './data.ts';
import { classify, type Animal } from './herd.ts';
import {
  animalsNamed,
  comparableFraction,
  fromCandidates,
  type Candidate,
  nextStep,
  notComparable,
  shownDistance,
  type Identification,
} from './identify.ts';
import { MIN_COMPARABLE_FRACTION, THRESHOLDS, compare, search } from './iris.ts';
import { search as searchMuzzle } from './muzzle.ts';
import { toMuzzleEnrolled } from './store.ts';
import { newEntry, type Entry } from './outbox.ts';
import type { CapabilityState, Device, Operator } from './ports.ts';
import {
  ActionBar,
  Address,
  Button,
  Chip,
  DistanceScale,
  Empty,
  Field,
  Fields,
  Loading,
  NationalId,
  Note,
  Panel,
  ReadAt,
  Silence,
  StatusChip,
  Tag,
} from './ui.tsx';

/**
 * Which way of asking.
 *
 * 'muzzle' is listed before 'biometric' — the iris — because it is the one that
 * works on a bare handset. The iris needs near-infrared and a clip-on module;
 * the muzzle needs an ordinary camera held close, which every phone in a pen
 * already has. An app whose strongest-looking option is greyed out because an
 * accessory is not paired is an app nobody carries.
 */
type Mode = 'identifier' | 'muzzle' | 'biometric';

/** What the screen is currently showing, over and above an identification. */
type Screen =
  | { phase: 'idle' }
  | { phase: 'working'; what: string }
  | { phase: 'answered'; identification: Identification; animal: Animal | null; from: 'chain' | 'replica' }
  | { phase: 'unrecognised'; raw: string }
  | { phase: 'collision'; animalIds: string[] };

export function Identify({ local, device, operator, queue }: {
  local: LocalHerd;
  device: Device;
  operator: Operator | null;
  queue: (entries: readonly Entry[]) => Promise<void>;
}) {
  const head = useHead();
  const connected = useOnline();
  const [mode, setMode] = useState<Mode>('identifier');
  const [typed, setTyped] = useState('');
  const [screen, setScreen] = useState<Screen>({ phase: 'idle' });

  const entry = classify(typed);

  const lookUp = useCallback(async () => {
    setScreen({ phase: 'working', what: t('herd.lookingUp') });
    const got = await resolveIdentifier(typed, local);
    switch (got.kind) {
      case 'resolved':
        setScreen({
          phase: 'answered',
          identification: {
            kind: 'identified', by: got.value.by,
            value: entry.value, animalId: got.value.animal.id,
          },
          animal: got.value.animal,
          from: got.value.from,
        });
        return;
      case 'not-found':
        setScreen({
          phase: 'answered',
          identification: { kind: 'not-found', by: got.by, value: got.value, scope: got.scope },
          animal: null, from: 'chain',
        });
        return;
      case 'unrecognised':
        setScreen({ phase: 'unrecognised', raw: got.raw });
        return;
      case 'collision':
        setScreen({ phase: 'collision', animalIds: got.animalIds });
        return;
      case 'silent':
        setScreen({
          phase: 'answered',
          identification: {
            kind: 'unasked',
            reason: got.reason === 'refused' ? 'unreachable' : got.reason,
            detail: got.detail,
          },
          animal: null, from: 'chain',
        });
    }
  }, [typed, local, entry.value]);

  /**
   * Capture, compare, and record the capture whatever the comparison said.
   *
   * The enrolment-worthy fact is that somebody stood in front of an animal with
   * an instrument at a time and a place. That is true of a capture that matched
   * nothing, and it is MORE interesting for one that produced an ambiguous
   * result — so the queue entry is written before the classification is
   * rendered and is not conditional on it.
   */
  /**
   * Photograph a muzzle and search the replica with it.
   *
   * Deliberately its own function rather than a branch inside the iris one.
   * The two share a shape and nothing else: different capture path, different
   * matcher, different reasons for being unable to compare — and a single
   * function taking a mode flag would have to keep explaining which half of
   * itself it was in.
   */
  const captureMuzzleAndSearch = useCallback(async () => {
    setScreen({ phase: 'working', what: t('herd.capturing') });

    let capture;
    try {
      capture = await device.muzzle.capture();
    } catch (err) {
      setScreen({
        phase: 'answered',
        identification: {
          kind: 'no-capture',
          refusal: err instanceof Error ? err.message : String(err),
        },
        animal: null, from: 'chain',
      });
      return;
    }

    const enrolled = local.enrolments
      .map(toMuzzleEnrolled)
      .filter((e): e is NonNullable<typeof e> => e !== null);

    if (enrolled.length === 0) {
      // NOT a statement about the animal. Nobody has enrolled a muzzle yet,
      // which is the ordinary state of a register that has been identifying by
      // iris, and saying "no match" here would be a finding this has not made.
      setScreen({
        phase: 'answered',
        identification: {
          kind: 'nothing-enrolled', scope: 'herd', animalsInHerd: local.animals.length,
        },
        animal: null, from: 'replica',
      });
      return;
    }

    const candidates = searchMuzzle(capture.descriptor, enrolled);
    const withTemplate = new Set(enrolled.map((e) => e.animalId)).size;
    const identification = fromCandidates(candidates, {
      scope: 'herd',
      marker: 'muzzle',
      compared: enrolled.length,
      withoutTemplate: Math.max(0, local.animals.length - withTemplate),
    });

    if (operator && local.meta) {
      const named = animalsNamed(identification);
      await queue([
        newEntry({
          kind: 'capture',
          animalId: named.length === 1 ? named[0]! : null,
          herdId: local.meta.herdId,
          observedAt: capture.capturedAt,
          deviceId: capture.deviceId,
          agent: operator.address,
          position: null,
          detail: {
            modality: 'muzzle',
            quality: capture.quality,
            outcome: identification.kind,
          },
        }),
      ]);
    }

    setScreen({
      phase: 'answered',
      identification,
      animal: null,
      from: 'replica',
    });
  }, [device, local, operator, queue]);

  const captureAndSearch = useCallback(async () => {
    setScreen({ phase: 'working', what: t('herd.capturing') });

    let capture;
    try {
      capture = await device.iris.capture();
    } catch (err) {
      setScreen({
        phase: 'answered',
        identification: {
          kind: 'no-capture',
          refusal: err instanceof Error ? err.message : String(err),
        },
        animal: null, from: 'chain',
      });
      return;
    }

    if (!capture.template) {
      setScreen({
        phase: 'answered',
        identification: { kind: 'no-capture', refusal: capture.refusal ?? 'no template' },
        animal: null, from: 'chain',
      });
      return;
    }

    const probe = capture.template;
    const enrolled = local.scope.enrolled;

    if (enrolled.length === 0) {
      setScreen({
        phase: 'answered',
        identification: {
          kind: 'nothing-enrolled', scope: 'herd', animalsInHerd: local.animals.length,
        },
        animal: null, from: 'replica',
      });
      return;
    }

    // Was ANYTHING comparable? `search()` drops a null distance, so an empty
    // result would otherwise read as "compared against nine and matched none"
    // when the truth is "none of the nine could be compared at all".
    const comparable = enrolled.some((e) => compare(probe, e.template).distance !== null);
    if (!comparable) {
      setScreen({
        phase: 'answered', identification: notComparable('herd'), animal: null, from: 'replica',
      });
      return;
    }

    const candidates = search(probe, enrolled);
    const identification = fromCandidates(candidates, {
      scope: 'herd',
      marker: 'iris',
      compared: enrolled.length,
      withoutTemplate: local.scope.withoutTemplate,
    });

    if (operator && local.meta) {
      const named = animalsNamed(identification);
      await queue([
        newEntry({
          kind: 'capture',
          // An ambiguous result names several animals, so the capture is filed
          // against none of them. Filing it against the best would be the very
          // collapse this screen exists to refuse, done quietly in a database.
          animalId: named.length === 1 ? named[0] : null,
          herdId: local.meta.herdId,
          observedAt: capture.capturedAt,
          deviceId: capture.deviceId,
          agent: operator.address,
          detail: {
            modality: 'iris',
            quality: capture.quality,
            outcome: identification.kind,
            candidates: candidates.map((c) => ({ animalId: c.animalId, distance: c.distance })),
          },
        }),
      ]);
    }

    setScreen({ phase: 'answered', identification, animal: null, from: 'replica' });
  }, [device, local, operator, queue]);

  return (
    <>
      <div className="y-screen">
        <h1 className="y-screen__title">{t('herd.identifyTitle')}</h1>

        <div className="modes" role="tablist" aria-label={t('herd.identifyTitle')}>
          <button type="button" role="tab" aria-selected={mode === 'identifier'}
                  className={`mode${mode === 'identifier' ? ' mode--on' : ''}`}
                  onClick={() => { setMode('identifier'); setScreen({ phase: 'idle' }); }}>
            {t('herd.modeIdentifier')}
          </button>
          {/* The muzzle sits before the iris because it is the one that
              works on a bare handset. */}
          <button type="button" role="tab" aria-selected={mode === 'muzzle'}
                  className={`mode${mode === 'muzzle' ? ' mode--on' : ''}`}
                  onClick={() => { setMode('muzzle'); setScreen({ phase: 'idle' }); }}>
            {t('herd.modeMuzzle')}
          </button>
          <button type="button" role="tab" aria-selected={mode === 'biometric'}
                  className={`mode${mode === 'biometric' ? ' mode--on' : ''}`}
                  onClick={() => { setMode('biometric'); setScreen({ phase: 'idle' }); }}>
            {t('herd.modeBiometric')}
          </button>
        </div>

        {mode === 'identifier' ? (
          <IdentifierPane typed={typed} setTyped={setTyped} device={device} />
        ) : mode === 'muzzle' ? (
          <MuzzlePane local={local} device={device} />
        ) : (
          <BiometricPane local={local} device={device} />
        )}

        <Result screen={screen} local={local} head={head} connected={connected} />
      </div>

      <ActionBar>
        {mode === 'identifier' ? (
          <Button kind="primary" wide onClick={lookUp}
                  disabled={entry.kind === 'empty' || screen.phase === 'working'}>
            {t('herd.lookUp')}
          </Button>
        ) : mode === 'muzzle' ? (
          <Button kind="primary" wide onClick={captureMuzzleAndSearch}
                  disabled={device.muzzle.state().kind !== 'ready' || screen.phase === 'working'}>
            {t('herd.capture')}
          </Button>
        ) : (
          <Button kind="primary" wide onClick={captureAndSearch}
                  disabled={device.iris.state().kind !== 'ready' || screen.phase === 'working'}>
            {t('herd.capture')}
          </Button>
        )}
      </ActionBar>
    </>
  );
}

/* --------------------------------------------------------- the two panes */

/**
 * A field, and two things that can fill it.
 *
 * The reader and the camera write INTO the field rather than submitting for the
 * operator. A stick reader occasionally reports the tag of the animal in the
 * next pen, and a tattoo OCR at dusk is wrong often enough that presenting its
 * output as an identifier would be a lie with a camera behind it. The person
 * sees what the instrument produced and presses the button.
 */
function IdentifierPane({ typed, setTyped, device }: {
  typed: string;
  setTyped: (v: string) => void;
  device: Device;
}) {
  const [busy, setBusy] = useState<'tag' | 'tattoo' | null>(null);
  const [failed, setFailed] = useState<string | null>(null);
  const entry = classify(typed);

  const run = async (which: 'tag' | 'tattoo') => {
    setBusy(which); setFailed(null);
    try {
      const value = which === 'tag'
        ? await device.tag.read()
        : (await device.tattoo.read()).text;
      setTyped(value);
    } catch (err) {
      setFailed(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  return (
    <>
      <label className="bigfield">
        <span className="y-label">{t('herd.identifierLabel')}</span>
        <input
          className="bigfield__input"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          placeholder="221/682/021029/L"
          // `text` and not `numeric`: a national identifier ends in a breed
          // letter, and a numeric keypad would hide it. The tag path is served
          // by the reader and by the digits on a text keyboard.
          inputMode="text"
          autoCapitalize="characters"
          autoComplete="off"
          spellCheck={false}
          aria-describedby="entrykind"
        />
        <span id="entrykind" className="bigfield__kind">
          {entry.kind === 'empty' ? t('herd.entryEmpty')
            : entry.kind === 'tag' ? t('herd.entryTag')
            : entry.kind === 'national-id' ? t('herd.entryNationalId')
            : t('herd.entryUnrecognised')}
        </span>
      </label>

      <div className="assists">
        <Assist name={t('herd.readTag')} state={device.tag.state()} busy={busy === 'tag'}
                onClick={() => run('tag')} why={t('herd.readTagWhy')} />
        <Assist name={t('herd.scanTattoo')} state={device.tattoo.state()} busy={busy === 'tattoo'}
                onClick={() => run('tattoo')} why={t('herd.scanTattooWhy')} />
      </div>

      {failed ? <Note tone="warn" title={t('herd.readFailed')}>{failed}</Note> : null}
    </>
  );
}

/**
 * The biometric pane, which mostly explains what it cannot do.
 *
 * Deliberately. §5 scopes an offline search to one herd; §2 keeps every
 * template off the chain; §7 puts the national service at step four of five. So
 * on a device today this compares against the templates enrolled on this
 * device, and the two numbers at the top of this pane — how many animals are
 * covered and how many are not — are the only honest framing for whatever comes
 * back.
 */
/**
 * What the operator sees before photographing a muzzle.
 *
 * Says how many muzzles are on this device, because the honest failure here is
 * not "no match" — it is that nobody has enrolled a muzzle yet, and a screen
 * that did not say so would let an agent conclude a registered animal was
 * unregistered.
 */
function MuzzlePane({ local, device }: { local: LocalHerd; device: Device }) {
  const state = device.muzzle.state();
  const enrolled = local.enrolments.filter((e) => e.modality === 'muzzle').length;

  return (
    <Panel>
      <p className="y-screen__lede">{t('herd.muzzleLede')}</p>
      {state.kind !== 'ready' ? (
        <Note tone="warn" title={t('herd.muzzleUnavailable')}>
          {state.detail ?? t('herd.muzzleUnavailableWhy')}
        </Note>
      ) : enrolled === 0 ? (
        <Note tone="warn" title={t('herd.muzzleNoneEnrolled')}>
          {t('herd.muzzleNoneEnrolledWhy')}
        </Note>
      ) : (
        <Fields>
          <Field label={t('herd.muzzlesOnDevice')}>
            <span className="num">{enrolled}</span>
          </Field>
        </Fields>
      )}
    </Panel>
  );
}

function BiometricPane({ local, device }: { local: LocalHerd; device: Device }) {
  const state = device.iris.state();
  const { withTemplate, withoutTemplate } = local.scope;

  return (
    <>
      <Note tone={withTemplate === 0 ? 'warn' : 'mute'} title={t('herd.scopeTitle')}>
        {t('herd.scopeBody', { covered: withTemplate, uncovered: withoutTemplate })}
      </Note>

      <div className="assists assists--one">
        <Assist name={t('herd.irisCamera')} state={state} busy={false}
                why={t('herd.irisCameraWhy')} />
      </div>

      {state.kind !== 'ready' ? (
        <Note tone="mute" title={t('herd.noIrisTitle')}>{t('herd.noIrisBody')}</Note>
      ) : null}
    </>
  );
}

/**
 * One instrument, and what it can do right now.
 *
 * The four capability states get four sentences because the operator's next
 * move differs: pair it, grant the permission, plug it in, or give up and type.
 * "Unavailable" for all four would send somebody looking for a setting that
 * does not exist.
 */
function Assist({ name, state, busy, onClick, why }: {
  name: string;
  state: CapabilityState;
  busy: boolean;
  onClick?: () => void;
  why: string;
}) {
  const ready = state.kind === 'ready';
  const tone = ready ? 'ok' : state.kind === 'denied' ? 'bad' : 'mute';
  return (
    <button type="button" className={`assist${ready ? '' : ' assist--off'}`}
            onClick={onClick} disabled={!ready || busy || !onClick}>
      <span className="assist__name">{name}</span>
      <Chip tone={tone}>{busy ? t('herd.reading') : t(`herd.cap.${state.kind}`)}</Chip>
      {/* `why` is the catalogue's sentence and `detail` is the device layer's.
          The operator reads the first: `detail` names GATT characteristics and
          USB endpoints in English, which is a procurement question rather than
          something a person in a pen can act on, and it rendered untranslated
          inside an Arabic interface. The technical list is still reachable — it
          is what `kit.unconfirmed()` returns, for whoever specifies the
          hardware. */}
      <span className="assist__why">{why}</span>
    </button>
  );
}

/* ---------------------------------------------------------- the answer */

function Result({ screen, local, head, connected }: {
  screen: Screen;
  local: LocalHerd;
  head: ReturnType<typeof useHead>;
  connected: boolean;
}) {
  if (screen.phase === 'idle') {
    return (
      <Panel>
        <p className="lede">{t('herd.identifyLede')}</p>
        {!connected ? <Note tone="mute" title={t('herd.offlineTitle')}>{t('herd.offlineBody')}</Note> : null}
      </Panel>
    );
  }

  if (screen.phase === 'working') return <Loading what={screen.what} />;

  if (screen.phase === 'unrecognised') {
    return (
      <Note tone="warn" title={t('herd.unrecognisedTitle')}>
        <p>{t('herd.unrecognisedBody')}</p>
        <ul className="forms">
          <li><NationalId value="221/682/021029/L" /> — {t('herd.formNationalId')}</li>
          <li><Tag value="982000000021029" /> — {t('herd.formTag')}</li>
        </ul>
      </Note>
    );
  }

  if (screen.phase === 'collision') {
    return (
      <Note tone="bad" title={t('herd.collisionTitle')}>
        <p>{t('herd.collisionBody')}</p>
        <ul className="forms">
          {screen.animalIds.map((id) => <li key={id}>{t('herd.animalN', { id })}</li>)}
        </ul>
      </Note>
    );
  }

  return (
    <Answer identification={screen.identification} animal={screen.animal}
            from={screen.from} local={local} head={head} />
  );
}

/**
 * An identification, rendered so that its kind is the first thing read.
 *
 * Each branch below is a different sentence, a different tone and a different
 * next control, and none of them shares a layout with another by accident. An
 * ambiguous result in particular is NOT a list with the best one highlighted —
 * it is a flat list with no primary button anywhere on it.
 */
function Answer({ identification: id, animal, from, local, head }: {
  identification: Identification;
  animal: Animal | null;
  from: 'chain' | 'replica';
  local: LocalHerd;
  head: ReturnType<typeof useHead>;
}) {
  const step = nextStep(id);

  switch (id.kind) {
    case 'identified':
      return (
        <Panel>
          <Note tone="ok" title={t('herd.foundTitle')}>
            {id.by === 'tag' ? t('herd.foundByTag') : t('herd.foundByNationalId')}
          </Note>
          {animal ? <Found animal={animal} from={from} /> : null}
          <Note tone="warn" title={t('herd.secondMarkerTitle')}>
            {t('herd.secondMarkerBody')}
          </Note>
          <ReadAt head={head} from={from} replicaHeight={local.meta?.height ?? null} />
        </Panel>
      );

    case 'not-found':
      return (
        <Panel>
          <Empty title={t('herd.notFoundTitle')}>
            <p>
              {id.scope === 'national' ? t('herd.notFoundNational') : t('herd.notFoundHerd')}
            </p>
            <p className="mono-line">
              {id.by === 'tag' ? <Tag value={id.value} /> : <NationalId value={id.value} />}
            </p>
            <p>{t('herd.notFoundNext')}</p>
          </Empty>
          <ReadAt head={head} />
        </Panel>
      );

    case 'ambiguous':
      return (
        <Panel>
          {/* No primary action anywhere on this branch. That is the design. */}
          <Note tone="bad" title={t('herd.ambiguousTitle')}>
            {t('herd.ambiguousBody', { n: animalsNamed(id).length })}
          </Note>
          <CandidateList candidates={id.candidates} local={local} flat />
          <Note tone="warn" title={t('herd.ambiguousNextTitle')}>
            {t('herd.ambiguousNextBody')}
          </Note>
          <ReadAt head={head} from="replica" replicaHeight={local.meta?.height ?? null} />
        </Panel>
      );

    case 'biometric-match': {
      const shown = local.index?.byId.get(id.best.animalId) ?? null;
      return (
        <Panel>
          <Note tone="brass" title={t('herd.matchTitle')}>{t('herd.matchBody')}</Note>
          {shown ? <Found animal={shown} from="replica" /> : null}
          <CandidateList
            candidates={[id.best, ...id.supporting, ...(id.runnerUp ? [id.runnerUp] : [])]}
            local={local}
          />
          <Note tone="warn" title={t('herd.secondMarkerTitle')}>
            {t('herd.secondMarkerBody')}
          </Note>
          {id.runnerUp ? (
            <Note tone="mute" title={t('herd.runnerUpTitle')}>
              {t('herd.runnerUpBody', {
                id: id.runnerUp.animalId,
                d: shownDistance(id.runnerUp) ?? '',
              })}
            </Note>
          ) : null}
          <ReadAt head={head} from="replica" replicaHeight={local.meta?.height ?? null} />
        </Panel>
      );
    }

    case 'inconclusive':
      return (
        <Panel>
          <Note tone="warn" title={t('herd.inconclusiveTitle')}>
            {id.why === 'not-comparable'
              ? t('herd.notComparableBody', { pct: Math.round(MIN_COMPARABLE_FRACTION * 100) })
              : t('herd.nearThresholdBody', { m: THRESHOLDS.match.toFixed(2), i: THRESHOLDS.inconclusive.toFixed(2) })}
          </Note>
          {id.candidates.length ? (
            <>
              <p className="lede">{t('herd.nearMisses')}</p>
              <CandidateList candidates={id.candidates} local={local} flat />
            </>
          ) : null}
          <Note tone="mute" title={t('herd.captureAgainTitle')}>{t('herd.captureAgainBody')}</Note>
        </Panel>
      );

    case 'no-candidate':
      return (
        <Panel>
          <Empty title={t('herd.noCandidateTitle')}>
            <p>{t('herd.noCandidateBody', { compared: id.compared })}</p>
            {id.withoutTemplate > 0 ? (
              <Note tone="warn" title={t('herd.notComparedTitle')}>
                {t('herd.notComparedBody', { n: id.withoutTemplate })}
              </Note>
            ) : null}
            <p>{t('herd.herdScopeOnly')}</p>
          </Empty>
        </Panel>
      );

    case 'nothing-enrolled':
      return (
        <Panel>
          <Empty title={t('herd.nothingEnrolledTitle')}>
            <p>{t('herd.nothingEnrolledBody', { n: id.animalsInHerd })}</p>
            <p>{t('herd.nothingEnrolledWhy')}</p>
          </Empty>
        </Panel>
      );

    case 'no-capture':
      return (
        <Note tone="warn" title={t('herd.noCaptureTitle')}>
          {t(`herd.refusal.${id.refusal}`) === `herd.refusal.${id.refusal}`
            ? id.refusal
            : t(`herd.refusal.${id.refusal}`)}
        </Note>
      );

    case 'unasked':
      return <Silence reason={id.reason} detail={id.detail} />;
  }

  // Exhaustive above; this keeps the compiler honest if a member is added.
  return <Loading what={step.step} />;
}

/**
 * The record, once there is one.
 *
 * Kept to the six facts that decide what happens next in a pen, with everything
 * else one tap away on the animal's own screen. A status that is not `active`
 * is the loudest thing here, because a frozen or disputed animal cannot begin a
 * handover and an operator about to start one needs that before the name.
 */
function Found({ animal, from }: { animal: Animal; from: 'chain' | 'replica' }) {
  return (
    <div className="found">
      <div className="found__head">
        <NationalId value={animal.nationalId} className="found__id" />
        <StatusChip status={animal.status} />
      </div>
      <p className="found__name">{animal.name || t('herd.unnamed')}</p>
      <Fields>
        <Field label={t('herd.tag')}><Tag value={animal.tag} /></Field>
        <Field label={t('herd.level')}>{t(`herd.level.${animal.level}`)}</Field>
        <Field label={t('herd.sex')}>{t(`herd.sex.${animal.sex}`)}</Field>
        <Field label={t('herd.holder')}><Address value={animal.holder} /></Field>
      </Fields>
      {from === 'replica' ? (
        <p className="found__from">{t('herd.fromReplica')}</p>
      ) : null}
      <Link className="btn btn--plain btn--wide" to={`/a/${animal.id}`}>
        {t('herd.openRecord')}
      </Link>
    </div>
  );
}

/**
 * The candidates, with their distances and nothing invented.
 *
 * `flat` removes every visual ordering cue — no rank number, no emphasis on the
 * first row — and is used for `ambiguous` and for near misses. A ranked list
 * with a highlighted top row IS an answer, whatever the heading above it says.
 */
function CandidateList({ candidates, local, flat = false }: {
  candidates: readonly Candidate[];
  local: LocalHerd;
  flat?: boolean;
}) {
  return (
    <ul className={`cands${flat ? ' cands--flat' : ''}`}>
      {candidates.map((c) => {
        const animal = local.index?.byId.get(c.animalId) ?? null;
        const frac = comparableFraction(c, 2048);
        return (
          <li key={`${c.animalId}:${c.enrolmentId}`} className="cand">
            <div className="cand__head">
              <Link className="cand__id" to={`/a/${c.animalId}`}>
                {animal
                  ? <NationalId value={animal.nationalId} />
                  : t('herd.animalN', { id: c.animalId })}
              </Link>
              <Chip tone={c.verdict === 'match' ? 'brass' : 'warn'}>
                {t(`herd.verdict.${c.verdict}`)}
              </Chip>
            </div>
            {animal ? <p className="cand__name">{animal.name || t('herd.unnamed')}</p> : null}
            <div className="cand__nums">
              {/* A distance, to two places, labelled as a distance. Never a
                  percentage: a normalised Hamming distance is not a
                  probability, and 0.30 is not "70% confident". */}
              <span className="cand__d">
                {t('herd.distance')} <strong className="num">{shownDistance(c)}</strong>
              </span>
              <span className="cand__bits">
                {frac === null ? null : t('herd.comparedBits', { pct: Math.round(frac * 100) })}
              </span>
            </div>
            <DistanceScale distance={c.distance} match={THRESHOLDS.match}
                           inconclusive={THRESHOLDS.inconclusive} />
          </li>
        );
      })}
      <li className="cands__caveat">{t('herd.thresholdsCaveat')}</li>
    </ul>
  );
}
