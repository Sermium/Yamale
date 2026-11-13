import { t } from '@yamale/chain';

import type { JewelRecord, Stone } from '../verify/record.ts';

export type Draft = Pick<JewelRecord, 'title' | 'metal' | 'weightGrams' | 'stones' | 'origin'>;

export const EMPTY_DRAFT: Draft = {
  title: '', metal: '', weightGrams: '', stones: [],
  origin: { workshop: '', stoneSource: '', labCertificate: '' },
};

function Field({ id, label, value, onChange, max, inputMode }: {
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

export function RecordForm({ value, onChange }: { value: Draft; onChange: (d: Draft) => void }) {
  const set = (patch: Partial<Draft>) => onChange({ ...value, ...patch });
  const setOrigin = (patch: Partial<Draft['origin']>) => set({ origin: { ...value.origin, ...patch } });
  const setStone = (i: number, patch: Partial<Stone>) =>
    set({ stones: value.stones.map((s, j) => (j === i ? { ...s, ...patch } : s)) });
  return (
    <div className="stack">
      <Field id="title" label={t('loyalty.fieldTitle')} value={value.title} onChange={(v) => set({ title: v })} max={120} />
      <Field id="metal" label={t('loyalty.metal')} value={value.metal} onChange={(v) => set({ metal: v })} max={80} />
      <Field id="weight" label={t('loyalty.fieldWeight')} value={value.weightGrams} inputMode="decimal"
             onChange={(v) => set({ weightGrams: v.replace(',', '.') })} max={12} />
      <fieldset className="stack">
        <legend className="y-label">{t('loyalty.stones')}</legend>
        {value.stones.map((s, i) => (
          <div key={i} className="stone-row">
            <input aria-label={t('loyalty.stoneKind')} className="bigfield" value={s.kind} maxLength={60}
                   placeholder={t('loyalty.stoneKind')} onChange={(e) => setStone(i, { kind: e.target.value })} />
            <input aria-label={t('loyalty.stoneCarat')} className="bigfield num" value={s.carat} maxLength={12}
                   inputMode="decimal" placeholder="ct" onChange={(e) => setStone(i, { carat: e.target.value.replace(',', '.') })} />
            <input aria-label={t('loyalty.stoneCount')} className="bigfield num" type="number" min={1} max={9999}
                   value={s.count} onChange={(e) => setStone(i, { count: Math.max(1, Number(e.target.value) || 1) })} />
            <button type="button" className="btn" aria-label={t('loyalty.removeStone')}
                    onClick={() => set({ stones: value.stones.filter((_, j) => j !== i) })}>×</button>
          </div>
        ))}
        <button type="button" className="btn" onClick={() => set({ stones: [...value.stones, { kind: '', carat: '', count: 1 }] })}>
          {t('loyalty.addStone')}
        </button>
      </fieldset>
      <Field id="workshop" label={t('loyalty.workshop')} value={value.origin.workshop} onChange={(v) => setOrigin({ workshop: v })} max={120} />
      <Field id="source" label={t('loyalty.stoneSource')} value={value.origin.stoneSource} onChange={(v) => setOrigin({ stoneSource: v })} max={120} />
      <Field id="lab" label={t('loyalty.labCertificate')} value={value.origin.labCertificate} onChange={(v) => setOrigin({ labCertificate: v })} max={80} />
    </div>
  );
}

/** What must be filled before a record is worth registering. */
export function draftComplete(d: Draft): boolean {
  return d.title.trim() !== '' && d.metal.trim() !== '' && /^[0-9]+(\.[0-9]+)?$/.test(d.weightGrams)
    && d.origin.workshop.trim() !== ''
    && d.stones.every((s) => s.kind.trim() !== '' && /^[0-9]+(\.[0-9]+)?$/.test(s.carat));
}
