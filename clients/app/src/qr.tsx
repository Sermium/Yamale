import { useMemo, useRef, useState } from 'react';
import qrcode from 'qrcode-generator';

/**
 * A QR code, rendered as inline SVG.
 *
 * The encoder is `qrcode-generator` rather than one written here. An earlier
 * version of this file was hand-rolled: it produced something that looked
 * exactly like a QR code, shipped, and was reported as working on the strength
 * of the bytes being present in the bundle. No phone could read it. The first
 * thing that actually checked it was a user pointing a camera at it.
 *
 * That is the argument for the dependency in one paragraph. These codes get
 * printed, taped to market stalls and scanned by cracked cameras in sunlight;
 * an encoder nobody can verify is a liability in exactly the setting this
 * product is for, and 10KB is a cheap price for correctness somebody else has
 * already proven.
 *
 * Error correction level M rather than L, for the same field conditions: the
 * extra redundancy costs a few modules of size and buys a real recovery rate
 * off a creased poster.
 */
export function QrCode({ text, size = 220 }: { text: string; size?: number }) {
  const modules = useMemo(() => {
    // Type 0 lets the library choose the smallest version that fits.
    const qr = qrcode(0, 'M');
    qr.addData(text);
    qr.make();
    const count = qr.getModuleCount();
    const grid: boolean[][] = [];
    for (let row = 0; row < count; row++) {
      const line: boolean[] = [];
      for (let col = 0; col < count; col++) line.push(qr.isDark(row, col));
      grid.push(line);
    }
    return grid;
  }, [text]);

  const n = modules.length;
  // The quiet zone is part of the specification, not padding: a scanner that
  // cannot find four clear modules of margin often will not lock on at all.
  const quiet = 4;
  const total = n + quiet * 2;

  const path: string[] = [];
  for (let row = 0; row < n; row++) {
    for (let col = 0; col < n; col++) {
      if (modules[row][col]) path.push(`M${col + quiet} ${row + quiet}h1v1h-1z`);
    }
  }

  return (
    <svg
      width={size} height={size} viewBox={`0 0 ${total} ${total}`}
      role="img" aria-label="QR code"
      style={{ borderRadius: 12 }}
    >
      {/* An explicit white ground. The page can be dark, and a QR drawn on a
          dark background is a QR no scanner will read. */}
      <rect width={total} height={total} fill="#ffffff" />
      <path d={path.join('')} fill="#12253F" shapeRendering="crispEdges" />
    </svg>
  );
}

/**
 * The square as a picture somebody can actually send.
 *
 * On screen it is an SVG, which is right for display and useless for sharing:
 * a person wanting to put their code in a WhatsApp message needs a file, not a
 * DOM node. So it is rasterised on demand — serialise the SVG, draw it to a
 * canvas at a size that survives being screenshotted and re-scanned, and hand
 * out a PNG.
 *
 * Rendered at 4x the on-screen size deliberately. A QR photographed off a
 * screen or forwarded through a service that recompresses images loses exactly
 * the fine detail the finder patterns are made of, and a code that will not
 * scan is worse than no code at all.
 */
async function toPng(svg: SVGSVGElement, pixels = 720): Promise<Blob> {
  const markup = new XMLSerializer().serializeToString(svg);
  // Encoded as a data URL rather than a blob URL: some browsers taint a canvas
  // drawn from a blob-backed image, and a tainted canvas refuses toBlob().
  const url = 'data:image/svg+xml;base64,' + btoa(unescape(encodeURIComponent(markup)));

  const image = new Image();
  await new Promise<void>((resolve, reject) => {
    image.onload = () => resolve();
    image.onerror = () => reject(new Error('could not rasterise'));
    image.src = url;
  });

  const canvas = document.createElement('canvas');
  canvas.width = pixels;
  canvas.height = pixels;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('no canvas');
  // White ground, always. A transparent PNG dropped into a dark chat becomes a
  // black-on-black square that no scanner will read.
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, pixels, pixels);
  ctx.drawImage(image, 0, 0, pixels, pixels);

  return await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error('no blob'))), 'image/png');
  });
}

export interface QrPanelLabels {
  copy: string;
  copied: string;
  share: string;
  save: string;
}

export function QrPanel(
  { text, size = 200, filename = 'yamale-code.png', labels }:
  { text: string; size?: number; filename?: string; labels: QrPanelLabels },
) {
  const holder = useRef<HTMLDivElement>(null);
  const [done, setDone] = useState(false);

  const svg = () => holder.current?.querySelector('svg') as SVGSVGElement | null;

  async function copy() {
    const el = svg();
    if (!el) return;
    try {
      const blob = await toPng(el, size * 4);
      await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]);
      setDone(true);
      setTimeout(() => setDone(false), 1600);
    } catch {
      // Clipboard images are refused on insecure origins and unsupported in
      // some browsers. Saving the file is the outcome the person wanted anyway.
      await save();
    }
  }

  async function share() {
    const el = svg();
    if (!el) return;
    const blob = await toPng(el, size * 4);
    const file = new File([blob], filename, { type: 'image/png' });
    // canShare with the actual file, not just a feature check: desktop Chrome
    // has navigator.share and refuses files, and finding that out by throwing
    // mid-gesture loses the user activation that share needs.
    if (navigator.canShare?.({ files: [file] })) {
      try {
        await navigator.share({ files: [file] });
        return;
      } catch {
        return; // Cancelled by the person; not an error.
      }
    }
    await save();
  }

  async function save() {
    const el = svg();
    if (!el) return;
    const blob = await toPng(el, size * 4);
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <>
      <div className="qr-wrap" ref={holder}><QrCode text={text} size={size} /></div>
      <div className="qr-actions">
        <button type="button" className="ghost" onClick={copy}>
          {done ? labels.copied : labels.copy}
        </button>
        <button type="button" className="ghost" onClick={share}>{labels.share}</button>
        <button type="button" className="ghost" onClick={save}>{labels.save}</button>
      </div>
    </>
  );
}
