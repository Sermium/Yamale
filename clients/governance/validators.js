// The validators this chain has, and the vote command for one of them.
//
// This used to live at clients/validator/vote.js and be imported by the
// governance page as `/validator/vote.js` — an absolute path into another
// console's directory. That was a production outage waiting to be noticed: the
// validator console had never been deployed under the site root, so the import
// answered 404, and a 404 on a module specifier does not degrade — the whole
// inline module fails to evaluate and the governance page renders its headings
// and nothing else. It looked exactly like the chain being unreachable. The
// validator console does not import this file and never did, so it lives with
// its only consumer and the governance console is now a directory that deploys
// on its own.
//
// Voting is the one action these pages offer, and it is offered *as a command
// to run on your own node*, not as a button that signs in the browser.
//
// That is not squeamishness. A validator's operator key lives in its node's
// keyring — it is not in a browser wallet and it should not be: a key that can
// vote on seizing other people's funds does not belong in a tab that also has
// forty other tabs open. So the page's job is to compose the exact command,
// correct for that validator and that proposal, and let the operator paste it
// where the key already is.
//
// The copy button matters more than it looks. An operator retyping a proposal
// id at 3am is an operator who votes on the wrong proposal.

export const VALIDATORS = [
  // moniker -> the local key name on that node, and where the node runs.
  { moniker: 'pi',   key: 'alice',  host: 'the Oracle VM', home: '/opt/yamale/node' },
  { moniker: 'pi-2', key: 'pival',  host: 'the Pi',        home: '/opt/yamale/join-node' },
];

/**
 * The vote command for one validator on one proposal.
 *
 * The chain id is a parameter and there is no default, for the same reason
 * `submitCommand` refuses to print without one: a command carrying the wrong
 * network is refused for a reason that has nothing to do with the vote, and a
 * command printed without one is a command somebody completes from memory. It is
 * read off the node the page is talking to, so a console opened against a
 * different chain composes commands for that chain.
 */
export function voteCommand({ validator, proposalId, option, chainId }) {
  if (!chainId) {
    throw new Error(
      'No chain id. A vote broadcast against the wrong chain is rejected for a reason that has '
      + 'nothing to do with the proposal.',
    );
  }
  return [
    '/opt/yamale/bin/blockchaind tx gov vote',
    proposalId,
    option,
    `--from ${validator.key}`,
    `--chain-id ${chainId}`,
    '--keyring-backend test',
    `--home ${validator.home}`,
    '--fees 2000uyml',
    '--yes',
  ].join(' ');
}
