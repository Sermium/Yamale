import { useEffect, useRef, useState } from 'react';
import { displayName, getContact, saveContact, truncateAddress, t} from '@yamale/chain';

import { useViewMode } from './chain.ts';
import { useWallet } from './wallet.tsx';

/**
 * The connected account, and the settings that belong to it.
 *
 * This replaces both the old connect button and the standalone Simple/Detailed
 * switch that sat beside it in the masthead. Level of detail is a preference
 * about how *you* want to read the chain, so it belongs with the other things
 * that are yours — your account, and the name you have given it — rather than
 * as a loose pair of buttons in the header competing with navigation.
 *
 * The account name uses the same three-tier rule as every other interface:
 * your address-book name, else the chain-assigned user ID, else the address.
 * Tier two stays dark until x/alias is on the chain.
 */
export function AccountBadge() {
  const { address, connecting, providers, connect, disconnect } = useWallet();
  const { mode, setMode } = useViewMode();
  const [open, setOpen] = useState(false);
  const [naming, setNaming] = useState(false);
  const [draft, setDraft] = useState('');
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false);
    document.addEventListener('mousedown', away);
    document.addEventListener('keydown', esc);
    return () => {
      document.removeEventListener('mousedown', away);
      document.removeEventListener('keydown', esc);
    };
  }, [open]);

  const shown = address ? displayName(address) : null;
  const contact = address ? getContact(address) : undefined;

  return (
    <div className="acct" ref={box}>
      <button
        type="button"
        className={address ? 'acct__button acct__button--on' : 'acct__button'}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        {address ? (
          <>
            <span className="acct__dot" aria-hidden="true" />
            <span className="acct__name">{shown!.label}</span>
          </>
        ) : (
          'Connect'
        )}
      </button>

      {open && (
        <div className="acct__panel">
          {address ? (
            <>
              <p className="acct__tier">
                {shown!.kind === 'pseudonym'
                  ? 'Your name for this account'
                  : shown!.kind === 'userId'
                    ? 'User ID, assigned by the chain'
                    : 'No name yet — showing the address'}
              </p>
              <p className="acct__row">
                <span className="acct__label">{t('watch.address')}</span>
                <code>{address}</code>
              </p>
              <p className="acct__row">
                <span className="acct__label">User ID</span>
                <span className="small muted">not yet issued on this chain</span>
              </p>

              {naming ? (
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (draft.trim()) {
                      saveContact({ address, pseudonym: draft.trim() });
                      setNaming(false);
                      setDraft('');
                    }
                  }}
                >
                  <input
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                    placeholder="Call it…"
                    autoFocus
                  />
                  <button type="submit">Save</button>
                </form>
              ) : (
                <button
                  type="button"
                  className="acct__minor"
                  onClick={() => {
                    setDraft(contact?.pseudonym ?? '');
                    setNaming(true);
                  }}
                >
                  {contact ? 'Rename' : 'Give it a name'}
                </button>
              )}
            </>
          ) : (
            <>
              <p className="acct__tier">{t('msg.notConnected')}</p>
              {providers.length === 0 ? (
                <p className="small muted">
                  No wallet found. Open the{' '}
                  <a href={`${location.protocol}//${location.hostname}:8092`}>Yamale Wallet</a> to
                  create an account.
                </p>
              ) : (
                providers.map((p) => (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => void connect(p.id)}
                    disabled={connecting}
                  >
                    {connecting ? 'Connecting…' : `Connect ${p.label}`}
                  </button>
                ))
              )}
            </>
          )}

          {/* Settings sit below the account, separated by a rule: they apply
              whether or not anybody is connected, which is why they are not
              inside the connected branch above. */}
          <div className="acct__settings">
            <span className="acct__label">Level of detail</span>
            <div className="modeswitch" role="group" aria-label="Level of detail">
              <button
                type="button"
                aria-pressed={mode === 'simple'}
                onClick={() => setMode('simple')}
              >
                Simple
              </button>
              <button
                type="button"
                aria-pressed={mode === 'expert'}
                onClick={() => setMode('expert')}
              >
                Detailed
              </button>
            </div>
            <p className="small muted acct__hint">
              {mode === 'simple'
                ? 'Activity in sentences, with the machinery hidden.'
                : 'Every message, hash, fee and signature, with addresses shown in full.'}
            </p>
          </div>

          {address && (
            <p className="acct__links">
              <button type="button" className="acct__minor" onClick={disconnect}>
                Disconnect {truncateAddress(address)}
              </button>
            </p>
          )}
        </div>
      )}
    </div>
  );
}
