/**
 * Who can send what, and where this console sends it.
 *
 * ===========================================================================
 * THIS FILE WAS MISSING, AND THE PAGE DID NOT LOAD
 * ===========================================================================
 *
 * `chain.js` has imported `./registrar.js` since it was written, and no such
 * file existed in this directory. An ES module import that 404s does not
 * degrade: the whole module graph fails, so `index.html` executed none of its
 * script and rendered its static markup and nothing else. A demonstrator that
 * looks like a page with no data is indistinguishable from a demonstrator
 * pointed at an empty register — which is exactly the confusion that cost a
 * day on the land register in this same repository, when a dead gateway was
 * read as an empty module.
 *
 * The lesson worth keeping is not "check imports". It is that a console whose
 * failure mode is a plausible-looking empty page needs something on screen
 * that can only appear if the code ran. Hence the chain head, height and all,
 * in the page's own header: if the height is missing, nothing behind it is
 * true either.
 *
 * ===========================================================================
 * THE THREE SHAPES A MESSAGE CAN TAKE
 * ===========================================================================
 *
 * Every write in x/herd is signed by one of three kinds of account, and which
 * one it is decides what a browser can do about it:
 *
 *   AN ORDINARY ACCOUNT — a keeper proposing a sale, a buyer accepting, any
 *   passer-by objecting. A browser signs these directly.
 *
 *   AN ACCREDITED AGENT — a veterinarian attesting. Also an ordinary key, but
 *   one the registry has admitted, so the chain refuses it from anybody else.
 *
 *   A REGISTRY OFFICE — enrolling a herd, identifying an animal, certifying
 *   parentage, resolving an objection. An office is a group account by
 *   construction (x/herd refuses to admit one that is not), so NO browser can
 *   produce its signature. What a browser can do is submit a proposal to the
 *   office's group policy in the registrar's own name, which is a different
 *   act by a different signer, and the office's members then vote.
 *
 * That third case is the design and not a limitation to be worked around. An
 * office that could act on one official's signature makes every registration
 * and every certification a single bribe, and no amount of cross-office quorum
 * on transfers repairs a register whose entries were wrong when written.
 */

/** The chain this console talks to, and what a command line would need. */
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
 * Generous rather than estimated. Nothing this console sends does unbounded
 * work — the largest is an objection writing a reason onto a transfer — and a
 * transaction that runs out of gas costs the fee and achieves nothing, which
 * is the worst of both outcomes for somebody standing in a market.
 */
export const GAS = 250000;

/**
 * The jurisdiction this demonstrator reads.
 *
 * Offices are admitted by chain governance and never by each other, which is
 * what stops a bergerie declaring itself a registry. Filtering the roll by
 * jurisdiction is therefore a real filter and not a cosmetic one: an animal
 * registered by an address that is not an admitted office of SN is not part of
 * the Senegalese register, whatever its record says.
 */
export const JURISDICTION = 'SN';

/**
 * How deep a pedigree this page asks for.
 *
 * Four generations is what a Ladoum certificate prints and what a buyer argues
 * about. The chain bounds the walk at ten whatever is asked, so this is a
 * choice about what to show rather than a safety limit — and when the bound is
 * what stopped the walk, the response says so and the page must repeat it.
 */
export const PEDIGREE_DEPTH = 4;

/**
 * Message type URLs used to search the transaction log.
 *
 * Discovery only, and worth being precise about why. x/herd publishes no
 * "every animal" query: a public list of who holds what valuable livestock,
 * and where, is a targeting list, and the module refuses to be one. So the
 * roll below learns which animals to ask about from the transaction log, and
 * every fact it then shows comes from state rather than from the log.
 *
 * The log is discovery and never evidence. A transaction that appears in it
 * may have failed; only state says what is true.
 */
export const MESSAGES = {
  enrol: '/blockchain.herd.v1.MsgEnrolHerd',
  identify: '/blockchain.herd.v1.MsgIdentifyAnimal',
  certify: '/blockchain.herd.v1.MsgCertifyAnimal',
  propose: '/blockchain.herd.v1.MsgProposeTransfer',
  object: '/blockchain.herd.v1.MsgObjectTransfer',
};
