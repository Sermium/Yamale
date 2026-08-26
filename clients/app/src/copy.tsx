/**
 * Identifiers: truncated, copyable, and revealable.
 *
 * Two shapes of the same rule. Nobody reads a bech32 address or a 60-character
 * signed code character by character, and nobody types one — they copy it. So
 * what is shown is short enough to sit on a line, what reaches the clipboard is
 * the whole of it, and the whole of it is one click away for anybody who wants
 * to audit rather than trust.
 *
 * Lifted out of App.tsx unchanged in behaviour so that the payment screens can
 * use it without importing a 2,200-line module and creating a cycle.
 */
import { useState } from 'react';
import { t } from '@yamale/chain';

/** The clipboard, with the fallback that works where the API is refused. */
async function toClipboard(value: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(value);
  } catch {
    // Clipboard access is refused on insecure origins and by some in-app
    // browsers. Selecting the text is the fallback that always works.
    const el = document.createElement('textarea');
    el.value = value;
    document.body.appendChild(el);
    el.select();
    document.execCommand('copy');
    el.remove();
  }
}

/**
 * A code on one line, with the whole of it one tap away.
 *
 * The full code is long because it carries a signature, and a signature is what
 * makes it safe to send over a channel nobody controls. But nobody reads it
 * aloud and nobody types it — they copy it. So the display is truncated to fit
 * a line and the *untruncated* value is what reaches the clipboard; showing all
 * of it only cost four lines of wrapping and made the screen look broken.
 */
export function CopyRow({ value }: { value: string }) {
  const [done, setDone] = useState(false);

  const copy = async () => {
    await toClipboard(value);
    setDone(true);
    setTimeout(() => setDone(false), 1600);
  };

  const shown = value.length > 22 ? `${value.slice(0, 12)}…${value.slice(-6)}` : value;

  return (
    <button type="button" className="copyrow" onClick={copy} title={value}>
      <span className="copyrow__code">{shown || '…'}</span>
      <span className="copyrow__icon" aria-hidden="true">
        {done ? <TickIcon /> : <CopyIcon />}
      </span>
      <span className="sr-only">{done ? t('app.copied') : t('app.copy')}</span>
    </button>
  );
}

/**
 * An identifier inside a dense table.
 *
 * Same rule, less furniture: clicking copies, and the chevron reveals the full
 * value in place rather than in a tooltip — a tooltip is unreachable from a
 * keyboard and invisible on a touch screen, which are two of the three ways
 * this is read.
 *
 * `label` is what a person should see instead of the raw value where the chain
 * has issued one — a user ID, a saved name. The raw value is still what is
 * copied and what the reveal shows, because the point of the reveal is to audit
 * the thing that was actually signed.
 */
export function CopyValue({ value, label }: { value: string; label?: string }) {
  const [done, setDone] = useState(false);
  const [open, setOpen] = useState(false);

  if (!value) return <span className="iso__none">{t('iso.notSet')}</span>;

  const short = value.length > 18 ? `${value.slice(0, 9)}…${value.slice(-6)}` : value;
  const face = open ? value : (label || short);

  return (
    <span className="copyval">
      <button
        type="button"
        className="copyval__face y-mono"
        onClick={async () => { await toClipboard(value); setDone(true); setTimeout(() => setDone(false), 1600); }}
        aria-label={`${t('app.copy')}: ${value}`}
      >
        {face}
        <span className="copyval__ico" aria-hidden="true">{done ? <TickIcon /> : <CopyIcon />}</span>
      </button>
      {(label || value !== short) && (
        <button
          type="button"
          className="copyval__reveal"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
        >
          {open ? t('app.hide') : t('app.reveal')}
        </button>
      )}
    </span>
  );
}

function TickIcon() {
  return (
    <svg viewBox="0 0 24 24" width="14" height="14" fill="none"
         stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20 6 9 17l-5-5" />
    </svg>
  );
}

function CopyIcon() {
  return (
    <svg viewBox="0 0 24 24" width="14" height="14" fill="none"
         stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <rect x="9" y="9" width="11" height="11" rx="2" />
      <path d="M5 15V5a2 2 0 0 1 2-2h10" />
    </svg>
  );
}
