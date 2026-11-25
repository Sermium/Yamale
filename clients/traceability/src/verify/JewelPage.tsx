/**
 * The public page of one jewel: what it is, where it came from, how many
 * owners it has had and when each change happened, and what it is still
 * owed. Never who owned it.
 *
 * The jeweler's own colours and logo frame the page, because the certificate
 * card in the customer's hand carries them. The register's facts below are
 * read from the chain directly, so they stand when traceability-api does not.
 */

import { type CSSProperties, type ReactNode } from 'react';
import { Link, useParams } from 'react-router-dom';
import {
  chainHead, isActive, plural, readJewel, readJeweler, t, useLocale, type Benefit, type Jewel, type JewelView,
} from '@yamale/chain';

import { fetchProfile } from '../api.ts';
import { AppBar, day, isId, Loading, NotFound, Problem, Section, useLoad } from '../ui.tsx';
import { benefitState, ownershipSteps, type IssuerStatus } from './history.ts';
import { listPriceDay, listPriceText } from './listprice.ts';
import { Gallery, VerifiedImage } from './Photos.tsx';
import { stoneLineKey, useRecord, type JewelRecord } from './record.ts';

export function Hallmark({ code }: { code: string }) {
  return <span className="hallmark">{code}</span>;
}

export function RecordFacts({ record }: { record: JewelRecord }) {
  return (
    <dl className="facts">
      <dt>{t('traceability.metal')}</dt><dd>{record.metal}</dd>
      <dt>{t('traceability.weight')}</dt><dd className="num">{t('traceability.grams', { n: record.weightGrams })}</dd>
      {record.stones.length > 0 && (<>
        <dt>{t('traceability.stones')}</dt>
        <dd>{record.stones.map((s, i) => (
          <div key={i}>{t(stoneLineKey(s), { count: s.count, kind: s.kind, carat: s.carat })}</div>
        ))}</dd>
      </>)}
      <dt>{t('traceability.workshop')}</dt><dd>{record.origin.workshop}</dd>
      {record.origin.stoneSource && (<><dt>{t('traceability.stoneSource')}</dt><dd>{record.origin.stoneSource}</dd></>)}
      {record.origin.labCertificate && (<><dt>{t('traceability.labCertificate')}</dt><dd>{record.origin.labCertificate}</dd></>)}
    </dl>
  );
}

/** What a catalogue-registered jewel adds: the jeweler's words, a dated list price, a link. */
export function DesignStory({ record }: { record: JewelRecord }) {
  const locale = useLocale();
  if (record.version !== 2) return null;
  return (
    <div className="stack">
      {record.description && <p className="story">{record.description}</p>}
      {record.listPrice && (
        <p className="hint num">{t('traceability.listPriceAt', {
          price: listPriceText(record.listPrice, locale), date: listPriceDay(record.listPrice.at, locale),
        })}</p>
      )}
      {record.productUrl && (
        <a className="btn" href={record.productUrl} target="_blank" rel="noopener noreferrer nofollow">{t('traceability.designLink')}</a>
      )}
    </div>
  );
}

const KNOWN_KINDS = ['cleaning', 'resizing', 'polishing', 'rhodium', 'repair'];

export function benefitKind(kind: string): string {
  return KNOWN_KINDS.includes(kind) ? t(`traceability.benefitKind.${kind}`) : kind;
}

export function BenefitList({ benefits, issuer, now }: { benefits: Benefit[]; issuer: IssuerStatus; now: number }) {
  if (benefits.length === 0) return <p className="hint">{t('traceability.noBenefits')}</p>;
  return (
    <ul className="y-cards">
      {benefits.map((b) => {
        const s = benefitState(b, issuer, now);
        return (
          <li key={b.id} className="y-card benefit">
            <span className="benefit__kind">{benefitKind(b.kind)}</span>
            {s.state === 'active' && (
              <span className="benefit__state">
                <span className="y-chip y-chip--ok num">{t('traceability.benefitLeft', { left: s.left, allowance: b.allowance })}</span>
                {s.resetsAt && <span className="hint">{t('traceability.benefitRenews', { date: day(s.resetsAt) })}</span>}
                {s.expiresAt && <span className="hint">{t('traceability.benefitUntil', { date: day(s.expiresAt) })}</span>}
              </span>
            )}
            {s.state === 'expired' && <span className="y-chip y-chip--mute">{t('traceability.benefitExpired', { date: day(s.expiredAt) })}</span>}
            {s.state === 'withdrawn' && <span className="y-chip y-chip--warn">{t('traceability.benefitWithdrawn')}</span>}
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
        {jewel.ownerCount === 0 ? t('traceability.noOwnerYet') : plural('traceability.owners', jewel.ownerCount, { count: jewel.ownerCount })}
      </p>
      <ol className="timeline">
        {steps.map((s) => (
          <li key={s.ordinal} className="timeline__step">
            <span>{s.kind === 'stock' ? t('traceability.stepStock') : t('traceability.stepOwner', { n: s.ordinal })}</span>
            <time className="num">{day(s.at)}</time>
          </li>
        ))}
      </ol>
      <p className="hint">{t('traceability.ownersPrivate')}</p>
    </>
  );
}

function Corrections({ jewel }: { jewel: Jewel }) {
  if (jewel.amendments.length === 0) return null;
  return (
    <Section title={t('traceability.corrections')}>
      <ol className="timeline">
        {jewel.amendments.map((a, i) => (
          <li key={i} className="timeline__step">
            <span>{t('traceability.correctedOn', { date: day(a.time) })}</span>
            <span className="correction__note">{a.note}</span>
          </li>
        ))}
      </ol>
      <details className="original">
        <summary>{t('traceability.originalRecord')}</summary>
        <OriginalRecord jewel={jewel} />
      </details>
    </Section>
  );
}

function OriginalRecord({ jewel }: { jewel: Jewel }) {
  const r = useRecord(jewel, 'original');
  if (r.state === 'ok') return <RecordFacts record={r.record} />;
  if (r.state === 'mismatch') return <p className="banner banner--bad" role="alert">{t('traceability.recordMismatch')}</p>;
  return <p className="hint">{t('traceability.recordMissing')}</p>;
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
  if (!view || !jeweler) return <><AppBar title={t('traceability.title')} eyebrow="" back /><main className="y-screen"><Loading /></main></>;
  if (!view.ok) return <><AppBar title={t('traceability.title')} eyebrow="" back /><main className="y-screen"><Problem outcome={view} /></main></>;
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
  const title = record.state === 'ok' ? record.record.title : t('traceability.jewelN', { id: jewel.id });

  return (
    <>
      <AppBar title={issuer?.name ?? t('traceability.title')} eyebrow={t('traceability.title')} back />
      <main className="y-screen jewel" style={brand}>
        <header className="jewel__mast">
          {profile?.logo ? <VerifiedImage hash={profile.logo} alt={issuer?.name ?? ''} className="jewel__logo" /> : null}
          <p className="jewel__by">
            <Link to={`/jewelers/${jewelerId}`}>{t('traceability.registeredBy', { name: issuer?.name ?? t('traceability.jewelerN', { id: jewelerId }) })}</Link>
          </p>
          <h1 className="jewel__title">{title}</h1>
          <Hallmark code={jewel.inscription} />
        </header>

        {jewel.stolen && (
          <div className="banner banner--bad" role="alert">
            <strong>{t('traceability.stolen')}</strong>
            <span>{t('traceability.stolenNote')}</span>
          </div>
        )}
        {view.value.claim && !jewel.stolen && (
          <div className="banner banner--warn" role="status">{t('traceability.claimPendingPublic')}</div>
        )}
        {view.value.reassignment && (
          <div className="banner banner--warn" role="status">
            {t('traceability.reassignment', { date: day(view.value.reassignment.windowEnds) })}
          </div>
        )}

        {record.state === 'ok' && <Gallery hashes={record.record.photos} alt={title} />}
        {record.state === 'mismatch' && <div className="banner banner--bad" role="alert">{t('traceability.recordMismatch')}</div>}
        {(record.state === 'missing' || record.state === 'unreachable' || record.state === 'unreadable') && (
          <p className="note">{t('traceability.recordMissing')}</p>
        )}

        <p className="declared">{t('traceability.declared')}</p>

        {head?.ok && actions?.(view.value, head.value.time, reload)}

        {record.state === 'ok' && (<>
          {record.record.version === 2 && <Section title={t('traceability.aboutDesign')}><DesignStory record={record.record} /></Section>}
          <Section title={t('traceability.materials')}><RecordFacts record={record.record} /></Section>
        </>)}
        <Section title={t('traceability.ownersTitle')}><Owners jewel={jewel} /></Section>
        <Section title={t('traceability.benefits')}>
          {jeweler.ok ? null : <Problem outcome={jeweler} />}
          <BenefitList benefits={view.value.benefits} issuer={issuerStatus} now={now} />
        </Section>
        <Corrections jewel={jewel} />

        <details className="advanced">
          <summary>{t('traceability.advanced')}</summary>
          <pre>{JSON.stringify({
            chain: head?.ok ? head.value.chainId : null, readAtHeight: view.height,
            classId: `jewel-${jewel.jewelerId}`, tokenId: `jewel-${jewel.id}`,
            owner: view.value.owner, jewel, offer: view.value.offer ?? null,
            reassignment: view.value.reassignment ?? null, benefits: view.value.benefits,
            hasCard: view.value.hasCard, claim: view.value.claim ?? null,
          }, null, 2)}</pre>
        </details>
      </main>
    </>
  );
}
