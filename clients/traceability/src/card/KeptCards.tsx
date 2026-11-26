/**
 * Finding a card again: every stock piece with its ownership card, as this
 * browser kept it when the card was made, to show at the counter, share with
 * the customer (WhatsApp, email, a copied link) or print. A piece whose card is
 * not kept here gets a new one from this same page. Also the one-card panel on
 * a piece's own page in the Atelier.
 */

import { useEffect, useState, type CSSProperties } from 'react';
import { cardLink, chainHead, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { PUBLIC_ORIGIN } from '../config.ts';
import { loadHoldings } from '../owner/HeldList.tsx';
import { AppBar, Loading, Problem, ShareButtons, useLoad } from '../ui.tsx';
import { Hallmark } from '../verify/JewelPage.tsx';
import { stockPieces } from '../atelier/Atelier.tsx';
import { useRole } from '../atelier/shared/role.ts';
import { forgetKeptCard, keptCard, type Selection } from './cards.ts';
import { CardSheet, OwnershipCard, PrintCards } from './PrintCards.tsx';
import { CardImageButtons } from './CardImageButtons.tsx';
import { readProfile } from '../api.ts';
import { syncVault, type VaultSync } from './vaultSync.ts';

interface Shop { name: string; jewelerId: string; business: string }
interface Piece { jewelId: string; title: string | null; inscription: string }

/** One card: what the customer receives, and the ways to send it. */
export function KeptCard({ shop, piece, privateKey }: { shop: Shop; piece: Piece; privateKey: Uint8Array }) {
  const [printing, setPrinting] = useState(false);
  const [profile] = useLoad(() => readProfile(shop.jewelerId), [shop.jewelerId], () => ({ state: 'none' as const }));
  const look = profile && profile.state === 'loaded' ? profile.profile : null;
  const link = cardLink(PUBLIC_ORIGIN, piece.jewelId, privateKey);
  const title = piece.title ?? t('traceability.jewelN', { id: piece.jewelId });
  const brand = { ...(look?.colour ? { '--jeweler-colour': look.colour } : {}), ...(look?.accent ? { '--jeweler-accent': look.accent } : {}) } as CSSProperties;
  return (
    <div className="stack keptcard">
      <div className="ocard-preview" style={brand}><OwnershipCard shop={shop} card={{ ...piece, privateKey }} look={look} /></div>
      <CardImageButtons face={{
        shop: shop.name, title, inscription: piece.inscription, link, scanLine: t('traceability.cardScanLine'),
        colour: look?.colour, accent: look?.accent,
      }} />
      <details className="keptcard__link">
        <summary>{t('traceability.cardLinkOnly')}</summary>
        <div className="stack">
          <ShareButtons link={link} title={t('traceability.cardShareTitle', { title })}
                        text={t('traceability.cardShareText', { shop: shop.name, title })} />
          <input className="bigfield cardlink" readOnly value={link} aria-label={t('traceability.copyLink')}
                 onFocus={(e) => e.currentTarget.select()} />
        </div>
      </details>
      <button type="button" className="btn" onClick={() => { setPrinting(true); setTimeout(() => { window.print(); setPrinting(false); }, 300); }}>
        {t('traceability.cardsPrint')}
      </button>
      {printing && <CardSheet shop={shop} cards={[{ ...piece, privateKey }]} preview={false} />}
      <p className="hint">{t('traceability.cardKeptHint')}</p>
    </div>
  );
}

/** /atelier/cards */
export function KeptCardsPage() {
  const session = useSession();
  const [role] = useRole(session.address);
  const [head] = useLoad(() => chainHead(), []);
  const shop = role.state === 'role' ? { name: role.jeweler.name, jewelerId: role.jeweler.id, business: role.jeweler.business } : null;
  const [stock, reload] = useLoad(
    () => (shop && head?.ok ? loadHoldings(shop.business, head.value.time, (h) => h.jewelerId === shop.jewelerId)
      : Promise.resolve({ ok: true as const, height: 0, value: [] })),
    [shop?.business, head?.ok]);
  const [printAll, setPrintAll] = useState(false);
  // 'choose' opens the full chooser: pieces without a card, or every piece of one design.
  const [making, setMaking] = useState<Selection | 'choose' | null>(null);
  // The shop's card vault: cards made on another device or by a colleague
  // arrive here, and the ones kept here go to the colleagues who lack them.
  const [vault, setVault] = useState<VaultSync | 'running' | null>(null);
  const withCard = stock?.ok ? stockPieces(stock.value).filter((p) => p.hasCard).map((p) => p.jewelId).join(',') : '';
  useEffect(() => {
    if (!shop || !session.unlocked || !session.address || !withCard) return;
    let live = true;
    setVault('running');
    syncVault(session.withKey, session.address, shop.jewelerId, withCard.split(',')).then((r) => { if (live) setVault(r); });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shop?.jewelerId, withCard, session.unlocked, session.address]);

  const bar = <AppBar title={t('traceability.cardsPageTitle')} eyebrow={shop?.name ?? ''} back />;
  if (role.state === 'loading' || !head || !stock) return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (!shop || role.state !== 'role') return <>{bar}<main className="y-screen"><p className="note">{t('traceability.staffOnly')}</p></main></>;
  if (!head.ok) return <>{bar}<main className="y-screen"><Problem outcome={head} /></main></>;
  if (!stock.ok) return <>{bar}<main className="y-screen"><Problem outcome={stock} /></main></>;

  const all = stockPieces(stock.value);
  // A card kept for a piece that has none on the chain any more is stale.
  for (const p of all) if (!p.hasCard) forgetKeptCard(p.jewelId);
  const rows = all.map((p) => ({ ...p, privateKey: p.hasCard ? keptCard(p.jewelId) : null }));
  const kept = rows.flatMap((r) => (r.privateKey ? [{ ...r, privateKey: r.privateKey }] : []));
  // Only pieces with no card at all: making a card for a piece that has one
  // cancels the card already handed to the customer.
  const missing = rows.filter((r) => !r.hasCard);
  const elsewhere = rows.filter((r) => r.hasCard && !r.privateKey).length;
  const canMake = role.staff && role.active;

  return (
    <>
      {bar}
      <main className="y-screen">
        <p className="lede">{t('traceability.cardsPageLede')}</p>
        {elsewhere > 0 && !session.unlocked && <p className="note">{t('traceability.cardVaultLocked')}</p>}
        {vault === 'running' && <p className="hint" role="status">{t('traceability.cardVaultRunning')}</p>}
        {vault && vault !== 'running' && vault.ok && vault.received > 0 && (
          <p className="note" role="status">{t('traceability.cardVaultReceived', { n: vault.received })}</p>
        )}
        {vault && vault !== 'running' && !vault.ok && <p className="note note--warn" role="alert">{t('traceability.cardVaultFailed')}</p>}
        {all.length === 0 && <p className="hint">{t('traceability.noStock')}</p>}
        {making ? (
          <UnlockGate>
            <PrintCards shop={shop} stock={all} initial={making === 'choose' ? undefined : making}
                        onClose={(changed) => { setMaking(null); if (changed) reload(); }} />
          </UnlockGate>
        ) : (<>
          <div className="actions">
            {canMake && missing.length > 0 && (
              <button type="button" className="btn btn--primary" onClick={() => setMaking({ kind: 'some', jewelIds: missing.map((p) => p.jewelId) })}>
                {t('traceability.cardsMakeMissing', { n: missing.length })}
              </button>
            )}
            {canMake && all.length > 0 && (
              <button type="button" className="btn" onClick={() => setMaking('choose')}>{t('traceability.cardsPrintButton')}</button>
            )}
            {kept.length > 0 && (
              <button type="button" className="btn" onClick={() => { setPrintAll(true); setTimeout(() => { window.print(); setPrintAll(false); }, 300); }}>
                {t('traceability.cardsPrintAll', { n: kept.length })}
              </button>
            )}
          </div>
          {printAll && <CardSheet shop={shop} cards={kept} preview={false} />}
          <ul className="y-cards">
            {rows.map((p) => (
              <li key={p.jewelId} className="y-card claimrow">
                <span className="jewelcard__title">{p.title ?? t('traceability.jewelN', { id: p.jewelId })}</span>
                <Hallmark code={p.inscription} />
                {p.privateKey ? (
                  <KeptCard shop={shop} piece={p} privateKey={p.privateKey} />
                ) : (<>
                  <p className="hint">{t(p.hasCard ? 'traceability.cardElsewhere' : 'traceability.cardNone')}</p>
                  {canMake && (
                    <div className="actions">
                      <button type="button" className={p.hasCard ? 'btn btn--danger' : 'btn'} onClick={() => setMaking({ kind: 'one', jewelId: p.jewelId })}>
                        {t(p.hasCard ? 'traceability.cardReplace' : 'traceability.cardMakeOne')}
                      </button>
                    </div>
                  )}
                </>)}
              </li>
            ))}
          </ul>
        </>)}
      </main>
    </>
  );
}
