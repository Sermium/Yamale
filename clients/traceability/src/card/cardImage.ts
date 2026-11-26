/**
 * The ownership card as a picture, to send to a buyer the way it looks
 * printed: the shop's name over its colour band, the QR code that carries the
 * card's link, the piece, its engraved code and the line that says what to do.
 *
 * Drawn on a canvas from the same facts the printed card shows, at three times
 * the 90 x 56 mm card's proportions, rather than by photographing the HTML:
 * the QR modules come straight from the same encoder, so the image scans as
 * well as the print.
 */

import qrcode from 'qrcode-generator';

export interface CardFace {
  shop: string;
  title: string;
  inscription: string;
  link: string;
  scanLine: string;
  /** The jeweler's own colours, if it set them. */
  colour?: string;
  accent?: string;
}

const W = 1080;
const H = 672;

export async function drawCard(face: CardFace): Promise<HTMLCanvasElement> {
  // The app's faces are loaded by the page; wait for them so the text is not drawn in a fallback.
  try { await document.fonts.ready; } catch { /* no font loading API: the fallback stack draws */ }
  const c = document.createElement('canvas');
  c.width = W;
  c.height = H;
  const g = c.getContext('2d')!;

  // Card and its border.
  g.fillStyle = '#fffdf8';
  rounded(g, 0, 0, W, H, 24);
  g.fill();
  g.strokeStyle = '#d9cfbd';
  g.lineWidth = 4;
  rounded(g, 2, 2, W - 4, H - 4, 22);
  g.stroke();
  // The colour band along the top.
  g.save();
  rounded(g, 0, 0, W, H, 24);
  g.clip();
  g.fillStyle = face.colour || '#2c4a3e';
  g.fillRect(0, 0, W, 26);
  g.restore();

  const pad = 60;
  // The shop's name.
  g.fillStyle = face.accent || '#7a5a1e';
  g.font = '600 38px Archivo, "Helvetica Neue", Arial, sans-serif';
  g.textBaseline = 'alphabetic';
  fitText(g, spaced(face.shop.toUpperCase()), pad, 104, W - pad * 2);

  // The QR code, on white with its quiet zone.
  const qrSize = 440;
  const qrX = pad;
  const qrY = 140;
  drawQr(g, face.link, qrX, qrY, qrSize);

  // The piece, its code and the scan line, centred beside the QR code.
  const x = qrX + qrSize + 48;
  const width = W - x - pad;
  g.font = '600 46px Archivo, "Helvetica Neue", Arial, sans-serif';
  const titleLines = wrap(g, face.title, width).slice(0, 3);
  g.font = '400 30px "IBM Plex Sans", system-ui, sans-serif';
  const scanLines = wrap(g, face.scanLine, width).slice(0, 3);
  const block = titleLines.length * 54 + 16 + 60 + 58 + (scanLines.length - 1) * 38;
  g.fillStyle = '#1d1a16';
  g.font = '600 46px Archivo, "Helvetica Neue", Arial, sans-serif';
  let y = qrY + (qrSize - block) / 2 + 40;
  for (const line of titleLines) {
    g.fillText(line, x, y);
    y += 54;
  }

  // The engraved code as a hallmark: a double brass rule round it.
  y += 16;
  g.font = '500 34px "IBM Plex Mono", ui-monospace, Menlo, Consolas, monospace';
  const code = spaced(face.inscription);
  const codeW = Math.min(g.measureText(code).width + 40, width);
  const codeH = 60;
  g.fillStyle = '#f6efe0';
  g.fillRect(x, y, codeW, codeH);
  g.strokeStyle = '#b08a3e';
  g.lineWidth = 3;
  g.strokeRect(x + 1.5, y + 1.5, codeW - 3, codeH - 3);
  g.strokeRect(x + 8, y + 8, codeW - 16, codeH - 16);
  g.fillStyle = '#6b4e17';
  g.fillText(code, x + 20, y + 41, codeW - 40);

  // What to do with it.
  y += codeH + 58;
  g.fillStyle = '#5b544b';
  g.font = '400 30px "IBM Plex Sans", system-ui, sans-serif';
  for (const line of scanLines) {
    g.fillText(line, x, y);
    y += 38;
  }
  return c;
}

export function cardBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('no image'))), 'image/png'));
}

/** A file name a person can recognise in their downloads or a chat. */
export function cardFileName(inscription: string): string {
  return `carte-${inscription.replace(/[^A-Za-z0-9-]+/g, '') || 'bijou'}.png`;
}

function drawQr(g: CanvasRenderingContext2D, text: string, x: number, y: number, size: number) {
  const qr = qrcode(0, 'M');
  qr.addData(text);
  qr.make();
  const n = qr.getModuleCount();
  const quiet = 4;
  const cell = Math.floor(size / (n + quiet * 2));
  const drawn = cell * (n + quiet * 2);
  const ox = x + Math.floor((size - drawn) / 2);
  const oy = y + Math.floor((size - drawn) / 2);
  g.fillStyle = '#ffffff';
  rounded(g, x, y, size, size, 16);
  g.fill();
  g.fillStyle = '#000000';
  for (let r = 0; r < n; r++) {
    for (let col = 0; col < n; col++) {
      if (qr.isDark(r, col)) g.fillRect(ox + (col + quiet) * cell, oy + (r + quiet) * cell, cell, cell);
    }
  }
}

function rounded(g: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  g.beginPath();
  g.moveTo(x + r, y);
  g.arcTo(x + w, y, x + w, y + h, r);
  g.arcTo(x + w, y + h, x, y + h, r);
  g.arcTo(x, y + h, x, y, r);
  g.arcTo(x, y, x + w, y, r);
  g.closePath();
}

/** Canvas has no letter-spacing everywhere yet; thin spaces stand in for the card's tracking. */
function spaced(s: string): string {
  return [...s].join(' ');
}

function fitText(g: CanvasRenderingContext2D, text: string, x: number, y: number, max: number) {
  let t = text;
  while (t.length > 1 && g.measureText(t).width > max) t = t.slice(0, -2) + '…';
  g.fillText(t, x, y);
}

function wrap(g: CanvasRenderingContext2D, text: string, max: number): string[] {
  const out: string[] = [];
  let line = '';
  for (const word of text.split(/\s+/).filter(Boolean)) {
    const next = line ? `${line} ${word}` : word;
    if (g.measureText(next).width <= max || !line) line = next;
    else { out.push(line); line = word; }
  }
  if (line) out.push(line);
  return out;
}
