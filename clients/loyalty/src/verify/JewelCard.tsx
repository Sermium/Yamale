import { type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { readJewel, t } from '@yamale/chain';

import { Loading, useLoad } from '../ui.tsx';
import { Hallmark } from './JewelPage.tsx';
import { VerifiedImage } from './Photos.tsx';
import { currentRecordHash, useRecord } from './record.ts';

export function JewelCard({ id, to, extra }: { id: string; to: string; extra?: ReactNode }) {
  const [view] = useLoad(() => readJewel(id), [id]);
  const record = useRecord(view?.ok ? currentRecordHash(view.value.jewel) : null);
  if (!view) return <li className="y-card"><Loading /></li>;
  if (!view.ok) return null;
  const title = record.state === 'ok' ? record.record.title : t('loyalty.jewelN', { id });
  const photo = record.state === 'ok' ? record.record.photos[0] : undefined;
  return (
    <li className="y-card jewelcard">
      <Link className="jewelcard__link" to={to}>
        {photo
          ? <VerifiedImage hash={photo} alt="" className="jewelcard__img" />
          : <span className="jewelcard__img photo--absent" />}
        <span className="jewelcard__text">
          <span className="jewelcard__title">{title}</span>
          <Hallmark code={view.value.jewel.inscription} />
        </span>
        {view.value.jewel.stolen && <span className="y-chip y-chip--bad">{t('loyalty.stolenShort')}</span>}
        {view.value.offer && <span className="y-chip y-chip--brass">{t('loyalty.onOffer')}</span>}
      </Link>
      {extra}
    </li>
  );
}
