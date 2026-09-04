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
  // moniker -> the local key name on that node, where the node runs, the
  // address that key actually resolves to, and the validator's own operator
  // account.
  //
  // The last two are here because they are NOT the same, and the difference is
  // the whole reason this page used to lie. A validator's voting power belongs
  // to its operator account; the signing service holds an ordinary hot key. So
  // "vote as pi" signs with alice, and alice's stake is her own — delegated, as
  // it happens, to pi-2. Voting as pi has never moved pi's power and never can,
  // because pi's operator key is in a passphrase-protected store that no
  // service reaches.
  //
  // `voter` was previously a third address again — yml1nls726x…, which holds no
  // balance and no delegation on this chain at all. The page looked up whether
  // THAT had voted, so pi was reported as not having voted no matter what
  // anybody did. See signingPower().
  {
    moniker: 'pi',
    key: 'alice',
    host: 'the Oracle VM',
    home: '/opt/yamale/node',
    voter: 'yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg',
    operator: 'yml1m9xhc6zy7fxfax9t5fnykh9k2e29faj7htmqms',
  },
  {
    moniker: 'pi-2',
    key: 'pival',
    host: 'the Pi',
    home: '/opt/yamale/join-node',
    voter: 'yml1vlukxvmeg6kjtu658sc7lvlu6uj7c4n4p0fmas',
    operator: 'yml1cgguvt0hvdg2602flzan9shg0g56rujev5see4',
  },
];

/**
 * What a vote cast through the signing service would actually weigh.
 *
 * `delegations` is the chain's answer for the signing address. The point of
 * this function is that the answer is frequently "nothing", and a console that
 * offers a vote button without saying so is offering a control that does not do
 * what it appears to do.
 *
 * Returns the staked amount in base units and a plain sentence for the operator.
 */
export function signingPower(validator, delegations) {
  const staked = (delegations || []).reduce(
    (sum, d) => sum + BigInt(d.balance?.amount ?? d.shares ?? 0), 0n,
  );
  const isOperator = validator.voter === validator.operator;

  if (staked === 0n) {
    return {
      staked,
      canVote: false,
      note: `${validator.key} holds no delegation, so a vote signed with it counts for nothing. `
        + `${validator.moniker}'s power belongs to its operator account, whose key is not on any node.`,
    };
  }
  if (!isOperator) {
    return {
      staked,
      canVote: true,
      note: `Signed with ${validator.key}, not with ${validator.moniker}'s operator account. `
        + `The vote carries ${validator.key}'s own delegation, not ${validator.moniker}'s power.`,
    };
  }
  return { staked, canVote: true, note: '' };
}

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
