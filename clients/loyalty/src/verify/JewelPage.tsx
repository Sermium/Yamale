/**
 * The public page of one jewel: what it is, where it came from, how many
 * owners it has had and when each change happened, and what it is still
 * owed. Never who owned it.
 *
 * The jeweler's own colours and logo frame the page, because the certificate
 * card in the customer's hand carries them. The register's facts below are
 * read from the chain directly, so they stand when loyalty-api does not.
 */

import { type CSSProperties, type ReactNode } from 'react';
import { useParams } from 'react-router-dom';
import {
  chainHead, isActive, plural, readJewel, readJeweler, t, type Benefit, type Jewel, type JewelView,
} from '@yamale/chain';

import { fetchProfile } from '../api.ts';
import { AppBar, day, isId, Loading, NotFound, Problem, Section, useLoad } from '../ui.tsx';
import { benefitState, ownershipSteps, type IssuerStatus } from './history.ts';
import { Gallery, VerifiedImage } from './Photos.tsx';
import { useRecord, type JewelRecord } from './record.ts';

export function Hallmark({ code }: { code: string }) {
  return <span className="hallmark">{code}</span>;
}

export function RecordFacts({ record }: { record: JewelRecord }) {
  return (
    <dl className="facts">
      <dt>{t('loyalty.metal')}</dt><dd>{record.metal}</dd>
      <dt>{t('loyalty.weight')}</dt><dd className="num">{t('loyalty.grams', { n: record.weightGrams })}</dd>
      {record.stones.length > 0 && (<>
        <dt>{t('loyalty.stones')}</dt>
        <dd>{record.stones.map((s, i) => (
          <div key={i}>{t('loyalty.stoneLine', { count: s.count, kind: s.kind, carat: s.carat })}</div>
        ))}</dd>
      </>)}
      <dt>{t('loyalty.workshop')}</dt><dd>{record.origin.workshop}</dd>
      {record.origin.stoneSource && (<><dt>{t('loyalty.stoneSource')}</dt><dd>{record.origin.stoneSource}</dd></>)}
      {record.origin.labCertificate && (<><dt>{t('loyalty.labCertificate')}</dt><dd>{record.origin.labCertificate}</dd></>)}
    </dl>
  );
}

const KNOWN_KINDS = ['cleaning', 'resizing', 'polishing', 'rhodium', 'repair'];

export function benefitKind(kind: string): string {
  return KNOWN_KINDS.includes(kind) ? t(`loyalty.benefitKind.${kind}`) : kind;
}

export function BenefitList({ benefits, issuer, now }: { benefits: Benefit[]; issuer: IssuerStatus; now: number }) {
  if (benefits.length === 0) return <p className="hint">{t('loyalty.noBenefits')}</p>;
  return (
    <ul className="y-cards">
      {benefits.map((b) => {
        const s = benefitState(b, issuer, now);
        return (
          <li key={b.id} className="y-card benefit">
            <span className="benefit__kind">{benefitKind(b.kind)}</span>
            {s.state === 'active' && (
              <span className="benefit__state">
                <span className="y-chip y-chip--ok num">{t('loyalty.benefitLeft', { left: s.left, allowance: b.allowance })}</span>
                {s.resetsAt && <span className="hint">{t('loyalty.benefitRenews', { date: day(s.resetsAt) })}</span>}
                {s.expiresAt && <span className="hint">{t('loyalty.benefitUntil', { date: day(s.expiresAt) })}</span>}
              </span>
            )}
            {s.state === 'expired' && <span className="y-chip y-chip--mute">{t('loyalty.benefitExpired', { date: day(s.expiredAt) })}</span>}
            {s.state === 'withdrawn' && <span className="y-chip y-chip--warn">{t('loyalty.benefitWithdrawn')}</span>}
          </li>
        );
      })}
    </ul>
  );
}

function Owners({ jewel }: { jewel: Jewel }) {
  const steps = ownershipSteps(jewel);
  return (
    <>
      <p className="owners__count">
        {jewel.ownerCount === 0 ? t('loyalty.noOwnerYet') : plural('loyalty.owners', jewel.ownerCount, { count: jewel.ownerCount })}
      </p>
      <ol className="timeline">
        {steps.map((s) => (
          <li key={s.ordinal} className="timeline__step">
            <span>{s.kind === 'stock' ? t('loyalty.stepStock') : t('loyalty.stepOwner', { n: s.ordinal })}</span>
            <time className="num">{day(s.at)}</time>
          </li>
        ))}
      </ol>
      <p className="hint">{t('loyalty.ownersPrivate')}</p>
    </>
  );
}

function Corrections({ jewel }: { jewel: Jewel }) {
  if (jewel.amendments.length === 0) return null;
  return (
    <Section title={t('loyalty.corrections')}>
      <ol className="timeline">
        {jewel.amendments.map((a, i) => (
          <li key={i} className="timeline__step">
            <span>{t('loyalty.correctedOn', { date: day(a.time) })}</span>
            <span className="correction__note">{a.note}</span>
          </li>
        ))}
      </ol>
      <details className="original">
        <summary>{t('loyalty.originalRecord')}</summary>
        <OriginalRecord jewel={jewel} />
      </details>
    </Section>
  );
}

function OriginalRecord({ jewel }: { jewel: Jewel }) {
  const r = useRecord(jewel, 'original');
  if (r.state === 'ok') return <RecordFacts record={r.record} />;
  if (r.state === 'mismatch') return <p className="banner banner--bad" role="alert">{t('loyalty.recordMismatch')}</p>;
  return <p className="hint">{t('loyalty.recordMissing')}</p>;
}

const NO_SUCH = { ok: false as const, reason: 'not-found' as const, detail: '' };

/** `actions` lets Task 8 add a signed-in action without forking this page. */
export function JewelPage({ actions }: { actions?: (view: JewelView, now: number, reload: () => void) => ReactNode } = {}) {
  const { jeweler: jewelerId = '', jewel: jewelId = '' } = useParams();
  const valid = isId(jewelerId) && isId(jewelId);
  const [view, reload] = useLoad(() => (valid ? readJewel(jewelId) : Promise.resolve(NO_SUCH)), [jewelId, valid]);
  const [jeweler] = useLoad(() => (valid ? readJeweler(jewelerId) : Promise.resolve(NO_SUCH)), [jewelerId, valid]);
  const [head] = useLoad(() => chainHead(), []);
  const [profile] = useLoad(() => (valid ? fetchProfile(jewelerId) : Promise.resolve(null)), [jewelerId, valid], () => null);
  const record = useRecord(view?.ok ? view.value.jewel : null);

  if (!valid) return <NotFound />;
  if (!view || !jeweler) return <><AppBar title={t('loyalty.title')} eyebrow="" back /><main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <><AppBar title={t('loyalty.title')} eyebrow="" back /><main className="y-screen"><Problem outcome={view} /></main></>;
  const { jewel } = view.value;
  if (jewel.jewelerId !== jewelerId) return <NotFound />;

  const now = head?.ok ? head.value.time : Math.floor(Date.now() / 1000);
  const issuer = jeweler.ok ? jeweler.value : null;
  const issuerStatus: IssuerStatus = issuer ? (isActive(issuer) ? 'active' : 'suspended') : 'unknown';
  const brand = {
    ...(profile?.colour ? { '--jeweler-colour': profile.colour } : {}),
    ...(profile?.accent ? { '--jeweler-accent': profile.accent } : {}),
  } as CSSProperties;
  // useRecord has already checked the record names this jewel's inscription and jeweler.
  const title = record.state === 'ok' ? record.record.title : t('loyalty.jewelN', { id: jewel.id });

  return (
    <>
      <AppBar title={issuer?.name ?? t('loyalty.title')} eyebrow={t('loyalty.title')} back />
      <main className="y-screen jewel" style={brand}>
        <header className="jewel__mast">
          {profile?.logo ? <VerifiedImage hash={profile.logo} alt={issuer?.name ?? ''} className="jewel__logo" /> : null}
          <p className="jewel__by">{t('loyalty.registeredBy', { name: issuer?.name ?? t('loyalty.jewelerN', { id: jewelerId }) })}</p>
          <h1 className="jewel__title">{title}</h1>
          <Hallmark code={jewel.inscription} />
        </header>

        {jewel.stolen && (
          <div className="banner banner--bad" role="alert">
            <strong>{t('loyalty.stolen')}</strong>
            <span>{t('loyalty.stolenNote')}</span>
          </div>
        )}
        {view.value.reassignment && (
          <div className="banner banner--warn" role="status">
            {t('loyalty.reassignment', { date: day(view.value.reassignment.windowEnds) })}
          </div>
        )}

        {record.state === 'ok' && <Gallery hashes={record.record.photos} alt={title} />}
        {record.state === 'mismatch' && <div className="banner banner--bad" role="alert">{t('loyalty.recordMismatch')}</div>}
        {(record.state === 'missing' || record.state === 'unreachable' || record.state === 'unreadable') && (
          <p className="note">{t('loyalty.recordMissing')}</p>
        )}

        <p className="declared">{t('loyalty.declared')}</p>

        {head?.ok && actions?.(view.value, head.value.time, reload)}

        {record.state === 'ok' && (<>
          <Section title={t('loyalty.materials')}><RecordFacts record={record.record} /></Section>
        </>)}
        <Section title={t('loyalty.ownersTitle')}><Owners jewel={jewel} /></Section>
        <Section title={t('loyalty.benefits')}>
          {jeweler.ok ? null : <Problem outcome={jeweler} />}
          <BenefitList benefits={view.value.benefits} issuer={issuerStatus} now={now} />
        </Section>
        <Corrections jewel={jewel} />

        <details className="advanced">
          <summary>{t('loyalty.advanced')}</summary>
          <pre>{JSON.stringify({
            chain: head?.ok ? head.value.chainId : null, readAtHeight: view.height,
            classId: `jewel-${jewel.jewelerId}`, tokenId: `jewel-${jewel.id}`,
            owner: view.value.owner, jewel, offer: view.value.offer ?? null,
            reassignment: view.value.reassignment ?? null, benefits: view.value.benefits,
          }, null, 2)}</pre>
        </details>
      </main>
    </>
  );
}
