/**
 * Making ownership cards from stock: choose the pieces, issue one card each on
 * the chain, then show, print or share them. Each card is also kept in this
 * browser (cards.ts keepCard), so it can be found again on the piece's page.
 */

import { useEffect, useMemo, useState, type CSSProperties } from 'react';
import { createPortal } from 'react-dom';
import { cardLink, issueJewelCard, t } from '@yamale/chain';

import { QrCode } from '../../../app/src/qr.tsx';
import { useSession } from '../account/session.tsx';
import { readProfile } from '../api.ts';
import { errorKey } from '../actions/errors.ts';
import { PUBLIC_ORIGIN } from '../config.ts';
import { useLoad } from '../ui.tsx';
import { Hallmark } from '../verify/JewelPage.tsx';
import { VerifiedImage } from '../verify/Photos.tsx';
import { syncVault } from './vaultSync.ts';
import { cancels, forgetKeptCard, issueCards, keepCard, pages, selectPieces, type IssueResult, type Selection, type StockPiece } from './cards.ts';

interface Shop { name: string; jewelerId: string; business: string }

export function PrintCards({ shop, stock, initial, onClose }: {
  shop: Shop; stock: StockPiece[]; initial?: Selection; onClose: (changed: boolean) => void;
}) {
  const session = useSession();
  const designs = useMemo(() => {
    const m = new Map<string, { title: string; n: number }>();
    for (const p of stock) {
      const d = m.get(p.design) ?? { title: p.title ?? t('traceability.jewelN', { id: p.jewelId }), n: 0 };
      d.n++;
      m.set(p.design, d);
    }
    return [...m.entries()];
  }, [stock]);
  const [sel, setSel] = useState<Selection>(initial ?? { kind: 'without' });
  const [phase, setPhase] = useState<'choose' | 'issuing' | 'done'>('choose');
  const [progress, setProgress] = useState<[number, number]>([0, 0]);
  const [result, setResult] = useState<IssueResult | null>(null);
  const [confirmCancel, setConfirmCancel] = useState(false);

  const chosen = selectPieces(stock, sel);
  const cancelled = cancels(chosen);

  async function issue() {
    setPhase('issuing');
    const out = await issueCards(chosen, async (batch) => {
      // Kept before sending: a submit that reaches the chain and then fails to report must not lose the card.
      for (const c of batch) keepCard(c.jewelId, c.privateKey);
      const r = await session.submit(batch.map((c) => issueJewelCard(session.address!, c.jewelId, c.cardKey)), shop.business);
      if (r.succeeded) return { ok: true };
      for (const c of batch) forgetKeptCard(c.jewelId);
      const raw = r.error?.raw ?? r.raw.rawLog ?? '';
      const key = errorKey(raw);
      return { ok: false, text: key ? t(`traceability.err.${key}`) : (r.error?.message ?? t('traceability.err.unknown')) };
    }, undefined, undefined, (d, of) => setProgress([d, of]));
    // Into the shop's vault before anything else, so the cards are on the
    // colleagues' devices even if this one is closed straight after printing.
    if (out.issued.length > 0 && session.address) {
      const ids = out.issued.map((c) => c.jewelId);
      await syncVault(session.withKey, session.address, shop.jewelerId, ids, new Set(ids));
    }
    setResult(out);
    setPhase('done');
  }

  function close() {
    onClose((result?.issued.length ?? 0) > 0);
  }

  if (phase === 'choose') {
    return (
      <div className="stack printcards">
        <fieldset className="stack">
          <legend className="y-label">{t('traceability.cardsWhich')}</legend>
          {initial?.kind === 'one' ? (
            <p>{t('traceability.cardsOne')}</p>
          ) : initial?.kind === 'some' ? (
            <p>{chosen.map((p) => p.inscription).join(', ')}</p>
          ) : (<>
            <label className="tick">
              <input type="radio" name="cards" checked={sel.kind === 'without'} onChange={() => setSel({ kind: 'without' })} />
              {t('traceability.cardsWithout', { n: stock.filter((p) => !p.hasCard).length })}
            </label>
            <label className="tick">
              <input type="radio" name="cards" checked={sel.kind === 'design'}
                     onChange={() => setSel({ kind: 'design', design: designs[0]?.[0] ?? '' })} disabled={designs.length === 0} />
              {t('traceability.cardsDesign')}
            </label>
            {sel.kind === 'design' && (
              <select className="bigfield" value={sel.design} onChange={(e) => setSel({ kind: 'design', design: e.target.value })}
                      aria-label={t('traceability.cardsDesign')}>
                {designs.map(([key, d]) => <option key={key} value={key}>{`${d.title} (${d.n})`}</option>)}
              </select>
            )}
          </>)}
        </fieldset>
        {cancelled > 0 && <p className="note note--warn">{t('traceability.cardsCancelWarning', { n: cancelled })}</p>}
        {cancelled > 0 && (
          <label className="tick">
            <input type="checkbox" checked={confirmCancel} onChange={(e) => setConfirmCancel(e.target.checked)} />
            {t('traceability.cardsCancelConfirm')}
          </label>
        )}
        <p className="hint">{t('traceability.cardsExplain')}</p>
        <div className="actions">
          <button type="button" className="btn btn--primary" disabled={chosen.length === 0 || (cancelled > 0 && !confirmCancel)}
                  onClick={issue}>{t('traceability.cardsIssue', { n: chosen.length })}</button>
          <button type="button" className="btn" onClick={() => onClose(false)}>{t('traceability.cancel')}</button>
        </div>
      </div>
    );
  }

  if (phase === 'issuing') {
    return <p className="loading" role="status">{t('traceability.cardsIssuing', { done: progress[0], of: progress[1] })}</p>;
  }

  const r = result!;
  const notIssued = chosen.filter((p) => r.outcomes.get(p.jewelId) !== 'issued');
  return (
    <div className="stack printcards">
      {r.issued.length > 0 && <p className="banner banner--ok" role="status">{t('traceability.cardsReady', { n: r.issued.length })}</p>}
      {r.stopped && (
        <div className="note note--warn" role="alert">
          <p>{t('traceability.cardsStopped', { n: notIssued.length })}</p>
          <p>{r.stopped}</p>
          <ul className="hint">
            {notIssued.map((p) => <li key={p.jewelId}>{p.inscription} · {t(`traceability.cardOutcome.${r.outcomes.get(p.jewelId)}`)}</li>)}
          </ul>
        </div>
      )}
      {r.issued.length > 0 && <p className="note note--warn">{t('traceability.cardsPrintNow')}</p>}
      <div className="actions">
        {r.issued.length > 0 && <button type="button" className="btn btn--primary" onClick={() => window.print()}>{t('traceability.cardsPrint')}</button>}
        <button type="button" className="btn" onClick={close}>{t('traceability.cardsClose')}</button>
      </div>
      {r.issued.length > 0 && <CardSheet shop={shop} cards={r.issued} />}
    </div>
  );
}

type Look = { logo: string; colour: string; accent: string } | null;

function brandOf(look: Look): CSSProperties {
  return {
    ...(look?.colour ? { '--jeweler-colour': look.colour } : {}),
    ...(look?.accent ? { '--jeweler-accent': look.accent } : {}),
  } as CSSProperties;
}

export function OwnershipCard({ shop, card, look }: { shop: Shop; card: IssueResult['issued'][number]; look: Look }) {
  return (
    <article className="ocard">
      <header className="ocard__shop">
        {look?.logo ? <VerifiedImage hash={look.logo} alt="" className="ocard__logo" /> : null}
        <span className="ocard__name">{shop.name}</span>
      </header>
      <div className="ocard__body">
        <div className="ocard__qr"><QrCode text={cardLink(PUBLIC_ORIGIN, card.jewelId, card.privateKey)} size={132} /></div>
        <div className="ocard__piece">
          <span className="ocard__title">{card.title ?? t('traceability.jewelN', { id: card.jewelId })}</span>
          <Hallmark code={card.inscription} />
          <span className="ocard__line">{t('traceability.cardScanLine')}</span>
        </div>
      </div>
    </article>
  );
}

/**
 * The first card as a preview in the panel, and every card on A4 pages in a
 * sheet that only exists in print: the print styles hide the app and show it.
 */
export function CardSheet({ shop, cards, preview = true }: { shop: Shop; cards: IssueResult['issued']; preview?: boolean }) {
  const [profile] = useLoad(() => readProfile(shop.jewelerId), [shop.jewelerId], () => ({ state: 'none' as const }));
  const look: Look = profile && profile.state === 'loaded' ? profile.profile : null;
  useEffect(() => {
    document.body.classList.add('printing-cards');
    return () => document.body.classList.remove('printing-cards');
  }, []);
  const sheet = (
    <div className="cardsheet" style={brandOf(look)}>
      {pages(cards).map((page, i) => (
        <section className="cardsheet__page" key={i}>
          {page.map((c) => <OwnershipCard key={c.jewelId} shop={shop} card={c} look={look} />)}
        </section>
      ))}
    </div>
  );
  return (
    <>
      {preview && (
        <div className="ocard-preview" style={brandOf(look)} aria-label={t('traceability.cardsPreview')}>
          <OwnershipCard shop={shop} card={cards[0]!} look={look} />
        </div>
      )}
      {createPortal(sheet, document.body)}
    </>
  );
}
