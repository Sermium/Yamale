// Shared by the validator console and the governance page.
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

export function voteCommand(v, proposalId, option) {
  return [
    '/opt/yamale/bin/blockchaind tx gov vote',
    proposalId,
    option,
    `--from ${v.key}`,
    '--chain-id yamale-devnet-1',
    '--keyring-backend test',
    `--home ${v.home}`,
    '--fees 2000uyml',
    '--yes',
  ].join(' ');
}
