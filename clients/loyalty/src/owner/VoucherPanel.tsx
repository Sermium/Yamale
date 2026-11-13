/**
 * The owner's consent to one use of a benefit, as a code the staff scan.
 *
 * Dated by the chain's clock, not the phone's, and renewed shortly before it
 * lapses, so the code on screen is always one the chain will accept.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { chainHead, encodeVoucherCode, makeVoucher, t } from '@yamale/chain';

import { QrCode } from '../../../app/src/qr.tsx';
import { useSession } from '../account/session.tsx';
import { CHAIN_ID } from '../config.ts';

export function VoucherPanel({ jewelId, benefitId }: { jewelId: string; benefitId: string }) {
  const session = useSession();
  const [code, setCode] = useState<{ text: string; deadline: number } | null>(null);
  const [failed, setFailed] = useState(false);
  const [now, setNow] = useState(Date.now());
  const making = useRef(false);

  const make = useCallback(async () => {
    if (making.current) return;
    making.current = true;
    try {
      const head = await chainHead();
      if (!head.ok) { setFailed(true); return; }
      const v = session.withKey((k) => makeVoucher(k, CHAIN_ID, benefitId, jewelId, head.value.time));
      const lifetime = Number(v.voucher.expiresAt) - head.value.time;
      setCode({ text: encodeVoucherCode(v), deadline: Date.now() + lifetime * 1000 });
      setFailed(false);
    } catch {
      setFailed(true);
    } finally {
      making.current = false;
    }
  }, [session, benefitId, jewelId]);

  useEffect(() => { make(); }, [make]);
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);
  useEffect(() => {
    if (code && code.deadline - now < 20_000) make();
  }, [now, code, make]);

  if (failed) return <p className="note note--warn">{t('loyalty.voucherNoChain')}</p>;
  if (!code) return <p className="loading">{t('loyalty.loading')}</p>;
  const seconds = Math.max(0, Math.round((code.deadline - now) / 1000));
  return (
    <div className="stack voucher">
      <p>{t('loyalty.voucherShow')}</p>
      <div className="linkshare__qr"><QrCode text={code.text} size={260} /></div>
      <p className="hint num" aria-live="off">{t('loyalty.voucherSeconds', { n: seconds })}</p>
      <button type="button" className="btn" onClick={() => navigator.clipboard.writeText(code.text).catch(() => {})}>
        {t('loyalty.copyVoucher')}
      </button>
    </div>
  );
}
