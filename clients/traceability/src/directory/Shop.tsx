/**
 * /jewelers/<id>: one shop. Who it is and how to reach it, then its portfolio:
 * the designs it has confirmed in its catalogue, photos checked against their
 * fingerprints like every photo in the app.
 */

import { useMemo, type CSSProperties } from 'react';
import { useParams } from 'react-router-dom';
import { isActive, readJeweler, t, useLocale } from '@yamale/chain';

import { readPortfolio, readProfile, type Contact, type PortfolioDesign } from '../api.ts';
import { AppBar, isId, Loading, NotFound, Problem, Section, useLoad } from '../ui.tsx';
import { VerifiedImage } from '../verify/Photos.tsx';
import { countryNamer, telLink, whatsappLink } from './directory.ts';

export function Shop() {
  const { jeweler: id = '' } = useParams();
  const valid = isId(id);
  const locale = useLocale();
  const names = useMemo(() => countryNamer(locale), [locale]);
  const [jeweler] = useLoad(() => (valid ? readJeweler(id) : Promise.resolve({ ok: false as const, reason: 'not-found' as const, detail: '' })), [id]);
  const [profile] = useLoad(() => readProfile(id), [id], (detail) => ({ state: 'failed' as const, detail }));
  const [portfolio] = useLoad(() => readPortfolio(id), [id], (detail) => ({ state: 'failed' as const, detail }));

  if (!valid) return <NotFound />;
  const bar = <AppBar title={jeweler?.ok ? jeweler.value.name : t('traceability.directoryTitle')} eyebrow={t('traceability.directoryTitle')} back />;
  if (!jeweler || !profile) return <>{bar}<main className="y-screen"><Loading /></main></>;
  if (!jeweler.ok) return jeweler.reason === 'not-found' ? <NotFound /> : <>{bar}<main className="y-screen"><Problem outcome={jeweler} /></main></>;

  const j = jeweler.value;
  const look = profile.state === 'loaded' ? profile.profile : null;
  const brand = {
    ...(look?.colour ? { '--jeweler-colour': look.colour } : {}),
    ...(look?.accent ? { '--jeweler-accent': look.accent } : {}),
  } as CSSProperties;
  const c = look?.contact;

  return (
    <>
      {bar}
      <main className="y-screen jewel" style={brand}>
        <header className="jewel__mast">
          {look?.logo ? <VerifiedImage hash={look.logo} alt={j.name} className="jewel__logo" /> : null}
          <h1 className="jewel__title">{j.name}</h1>
          {(c?.city || c?.country) && <p className="jewel__by">{[c?.city, c?.country ? names(c.country) : ''].filter(Boolean).join(', ')}</p>}
        </header>
        {!isActive(j) && <p className="banner banner--warn">{t('traceability.jewelerSuspended')}</p>}
        {c?.about && <p className="story">{c.about}</p>}
        {c && <Section title={t('traceability.contactTitle')}><ContactCard c={c} /></Section>}
        {!c && <p className="hint">{t('traceability.contactNone')}</p>}
        <Section title={t('traceability.portfolioTitle')}>
          {!portfolio && <Loading />}
          {portfolio?.state === 'failed' && <Problem outcome={{ reason: 'unreachable', detail: portfolio.detail }} />}
          {(portfolio?.state === 'none' || (portfolio?.state === 'loaded' && portfolio.designs.length === 0)) && (
            <p className="hint">{t('traceability.portfolioEmpty')}</p>
          )}
          {portfolio?.state === 'loaded' && portfolio.designs.length > 0 && (
            <ul className="portfolio">{portfolio.designs.map((d) => <Design key={d.id} d={d} />)}</ul>
          )}
        </Section>
      </main>
    </>
  );
}

function ContactCard({ c }: { c: Contact }) {
  return (
    <dl className="contactcard">
      {c.address && <><dt>{t('traceability.contact.address')}</dt><dd className="story">{c.address}</dd></>}
      {c.hours && <><dt>{t('traceability.contact.hours')}</dt><dd className="story">{c.hours}</dd></>}
      {c.phone && <><dt>{t('traceability.contact.phone')}</dt><dd><a href={telLink(c.phone)}>{c.phone}</a></dd></>}
      {c.whatsapp && <><dt>{t('traceability.contact.whatsapp')}</dt><dd><a href={whatsappLink(c.whatsapp)} rel="noopener noreferrer" target="_blank">{c.whatsapp}</a></dd></>}
      {c.email && <><dt>{t('traceability.contact.email')}</dt><dd><a href={`mailto:${c.email}`}>{c.email}</a></dd></>}
      {c.website && <><dt>{t('traceability.contact.website')}</dt><dd><a href={c.website} rel="noopener noreferrer" target="_blank">{c.website.replace(/^https:\/\//, '')}</a></dd></>}
    </dl>
  );
}

function Design({ d }: { d: PortfolioDesign }) {
  const materials = [d.fields.metal, d.fields.weightGrams ? `${d.fields.weightGrams} g` : '',
    ...d.fields.stones.map((s) => s.kind)].filter(Boolean).join(' · ');
  return (
    <li className="portfolio__item">
      {d.photos[0]
        ? <VerifiedImage hash={d.photos[0]} alt={d.title} className="portfolio__img" />
        : <span className="portfolio__img photo--absent" />}
      <span className="portfolio__title">{d.title}</span>
      {materials && <span className="hint">{materials}</span>}
      {d.productUrl && <a className="hint" href={d.productUrl} rel="noopener noreferrer" target="_blank">{t('traceability.portfolioOnSite')}</a>}
    </li>
  );
}
