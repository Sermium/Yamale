/**
 * What a vehicle is, in the terms an investor decides in.
 *
 * Everything here is pure and takes the chain's own numbers as strings. That is
 * deliberate: a shareholding, a bond and a redemption payout are all computed
 * on this chain by integer arithmetic in a keeper, and the only way an interface
 * can promise a figure is to do the same arithmetic on the same integers. Every
 * function below that mirrors a keeper says which one, because when the keeper
 * changes, these have to change with it or the app quietly starts lying about
 * money.
 *
 * Nothing here formats. Formatting is a display-boundary concern and lives in
 * money.ts; a module that both computes a payout and decides how many decimal
 * places to print it to is a module where a rounding rule can leak into a
 * transfer amount.
 */

/* ------------------------------------------------------------ chain shapes */

/** blockchain.tokenisation.v1.VerificationMode, as the wire carries it. */
export type Verification =
  | 'VERIFICATION_UNSPECIFIED'
  | 'VERIFY_VALUER'
  | 'VERIFY_ATTESTORS'
  | 'VERIFY_GOVERNANCE'
  | 'VERIFY_SCHEDULE';

/** blockchain.tokenisation.v1.Status. */
export type VehicleStatus =
  | 'STATUS_UNSPECIFIED'
  | 'STATUS_HELD'
  | 'STATUS_ACTIVE'
  | 'STATUS_REPORTED'
  | 'STATUS_REALISED'
  | 'STATUS_CLOSED'
  | 'STATUS_DISPUTED';

export interface Collection {
  id: string;
  authority: string;
  verification: Verification;
  attestationThreshold: number;
  challengeWindowSeconds: number;
  disputeBondBps: number;
}

export interface Asset {
  id: string;
  collectionId: string;
  owner: string;
  uri: string;
  fractionDenom: string;
  holderShareBps: number;
  status: VehicleStatus;
  /** The x/land parcel, or '0' when the vehicle is not over land. */
  parcelId: string;
}

export interface Coin {
  denom: string;
  amount: string;
}

export interface Vault {
  assetId: string;
  cumulativePerToken: string;
  funded: Coin[];
  denom: string;
}

export interface SaleReport {
  assetId: string;
  price: Coin;
  reporter: string;
  reportedAt: Date;
  claimableAt: Date;
  attestors: string[];
  disputed: boolean;
}

/* --------------------------------------------------------- ids and the zero */

/**
 * Whether a number that arrived on the wire names a real record.
 *
 * In proto3 an id of 0 is byte-identical to an absent field, so a decoder hands
 * back 0 for "the sponsor did not set this" and for "record zero" alike. Both
 * of the sequences this app reads deliberately skip zero for that reason —
 * x/tokenisation's MintAsset increments past it, and x/land's RegisterParcel
 * draws a second id when the first is 0 — so on this chain a zero is always the
 * absent case and never a record.
 *
 * Written as a named function rather than as `!== '0'` at eleven call sites,
 * because the reasoning above is what makes the check correct and it does not
 * survive being inlined.
 */
export function isRealId(id: string | number | undefined | null): boolean {
  if (id === undefined || id === null) return false;
  const text = String(id).trim();
  if (!/^\d+$/.test(text)) return false;
  return BigInt(text) > 0n;
}

/** True when this vehicle is over a registered parcel, so x/land governs it. */
export function isOverLand(asset: Pick<Asset, 'parcelId'>): boolean {
  return isRealId(asset.parcelId);
}

/* -------------------------------------------------------------- percentages */

/**
 * Basis points as a percentage number: 4000 → 40.
 *
 * A plain division, kept here so that no screen writes `bps / 100` and no
 * screen writes `bps / 10000` by mistake. The two differ by a factor of a
 * hundred and both look plausible in a diff.
 */
export function bpsToPercent(bps: number): number {
  return bps / 100;
}

export interface Shareholding {
  /** Share of the token supply, as a percentage. Null when supply is zero. */
  ofSupply: number | null;
  /**
   * Share of the *asset's economics*, as a percentage. This is the number an
   * investor actually holds, and it is strictly smaller than `ofSupply`
   * whenever the sponsor kept anything back.
   */
  ofAsset: number | null;
  /** True when this account holds nothing of the vehicle. */
  empty: boolean;
}

/**
 * What a token balance actually means.
 *
 * Two percentages rather than one, because they answer different questions and
 * an app that shows only the first is overstating what somebody owns. A holder
 * of half the tokens in a vehicle whose tokens carry 40% of the asset owns 20%
 * of the asset, not 50% of it, and the gap is the sponsor's retained share.
 *
 * Computed in floating point on purpose: this is a percentage for a human to
 * read, never an amount that moves. Every figure that moves money in this file
 * is BigInt. The ratio is taken at 1e9 precision before the division so that a
 * holding of one token in a supply of a billion does not collapse to zero.
 */
export function shareholding(
  balance: string,
  supply: string,
  holderShareBps: number,
): Shareholding {
  let held: bigint;
  let total: bigint;
  try {
    held = BigInt(balance || '0');
    total = BigInt(supply || '0');
  } catch {
    return { ofSupply: null, ofAsset: null, empty: true };
  }

  if (total <= 0n) return { ofSupply: null, ofAsset: null, empty: held <= 0n };
  if (held <= 0n) return { ofSupply: 0, ofAsset: 0, empty: true };

  const SCALE = 1_000_000_000n;
  const ratio = Number((held * SCALE) / total) / Number(SCALE);

  return {
    ofSupply: ratio * 100,
    ofAsset: ratio * holderShareBps / 100,
    empty: false,
  };
}

/* -------------------------------------------------------------- protections */

export type ProtectionLevel = 'none' | 'weak' | 'standard' | 'strong';

export interface ProtectionFinding {
  /** Catalogue key for the sentence. */
  key: string;
  tone: 'ok' | 'warn' | 'bad';
  values?: Record<string, string | number>;
}

export interface Protection {
  level: ProtectionLevel;
  findings: ProtectionFinding[];
}

/** A day, in seconds. The floor x/tokenisation's own params enforce. */
export const DAY = 86_400;
const WEEK = 7 * DAY;

/**
 * How much a collection actually protects the people who buy into it.
 *
 * This is the most important computation in the app, and it is about one
 * failure: the sale price. Nothing about the token supply, the vault or the
 * title stops a sponsor reporting a sale below what was really received and
 * paying every holder a proportion of a lie. Two things stop it — how many
 * independent parties have to sign the figure, and how long anybody has to
 * challenge it — and both are set per collection.
 *
 * So a collection with no verification mode and a zero window is not "less
 * configured", it is a vehicle where whoever reports the sale decides what
 * everyone else gets. It is graded `none` and it is meant to look alarming.
 *
 * The grade is the worst finding rather than a score, because a sum lets a long
 * window paper over the absence of any verification at all, and those are not
 * substitutes: a figure nobody checks is not made true by being contestable for
 * a month by people who cannot see the contract.
 */
export function protectionOf(c: Collection): Protection {
  const findings: ProtectionFinding[] = [];
  let level: ProtectionLevel = 'strong';
  const worsen = (l: ProtectionLevel) => {
    const order: ProtectionLevel[] = ['none', 'weak', 'standard', 'strong'];
    if (order.indexOf(l) < order.indexOf(level)) level = l;
  };

  // --- how the figure is checked
  switch (c.verification) {
    case 'VERIFY_ATTESTORS':
      if (c.attestationThreshold < 2) {
        // The keeper refuses this at creation. It can still arrive through a
        // genesis import, and a threshold of one is not a threshold — it is a
        // single signature away from unlimited theft.
        findings.push({ key: 'rwa.protect.thresholdTooLow', tone: 'bad' });
        worsen('none');
      } else {
        findings.push({
          key: 'rwa.protect.attestors',
          tone: 'ok',
          values: { n: c.attestationThreshold },
        });
        if (c.attestationThreshold < 3) worsen('standard');
      }
      break;
    case 'VERIFY_VALUER':
      findings.push({ key: 'rwa.protect.valuer', tone: 'ok' });
      break;
    case 'VERIFY_GOVERNANCE':
      findings.push({ key: 'rwa.protect.governance', tone: 'ok' });
      break;
    case 'VERIFY_SCHEDULE':
      findings.push({ key: 'rwa.protect.schedule', tone: 'ok' });
      break;
    default:
      findings.push({ key: 'rwa.protect.noVerification', tone: 'bad' });
      worsen('none');
  }

  // --- how long anybody has to object
  if (c.challengeWindowSeconds <= 0) {
    findings.push({ key: 'rwa.protect.noWindow', tone: 'bad' });
    worsen('none');
  } else if (c.challengeWindowSeconds < DAY) {
    findings.push({
      key: 'rwa.protect.shortWindow',
      tone: 'warn',
      values: { seconds: c.challengeWindowSeconds },
    });
    worsen('weak');
  } else {
    findings.push({
      key: 'rwa.protect.window',
      tone: 'ok',
      values: { seconds: c.challengeWindowSeconds },
    });
    if (c.challengeWindowSeconds < WEEK) worsen('standard');
  }

  // --- what it costs to object
  //
  // Reported as information rather than graded. A high bond prices a small
  // holder out of challenging the sponsor; a zero bond makes a challenge free
  // and therefore makes delay free for a competitor. Neither is a scandal on
  // its own, and neither substitutes for the two findings above.
  if (c.disputeBondBps === 0) {
    findings.push({ key: 'rwa.protect.freeChallenge', tone: 'warn' });
  } else {
    findings.push({
      key: 'rwa.protect.bond',
      tone: 'ok',
      values: { percent: bpsToPercent(c.disputeBondBps) },
    });
  }

  // --- the headline case, stated as one sentence rather than as two findings
  if (c.attestationThreshold === 0 && c.challengeWindowSeconds <= 0) {
    findings.unshift({ key: 'rwa.protect.unchecked', tone: 'bad' });
    level = 'none';
  }

  return { level, findings };
}

/**
 * The one thing that is true of every vehicle here and is unusual enough to be
 * worth saying out loud: supply is fixed at fractionalisation and there is no
 * second issuance, so a holder's percentage cannot be diluted by anybody,
 * including the sponsor.
 *
 * A function rather than a constant string so the claim is only made about
 * assets that have actually been fractionalised — before that there is no
 * supply to be fixed, and promising a protection that has not attached yet is
 * exactly the kind of statement this app exists not to make.
 */
export function dilutionProtected(asset: Pick<Asset, 'fractionDenom' | 'status'>): boolean {
  return asset.fractionDenom !== '' && asset.status !== 'STATUS_UNSPECIFIED';
}

/* ------------------------------------------------------------ the sale clock */

export type SalePhase =
  /** No price reported. Ordinary life for an ACTIVE vehicle. */
  | 'none'
  /** Reported, inside its challenge window. Disputable; not redeemable. */
  | 'in-window'
  /** Window passed, but the collection's attestations are not all in. */
  | 'awaiting-attestations'
  /** Somebody staked a bond against the figure. Everything stops. */
  | 'disputed'
  /** Window passed, verification satisfied. Nothing stands in the way. */
  | 'clear'
  /** The price is final. Redemption is open and trading has stopped. */
  | 'realised';

export interface SaleState {
  phase: SalePhase;
  /** Attestations gathered, and how many the collection requires. */
  attestations: number;
  needed: number;
  /** Seconds until the window closes; 0 once it has. */
  remainingSeconds: number;
  /** The window's full length, so a progress bar has a denominator. */
  totalSeconds: number;
  /** True while a holder may still stake a bond and stop the payout. */
  canDispute: boolean;
  /** What one would have to stake, in base units of the price's denom. */
  bond: string;
}

/**
 * Where a reported sale stands, against the clock and against the collection.
 *
 * The predicates mirror the keeper exactly, and the mirroring is the point:
 *
 *   - DisputeSale refuses once `BlockTime()` is *after* `claimable_at`, so the
 *     window is inclusive of its final instant and `canDispute` is `<=`, not
 *     `<`. An interface that closed the window a second early would tell a
 *     holder they had missed a deadline they had not missed.
 *   - DisputeSale also requires STATUS_REPORTED and refuses a second dispute,
 *     so both are conditions here rather than only the clock.
 *   - FinaliseSale requires the attestation threshold only when the mode is
 *     VERIFY_ATTESTORS. Under the other three modes the threshold field is
 *     meaningless and showing "0 of 3 attestations" against a valuer's figure
 *     would be inventing a requirement.
 *
 * `now` is passed in rather than read from the clock so this is testable and so
 * that a caller can drive it from the chain's block time — which is the clock
 * the keeper will actually use — rather than from the reader's laptop.
 */
export function saleState(
  asset: Pick<Asset, 'status'>,
  sale: SaleReport | null,
  collection: Collection,
  now: Date,
): SaleState {
  const needed = collection.verification === 'VERIFY_ATTESTORS'
    ? collection.attestationThreshold
    : 0;

  if (asset.status === 'STATUS_REALISED' || asset.status === 'STATUS_CLOSED') {
    return {
      phase: 'realised',
      attestations: sale?.attestors.length ?? 0,
      needed,
      remainingSeconds: 0,
      totalSeconds: collection.challengeWindowSeconds,
      canDispute: false,
      bond: '0',
    };
  }

  if (!sale) {
    return {
      phase: 'none',
      attestations: 0,
      needed,
      remainingSeconds: 0,
      totalSeconds: collection.challengeWindowSeconds,
      canDispute: false,
      bond: '0',
    };
  }

  const attestations = sale.attestors.length;
  const bond = disputeBond(sale.price.amount, collection.disputeBondBps);
  const remainingMs = sale.claimableAt.getTime() - now.getTime();
  const remainingSeconds = remainingMs > 0 ? Math.ceil(remainingMs / 1000) : 0;
  const inWindow = now.getTime() <= sale.claimableAt.getTime();

  if (sale.disputed || asset.status === 'STATUS_DISPUTED') {
    return {
      phase: 'disputed',
      attestations,
      needed,
      remainingSeconds,
      totalSeconds: collection.challengeWindowSeconds,
      canDispute: false,
      bond,
    };
  }

  const reported = asset.status === 'STATUS_REPORTED';

  if (inWindow) {
    return {
      phase: 'in-window',
      attestations,
      needed,
      remainingSeconds,
      totalSeconds: collection.challengeWindowSeconds,
      canDispute: reported,
      bond,
    };
  }

  return {
    phase: needed > 0 && attestations < needed ? 'awaiting-attestations' : 'clear',
    attestations,
    needed,
    remainingSeconds: 0,
    totalSeconds: collection.challengeWindowSeconds,
    canDispute: false,
    bond,
  };
}

/**
 * What a challenge costs, in base units of the reported price's denomination.
 *
 * Mirrors DisputeSale: `price.Amount.MulRaw(bps).QuoRaw(10_000)`. Integer
 * division truncates toward zero, so a bond computed in floating point and
 * rounded up would occasionally quote a figure larger than the chain will take,
 * and one rounded down on a different input would quote one it will refuse.
 * BigInt reproduces the keeper exactly.
 *
 * Returns '0' rather than throwing on rubbish, because the caller's job when
 * the bond is unknown is to refuse to offer the action, and a thrown error in
 * a render path takes the page with it.
 */
export function disputeBond(priceAmount: string, bps: number): string {
  if (!Number.isFinite(bps) || bps <= 0) return '0';
  let price: bigint;
  try {
    price = BigInt((priceAmount || '0').trim());
  } catch {
    return '0';
  }
  if (price <= 0n) return '0';
  return ((price * BigInt(Math.trunc(bps))) / 10_000n).toString();
}

/**
 * What redeeming `amount` shares would actually pay, in base units.
 *
 * Mirrors Redeem: `pos.Accrued.Mul(amount).Quo(balance)`, truncated. The
 * truncation favours the vault — a fraction of a unit that cannot be divided
 * stays behind rather than being conjured — which is the same rule that governs
 * x/amm and AccrueIncome.
 *
 * Returns null when the chain would refuse the message rather than returning a
 * zero: redeeming more than you hold, or redeeming against a zero balance, are
 * refusals with different reasons and a screen has to be able to say which.
 */
export function redeemPayout(
  accrued: string,
  amount: string,
  balance: string,
): string | null {
  let owed: bigint;
  let want: bigint;
  let held: bigint;
  try {
    owed = BigInt((accrued || '0').trim());
    want = BigInt((amount || '0').trim());
    held = BigInt((balance || '0').trim());
  } catch {
    return null;
  }
  if (want <= 0n) return null;
  if (held <= 0n) return null;
  if (want > held) return null;
  if (owed <= 0n) return '0';
  return ((owed * want) / held).toString();
}

/* --------------------------------------------------- the land authorisation */

export interface LandAuthorisation {
  parcelId: string;
  right: string;
  maxShareBps: number;
  /** Unix seconds. */
  expiresAt: number;
  grantedBy: string;
  grantedAt: number;
  withdrawn: boolean;
  withdrawnAt: number;
}

export type LandGate =
  /** Not over a parcel. x/land governs nothing here. */
  | { kind: 'not-land' }
  /** The registry never permitted it, or the record could not be read. */
  | { kind: 'absent' }
  | { kind: 'unreachable' }
  | { kind: 'withdrawn'; auth: LandAuthorisation }
  | { kind: 'expired'; auth: LandAuthorisation }
  /** Live permission, but the parcel has since acquired a restriction. */
  | { kind: 'restricted'; auth: LandAuthorisation }
  | { kind: 'live'; auth: LandAuthorisation };

/**
 * Whether the registry's permission to sell shares in this land actually
 * stands, right now.
 *
 * This is the finding that belongs on a vehicle's own page in plain words,
 * because a vehicle offering shares against a withdrawn or expired
 * authorisation is offering something the chain will refuse to mint — and it
 * will refuse it at the moment of issuance, with a buyer on the other side of
 * the transaction.
 *
 * `live` comes from x/land rather than being recomputed here. The query
 * computes it in the keeper precisely so that a wallet and the chain cannot
 * disagree about what "live" means, and a client that re-derived it would
 * reintroduce the disagreement it was written to prevent. What this function
 * adds is only *why* it is not live, which the flag alone does not say:
 * withdrawn and expired are visible on the record, and a permission that is
 * neither but still not live is one the parcel's own restrictions are stopping.
 */
export function landGate(
  parcelId: string,
  found: { authorisation: LandAuthorisation; live: boolean } | 'absent' | 'unreachable',
  nowSeconds: number,
): LandGate {
  if (!isRealId(parcelId)) return { kind: 'not-land' };
  if (found === 'absent') return { kind: 'absent' };
  if (found === 'unreachable') return { kind: 'unreachable' };

  const auth = found.authorisation;
  if (found.live) return { kind: 'live', auth };
  if (auth.withdrawn) return { kind: 'withdrawn', auth };
  if (auth.expiresAt <= nowSeconds) return { kind: 'expired', auth };
  return { kind: 'restricted', auth };
}

/** True when the chain would accept a fractionalisation against this gate. */
export function issuancePermitted(gate: LandGate): boolean {
  return gate.kind === 'live' || gate.kind === 'not-land';
}

/* ---------------------------------------------------------- status, in words */

/** Catalogue key for a vehicle's status. */
export function statusKey(status: VehicleStatus): string {
  switch (status) {
    case 'STATUS_HELD': return 'rwa.status.held';
    case 'STATUS_ACTIVE': return 'rwa.status.active';
    case 'STATUS_REPORTED': return 'rwa.status.reported';
    case 'STATUS_REALISED': return 'rwa.status.realised';
    case 'STATUS_CLOSED': return 'rwa.status.closed';
    case 'STATUS_DISPUTED': return 'rwa.status.disputed';
    default: return 'rwa.status.unknown';
  }
}

/**
 * Catalogue key for a parcel's status in x/land.
 *
 * Separate from the vehicle's own status and deliberately not the enum name.
 * `STATUS_TRANSFER_PENDING` is not something an investor can act on; "a
 * transfer of this land is under way" is, and it is the fact that decides
 * whether the sponsor will still be the parcel's holder when shares are issued.
 */
export function parcelStatusKey(status: string): string {
  switch (status) {
    case 'STATUS_REGISTERED': return 'rwa.parcelState.registered';
    case 'STATUS_TRANSFER_PENDING': return 'rwa.parcelState.transferPending';
    case 'STATUS_DISPUTED': return 'rwa.parcelState.disputed';
    case 'STATUS_FROZEN': return 'rwa.parcelState.frozen';
    default: return 'rwa.parcelState.unknown';
  }
}

export function parcelTone(status: string): 'ok' | 'warn' | 'bad' | 'mute' {
  switch (status) {
    case 'STATUS_REGISTERED': return 'ok';
    case 'STATUS_TRANSFER_PENDING': return 'warn';
    case 'STATUS_DISPUTED': return 'bad';
    case 'STATUS_FROZEN': return 'bad';
    default: return 'mute';
  }
}

/** The chip tone a status should carry. Never colour alone — the word leads. */
export function statusTone(status: VehicleStatus): 'ok' | 'warn' | 'bad' | 'mute' {
  switch (status) {
    case 'STATUS_ACTIVE': return 'ok';
    case 'STATUS_REPORTED': return 'warn';
    case 'STATUS_DISPUTED': return 'bad';
    case 'STATUS_REALISED': return 'warn';
    case 'STATUS_CLOSED': return 'mute';
    default: return 'mute';
  }
}

/**
 * Which of the four holder actions the chain would accept right now, and why
 * not when it would not.
 *
 * Prevention rather than translation: the dapp rule is to disable the action,
 * state the precondition and offer the fix inline, rather than to let somebody
 * sign a message the keeper is going to refuse. Each `whyKey` below names the
 * refusal the keeper would have produced.
 */
export interface Actions {
  claim: { enabled: boolean; whyKey?: string };
  redeem: { enabled: boolean; whyKey?: string };
  dispute: { enabled: boolean; whyKey?: string };
}

export function actionsFor(
  asset: Pick<Asset, 'status' | 'fractionDenom'>,
  sale: SaleState,
  balance: string,
  entitlement: string,
  connected: boolean,
): Actions {
  const holds = (() => { try { return BigInt(balance || '0') > 0n; } catch { return false; } })();
  const owed = (() => { try { return BigInt(entitlement || '0') > 0n; } catch { return false; } })();

  if (!connected) {
    const why = { enabled: false, whyKey: 'rwa.act.needAccount' };
    return { claim: why, redeem: why, dispute: why };
  }

  return {
    claim: !holds
      ? { enabled: false, whyKey: 'rwa.act.noShares' }
      : !owed
        ? { enabled: false, whyKey: 'rwa.act.nothingOwed' }
        : { enabled: true },

    redeem: asset.status !== 'STATUS_REALISED'
      ? { enabled: false, whyKey: 'rwa.act.notRealised' }
      : !holds
        ? { enabled: false, whyKey: 'rwa.act.noShares' }
        : { enabled: true },

    // Deliberately not gated on holding shares. The keeper does not require it
    // either, and it should not: the party best placed to know a reported price
    // is a lie is often the buyer who paid the real one, and they hold nothing.
    dispute: !sale.canDispute
      ? {
        enabled: false,
        whyKey: sale.phase === 'disputed' ? 'rwa.act.alreadyDisputed'
          : sale.phase === 'none' ? 'rwa.act.noSale'
            : 'rwa.act.windowClosed',
      }
      : { enabled: true },
  };
}
