// Composing the registry's transactions, for an official to run where their key is.
//
// The land registry offers no signing button, and the reason is the same one
// that keeps the validator console read-only — but sharper here, because of who
// signs what.
//
// A registry office is an **x/group account**. Its decisions are M-of-N by
// construction: registering a parcel, validating a transfer, freezing land are
// each a group proposal that several registrars vote on before the office
// signs anything at all. That is the module's whole intra-office protection
// (`ErrOfficeNotGroup`), and a browser page that could produce an office's
// signature on its own would be a way around it.
//
// So this file does what `vote.js` does for governance: it composes the exact
// command to run, correct for that office and that parcel, and leaves the
// signing where the key is. The copy button matters more than it looks — a
// clerk retyping a parcel id is a clerk registering a deed against the wrong
// field.
//
// x/land now has AutoCLIOptions, so what is composed here is the real
// `blockchaind tx land …` command rather than a hand-assembled transaction
// document. That matters beyond tidiness: a document this page builds is a
// document this page can get wrong, and it is checked against nothing until it
// is broadcast. A CLI command is parsed by the binary that owns the message —
// a wrong flag fails at the terminal with the field named, in front of the
// person who can fix it, rather than as a decoded rejection in a block.
//
// The one thing still missing, named rather than worked around: `@yamale/connect`
// is TypeScript with a CosmJS dependency, so it cannot be imported by a page
// with no build step. Signing in the browser through the wallet is the right
// end state for the *citizen* messages — objecting to a transfer, and a holder
// proposing one — because those are single people with single keys, not
// offices. It is not the right end state for an office, for the reason above.

/** The chain these commands are for. One place, so a devnet reset is one edit. */
export const CHAIN = {
  id: 'yamale-devnet-2',
  bin: 'blockchaind',
  home: '~/.blockchain',
  fees: '2000uyml',
  gas: '400000',
};

/**
 * Message type URLs, used only to search the transaction log.
 *
 * Not used to build transactions any more — the CLI owns that. They survive
 * because two of this page's panels need to know *which* parcels to ask the
 * register about, and x/land deliberately publishes no "every parcel" query: a
 * public list of who owns what is a targeting list. Discovery therefore comes
 * from the log; every fact shown afterwards comes from state.
 */
export const MESSAGES = {
  register: '/blockchain.land.v1.MsgRegisterParcel',
  fractionalise: '/blockchain.land.v1.MsgAuthoriseFractionalisation',
};

/**
 * The flags every one of these commands needs, kept in one place so a composed
 * command can be pasted and run rather than pasted and then corrected.
 */
const commonFlags = (from) => [
  `--from ${from}`,
  `--chain-id ${CHAIN.id}`,
  `--home ${CHAIN.home}`,
  `--fees ${CHAIN.fees}`,
  `--gas ${CHAIN.gas}`,
];

/** Shell-quotes a value only when it needs it, so ordinary ids stay readable. */
export function sh(value) {
  const s = String(value ?? '');
  if (s !== '' && /^[A-Za-z0-9_@%+=:,.\/-]+$/.test(s)) return s;
  return `'${s.replace(/'/g, `'\\''`)}'`;
}

/**
 * One `blockchaind tx land …` command, wrapped for reading.
 *
 * `args` are the positionals the subcommand takes, in the order autocli
 * declares them; `flags` are the named ones. Which of the two a field is
 * happens to be load-bearing — several of these messages take flags rather
 * than positionals — so the caller states it rather than the composer guessing.
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
    `${officeName} is a group account. The command below is generated unsigned and ` +
    `submitted as a group proposal that its registrars vote on first, so this is ` +
    `not something one registrar can send alone.`
  );
}
