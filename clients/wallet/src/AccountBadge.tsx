import { useQuery } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { displayName, getContact, saveContact, LanguagePicker, t } from '@yamale/chain';

import { client } from './chain.ts';
import { destroyVault, lock, vaultSummary } from './vault.ts';
import { ClaimUserId } from './ClaimUserId.tsx';

/**
 * The connected account, top right.
 *
 * It shows the account by the same three-tier rule every interface uses — your
 * own name for it, else its chain-assigned user ID, else the address — so the
 * header answers "who am I signed in as" in words rather than in bech32.
 *
 * The tier is labelled rather than left implicit. A pseudonym is a word *you*
 * chose and a user ID is one the *chain* assigned, and those carry very
 * different authority: presenting them identically would train people to trust
 * a name they typed themselves as if the network had confirmed it.
 *
 * Tier two resolves against `x/alias`, which issues an identifier only for an
 * account recorded in a country — so an account no authority has placed shows
 * its address, and that is accurate rather than a fallback: it has no user ID
 * to show. See docs/guides/identity.md.
 */
export function AccountBadge() {
  const [open, setOpen] = useState(false);
  const [account, setAccount] = useState(() => vaultSummary());
  const [naming, setNaming] = useState(false);
  const [draft, setDraft] = useState('');
  const location = useLocation();
  const box = useRef<HTMLDivElement>(null);

  // Asked once and cached here rather than inside displayName, which stays
  // synchronous and testable on purpose. Null is the ordinary answer for an
  // account nobody has placed in a country.
  const userId = useQuery({
    queryKey: ['userId', account?.address],
    queryFn: () => client.userIdOf(account!.address),
    enabled: Boolean(account?.address),
  });

  // The vault is written by another page in this same tab, so no storage event
  // fires; re-reading on navigation keeps the badge honest.
  useEffect(() => {
    setAccount(vaultSummary());
    setOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    if (!open) return;
    const onAway = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false);
    };
    const onEsc = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false);
    document.addEventListener('mousedown', onAway);
    document.addEventListener('keydown', onEsc);
    return () => {
      document.removeEventListener('mousedown', onAway);
      document.removeEventListener('keydown', onEsc);
    };
  }, [open]);

  if (!account) {
    // The picker rides along here too. With no account there is no dropdown to
    // put it in, and the create/load screens are precisely where somebody who
    // cannot read the interface needs to change its language.
    return (
      <div className="acct-anon">
        <LanguagePicker className="lang" />
        <NavLink to="/create" className="acct-connect">
          Connect an account
        </NavLink>
      </div>
    );
  }

  // The lookup answers for this one account and nothing else. A closure that
  // returned a value for any address would be a directory, and the chain
  // deliberately offers none.
  const shown = displayName(account.address, (a) =>
    a === account.address ? (userId.data ?? undefined) : undefined);
  const contact = getContact(account.address);

  return (
    <div className="acct" ref={box}>
      <button type="button" className="acct__button" onClick={() => setOpen((v) => !v)}>
        <span className="acct__dot" aria-hidden="true" />
        <span className="acct__name">{shown.label}</span>
      </button>

      {open && (
        <div className="acct__panel">
          <p className="acct__tier">
            {shown.kind === 'pseudonym'
              ? 'Your name for this account'
              : shown.kind === 'userId'
                ? 'User ID, assigned by the chain'
                : 'No name yet — showing the address'}
          </p>

          <p className="acct__row">
            <span className="acct__label">Address</span>
            <code>{account.address}</code>
          </p>

          <ClaimUserId address={account.address} label={account.label} />

          {naming ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (draft.trim()) {
                  saveContact({ address: account.address, pseudonym: draft.trim() });
                  setNaming(false);
                  setDraft('');
                  setOpen(false);
                }
              }}
            >
              <label className="field">
                <span>Call it</span>
                <input value={draft} onChange={(e) => setDraft(e.target.value)} autoFocus />
              </label>
              <button type="submit">Save</button>
            </form>
          ) : (
            <button
              type="button"
              onClick={() => {
                setDraft(contact?.pseudonym ?? '');
                setNaming(true);
              }}
            >
              {contact ? 'Rename' : 'Give it a name'}
            </button>
          )}

          <p className="acct__links">
            <NavLink to={`/a/${account.address}`}>Balances</NavLink> ·{' '}
            <NavLink to="/send">Send</NavLink> · <NavLink to="/faucet">Get funds</NavLink>
          </p>

          {/* Disconnect is the everyday action and the only one offered plainly.
              In a self-custody wallet there is no session to disconnect from —
              this wallet *is* the key holder — so disconnecting means locking:
              the key leaves memory, the account stays on the device, and the
              next signature asks for the password again.

              Deleting the vault is a different and much rarer act, so it lives
              behind a disclosure rather than as a button beside this one. A
              destructive action sitting next to an everyday one is how people
              destroy things by accident. */}
          <div className="acct__settings">
            {/* Language lives here rather than in the header. It is a setting
                somebody touches once, and it was taking permanent space beside
                the account — the thing they look at constantly. */}
            <label className="field acct__lang">
              <span>{t('nav.language')}</span>
              <LanguagePicker className="lang" />
            </label>

            <button
              type="button"
              onClick={() => {
                lock();
                setOpen(false);
              }}
            >
              Disconnect
            </button>
            <p className="small muted acct__hint">
              Locks this wallet. The account stays on this device and the password is asked for
              again next time you sign.
            </p>

            <details className="acct__danger">
              <summary>Remove this account from the browser</summary>
              <div className="notice notice--bad">
                <strong>This deletes the account from this browser.</strong> Without the 24-word
                phrase it cannot be recovered by anyone — not by us, not by a validator. The money
                stays on the chain and becomes unreachable.
                <button
                  type="button"
                  onClick={() => {
                    destroyVault();
                    lock();
                    setAccount(null);
                    setOpen(false);
                  }}
                >
                  I have the phrase — remove it
                </button>
              </div>
            </details>
          </div>
        </div>
      )}
    </div>
  );
}
