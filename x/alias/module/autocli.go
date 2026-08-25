package alias

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/alias/types"
)

// AutoCLIOptions builds the CLI from the proto service definitions, so the
// commands cannot drift from the messages they send.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Show the module parameters"},
				{
					RpcMethod:      "Alias",
					Use:            "resolve [id]",
					Short:          "Resolve a user ID to an address",
					Long:           "Accepts the identifier in any form: hyphenated or not, upper or lower case, with I and O where 1 and 0 belong.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "AliasOf",
					Use:            "id-of [address]",
					Short:          "Show the user ID held by an address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}},
				},
				{
					RpcMethod:      "Retired",
					Use:            "retired [id]",
					Short:          "Report whether a user ID has been given up",
					Long:           "A retired identifier resolves to nothing and is never issued again. This tells it apart from one that never existed.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "Jurisdiction",
					Use:            "jurisdiction [address]",
					Short:          "Show the country recorded against an account",
					Long:           "An account with none holds no user ID and will not be issued one, so \"not found\" here is an answer rather than a gap.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}},
				},
				{
					RpcMethod:      "Perimeter",
					Use:            "perimeter [country]",
					Short:          "List the accounts recorded in one country",
					Long:           "The accounts a national authority may act on, and no others. Returns jurisdiction records, not user IDs.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "country"}},
				},
				{
					RpcMethod:      "ViewingKeys",
					Use:            "viewing-keys [address]",
					Short:          "List every viewing key version an account has published",
					Long:           "Newest first, revoked ones included. A payload sealed last year names the version that was live last year, so the old keys are the answer rather than clutter.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}},
				},
				{
					RpcMethod:      "Regulator",
					Use:            "regulator [country]",
					Short:          "Show the authority that can open payloads settling in one country",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "country"}},
				},
				{
					RpcMethod:      "PayloadReaders",
					Use:            "payload-readers [country]",
					Short:          "List everyone entitled to open payloads settling in one country",
					Long:           "The appointed regulator and every supervisor granted in that country or chain-wide, each with the key a payload has to be wrapped to. This is the set a sender resolves before it seals; an entitled reader left out of an envelope can never open it, and nothing on chain detects that.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "country"}},
				},
				{
					RpcMethod: "Auditors",
					Use:       "auditors",
					Short:     "List the live cross-account reading grants",
					Long:      "Who may read payment detail belonging to people who never dealt with them. Listed because the people being read about are entitled to see it.",
				},
				{
					RpcMethod:      "RoleGrants",
					Use:            "role-grants [holder]",
					Short:          "Show every role an account holds, and where",
					Long:           "What the chain will let this key do. An empty answer means it may act nowhere, which is the default for every account.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "holder"}},
				},
				{
					RpcMethod:      "RoleHolders",
					Use:            "role-holders [jurisdiction]",
					Short:          "List who may act inside one country",
					Long:           "What that country granted. Chain-wide grants are deliberately not mixed in; see chain-wide-grants.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jurisdiction"}},
				},
				{
					RpcMethod: "ChainWideGrants",
					Use:       "chain-wide-grants",
					Short:     "List every role no border bounds",
					Long:      "The exception, on its own page. These accounts may act on any jurisdiction, so the list should be short and every entry should be one somebody can account for.",
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: types.Msg_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "RegisterAlias",
					Use:       "register",
					Short:     "Claim a user ID for your account",
					Long:      "The chain assigns the identifier; there is nothing to choose. One per account.",
				},
				{
					RpcMethod: "RotateAlias",
					Use:       "rotate",
					Short:     "Retire your user ID and take a new one",
					Long:      "For an account whose key was compromised. The old identifier is never issued again, so a payment sent to it arrives nowhere rather than with whoever took the key.",
				},
				{
					RpcMethod: "SetJurisdiction",
					Use:       "set-jurisdiction [account] [country]",
					Short:     "Record where an account is",
					Long:      "The approved participant that onboarded the account records it once. Correcting one already recorded is a foundation administrator's act, and it retires the account's user ID and issues a replacement carrying the new country — an identifier whose prefix could go stale is an identifier whose prefix can lie.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "account"},
						{ProtoField: "country"},
					},
				},
				{
					RpcMethod:      "RegisterViewingKey",
					Use:            "register-viewing-key [public-key]",
					Short:          "Publish the X25519 public key your payment payloads are sealed to",
					Long:           "Only the public half. Sending it again rotates: the old version stays queryable so payloads already sealed to it remain readable, and new payloads are sealed to the new one.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "public_key"}},
				},
				{
					RpcMethod:      "RevokeViewingKey",
					Use:            "revoke-viewing-key [version]",
					Short:          "Mark one of your key versions compromised",
					Long:           "Stops senders sealing to it. It does not make the payloads already sealed to it unreadable — ciphertext that has been distributed cannot be recalled. Destroying those is the payload store's job.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "version"}},
				},
				{
					RpcMethod: "AppointRegulator",
					Use:       "appoint-regulator [country] [address]",
					Short:     "Name the authority that may open payloads settling in one country",
					Long:      "Governance or a foundation administrator. The appointee can read the ISO 20022 detail of every payment declaring that country from the moment they are appointed.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "country"},
						{ProtoField: "address"},
					},
				},
				{
					RpcMethod: "GrantAuditor",
					Use:       "grant-auditor [address] [expires-at-height]",
					Short:     "Grant the time-boxed cross-account reading role",
					Long:      "Expires by itself at the height given; there is no unbounded form. This role reads payment detail belonging to people who never dealt with the holder.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "address"},
						{ProtoField: "expires_at_height"},
					},
				},
				// GrantRole and RevokeRole are still not offered, and the reason has
				// changed even though the conclusion has not.
				//
				// They used to be governance-only, which made this obvious. They now
				// accept governance OR the foundation for a country scope — and the
				// foundation is an x/group policy address, so neither acceptable
				// signer is an account anybody holds a key for. A `--from` on this
				// command can only ever name a key that is refused, and a command
				// that can only ever fail is a support ticket.
				//
				// Both arrive as proposal payloads: a governance proposal, or an
				// x/group proposal to the foundation. `ceremony country` composes the
				// second — see docs/guides/country-enrolment.md — through the chain's
				// own message types rather than by hand.
				{RpcMethod: "GrantRole", Skip: true},
				{RpcMethod: "RevokeRole", Skip: true},
				// UpdateParams is governance-only and is submitted as a proposal
				// payload, so it is deliberately not offered as a CLI command:
				// a command that can only ever fail is a support ticket.
				{RpcMethod: "UpdateParams", Skip: true},
			},
		},
	}
}
