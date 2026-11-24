/**
 * Photos are fetched as bytes, checked against their fingerprint, and only
 * then shown, from a blob URL. An <img src> pointed at the API would display
 * whatever the API served, which is the one thing this page must not trust.
 *
 * The loupe exists for one job: comparing the engraving on the jewel in hand
 * with the engraving in the photo. So it magnifies to 4x and pans, and nothing
 * else: with a mouse, drag to move and roll the wheel to zoom about the point
 * under the cursor; on a phone, swipe to move; the slider works everywhere.
 */

import { useEffect, useLayoutEffect, useRef, useState, type PointerEvent } from 'react';
import { t } from '@yamale/chain';

import { fetchVerified } from '../api.ts';
import { MAX_ZOOM, MIN_ZOOM, scrollAbout, wheelZoom } from './loupe.ts';

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
      {img.state === 'loading' ? '' : img.reason === 'mismatch' ? t('traceability.photoMismatch') : t('traceability.photoUnavailable')}
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
              aria-label={`${t('traceability.loupe')}: ${t('traceability.photoN', { n: index + 1, count: hashes.length })}`}>
        <VerifiedImage hash={current} alt={alt} className="gallery__img" />
        <span className="gallery__hint">{t('traceability.loupe')}</span>
      </button>
      {hashes.length > 1 && (
        <ul className="gallery__thumbs">
          {hashes.map((h, i) => (
            <li key={h}>
              <button type="button" aria-current={i === index}
                      className={`gallery__thumb${i === index ? ' is-on' : ''}`} onClick={() => setIndex(i)}
                      aria-label={t('traceability.photoN', { n: i + 1, count: hashes.length })}>
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
  const view = useRef<HTMLDivElement>(null);
  const [zoom, setZoom] = useState(2);
  // Where the zoom was asked for: the point under the cursor (or the view's centre) stays put.
  const anchor = useRef<{ x: number; y: number; from: number } | null>(null);
  const drag = useRef<{ id: number; x: number; y: number; left: number; top: number } | null>(null);
  const [dragging, setDragging] = useState(false);
  useEffect(() => { dialog.current?.showModal(); }, []);

  const zoomTo = (next: number, x?: number, y?: number) => {
    const v = view.current;
    if (!v || next === zoom) return;
    anchor.current = { x: x ?? v.clientWidth / 2, y: y ?? v.clientHeight / 2, from: zoom };
    setZoom(next);
  };

  useLayoutEffect(() => {
    const v = view.current, a = anchor.current;
    if (!v || !a) return;
    anchor.current = null;
    v.scrollLeft = scrollAbout(v.scrollLeft, a.x, a.from, zoom);
    v.scrollTop = scrollAbout(v.scrollTop, a.y, a.from, zoom);
  }, [zoom]);

  // A wheel listener has to be registered by hand: React's is passive and cannot stop the page scrolling.
  useEffect(() => {
    const v = view.current;
    if (!v) return;
    const onWheel = (e: WheelEvent) => {
      if (e.ctrlKey && Math.abs(e.deltaY) < 1) return;
      e.preventDefault();
      const r = v.getBoundingClientRect();
      zoomTo(wheelZoom(zoom, e.deltaY), e.clientX - r.left, e.clientY - r.top);
    };
    v.addEventListener('wheel', onWheel, { passive: false });
    return () => v.removeEventListener('wheel', onWheel);
  });

  // Dragging is for a mouse or a pen; a finger already pans the view by scrolling it.
  const down = (e: PointerEvent<HTMLDivElement>) => {
    const v = view.current;
    if (!v || e.pointerType === 'touch' || e.button !== 0) return;
    e.preventDefault();
    v.setPointerCapture(e.pointerId);
    drag.current = { id: e.pointerId, x: e.clientX, y: e.clientY, left: v.scrollLeft, top: v.scrollTop };
    setDragging(true);
  };
  const move = (e: PointerEvent<HTMLDivElement>) => {
    const v = view.current, d = drag.current;
    if (!v || !d || d.id !== e.pointerId) return;
    v.scrollLeft = d.left - (e.clientX - d.x);
    v.scrollTop = d.top - (e.clientY - d.y);
  };
  const up = (e: PointerEvent<HTMLDivElement>) => {
    if (drag.current?.id !== e.pointerId) return;
    drag.current = null;
    setDragging(false);
  };

  return (
    <dialog ref={dialog} className="loupe" onClose={onClose} aria-label={t('traceability.loupe')}>
      <div ref={view} className={`loupe__view${dragging ? ' is-dragging' : ''}`}
           onPointerDown={down} onPointerMove={move} onPointerUp={up} onPointerCancel={up}
           onDragStart={(e) => e.preventDefault()}>
        <div style={{ width: `${zoom * 100}%` }}><VerifiedImage hash={hash} alt={alt} className="loupe__img" /></div>
      </div>
      <div className="loupe__bar">
        <label className="loupe__zoom">
          <span>{t('traceability.zoom')}</span>
          <input type="range" min={MIN_ZOOM} max={MAX_ZOOM} step={0.1} value={zoom} onChange={(e) => zoomTo(Number(e.target.value))} />
          <span className="num">{zoom.toFixed(1)}×</span>
        </label>
        <button type="button" className="btn" onClick={() => dialog.current?.close()}>{t('traceability.close')}</button>
      </div>
    </dialog>
  );
}
