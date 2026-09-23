/**
 * Registration mode: putting an animal into the register for the first time.
 *
 * # Why this is a separate screen and not a button on Identify
 *
 * Identify answers a question. This makes an assertion, and it is the only
 * screen in the application that can create something the register did not
 * have. Those are different acts and a person should know which one they are
 * performing before they start, not discover it from the outcome.
 *
 * # The shape of the screen is the shape of the refusal
 *
 * The order is: the reference, then the identifiers, then the check, then —
 * only if the check allows — the particulars and the button. The check is not a
 * validation that runs on submit; it is a gate that stands between the operator
 * and the form, because the duplicate this screen exists to prevent is created
 * by somebody acting in good faith who simply could not know.
 *
 * So when the register already holds this animal, there is no form to fill in
 * and no button to press. There is the animal, and a way to open it.
 *
 * # What a biometric may and may not do here
 *
 * A muzzle match produces a QUESTION, never a refusal — `register.ts` argues
 * why, and the screen honours it by showing what matched and making the
 * operator say "it is a different animal" in so many words. An identifier
 * collision produces a refusal with no override anywhere on the screen.
 */

import { useCallback, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { t } from '@yamale/chain';

import type { Enrolled } from './muzzle.ts';
import type { Device } from './ports.ts';
import type { Operator } from './ports.ts';
import {
  captureAdvice,
  check,
  register,
  withParents,
  EMPTY_KNOWN,
  RegisterError,
  type Check,
  type Known,
  type Particulars,
  type Sex,
} from './register.ts';
import { Button, Field, Fields, Note, Panel } from './ui.tsx';

export interface RegisterProps {
  /** What this device holds, for the duplicate check. */
  known?: Known;
  device: Device;
  /** Null until somebody has attached a key. Registration needs one: a record
   *  nobody signed is a record nobody can be asked about. */
  operator: Operator | null;
  herdId: string;
  /** Called with everything the registration produced, for the outbox. */
  onRegistered: (result: {
    reference: string;
    nationalId: string;
    tag: string;
    particulars: Particulars;
    herdId: string;
    pedigree?: { dam: string; sire: string };
  }) => void;
}

export function Register({ known = EMPTY_KNOWN, device, operator, herdId, onRegistered }: RegisterProps) {
  const navigate = useNavigate();

  const [reference, setReference] = useState('');
  const [nationalId, setNationalId] = useState('');
  const [tag, setTag] = useState('');
  const [particulars, setParticulars] = useState<Particulars>({});
  const [dam, setDam] = useState('');
  const [sire, setSire] = useState('');

  const [muzzle, setMuzzle] = useState<Enrolled['descriptor'] | undefined>();
  const [muzzleNote, setMuzzleNote] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const [overridden, setOverridden] = useState(false);

  const claimed = useMemo(() => ({ reference, nationalId, tag }), [reference, nationalId, tag]);

  // Recomputed on every keystroke rather than on submit. The point of the check
  // is to stand between the operator and the form, and a check that only runs
  // at the end has already let them fill it in.
  const checked: Check = useMemo(
    () => check({ claimed, known, muzzle }),
    [claimed, known, muzzle],
  );

  const readTattoo = useCallback(async () => {
    setProblem(null);
    try {
      const reading = await device.tattoo.read();
      if (reading.text === '') {
        setProblem(t('herd.regTattooNothing'));
        return;
      }
      // Into the FIELD, never into the record. The reading is a suggestion and
      // the person is the one who decides — see ocr.ts, which refuses to call
      // anything certain for exactly this reason.
      setNationalId(reading.text);
      setProblem(
        reading.confidence !== null && reading.confidence < 0.62
          ? t('herd.regTattooCheck')
          : null,
      );
    } catch (e) {
      setProblem(e instanceof Error ? e.message : String(e));
    }
  }, [device]);

  const captureMuzzle = useCallback(async () => {
    setBusy(true);
    setProblem(null);
    try {
      const capture = await device.muzzle.capture();
      setMuzzle(capture.descriptor);
      setMuzzleNote(captureAdvice(capture.quality).reason);
    } catch (e) {
      setProblem(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, [device]);

  const submit = useCallback(() => {
    setProblem(null);
    try {
      const made = register(
        { claimed, known, muzzle, herdId, particulars },
        checked,
        overridden,
      );
      const pedigree =
        dam.trim() !== '' || sire.trim() !== ''
          ? withParents('pending', { dam, sire }).pedigree
          : undefined;
      onRegistered({ ...made, pedigree });
      navigate('/');
    } catch (e) {
      setProblem(e instanceof RegisterError ? e.message : String(e));
    }
  }, [claimed, known, muzzle, herdId, particulars, checked, overridden, dam, sire, onRegistered, navigate]);

  /* ---------------------------------------------- nothing to register into */

  // Said before the form rather than at the button. A screen that lets somebody
  // fill in twelve fields and then refuses has wasted their time in a pen.
  if (!operator) {
    return (
      <Panel title={t('herd.regTitle')} sub={t('herd.regSub')}>
        <Note tone="bad" title={t('herd.regNoOperator')}>{t('herd.regNoOperatorWhy')}</Note>
      </Panel>
    );
  }
  if (herdId === '') {
    return (
      <Panel title={t('herd.regTitle')} sub={t('herd.regSub')}>
        <Note tone="bad" title={t('herd.regNoHerd')}>{t('herd.regNoHerdWhy')}</Note>
      </Panel>
    );
  }

  /* ------------------------------------------------------------ blocked */

  if (checked.verdict === 'blocked' && reference.trim() !== '') {
    return (
      <Panel title={t('herd.regTitle')} sub={t('herd.regBlockedSub')}>
        <Note tone="bad" title={t('herd.regBlocked')}>{checked.reason}</Note>
        {checked.animalId ? (
          <Button kind="plain" wide onClick={() => navigate(`/a/${checked.animalId}`)}>
            {t('herd.regOpenExisting')}
          </Button>
        ) : null}
        <Button kind="plain" wide onClick={() => setReference('')}>
          {t('herd.regStartOver')}
        </Button>
      </Panel>
    );
  }

  const ready = checked.verdict === 'ready' || (checked.verdict === 'review' && overridden);

  return (
    <Panel title={t('herd.regTitle')} sub={t('herd.regSub')}>
      <Fields>
        <Field label={t('herd.regReference')}>
          <input
            className="herd-input"
            inputMode="text"
            autoComplete="off"
            value={reference}
            onChange={(e) => setReference(e.target.value)}
            placeholder={t('herd.regReferenceHint')}
          />
        </Field>

        <Field label={t('herd.regNationalId')}>
          <input
            className="herd-input y-mono"
            inputMode="text"
            autoComplete="off"
            value={nationalId}
            onChange={(e) => setNationalId(e.target.value)}
            placeholder="221/682/001029/L"
          />
        </Field>

        <Field label={t('herd.regTag')}>
          <input
            className="herd-input y-mono"
            inputMode="numeric"
            autoComplete="off"
            value={tag}
            onChange={(e) => setTag(e.target.value)}
            placeholder="982000000000003"
          />
        </Field>
      </Fields>

      <div className="herd-actions">
        <Button kind="plain" onClick={() => void readTattoo()}>
          {t('herd.regReadTattoo')}
        </Button>
        <Button kind="plain" onClick={() => void captureMuzzle()} disabled={busy}>
          {busy ? t('herd.regCapturing') : t('herd.regCaptureMuzzle')}
        </Button>
      </div>

      {muzzleNote ? <Note tone="mute">{muzzleNote}</Note> : null}

      {/* The question, never a refusal. What matched is named, and the operator
          has to say it is a different animal in so many words. */}
      {checked.verdict === 'review' ? (
        <Note tone="warn" title={t('herd.regReview')}>
          {checked.reason}
          <ul>
            {checked.animalIds.map((id) => (
              <li key={id}>
                <a href={`#/a/${id}`}>{id}</a>
              </li>
            ))}
          </ul>
          <label className="herd-check">
            <input
              type="checkbox"
              checked={overridden}
              onChange={(e) => setOverridden(e.target.checked)}
            />
            {t('herd.regDifferentAnimal')}
          </label>
        </Note>
      ) : null}

      {ready ? (
        <>
          <Fields>
            <Field label={t('herd.regName')}>
              <input
                className="herd-input"
                value={particulars.name ?? ''}
                onChange={(e) => setParticulars((p) => ({ ...p, name: e.target.value }))}
              />
            </Field>
            <Field label={t('herd.regSex')}>
              <select
                className="herd-input"
                value={particulars.sex ?? 'unknown'}
                onChange={(e) =>
                  setParticulars((p) => ({ ...p, sex: e.target.value as Sex }))
                }
              >
                <option value="unknown">{t('herd.sexUnknown')}</option>
                <option value="female">{t('herd.sexFemale')}</option>
                <option value="male">{t('herd.sexMale')}</option>
              </select>
            </Field>
            <Field label={t('herd.regCoat')}>
              <input
                className="herd-input"
                value={particulars.coat ?? ''}
                onChange={(e) => setParticulars((p) => ({ ...p, coat: e.target.value }))}
              />
            </Field>
          </Fields>

          {/* Pedigree is a SEPARATE assertion and the screen says so: an agent
              who knows the dam and not the sire records what they know, and
              comes back when they find out. */}
          <details className="herd-more">
            <summary>{t('herd.regPedigree')}</summary>
            <p className="herd-hint">{t('herd.regPedigreeHint')}</p>
            <Fields>
              <Field label={t('herd.regDam')}>
                <input className="herd-input y-mono" value={dam}
                       onChange={(e) => setDam(e.target.value)} />
              </Field>
              <Field label={t('herd.regSire')}>
                <input className="herd-input y-mono" value={sire}
                       onChange={(e) => setSire(e.target.value)} />
              </Field>
            </Fields>
          </details>

          <Button kind="primary" wide onClick={submit}>
            {t('herd.regSubmit')}
          </Button>
        </>
      ) : (
        <Note tone="mute">{t('herd.regNeedReference')}</Note>
      )}

      {problem ? <Note tone="bad">{problem}</Note> : null}
    </Panel>
  );
}
