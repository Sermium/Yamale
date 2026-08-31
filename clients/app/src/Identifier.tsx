/**
 * "Nobody can pay you yet", said out loud.
 *
 * This app registers an identifier for every account on sign-in and, on
 * yamale-devnet-2, that registration fails in the block: x/alias refuses to
 * issue an identifier to an account whose jurisdiction nobody has recorded, and
 * only an approved participant or a foundation administrator can record one.
 *
 * Before this, the failure was silent. `ensureUserId` returned null, the app
 * carried on, the request screen had nothing to show, and the person was left
 * with an account that could send money and could never receive any — with
 * nothing anywhere telling them so. On a payments product that is the worst
 * class of bug, because everything looks like it worked.
 *
 * Nothing here offers a fix, because the person holding the phone does not have
 * one. It says what is true and who can change it, which is the most an honest
 * screen can do about somebody else's permission.
 */
import { t } from '@yamale/chain';

import type { Signer } from './account.ts';
import { useMyUserId } from './identity.ts';

export function PayableNote({ signer }: { signer: Signer }) {
  const identity = useMyUserId(signer);

  // Silent in both the ordinary cases: an account that has an identifier, and
  // one whose registration is still in flight. A warning that appears for five
  // seconds on every sign-in is a warning people learn to wait out.
  if (identity.state !== 'absent') return null;

  return (
    <p className="notice notice--bad" role="status">
      <strong>{t('iso.notPayable')}</strong>{' '}{t('iso.notPayableWhy')}
    </p>
  );
}
