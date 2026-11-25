/**
 * A list of held jewels, one row per design: the name, how many, and a menu of
 * serial numbers. "My jewels" and the Atelier's stock both show it.
 */

import { Link, useNavigate } from 'react-router-dom';
import { jewelsHeldBy, readJewel, t, type Outcome } from '@yamale/chain';

import { fetchVerified } from '../api.ts';
import { Hallmark } from '../verify/JewelPage.tsx';
import { VerifiedImage } from '../verify/Photos.tsx';
import { currentRecordHash, parseRecord, recordMatches } from '../verify/record.ts';
import { groupHoldings, mapLimit, type Held, type HeldGroup } from './holdings.ts';
import { forgetStaleLinks } from './links.ts';
import { liveClaimKey, offerIsOpen } from './offers.ts';

/** One jewel as the list needs it; null when the chain no longer has it. Its record is checked as every page checks it. */
async function loadHeld(id: string, now: number): Promise<Held | null> {
  const view = await readJewel(id);
  if (!view.ok) return null;
  const { jewel, offer, hasCard, claim } = view.value;
  forgetStaleLinks(id, liveClaimKey(offer, now));
  const base = {
    id, jewelerId: jewel.jewelerId, inscription: jewel.inscription, stolen: jewel.stolen, onOffer: offerIsOpen(offer, now),
    hasCard, claimed: !!claim,
  };
  const f = await fetchVerified(currentRecordHash(jewel));
  const record = f.ok ? parseRecord(f.bytes) : null;
  if (!record || !recordMatches(record, jewel)) return { ...base, title: null, sku: '' };
  return { ...base, title: record.title, sku: record.version === 2 ? record.sku : '', photo: record.photos[0] };
}

/** `keep` decides which held jewels belong on this list (a shop's own stock, or everything else). */
export async function loadHoldings(address: string, now: number, keep: (h: Held) => boolean = () => true): Promise<Outcome<HeldGroup[]>> {
  const ids = await jewelsHeldBy(address);
  if (!ids.ok) return ids;
  const held = await mapLimit(ids.value, 6, (id) => loadHeld(id, now));
  return { ok: true, height: ids.height, value: groupHoldings(held.filter((h): h is Held => h !== null && keep(h))) };
}

/**
 * A design held once is a link; held more than once, a row with the count and a
 * menu of its serial numbers. `cards` adds the shop's view: pieces without an
 * ownership card, and claims waiting.
 */
export function HeldRow({ group, to, cards }: { group: HeldGroup; to: (id: string) => string; cards?: boolean }) {
  const navigate = useNavigate();
  const first = group.items[0]!;
  const title = group.title ?? t('traceability.jewelN', { id: first.id });
  const image = group.photo
    ? <VerifiedImage hash={group.photo} alt="" className="jewelcard__img" />
    : <span className="jewelcard__img photo--absent" />;
  const chips = (
    <>
      {group.stolen > 0 && <span className="y-chip y-chip--bad">{t('traceability.stolenShort')}</span>}
      {group.onOffer > 0 && <span className="y-chip y-chip--brass">{t('traceability.onOffer')}</span>}
      {cards && group.claimed > 0 && <span className="y-chip y-chip--brass">{t('traceability.claimedShort')}</span>}
      {cards && group.withoutCard > 0 && (
        <span className="y-chip y-chip--mute">
          {group.items.length === 1 ? t('traceability.noCardShort') : t('traceability.withoutCardN', { n: group.withoutCard })}
        </span>
      )}
    </>
  );
  if (group.items.length === 1) {
    return (
      <li className="y-card jewelcard">
        <Link className="jewelcard__link" to={to(first.id)}>
          {image}
          <span className="jewelcard__text"><span className="jewelcard__title">{title}</span><Hallmark code={first.inscription} /></span>
          {chips}
        </Link>
      </li>
    );
  }
  const mark = (h: Held) => [
    h.inscription, h.stolen ? t('traceability.stolenShort') : '', h.onOffer ? t('traceability.onOffer') : '',
    cards && h.claimed ? t('traceability.claimedShort') : '', cards && !h.hasCard ? t('traceability.noCardShort') : '',
  ].filter(Boolean).join(' · ');
  return (
    <li className="y-card jewelcard">
      <div className="jewelcard__link">
        {image}
        <span className="jewelcard__text">
          <span className="jewelcard__title">{title}</span>
          <span className="hint num">{t('traceability.pieceCount', { n: group.items.length })}</span>
        </span>
        {chips}
      </div>
      <select className="jewelcard__serials" value="" aria-label={t('traceability.serialsOf', { title })}
              onChange={(e) => { if (e.target.value) navigate(to(e.target.value)); }}>
        <option value="">{t('traceability.chooseSerial')}</option>
        {group.items.map((h) => <option key={h.id} value={h.id}>{mark(h)}</option>)}
      </select>
    </li>
  );
}
