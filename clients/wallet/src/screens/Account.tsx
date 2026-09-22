/**
 * The account on this device, and everything about how it is held.
 *
 * The desktop wallet put all of this in a dropdown behind a badge in the
 * masthead — the account's name, its user ID, the language, locking, and the
 * irreversible delete, stacked inside one popover. On a phone there is no
 * masthead to hang a popover from, and stacking a destructive control under a
 * daily one was never right anyway.
 *
 * The first section is deliberately called "how you are attached", not "your
 * key". It is the seam this product is about to change: accounts are moving to
 * an email and a password with no phrase, and the device will sign alone. When
 * that lands it is a different value in this section, and a different row on
 * the attach screen — not a new information architecture.
 */
import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  LanguagePicker,
  displayName,
  formatUserId,
  getContact,
  saveContact,
  t,
} from '@yamale/chain';

import { client } from '../chain.ts';
import { Identifier } from '../Identifier.tsx';
import { ClaimUserId } from '../ClaimUserId.tsx';
import { destroyVault, getUnlocked, lock, vaultSummary } from '../vault.ts';
import { Field, Note, ScreenTitle } from '../ui.tsx';
import { Placement } from './Attach.tsx';
import { Protection } from './Protection.tsx';

type Theme = 'light' | 'dark' | 'system';

const THEME_KEY = 'yamale.wallet.theme';

export function savedTheme(): Theme {
  try {
    const v = localStorage.getItem(THEME_KEY);
    if (v === 'light' || v === 'dark') return v;
  } catch { /* private mode */ }
  return 'system';
}

export function AccountScreen({ onDetached, onAttach }: {
  onDetached: () => void;
  onAttach: () => void;
}) {
  const [account, setAccount] = useState(() => vaultSummary());
  const [naming, setNaming] = useState(false);
  const [draft, setDraft] = useState('');
  const [theme, setTheme] = useState<Theme>(savedTheme);
  // Re-read on a tick so the padlock in this screen agrees with the vault's
  // own auto-lock timer, which fires without telling anybody.
  const [, bump] = useState(0);
  useEffect(() => {
    const id = setInterval(() => bump((n) => n + 1), 5000);
    return () => clearInterval(id);
  }, []);

  const userId = useQuery({
    queryKey: ['userId', account?.address],
    queryFn: () => client.userIdOf(account!.address),
    enabled: Boolean(account?.address),
  });
  const placement = useQuery({
    queryKey: ['jurisdiction', account?.address],
    queryFn: () => client.jurisdictionOf(account!.address),
    enabled: Boolean(account?.address),
  });

  function applyTheme(next: Theme) {
    setTheme(next);
    if (next === 'system') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.setAttribute('data-theme', next);
    try {
      if (next === 'system') localStorage.removeItem(THEME_KEY);
      else localStorage.setItem(THEME_KEY, next);
    } catch { /* private mode */ }
  }

  if (!account) {
    return (
      <>
        <ScreenTitle lede={t('wal.noAccountHere')}>{t('wal.tabAccount')}</ScreenTitle>
        <button type="button" className="wal-btn" onClick={onAttach}>{t('wal.attachTitle')}</button>
        <Preferences theme={theme} onTheme={applyTheme} />
      </>
    );
  }

  // The lookup answers for this one account and nothing else. A closure that
  // answered for any address would be a directory, and the chain deliberately
  // offers none.
  const shown = displayName(account.address, (a) =>
    a === account.address ? (userId.data ?? undefined) : undefined);
  const contact = getContact(account.address);
  const unlocked = Boolean(getUnlocked());

  return (
    <>
      <ScreenTitle>{t('wal.tabAccount')}</ScreenTitle>

      <div className="y-card y-card--money">
        <span className="wal-fact__label">
          {shown.kind === 'pseudonym' ? t('wal.tierYours')
            : shown.kind === 'userId' ? t('wal.tierChain')
            : t('wal.tierNone')}
        </span>
        <p className="wal-idBig">{shown.label}</p>

        {naming ? (
          <form onSubmit={(e) => {
            e.preventDefault();
            if (draft.trim()) {
              saveContact({ address: account.address, pseudonym: draft.trim() });
              setNaming(false);
              setDraft('');
              setAccount(vaultSummary());
            }
          }}>
            <Field label={t('wal.callIt')}>
              <input className="wal-input" value={draft} onChange={(e) => setDraft(e.target.value)} autoFocus />
            </Field>
            <button type="submit" className="wal-btn wal-btn--ghost">{t('wal.save')}</button>
          </form>
        ) : (
          <button type="button" className="wal-chip"
                  onClick={() => { setDraft(contact?.pseudonym ?? ''); setNaming(true); }}>
            {contact ? t('wal.rename') : t('wal.giveItAName')}
          </button>
        )}
      </div>

      <h2 className="y-screen__sub">{t('watch.address')}</h2>
      <div className="y-card"><Identifier value={account.address} name={false} /></div>

      <h2 className="y-screen__sub">{t('wal.yourUserId')}</h2>
      <div className="y-card">
        {userId.isPending ? (
          <p className="wal-muted">{t('acct.reading')}</p>
        ) : userId.data ? (
          <p className="wal-idBig y-mono">{formatUserId(userId.data)}</p>
        ) : (
          <ClaimUserId address={account.address} label={account.label} />
        )}
      </div>

      {/* ---------------------------------------------- how you are attached */}

      <h2 className="y-screen__sub">{t('wal.howAttached')}</h2>
      <div className="y-card">
        <p className="wal-row__strong">{t('wal.attachedPhrase')}</p>
        <p className="wal-small wal-muted">{t('wal.attachedPhraseBody')}</p>
        <p className={unlocked ? 'wal-lockState wal-lockState--open' : 'wal-lockState'}>
          {unlocked ? t('wal.unlockedNow') : t('wal.lockedNow')}
        </p>
        {unlocked && (
          <button type="button" className="wal-btn wal-btn--ghost" onClick={() => { lock(); bump((n) => n + 1); }}>
            {t('wal.lockNow')}
          </button>
        )}
      </div>

      {/* Above placement, and above preferences, because it answers a bigger
          question than either: what happens to this account if this phone goes
          in a river. An account that cannot be recovered should say so on the
          screen that is about how the account is held. */}
      <Protection address={account.address} />

      {/* The placement request, offered only where it is still needed. An
          account the chain has already placed does not want a form asking it
          to be placed. */}
      {placement.isSuccess && placement.data === null && <Placement address={account.address} />}

      <Preferences theme={theme} onTheme={applyTheme} />

      {/* ------------------------------------------------------------ danger */}

      <h2 className="y-screen__sub">{t('wal.replaceOrRemove')}</h2>
      <button type="button" className="y-row" onClick={onAttach}>
        <span className="y-row__main">
          <span className="y-row__title">{t('wal.loadDifferent')}</span>
          <span className="y-row__sub">{t('wal.loadDifferentSub')}</span>
        </span>
      </button>

      {/* Behind a disclosure rather than beside the everyday controls. A
          destructive action sitting next to a daily one is how people destroy
          things by accident. */}
      <details className="wal-danger">
        <summary>{t('wal.removeFromBrowser')}</summary>
        <Note tone="bad" title={t('wal.removeTitle')}>{t('wal.removeBody')}</Note>
        <button type="button" className="wal-btn wal-btn--danger"
                onClick={() => { destroyVault(); lock(); setAccount(null); onDetached(); }}>
          {t('wal.removeConfirm')}
        </button>
      </details>
    </>
  );
}

function Preferences({ theme, onTheme }: { theme: Theme; onTheme: (t: Theme) => void }) {
  return (
    <>
      <h2 className="y-screen__sub">{t('wal.preferences')}</h2>
      <div className="y-card">
        <Field label={t('nav.language')}>
          <LanguagePicker className="wal-input" />
        </Field>

        {/*
          Three states, not a switch. "System" is a choice in its own right — it
          keeps following the phone as the phone dims itself in the evening —
          and collapsing it into a two-way toggle takes that behaviour away from
          everybody who never opens this screen.
        */}
        <span className="wal-field__label">{t('rwa.theme')}</span>
        <div className="wal-seg" role="group" aria-label={t('rwa.theme')}>
          {([
            ['light', t('rwa.themeLight')],
            ['dark', t('rwa.themeDark')],
            ['system', t('rwa.themeSystem')],
          ] as const).map(([id, label]) => (
            <button key={id} type="button"
                    className={theme === id ? 'wal-seg__btn wal-seg__btn--on' : 'wal-seg__btn'}
                    aria-pressed={theme === id}
                    onClick={() => onTheme(id)}>
              {label}
            </button>
          ))}
        </div>
      </div>
    </>
  );
}
