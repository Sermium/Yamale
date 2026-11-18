/**
 * The owner's consent to one use of a benefit, as a code the staff scan.
 *
 * Dated by the chain's clock, not the phone's. ONE code is issued: it is never
 * renewed behind the owner's back. When it lapses the owner must ask for a new
 * one, and while it is shown the chain is polled so a recorded use closes it.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { chainHead, encodeVoucherCode, makeVoucher, readJewel, t } from '@yamale/chain';

import { QrCode } from '../../../app/src/qr.tsx';
import { useSession } from '../account/session.tsx';
import { CHAIN_ID } from '../config.ts';
import { markOf, voucherPhase, type UseMark } from './voucher.ts';

const POLL_MS = 4000;

export function VoucherPanel({ jewelId, benefitId }: { jewelId: string; benefitId: string }) {
  const { withKey } = useSession();
  const [code, setCode] = useState<{ text: string; deadline: number; issued: UseMark } | null>(null);
  const [current, setCurrent] = useState<UseMark | null>(null);
  const [failed, setFailed] = useState(false);
  const [now, setNow] = useState(Date.now());
  const making = useRef(false);

  const make = useCallback(async () => {
    if (making.current) return;
    making.current = true;
    try {
      const [head, view] = await Promise.all([chainHead(), readJewel(jewelId)]);
      if (!head.ok || !view.ok) { setFailed(true); return; }
      const benefit = view.value.benefits.find((b) => b.id === benefitId);
      if (!benefit) { setFailed(true); return; }
      const v = withKey((k) => makeVoucher(k, CHAIN_ID, benefitId, jewelId, head.value.time));
      const lifetime = Number(v.voucher.expiresAt) - head.value.time;
      const issued = markOf(benefit.uses);
      setCurrent(issued);
      setCode({ text: encodeVoucherCode(v), deadline: Date.now() + lifetime * 1000, issued });
      setFailed(false);
    } catch {
      setFailed(true);
    } finally {
      making.current = false;
    }
  }, [withKey, benefitId, jewelId]);

  useEffect(() => { make(); }, [make]);
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);

  const phase = code ? voucherPhase(code.issued, current, now, code.deadline) : null;
  useEffect(() => {
    if (phase !== 'showing') return;
    let live = true;
    const id = window.setInterval(async () => {
      try {
        const view = await readJewel(jewelId);
        if (!live || !view.ok) return;
        const b = view.value.benefits.find((x) => x.id === benefitId);
        if (b) setCurrent(markOf(b.uses));
      } catch { /* the next poll tries again */ }
    }, POLL_MS);
    return () => { live = false; window.clearInterval(id); };
  }, [phase, jewelId, benefitId]);

  if (failed) {
    return (
      <div className="stack">
        <p className="note note--warn">{t('traceability.voucherNoChain')}</p>
        <button type="button" className="btn" onClick={() => { make(); }}>{t('traceability.retry')}</button>
      </div>
    );
  }
  if (!code || !phase) return <p className="loading">{t('traceability.loading')}</p>;
  if (phase === 'used') return <p className="banner banner--ok" role="status">{t('traceability.voucherUsed')}</p>;
  if (phase === 'lapsed') {
    return (
      <div className="stack">
        <p className="note">{t('traceability.voucherLapsed')}</p>
        <button type="button" className="btn" onClick={() => { setCode(null); make(); }}>{t('traceability.voucherNew')}</button>
      </div>
    );
  }
  const seconds = Math.max(0, Math.round((code.deadline - now) / 1000));
  return (
    <div className="stack voucher">
      <p>{t('traceability.voucherShow')}</p>
      <div className="linkshare__qr"><QrCode text={code.text} size={260} /></div>
      <p className="hint num" aria-live="off">{t('traceability.voucherSeconds', { n: seconds })}</p>
      <button type="button" className="btn" onClick={() => navigator.clipboard.writeText(code.text).catch(() => {})}>
        {t('traceability.copyVoucher')}
      </button>
    </div>
  );
}
