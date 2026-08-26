/**
 * Whether this account has an identifier, and the honest three-way answer.
 *
 * Being payable is the whole point of having an account here, and an account is
 * payable only through the user ID the chain issues it. So "does this account
 * have one" is not a detail — it is the difference between an account and a
 * dead end.
 *
 * The three states matter separately. `looking` is the ordinary case for the
 * first few seconds after sign-up, because the identifier is registered in a
 * transaction that has to reach a block. `absent` is a real answer and means
 * nobody can pay this account. Collapsing the two would either flash a warning
 * at every user for five seconds or hide a permanent failure behind a spinner,
 * and both were on the table before this was split out.
 *
 * Verified against yamale-devnet-2: an account created through this app is
 * refused an identifier with `this account has no recorded jurisdiction`
 * (codespace alias), because x/alias will not issue one to an account no
 * approved participant or foundation administrator has placed. The registration
 * is broadcast, included in a block, and fails there — which is exactly the
 * kind of failure that reads as success to anything watching the broadcast.
 */
import { useEffect, useState } from 'react';

import type { Signer } from './account.ts';
import * as book from './book.ts';

export interface Identity {
  address: string;
  /** Empty unless `state` is 'found'. */
  userId: string;
  state: 'looking' | 'found' | 'absent';
}

/** How many times to ask, and how long between. Six over half a minute covers
 *  a registration landing in a block; past that it is not coming. */
const ATTEMPTS = 6;
const GAP_MS = 5000;

export function useMyUserId(signer: Signer): Identity {
  const [identity, setIdentity] = useState<Identity>({ address: '', userId: '', state: 'looking' });

  useEffect(() => {
    let live = true;
    setIdentity({ address: '', userId: '', state: 'looking' });

    (async () => {
      const address = await signer.internalAddress();
      if (!live) return;
      setIdentity({ address, userId: '', state: 'looking' });

      for (let attempt = 0; attempt < ATTEMPTS && live; attempt++) {
        const id = await book.myUserId(address);
        if (!live) return;
        if (id) { setIdentity({ address, userId: id, state: 'found' }); return; }
        await new Promise((resolve) => setTimeout(resolve, GAP_MS));
      }

      // Not a permanent poll. The identifier does not appear on its own — it
      // takes somebody with the authority to record a jurisdiction — so
      // continuing to ask would spend a data plan on a fact that will not
      // change while this screen is open.
      if (live) setIdentity({ address, userId: '', state: 'absent' });
    })();

    return () => { live = false; };
  }, [signer]);

  return identity;
}
