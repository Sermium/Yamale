import { truncateAddress } from '@yamale/chain';
import { useState } from 'react';

import { Named } from './Named.tsx';

/**
 * An account on screen: the name if the chain or the address book knows one,
 * the truncated address otherwise, and the whole thing one click from the
 * clipboard.
 *
 * Three things have to be true at once, and picking any two produces a screen
 * somebody gets wrong.
 *
 * A **name** is what a reader recognises — "Treasury 3's admin" means nothing,
 * "NG-CAQ3-C04Z-M" is checkable against a document.
 *
 * The **address** is what actually receives, so it has to be reachable. Not
 * printed in full by default: 43 characters of bech32 in a table cell wraps to
 * three lines and destroys the row it sits in, and nobody verifies an address by
 * reading it anyway.
 *
 * And **copying** has to work without a selection. On a phone, dragging a
 * selection across a monospace address is the single most error-prone
 * interaction in a wallet, and a half-copied address pays nobody.
 */
export function Address({
  address,
  name = true,
}: {
  address: string;
  /** Resolve to a name. False for places where the address itself is the subject. */
  name?: boolean;
}) {
  const [copied, setCopied] = useState(false);
  const [shown, setShown] = useState(false);

  if (!address) return <span className="muted">—</span>;

  async function copy() {
    try {
      await navigator.clipboard.writeText(address);
    } catch {
      // Refused on insecure origins and in some in-app browsers. Revealing the
      // address is the fallback that always works: it can then be selected by
      // hand, which is what somebody would have had to do anyway.
      setShown(true);
      return;
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 1600);
  }

  return (
    <span className="addr">
      <button
        type="button"
        className="addr__copy"
        onClick={copy}
        /* The full address in the tooltip and in the accessible name, so an
           expert can check it without a click and a screen reader announces
           what will be copied rather than "button". */
        title={address}
        aria-label={copied ? `Copied ${address}` : `Copy ${address}`}
      >
        <span className="addr__label">
          {name ? <Named address={address} /> : <span className="y-mono">{truncateAddress(address)}</span>}
        </span>
        <span className="addr__icon" aria-hidden="true">
          {copied ? (
            <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor"
                 strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round">
              <path d="M20 6 9 17l-5-5" />
            </svg>
          ) : (
            <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor"
                 strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <rect x="9" y="9" width="11" height="11" rx="2" />
              <path d="M5 15V5a2 2 0 0 1 2-2h10" />
            </svg>
          )}
        </span>
      </button>
      {/* The reveal, for the moment somebody does need to read every character
          — comparing against a printed document, or dictating it. A details
          element rather than a hover: it works by keyboard and on a touch
          screen, which a tooltip does not. */}
      <button
        type="button"
        className="addr__reveal"
        aria-expanded={shown}
        onClick={() => setShown(!shown)}
      >
        {shown ? 'hide' : 'show'}
      </button>
      {shown && <code className="addr__full y-addr">{address}</code>}
    </span>
  );
}
