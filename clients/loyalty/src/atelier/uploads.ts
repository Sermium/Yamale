import { t } from '@yamale/chain';

import { uploadFile } from '../api.ts';
import { CHAIN_ID } from '../config.ts';
import { preparePhoto } from './photo.ts';

/** `raw` is the API's own text: shown under Advanced, never inline. */
export type Up = { ok: true; hash: string } | { ok: false; text: string; raw: string };
type WithKey = <T>(use: (k: Uint8Array) => T) => T;

function failure(status: number, error: string): Up {
  // 503: loyalty-api is busy with other uploads. It says nothing about this file.
  const raw = `${status} ${error}`.trim();
  return { ok: false, text: status === 503 ? t('loyalty.uploadBusy') : t('loyalty.uploadFailed'), raw };
}

/** Re-encode a photo on this device, then store it under its fingerprint. */
export async function uploadPhoto(file: File, withKey: WithKey): Promise<Up> {
  let bytes: Uint8Array;
  try {
    bytes = await preparePhoto(file);
  } catch (e) {
    return { ok: false, text: t('loyalty.photoUnreadable', { name: file.name }), raw: e instanceof Error ? e.message : String(e) };
  }
  const up = await uploadFile(withKey((k) => k), bytes, CHAIN_ID);
  return up.ok ? up : failure(up.status, up.error);
}

export async function uploadRecord(bytes: Uint8Array, withKey: WithKey): Promise<Up> {
  const up = await uploadFile(withKey((k) => k), bytes, CHAIN_ID);
  return up.ok ? up : failure(up.status, up.error);
}
