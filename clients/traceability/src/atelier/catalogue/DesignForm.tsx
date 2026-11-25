import { t } from '@yamale/chain';

import type { DesignFields } from './draft.ts';
import { Field, StonesFieldset } from '../register/RecordForm.tsx';

/** A design's title and materials. The weight is optional: each CSV row can carry its own. */
export function DesignForm({ title, fields, onTitle, onFields }: {
  title: string; fields: DesignFields; onTitle: (t: string) => void; onFields: (f: DesignFields) => void;
}) {
  return (
    <div className="stack">
      <Field id="d-title" label={t('traceability.fieldTitle')} value={title} onChange={onTitle} max={120} />
      <Field id="d-metal" label={t('traceability.metal')} value={fields.metal} onChange={(metal) => onFields({ ...fields, metal })} max={80} />
      <Field id="d-weight" label={t('traceability.weightOptional')} value={fields.weightGrams} inputMode="decimal"
             onChange={(v) => onFields({ ...fields, weightGrams: v.replace(',', '.') })} max={12} />
      <StonesFieldset stones={fields.stones} onChange={(stones) => onFields({ ...fields, stones })} unstated />
    </div>
  );
}
