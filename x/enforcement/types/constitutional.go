package types

import (
	"fmt"

	constitutiontypes "yamale/blockchain/x/constitution/types"
)

// AssertConstitutional refuses a parameter set that disagrees with the values
// this chain fixed at genesis.
//
// Four of this module's parameters are held in x/constitution as well: the
// seizure threshold, the address seized assets go to, and the two delays that
// make a seizure answerable. They are not stored twice for redundancy — they
// are stored here because this is where they are read at speed, and there
// because governance must not be able to move them. This function is what makes
// the duplication safe: divergence is refused at both write paths, at
// MsgUpdateParams and at InitGenesis, so the two copies cannot drift rather
// than merely being unlikely to.
//
// Which four, and why those. A chain that can vote to lower its own seizure
// threshold does not have one. A destination governance can repoint turns a
// recovery mechanism into a way of paying somebody. A shortened voting period
// is how a supermajority becomes whoever happened to be awake. A shortened
// provisional freeze hands an account back in the middle of its own case. Every
// one of them is the kind of change that reads as housekeeping in a proposal
// and is not.
func (p Params) AssertConstitutional(inv constitutiontypes.Invariants) error {
	if p.ThresholdBps != inv.EnforcementThresholdBps {
		return fmt.Errorf(
			"threshold_bps is fixed at %d by this chain's constitution and cannot be set to %d by a parameter update; it takes an amendment",
			inv.EnforcementThresholdBps, p.ThresholdBps,
		)
	}
	if p.RecoveryDestination != inv.EnforcementRecoveryDestination {
		return fmt.Errorf(
			"recovery_destination is fixed at %s by this chain's constitution and cannot be set to %s by a parameter update; it takes an amendment",
			inv.EnforcementRecoveryDestination, p.RecoveryDestination,
		)
	}
	if p.VotingPeriodBlocks != inv.EnforcementVotingPeriodBlocks {
		return fmt.Errorf(
			"voting_period_blocks is fixed at %d by this chain's constitution and cannot be set to %d by a parameter update; it takes an amendment",
			inv.EnforcementVotingPeriodBlocks, p.VotingPeriodBlocks,
		)
	}
	if p.ProvisionalFreezeBlocks != inv.EnforcementProvisionalFreezeBlocks {
		return fmt.Errorf(
			"provisional_freeze_blocks is fixed at %d by this chain's constitution and cannot be set to %d by a parameter update; it takes an amendment",
			inv.EnforcementProvisionalFreezeBlocks, p.ProvisionalFreezeBlocks,
		)
	}
	return nil
}
