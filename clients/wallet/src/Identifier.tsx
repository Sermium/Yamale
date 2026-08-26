import { t, truncateAddress, truncateHash } from '@yamale/chain';
import { useState } from 'react';

import { Named } from './Named.tsx';

/**
 * An identifier on screen — an address, a transaction hash — truncated, one tap
 * from the clipboard, and revealable in full.
 *
 * The wallet printed these three ways in three places: a full 43-character
 * bech32 inside `<code class="address">`, a `truncateAddress()` with no way to
 * see the rest, and a raw hash that wrapped across four lines. All three fail
 * the same person for different reasons.
 *
 * A full address is not checkable by reading — nobody reads bech32 — and it
 * wraps, which on the approval screen pushes the amount off a phone. A
 * truncation with no reveal is unusable at the one moment somebody genuinely
 * needs every character: reading it back down a telephone, or comparing it to a
 * printed mandate. And a copy that needs a drag-selection across monospace text
 * is the most error-prone interaction in a wallet, on the screen where a
 * half-copied value pays the wrong account.
 *
 * So: the name where the chain issued one, the truncation otherwise, copy from
 * the whole target, and a separate reveal. Copying and revealing are different
 * intents — merging them reflows the row on every copy.
 */
export function Identifier({
  value,
  /** Resolve an address to its user ID or address-book name. Off for hashes. */
  name,
}: {
  value: string;
  name?: boolean;
}) {
  const [copied, setCopied] = useState(false);
  const [shown, setShown] = useState(false);

  if (!value) return <span className="muted">—</span>;

  const isAddress = value.startsWith('yml1');
  const resolve = name ?? isAddress;
  const short = isAddress ? truncateAddress(value) : truncateHash(value);

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      // Refused on an insecure origin or inside an in-app browser. Revealing is
      // the fallback that always works: it can then be selected by hand, which
      // is what somebody would have had to do anyway. A false "Copied!" is the
      // one response that loses them the value.
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
        title={value}
        aria-label={copied ? t('id.copied', { value }) : t('id.copy', { value })}
      >
        <span className="addr__label">
          {resolve ? <Named address={value} /> : <span className="y-mono">{short}</span>}
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
      <button
        type="button"
        className="addr__reveal"
        aria-expanded={shown}
        onClick={() => setShown(!shown)}
      >
        {shown ? t('id.hide') : t('id.show')}
      </button>
      {shown && <code className="addr__full y-addr">{value}</code>}
    </span>
  );
}
