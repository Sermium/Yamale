/**
 * The card as an image: shared to a chat on a phone, copied to paste into an
 * email or a chat on a computer, or saved. Whichever the device can do is
 * offered; download always works.
 */

import { useState } from 'react';
import { t } from '@yamale/chain';

import { cardBlob, cardFileName, drawCard, type CardFace } from './cardImage.ts';

type State = 'idle' | 'busy' | 'copied' | 'failed';

export function CardImageButtons({ face }: { face: CardFace }) {
  const [state, setState] = useState<State>('idle');
  const name = cardFileName(face.inscription);
  const canCopy = typeof window !== 'undefined' && 'ClipboardItem' in window && !!navigator.clipboard?.write;

  async function image(): Promise<Blob> {
    return cardBlob(await drawCard(face));
  }

  async function share() {
    setState('busy');
    try {
      const file = new File([await image()], name, { type: 'image/png' });
      if (navigator.canShare?.({ files: [file] })) {
        await navigator.share({ files: [file], title: face.title, text: `${face.shop} - ${face.link}` });
        setState('idle');
      } else {
        download(file);
        setState('idle');
      }
    } catch (e) {
      // Closing the share sheet is not a failure.
      setState(e instanceof DOMException && e.name === 'AbortError' ? 'idle' : 'failed');
    }
  }

  async function copy() {
    setState('busy');
    try {
      // The promise form keeps Safari's user-gesture rule satisfied while the image is drawn.
      await navigator.clipboard.write([new ClipboardItem({ 'image/png': image() })]);
      setState('copied');
    } catch {
      setState('failed');
    }
  }

  async function save() {
    setState('busy');
    try {
      download(new File([await image()], name, { type: 'image/png' }));
      setState('idle');
    } catch {
      setState('failed');
    }
  }

  const canShareFiles = typeof navigator !== 'undefined' && 'canShare' in navigator;
  return (
    <div className="stack">
      <div className="actions">
        {canShareFiles && <button type="button" className="btn btn--primary" disabled={state === 'busy'} onClick={share}>{t('traceability.cardImageShare')}</button>}
        {canCopy && <button type="button" className="btn" disabled={state === 'busy'} onClick={copy}>{state === 'copied' ? t('traceability.cardImageCopied') : t('traceability.cardImageCopy')}</button>}
        <button type="button" className="btn" disabled={state === 'busy'} onClick={save}>{t('traceability.cardImageSave')}</button>
      </div>
      {state === 'failed' && <p className="note note--warn" role="alert">{t('traceability.cardImageFailed')}</p>}
    </div>
  );
}

function download(file: File) {
  const url = URL.createObjectURL(file);
  const a = document.createElement('a');
  a.href = url;
  a.download = file.name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}
