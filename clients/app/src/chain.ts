/**
 * The one place the app touches the chain to *move* money.
 *
 * Reads go through plain fetch against the REST proxy; writes go through here,
 * because a write is signed and a signature is the only thing in this app that
 * cannot be undone. Keeping them in one file means the list of things this
 * application can do to somebody's money is a list you can read in a minute.
 */
import { ChainSigner } from '@yamale/chain';
import type { Signer } from './account.ts';

const RPC = `${window.location.origin}/api/rpc/`;
const CHAIN_ID = 'yamale-devnet-1';

/**
 * Moderators anybody may choose when opening a secured payment.
 *
 * A list rather than a constant, because "who decides if this goes wrong" is
 * the buyer's decision to make and not ours to assume. The default is the
 * service operator; a trade association, a co-operative or a chamber of
 * commerce can be added here and picked instead, and nothing in the chain
 * treats one differently from another.
 *
 * What matters is that the choice happens *before* the money moves. A moderator
 * appointed at the moment of a dispute is a moderator appointed by whoever is
 * losing, which is not moderation.
 */
export interface Moderator {
  /** Chain user id, resolved to an address at the moment of use. */
  id: string;
  name: string;
}

export const MODERATORS: Moderator[] = [
  { id: 'ZW1AKM9AT', name: 'Yamale (service operator)' },
];

function signerFor(account: Signer): ChainSigner {
  return new ChainSigner(account.offlineSigner(), {
    rpcUrl: RPC,
    chainId: CHAIN_ID,
    // Fees are paid by a grant, so the number a person sees leaving their
    // account is the number they sent.
    gasPrice: 0,
  });
}

export interface Result {
  ok: boolean;
  /** Chain error code, for mapping to a translated message. Never rendered raw. */
  code?: number;
  lockId?: string;
}

/** Commit money to an escrow. The funds leave the buyer and reach the module
 *  account — not the seller, and not us. */
export async function openEscrow(
  account: Signer,
  beneficiary: string,
  moderator: string,
  amount: string,
  denom: string,
  memo: string,
): Promise<Result> {
  const signer = signerFor(account);
  const depositor = await signer.address();
  const res = await signer.submit([{
    typeUrl: '/blockchain.treasury.v1.MsgOpenEscrow',
    value: { depositor, beneficiary, moderator, amount: { denom, amount }, memo },
  }], memo, 250_000);

  // The lock id is not read out of the response. Decoding reply bytes here
  // would put protobuf handling in the one file that should stay readable, and
  // the escrow list is re-read from the chain anyway — which is also the only
  // version of the truth that survives closing the app.
  return { ok: res.succeeded, code: res.code, lockId: undefined };
}

/** Confirm delivery. Only the buyer can, which is the whole condition. */
export async function releaseEscrow(account: Signer, lockId: string): Promise<Result> {
  const signer = signerFor(account);
  const depositor = await signer.address();
  const res = await signer.submit([{
    typeUrl: '/blockchain.treasury.v1.MsgReleaseEscrow',
    value: { depositor, lockId },
  }], '', 200_000);
  return { ok: res.succeeded, code: res.code };
}

/** Open a case. Either party may, which is why no deadline is needed. */
export async function disputeEscrow(account: Signer, lockId: string, reason: string): Promise<Result> {
  const signer = signerFor(account);
  const party = await signer.address();
  const res = await signer.submit([{
    typeUrl: '/blockchain.treasury.v1.MsgDisputeEscrow',
    value: { party, lockId, reason },
  }], '', 200_000);
  return { ok: res.succeeded, code: res.code };
}

/** Every escrow this account is a party to, read from the chain. */
export async function myEscrows(address: string): Promise<unknown[]> {
  try {
    const res = await fetch(
      `/api/rest/yamale/blockchain/treasury/v1/beneficiary/${address}/locks?pagination.limit=100`,
    );
    if (!res.ok) return [];
    const json = await res.json();
    // QueryLocksByBeneficiaryResponse names the field `lock`, singular.
    return json.lock ?? [];
  } catch {
    return [];
  }
}

/**
 * Give the account its user ID, if it does not have one.
 *
 * Identifiers are free and arrive with the account. Making somebody go and ask
 * for one would mean an account that cannot be paid until its owner discovers
 * a screen they had no reason to look for — and being payable is the whole
 * point of having an account here.
 *
 * Runs on every sign-in rather than only at sign-up, because an account created
 * before the module existed, or one whose registration failed while the node
 * was unreachable, is otherwise stuck without one forever.
 */
export async function ensureUserId(account: Signer): Promise<string | null> {
  const signer = signerFor(account);
  const address = await signer.address();

  const existing = await lookupId(address);
  if (existing) return existing;

  const res = await signer.submit([{
    typeUrl: '/blockchain.alias.v1.MsgRegisterAlias',
    // The field is 'account', not 'address'. A mismatched name encodes as an
    // empty signer rather than failing loudly, so the node refuses a message
    // that looked correct on the way out.
    value: { account: address },
  }], '', 200_000);
  if (!res.succeeded) return null;

  // Read it back rather than assuming. The identifier is derived by the chain,
  // so the chain is the only thing that knows which one this account got.
  return lookupId(address);
}

async function lookupId(address: string): Promise<string | null> {
  try {
    const res = await fetch(`/api/rest/yamale/blockchain/alias/v1/alias_of/${address}`);
    if (!res.ok) return null;
    const json = await res.json();
    return json.alias?.id ?? null;
  } catch {
    return null;
  }
}

/**
 * Change one currency into another.
 *
 * `minOut` is required by this function rather than defaulted, because a swap
 * without a floor is a swap somebody else decides the price of. Making the
 * caller pass it means no screen can quietly omit it.
 */
export async function swap(
  account: Signer,
  poolId: string,
  fromDenom: string,
  amountIn: string,
  toDenom: string,
  minOut: string,
): Promise<Result> {
  const signer = signerFor(account);
  const sender = await signer.address();
  const res = await signer.submit([{
    typeUrl: '/blockchain.amm.v1.MsgSwap',
    value: {
      sender, poolId,
      tokenInDenom: fromDenom, tokenInAmount: amountIn,
      tokenOutDenom: toDenom, minAmountOut: minOut,
    },
  }], '', 300_000);
  return { ok: res.succeeded, code: res.code };
}

/**
 * Send every coin this account holds to another account.
 *
 * One MsgSend carrying every denomination, not one per currency: a sweep that
 * is seven transactions can half-succeed, and half a sweep before an account is
 * abandoned is money left in a wallet nobody will open again.
 *
 * A little YML is held back when it is the only thing that could pay for the
 * transaction. Fees are normally covered by the faucet's allowance, but an
 * allowance that has lapsed would otherwise turn "send everything" into a
 * transaction that cannot afford itself — and it fails *after* the person has
 * agreed to close their account.
 */
const FEE_RESERVE = 5_000n;

export async function sweepTo(account: Signer, toAddress: string): Promise<Result & { sent: number }> {
  const signer = signerFor(account);
  const sender = await signer.address();

  const res = await fetch(`/api/rest/cosmos/bank/v1beta1/balances/${sender}`);
  if (!res.ok) return { ok: false, code: -1, sent: 0 };
  const balances: { denom: string; amount: string }[] = (await res.json()).balances ?? [];

  const coins: { denom: string; amount: string }[] = [];
  for (const b of balances) {
    let amount = BigInt(b.amount);
    if (b.denom === 'uyml' && amount > FEE_RESERVE) amount -= FEE_RESERVE;
    else if (b.denom === 'uyml') continue;
    if (amount > 0n) coins.push({ denom: b.denom, amount: amount.toString() });
  }
  if (coins.length === 0) return { ok: true, code: 0, sent: 0 };

  // Sorted by denom: the bank module rejects coins that are not in canonical
  // order, and REST returns them in whatever order the store iterated.
  coins.sort((a, b) => (a.denom < b.denom ? -1 : 1));

  const out = await signer.submit([{
    typeUrl: '/cosmos.bank.v1beta1.MsgSend',
    value: { fromAddress: sender, toAddress, amount: coins },
  }], '', 300_000);

  return { ok: out.succeeded, code: out.code, sent: coins.length };
}
