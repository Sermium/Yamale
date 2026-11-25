/**
 * Finding a card again: every stock piece's ownership card, as this browser
 * kept it when the card was made, to show at the counter, share with the
 * customer (WhatsApp, email, a copied link) or print. Also the one-card panel
 * on a piece's own page in the Atelier.
 */

import { useState } from 'react';
import { cardLink, chainHead, t } from '@yamale/chain';

import { useSession } from '../account/session.tsx';
import { PUBLIC_ORIGIN } from '../config.ts';
import { loadHoldings } from '../owner/HeldList.tsx';
import { AppBar, Loading, Problem, ShareButtons, useLoad } from '../ui.tsx';
import { Hallmark } from '../verify/JewelPage.tsx';
import { useRole } from '../atelier/role.ts';
import { forgetKeptCard, keptCard } from './cards.ts';
import { CardSheet, OwnershipCard } from './PrintCards.tsx';

interface Shop { name: string; jewelerId: string; business: string }
interface Piece { jewelId: string; title: string | null; inscription: string }

/** One card: what the customer receives, and the ways to send it. */
export function KeptCard({ shop, piece, privateKey }: { shop: Shop; piece: Piece; privateKey: Uint8Array }) {
  const [printing, setPrinting] = useState(false);
  const link = cardLink(PUBLIC_ORIGIN, piece.jewelId, privateKey);
  const title = piece.title ?? t('traceability.jewelN', { id: piece.jewelId });
  return (
    <div className="stack keptcard">
      <div className="ocard-preview"><OwnershipCard shop={shop} card={{ ...piece, privateKey }} look={null} /></div>
      <ShareButtons link={link} title={t('traceability.cardShareTitle', { title })}
                    text={t('traceability.cardShareText', { shop: shop.name, title })} />
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
  const [stock] = useLoad(
    () => (shop && head?.ok ? loadHoldings(shop.business, head.value.time, (h) => h.jewelerId === shop.jewelerId)
      : Promise.resolve({ ok: true as const, height: 0, value: [] })),
    [shop?.business, head?.ok]);
  const [printAll, setPrintAll] = useState(false);

  const bar = <AppBar title={t('traceability.cardsPageTitle')} eyebrow={shop?.name ?? ''} back />;
  if (role.state === 'loading' || !head || !stock) return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (!shop) return <>{bar}<main className="y-screen"><p className="note">{t('traceability.staffOnly')}</p></main></>;
  if (!head.ok) return <>{bar}<main className="y-screen"><Problem outcome={head} /></main></>;
  if (!stock.ok) return <>{bar}<main className="y-screen"><Problem outcome={stock} /></main></>;

  const pieces = stock.value.flatMap((g) => g.items.map((h) => ({ jewelId: h.id, title: g.title, inscription: h.inscription, hasCard: !!h.hasCard })));
  // A card kept for a piece that has none on the chain any more is stale.
  for (const p of pieces) if (!p.hasCard) forgetKeptCard(p.jewelId);
  const kept = pieces.flatMap((p) => { const k = p.hasCard ? keptCard(p.jewelId) : null; return k ? [{ ...p, privateKey: k }] : []; });
  const elsewhere = pieces.filter((p) => p.hasCard && !keptCard(p.jewelId));
  const none = pieces.filter((p) => !p.hasCard);

  return (
    <>
      {bar}
      <main className="y-screen">
        <p className="lede">{t('traceability.cardsPageLede')}</p>
        {kept.length > 0 && (
          <div className="actions">
            <button type="button" className="btn btn--primary" onClick={() => { setPrintAll(true); setTimeout(() => { window.print(); setPrintAll(false); }, 300); }}>
              {t('traceability.cardsPrintAll', { n: kept.length })}
            </button>
          </div>
        )}
        {printAll && <CardSheet shop={shop} cards={kept} preview={false} />}
        {kept.length === 0 && <p className="hint">{t('traceability.cardsNoneKept')}</p>}
        <ul className="y-cards">
          {kept.map((p) => (
            <li key={p.jewelId} className="y-card claimrow">
              <span className="jewelcard__title">{p.title ?? t('traceability.jewelN', { id: p.jewelId })}</span>
              <Hallmark code={p.inscription} />
              <KeptCard shop={shop} piece={p} privateKey={p.privateKey} />
            </li>
          ))}
        </ul>
        {elsewhere.length > 0 && (
          <p className="note">{t('traceability.cardsElsewhere', { n: elsewhere.length, list: elsewhere.map((p) => p.inscription).join(', ') })}</p>
        )}
        {none.length > 0 && <p className="hint">{t('traceability.cardsMissing', { n: none.length })}</p>}
      </main>
    </>
  );
}
