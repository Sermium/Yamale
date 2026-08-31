// The catalogue: every mechanism this chain implements, what it refuses, and
// the query that proves the refusal is real right now.
//
// THE SHAPE, and why it is this shape.
//
// A link farm would list thirteen surfaces and let the room work out what they
// are for. What a finance ministry is actually asking is narrower and harder:
// *what can the operator of this thing not do to me?* So every entry leads with
// a refusal, and every refusal is followed by a query against the running chain
// that would fail if the refusal were decorative.
//
// `read` is deliberately not allowed to return a number on failure. It returns
// a tagged proof, and format.js has no path from an error to a numeral. The
// failure mode this rules out is the one that has embarrassed this project
// before: a mechanism whose proof could not be fetched rendering as "0
// objections", which reads as "nothing has ever gone wrong here" when it means
// "the page did not ask".
//
// `href` values are checked against SURFACES by mechanisms.test.js, so a typo
// in a link is a failing test rather than a dead click in a room.

import {
  abci, abciOrNull, byAction, byId, bySender, byString, rest, txSearch,
} from './chain.js';
import {
  decodeAuthority, decodeLandParams, decodeList, decodeOne, decodeParcel,
  decodeTransfer, num, read, statusName, str,
} from './proto.js';
import {
  amount, blocksAbout, bps, count, describeFailure, duration, elide, proven, unread, whenUnix,
} from './format.js';

/* ========================================================================= */
/*  The surfaces                                                             */
/* ========================================================================= */

/**
 * Every place this page can send somebody, and whether it is up.
 *
 * `building` is stated rather than hidden. Two of these are being written right
 * now; linking them and saying so is honest, and quietly omitting them would
 * make the tour look complete in a way it is not.
 */
export const SURFACES = {
  site: { href: '/', label: 'Yamale', blurb: 'What the network is', status: 'live' },
  app: { href: '/app/', label: 'Pay', blurb: 'The citizen app: hold money, get paid', status: 'live' },
  wallet: { href: '/wallet/', label: 'Wallet', blurb: 'Create or watch an account', status: 'live' },
  safe: { href: '/safe/', label: 'Safe', blurb: 'Shared treasuries and spending controls', status: 'live' },
  explorer: { href: '/explorer/', label: 'Explorer', blurb: 'Blocks, transactions, accounts', status: 'live' },
  land: { href: '/land/', label: 'Land register', blurb: 'Titles, transfers, objections', status: 'live' },
  rwa: { href: '/rwa/', label: 'Vehicles', blurb: 'Closed-end real-world-asset vehicles', status: 'live' },
  governance: { href: '/governance/', label: 'Governance', blurb: 'Proposals, votes, the constitution', status: 'live' },
  foundation: { href: '/foundation/', label: 'Foundation', blurb: 'The 3-of-5 custodian group', status: 'live' },
  validator: { href: '/validator/', label: 'Validators', blurb: 'Who produces blocks, and on what terms', status: 'live' },
  docs: { href: '/docs/', label: 'Documentation', blurb: 'Every module, generated from the protos', status: 'live' },
  oversight: { href: '/oversight/', label: 'Oversight', blurb: 'Enforcement cases and netting cycles', status: 'building' },
  markets: { href: '/markets/', label: 'Markets', blurb: 'Prices, pools, currency issuance', status: 'building' },
};

/* ========================================================================= */
/*  Query paths                                                              */
/* ========================================================================= */

const LAND = '/blockchain.land.v1.Query';
const PAYMSG = '/blockchain.paymsg.v1.Query';
const NETTING = '/blockchain.netting.v1.Query';

/**
 * The account whose key exists only in three shares.
 *
 * Hard-coded because it is an identifier, not a fact: it names *which* account
 * to go and check. Everything asserted about it — that it signed, at which
 * heights, with which result — is read from the chain below and is allowed to
 * come back empty.
 */
export const THRESHOLD_ACCOUNT = 'yml1ael7jxwlvacc3daawzc2kpd6lst6w8nmml6a97';

/* ========================================================================= */
/*  The catalogue                                                            */
/* ========================================================================= */

/**
 * Five acts, in the order somebody would walk a room through them: money
 * first, because that is what a payments network is for; then the property
 * mechanisms, which are the ones that surprise people; then the limits on the
 * operator, which is what the room is really assessing; then who runs it; then
 * the market plumbing, which is the least finished.
 */
export const ACTS = [
  {
    id: 'money',
    title: 'Money that moves',
    lede: 'A payment is refused before it moves, not investigated after.',
  },
  {
    id: 'property',
    title: 'Property that cannot be taken quietly',
    lede: 'Every step of a title change is signed by somebody who can be named, and any stranger can stop it.',
  },
  {
    id: 'limits',
    title: 'Limits on whoever runs this',
    lede: 'The operator has powers. Each one is bounded by a number that is on the chain and readable by anybody.',
  },
  {
    id: 'institutions',
    title: 'Who runs it',
    lede: 'Custody, block production and treasury control, and what each of them cannot do alone.',
  },
  {
    id: 'markets',
    title: 'Prices and currency',
    lede: 'The least finished part of the system, and said so.',
  },
];

export const MECHANISMS = [
  /* ------------------------------------------------------------- money */
  {
    id: 'approved-participants',
    act: 'money',
    module: 'x/paymsg',
    name: 'Institutional payments, ISO 20022',
    does: 'Carries a bank-to-bank payment with its ISO 20022 payload attached, between institutions that hold accounts on the chain.',
    refuses: 'A payment from an institution no national authority has approved. The approval list lives in chain state and is checked when the message runs, not when it is composed — so it cannot be routed around by a client.',
    surface: 'app',
    watch: 'The list of approved participants, and how many payments it has carried.',
    async read() {
      try {
        const [participants, records] = await Promise.all([
          abci(`${PAYMSG}/ListApprovedParticipant`),
          abci(`${PAYMSG}/ListPaymentRecord`),
        ]);
        const p = decodeList(participants.bytes, (b) => ({ address: str(read(b), 1) }), { itemField: 1 });
        const r = decodeList(records.bytes, () => ({}), { itemField: 1 });
        return proven(participants.height, [
          { label: 'Approved participants', value: count(p.total, 'institution') },
          { label: 'Payment records held', value: count(r.total, 'payment') },
        ], p.total === 0
          ? 'Nought, read from the chain rather than assumed. On this devnet no participant has been approved, '
            + 'so every institutional payment message would be refused today. That is the mechanism working, '
            + 'not the mechanism missing — but it also means there is no completed ISO 20022 payment to show you here.'
          : '');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'threshold-key',
    act: 'money',
    module: 'mpc',
    name: 'A consumer key the operator does not hold',
    does: 'Splits a consumer account key into three shares held in different places. Any two of them together produce a signature; the whole key is never assembled anywhere.',
    refuses: 'The operator moving a consumer\'s money. There is no copy of the key to seize, subpoena or leak — and a password reset re-shares the key without changing the address, so recovery does not mean handing custody to whoever runs the recovery.',
    surface: 'wallet',
    watch: 'Transactions this address has signed. It has no single private key anywhere.',
    async read() {
      try {
        const found = await txSearch(bySender(THRESHOLD_ACCOUNT), { perPage: 20, order: 'asc' });
        if (found.total === 0) {
          return proven(0, [{ label: 'Transactions signed', value: count(0, 'transaction') }],
            'The index holds nothing for this address. Nothing is being claimed that is not there.');
        }
        const rows = [
          { label: 'Address', value: THRESHOLD_ACCOUNT, mono: true, full: true },
          { label: 'Transactions signed', value: count(found.total, 'transaction') },
        ];
        found.txs.forEach((t, i) => rows.push({
          label: `Signature ${i + 1}`,
          value: `height ${t.height.toLocaleString('en')} — ${t.code === 0 ? 'accepted by the block' : `failed, code ${t.code}`}`,
          mono: true,
        }));
        return proven(found.txs.at(-1)?.height ?? 0, rows,
          found.total >= 2
            ? 'The shares behind the second signature were created in a password reset. The address did not change, '
              + 'which is the point: recovery re-shares the key rather than replacing the account.'
            : '');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'netting',
    act: 'money',
    module: 'x/netting',
    name: 'Multilateral netting',
    does: 'Collects obligations between participants over a cycle and settles the net position, so a hundred bilateral payments move a handful of balances.',
    refuses: 'Being switched half on. The cycle length is the divisor that decides where a window closes, and a zero is read as "netting is off" everywhere at once — every obligation settles gross and the end blocker returns before it computes anything. The alternative reading, zero meaning every block, divides by zero inside an end blocker, and that is not a failed transaction: it is a chain that produces no further blocks.',
    surface: 'oversight',
    watch: 'The cycle length, and therefore whether netting is running on this chain at all.',
    async read(ctx) {
      try {
        const [params, cycle] = await Promise.all([
          abci(`${NETTING}/Params`),
          abci(`${NETTING}/CurrentCycle`),
        ]);
        // params.proto: Params { uint64 cycle_blocks = 1 }, inside
        // QueryParamsResponse { Params params = 1 }.
        const cycleBlocks = num(read(read(params.bytes).get(1)?.[0] ?? new Uint8Array()), 1);
        // query.proto: QueryCurrentCycleResponse { Cycle cycle = 1,
        // int64 closes_at_height = 2 }. Zero when netting is off, because then
        // no block ever will close it.
        const closesAt = num(read(cycle.bytes), 2);
        const on = cycleBlocks > 0;
        return proven(params.height, [
          { label: 'Module answering on this chain', value: 'yes' },
          {
            label: 'Cycle length',
            value: on ? blocksAbout(cycleBlocks, ctx?.secondsPerBlock) : '0 blocks — netting is switched off',
            emphasis: true,
          },
          {
            label: 'Current window closes at',
            value: closesAt ? `block ${closesAt.toLocaleString('en')}` : 'no block will close it',
            mono: true,
          },
        ], on
          ? ''
          : 'Read from the chain, and not a failure to read it: netting is deliberately off here. Nought is a '
            + 'meaningful setting rather than an unset one, and the module treats it as "off everywhere" rather '
            + 'than guessing — which is the difference between a feature nobody turned on and an end blocker '
            + 'that divides by zero and stops the chain. The oversight console that drives cycles is still being written.');
      } catch (e) { return describeFailure(e); }
    },
  },

  /* ---------------------------------------------------------- property */
  {
    id: 'land-uniqueness',
    act: 'property',
    module: 'x/land',
    name: 'One title per piece of ground',
    does: 'Holds each parcel as a single indivisible token with exactly one holder, indexed by the hash of its surveyed boundary.',
    refuses: 'A second title over the same survey. The geometry hash is the index, and registering a parcel whose hash is already present fails in the keeper — which is what makes "cannot be owned twice" a property of the database rather than a promise in a policy document.',
    surface: 'land',
    watch: 'A parcel looked up by its survey hash comes back as the one title that hash belongs to.',
    async read() {
      try {
        const first = await abci(`${LAND}/Parcel`, byId(1));
        const parcel = decodeOne(first.bytes, decodeParcel);
        if (!parcel) return unread('absent', 'The register answered, and holds no parcel 1.');
        const back = await abciOrNull(`${LAND}/ParcelByGeometry`, byString(parcel.geometryHash));
        const resolved = back ? decodeOne(back.bytes, decodeParcel) : null;
        return proven(first.height, [
          { label: 'Parcel 1 reference', value: parcel.cadastralRef, mono: true },
          { label: 'Survey hash', value: parcel.geometryHash, mono: true, full: true },
          {
            label: 'That hash resolves to',
            value: resolved ? `parcel ${resolved.id} — ${resolved.cadastralRef}` : 'nothing',
            mono: true,
          },
          { label: 'Status', value: statusName(parcel.status), mono: true },
        ], resolved && resolved.id === parcel.id
          ? 'The hash resolves to exactly one title. A registration carrying this hash again is refused by the keeper.'
          : 'The uniqueness index did not resolve back to the parcel it was taken from — worth investigating before this is shown.');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'land-transfer',
    act: 'property',
    module: 'x/land',
    name: 'A transfer takes four parties',
    does: 'Moves a title only after the seller proposes, the office whose jurisdiction the land falls in validates, independent registry offices attest to a quorum, and a public challenge window closes.',
    refuses: 'A transfer signed by the seller alone, and an attestation from the office that proposed it. Same-office attestation is refused in the keeper — allowing it would collapse the whole mechanism back to a single bribe.',
    surface: 'land',
    watch: 'The completed transfer on parcel 1: who validated, how many independent offices attested, and the block it completed in.',
    async read() {
      try {
        const [params, transfer, offices] = await Promise.all([
          abci(`${LAND}/Params`),
          abci(`${LAND}/Transfer`, byId(0)),
          abci(`${LAND}/Authorities`),
        ]);
        const p = decodeLandParams(read(params.bytes).get(1)?.[0] ?? new Uint8Array());
        const t = decodeOne(transfer.bytes, decodeTransfer);
        const list = decodeList(offices.bytes, decodeAuthority);
        if (!t) return unread('absent', 'The register holds no transfer 0.');

        // The completing block is asked of the transaction index rather than
        // inferred. The state carries a unix timestamp, not a height, and a
        // height guessed from a timestamp is a height somebody will cite.
        let completedAt = null;
        try {
          const done = await txSearch(byAction('/blockchain.land.v1.MsgCompleteTransfer'), { perPage: 5 });
          completedAt = done.txs.find((x) => x.code === 0)?.height ?? null;
        } catch { /* the index is discovery, not fact. The transfer stands without it. */ }

        return proven(transfer.height, [
          { label: 'Registry offices on the chain', value: count(list.total, 'office') },
          { label: 'Independent attestations required', value: count(p.attestationQuorum, 'office') },
          { label: 'Transfer 0, parcel', value: String(t.parcelId), mono: true },
          { label: 'Validated by the local office', value: t.validated ? `yes — ${elide(t.validatedBy)}` : 'no', mono: true },
          { label: 'Attested by', value: count(t.attestors.length, 'independent office') },
          // Spoken, not counted in seconds. "600 seconds" is a number somebody
          // has to divide in their head while being talked to; "10 minutes" is
          // the fact. The raw figure stays beside it because it is the one that
          // is actually in the chain's parameters.
          {
            label: 'Challenge window',
            value: `${duration(p.challengeWindowSeconds) ?? '—'} from quorum (${count(p.challengeWindowSeconds, 'second')})`,
          },
          {
            label: 'Completed',
            value: completedAt
              ? `block ${completedAt.toLocaleString('en')}${whenUnix(t.completedAt) ? ` — ${whenUnix(t.completedAt)}` : ''}`
              : (whenUnix(t.completedAt) ?? 'not completed'),
            mono: true,
          },
        ], 'Four separate accounts had to act, in order, before the title moved.');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'land-objection',
    act: 'property',
    module: 'x/land',
    name: 'Any stranger can stop a sale',
    does: 'Lets any account object to a pending transfer during the challenge window, with a reason, and records the objection permanently.',
    refuses: 'Completing a transfer somebody objected to. One objection is terminal — the chain preserves the evidence, marks the parcel disputed, and does not adjudicate. A court does that, and it now has a dated record to work from.',
    surface: 'land',
    watch: 'Parcel 2: stopped by an objection from an account that is not a party to the sale, and stopped permanently.',
    async read() {
      try {
        const [parcel, transfers] = await Promise.all([
          abci(`${LAND}/Parcel`, byId(2)),
          abci(`${LAND}/TransfersByParcel`, byId(2)),
        ]);
        const p = decodeOne(parcel.bytes, decodeParcel);
        const list = decodeList(transfers.bytes, decodeTransfer);
        const stopped = list.items.find((t) => t.objectedBy);
        if (!p) return unread('absent', 'The register holds no parcel 2.');

        let objectedAt = null;
        try {
          const found = await txSearch(byAction('/blockchain.land.v1.MsgObject'), { perPage: 5 });
          objectedAt = found.txs.find((x) => x.code === 0)?.height ?? null;
        } catch { /* the index is discovery, not fact */ }

        const rows = [
          { label: 'Parcel 2 reference', value: p.cadastralRef, mono: true },
          { label: 'Status', value: statusName(p.status), mono: true, emphasis: p.status === 3 },
        ];
        if (stopped) {
          rows.push(
            { label: 'Objector', value: stopped.objectedBy, mono: true, full: true },
            { label: 'A party to the sale?', value: (stopped.objectedBy === stopped.from || stopped.objectedBy === stopped.to) ? 'yes' : 'no — neither buyer nor seller' },
            { label: 'Reason given', value: stopped.objectionReason, quote: true },
            { label: 'Attestations it already had', value: count(stopped.attestors.length, 'office') },
            { label: 'Completed', value: stopped.completedAt ? 'yes' : 'no — and it never can be', emphasis: !stopped.completedAt },
          );
        }
        if (objectedAt) rows.push({ label: 'Objection recorded in block', value: objectedAt.toLocaleString('en'), mono: true });
        return proven(parcel.height, rows,
          stopped
            ? 'The transfer had already been validated and attested. One sentence from a stranger ended it.'
            : 'No objection is recorded against parcel 2 right now.');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'closed-end',
    act: 'property',
    module: 'x/tokenisation',
    name: 'A vehicle that cannot dilute its holders',
    does: 'Opens a closed-end vehicle over a registered parcel and issues a fixed number of shares representing a fixed percentage of it. The title itself never moves into the vehicle.',
    refuses: 'Issuing more shares after the vehicle is open. The supply is fixed at issuance, so a holder\'s percentage cannot be reduced by the sponsor later — which is the ordinary way a minority holding in an unlisted vehicle is destroyed.',
    surface: 'rwa',
    watch: 'The vehicle over parcel 3: its share denom, and the total supply of that denom across the whole chain.',
    async read() {
      try {
        const [assets, supply] = await Promise.all([
          rest('/yamale/blockchain/tokenisation/v1/assets', 'the vehicles'),
          rest('/cosmos/bank/v1beta1/supply?pagination.limit=500', 'the token supply'),
        ]);
        const asset = (assets.assets ?? [])[0];
        if (!asset) return unread('absent', 'The chain holds no vehicle.');
        const issued = (supply.supply ?? []).find((c) => c.denom === asset.fraction_denom);
        return proven(0, [
          { label: 'Vehicle', value: `asset ${asset.id} in collection ${asset.collection_id}`, mono: true },
          { label: 'Over parcel', value: String(asset.parcel_id ?? '—'), mono: true },
          { label: 'Share denom', value: asset.fraction_denom, mono: true },
          { label: 'Shares in existence', value: issued ? Number(issued.amount).toLocaleString('en') : 'none issued' },
          // NOT "what the sponsor keeps". tokenisation.proto is explicit that
          // this is the share the tokens carry and the sponsor keeps the rest,
          // and it is the figure the registry's own ceiling caps. Stating it
          // the other way round would show a 60% holding as a 40% one.
          { label: 'Share of the asset the shares carry', value: bps(asset.holder_share_bps) ?? '—' },
          { label: 'Status', value: asset.status, mono: true },
        ], 'The share count is read from the chain-wide token supply, not from the vehicle\'s own record. '
          + 'Those two agreeing is what makes "fixed" checkable by somebody who does not trust the module.');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'attested-price',
    act: 'property',
    module: 'x/tokenisation',
    name: 'The sale price is attested, not asserted',
    does: 'Requires a reported sale of a vehicle\'s underlying asset to be attested inside a challenge window, against a bond, before proceeds are distributed.',
    refuses: 'A price the seller states about themselves. Attestors are appointed by governance rather than chosen by the seller, and a dispute costs the disputer a bond — so the cheap attack is asserting a low price to a friendly buyer, and it is exactly the one this closes.',
    surface: 'rwa',
    watch: 'The collection\'s own terms: who verifies, how long the window is, and what a dispute costs.',
    async read() {
      try {
        const body = await rest('/yamale/blockchain/tokenisation/v1/collections', 'the collections');
        const c = (body.collections ?? [])[0];
        if (!c) return unread('absent', 'The chain holds no collection.');
        return proven(0, [
          { label: 'Collection', value: c.id, mono: true },
          { label: 'Who verifies a sale', value: c.verification, mono: true },
          { label: 'Attestations required', value: count(Number(c.attestation_threshold ?? 0), 'attestor') },
          {
            label: 'Challenge window',
            value: `${duration(Number(c.challenge_window_seconds ?? 0)) ?? '—'} (${count(Number(c.challenge_window_seconds ?? 0), 'second')})`,
          },
          { label: 'Cost of disputing', value: `${bps(c.dispute_bond_bps) ?? '—'} of the sale, posted as a bond` },
          { label: 'Authority', value: c.authority, mono: true, full: true },
        ], Number(c.attestation_threshold ?? 0) === 0
          ? 'This collection is set to governance verification with no separate attestor threshold, so the '
            + 'governance authority above is the check. The per-sale attestor path exists in the module and is not exercised here.'
          : '');
      } catch (e) { return describeFailure(e); }
    },
  },

  /* ------------------------------------------------------------ limits */
  {
    id: 'freeze',
    act: 'limits',
    module: 'x/enforcement',
    name: 'A freeze is fast, and expires by itself',
    does: 'Lets a single authorised account stop an account\'s funds within one block, so a fraud in progress can be interrupted at the speed it is moving.',
    refuses: 'An indefinite freeze. The provisional stop runs out on its own unless a case is opened and carried — so the cheap, unilateral power is the reversible one, and the permanent one is not unilateral.',
    surface: 'oversight',
    watch: 'How long a provisional freeze lasts, and how many accounts are frozen right now.',
    async read(ctx) {
      try {
        const [params, freezes] = await Promise.all([
          rest('/yamale/blockchain/enforcement/v1/params', 'the enforcement parameters'),
          rest('/yamale/blockchain/enforcement/v1/freeze', 'the freeze list'),
        ]);
        const p = params.params ?? {};
        return proven(0, [
          {
            label: 'A provisional freeze lasts',
            value: blocksAbout(p.provisional_freeze_blocks, ctx?.secondsPerBlock) ?? '—',
          },
          { label: 'Accounts frozen right now', value: count(Number(freezes.pagination?.total ?? (freezes.freeze ?? []).length), 'account') },
          { label: 'Reason required', value: `yes — up to ${Number(p.max_reason_length ?? 0).toLocaleString('en')} characters, stored` },
        ], 'The reason is not optional. A stop whose grounds cannot be read is indistinguishable from an arbitrary one.');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'seizure',
    act: 'limits',
    module: 'x/enforcement',
    name: 'Seizure needs two thirds of the validators',
    does: 'Moves funds out of an account only after a case is opened, voted on by validators over a fixed period, and passes a supermajority of voting power.',
    refuses: 'Taking money on one authority\'s say-so, taking it quickly, taking it without evidence, and taking very much of it. Two thirds of voting power must vote yes over a fixed period; then a delay runs that gets longer as the amount gets larger; and a cap limits how much can be taken across a whole window however many cases pass.',
    surface: 'oversight',
    watch: 'The threshold, the delay, the standing cap on total seizure, and how much of it has been used.',
    async read(ctx) {
      try {
        const [params, window, recovered] = await Promise.all([
          rest('/yamale/blockchain/enforcement/v1/params', 'the enforcement parameters'),
          rest('/yamale/blockchain/enforcement/v1/window', 'the seizure window'),
          rest('/yamale/blockchain/enforcement/v1/recovered', 'the seizure history'),
        ]);
        const p = params.params ?? {};
        const cap = (window.cap ?? [])[0];
        const left = (window.remaining ?? [])[0];
        // The largest tier, because that is the one that governs the seizure a
        // reader is actually worried about. Quoting the base delay alone would
        // understate the constraint by a factor of twelve.
        const tiers = p.seizure_delay_tiers ?? [];
        const worst = tiers.reduce((a, t) => (Number(t.delay_blocks) > Number(a?.delay_blocks ?? 0) ? t : a), null);
        return proven(0, [
          { label: 'Voting power required', value: `${bps(p.threshold_bps) ?? '—'} — a two-thirds supermajority`, emphasis: true },
          { label: 'Voting period', value: blocksAbout(p.voting_period_blocks, ctx?.secondsPerBlock) ?? '—' },
          { label: 'Evidence required', value: p.seize_requires_evidence ? 'yes — a case without it cannot pass' : 'no' },
          {
            label: 'Delay after a case passes',
            value: worst
              ? `${blocksAbout(p.seizure_delay_blocks, ctx?.secondsPerBlock)}, rising to ${blocksAbout(worst.delay_blocks, ctx?.secondsPerBlock)} above ${amount(worst.threshold.amount, worst.threshold.denom)}`
              : (blocksAbout(p.seizure_delay_blocks, ctx?.secondsPerBlock) ?? '—'),
          },
          {
            label: 'Cap across a window',
            value: cap
              ? `${amount(cap.amount, cap.denom)} and ${count(Number(p.max_seizures_per_window ?? 0), 'case')} per ${blocksAbout(p.seizure_window_blocks, ctx?.secondsPerBlock)}`
              : '—',
            emphasis: true,
          },
          { label: 'Used in the current window', value: cap && left ? `${amount((BigInt(cap.amount) - BigInt(left.amount)).toString(), cap.denom)} of ${amount(cap.amount, cap.denom)}` : '—' },
          { label: 'Cases ever opened / passed', value: `${Number(recovered.cases_opened ?? 0).toLocaleString('en')} / ${Number(recovered.cases_passed ?? 0).toLocaleString('en')}` },
          { label: 'Seized funds go to', value: p.recovery_destination ?? '—', mono: true, full: true },
        ], 'The destination is a parameter, not a discretion. A validator voting for a seizure is not voting on '
          + 'where the money lands, which removes most of the reason to want one. The window cap is the answer to '
          + 'the question a ministry actually asks: what is the worst a captured validator set could do in a day.');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'constitution',
    act: 'limits',
    module: 'x/constitution',
    name: 'Thirteen rules ordinary governance cannot reach',
    does: 'Holds thirteen numbers that bound every other module — concentration caps, the enforcement threshold, the foundation\'s own quorum — as chain state that other modules read at execution time.',
    refuses: 'Any ordinary proposal that changes them. An amendment is a different instrument: it needs four fifths, and it does not take effect for a delay measured in blocks, so nobody can pass one and act on it the same week.',
    surface: 'governance',
    watch: 'All thirteen values, read live, plus what changing one would cost.',
    async read(ctx) {
      try {
        const body = await rest('/yamale/blockchain/constitution/v1/invariants', 'the constitution');
        const inv = body.invariants ?? {};
        const entries = Object.entries(inv);
        // A well-formed answer with nothing in it. The node is up, the gateway
        // is up, and the module is gone — which is the case most likely to
        // render as a row of confident noughts, because nothing threw.
        if (entries.length === 0) {
          return unread('absent',
            'The chain answered, and returned no invariants at all. That is not a constitution of nought rules; '
            + 'it is a module that did not report. Treat it as unread.');
        }
        const rows = [
          { label: 'Invariants held', value: count(entries.length, 'rule'), emphasis: true },
          { label: 'To amend one', value: `${bps(inv.amendment_threshold_bps) ?? '—'} of voting power`, },
          { label: 'And then a delay of', value: blocksAbout(inv.amendment_delay_blocks, ctx?.secondsPerBlock) ?? '—' },
          { label: 'Seizure threshold', value: bps(inv.enforcement_threshold_bps) ?? '—' },
          { label: 'Foundation quorum', value: `${inv.foundation_signature_threshold} of ${inv.foundation_custodian_count} custodians` },
          { label: 'Minimum active validators', value: count(Number(inv.min_active_validators ?? 0), 'validator') },
        ];
        return proven(0, rows,
          entries.length === 13
            ? 'Thirteen, counted from the answer rather than from the documentation.'
            : `The chain returned ${entries.length} rules, not thirteen. The number stated is the one the chain gave.`);
      } catch (e) { return describeFailure(e); }
    },
  },

  /* ------------------------------------------------------ institutions */
  {
    id: 'foundation',
    act: 'institutions',
    module: 'x/group',
    name: 'The foundation is three of five',
    does: 'Holds the foundation\'s authority in an x/group account whose policy needs three of five custodian signatures, with the shares kept on air-gapped paper.',
    refuses: 'One custodian acting alone, and two acting together. The quorum is in the constitution rather than in the group policy alone, so changing it is an amendment rather than an administrative edit.',
    surface: 'foundation',
    watch: 'The quorum, as the constitution states it.',
    async read() {
      try {
        const body = await rest('/yamale/blockchain/constitution/v1/invariants', 'the constitution');
        const inv = body.invariants ?? {};
        const custodians = Number(inv.foundation_custodian_count);
        const threshold = Number(inv.foundation_signature_threshold);
        // A quorum of nought is not a quorum. If either figure is missing this
        // is an unread proof, not a foundation anybody can sign for alone.
        if (!Number.isFinite(custodians) || !Number.isFinite(threshold) || custodians === 0) {
          return unread('absent', 'The chain answered, and reported no custodian quorum. Treat it as unread.');
        }
        return proven(0, [
          { label: 'Custodians', value: count(custodians, 'custodian') },
          { label: 'Signatures required', value: count(threshold, 'signature'), emphasis: true },
          { label: 'Any one custodian alone', value: 'cannot act' },
        ], 'The devnet key this was rehearsed with has had its recovery phrase destroyed and is not carried forward.');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'validators',
    act: 'institutions',
    module: 'x/validatorgov',
    name: 'Who is allowed to produce blocks',
    does: 'Gates validator admission behind governance, and caps how much voting power one entity, one beneficial owner or one jurisdiction may hold.',
    refuses: 'Unbounded concentration — the caps are constitutional. What it does not currently refuse is an unadmitted validator: the admission list is empty on this devnet, and the two validators running predate it.',
    surface: 'validator',
    watch: 'The validators actually producing blocks, their power, and the admission list.',
    async read() {
      try {
        const [validators, approved] = await Promise.all([
          rest('/cosmos/staking/v1beta1/validators?status=BOND_STATUS_BONDED', 'the validator set'),
          rest('/yamale/blockchain/validatorgov/v1/approved_validator', 'the admission list'),
        ]);
        const set = validators.validators ?? [];
        const total = set.reduce((sum, v) => sum + BigInt(v.tokens ?? '0'), 0n);
        const rows = [
          { label: 'Validators producing blocks', value: count(set.length, 'validator'), emphasis: true },
          { label: 'Total bonded', value: amount(total.toString(), 'uyml') ?? '—' },
        ];
        set.forEach((v) => {
          const share = total > 0n ? Number((BigInt(v.tokens) * 10000n) / total) : 0;
          rows.push({
            label: v.description?.moniker || elide(v.operator_address),
            value: `${bps(share)} of voting power${v.jailed ? ' — jailed' : ''}`,
            mono: true,
          });
        });
        rows.push({
          label: 'On the admission list',
          value: count(Number(approved.pagination?.total ?? (approved.approved_validator ?? []).length), 'candidate'),
        });
        return proven(0, rows,
          set.length <= 2
            ? 'Two validators is not a decentralised network and this page will not pretend otherwise. '
              + 'Losing one of them stops the chain. It is enough to demonstrate the mechanisms and not enough to run anything.'
            : '');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'upgrades',
    act: 'institutions',
    module: 'x/upgrade',
    name: 'Upgrades that cannot silently diverge',
    does: 'Changes the software every node runs by governance vote, at a block height named in the proposal. Every node stops at that height and will not continue until it is running the new binary.',
    refuses: 'Two nodes disagreeing quietly about what the rules are. Each node computes an application hash for every block, and consensus requires them to match — so a node running different code does not produce a subtly different ledger, it fails to produce a block at all. The chain stopping is the mechanism working.',
    surface: 'validator',
    watch: 'Upgrades already applied, how far past them the chain has run, and the one it is about to stop for.',
    async read(ctx) {
      try {
        const body = await rest('/cosmos/gov/v1/proposals?pagination.limit=100', 'the proposals');
        const upgrades = (body.proposals ?? [])
          .flatMap((p) => (p.messages ?? [])
            .filter((m) => m['@type'] === '/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade')
            .map((m) => ({ id: p.id, status: p.status, name: m.plan?.name, height: Number(m.plan?.height ?? 0) })))
          .sort((a, b) => a.height - b.height);
        if (upgrades.length === 0) {
          return unread('absent', 'The chain answered, and records no software upgrade proposal.');
        }
        const now = ctx?.head?.height ?? 0;
        const applied = upgrades.filter((u) => u.height <= now);
        const pending = upgrades.filter((u) => u.height > now);
        const rows = [
          { label: 'Upgrades applied', value: count(applied.length, 'upgrade'), emphasis: true },
        ];
        applied.forEach((u) => rows.push({
          label: u.name,
          value: `at block ${u.height.toLocaleString('en')}${now ? ` — ${(now - u.height).toLocaleString('en')} blocks ago` : ''}`,
          mono: true,
        }));
        pending.forEach((u) => rows.push({
          label: `${u.name} — scheduled`,
          value: now
            ? `at block ${u.height.toLocaleString('en')} — ${blocksAbout(u.height - now, ctx?.secondsPerBlock)} from now`
            : `at block ${u.height.toLocaleString('en')}`,
          mono: true,
          emphasis: true,
        }));
        return proven(now, rows,
          applied.length
            ? 'Every block since those heights is the proof. Had the nodes computed different application hashes '
              + 'at an upgrade the chain would have halted there and stayed halted — it cannot proceed on a '
              + 'majority view of the state. It is '
              + `${now && applied.length ? (now - applied[0].height).toLocaleString('en') : 'many'} blocks past the first one.`
            : '');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'treasury',
    act: 'institutions',
    module: 'x/treasury',
    name: 'A treasury cannot spend what it has committed',
    does: 'Holds shared funds in a module account with roles and an M-of-N spending policy, and locks amounts against future commitments.',
    refuses: 'Spending locked funds. The locked-versus-available split holds because the money genuinely sits in the module account rather than behind a flag on somebody\'s own balance — a flag can be ignored by a direct transfer, and a custody account cannot.',
    surface: 'safe',
    watch: 'A funded treasury, split into what is committed and what can still be spent — and the arithmetic between them.',
    async read() {
      try {
        const body = await rest('/yamale/blockchain/treasury/v1/treasury', 'the treasuries');
        const list = body.treasury ?? [];
        if (list.length === 0) return unread('absent', 'The chain answered, and holds no treasury.');

        // The one with money in it, not simply the first. A treasury with no
        // balance demonstrates the invariant the way an empty page demonstrates
        // a layout: it cannot be wrong, and it cannot be right either.
        let funded = null;
        for (const t of list) {
          const b = await rest(`/yamale/blockchain/treasury/v1/treasury/${t.id}/balances`, `treasury ${t.id}`);
          if ((b.balances ?? []).length) { funded = { treasury: t, balances: b.balances }; break; }
        }

        const rows = [
          { label: 'Treasuries on the chain', value: count(list.length, 'treasury', 'treasuries') },
        ];
        if (!funded) {
          rows.push({ label: 'Holding a balance', value: 'none of them' });
          return proven(0, rows, 'No treasury on this chain holds funds, so the locked-against-available split '
            + 'cannot be shown working. Said rather than illustrated with an empty table.');
        }

        const locks = await rest(
          `/yamale/blockchain/treasury/v1/treasury/${funded.treasury.id}/locks`,
          'the treasury commitments',
        );
        rows.push({ label: 'Shown', value: funded.treasury.name || `Treasury ${funded.treasury.id}`, mono: true });
        rows.push({ label: 'Standing commitments', value: count((locks.lock ?? []).length, 'lock') });

        // Every currency, with the arithmetic spelled out. The invariant is
        // total = locked + available, and it holds because the money is in a
        // module account rather than behind a flag on somebody's own balance —
        // a flag is ignored by an ordinary transfer, a custody account is not.
        let holds = true;
        funded.balances.forEach((b) => {
          const total = BigInt(b.total); const locked = BigInt(b.locked); const available = BigInt(b.available);
          if (locked + available !== total) holds = false;
          rows.push({
            label: `${b.denom.slice(1).toUpperCase()} — committed / spendable`,
            value: `${amount(b.locked, b.denom)} / ${amount(b.available, b.denom)} of ${amount(b.total, b.denom)}`,
            mono: true,
          });
        });
        rows.push({
          label: 'Committed plus spendable equals the balance',
          value: holds ? 'yes, for every currency' : 'NO — the figures do not reconcile',
          emphasis: true,
        });

        return proven(0, rows, holds
          ? 'The arithmetic is checked here rather than trusted: the page adds the two figures and compares them '
            + 'with the balance the chain reports. The custody accounts behind these are also on the chain\'s '
            + 'blocked list, so a direct bank transfer to one is refused — money sent that way would sit outside '
            + 'the treasury\'s accounting and be unrecoverable.'
          : 'The committed and spendable figures do not add up to the reported balance. That is worth '
            + 'investigating before this is shown to anybody.');
      } catch (e) { return describeFailure(e); }
    },
  },

  /* ----------------------------------------------------------- markets */
  {
    id: 'stablecoin',
    act: 'markets',
    module: 'x/stablecoin',
    name: 'One approved issuer per currency',
    does: 'Binds each national-currency denomination on the chain to a single issuer approved through governance.',
    refuses: 'Minting a currency denom by anyone but its approved issuer. The binding is denom-to-issuer in state, so an approval for one currency confers nothing over another.',
    surface: 'markets',
    watch: 'How many currencies have an approved issuer, and whether one account holds them all.',
    async read() {
      try {
        const body = await rest('/yamale/blockchain/stablecoin/v1/approved_issuer?pagination.limit=200', 'the issuer list');
        const list = body.approved_issuer ?? [];
        const issuers = new Set(list.map((i) => i.issuer));
        return proven(0, [
          { label: 'Currencies with an approved issuer', value: count(list.length, 'currency', 'currencies') },
          { label: 'Distinct issuing accounts', value: count(issuers.size, 'account'), emphasis: issuers.size === 1 },
        ], issuers.size === 1
          ? 'One account currently issues all of them. That is a property of this devnet, not of the module — the '
            + 'binding is per denomination and a real deployment would have a different issuer per central bank.'
          : '');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'oracle',
    act: 'markets',
    module: 'x/oracle',
    name: 'A price too old to trust is not a price',
    does: 'Takes exchange rates by validator vote each period, and asset valuations from valuers governance appointed, and stamps every value with when it was set.',
    refuses: 'Serving a stale value as if it were current. A rate that has not been refreshed within its window is absent rather than old, so a module reading it fails loudly instead of pricing off last week.',
    surface: 'markets',
    watch: 'How many rates are actually live right now, against how many currencies the module accepts.',
    async read(ctx) {
      try {
        const [params, rates] = await Promise.all([
          rest('/yamale/blockchain/oracle/v1/params', 'the oracle parameters'),
          rest('/yamale/blockchain/oracle/v1/rate?pagination.limit=200', 'the live rates'),
        ]);
        const p = params.params ?? {};
        const live = (rates.rates ?? []).length;
        return proven(0, [
          { label: 'Currencies accepted', value: count((p.accepted_denoms ?? []).length, 'currency', 'currencies') },
          { label: 'Rates live right now', value: count(live, 'rate'), emphasis: live === 0 },
          { label: 'Vote period', value: blocksAbout(p.vote_period, ctx?.secondsPerBlock) ?? '—' },
          { label: 'Votes needed to set a rate', value: `${bps(p.vote_threshold_bps) ?? '—'} of voting power` },
        ], live === 0
          ? 'No rate is live. With two validators, a rate needs both to vote every period, and neither is running a '
            + 'price feeder. The module refusing to serve a stale number is why this reads nought rather than a figure from last week.'
          : '');
      } catch (e) { return describeFailure(e); }
    },
  },
  {
    id: 'amm',
    act: 'markets',
    module: 'x/amm',
    name: 'A pool that cannot be drained by rounding',
    does: 'Swaps one currency for another against a constant-product pool with a fee, so a small conversion does not need a counterparty.',
    refuses: 'An output rounded in the trader\'s favour. Every division rounds toward the pool — the arithmetically tidier form of the same formula rounds the other way and bleeds reserves one unit at a time, which is a real way pools have been emptied.',
    surface: 'markets',
    watch: 'The pools that exist and what is in them.',
    async read() {
      try {
        const body = await rest('/yamale/blockchain/amm/v1/pool', 'the pools');
        const pools = body.pool ?? [];
        const rows = [{ label: 'Pools', value: count(pools.length, 'pool') }];
        pools.forEach((p) => rows.push({
          label: `Pool ${p.id}`,
          value: `${amount(p.reserve_a, p.denom_a)} / ${amount(p.reserve_b, p.denom_b)} — ${bps(p.swap_fee_bps)} fee`,
          mono: true,
        }));
        return proven(0, rows, 'Seeded reserves on a devnet. Nobody has traded against them.');
      } catch (e) { return describeFailure(e); }
    },
  },
];

/**
 * What is honestly missing, stated on the page rather than left to be found.
 *
 * A tour that oversells gets caught in the room, and being caught is worse than
 * being modest — a ministry that finds one overstatement stops believing the
 * other fifteen claims, including the true ones.
 */
export const NOT_BUILT = [
  'There is no audit module. Nothing here produces an auditor\'s view of an institution\'s position.',
  'There is no account service. Opening an account for a real customer, with the identity checks that implies, is not built.',
  'Two validators produce every block, on a development network. Losing one stops the chain.',
  'No participant is approved in x/paymsg on this chain, so there is no completed institutional payment to show. The refusal is live; the happy path is not.',
  'No exchange rate is live, because neither validator runs a price feeder.',
  'The oversight and markets consoles are being written now. Their links here will be dead until they are deployed.',
  'Nothing on this chain has been through an external security audit.',
];
