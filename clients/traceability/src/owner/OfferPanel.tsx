/**
 * Giving a jewel to someone with an account: the owner scans the recipient's
 * account code, confirms, and the recipient accepts.
 *
 * Offers by link are no longer made: a shop hands a piece over with its
 * ownership card, and owners give to an account until the marketplace exists.
 * A link offer made before that still shows here, with its QR if this phone
 * kept it, so it can be shown again or cancelled.
 */

import { useState } from 'react';
import {
  cancelJewelOffer, offerJewelToAccount, t, type EncodeObject, type JewelView, type SubmitResult,
} from '@yamale/chain';

import { QrCode } from '../../../app/src/qr.tsx';
import { Scanner } from '../../../app/src/scan.tsx';
import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { day, ShareButtons } from '../ui.tsx';
import { forgetLinks, isAccountCode, storedLink } from './links.ts';
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
  const [giving, setGiving] = useState(false);
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

  if (!giving) {
    return (
      <div className="stack">
        {lapsed && <p className="hint">{t('traceability.offerExpiredNote')}</p>}
        <p className="hint">{t('traceability.giveExplain')}</p>
        <button type="button" className="btn" onClick={() => setGiving(true)}>{t('traceability.give')}</button>
      </div>
    );
  }

  return (
    <div className="stack">
      <AccountCodeInput id="offer-to" value={to} onChange={setTo} />
      <p className="note note--warn">{t('traceability.offerIrreversible')}</p>
      <div className="actions">
        <button type="button" className="btn btn--primary" disabled={action.kind === 'busy' || !isAccountCode(to)}
                onClick={async () => {
                  if (await run(() => send([offerJewelToAccount(actor, id, to.trim())]))) onChange();
                }}>
          {t('traceability.confirmOffer')}
        </button>
        <button type="button" className="btn" onClick={() => setGiving(false)}>{t('traceability.cancel')}</button>
      </div>
      <ActionStatus state={action} />
    </div>
  );
}
