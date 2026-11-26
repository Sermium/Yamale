/**
 * The shop's home. Read top to bottom in the order a shop works: who we are
 * and whether we may act, the one thing done most (register a piece), what
 * is waiting on us (claims), what we hold (stock), and then the tools.
 */

import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { chainHead, findByInscription, normaliseInscription, t, type Jeweler } from '@yamale/chain';

import { useSession } from '../account/session.tsx';
import { fetchProfile } from '../api.ts';
import { Icon } from '../icons.tsx';
import { MenuGroup, MenuLink } from '../menu.tsx';
import { AppBar, Loading, Problem, useLoad } from '../ui.tsx';
import { HeldRow, loadHoldings } from '../owner/HeldList.tsx';
import type { HeldGroup } from '../owner/holdings.ts';
import type { StockPiece } from '../card/cards.ts';
import { VerifiedImage } from '../verify/Photos.tsx';
import { ClaimsToValidate } from './claims/Claims.tsx';
import { useRole } from './shared/role.ts';
import { BusinessMoveInvite } from './shop/BusinessMove.tsx';

export function Atelier() {
  const session = useSession();
  const [role, reloadRole] = useRole(session.address);
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
        <main className="y-screen page">
          {role.state === 'failed' ? <Problem outcome={role.outcome} /> : (
            <>
              {session.address && <BusinessMoveInvite me={session.address} onDone={reloadRole} />}
              <div className="emptystate">
                <span className="emptystate__icon"><Icon name="jewelers" size={28} /></span>
                <p>{t('traceability.notAJeweler')}</p>
                <Link className="btn btn--primary" to="/apply">{t('traceability.applyLink')}</Link>
              </div>
            </>
          )}
        </main>
      </>
    );
  }
  const j = role.jeweler;
  const canWork = role.staff && role.active;

  return (
    <>
      <AppBar title={t('traceability.atelierTitle')} eyebrow={t('traceability.title')} />
      <main className="y-screen page">
        <ShopHeader jeweler={j} active={role.active} business={role.business} />

        {!role.active && <p className="banner banner--warn">{t('traceability.suspendedNote')}</p>}
        {!role.business && session.address && <BusinessMoveInvite me={session.address} onDone={reloadRole} />}
        {role.business && !role.staff && <p className="note">{t('traceability.businessAddYourself')}</p>}

        {canWork && (
          <Link className="btn btn--primary btn--block" to="/atelier/register">
            <Icon name="plus" /> {t('traceability.registerTitle')}
          </Link>
        )}

        {head?.ok && business && (
          <ClaimsToValidate jewelerId={j.id} business={business} canAct={canWork} now={head.value.time}
                            onChange={() => setStockVersion((v) => v + 1)} />
        )}

        <section className="pagesection">
          <h2 className="pagesection__title">{t('traceability.stock')}</h2>
          <FindPiece jewelerId={j.id} />
          {!head && <Loading />}
          {head && !head.ok && <Problem outcome={head} />}
          {head?.ok && business && (
            <Stock key={stockVersion} shop={{ jewelerId: j.id, business }} now={head.value.time} />
          )}
        </section>

        <MenuGroup title={t('traceability.atelierPiecesGroup')}>
          {role.active && <MenuLink to="/atelier/catalogue" icon="catalogue" title={t('traceability.catalogueLink')} sub={t('traceability.atelierCatalogueSub')} />}
          <MenuLink to="/atelier/cards" icon="card" tone="brass" title={t('traceability.cardsPageTitle')} sub={t('traceability.atelierCardsSub')} />
          {canWork && <MenuLink to="/atelier/redeem" icon="ticket" title={t('traceability.redeemLink')} sub={t('traceability.atelierRedeemSub')} />}
        </MenuGroup>

        {role.business && (
          <MenuGroup title={t('traceability.atelierShopGroup')}>
            <MenuLink to="/atelier/look" icon="palette" title={t('traceability.lookLink')} sub={t('traceability.atelierLookSub')} />
            <MenuLink to="/atelier/staff" icon="team" title={t('traceability.staffLink')}
                      sub={t('traceability.atelierStaffSub', { n: j.staff.length })} />
          </MenuGroup>
        )}
      </main>
    </>
  );
}

/** The shop's mark (its logo, or its initial), its name, and what this account may do there. */
function ShopHeader({ jeweler, active, business }: { jeweler: Jeweler; active: boolean; business: boolean }) {
  const [profile] = useLoad(() => fetchProfile(jeweler.id), [jeweler.id], () => null);
  return (
    <header className="shophead">
      <span className="shophead__mark">
        {profile?.logo ? <VerifiedImage hash={profile.logo} alt="" className="shophead__logo" /> : jeweler.name.trim().charAt(0).toUpperCase()}
      </span>
      <div className="shophead__text">
        <h1 className="shophead__name">{jeweler.name}</h1>
        <div className="shophead__meta">
          <span className={`y-chip ${active ? 'y-chip--ok' : 'y-chip--bad'}`}>
            {active ? t('traceability.jewelerActive') : t('traceability.jewelerSuspended')}
          </span>
          <span className="y-chip y-chip--mute">{business ? t('traceability.youAreBusiness') : t('traceability.youAreStaff')}</span>
        </div>
      </div>
    </header>
  );
}

/** Straight to a piece by the code engraved on it. */
function FindPiece({ jewelerId }: { jewelerId: string }) {
  const navigate = useNavigate();
  const [code, setCode] = useState('');
  const [missing, setMissing] = useState(false);
  const [failed, setFailed] = useState<{ reason: string; detail: string } | null>(null);

  async function find(e: FormEvent) {
    e.preventDefault();
    const n = normaliseInscription(code);
    if (!n) return;
    setFailed(null);
    const out = await findByInscription(n);
    if (!out.ok) { setFailed(out); return; }
    const mine = out.value.find((x) => x.jewelerId === jewelerId);
    if (mine) navigate(`/atelier/jewel/${mine.id}`); else setMissing(true);
  }

  return (
    <form className="searchbar" onSubmit={find} role="search">
      <Icon name="verify" className="searchbar__icon" />
      <input className="searchbar__input inscription" value={code} placeholder={t('traceability.codeLabel')}
             onChange={(e) => { setCode(e.target.value); setMissing(false); setFailed(null); }}
             aria-label={t('traceability.codeLabel')} autoCapitalize="characters" spellCheck={false} />
      <button className="btn searchbar__go" disabled={!normaliseInscription(code)}>{t('traceability.verifyButton')}</button>
      {missing && <p className="hint searchbar__note">{t('traceability.notOurs')}</p>}
      {failed && <div className="searchbar__note"><Problem outcome={failed} /></div>}
    </form>
  );
}

/** Every piece of the stock as the card printer needs it, the design being the list's own row. */
export function stockPieces(groups: HeldGroup[]): StockPiece[] {
  return groups.flatMap((g) => g.items.map((h) => ({
    jewelId: h.id, title: g.title, inscription: h.inscription, hasCard: !!h.hasCard, design: g.key,
  })));
}

/** The shop's stock: what its business account holds of its own jewels, one row per design. */
function Stock({ shop, now }: { shop: { jewelerId: string; business: string }; now: number }) {
  // A business account can hold jewels of other jewelers; a jeweler's stock is its own.
  const [stock] = useLoad(() => loadHoldings(shop.business, now, (h) => h.jewelerId === shop.jewelerId), [shop.business, shop.jewelerId]);
  if (!stock) return <Loading />;
  if (!stock.ok) return <Problem outcome={stock} />;
  if (stock.value.length === 0) return <p className="hint">{t('traceability.noStock')}</p>;
  const pieces = stock.value.reduce((n, g) => n + g.items.length, 0);
  const without = stock.value.reduce((n, g) => n + g.withoutCard, 0);
  return (
    <>
      <p className="pagesection__summary num">
        {t('traceability.stockSummary', { n: pieces, designs: stock.value.length })}
        {without > 0 && <> · <Link to="/atelier/cards">{t('traceability.cardsMissing', { n: without })}</Link></>}
      </p>
      <ul className="y-cards">{stock.value.map((g) => <HeldRow key={g.key} group={g} cards to={(id) => `/atelier/jewel/${id}`} />)}</ul>
    </>
  );
}
