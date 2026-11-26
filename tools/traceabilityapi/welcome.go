package main

// The devnet welcome. On a test network every account the sponsor creates is
// also handed test money (10,000 USDC on devnet-2), so a new person can try
// the marketplace without asking anyone for funds. The sponsor pays it from a
// stock the issuer minted to it.
//
// It is refused on any chain whose id does not name itself a test network. On
// a real chain money enters through an issuer, against real money that issuer
// received; a sponsor handing it out would be the platform printing money.

import (
	"context"
	"strings"
)

// Sender pays coins from the sponsor's account.
type Sender interface {
	// Send transfers coins (e.g. "10000000000uusdc") to the address and
	// returns the transaction hash once a block has committed it.
	Send(ctx context.Context, to, coins string) (string, error)
}

// welcomeAllowed reports whether a chain id names a local or devnet chain.
func welcomeAllowed(chainID string) bool {
	return strings.Contains(chainID, "devnet") || strings.Contains(chainID, "local")
}

func (g cliGranter) sendArgs(to, coins string) []string {
	args := []string{"tx", "bank", "send", g.from, to, coins,
		"--chain-id", g.chainID, "--keyring-backend", g.keyring}
	if g.home != "" {
		args = append(args, "--home", g.home)
	}
	return append(args, "--node", g.node,
		"--fees", g.fees, "--gas", "250000", "-y", "--output", "json")
}

// Send broadcasts a bank send from the sponsor and waits for its block.
func (g cliGranter) Send(ctx context.Context, to, coins string) (string, error) {
	return g.commit(ctx, g.sendArgs(to, coins))
}
