import { Link, useParams } from 'react-router-dom';
import { chainHead, readJewel, t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { OfferPanel } from '../owner/OfferPanel.tsx';
import { AppBar, day, isId, Loading, NotFound, Problem, Section, useLoad } from '../ui.tsx';
import { Hallmark, RecordFacts } from '../verify/JewelPage.tsx';
import { Gallery } from '../verify/Photos.tsx';
import { currentRecordHash, useRecord } from '../verify/record.ts';
import { AttachBenefit, JewelBenefits } from './Benefits.tsx';
import { useRole } from './role.ts';

const NO_SUCH = { ok: false as const, reason: 'not-found' as const, detail: '' };

export function AtelierJewel() {
  const { jewel: id = '' } = useParams();
  const valid = isId(id);
  const session = useSession();
  const [role] = useRole(session.address);
  const [view, reload] = useLoad(() => (valid ? readJewel(id) : Promise.resolve(NO_SUCH)), [id, valid]);
  const [head] = useLoad(() => chainHead(), []);
  const record = useRecord(view?.ok ? currentRecordHash(view.value.jewel) : null);

  const bar = <AppBar title={t('loyalty.atelierTitle')} eyebrow="" back />;
  if (!valid) return <NotFound />;
  if (!view || !head || role.state === 'loading') return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <>{bar}<main className="y-screen"><Problem outcome={view} /></main></>;
  if (!head.ok) return <>{bar}<main className="y-screen"><Problem outcome={head} /></main></>;
  if (role.state === 'failed') return <>{bar}<main className="y-screen"><Problem outcome={role.outcome} /></main></>;
  if (role.state !== 'role' || view.value.jewel.jewelerId !== role.jeweler.id) return <NotFound />;

  const v = view.value;
  const now = head.value.time;
  const inStock = v.owner === role.jeweler.business;
  const title = record.state === 'ok' ? record.record.title : t('loyalty.jewelN', { id });
  const lastChange = v.jewel.ownerSince[v.jewel.ownerSince.length - 1];
  const canAct = role.staff && role.active;

  return (
    <>
      <AppBar title={title} eyebrow={role.jeweler.name} back />
      <main className="y-screen">
        <header className="jewel__mast"><h1 className="jewel__title">{title}</h1><Hallmark code={v.jewel.inscription} /></header>
        {record.state === 'ok' && <Gallery hashes={record.record.photos} alt={title} />}
        {record.state === 'mismatch' && <div className="banner banner--bad" role="alert">{t('loyalty.recordMismatch')}</div>}
        {(record.state === 'missing' || record.state === 'unreachable' || record.state === 'unreadable') && (
          <p className="note">{t('loyalty.recordMissing')}</p>
        )}
        <div className="actions">
          <Link className="btn" to={`/j/${v.jewel.jewelerId}/${v.jewel.id}`}>{t('loyalty.seePublicPage')}</Link>
          {canAct && v.jewel.amendments.length < 50 && (
            <Link className="btn" to={`/atelier/jewel/${id}/amend`}>{t('loyalty.correctRecord')}</Link>
          )}
        </div>
        {record.state === 'ok' && <Section title={t('loyalty.materials')}><RecordFacts record={record.record} /></Section>}
        {inStock ? (
          <Section title={t('loyalty.sell')}>
            {canAct ? (
              <UnlockGate>
                <OfferPanel view={v} actor={session.address!} now={now}
                            send={(m) => session.submit(m, role.jeweler.business)} onChange={reload} />
              </UnlockGate>
            ) : <p className="hint">{t('loyalty.staffOnly')}</p>}
          </Section>
        ) : (
          <p className="note">{lastChange ? t('loyalty.soldOn', { date: day(lastChange) }) : t('loyalty.notInStock')}</p>
        )}
        <Section title={t('loyalty.benefits')}>
          <JewelBenefits view={v} active={role.active} now={now} />
          {canAct && <AttachBenefit view={v} business={role.jeweler.business} now={now} onChange={reload} />}
        </Section>
      </main>
    </>
  );
}
