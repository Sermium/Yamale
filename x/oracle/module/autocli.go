package oracle

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/oracle/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Shows the parameters of the module",
				},
				{
					RpcMethod:      "ExchangeRate",
					Use:            "rate [denom]",
					Short:          "Shows the agreed price of a denom, with its age",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}},
				},
				{
					RpcMethod: "ExchangeRates",
					Use:       "rates",
					Short:     "Lists every agreed price",
				},
				{
					RpcMethod:      "Appraisal",
					Use:            "appraisal [class-id] [nft-id]",
					Short:          "Shows the current valuation of a tokenised asset",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "class_id"}, {ProtoField: "nft_id"}},
				},
				{
					RpcMethod:      "AppraisalHistory",
					Use:            "appraisal-history [class-id] [nft-id]",
					Short:          "Lists the superseded valuations of a tokenised asset",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "class_id"}, {ProtoField: "nft_id"}},
				},
				{
					RpcMethod:      "GetAppraiser",
					Use:            "appraiser [address]",
					Short:          "Shows one valuer and the scope it may value",
					Alias:          []string{"show-appraiser"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}},
				},
				{
					RpcMethod: "ListAppraiser",
					Use:       "list-appraiser",
					Short:     "Lists every valuer and applicant",
				},
				{
					RpcMethod: "MissCounters",
					Use:       "misses",
					Short:     "Shows how reliably each validator has been reporting",
				},
				{
					RpcMethod:      "FeederDelegation",
					Use:            "feeder [validator]",
					Short:          "Shows which account votes for a validator",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "validator"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              types.Msg_serviceDesc.ServiceName,
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // skipped because authority gated
				},
				{
					RpcMethod: "SubmitExchangeRates",
					Use:       "submit-rates [validator]",
					Short:     "Report observed prices for the current voting round",
					// One --rates per denom. Not a JSON array: autocli binds a
					// repeated message field as a repeatable flag, and an array
					// fails with "unexpected token [" — which reads like a
					// malformed payload rather than the wrong shape entirely.
					Long: "Report observed prices for the current voting round.\n\n" +
						"One --rates per denom, each a single JSON object:\n" +
						"  --rates '{\"denom\":\"uusd\",\"rate\":\"1.00\"}' \\\n" +
						"  --rates '{\"denom\":\"ueur\",\"rate\":\"1.15\"}'\n\n" +
						"A JSON array is not accepted.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "validator"}},
				},
				{
					RpcMethod:      "DelegateFeeder",
					Use:            "delegate-feeder [validator] [feeder]",
					Short:          "Nominate the hot key allowed to submit a validator's votes",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "validator"}, {ProtoField: "feeder"}},
				},
				{
					RpcMethod:      "ApplyAppraiser",
					Use:            "apply-appraiser [name] [credentials] [class-ids]",
					Short:          "Ask to be admitted as an independent valuer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "name"}, {ProtoField: "credentials"}, {ProtoField: "class_ids", Optional: true}},
				},
				{
					RpcMethod: "ApproveAppraiser",
					Skip:      true, // skipped because authority gated
				},
				{
					RpcMethod: "RevokeAppraiser",
					Skip:      true, // skipped because authority gated
				},
				{
					RpcMethod: "SubmitAppraisal",
					Use:       "submit-appraisal [class-id] [nft-id] [value] [value-denom] [valued-at] [method]",
					Short:     "Record a signed valuation of a tokenised asset",
					Long: "Record a signed valuation of a tokenised asset.\n\n" +
						"The report the number comes from is pinned with --report-uri and --report-hash, " +
						"so the on-chain value and the off-chain document cannot drift apart unnoticed.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "class_id"}, {ProtoField: "nft_id"}, {ProtoField: "value"}, {ProtoField: "value_denom"},
						{ProtoField: "valued_at"}, {ProtoField: "method"},
					},
				},
			},
		},
	}
}
