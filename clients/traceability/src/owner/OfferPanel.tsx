/**
 * Passing a jewel on: to someone with an account, or by a link.
 *
 * The link carries a one-time claim key in its fragment and leaves this
 * phone directly: as a QR code at the counter, through the share sheet, or
 * by mail. No server of ours ever holds it. Both ways end in the same
 * sentence before the seller confirms: once accepted, nobody but the new
 * owner can reverse it.
 */

import { useState } from 'react';
import {
  cancelJewelOffer, claimLink, newClaimKey, offerJewelByLink, offerJewelToAccount, t,
  type EncodeObject, type JewelView, type SubmitResult,
} from '@yamale/chain';

import { QrCode } from '../../../app/src/qr.tsx';
import { Scanner } from '../../../app/src/scan.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { PUBLIC_ORIGIN } from '../config.ts';
import { day, ShareButtons } from '../ui.tsx';
import { forgetLinks, isAccountCode, rememberLink, storedLink } from './links.ts';
import { offerIsOpen } from './offers.ts';

export function AccountCodeInput({ value, onChange, id }: { value: string; onChange: (v: string) => void; id: string }) {
  const [scanning, setScanning] = useState(false);
  return (
    <div className="stack">
      <label className="y-label" htmlFor={id}>{t('traceability.theirCode')}</label>
      <input id={id} className="bigfield inscription" value={value} onChange={(e) => onChange(e.target.value)}
             autoComplete="off" spellCheck={false} />
      {value !== '' && !isAccountCode(value) && <p className="hint">{t('traceability.notACode')}</p>}
      {scanning
        ? <Scanner onRead={(text) => { onChange(text.trim()); setScanning(false); }} onClose={() => setScanning(false)} />
        : <button type="button" className="btn" onClick={() => setScanning(true)}>{t('traceability.scanCode')}</button>}
    </div>
  );
}

function LinkShare({ link, expiresAt }: { link: string; expiresAt: string }) {
  return (
    <div className="stack linkshare">
      <p className="note note--warn">{t('traceability.linkWarning', { date: day(expiresAt) })}</p>
      <div className="linkshare__qr"><QrCode text={link} size={240} /></div>
      <p className="hint">{t('traceability.linkScanHint')}</p>
      <ShareButtons link={link} title={t('traceability.shareTitle')} text={t('traceability.shareText')} />
    </div>
  );
}

export function OfferPanel({ view, actor, now, send, onChange }: {
  view: JewelView; actor: string; now: number; send: (messages: EncodeObject[]) => Promise<SubmitResult>; onChange: () => void;
}) {
  const [action, run] = useAction();
  const [mode, setMode] = useState<'choose' | 'account' | 'link'>('choose');
  const [to, setTo] = useState('');
  const id = view.jewel.id;
  const offer = offerIsOpen(view.offer, now) ? view.offer : null;
  const lapsed = !!view.offer && !offer;

  if (view.jewel.stolen) return <p className="note">{t('traceability.cannotOfferStolen')}</p>;

  if (offer) {
    const link = offer.claimKey ? storedLink(id, offer.claimKey) : null;
    return (
      <div className="stack">
        <p>{offer.to ? t('traceability.offeredToAccount', { date: day(offer.expiresAt) }) : t('traceability.offeredByLink', { date: day(offer.expiresAt) })}</p>
        {link && <LinkShare link={link} expiresAt={offer.expiresAt} />}
        {offer.claimKey && !link && <p className="hint">{t('traceability.linkNotOnThisPhone')}</p>}
        <button type="button" className="btn btn--danger" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => send([cancelJewelOffer(actor, id)]))) { forgetLinks(id); onChange(); }
        }}>{t('traceability.cancelOffer')}</button>
        <ActionStatus state={action} />
      </div>
    );
  }

  if (mode === 'choose') {
    return (
      <div className="stack">
        {lapsed && <p className="hint">{t('traceability.offerExpiredNote')}</p>}
        <button type="button" className="btn" onClick={() => setMode('account')}>{t('traceability.offerToAccount')}</button>
        <button type="button" className="btn" onClick={() => setMode('link')}>{t('traceability.offerByLink')}</button>
      </div>
    );
  }

  return (
    <div className="stack">
      {mode === 'account' && <AccountCodeInput id="offer-to" value={to} onChange={setTo} />}
      {mode === 'link' && <p className="hint">{t('traceability.offerByLinkExplain')}</p>}
      <p className="note note--warn">{t('traceability.offerIrreversible')}</p>
      <div className="actions">
        <button type="button" className="btn btn--primary"
                disabled={action.kind === 'busy' || (mode === 'account' && !isAccountCode(to))}
                onClick={async () => {
                  if (mode === 'account') {
                    if (await run(() => send([offerJewelToAccount(actor, id, to.trim())]))) onChange();
                    return;
                  }
                  // Kept before sending: a submit that fails after broadcasting must not lose the key.
                  // A link whose offer never lands is never shown, and is swept when the jewel is next read.
                  const { privateKey, claimKey } = newClaimKey();
                  rememberLink(id, claimKey, claimLink(PUBLIC_ORIGIN, id, privateKey));
                  if (await run(() => send([offerJewelByLink(actor, id, claimKey)]))) onChange();
                }}>
          {t('traceability.confirmOffer')}
        </button>
        <button type="button" className="btn" onClick={() => setMode('choose')}>{t('traceability.cancel')}</button>
      </div>
      <ActionStatus state={action} />
    </div>
  );
}
