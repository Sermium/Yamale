/**
 * Photos are fetched as bytes, checked against their fingerprint, and only
 * then shown, from a blob URL. An <img src> pointed at the API would display
 * whatever the API served, which is the one thing this page must not trust.
 *
 * The loupe exists for one job: comparing the engraving on the jewel in hand
 * with the engraving in the photo. So it magnifies to 4x and pans by
 * scrolling, and nothing else.
 */

import { useEffect, useRef, useState } from 'react';
import { t } from '@yamale/chain';

import { fetchVerified } from '../api.ts';

type Img = { state: 'loading' } | { state: 'ok'; url: string } | { state: 'failed'; reason: string };

function useVerifiedImage(hash: string): Img {
  // The state is kept with its hash: after a hash change the old blob URL is
  // revoked, so a state for any other hash is "loading", never displayed.
  const [held, setHeld] = useState<{ hash: string; img: Img } | null>(null);
  useEffect(() => {
    let live = true;
    let url = '';
    fetchVerified(hash).then((f) => {
      if (!live) return;
      if (!f.ok) { setHeld({ hash, img: { state: 'failed', reason: f.reason } }); return; }
      url = URL.createObjectURL(new Blob([f.bytes as BlobPart], { type: f.type }));
      setHeld({ hash, img: { state: 'ok', url } });
    });
    return () => { live = false; if (url) URL.revokeObjectURL(url); };
  }, [hash]);
  return held && held.hash === hash ? held.img : { state: 'loading' };
}

export function VerifiedImage({ hash, alt, className }: { hash: string; alt: string; className?: string }) {
  const img = useVerifiedImage(hash);
  if (img.state === 'ok') return <img className={className} src={img.url} alt={alt} />;
  return (
    <div className={`${className ?? ''} photo--absent`}>
      {img.state === 'loading' ? '' : img.reason === 'mismatch' ? t('loyalty.photoMismatch') : t('loyalty.photoUnavailable')}
    </div>
  );
}

export function Gallery({ hashes, alt }: { hashes: string[]; alt: string }) {
  const [index, setIndex] = useState(0);
  const [loupe, setLoupe] = useState(false);
  const current = hashes[Math.min(index, hashes.length - 1)]!;
  return (
    <div className="gallery">
      <button type="button" className="gallery__main" onClick={() => setLoupe(true)}
              aria-label={`${t('loyalty.loupe')}: ${t('loyalty.photoN', { n: index + 1, count: hashes.length })}`}>
        <VerifiedImage hash={current} alt={alt} className="gallery__img" />
        <span className="gallery__hint">{t('loyalty.loupe')}</span>
      </button>
      {hashes.length > 1 && (
        <ul className="gallery__thumbs">
          {hashes.map((h, i) => (
            <li key={h}>
              <button type="button" aria-current={i === index}
                      className={`gallery__thumb${i === index ? ' is-on' : ''}`} onClick={() => setIndex(i)}
                      aria-label={t('loyalty.photoN', { n: i + 1, count: hashes.length })}>
                <VerifiedImage hash={h} alt="" className="gallery__thumbimg" />
              </button>
            </li>
          ))}
        </ul>
      )}
      {loupe && <Loupe hash={current} alt={alt} onClose={() => setLoupe(false)} />}
    </div>
  );
}

function Loupe({ hash, alt, onClose }: { hash: string; alt: string; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [zoom, setZoom] = useState(2);
  useEffect(() => { dialog.current?.showModal(); }, []);
  return (
    <dialog ref={dialog} className="loupe" onClose={onClose} aria-label={t('loyalty.loupe')}>
      <div className="loupe__view">
        <div style={{ width: `${zoom * 100}%` }}><VerifiedImage hash={hash} alt={alt} className="loupe__img" /></div>
      </div>
      <div className="loupe__bar">
        <label className="loupe__zoom">
          <span>{t('loyalty.zoom')}</span>
          <input type="range" min={1} max={4} step={0.5} value={zoom} onChange={(e) => setZoom(Number(e.target.value))} />
          <span className="num">{zoom}×</span>
        </label>
        <button type="button" className="btn" onClick={() => dialog.current?.close()}>{t('loyalty.close')}</button>
      </div>
    </dialog>
  );
}
