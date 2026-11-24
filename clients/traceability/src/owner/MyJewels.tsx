import { useState } from 'react';
import { Link } from 'react-router-dom';
import { acceptJewelOffer, chainHead, offersAddressedTo, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { AppBar, Loading, Problem, Section, useLoad } from '../ui.tsx';
import { useRole } from '../atelier/role.ts';
import { JewelCard } from '../verify/JewelCard.tsx';
import { HeldRow, loadHoldings } from './HeldList.tsx';

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
  const [role] = useRole(address);
  if (!head || role.state === 'loading') return <Loading />;
  if (!head.ok) return <Problem outcome={head} />;
  // A jeweler's business account holds every piece it registers until it is sold:
  // that is the shop's stock, listed in the Atelier, not jewels this person owns.
  const shop = role.state === 'role' && role.business ? role.jeweler.id : null;
  return <HoldingsAt address={address} now={head.value.time} shop={shop} />;
}

function HoldingsAt({ address, now, shop }: { address: string; now: number; shop: string | null }) {
  const [offers, reloadOffers] = useLoad(() => offersAddressedTo(address, now), [address]);
  const [held, reloadHeld] = useLoad(() => loadHoldings(address, now, (h) => h.jewelerId !== shop), [address, shop]);
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
        {shop && <p className="hint">{t('traceability.shopStockElsewhere')} <Link to="/atelier">{t('traceability.navAtelier')}</Link></p>}
        {held?.ok && held.value.length === 0 && <p className="hint">{t('traceability.noJewels')}</p>}
        {held?.ok && held.value.length > 0 && (
          <ul className="y-cards">{held.value.map((g) => <HeldRow key={g.key} group={g} to={(id) => `/mine/${id}`} />)}</ul>
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
