package app

import (
	"slices"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"

	constitutionante "yamale/blockchain/x/constitution/ante"
	validatorgovante "yamale/blockchain/x/validatorgov/ante"
)

// newAnteHandler builds the app's AnteHandler. It replicates the Cosmos SDK
// default decorator chain (see x/auth/tx/config.newAnteHandler) and inserts
// the validatorgov gate that blocks MsgCreateValidator for candidates that
// haven't been approved through governance. The SDK's own default ante
// handler is disabled via SkipAnteHandler in app_config.go, so this is the
// only ante handler in effect.
func newAnteHandler(app *App) (sdk.AnteHandler, error) {
	decorators := slices.Concat([]sdk.AnteDecorator{
		authante.NewSetUpContextDecorator(), // outermost decorator, must run first
		authante.NewExtensionOptionsDecorator(nil),
		authante.NewValidateBasicDecorator(),
		authante.NewTxTimeoutHeightDecorator(),
		authante.NewValidateMemoDecorator(app.AuthKeeper),
		authante.NewConsumeGasForTxSizeDecorator(app.AuthKeeper),
		// Spliced immediately ahead of fee deduction rather than appended: a
		// profile that restricts which denominations may pay a fee has to say
		// so before the deduction fails on a balance instead. See
		// profile_settlementfee_on.go.
	}, app.profileAnteFeeDecorators(), []sdk.AnteDecorator{
		authante.NewDeductFeeDecorator(app.AuthKeeper, app.BankKeeper, app.FeeGrantKeeper, nil),
		authante.NewSetPubKeyDecorator(app.AuthKeeper), // must run before all signature verification decorators
		authante.NewValidateSigCountDecorator(app.AuthKeeper),
		authante.NewSigGasConsumeDecorator(app.AuthKeeper, authante.DefaultSigVerificationGasConsumer),
		authante.NewSigVerificationDecorator(app.AuthKeeper, app.txConfig.SignModeHandler()),
		authante.NewIncrementSequenceDecorator(app.AuthKeeper),
		// Both validatorgov decorators must run after SigVerificationDecorator.
		// The veto one especially: it treats a signature by a validator's
		// operator as proof that the operator's key is not lost, and placed
		// before verification an unsigned transaction claiming to be from that
		// operator would cancel a legitimate recovery for free.
		validatorgovante.NewOperatorVetoDecorator(app.ValidatorgovKeeper),
		validatorgovante.NewValidatorGateDecorator(app.ValidatorgovKeeper),
		// The demotion gate closes the route back out of a concentration
		// demotion. Jailing is how a breach is corrected, and x/staking's Jail
		// sets no jailed-until time, so without this an operator over its
		// ceiling could unjail itself in the next block and stay in the set
		// until somebody noticed.
		validatorgovante.NewDemotionGateDecorator(app.ValidatorgovKeeper),
		// The foundation group must stay the size and shape the constitution
		// says it is. Placed here rather than earlier for the same reason as
		// the validatorgov gates: it reads state and rejects, so paying for it
		// before the signature has been verified would let anyone make every
		// node do those store reads for free.
		constitutionante.NewFoundationGuardDecorator(app.ConstitutionKeeper, app.GroupKeeper),
	})

	return sdk.ChainAnteDecorators(decorators...), nil
}
