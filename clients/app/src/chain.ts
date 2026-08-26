/**
 * The one place the app touches the chain to *move* money.
 *
 * Reads go through plain fetch against the REST proxy; writes go through here,
 * because a write is signed and a signature is the only thing in this app that
 * cannot be undone. Keeping them in one file means the list of things this
 * application can do to somebody's money is a list you can read in a minute.
 */
import { ChainSigner, translateError, type TranslatedError } from '@yamale/chain';
import type { Signer } from './account.ts';
import { MEMO_LIMIT } from './iso.ts';

const RPC = `${window.location.origin}/api/rpc/`;
const CHAIN_ID = 'yamale-devnet-2';

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

/**
 * What actually happened to a payment, as opposed to what was attempted.
 *
 * `ok` is set from execution and not from broadcast. Four separate bugs on this
 * project came from believing that a `code: 0` reply to a broadcast meant the
 * transaction had run: it means the node put it in its mempool. `submit()`
 * waits for the block for exactly this reason.
 */
export interface PaymentOutcome {
  ok: boolean;
  /** Empty when the transaction never reached a block. */
  hash: string;
  height: number;
  error?: TranslatedError;
  /**
   * Which rails carried it, so a receipt can say so rather than implying more
   * than happened. See `pay` for why this is only ever `transfer` today.
   */
  rails: 'transfer';
  /** Where the reference ended up, for a receipt that has to be honest. */
  reference: { value: string; on: 'memo' | 'none' };
}

/**
 * Pay somebody.
 *
 * **This sends a bank transfer, not an ISO 20022 payment instruction, and the
 * distinction is not cosmetic.** The chain has a message for the second thing —
 * `MsgSendPayment` in x/paymsg, which writes a queryable PaymentRecord with an
 * end-to-end id, a purpose code and both participants' identities — and it is
 * unreachable from this app today, for a reason that is not a client bug:
 *
 *   - `MsgSendPayment` requires that both the instructing and the instructed
 *     participant be governance-approved participants
 *     (`ErrNotApprovedParticipant`), and that the debtor have been *registered
 *     as a customer* by the participant it names (`ErrNotACustomer`). See
 *     x/paymsg/keeper/msg_server_send_payment.go.
 *   - `yamale-devnet-2` currently has **zero** approved participants, so there
 *     is no pair of institutions any account here could legitimately name.
 *   - The app cannot even discover its own standing: the whole
 *     `/api/rest/yamale/blockchain/paymsg/` prefix sits behind supervisor
 *     credentials under the split-visibility policy, so a browser gets a 401
 *     asking whether it is anybody's customer.
 *
 * Given that, the choice was between a receipt for a payment that never
 * happened — which is what this screen used to show — and a real transfer that
 * says what it is. The reference travels in the transaction memo: a real field,
 * on the ledger, queryable, and enough to reconcile against. It is not a
 * PaymentRecord and the receipt does not pretend it is.
 *
 * One correction to the third bullet above, found by pointing this at the
 * running chain rather than reading the nginx config: the REST prefix is
 * gated, but the same queries answer over the node's ABCI interface at
 * /api/rpc/, which is not. So the app *can* discover the chain's standing and
 * now does — see standing.ts. It reads zero approved participants at a stated
 * block height instead of asserting it.
 *
 * `memo` arrives already encoded. Building it here would put the ISO field
 * layout in the file that signs transactions, and the point of this file is
 * that the list of things it can do to somebody's money reads in a minute.
 */
export async function pay(
  account: Signer,
  toAddress: string,
  amount: string,
  denom: string,
  memoIn: string,
): Promise<PaymentOutcome> {
  const signer = signerFor(account);
  const from = await signer.address();

  const memo = memoIn.trim();
  const reported = { value: memo, on: memo === '' ? ('none' as const) : ('memo' as const) };

  // Refused here as well as prevented in the form. A memo over the node's
  // `max_memo_characters` is rejected after signing, and no screen should be
  // able to reach that by forgetting a check of its own.
  if (memo.length > MEMO_LIMIT) {
    return {
      ok: false, hash: '', height: 0, rails: 'transfer', reference: reported,
      error: translateError(`memo is too long: ${memo.length} > ${MEMO_LIMIT}`),
    };
  }

  try {
    const res = await signer.submit(
      [{
        typeUrl: '/cosmos.bank.v1beta1.MsgSend',
        value: { fromAddress: from, toAddress, amount: [{ denom, amount }] },
      }],
      memo,
      200_000,
    );

    return {
      ok: res.succeeded,
      hash: res.hash,
      height: res.height,
      error: res.error,
      rails: 'transfer',
      reference: reported,
    };
  } catch (err) {
    // Reached when the node is unreachable rather than when it refuses — a
    // refusal comes back through `submit` as a translated error.
    return {
      ok: false,
      hash: '',
      height: 0,
      error: translateError(err instanceof Error ? err.message : String(err)),
      rails: 'transfer',
      reference: reported,
    };
  }
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
