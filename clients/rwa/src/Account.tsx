/**
 * Attaching an account, and the settings that go with it.
 *
 * This was a cluster of controls squeezed into the desktop header: a chip, a
 * truncated address, a disconnect button, a theme toggle and a language select,
 * all competing with the product's name for a row that was already full. On a
 * phone none of that fits, and none of it is urgent — a reader attaches an
 * account once and then never touches this screen again. So it is a tab, and
 * the header gets to carry a name.
 *
 * The two ways in stay exactly as distinct as they were, because they are not
 * the same promise:
 *
 *   CONNECT   a wallet signs. Claiming, redeeming, disputing and every market
 *             action need this.
 *   WATCH     an address is read. Nothing is signed, no key is asked for, and
 *             the address does not have to be the reader's own.
 *
 * The chip on an attached account says which of the two it is, in a word,
 * because "connected" and "watching" look identical until the moment somebody
 * presses a button that needs a signature.
 */
import { useState } from 'react';
import { LanguagePicker, t } from '@yamale/chain';

import type { Head } from './abci.ts';
import type { useAccount } from './account.ts';
import { addressOf, canSign } from './address.ts';
import { applyTheme, savedTheme, type Theme } from './App.tsx';
import { Address, ChainLine, Chip, Note, Panel } from './ui.tsx';

function ThemeChoice() {
  const [theme, setTheme] = useState<Theme>(savedTheme);

  // Three buttons rather than a cycling glyph, because this is the screen where
  // somebody comes to *set* it rather than to flick it: a control that shows
  // only its current state makes the reader press it twice to find out what the
  // other states are.
  const options: { id: Theme; label: string }[] = [
    { id: 'system', label: t('rwa.themeSystem') },
    { id: 'light', label: t('rwa.themeLight') },
    { id: 'dark', label: t('rwa.themeDark') },
  ];

  return (
    <div className="btns btns--row" role="group" aria-label={t('rwa.theme')}>
      {options.map((o) => (
        <button key={o.id} type="button"
                className={`btn btn--ghost${theme === o.id ? ' btn--on' : ''}`}
                aria-pressed={theme === o.id}
                onClick={() => { setTheme(o.id); applyTheme(o.id); }}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function Account({ api, head }: { api: ReturnType<typeof useAccount>; head: Head }) {
  const { account, wallets, connecting, problemKey } = api;
  const [typed, setTyped] = useState('');

  return (
    <main className="y-screen">
      <h1 className="y-screen__title">{t('rwa.accountTitle')}</h1>
      <p className="y-screen__lede">{t('rwa.accountLede')}</p>

      {account.mode !== 'none' ? (
        <Panel title={t('rwa.holdEyebrow')}>
          <div className="y-card y-card--money">
            <div className="y-card__head">
              <Chip tone={canSign(account) ? 'ok' : 'mute'}>
                {canSign(account) ? t('rwa.connected') : t('rwa.watching')}
              </Chip>
            </div>
            <p className="y-card__note"><Address value={addressOf(account)} /></p>
          </div>
          {/* A watching account cannot sign, and the button that would need a
              signature is on a different screen. Saying so here is cheaper than
              a disabled button with a footnote three taps away. */}
          {!canSign(account) && <Note tone="mute">{t('rwa.watchHint')}</Note>}
          <button type="button" className="btn btn--ghost btn--block" onClick={api.forget}>
            {t('action.disconnect')}
          </button>
        </Panel>
      ) : (
        <Panel title={t('rwa.attach')}>
          <p className="panel__lede">{t('rwa.attachLede')}</p>

          {wallets.length > 0 ? (
            <div className="btns">
              {wallets.map((w) => (
                <button key={w.id} type="button" className="btn btn--block"
                        disabled={connecting} onClick={() => api.connectWallet(w.id)}>
                  {connecting ? t('rwa.connecting') : `${t('action.connect')} ${w.label}`}
                </button>
              ))}
            </div>
          ) : (
            <Note tone="mute">{t('rwa.noWallet')}</Note>
          )}

          <form
            className="fields"
            onSubmit={(e) => {
              e.preventDefault();
              if (api.watch(typed)) setTyped('');
            }}
          >
            <label className="field2">
              <span className="y-label">{t('rwa.watchLabel')}</span>
              <input
                className="y-mono"
                value={typed}
                spellCheck={false}
                autoComplete="off"
                autoCapitalize="none"
                placeholder="yml1…"
                onChange={(e) => setTyped(e.target.value)}
              />
              <span className="field2__hint">{t('rwa.watchHint')}</span>
            </label>
            <button type="submit" className="btn btn--ghost btn--block">{t('rwa.watchGo')}</button>
          </form>

          {problemKey && <Note tone="bad">{t(problemKey)}</Note>}
        </Panel>
      )}

      <Panel title={t('app.appearance')}>
        <ThemeChoice />
      </Panel>

      <Panel title={t('app.language')}>
        <LanguagePicker className="select" />
      </Panel>

      <Note tone="mute">{t('rwa.footNote')}</Note>

      <ChainLine head={head} chainId />
    </main>
  );
}
