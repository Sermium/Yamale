import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { chainHead, findByInscription, normaliseInscription, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { AppBar, Loading, Problem, Section, useLoad } from '../ui.tsx';
import { HeldRow, loadHoldings } from '../owner/HeldList.tsx';
import type { HeldGroup } from '../owner/holdings.ts';
import { PrintCards } from '../card/PrintCards.tsx';
import type { StockPiece } from '../card/cards.ts';
import { ClaimsToValidate } from './claims/Claims.tsx';
import { useRole } from './shared/role.ts';
import { BusinessMoveInvite } from './shop/BusinessMove.tsx';

export function Atelier() {
  const session = useSession();
  const [role, reloadRole] = useRole(session.address);
  const navigate = useNavigate();
  const [code, setCode] = useState('');
  const [missing, setMissing] = useState(false);
  const [findFailed, setFindFailed] = useState<{ reason: string; detail: string } | null>(null);
  const business = role.state === 'role' ? role.jeweler.business : null;
  // An offer past the chain's clock is no offer: the list waits for that clock, not the phone's.
  const [head] = useLoad(() => chainHead(), []);
  // Validating a claim moves a piece out of stock: the stock list reloads with it.
  const [stockVersion, setStockVersion] = useState(0);

  if (role.state === 'loading') return <><AppBar title={t('traceability.atelierTitle')} eyebrow={t('traceability.title')} /><main className="y-screen"><Loading /></main></>;
  if (role.state !== 'role') {
    return (
      <>
        <AppBar title={t('traceability.atelierTitle')} eyebrow={t('traceability.title')} />
        <main className="y-screen">
          {role.state === 'failed' ? <Problem outcome={role.outcome} /> : (
            <>
              {session.address && <BusinessMoveInvite me={session.address} onDone={reloadRole} />}
              <p className="note">{t('traceability.notAJeweler')}</p>
              <Link className="btn" to="/apply">{t('traceability.applyLink')}</Link>
            </>
          )}
        </main>
      </>
    );
  }
  const j = role.jeweler;

  async function find(e: FormEvent) {
    e.preventDefault();
    const n = normaliseInscription(code);
    if (!n) return;
    setFindFailed(null);
    const out = await findByInscription(n);
    if (!out.ok) { setFindFailed(out); return; }
    const mine = out.value.find((x) => x.jewelerId === j.id);
    if (mine) navigate(`/atelier/jewel/${mine.id}`); else setMissing(true);
  }

  return (
    <>
      <AppBar title={j.name} eyebrow={t('traceability.atelierTitle')} />
      <main className="y-screen">
        <div className="actions">
          <span className={`y-chip ${role.active ? 'y-chip--ok' : 'y-chip--bad'}`}>
            {role.active ? t('traceability.jewelerActive') : t('traceability.jewelerSuspended')}
          </span>
          {role.business && <span className="y-chip y-chip--brass">{t('traceability.youAreBusiness')}</span>}
        </div>
        {!role.active && <p className="banner banner--warn">{t('traceability.suspendedNote')}</p>}
        {!role.business && session.address && <BusinessMoveInvite me={session.address} onDone={reloadRole} />}
        {role.business && !role.staff && <p className="hint">{t('traceability.businessAddYourself')}</p>}

        <div className="actions">
          {role.staff && role.active && <Link className="btn btn--primary" to="/atelier/register">{t('traceability.registerTitle')}</Link>}
          {role.active && <Link className="btn" to="/atelier/catalogue">{t('traceability.catalogueLink')}</Link>}
          {role.staff && role.active && <Link className="btn" to="/atelier/redeem">{t('traceability.redeemLink')}</Link>}
          {role.business && <Link className="btn" to="/atelier/look">{t('traceability.lookLink')}</Link>}
          {role.business && <Link className="btn" to="/atelier/staff">{t('traceability.staffLink')}</Link>}
        </div>

        {head?.ok && business && (
          <ClaimsToValidate jewelerId={j.id} business={business} canAct={role.staff && role.active} now={head.value.time}
                            onChange={() => setStockVersion((v) => v + 1)} />
        )}

        <Section title={t('traceability.findJewel')}>
          <form className="stack" onSubmit={find}>
            <input className="bigfield inscription" value={code} onChange={(e) => { setCode(e.target.value); setMissing(false); setFindFailed(null); }}
                   aria-label={t('traceability.codeLabel')} autoCapitalize="characters" spellCheck={false} />
            <button className="btn">{t('traceability.verifyButton')}</button>
            {missing && <p className="hint">{t('traceability.notOurs')}</p>}
            {findFailed && <Problem outcome={findFailed} />}
          </form>
        </Section>

        <Section title={t('traceability.stock')}>
          {!head && <Loading />}
          {head && !head.ok && <Problem outcome={head} />}
          {head?.ok && business && (
            <Stock key={stockVersion} shop={{ name: j.name, jewelerId: j.id, business }} now={head.value.time}
                   canPrint={role.staff && role.active} />
          )}
        </Section>
      </main>
    </>
  );
}

/** Every piece of the stock as the card printer needs it, the design being the list's own row. */
export function stockPieces(groups: HeldGroup[]): StockPiece[] {
  return groups.flatMap((g) => g.items.map((h) => ({
    jewelId: h.id, title: g.title, inscription: h.inscription, hasCard: !!h.hasCard, design: g.key,
  })));
}

/** The shop's stock: what its business account holds of its own jewels, one row per design. */
function Stock({ shop, now, canPrint }: {
  shop: { name: string; jewelerId: string; business: string }; now: number; canPrint: boolean;
}) {
  // A business account can hold jewels of other jewelers; a jeweler's stock is its own.
  const [stock, reload] = useLoad(() => loadHoldings(shop.business, now, (h) => h.jewelerId === shop.jewelerId), [shop.business, shop.jewelerId]);
  const [printing, setPrinting] = useState(false);
  if (!stock) return <Loading />;
  if (!stock.ok) return <Problem outcome={stock} />;
  if (stock.value.length === 0) return <p className="hint">{t('traceability.noStock')}</p>;
  const without = stock.value.reduce((n, g) => n + g.withoutCard, 0);
  return (
    <>
      {canPrint && !printing && (
        <div className="actions">
          <button type="button" className="btn btn--primary" onClick={() => setPrinting(true)}>{t('traceability.cardsPrintButton')}</button>
          <Link className="btn" to="/atelier/cards">{t('traceability.cardsFindButton')}</Link>
          {without > 0 && <span className="hint">{t('traceability.cardsMissing', { n: without })}</span>}
        </div>
      )}
      {printing && (
        <UnlockGate>
          <PrintCards shop={shop} stock={stockPieces(stock.value)}
                      onClose={(changed) => { setPrinting(false); if (changed) reload(); }} />
        </UnlockGate>
      )}
      <ul className="y-cards">{stock.value.map((g) => <HeldRow key={g.key} group={g} cards to={(id) => `/atelier/jewel/${id}`} />)}</ul>
    </>
  );
}
