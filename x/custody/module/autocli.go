package custody

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/custody/types"
)

// AutoCLIOptions builds the CLI from the proto service definitions, so the
// commands cannot drift from the messages they send.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Show the module parameters"},
				{RpcMethod: "Assets", Use: "assets", Short: "List the assets held in custody"},
				{
					RpcMethod: "Solvency",
					Use:       "solvency",
					Short:     "Issued versus held, per asset",
					Long: "Computed from the chain's own supply and the last attested reserve, " +
						"not reported by the custodian. Never attested counts as not solvent.",
				},
				{
					RpcMethod:      "Deposit",
					Use:            "deposit [id]",
					Short:          "Show one deposit",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "Redemption",
					Use:            "redemption [id]",
					Short:          "Show one redemption",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: types.Msg_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "AttestDeposit",
					Use:       "attest [denom] [recipient] [amount] [external-ref]",
					Short:     "Attest that a deposit arrived on the source chain",
					Long: "Signed by an appointed attestor. Nothing is minted until the threshold " +
						"of distinct attestors agree on the same denom, recipient, amount and reference.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom"}, {ProtoField: "recipient"},
						{ProtoField: "amount"}, {ProtoField: "external_ref"},
					},
				},
				{
					RpcMethod: "ReportReserve",
					Use:       "report-reserve [denom] [held]",
					Short:     "State what is held off-chain against an asset",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom"}, {ProtoField: "held"},
					},
				},
				{
					RpcMethod: "RequestRedemption",
					Use:       "redeem [denom] [amount] [destination]",
					Short:     "Burn a claim and queue the payout",
					Long: "The claim is burned immediately; the payout waits out the redemption " +
						"delay, which is the window in which a mistaken mint can still be caught.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom"}, {ProtoField: "amount"}, {ProtoField: "destination"},
					},
				},
				{
					RpcMethod: "SettleRedemption",
					Use:       "settle [redemption-id] [settled-ref]",
					Short:     "Record that the asset was sent on the source chain",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "redemption_id"}, {ProtoField: "settled_ref"},
					},
				},
				// Governance-only, submitted as proposal payloads. A command
				// that can only ever fail is a support ticket.
				{RpcMethod: "WithdrawFees", Skip: true}, // governance only
				{RpcMethod: "RegisterAsset", Skip: true},
				{RpcMethod: "SetAttestor", Skip: true},
				{RpcMethod: "UpdateParams", Skip: true},
			},
		},
	}
}
