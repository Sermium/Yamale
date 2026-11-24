import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { acceptJewelOffer, chainHead, jewelsHeldBy, offersAddressedTo, readJewel, t, type Outcome } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { AppBar, Loading, Problem, Section, useLoad } from '../ui.tsx';
import { fetchVerified } from '../api.ts';
import { JewelCard } from '../verify/JewelCard.tsx';
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
  const { jewel, offer } = view.value;
  forgetStaleLinks(id, liveClaimKey(offer, now));
  const base = { id, jewelerId: jewel.jewelerId, inscription: jewel.inscription, stolen: jewel.stolen, onOffer: offerIsOpen(offer, now) };
  const f = await fetchVerified(currentRecordHash(jewel));
  const record = f.ok ? parseRecord(f.bytes) : null;
  if (!record || !recordMatches(record, jewel)) return { ...base, title: null, sku: '' };
  return { ...base, title: record.title, sku: record.version === 2 ? record.sku : '', photo: record.photos[0] };
}

async function loadHoldings(address: string, now: number): Promise<Outcome<HeldGroup[]>> {
  const ids = await jewelsHeldBy(address);
  if (!ids.ok) return ids;
  const held = await mapLimit(ids.value, 6, (id) => loadHeld(id, now));
  return { ok: true, height: ids.height, value: groupHoldings(held.filter((h): h is Held => h !== null)) };
}

/** A design held once is a link; held more than once, a row with the count and a menu of its serial numbers. */
function HeldRow({ group }: { group: HeldGroup }) {
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
    </>
  );
  if (group.items.length === 1) {
    return (
      <li className="y-card jewelcard">
        <Link className="jewelcard__link" to={`/mine/${first.id}`}>
          {image}
          <span className="jewelcard__text"><span className="jewelcard__title">{title}</span><Hallmark code={first.inscription} /></span>
          {chips}
        </Link>
      </li>
    );
  }
  const mark = (h: Held) => [h.inscription, h.stolen ? t('traceability.stolenShort') : '', h.onOffer ? t('traceability.onOffer') : ''].filter(Boolean).join(' · ');
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
              onChange={(e) => { if (e.target.value) navigate(`/mine/${e.target.value}`); }}>
        <option value="">{t('traceability.chooseSerial')}</option>
        {group.items.map((h) => <option key={h.id} value={h.id}>{mark(h)}</option>)}
      </select>
    </li>
  );
}

function AcceptButton({ jewelId, onDone }: { jewelId: string; onDone: () => void }) {
  const session = useSession();
  const [action, run] = useAction();
  const [confirm, setConfirm] = useState(false);
  if (!confirm) return <button type="button" className="btn btn--primary" onClick={() => setConfirm(true)}>{t('traceability.accept')}</button>;
  return (
    <UnlockGate>
      <p className="note">{t('traceability.acceptExplain')}</p>
      <div className="actions">
        <button type="button" className="btn btn--primary" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([acceptJewelOffer(session.address!, jewelId)]))) onDone();
        }}>{t('traceability.acceptConfirm')}</button>
        <button type="button" className="btn" onClick={() => setConfirm(false)}>{t('traceability.cancel')}</button>
      </div>
      <ActionStatus state={action} />
    </UnlockGate>
  );
}

/** Waits for the chain's clock: whether an offer is still open is decided by block time, not the phone's. */
function Holdings({ address }: { address: string }) {
  const [head] = useLoad(() => chainHead(), []);
  if (!head) return <Loading />;
  if (!head.ok) return <Problem outcome={head} />;
  return <HoldingsAt address={address} now={head.value.time} />;
}

function HoldingsAt({ address, now }: { address: string; now: number }) {
  const [offers, reloadOffers] = useLoad(() => offersAddressedTo(address, now), [address]);
  const [held, reloadHeld] = useLoad(() => loadHoldings(address, now), [address]);
  const reload = () => { reloadOffers(); reloadHeld(); };
  return (
    <>
      {offers?.ok && offers.value.length > 0 && (
        <Section title={t('traceability.offersForYou')}>
          <ul className="y-cards">
            {offers.value.map((v) => (
              <JewelCard key={v.jewel.id} id={v.jewel.id} now={now} to={`/j/${v.jewel.jewelerId}/${v.jewel.id}`}
                         extra={<AcceptButton jewelId={v.jewel.id} onDone={reload} />} />
            ))}
          </ul>
        </Section>
      )}
      {offers && !offers.ok && <Problem outcome={offers} />}
      <Section title={t('traceability.yourJewels')}>
        {!held && <p className="loading">{t('traceability.loading')}</p>}
        {held && !held.ok && <Problem outcome={held} />}
        {held?.ok && held.value.length === 0 && <p className="hint">{t('traceability.noJewels')}</p>}
        {held?.ok && held.value.length > 0 && (
          <ul className="y-cards">{held.value.map((g) => <HeldRow key={g.key} group={g} />)}</ul>
        )}
      </Section>
    </>
  );
}

export function MyJewels() {
  const session = useSession();
  return (
    <>
      <AppBar title={t('traceability.mineTitle')} eyebrow={t('traceability.title')} />
      <main className="y-screen">
        {session.address ? <Holdings address={session.address} /> : <UnlockGate>{null}</UnlockGate>}
      </main>
    </>
  );
}
