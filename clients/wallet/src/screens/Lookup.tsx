/**
 * Looking up any account.
 *
 * The harmless half of a wallet, and the half most people arrive wanting: they
 * are checking whether money landed, not creating a key. It needs nothing from
 * them, stores nothing, and signs nothing — so it is a tab of its own rather
 * than something reached past a screen offering to generate a phrase.
 */
import { useState } from 'react';
import { t, truncateAddress } from '@yamale/chain';

import { Holdings } from '../Holdings.tsx';
import { Identifier } from '../Identifier.tsx';
import { Back, Note, ScreenTitle } from '../ui.tsx';

/** Recent lookups, this tab only. Not persisted: whose balances somebody
 *  checked is not a fact worth leaving on a shared phone. */
export function LookupScreen({ address, onOpen, onClose }: {
  address: string | null;
  onOpen: (address: string) => void;
  onClose: () => void;
}) {
  const [value, setValue] = useState('');
  const trimmed = value.trim();
  // Checked before opening rather than after: sending somebody to a screen that
  // then says "no such account" spends a round trip to repeat what the prefix
  // already said.
  const looksRight = /^yml1[0-9a-z]{38,}$/.test(trimmed);

  if (address) {
    return (
      <>
        <Back onClick={onClose}>{t('nav.lookUp')}</Back>
        <h1 className="y-screen__title wal-whoTitle">
          <Identifier value={address} />
        </h1>
        <p className="wal-small wal-muted">{t('wal.watchingOnly')}</p>
        <Holdings address={address} own={false} />
      </>
    );
  }

  return (
    <>
      <ScreenTitle lede={t('watch.lede')}>{t('watch.title')}</ScreenTitle>

      <form
        className="wal-form"
        onSubmit={(e) => { e.preventDefault(); if (looksRight) onOpen(trimmed); }}
      >
        <label className="wal-field">
          <span className="wal-field__label">{t('watch.address')}</span>
          <input
            className="wal-input y-mono"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="yml1…"
            spellCheck={false}
            autoComplete="off"
            autoCapitalize="none"
          />
        </label>

        {trimmed !== '' && !looksRight && <Note tone="bad">{t('wal.addressShape')}</Note>}

        <button type="submit" className="wal-btn" disabled={!looksRight}>
          {t('watch.show')}
        </button>
      </form>

      {trimmed !== '' && looksRight && (
        <p className="wal-small wal-muted">
          {t('wal.willOpen')} <span className="y-mono">{truncateAddress(trimmed)}</span>
        </p>
      )}

      <p className="wal-small wal-muted wal-keysNote">{t('watch.keysNote')}</p>
    </>
  );
}
