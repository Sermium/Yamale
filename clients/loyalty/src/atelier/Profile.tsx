import { useEffect, useState } from 'react';
import { t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { fetchProfile, saveProfile } from '../api.ts';
import { CHAIN_ID } from '../config.ts';
import { AppBar, Loading, Problem, Section } from '../ui.tsx';
import { Hallmark } from '../verify/JewelPage.tsx';
import { VerifiedImage } from '../verify/Photos.tsx';
import { FailureNote } from './FailureNote.tsx';
import { useRole } from './role.ts';
import { uploadLogo } from './uploads.ts';

type Note = { text: string; raw: string } | null;

export function Profile() {
  const session = useSession();
  const [role] = useRole(session.address);
  const [logo, setLogo] = useState('');
  const [colour, setColour] = useState('#12253f');
  const [accent, setAccent] = useState('#a87b3c');
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [failed, setFailed] = useState<Note>(null);
  const id = role.state === 'role' ? role.jeweler.id : null;

  useEffect(() => {
    if (!id) return;
    let live = true;
    fetchProfile(id).then((p) => { if (live && p) { setLogo(p.logo); setColour(p.colour); setAccent(p.accent); } });
    return () => { live = false; };
  }, [id]);

  const bar = (eyebrow: string) => <AppBar title={t('loyalty.lookTitle')} eyebrow={eyebrow} back />;
  if (role.state === 'loading') return <>{bar('')}<main className="y-screen"><Loading /></main></>;
  if (role.state === 'failed') return <>{bar('')}<main className="y-screen"><Problem outcome={role.outcome} /></main></>;
  if (role.state !== 'role' || !role.business) {
    return <>{bar('')}<main className="y-screen"><p className="note">{t('loyalty.businessOnly')}</p></main></>;
  }
  const jeweler = role.jeweler;

  async function chooseLogo(file: File | undefined) {
    if (!file) return;
    setBusy(true); setSaved(false); setFailed(null);
    try {
      const up = await uploadLogo(file, session.withKey);
      if (up.ok) setLogo(up.hash); else setFailed({ text: up.text, raw: up.raw });
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    setBusy(true); setSaved(false); setFailed(null);
    try {
      const out = await saveProfile(session.withKey((k) => k), jeweler.id, { logo, colour, accent }, CHAIN_ID);
      if (out.ok) setSaved(true);
      else setFailed({ text: t('loyalty.lookFailed'), raw: `${out.status} ${out.error}`.trim() });
    } catch (e) {
      setFailed({ text: t('loyalty.lookFailed'), raw: e instanceof Error ? e.message : String(e) });
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
            <p className="jewel__by">{t('loyalty.registeredBy', { name: jeweler.name })}</p>
            <Hallmark code="MD20260001" />
          </div>
          <Section title={t('loyalty.logo')}>
            <input type="file" accept="image/*" aria-label={t('loyalty.logo')} disabled={busy}
                   onChange={(e) => { void chooseLogo(e.target.files?.[0]); e.target.value = ''; }} />
          </Section>
          <Section title={t('loyalty.colours')}>
            <label className="y-label">{t('loyalty.colourMain')} <input type="color" value={colour} onChange={(e) => { setColour(e.target.value); setSaved(false); }} /></label>
            <label className="y-label">{t('loyalty.colourAccent')} <input type="color" value={accent} onChange={(e) => { setAccent(e.target.value); setSaved(false); }} /></label>
          </Section>
          {failed && <FailureNote text={failed.text} raw={failed.raw} />}
          {saved && <p className="banner banner--ok" role="status">{t('loyalty.lookSaved')}</p>}
          <button type="button" className="btn btn--primary" disabled={busy} onClick={() => void save()}>{t('loyalty.saveLook')}</button>
        </UnlockGate>
      </main>
    </>
  );
}
