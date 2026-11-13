import { useEffect, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { readJewel, t } from '@yamale/chain';

import { forgetStaleLinks } from '../owner/links.ts';
import { liveClaimKey, offerIsOpen } from '../owner/offers.ts';
import { Loading, useLoad } from '../ui.tsx';
import { Hallmark } from './JewelPage.tsx';
import { VerifiedImage } from './Photos.tsx';
import { useRecord } from './record.ts';

/** `now` is the chain's block time: an offer past it is not on offer, though the chain still holds it. */
export function JewelCard({ id, to, now, extra, onlyJeweler }: { id: string; to: string; now: number; extra?: ReactNode; onlyJeweler?: string }) {
  const [view] = useLoad(() => readJewel(id), [id]);
  useEffect(() => {
    if (view?.ok) forgetStaleLinks(id, liveClaimKey(view.value.offer, now));
  }, [view, id, now]);
  const record = useRecord(view?.ok ? view.value.jewel : null);
  if (!view) return <li className="y-card"><Loading /></li>;
  if (!view.ok) return null;
  // A business account can hold jewels of other jewelers; a jeweler's stock is its own.
  if (onlyJeweler && view.value.jewel.jewelerId !== onlyJeweler) return null;
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
        {offerIsOpen(view.value.offer, now) && <span className="y-chip y-chip--brass">{t('loyalty.onOffer')}</span>}
      </Link>
      {extra}
    </li>
  );
}
