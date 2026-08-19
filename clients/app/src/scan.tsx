import { useEffect, useRef, useState } from 'react';
import { t } from '@yamale/chain';

/**
 * Reading a QR with the phone's camera.
 *
 * This uses the browser's own BarcodeDetector rather than bundling a decoder.
 * A QR decoder is several hundred kilobytes of Reed-Solomon and perspective
 * correction, and shipping that to a phone on a metered connection to save one
 * fallback is the wrong trade in a region where data is the constraint.
 *
 * Where the API is missing — older Android browsers, desktop Safari — the
 * component says so plainly and the person types the ID instead. That path
 * exists anyway and always has: a code that can only be scanned is a code that
 * excludes anyone whose camera is broken, whose lens is scratched, or who is
 * reading it out down a phone line.
 *
 * The camera is released on unmount and on the first successful read. A preview
 * left running behind a closed screen is a light on the phone that the person
 * did not ask for and cannot explain.
 */
export function Scanner({ onRead, onClose }: { onRead: (text: string) => void; onClose: () => void }) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [error, setError] = useState<'unsupported' | 'denied' | null>(null);

  useEffect(() => {
    const Detector = (window as unknown as { BarcodeDetector?: new (o: { formats: string[] }) => {
      detect(source: CanvasImageSource): Promise<{ rawValue: string }[]>;
    } }).BarcodeDetector;

    if (!Detector) { setError('unsupported'); return; }

    let stream: MediaStream | null = null;
    let timer: number | null = null;
    let stopped = false;

    const stop = () => {
      stopped = true;
      if (timer !== null) clearInterval(timer);
      stream?.getTracks().forEach((track) => track.stop());
    };

    (async () => {
      try {
        stream = await navigator.mediaDevices.getUserMedia({
          // The back camera, because nobody scans a poster with the selfie lens.
          video: { facingMode: 'environment' },
        });
        if (stopped) { stream.getTracks().forEach((tr) => tr.stop()); return; }
        if (videoRef.current) {
          videoRef.current.srcObject = stream;
          await videoRef.current.play();
        }

        const detector = new Detector({ formats: ['qr_code'] });
        timer = window.setInterval(async () => {
          if (!videoRef.current || stopped) return;
          try {
            const found = await detector.detect(videoRef.current);
            if (found.length > 0) {
              stop();
              onRead(found[0].rawValue);
            }
          } catch {
            // A frame that fails to decode is the normal case, not an error —
            // most frames are blur, glare, or the poster half out of shot.
          }
        }, 250);
      } catch {
        setError('denied');
      }
    })();

    return stop;
  }, [onRead]);

  return (
    <div className="scanner">
      {error === null && (
        <>
          <video ref={videoRef} className="scanner__view" playsInline muted />
          <div className="scanner__frame" aria-hidden="true" />
          <p className="muted center">{t('app.pointAtCode')}</p>
        </>
      )}
      {error === 'unsupported' && <p className="notice notice--bad">{t('app.scanUnsupported')}</p>}
      {error === 'denied' && <p className="notice notice--bad">{t('app.scanDenied')}</p>}
      <button type="button" className="ghost" onClick={onClose}>{t('app.cancel')}</button>
    </div>
  );
}
