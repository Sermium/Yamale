/**
 * The signed-in state: which account this device holds, whether its key is
 * open in memory, and the one way the app sends a transaction.
 *
 * The key lives in a ref, not in React state, so it never reaches a devtools
 * snapshot or a re-render log. It is dropped after ten minutes without use,
 * and on lock or reload.
 */

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { DirectSecp256k1Wallet } from '@cosmjs/proto-signing';
import { ChainSigner, JEWEL_GAS, type EncodeObject, type SubmitResult } from '@yamale/chain';

import { enrol, servicesAvailable } from '../../../wallet/src/account/enrol.ts';
import { forgetEnrolment, rememberEnrolment } from '../../../wallet/src/account/stored.ts';
import { requestSponsorship, sponsorInfo } from '../api.ts';
import { CHAIN_ID, GAS_PRICE, RPC_ENDPOINT, SERVICES } from '../config.ts';
import {
  ACCOUNT_EXISTS, adoptKey, createAccount, forgetAccount, readAccount, unlockAccount, updateAccount, type StoredAccount,
} from '../../../shared/account/device.ts';
import { protectionOutcome } from '../../../shared/account/protection.ts';
import { takeHandoffKey } from '../../../shared/account/handoff.ts';

/** The sponsor's current message list version (traceabilityapi ownerList). */
export const SPONSOR_LIST = 4;
import { sessionAccountState } from './sessionState.ts';

const IDLE_MS = 10 * 60 * 1000;

export interface Session {
  account: StoredAccount | null;
  address: string | null;
  unlocked: boolean;
  sponsor: string | null;
  create(email: string, password: string): Promise<void>;
  unlock(password: string): Promise<void>;
  lock(): void;
  forget(): void;
  /** Re-reads the account from this device's storage, locked. For an account written by another route after mount. */
  refresh(): void;
  adopt(privateKey: Uint8Array, email: string, password: string): Promise<void>;
  withKey<T>(use: (key: Uint8Array) => T): T;
  submit(messages: EncodeObject[], granter: string | null): Promise<SubmitResult>;
  submitAsOwner(messages: EncodeObject[]): Promise<SubmitResult>;
  ensureSponsored(): Promise<'sponsored' | 'unavailable' | 'refused'>;
  /** Throws WrongPassword when the password does not open this account's key. */
  protect(password: string): Promise<'enrolled' | 'unavailable' | 'email-in-use'>;
  /**
   * The automatic save of an account that exists only on this device, started
   * when it is unlocked: nobody should have to find a button for it.
   */
  saving: 'idle' | 'busy' | 'failed' | 'unavailable' | 'email-in-use';
}

/** Owners hold no coins: without the platform's allowance an owner cannot act. */
export class SponsorshipUnavailable extends Error {
  constructor(public readonly state: 'unavailable' | 'refused') {
    super(`fees cannot be sponsored: ${state}`);
    this.name = 'SponsorshipUnavailable';
  }
}

const SessionContext = createContext<Session | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [account, setAccount] = useState<StoredAccount | null>(() => sessionAccountState().account);
  const [unlocked, setUnlocked] = useState(false);
  const [sponsor, setSponsor] = useState<string | null>(null);
  const [saving, setSaving] = useState<Session['saving']>('idle');
  // The granter as of now, read by submitAsOwner. State alone would hand a
  // closure the value from the render it was created in.
  const sponsorRef = useRef<string | null>(null);
  const key = useRef<Uint8Array | null>(null);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => { sponsorInfo().then((g) => { sponsorRef.current = g; setSponsor(g); }); }, []);

  const lock = useCallback(() => {
    window.clearTimeout(timer.current);
    key.current?.fill(0);
    key.current = null;
    setUnlocked(false);
  }, []);

  const touch = useCallback(() => {
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(lock, IDLE_MS);
  }, [lock]);

  const hold = useCallback((k: Uint8Array) => { key.current?.fill(0); key.current = k; setUnlocked(true); touch(); }, [touch]);

  // An account handed over by accounts.yamaletech.com right after signing in
  // there brings its open key: it opens at once, in memory only.
  useEffect(() => {
    const handed = takeHandoffKey();
    if (!handed) return;
    const here = readAccount();
    if (here && here.address === handed.account.address) { setAccount(here); hold(handed.key); } else handed.key.fill(0);
  }, [hold]);

  const withKey = useCallback(<T,>(use: (k: Uint8Array) => T): T => {
    if (!key.current) throw new Error('locked');
    touch();
    return use(key.current);
  }, [touch]);

  const ensureSponsored = useCallback(async () => {
    const current = readAccount();
    if (!current || !key.current) return 'refused' as const;
    if (current.sponsored && (current.sponsorList ?? 1) >= SPONSOR_LIST) return 'sponsored' as const;
    const out = await requestSponsorship(key.current, current.address, CHAIN_ID);
    if (out.ok) {
      setAccount(updateAccount({ sponsored: true, sponsorList: SPONSOR_LIST }));
      sponsorRef.current = out.granter;
      setSponsor(out.granter);
      return 'sponsored' as const;
    }
    return out.status === 0 || out.status >= 500 ? 'unavailable' as const : 'refused' as const;
  }, []);

  const protect = useCallback(async (password: string) => {
    const current = readAccount();
    if (!current || !key.current) throw new Error('locked');
    // The password sealed into the service copies must be the one that opens
    // this key; a typo would seal them under a password nobody set.
    (await unlockAccount(current, password)).fill(0);
    if (!(await servicesAvailable(SERVICES))) return 'unavailable' as const;
    if (!key.current) throw new Error('locked');
    const done = await enrol(key.current, current.email, password, SERVICES);
    // Only brand-new keys are enrolled here, so a 409 is another key's copies.
    if (protectionOutcome(done) === 'email-in-use') return 'email-in-use' as const;
    rememberEnrolment({ address: done.address, email: current.email, holding: done.deviceHolding,
      enrolledAt: new Date().toISOString() });
    setAccount(updateAccount({ protection: 'enrolled' }));
    return 'enrolled' as const;
  }, []);

  const submitWith = useCallback(async (messages: EncodeObject[], granter: string | null) => {
    const wallet = await withKey((k) => DirectSecp256k1Wallet.fromKey(k, 'yml'));
    const signer = new ChainSigner(wallet, {
      rpcUrl: RPC_ENDPOINT, chainId: CHAIN_ID, gasPrice: GAS_PRICE, ...(granter ? { feeGranter: granter } : {}),
    });
    try {
      return await signer.submit(messages, '', JEWEL_GAS);
    } finally {
      signer.disconnect();
    }
  }, [withKey]);

  const submitAsOwner = useCallback(async (messages: EncodeObject[]) => {
    const state = await ensureSponsored();
    if (state !== 'sponsored') throw new SponsorshipUnavailable(state);
    if (!sponsorRef.current) sponsorRef.current = await sponsorInfo();
    if (!sponsorRef.current) throw new SponsorshipUnavailable('unavailable');
    return submitWith(messages, sponsorRef.current);
  }, [ensureSponsored, submitWith]);

  const value = useMemo<Session>(() => ({
    account,
    address: account?.address ?? null,
    unlocked,
    sponsor,
    async create(email, password) {
      const made = await createAccount(email, password);
      setAccount(made.account);
      hold(made.privateKey);
    },
    async unlock(password) {
      if (!account) throw new Error('no account on this device');
      hold(await unlockAccount(account, password));
      // An account made before saving was part of creation, or whose save
      // failed, is saved now with the password just proven, in the
      // background: the person carries on, and the banner says how it went.
      if (account.protection !== 'enrolled') {
        setSaving('busy');
        protect(password).then(
          (out) => setSaving(out === 'enrolled' ? 'idle' : out),
          () => setSaving('failed'));
      }
    },
    lock,
    forget() {
      lock();
      if (account) forgetEnrolment(account.address);
      forgetAccount();
      setAccount(null);
    },
    refresh() {
      lock();
      setAccount(sessionAccountState().account);
    },
    async adopt(privateKey, email, password) {
      if (readAccount()) throw new Error(ACCOUNT_EXISTS);
      const adopted = await adoptKey(privateKey, email, password);
      setAccount(updateAccount({ protection: 'enrolled', sponsored: adopted.sponsored }));
      hold(privateKey);
    },
    withKey,
    submit: submitWith,
    submitAsOwner,
    ensureSponsored,
    protect,
    saving,
  }), [account, unlocked, sponsor, hold, lock, withKey, submitWith, submitAsOwner, ensureSponsored, protect, saving]);

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): Session {
  const s = useContext(SessionContext);
  if (!s) throw new Error('useSession outside SessionProvider');
  return s;
}
