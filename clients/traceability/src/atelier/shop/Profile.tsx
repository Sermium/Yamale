import { useEffect, useRef, useState } from 'react';
import { t } from '@yamale/chain';

import { UnlockGate } from '../../account/Account.tsx';
import { useSession } from '../../account/session.tsx';
import { readProfile, saveProfile, type Contact } from '../../api.ts';
import { CHAIN_ID } from '../../config.ts';
import { AppBar, Loading, Problem, Section } from '../../ui.tsx';
import { Hallmark } from '../../verify/JewelPage.tsx';
import { VerifiedImage } from '../../verify/Photos.tsx';
import { FailureNote } from '../shared/FailureNote.tsx';
import { canSaveLook, mayApplyLoad, type LookLoad } from '../shared/rules.ts';
import { useRole } from '../shared/role.ts';
import { uploadLogo } from '../shared/uploads.ts';

type Note = { text: string; raw: string } | null;

export function Profile() {
  const session = useSession();
  const [role] = useRole(session.address);
  const [logo, setLogo] = useState('');
  const [colour, setColour] = useState('#12253f');
  const [accent, setAccent] = useState('#a87b3c');
  const [contact, setContact] = useState<Contact>({});
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [failed, setFailed] = useState<Note>(null);
  const [load, setLoad] = useState<LookLoad>('loading');
  const [loadDetail, setLoadDetail] = useState('');
  // Once the user has chosen something, a late read of the stored look must not overwrite it.
  const edited = useRef(false);
  const id = role.state === 'role' ? role.jeweler.id : null;

  useEffect(() => {
    if (!id) return;
    let live = true;
    setLoad('loading');
    readProfile(id).then((r) => {
      if (!live) return;
      if (r.state === 'failed') { setLoadDetail(r.detail); setLoad('failed'); return; }
      if (r.state === 'loaded' && mayApplyLoad(edited.current)) { setLogo(r.profile.logo); setColour(r.profile.colour); setAccent(r.profile.accent); setContact(r.profile.contact ?? {}); }
      setLoad(r.state);
    });
    return () => { live = false; };
  }, [id]);

  const bar = (eyebrow: string) => <AppBar title={t('traceability.lookTitle')} eyebrow={eyebrow} back />;
  if (role.state === 'loading') return <>{bar('')}<main className="y-screen"><Loading /></main></>;
  if (role.state === 'failed') return <>{bar('')}<main className="y-screen"><Problem outcome={role.outcome} /></main></>;
  if (role.state !== 'role' || !role.business) {
    return <>{bar('')}<main className="y-screen"><p className="note">{t('traceability.businessOnly')}</p></main></>;
  }
  const jeweler = role.jeweler;
  if (load === 'failed') {
    return <>{bar(jeweler.name)}<main className="y-screen"><Problem outcome={{ reason: 'unreachable', detail: loadDetail }} /></main></>;
  }
  if (load === 'loading') return <>{bar(jeweler.name)}<main className="y-screen"><Loading /></main></>;

  async function chooseLogo(file: File | undefined) {
    if (!file) return;
    edited.current = true;
    setBusy(true); setSaved(false); setFailed(null);
    try {
      const up = await uploadLogo(file, session.withKey);
      if (up.ok) setLogo(up.hash); else setFailed({ text: up.text, raw: up.raw });
    } catch (e) {
      setFailed({ text: t('traceability.interrupted'), raw: e instanceof Error ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    setBusy(true); setSaved(false); setFailed(null);
    try {
      const out = await saveProfile(session.withKey((k) => k), jeweler.id, { logo, colour, accent, contact }, CHAIN_ID);
      if (out.ok) setSaved(true);
      else setFailed({ text: t('traceability.lookFailed'), raw: `${out.status} ${out.error}`.trim() });
    } catch (e) {
      setFailed({ text: t('traceability.lookFailed'), raw: e instanceof Error ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      {bar(jeweler.name)}
      <main className="y-screen">
        <UnlockGate>
          <div className="jewel__mast" style={{ ['--jeweler-colour' as string]: colour, ['--jeweler-accent' as string]: accent }}>
            {logo && <VerifiedImage hash={logo} alt={jeweler.name} className="jewel__logo" />}
            <p className="jewel__by">{t('traceability.registeredBy', { name: jeweler.name })}</p>
            <Hallmark code="MD20260001" />
          </div>
          <Section title={t('traceability.logo')}>
            <input type="file" accept="image/*" aria-label={t('traceability.logo')} disabled={busy}
                   onChange={(e) => { void chooseLogo(e.target.files?.[0]); e.target.value = ''; }} />
          </Section>
          <Section title={t('traceability.colours')}>
            <label className="y-label">{t('traceability.colourMain')} <input type="color" value={colour} onChange={(e) => { edited.current = true; setColour(e.target.value); setSaved(false); }} /></label>
            <label className="y-label">{t('traceability.colourAccent')} <input type="color" value={accent} onChange={(e) => { edited.current = true; setAccent(e.target.value); setSaved(false); }} /></label>
          </Section>
          <Section title={t('traceability.contactTitle')}>
            <p className="hint">{t('traceability.contactHint')}</p>
            <ContactFields value={contact} onChange={(c) => { edited.current = true; setContact(c); setSaved(false); }} />
          </Section>
          {failed && <FailureNote text={failed.text} raw={failed.raw} />}
          {saved && <p className="banner banner--ok" role="status">{t('traceability.lookSaved')}</p>}
          <button type="button" className="btn btn--primary" disabled={!canSaveLook(load, busy)} onClick={() => void save()}>{t('traceability.saveLook')}</button>
        </UnlockGate>
      </main>
    </>
  );
}

/** The shop's public contact details, every one optional. */
function ContactFields({ value, onChange }: { value: Contact; onChange: (c: Contact) => void }) {
  const field = (k: keyof Contact, opts: { area?: boolean; type?: string; max: number; mode?: 'tel' | 'email' | 'url' }) => (
    <label className="y-label" key={k}>
      {t(`traceability.contact.${k}`)}
      {opts.area
        ? <textarea className="bigfield" rows={3} maxLength={opts.max} value={value[k] ?? ''} onChange={(e) => onChange({ ...value, [k]: e.target.value })} />
        : <input className="bigfield" type={opts.type ?? 'text'} inputMode={opts.mode} maxLength={opts.max} value={value[k] ?? ''}
                 onChange={(e) => onChange({ ...value, [k]: e.target.value })} />}
    </label>
  );
  return (
    <div className="stack">
      {field('about', { area: true, max: 600 })}
      {field('address', { area: true, max: 200 })}
      {field('city', { max: 80 })}
      {field('country', { max: 2 })}
      {field('hours', { area: true, max: 200 })}
      {field('phone', { type: 'tel', mode: 'tel', max: 30 })}
      {field('whatsapp', { type: 'tel', mode: 'tel', max: 30 })}
      {field('email', { type: 'email', mode: 'email', max: 254 })}
      {field('website', { type: 'url', mode: 'url', max: 200 })}
    </div>
  );
}
