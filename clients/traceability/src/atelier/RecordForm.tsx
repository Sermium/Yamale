import { t } from '@yamale/chain';

import type { Stone } from '../verify/record.ts';
import { countFromInput, MAX_STONES, type Draft } from './draft.ts';

export { draftComplete, EMPTY_DRAFT, type Draft } from './draft.ts';

export function Field({ id, label, value, onChange, max, inputMode }: {
  id: string; label: string; value: string; onChange: (v: string) => void; max: number;
  inputMode?: 'decimal' | 'text';
}) {
  return (
    <>
      <label className="y-label" htmlFor={id}>{label}</label>
      <input id={id} className="bigfield" value={value} maxLength={max} inputMode={inputMode}
             onChange={(e) => onChange(e.target.value)} />
    </>
  );
}

/**
 * `unstated`: a catalogue record may leave a stone's carats or count empty when
 * the jeweler's website does not give them; the record then reads "not stated".
 */
export function StonesFieldset({ stones, onChange, unstated = false }: { stones: Stone[]; onChange: (s: Stone[]) => void; unstated?: boolean }) {
  const setStone = (i: number, patch: Partial<Stone>) =>
    onChange(stones.map((s, j) => (j === i ? { ...s, ...patch } : s)));
  return (
    <fieldset className="stack">
      <legend className="y-label">{t('traceability.stones')}</legend>
      {stones.map((s, i) => (
        <div key={i} className="stone-row">
          <input aria-label={t('traceability.stoneKind')} className="bigfield" value={s.kind} maxLength={60}
                 placeholder={t('traceability.stoneKind')} onChange={(e) => setStone(i, { kind: e.target.value })} />
          <input aria-label={t('traceability.stoneCarat')} className="bigfield num" value={s.carat} maxLength={12}
                 inputMode="decimal" placeholder={unstated ? t('traceability.notStatedShort') : 'ct'} onChange={(e) => setStone(i, { carat: e.target.value.replace(',', '.') })} />
          <input aria-label={t('traceability.stoneCount')} className="bigfield num" type="number" min={unstated ? 0 : 1} max={9999}
                 value={unstated && s.count === 0 ? '' : s.count} placeholder={unstated ? t('traceability.notStatedShort') : undefined}
                 onChange={(e) => setStone(i, { count: countFromInput(e.target.value, unstated) })} />
          <button type="button" className="btn" aria-label={t('traceability.removeStone')}
                  onClick={() => onChange(stones.filter((_, j) => j !== i))}>×</button>
        </div>
      ))}
      {unstated && stones.length > 0 && <p className="hint">{t('traceability.stonesUnstatedHint')}</p>}
      <button type="button" className="btn" disabled={stones.length >= MAX_STONES} onClick={() => onChange([...stones, { kind: '', carat: '', count: 1 }])}>
        {t('traceability.addStone')}
      </button>
    </fieldset>
  );
}

export function RecordForm({ value, onChange, unstated = false }: { value: Draft; onChange: (d: Draft) => void; unstated?: boolean }) {
  const set = (patch: Partial<Draft>) => onChange({ ...value, ...patch });
  const setOrigin = (patch: Partial<Draft['origin']>) => set({ origin: { ...value.origin, ...patch } });
  return (
    <div className="stack">
      <Field id="title" label={t('traceability.fieldTitle')} value={value.title} onChange={(v) => set({ title: v })} max={120} />
      <Field id="metal" label={t('traceability.metal')} value={value.metal} onChange={(v) => set({ metal: v })} max={80} />
      <Field id="weight" label={t('traceability.fieldWeight')} value={value.weightGrams} inputMode="decimal"
             onChange={(v) => set({ weightGrams: v.replace(',', '.') })} max={12} />
      <StonesFieldset stones={value.stones} onChange={(stones) => set({ stones })} unstated={unstated} />
      <Field id="workshop" label={t('traceability.workshop')} value={value.origin.workshop} onChange={(v) => setOrigin({ workshop: v })} max={120} />
      <Field id="source" label={t('traceability.stoneSource')} value={value.origin.stoneSource} onChange={(v) => setOrigin({ stoneSource: v })} max={120} />
      <Field id="lab" label={t('traceability.labCertificate')} value={value.origin.labCertificate} onChange={(v) => setOrigin({ labCertificate: v })} max={80} />
    </div>
  );
}

