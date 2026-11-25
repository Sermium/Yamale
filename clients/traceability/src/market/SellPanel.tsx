/**
 * Putting a piece up for sale on the marketplace, changing its price, or
 * taking it off. Used by an owner on their piece's page, and by staff on a
 * stock piece in the Atelier: the same messages, sent by a different account.
 */

import { useState } from 'react';
import { listJewel, readJewelParams, resolveDenom, saleSplit, t, toBaseUnitsOf, unlistJewel, type JewelView, type SubmitResult } from '@yamale/chain';
import type { EncodeObject } from '@cosmjs/proto-signing';

import { ActionStatus, useAction } from '../actions/useAction.tsx';
import { Loading, Problem, useLoad } from '../ui.tsx';
import { money } from './money.ts';

export function SellPanel({ view, actor, send, onChange }: {
  view: JewelView; actor: string;
  send: (m: EncodeObject[]) => Promise<SubmitResult>;
  onChange: () => void;
}) {
  const [params] = useLoad(() => readJewelParams(), []);
  const listing = view.listing;
  const [editing, setEditing] = useState(false);
  const [amount, setAmount] = useState('');
  const [denom, setDenom] = useState(listing?.price?.denom ?? '');
  const [action, run] = useAction();

  if (!params) return <Loading />;
  if (!params.ok) return <Problem outcome={params} />;
  const denoms = params.value.saleDenoms;
  const feeBps = params.value.saleFeeBps;
  if (view.jewel.stolen) return <p className="hint">{t('traceability.sellStolen')}</p>;
  if (view.reassignment) return <p className="hint">{t('traceability.sellReassignment')}</p>;
  if (denoms.length === 0) return <p className="hint">{t('traceability.sellClosed')}</p>;

  const chosen = denom || denoms[0]!;
  const base = toBaseUnitsOf(amount, chosen);
  const valid = !!base && BigInt(base.base) > 0n;
  const split = valid ? saleSplit(base!.base, feeBps) : null;

  async function list() {
    if (!valid) return;
    if (await run(() => send([listJewel(actor, view.jewel.id, { denom: chosen, amount: base!.base })]))) {
      setEditing(false); setAmount(''); onChange();
    }
  }
  async function unlist() {
    if (await run(() => send([unlistJewel(actor, view.jewel.id)]))) onChange();
  }

  return (
    <div className="stack sellpanel">
      {listing?.price && !editing && (
        <>
          <p className="price-tag"><span className="hint">{t('traceability.listedAt')}</span> <strong className="num">{money(listing.price)}</strong></p>
          <p className="hint">{t('traceability.sellListedNote')}</p>
          <div className="actions">
            <button type="button" className="btn" disabled={action.kind === 'busy'} onClick={() => { setDenom(listing.price!.denom); setEditing(true); }}>
              {t('traceability.sellChangePrice')}
            </button>
            <button type="button" className="btn btn--danger" disabled={action.kind === 'busy'} onClick={unlist}>{t('traceability.sellWithdraw')}</button>
          </div>
        </>
      )}
      {(!listing?.price || editing) && (
        <>
          <p className="hint">{t('traceability.sellExplain', { fee: (feeBps / 100).toString() })}</p>
          <div className="pricefield">
            <input className="bigfield num" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)}
                   aria-label={t('traceability.sellPrice')} placeholder="0" />
            <select className="bigfield" value={chosen} onChange={(e) => setDenom(e.target.value)} aria-label={t('traceability.sellCurrency')}>
              {denoms.map((d) => <option key={d} value={d}>{resolveDenom(d).symbol}</option>)}
            </select>
          </div>
          {split && (
            <p className="hint num">
              {t('traceability.sellYouReceive', { amount: money({ denom: chosen, amount: split.seller }), fee: money({ denom: chosen, amount: split.fee }) })}
            </p>
          )}
          <div className="actions">
            <button type="button" className="btn btn--primary" disabled={!valid || action.kind === 'busy'} onClick={list}>
              {t(listing?.price ? 'traceability.sellUpdate' : 'traceability.sellList')}
            </button>
            {editing && <button type="button" className="btn" onClick={() => setEditing(false)}>{t('traceability.cancel')}</button>}
          </div>
        </>
      )}
      <ActionStatus state={action} />
    </div>
  );
}
