import { AccountName } from '@yamale/chain';

import { client } from './chain.ts';

/** An account, named by the shared rule. See the SDK's name.tsx. */
export function Named({ address, full }: { address: string; full?: boolean }) {
  return <AccountName address={address} lookup={(a) => client.userIdOf(a)} full={full} />;
}
