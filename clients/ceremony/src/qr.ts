// An invite link as a QR code.
//
// It is here because custodians will be on phones, and nobody types a URL with a
// two-hundred-and-fifty-six-bit token in it correctly. A link that has to be
// retyped is a link that gets shortened, and a shortened invite token is a
// custodian staring at "this invitation is not one this ceremony issued".
//
// The encoder is qrcode-generator, the same one clients/app already uses. Not
// written here: Reed-Solomon error correction is exactly the kind of code that
// works on the first twelve inputs and fails on the thirteenth, and the
// thirteenth would be the link somebody could not scan with four other
// custodians waiting.

import qrcode from 'qrcode-generator';

// SVG rather than a canvas or a data URI. The page's Content-Security-Policy
// allows img-src data: for exactly this, but an inline SVG scales to whatever the
// phone's camera needs and stays crisp when the coordinator zooms the browser to
// show it across a desk.
export function qrSVG(text: string, size = 148): SVGElement {
  // Type number 0 lets the library pick the smallest version that fits, and 'M'
  // is fifteen per cent error correction: enough for a screen photographed at an
  // angle, not so much that the modules shrink below what a camera resolves.
  const qr = qrcode(0, 'M');
  qr.addData(text);
  qr.make();

  const count = qr.getModuleCount();
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  // A quiet zone of four modules, because a code butted against a coloured panel
  // is a code half the scanners refuse.
  const quiet = 4;
  const span = count + quiet * 2;
  svg.setAttribute('viewBox', `0 0 ${span} ${span}`);
  svg.setAttribute('width', String(size));
  svg.setAttribute('height', String(size));
  svg.setAttribute('role', 'img');
  svg.setAttribute('aria-label', 'invitation link as a QR code');
  svg.setAttribute('class', 'qr');

  const background = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
  background.setAttribute('width', String(span));
  background.setAttribute('height', String(span));
  background.setAttribute('fill', '#ffffff');
  svg.append(background);

  let path = '';
  for (let row = 0; row < count; row++) {
    for (let column = 0; column < count; column++) {
      if (qr.isDark(row, column)) {
        path += `M${column + quiet} ${row + quiet}h1v1h-1z`;
      }
    }
  }
  const modules = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  modules.setAttribute('d', path);
  // Black on white regardless of the page's theme. A QR code rendered in the
  // dark palette inverts, and inverted codes are a coin flip across scanners.
  modules.setAttribute('fill', '#000000');
  svg.append(modules);

  return svg;
}
