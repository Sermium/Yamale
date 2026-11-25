import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { chainHead, readJewel, t } from '@yamale/chain';

import { UnlockGate } from '../../account/Account.tsx';
import { useSession } from '../../account/session.tsx';
import { OfferPanel } from '../../owner/OfferPanel.tsx';
import { offerIsOpen } from '../../owner/offers.ts';
import { PrintCards } from '../../card/PrintCards.tsx';
import { forgetKeptCard, keptCard } from '../../card/cards.ts';
import { KeptCard } from '../../card/KeptCards.tsx';
import { ClaimDecision } from '../claims/Claims.tsx';
import { AppBar, day, isId, Loading, NotFound, Problem, Section, useLoad } from '../../ui.tsx';
import { Hallmark, RecordFacts } from '../../verify/JewelPage.tsx';
import { Gallery } from '../../verify/Photos.tsx';
import { useRecord } from '../../verify/record.ts';
import { AttachBenefit, JewelBenefits } from '../benefits/Benefits.tsx';
import { useRole } from '../shared/role.ts';

const NO_SUCH = { ok: false as const, reason: 'not-found' as const, detail: '' };

export function AtelierJewel() {
  const { jewel: id = '' } = useParams();
  const valid = isId(id);
  const session = useSession();
  const [role] = useRole(session.address);
  const [view, reload] = useLoad(() => (valid ? readJewel(id) : Promise.resolve(NO_SUCH)), [id, valid]);
  const [head, reloadHead] = useLoad(() => chainHead(), []);
  const record = useRecord(view?.ok ? view.value.jewel : null);
  const [printing, setPrinting] = useState(false);

  const bar = <AppBar title={t('traceability.atelierTitle')} eyebrow="" back />;
  if (!valid) return <NotFound />;
  if (!view || !head || role.state === 'loading') return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <>{bar}<main className="y-screen"><Problem outcome={view} /></main></>;
  if (!head.ok) return <>{bar}<main className="y-screen"><Problem outcome={head} /></main></>;
  if (role.state === 'failed') return <>{bar}<main className="y-screen"><Problem outcome={role.outcome} /></main></>;
  if (role.state !== 'role' || view.value.jewel.jewelerId !== role.jeweler.id) return <NotFound />;

  const v = view.value;
  const now = head.value.time;
  const inStock = v.owner === role.jeweler.business;
  const title = record.state === 'ok' ? record.record.title : t('traceability.jewelN', { id });
  const lastChange = v.jewel.ownerSince[v.jewel.ownerSince.length - 1];
  // A kept card for a piece that has left stock, or lost its card, can claim nothing.
  if (!inStock || !v.hasCard) forgetKeptCard(id);
  const canAct = role.staff && role.active;

  return (
    <>
      <AppBar title={title} eyebrow={role.jeweler.name} back />
      <main className="y-screen">
        <header className="jewel__mast"><h1 className="jewel__title">{title}</h1><Hallmark code={v.jewel.inscription} /></header>
        {record.state === 'ok' && <Gallery hashes={record.record.photos} alt={title} />}
        {record.state === 'mismatch' && <div className="banner banner--bad" role="alert">{t('traceability.recordMismatch')}</div>}
        {(record.state === 'missing' || record.state === 'unreachable' || record.state === 'unreadable') && (
          <p className="note">{t('traceability.recordMissing')}</p>
        )}
        <div className="actions">
          <Link className="btn" to={`/j/${v.jewel.jewelerId}/${v.jewel.id}`}>{t('traceability.seePublicPage')}</Link>
          {canAct && v.jewel.amendments.length < 50 && (
            <Link className="btn" to={`/atelier/jewel/${id}/amend`}>{t('traceability.correctRecord')}</Link>
          )}
        </div>
        {record.state === 'ok' && <Section title={t('traceability.materials')}><RecordFacts record={record.record} /></Section>}
        {inStock ? (
          <Section title={t('traceability.ownershipCard')}>
            {/* Offers made before cards existed can still be cancelled here; no new one is made. */}
            {canAct && offerIsOpen(v.offer, now) && (
              <UnlockGate>
                <OfferPanel view={v} actor={session.address!} now={now}
                            send={(m) => session.submit(m, role.jeweler.business)} onChange={reload} />
              </UnlockGate>
            )}
            {v.claim && (
              <ClaimDecision jewelId={id} claim={v.claim} business={role.jeweler.business} canAct={canAct} onDone={reload} />
            )}
            <p className={v.hasCard ? '' : 'hint'}>{t(v.hasCard ? 'traceability.cardIssued' : 'traceability.cardNone')}</p>
            {v.hasCard && !printing && (() => {
              const k = keptCard(id);
              return k
                ? <KeptCard shop={{ name: role.jeweler.name, jewelerId: role.jeweler.id, business: role.jeweler.business }}
                            piece={{ jewelId: id, title: record.state === 'ok' ? record.record.title : null, inscription: v.jewel.inscription }}
                            privateKey={k} />
                : <p className="hint">{t('traceability.cardNotKept')}</p>;
            })()}
            {!canAct && <p className="hint">{t('traceability.staffOnly')}</p>}
            {canAct && !printing && !v.jewel.stolen && (
              <button type="button" className="btn" onClick={() => setPrinting(true)}>
                {t(v.hasCard ? 'traceability.cardReprint' : 'traceability.cardPrintOne')}
              </button>
            )}
            {printing && (
              <UnlockGate>
                <PrintCards shop={{ name: role.jeweler.name, jewelerId: role.jeweler.id, business: role.jeweler.business }}
                            stock={[{ jewelId: id, title: record.state === 'ok' ? record.record.title : null,
                              inscription: v.jewel.inscription, hasCard: v.hasCard, design: id }]}
                            initial={{ kind: 'one', jewelId: id }}
                            onClose={(changed) => { setPrinting(false); if (changed) reload(); }} />
              </UnlockGate>
            )}
          </Section>
        ) : (
          <p className="note">{lastChange ? t('traceability.soldOn', { date: day(lastChange) }) : t('traceability.notInStock')}</p>
        )}
        <Section title={t('traceability.benefits')}>
          <JewelBenefits view={v} active={role.active} now={now} />
          {canAct && <AttachBenefit view={v} business={role.jeweler.business} now={now} onChange={() => { reload(); reloadHead(); }} />}
        </Section>
      </main>
    </>
  );
}
