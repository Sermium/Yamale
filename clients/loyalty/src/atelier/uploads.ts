import { t } from '@yamale/chain';

import { uploadFile } from '../api.ts';
import { CHAIN_ID } from '../config.ts';
import { preparePhoto } from './photo.ts';

type Up = { ok: true; hash: string } | { ok: false; text: string };

function failure(status: number, error: string): string {
  // 503: loyalty-api is busy with other uploads. It says nothing about this file.
  return status === 503 ? t('loyalty.uploadBusy') : t('loyalty.uploadFailed', { error });
}

/** Re-encode a photo on this device, then store it under its fingerprint. */
export async function uploadPhoto(file: File, withKey: <T>(use: (k: Uint8Array) => T) => T): Promise<Up> {
  let bytes: Uint8Array;
  try {
    bytes = await preparePhoto(file);
  } catch {
    return { ok: false, text: t('loyalty.photoUnreadable', { name: file.name }) };
  }
  const up = await uploadFile(withKey((k) => k), bytes, CHAIN_ID);
  return up.ok ? up : { ok: false, text: failure(up.status, up.error) };
}

export async function uploadRecord(bytes: Uint8Array, withKey: <T>(use: (k: Uint8Array) => T) => T): Promise<Up> {
  const up = await uploadFile(withKey((k) => k), bytes, CHAIN_ID);
  return up.ok ? up : { ok: false, text: failure(up.status, up.error) };
}
