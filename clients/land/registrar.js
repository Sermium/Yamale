// Who may send each of x/land's twelve messages, and therefore what this page
// is allowed to do about it.
//
// ===========================================================================
// THE DECISION THIS FILE EXISTS TO RECORD
// ===========================================================================
//
// This console used to answer every action the same way: a card saying "run
// this where the key is", with a `blockchaind tx land …` string to copy. That
// was not laziness — it was one correct answer applied to twelve questions that
// are not the same question, and it was wrong for the ones that matter most.
//
// The thing that decides the answer is `cosmos.msg.v1.signer`, read off
// proto/blockchain/land/v1/tx.proto and cross-checked against the keeper:
//
//   * `MsgRegisterAuthority` and `MsgUpdateParams` name `authority`, and the
//     keeper calls `assertGovernance` on it (x/land/keeper/msg_server_admin.go).
//     The signer is x/gov's module account. No person can produce it.
//
//   * Seven messages name `creator` and the keeper resolves it through
//     `activeAuthority`, which requires a registered office — and
//     `RegisterAuthority` refuses to admit an office that is not a group
//     account (`ErrOfficeNotGroup`, msg_server_admin.go:331). So the signer of
//     those is always a **group policy address**. A browser holding one key
//     cannot produce that signature either. This is a fact about the chain,
//     not a policy this page chose.
//
//   * Three messages name `creator` and the keeper checks it against a person:
//     the parcel's holder (`ProposeTransfer`), anybody at all (`Object`,
//     `CompleteTransfer`). Those are single people with single keys.
//
// From which three shapes fall out, and every message below is assigned one:
//
//   SIGN      The signer is one person's own key, so this page connects the
//             Yamale wallet and signs. Three messages.
//
//   PROPOSE   The signer is an office, so this page cannot sign it — but a
//             registrar can sign `MsgSubmitProposal` **in their own name**,
//             carrying the land message to their office's policy, where the
//             office's M-of-N then votes. That is a different act by a
//             different signer, and it is the honest browser action for an
//             office message. Seven messages.
//
//   COMMAND   Composed, never sent. Two messages, for two different reasons.
//
// Nothing here converts an office message into something a browser can sign.
// The point of the group account is that no single machine holds the office's
// signature, and a page that appeared to offer one would be lying about what
// it had done.
//
// ===========================================================================
// THE TWO THAT STAY AS COMMANDS, AND WHY THEY ARE NOT THE SAME CASE
// ===========================================================================
//
// `RegisterAuthority` is not an office action at all — it is a governance
// proposal, gated by `assertGovernance` precisely so that an office cannot
// admit an office and thereby manufacture the independent attestors a
// transfer's quorum rests on. A form that took a name and a jurisdiction and
// composed something would be composing a message that can never be signed by
// anybody reading this page. So the console says so, and composes the gov
// proposal document instead of a transaction.
//
// `FreezeParcel` is a group message like the other six, and this page could
// perfectly well submit a proposal for it. It deliberately does not. A freeze
// stops every dealing with somebody's land the moment it lands, its legitimate
// trigger — a court order, a fraud inquiry — arrives at the office on paper,
// and the person it is done to is never in the room. The office's M-of-N is
// unchanged either way; what changes is the cost of the *first move*. An
// official who must go to the registry terminal and type the order with its
// grounds faces a different threshold from one who taps a button on a phone
// while standing in front of the owner, and that difference is the only part
// of an extortion the chain can affect. So the console shows the freeze
// history in full, explains what lifting takes, and composes the command —
// which is exactly the affordance you want for the one action nobody should be
// able to take absent-mindedly.
//
// `Object` is the mirror image and gets the opposite treatment. It is open to
// anyone, needs no standing, and the people it protects are the ones with no
// official relationship to prove — a daughter, a neighbour, a co-heir. Making
// them install a chain binary to say "that is my father's land" excludes
// exactly the population the message was written for. It signs in the browser.
//
// ===========================================================================
// WHAT IS STILL MISSING, NAMED RATHER THAN WORKED AROUND
// ===========================================================================
//
// A holder can be a group account too — co-ownership is expressed that way on
// purpose. When it is, `ProposeTransfer` needs a group proposal rather than a
// signature, and this page cannot tell a group account from a key by looking
// at the address (both are bech32; only the length hints, and 32-byte
// addresses are not exclusively groups). So the transfer screen offers both
// paths and says which is which, rather than guessing and producing a
// signature the chain refuses.

import { GROUP_SUBMIT_PROPOSAL, MSG, TYPE, groupSubmitProposal, landAny } from './proto.js';

/** The chain these commands and transactions are for. One place, so a devnet
 *  reset is one edit. */
export const CHAIN = {
  id: 'yamale-devnet-2',
  bin: 'blockchaind',
  home: '~/.blockchain',
  fees: '2000uyml',
  gas: '400000',
};

/**
 * Gas for a browser-signed transaction.
 *
 * Generous rather than estimated. None of these messages does unbounded work —
 * the largest is an objection writing a reason onto a transfer — and a
 * simulation round-trip to shave the number would add a request on the
 * connection this page is actually opened on, to save a fee the chain does not
 * charge (`minimum-gas-prices = "0uyml"`).
 */
export const GAS = 250000;

/**
 * Message type URLs used to search the transaction log.
 *
 * Discovery only. x/land deliberately publishes no "every parcel" query — a
 * public list of who owns what is a targeting list — so two panels learn which
 * parcels to ask about from the log, and every fact they then show comes from
 * state.
 */
export const MESSAGES = {
  register: TYPE('MsgRegisterParcel'),
  fractionalise: TYPE('MsgAuthoriseFractionalisation'),
};

/* ========================================================================= */
/*  The catalogue                                                            */
/* ========================================================================= */

/**
 * All twelve, with the shape each one takes and the reason in the reader's
 * words rather than in the chain's.
 *
 *   name    the proto message
 *   sub     the autocli subcommand, for the composed command
 *   how     'sign' | 'propose' | 'command' | 'governance'
 *   who     who the chain will accept a signature from
 *   title   what a person calls this act
 *   why     shown on the page beside the action — the reason it takes the
 *           shape it does, because a control whose form is unexplained reads
 *           as an arbitrary restriction
 *   undo    what can be done afterwards. Empty string means nothing can.
 */
export const ACTIONS = {
  RegisterAuthority: {
    name: 'MsgRegisterAuthority',
    sub: null,                       // autocli skips it; there is no tx command
    how: 'governance',
    who: 'the chain’s governance, by a vote of the whole validator set',
    title: 'Admit a registry office',
    why: 'An office that could admit another office could manufacture the '
       + 'independent attestors a transfer’s quorum depends on, so buying one '
       + 'office would buy the whole mechanism. Admission is therefore a '
       + 'governance proposal and nothing else — not a form on this page, and '
       + 'not something an existing office can do.',
    undo: 'A later proposal can deactivate the office. Parcels it already '
        + 'registered stay registered and keep naming it.',
  },

  RegisterParcel: {
    name: 'MsgRegisterParcel',
    sub: 'register-parcel',
    how: 'propose',
    who: 'the registry office, through its own vote',
    title: 'Open a title over unregistered ground',
    why: 'First registration is the one act with no earlier record to check it '
       + 'against, which makes it the most attractive thing on this chain to '
       + 'get wrong on purpose. It is an office act, and offices decide by '
       + 'their own M-of-N.',
    undo: 'Nothing. There is no delete. A parcel registered in error is '
        + 'corrected by restrictions, freezes and the chain of title, all of '
        + 'which stay visible — the register is designed so that a mistake is '
        + 'arguable, not so that it is erasable.',
  },

  ProposeTransfer: {
    name: 'MsgProposeTransfer',
    sub: 'propose-transfer',
    how: 'sign',
    who: 'the current holder, and nobody else',
    title: 'Offer your land to a buyer',
    why: 'No office can start the sale of somebody’s land. That is the first '
       + 'of the four separations, and it is why this is signed by a person '
       + 'rather than proposed to an office.',
    undo: 'The parcel is locked to this transfer until it completes or somebody '
        + 'objects. No second transfer can start meanwhile.',
  },

  ValidateTransfer: {
    name: 'MsgValidateTransfer',
    sub: 'validate-transfer',
    how: 'propose',
    who: 'the office whose jurisdiction the parcel falls in',
    title: 'Confirm the seller against the paper file',
    why: 'Only the office holding the file can check the seller against paper '
       + 'the chain cannot see. Exactly one validation is accepted and it does '
       + 'not count toward the attestation quorum.',
    undo: 'Nothing. A validation cannot be withdrawn; an objection is what '
        + 'stops a transfer after it.',
  },

  AttestTransfer: {
    name: 'MsgAttestTransfer',
    sub: 'attest-transfer',
    how: 'propose',
    who: 'an active office that is NOT the one holding the parcel',
    title: 'Attest to a transfer as an independent office',
    why: 'An attestor from the same office is not independent, and allowing it '
       + 'collapses a quorum of many offices back into a single bribe. One '
       + 'office, one attestation.',
    undo: 'Nothing. Reaching quorum starts the challenge window, and the clock '
        + 'runs from that moment rather than from the proposal.',
  },

  Object: {
    name: 'MsgObject',
    sub: 'object',
    how: 'sign',
    who: 'anybody at all — no standing has to be proved',
    title: 'Object to a transfer',
    why: 'Somebody whose land is being sold from under them usually has no '
       + 'official relationship to prove, and requiring standing would exclude '
       + 'exactly the people this protects. Requiring a command-line tool '
       + 'would exclude them just as effectively, which is why this one signs '
       + 'here.',
    undo: '',   // terminal, and the page must say so before the signature
    terminal: 'One objection is enough and it cannot be withdrawn. The transfer '
            + 'stops, the parcel is marked disputed, and it stays disputed '
            + 'until a court decides — the chain preserves the evidence and '
            + 'does not adjudicate. The reason you give goes on the permanent '
            + 'record and is what that court reads.',
  },

  CompleteTransfer: {
    name: 'MsgCompleteTransfer',
    sub: 'complete-transfer',
    how: 'sign',
    who: 'anybody at all — the buyer, the seller, or a stranger',
    title: 'Complete a transfer',
    why: 'Mechanical: the chain checks every condition and applies the result. '
       + 'If only an official could finalise a transfer, an official could '
       + 'refuse to — and a refusal that costs a seller their sale is leverage '
       + 'worth paying to remove.',
    undo: '',
    terminal: 'This moves the title. The land becomes the buyer’s in the block '
            + 'this lands in, and nothing on the chain reverses it — a court '
            + 'can order a further transfer, but the register has no undo.',
  },

  RecordEncumbrance: {
    name: 'MsgRecordEncumbrance',
    sub: 'record-encumbrance',
    how: 'propose',
    who: 'the office in charge of the parcel',
    title: 'Record a mortgage, lien or right of way — or release one',
    why: 'A title shown without its encumbrances is a lie that gets somebody’s '
       + 'house taken, so recording one is an office act with the office’s own '
       + 'quorum behind it.',
    undo: 'A release marks the entry rather than deleting it. An encumbrance '
        + 'that vanishes takes with it the evidence that it ever constrained '
        + 'the title.',
  },

  FreezeParcel: {
    name: 'MsgFreezeParcel',
    sub: 'freeze-parcel',
    how: 'command',
    who: 'the office in charge of the parcel, through its own vote',
    title: 'Stop all dealings with a parcel — or lift a stop',
    why: 'This is the only action this console will not start for you, and the '
       + 'reason is not that the chain forbids it. A freeze halts everything a '
       + 'holder can do with their land the moment it lands; its legitimate '
       + 'trigger arrives at the office on paper; and the person it is done to '
       + 'is never in the room. The office’s vote is required either way. What '
       + 'a button would change is the cost of the first move, and that cost '
       + 'is the only part of an extortion a register can affect.',
    undo: 'Liftable by the same office, with grounds, and both the freeze and '
        + 'the lift stay on the parcel for good. A holder can read why their '
        + 'land was stopped and argue with it.',
  },

  AttachDeed: {
    name: 'MsgAttachDeed',
    sub: 'attach-deed',
    how: 'propose',
    who: 'the office in charge of the parcel',
    title: 'Add a document to the chain of title',
    why: 'The chain carries the hash and a pointer, never the document — a scan '
       + 'of a 1974 grant is megabytes and usually carries somebody’s personal '
       + 'details. The registry serves the paper to whoever is entitled to '
       + 'read it.',
    undo: 'Nothing. A deed is added, never removed: the chain of title is the '
        + 'receipt a dispossessed owner does not otherwise get.',
  },

  SetRestriction: {
    name: 'MsgSetRestriction',
    sub: 'set-restriction',
    how: 'propose',
    who: 'the office in charge of the parcel',
    title: 'Impose a limit on what may be done with a parcel — or lift one',
    why: 'Land law differs by country, so the limits are data rather than code. '
       + 'A chain that hard-codes one country’s law is a chain only that '
       + 'country can use.',
    undo: 'Lifting marks the restriction rather than removing it, so the record '
        + 'still shows the land was once constrained and which office released '
        + 'it.',
  },

  AuthoriseFractionalisation: {
    name: 'MsgAuthoriseFractionalisation',
    sub: 'authorise-fractionalisation',
    how: 'propose',
    who: 'the office in charge of the parcel',
    title: 'Permit shares to be sold over a parcel',
    why: 'Without a live authorisation x/tokenisation refuses to open a '
       + 'vehicle. The title never leaves this register — what is sold is a '
       + 'right referencing it, up to a ceiling, until an expiry.',
    undo: 'Withdrawal stops new issuance. It does not expropriate existing '
        + 'holders — that is a taking, and it belongs to a court.',
  },
};

/** The twelve, in the order a reader meets them. */
export const ACTION_NAMES = Object.keys(ACTIONS);

/** Every action that takes one shape. */
export const actionsBy = (how) =>
  ACTION_NAMES.filter((k) => ACTIONS[k].how === how);

/**
 * Whether this page will produce a signature for a message, and from whose key.
 *
 * Deliberately a function of the catalogue rather than of the caller, so a new
 * screen cannot decide for itself that an office message is signable.
 */
export function signable(action) {
  const spec = ACTIONS[action];
  if (!spec) throw new Error(`no such land action: ${action}`);
  return spec.how === 'sign';
}

/** True when the page composes a group proposal for an office to vote on. */
export function proposable(action) {
  const spec = ACTIONS[action];
  if (!spec) throw new Error(`no such land action: ${action}`);
  return spec.how === 'propose';
}

/* ========================================================================= */
/*  Composing what gets signed                                               */
/* ========================================================================= */

/**
 * The message a person signs with their own key.
 *
 * Refuses anything the catalogue does not mark `sign`. That refusal is the
 * enforcement of everything argued at the top of this file: without it the
 * rule lives only in a comment, and the first screen in a hurry breaks it.
 */
export function personalMessage(action, fields) {
  if (!signable(action)) {
    throw new Error(
      `${action} is not signed by a person: ${ACTIONS[action].who}. `
      + `See registrar.js — this page does not produce an office's signature.`);
  }
  return landAny(ACTIONS[action].name, fields);
}

/**
 * A registrar's proposal to their own office, carrying one land message.
 *
 * `creator` inside the land message is the **office**, because the office is
 * what will execute it; `proposers` is the **registrar**, because the
 * registrar is what signs this transaction. Swapping them is the mistake worth
 * guarding: a proposal proposed by the office is one no registrar can sign,
 * and a land message whose creator is a registrar is one the keeper refuses
 * with `ErrNotAnAuthority` after the office has already voted on it.
 */
export function officeProposal(action, { office, proposer, fields, metadata }) {
  if (!proposable(action)) {
    throw new Error(
      `${action} is not an office proposal: ${ACTIONS[action].who}.`);
  }
  if (!office) throw new Error('an office proposal needs the office it is put to');
  if (!proposer) throw new Error('an office proposal needs the registrar proposing it');
  const inner = landAny(ACTIONS[action].name, { ...fields, creator: office });
  return {
    typeUrl: GROUP_SUBMIT_PROPOSAL,
    bytes: groupSubmitProposal({
      policy: office,
      proposers: [proposer],
      metadata: metadata ?? ACTIONS[action].title,
      messages: [inner],
    }),
  };
}

/**
 * What a proposal will do, in the words the office's other registrars will read
 * it in. Kept short: x/group metadata is stored on-chain and some deployments
 * cap it, and a paragraph nobody reads is worse than a line somebody does.
 */
export function proposalMetadata(action, summary) {
  const line = `${ACTIONS[action].title}${summary ? ` — ${summary}` : ''}`;
  return line.length <= 255 ? line : `${line.slice(0, 252)}…`;
}

/* ========================================================================= */
/*  Composing what gets typed                                                */
/* ========================================================================= */

/** Shell-quotes a value only when it needs it, so ordinary ids stay readable. */
export function sh(value) {
  const s = String(value ?? '');
  if (s !== '' && /^[A-Za-z0-9_@%+=:,.\/-]+$/.test(s)) return s;
  return `'${s.replace(/'/g, `'\\''`)}'`;
}

const commonFlags = (from) => [
  `--from ${from}`,
  `--chain-id ${CHAIN.id}`,
  `--home ${CHAIN.home}`,
  `--fees ${CHAIN.fees}`,
  `--gas ${CHAIN.gas}`,
];

/**
 * One `blockchaind tx land …` command, wrapped for reading.
 *
 * `args` are the positionals in the order autocli declares them; `flags` are
 * the named ones. Which of the two a field is happens to be load-bearing —
 * several of these messages take flags rather than positionals — so the caller
 * states it rather than the composer guessing.
 */
export function landTx({ sub, args = [], flags = [], from }) {
  const lines = [`${CHAIN.bin} tx land ${sub}`];
  [...args.map(sh), ...flags, ...commonFlags(from)].forEach((part) => {
    const last = lines[lines.length - 1];
    if (last.length + part.length + 1 <= 78) lines[lines.length - 1] = `${last} ${part}`;
    else lines.push(`  ${part}`);
  });
  return lines.map((l, i) => (i < lines.length - 1 ? `${l} \\` : l)).join('\n');
}

/**
 * The office's version of a command.
 *
 * An office cannot broadcast: it is a group account, so the command is
 * generated unsigned and submitted as a proposal its registrars vote on. That
 * second step is the office's own M-of-N, and printing only the first step
 * would suggest a clerk can move land alone.
 */
export function officeTx(spec, proposalFile = 'proposal-msg.json') {
  const generate = `${landTx(spec)} \\\n  --generate-only > ${proposalFile}`;
  const submit = [
    `${CHAIN.bin} tx group submit-proposal ${proposalFile} \\`,
    `  --from ${spec.from} --chain-id ${CHAIN.id} --home ${CHAIN.home}`,
  ].join('\n');
  return `${generate}\n\n${submit}`;
}

/** An office signs through its group policy, so the office's own step comes first. */
export function groupPreamble(officeName) {
  return (
    `${officeName} is a group account. The command below is generated unsigned and `
    + `submitted as a group proposal that its registrars vote on first, so this is `
    + `not something one registrar can send alone.`
  );
}

/**
 * The governance proposal that admits a registry office.
 *
 * A document rather than a command, because `RegisterAuthority` has no tx
 * subcommand at all — autocli skips it (`Skip: true`) for the same reason the
 * keeper gates it. What can be filed is a `submit-proposal` carrying the
 * message, and the `authority` field must be x/gov's own module account: the
 * chain compares the message's authority against it and refuses anything else,
 * so a proposal naming a person is a proposal that passes its vote and then
 * fails to execute.
 */
export function admissionProposal({
  govAuthority, office, name, jurisdiction, title, summary, deposit,
}) {
  // Both read from the chain by the caller, and both refused here rather than
  // defaulted. A wrong authority produces a proposal that passes its vote and
  // then fails to execute; a deposit below the minimum produces one that never
  // enters voting at all, which from outside looks exactly like a proposal
  // nobody supported. Neither failure names itself, so neither gets a fallback.
  if (!govAuthority) throw new Error('the gov module account must be read from the chain');
  if (!deposit) throw new Error('the minimum deposit must be read from the chain');
  return JSON.stringify({
    messages: [{
      '@type': TYPE('MsgRegisterAuthority'),
      authority: govAuthority,
      office,
      name,
      // Two letters, and normalised by the keeper. A jurisdiction the chain
      // does not recognise is refused with ErrInvalidJurisdiction — the office
      // would look admitted and be unable to register a single parcel.
      jurisdiction: String(jurisdiction || '').toUpperCase(),
      active: true,
    }],
    metadata: '',
    deposit,
    title,
    summary,
  }, null, 2);
}

/**
 * The other half of admitting an office, which is not in x/land at all.
 *
 * `MsgRegisterAuthority` writes the record `query land authorities` reads.
 * Every message the office then sends resolves through `activeAuthority`, which
 * asserts a `ROLE_REGISTRY_AUTHORITY` grant in **x/alias** covering the same
 * country. Without it the office is listed, marked active, and refused by every
 * message it sends — which reads as a bug in the admission and is not one.
 *
 * The two acts have different signers, and that is why they cannot be one
 * proposal composed here without saying so: the x/land admission is governance,
 * while a country-scoped role grant may come from governance *or* from the
 * foundation's 3-of-5. Bringing a country onto the rail is the foundation's act
 * by design — it is an M-of-N group per office, several grants across them, and
 * the jurisdiction records, in a particular order.
 *
 * Composed as a governance proposal here because that is the path that needs no
 * standing to file. A deployment whose foundation is doing the enrolment will
 * use its own runbook and does not need a page for it.
 */
export function roleGrantProposal({
  govAuthority, office, name, jurisdiction, deposit,
}) {
  if (!govAuthority) throw new Error('the gov module account must be read from the chain');
  if (!deposit) throw new Error('the minimum deposit must be read from the chain');
  return JSON.stringify({
    messages: [{
      '@type': '/blockchain.alias.v1.MsgGrantRole',
      authority: govAuthority,
      holder: office,
      // The enum by name: proto3 JSON accepts either, and the name is what a
      // reviewer of the proposal can check without a lookup table.
      role: 'ROLE_REGISTRY_AUTHORITY',
      jurisdiction: String(jurisdiction || '').toUpperCase(),
    }],
    metadata: '',
    deposit,
    title: `Grant ${name} authority over land in ${String(jurisdiction || '').toUpperCase()}`,
    summary: `${name} is admitted to the land register but holds no role grant, so every `
      + `message it sends is refused. This grants ROLE_REGISTRY_AUTHORITY for `
      + `${String(jurisdiction || '').toUpperCase()}.`,
  }, null, 2);
}

/** The command that files either of the documents above. */
export function admissionCommand(from, file = 'admit-office.json') {
  return [
    `${CHAIN.bin} tx gov submit-proposal ${file} \\`,
    `  --from ${from} --chain-id ${CHAIN.id} --home ${CHAIN.home} \\`,
    `  --fees ${CHAIN.fees} --gas ${CHAIN.gas}`,
  ].join('\n');
}

/* ========================================================================= */
/*  Reading a refusal                                                        */
/* ========================================================================= */

/**
 * x/land's refusals, in the words of the person they happened to.
 *
 * Keyed by `codespace` and `code` exactly as they come back on a transaction
 * result, and transcribed from x/land/types/errors.go — which is worth reading:
 * those errors are already written for a registrar rather than for a node
 * operator. What they are not written for is the second reader of this page,
 * who is about to hand over their savings for a field and needs to be told what
 * the refusal means for them.
 *
 * `text` is the chain's own description, kept so a match can be made on the log
 * line when a code is not available (a CheckTx rejection carries the log and
 * not always the pair). `says` is the sentence shown.
 *
 * Anything not listed falls through to the raw text rather than to a reassuring
 * generic sentence. A refusal this file has not learned yet must not be
 * smoothed into one it has — that is how "your objection was not recorded"
 * comes to read as "everything is fine".
 */
export const CODESPACE = 'land';

export const REFUSALS = {
  1: { text: 'signer is not a registry office',
       says: 'The chain does not recognise that account as a registry office. Offices '
           + 'are admitted by governance, never by each other.' },
  2: { text: 'this registry office is not active',
       says: 'That office exists on the register but has been deactivated, so it can '
           + 'no longer act.' },
  3: { text: 'a survey hash is required',
       says: 'No survey hash was given. It is the field that makes the parcel unique.' },
  4: { text: 'a cadastral reference is required',
       says: 'No cadastral reference was given — the number on the paper file.' },
  5: { text: 'the holder is not a valid account',
       says: 'The holder’s account reference is not a valid address on this chain.' },
  6: { text: 'this ground is already titled',
       says: 'This ground is already titled. The survey hash is the uniqueness '
           + 'constraint, and refusing a second title over it is the whole “cannot be '
           + 'owned twice” guarantee.' },
  7: { text: 'this cadastral reference is already used',
       says: 'Another parcel already uses that cadastral reference. Two records '
           + 'claiming to be the same paper file makes reconciliation guesswork.' },
  8: { text: 'no such parcel', says: 'The register holds no parcel with that number.' },
  9: { text: 'only the holder may propose a transfer',
       says: 'Only the current holder can offer their own land. No office can start '
           + 'the sale of somebody’s land — that is the first of the four separations.' },
  10: { text: 'this parcel cannot be transferred in its current state',
        says: 'The parcel is frozen, disputed, or already has a transfer under way. '
            + 'Those are stops rather than warnings.' },
  11: { text: 'the recipient is not a valid account',
        says: 'The buyer’s account reference is not a valid address on this chain.' },
  12: { text: 'the recipient is already the holder',
        says: 'The buyer named is the person who already holds the land.' },
  13: { text: 'no such transfer', says: 'The register holds no transfer with that number.' },
  14: { text: 'this transfer is already complete',
        says: 'This transfer has already completed. The land has changed hands and an '
            + 'objection can no longer stop it — a court can still undo it.' },
  15: { text: 'this transfer has been objected to',
        says: 'Somebody has already objected. One objection is enough, and it is '
            + 'terminal: the parcel is disputed until a court decides.' },
  16: { text: 'only the office holding this parcel may validate it',
        says: 'Validation belongs to the office whose jurisdiction the parcel falls '
            + 'in — the one that can check the seller against paper the chain cannot see.' },
  17: { text: 'already validated',
        says: 'This transfer has already been validated. Exactly one validation is '
            + 'accepted, and it does not count toward the attestation quorum.' },
  18: { text: "an attestor from the parcel's own office is not independent",
        says: 'An attestor from the parcel’s own office is not independent, and '
            + 'allowing it would collapse a quorum of many offices into a single bribe.' },
  19: { text: 'this office has already attested',
        says: 'That office has already attested. One office, one attestation.' },
  20: { text: 'not yet validated by the office in charge',
        says: 'The office holding the parcel’s file has not validated this transfer '
            + 'yet. Step 2 comes before step 3.' },
  21: { text: 'not enough independent attestations',
        says: 'Not enough independent offices have attested yet.' },
  22: { text: 'the challenge window has not closed yet',
        says: 'The challenge window is still open. Anybody may still object, and until '
            + 'it closes nothing can be completed.' },
  23: { text: 'an objection must give a reason',
        says: 'An objection must give a reason. That is the whole point of it — the '
            + 'reason is what a court reads afterwards.' },
  24: { text: 'this message may only come from governance',
        says: 'Only the chain’s governance can send this. An office that could admit '
            + 'an office could manufacture the attestors a quorum depends on.' },
  25: { text: 'a document hash is required',
        says: 'No document hash was given. The chain carries the hash, never the '
            + 'document itself.' },
  26: { text: 'no such restriction',
        says: 'There is no restriction at that position. `query land parcel` numbers '
            + 'them from zero.' },
  27: { text: 'a restriction kind is required', says: 'No restriction kind was given.' },
  28: { text: 'no such encumbrance',
        says: 'There is no encumbrance at that position. `query land parcel` numbers '
            + 'them from zero.' },
  29: { text: 'this parcel is not frozen',
        says: 'There is no freeze on this parcel to lift.' },
  30: { text: 'the share ceiling must be between 1 and 10000 basis points',
        says: 'The share ceiling must be between 1 and 10,000 basis points — that is '
            + '0.01% to 100%.' },
  31: { text: 'a restriction on this parcel forbids fractionalisation',
        says: 'A restriction on this parcel forbids fractionalisation, and a '
            + 'restriction outranks an office’s permission. Otherwise recording '
            + 'restrictions would be decorative.' },
  32: { text: 'a registry office must be a group account',
        says: 'An office must be a group account, so that every decision it makes '
            + 'already needs several registrars to agree. A single key would make each '
            + 'of them one bribe.' },
  33: { text: 'the authorisation must expire at a time in the future',
        says: 'The authorisation must expire at a time in the future. One with no '
            + 'expiry sits open for years, which is the thing that field exists to '
            + 'prevent.' },
  34: { text: 'this parcel has no fractionalisation authorisation to withdraw',
        says: 'There is no fractionalisation authorisation on this parcel to withdraw.' },
  35: { text: "a registry office's jurisdiction must be an assigned ISO 3166-1 alpha-2 country code",
        says: 'The jurisdiction must be a two-letter country code the chain '
            + 'recognises. An office admitted for somewhere that is not a country '
            + 'would look admitted and be unable to register a single parcel.' },
};

/**
 * Turn a chain refusal into a sentence, keeping the original.
 *
 * Two ways in, because a refusal arrives two ways. A transaction result carries
 * `codespace` and `code`, which is exact. A CheckTx rejection and an ABCI query
 * failure carry only a log line, so the description text is matched as well —
 * those strings are stable in errors.go and a change to one is a visible diff
 * there rather than a silent drift here.
 *
 * `known: false` is returned rather than a soothing default. The raw text is
 * always carried through: an operator who cannot see the original stops
 * trusting the translation, and is right to.
 */
export function explainRefusal({ codespace, code, log } = {}) {
  const raw = String(log ?? '');
  if (codespace === CODESPACE && REFUSALS[code]) {
    return { known: true, says: REFUSALS[code].says, code, raw };
  }
  const lower = raw.toLowerCase();
  for (const [code_, entry] of Object.entries(REFUSALS)) {
    if (lower.includes(entry.text.toLowerCase())) {
      return { known: true, says: entry.says, code: Number(code_), raw };
    }
  }
  // Not a land refusal at all: an ante-handler rejection, an unknown account, a
  // sequence mismatch. Those are worth naming as *not* being about the land.
  if (lower.includes('account sequence mismatch')) {
    return { known: true, code: null, raw,
      says: 'The account’s sequence number moved between reading it and signing — '
          + 'usually another transaction from the same account. Nothing was recorded; '
          + 'try again.' };
  }
  if (lower.includes('key not found') || lower.includes('unknown address')) {
    return { known: true, code: null, raw,
      says: 'The chain has never seen that account. An account exists once something '
          + 'has been sent to it, and until then it cannot sign anything.' };
  }
  return { known: false, says: '', code: null, raw };
}

export { GROUP_SUBMIT_PROPOSAL, MSG, TYPE };
