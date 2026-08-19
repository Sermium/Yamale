package app

import (
	"slices"
	"time"
	_ "yamale/blockchain/x/alias/module"
	aliasmoduletypes "yamale/blockchain/x/alias/types"
	_ "yamale/blockchain/x/enforcement/module"
	enforcementmoduletypes "yamale/blockchain/x/enforcement/types"
	_ "yamale/blockchain/x/oracle/module"
	oraclemoduletypes "yamale/blockchain/x/oracle/types"
	_ "yamale/blockchain/x/paymsg/module"
	paymsgmoduletypes "yamale/blockchain/x/paymsg/types"
	_ "yamale/blockchain/x/stablecoin/module"
	stablecoinmoduletypes "yamale/blockchain/x/stablecoin/types"
	_ "yamale/blockchain/x/treasury/module"
	treasurymoduletypes "yamale/blockchain/x/treasury/types"
	_ "yamale/blockchain/x/validatorgov/module"
	validatorgovmoduletypes "yamale/blockchain/x/validatorgov/types"

	runtimev1alpha1 "cosmossdk.io/api/cosmos/app/runtime/v1alpha1"
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	authzmodulev1 "cosmossdk.io/api/cosmos/authz/module/v1"
	bankmodulev1 "cosmossdk.io/api/cosmos/bank/module/v1"
	circuitmodulev1 "cosmossdk.io/api/cosmos/circuit/module/v1"
	consensusmodulev1 "cosmossdk.io/api/cosmos/consensus/module/v1"
	distrmodulev1 "cosmossdk.io/api/cosmos/distribution/module/v1"
	epochsmodulev1 "cosmossdk.io/api/cosmos/epochs/module/v1"
	evidencemodulev1 "cosmossdk.io/api/cosmos/evidence/module/v1"
	feegrantmodulev1 "cosmossdk.io/api/cosmos/feegrant/module/v1"
	genutilmodulev1 "cosmossdk.io/api/cosmos/genutil/module/v1"
	govmodulev1 "cosmossdk.io/api/cosmos/gov/module/v1"
	groupmodulev1 "cosmossdk.io/api/cosmos/group/module/v1"
	nftmodulev1 "cosmossdk.io/api/cosmos/nft/module/v1"
	paramsmodulev1 "cosmossdk.io/api/cosmos/params/module/v1"
	slashingmodulev1 "cosmossdk.io/api/cosmos/slashing/module/v1"
	stakingmodulev1 "cosmossdk.io/api/cosmos/staking/module/v1"
	txconfigv1 "cosmossdk.io/api/cosmos/tx/config/v1"
	upgrademodulev1 "cosmossdk.io/api/cosmos/upgrade/module/v1"
	vestingmodulev1 "cosmossdk.io/api/cosmos/vesting/module/v1"
	"cosmossdk.io/depinject/appconfig"
	_ "cosmossdk.io/x/circuit" // import for side-effects
	circuittypes "cosmossdk.io/x/circuit/types"
	_ "cosmossdk.io/x/evidence" // import for side-effects
	evidencetypes "cosmossdk.io/x/evidence/types"
	"cosmossdk.io/x/feegrant"
	_ "cosmossdk.io/x/feegrant/module" // import for side-effects
	"cosmossdk.io/x/nft"
	_ "cosmossdk.io/x/nft/module" // import for side-effects
	_ "cosmossdk.io/x/upgrade"    // import for side-effects
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	_ "github.com/cosmos/cosmos-sdk/x/auth/tx/config" // import for side-effects
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	_ "github.com/cosmos/cosmos-sdk/x/auth/vesting" // import for side-effects
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	_ "github.com/cosmos/cosmos-sdk/x/authz/module" // import for side-effects
	_ "github.com/cosmos/cosmos-sdk/x/bank"         // import for side-effects
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	_ "github.com/cosmos/cosmos-sdk/x/consensus" // import for side-effects
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	_ "github.com/cosmos/cosmos-sdk/x/distribution" // import for side-effects
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	_ "github.com/cosmos/cosmos-sdk/x/epochs" // import for side-effects
	epochstypes "github.com/cosmos/cosmos-sdk/x/epochs/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	_ "github.com/cosmos/cosmos-sdk/x/gov" // import for side-effects
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/group"
	_ "github.com/cosmos/cosmos-sdk/x/group/module" // import for side-effects
	_ "github.com/cosmos/cosmos-sdk/x/params"       // import for side-effects
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	_ "github.com/cosmos/cosmos-sdk/x/slashing" // import for side-effects
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	_ "github.com/cosmos/cosmos-sdk/x/staking" // import for side-effects
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	// Spliced rather than appended: the profile-varying entries keep the exact
	// positions they held when this was one literal, so the default build's
	// module-account set is unchanged.
	moduleAccPerms = slices.Concat([]*authmodulev1.ModuleAccountPermission{
		{Account: authtypes.FeeCollectorName},
		{Account: distrtypes.ModuleName},
		{Account: stakingtypes.BondedPoolName, Permissions: []string{authtypes.Burner, stakingtypes.ModuleName}},
		{Account: stakingtypes.NotBondedPoolName, Permissions: []string{authtypes.Burner, stakingtypes.ModuleName}},
		{Account: govtypes.ModuleName, Permissions: []string{authtypes.Burner}},
		{Account: nft.ModuleName},
	}, ibcModuleAccPerms, []*authmodulev1.ModuleAccountPermission{
		{Account: stablecoinmoduletypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner, authtypes.Staking}},
	}, ammModuleAccPerms, builderfeeModuleAccPerms, []*authmodulev1.ModuleAccountPermission{
		{Account: paymsgmoduletypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner, authtypes.Staking}},
	}, emissionModuleAccPerms, []*authmodulev1.ModuleAccountPermission{
		// The treasury holds user deposits in custody and must never be able to
		// create or destroy value, so it is granted no permissions at all.
		{Account: treasurymoduletypes.ModuleName},
	}, custodyModuleAccPerms, tokenisationModuleAccPerms)

	// blocked account addresses
	// blockAccAddrs are the module accounts that must never be the recipient of
	// an ordinary bank transfer.
	//
	// Each of these holds funds on behalf of state its module maintains
	// separately — pool reserves, a treasury's ledger, fees awaiting
	// distribution. Coins that arrive by a direct MsgSend land in the account
	// without any of that bookkeeping knowing, so they are unattributable and
	// unreachable forever: no module message will pay them out, because every
	// payout path checks the module's own records, and no bank message can move
	// them, because a module account has no key to sign with.
	//
	// Blocking them costs nothing. Deposits still work — MsgDeposit and every
	// other legitimate inbound path goes through a keeper call, which this list
	// does not affect. It only rejects the transfer a person makes by pasting a
	// module address into a wallet, which is a mistake with no recovery.
	blockAccAddrs = slices.Concat([]string{
		authtypes.FeeCollectorName,
		distrtypes.ModuleName,
		stakingtypes.BondedPoolName,
		stakingtypes.NotBondedPoolName,
		nft.ModuleName,
		// Chain modules that hold custody against their own accounting.
		treasurymoduletypes.ModuleName,
	}, custodyBlockedAccounts, tokenisationBlockedAccounts, ammBlockedAccounts, []string{
		stablecoinmoduletypes.ModuleName,
		paymsgmoduletypes.ModuleName,
	}, emissionBlockedAccounts, builderfeeBlockedAccounts)
	// We allow the following module accounts to receive funds:
	// govtypes.ModuleName

	// runtimeConfig is lifted out of the composed config so that the block
	// orderings it holds can be asserted on directly. They are the part of the
	// wiring a profile is most able to break silently: a module spliced into
	// the wrong position still compiles, still starts, and is simply a block
	// late on everything it does.
	runtimeConfig = &runtimev1alpha1.Module{
		AppName: Name,
		// NOTE: upgrade module is required to be prioritized
		PreBlockers: []string{
			upgradetypes.ModuleName,
			authtypes.ModuleName,
		},
		// During begin block slashing happens after distr.BeginBlocker so that
		// there is nothing left over in the validator fee pool, so as to keep the
		// CanWithdrawInvariant invariant.
		// NOTE: staking module is required if HistoricalEntries param > 0
		BeginBlockers: slices.Concat(
			// Spliced ahead of distribution rather than appended, because
			// emission has to mint into the fee collector before
			// distribution allocates it this block — the ordering
			// constraint x/mint carried. Appending it would leave the
			// chain a block behind on every payout.
			emissionBeginBlockers,
			[]string{
				distrtypes.ModuleName,
				slashingtypes.ModuleName,
				evidencetypes.ModuleName,
				stakingtypes.ModuleName,
				authz.ModuleName,
				epochstypes.ModuleName,
			},
			// ibc modules
			ibcBeginBlockers,
			// chain modules
			[]string{validatorgovmoduletypes.ModuleName, stablecoinmoduletypes.ModuleName},
			ammBeginBlockers,
			builderfeeBeginBlockers,
			[]string{paymsgmoduletypes.ModuleName, treasurymoduletypes.ModuleName, oraclemoduletypes.ModuleName},
		),
		EndBlockers: slices.Concat(
			[]string{
				govtypes.ModuleName,
				stakingtypes.ModuleName,
				feegrant.ModuleName,
				group.ModuleName,
			},
			// chain modules
			// oracle tallies after staking, so a validator that stopped being
			// bonded during this block no longer carries weight in the rate
			// its earlier vote asked for.
			[]string{validatorgovmoduletypes.ModuleName, stablecoinmoduletypes.ModuleName},
			ammEndBlockers,
			builderfeeEndBlockers,
			[]string{paymsgmoduletypes.ModuleName},
			emissionEndBlockers,
			[]string{treasurymoduletypes.ModuleName, oraclemoduletypes.ModuleName},
			// enforcement resolves cases after staking, for the same reason
			// as oracle: a validator that stopped being bonded in this block
			// should not still be carrying weight in a decision to freeze or
			// seize somebody's assets.
			[]string{enforcementmoduletypes.ModuleName},
		),
		// The following is mostly only needed when ModuleName != StoreKey name.
		OverrideStoreKeys: []*runtimev1alpha1.StoreKeyConfig{
			{
				ModuleName: authtypes.ModuleName,
				KvStoreKey: "acc",
			},
		},
		// NOTE: The genutils module must occur after staking so that pools are
		// properly initialized with tokens from genesis accounts.
		// NOTE: The genutils module must also occur after auth so that it can access the params from auth.
		InitGenesis: slices.Concat(
			[]string{
				consensustypes.ModuleName,
				authtypes.ModuleName,
				banktypes.ModuleName,
				distrtypes.ModuleName,
				stakingtypes.ModuleName,
				slashingtypes.ModuleName,
				govtypes.ModuleName,
				genutiltypes.ModuleName,
				evidencetypes.ModuleName,
				authz.ModuleName,
				feegrant.ModuleName,
				vestingtypes.ModuleName,
				nft.ModuleName,
				group.ModuleName,
				upgradetypes.ModuleName,
				circuittypes.ModuleName,
				epochstypes.ModuleName,
			},
			// ibc modules
			ibcInitGenesis,
			// chain modules
			[]string{validatorgovmoduletypes.ModuleName, stablecoinmoduletypes.ModuleName},
			ammInitGenesis,
			builderfeeInitGenesis,
			[]string{paymsgmoduletypes.ModuleName},
			emissionInitGenesis,
			[]string{treasurymoduletypes.ModuleName, oraclemoduletypes.ModuleName, enforcementmoduletypes.ModuleName, aliasmoduletypes.ModuleName},
			custodyInitGenesis,
			tokenisationInitGenesis,
			landInitGenesis,
		),
	}

	// application configuration (used by depinject)
	appConfig = appconfig.Compose(&appv1alpha1.Config{
		Modules: slices.Concat([]*appv1alpha1.ModuleConfig{
			{
				Name:   runtime.ModuleName,
				Config: appconfig.WrapAny(runtimeConfig),
			},
			{
				Name: authtypes.ModuleName,
				Config: appconfig.WrapAny(&authmodulev1.Module{
					Bech32Prefix:                AccountAddressPrefix,
					ModuleAccountPermissions:    moduleAccPerms,
					EnableUnorderedTransactions: true,
					// By default modules authority is the governance module. This is configurable with the following:
					// Authority: "group", // A custom module authority can be set using a module name
					// Authority: "cosmos1cwwv22j5ca08ggdv9c2uky355k908694z577tv", // or a specific address
				}),
			},
			{
				Name:   vestingtypes.ModuleName,
				Config: appconfig.WrapAny(&vestingmodulev1.Module{}),
			},
			{
				Name: banktypes.ModuleName,
				Config: appconfig.WrapAny(&bankmodulev1.Module{
					BlockedModuleAccountsOverride: blockAccAddrs,
				}),
			},
			{
				Name:   stakingtypes.ModuleName,
				Config: appconfig.WrapAny(&stakingmodulev1.Module{}),
			},
			{
				Name:   slashingtypes.ModuleName,
				Config: appconfig.WrapAny(&slashingmodulev1.Module{}),
			},
			{
				Name: "tx",
				Config: appconfig.WrapAny(&txconfigv1.Config{
					// The default ante/post handlers are replaced with custom ones
					// (app/ante.go, app/posthandler.go) that add the validatorgov
					// validator gate and the builderfee gas fee split.
					SkipAnteHandler: true,
					SkipPostHandler: true,
				}),
			},
			{
				Name:   genutiltypes.ModuleName,
				Config: appconfig.WrapAny(&genutilmodulev1.Module{}),
			},
			{
				Name:   authz.ModuleName,
				Config: appconfig.WrapAny(&authzmodulev1.Module{}),
			},
			{
				Name:   upgradetypes.ModuleName,
				Config: appconfig.WrapAny(&upgrademodulev1.Module{}),
			},
			{
				Name:   distrtypes.ModuleName,
				Config: appconfig.WrapAny(&distrmodulev1.Module{}),
			},
			{
				Name:   evidencetypes.ModuleName,
				Config: appconfig.WrapAny(&evidencemodulev1.Module{}),
			},
			{
				Name: group.ModuleName,
				Config: appconfig.WrapAny(&groupmodulev1.Module{
					MaxExecutionPeriod: durationpb.New(time.Second * 1209600),
					MaxMetadataLen:     255,
				}),
			},
			{
				Name:   nft.ModuleName,
				Config: appconfig.WrapAny(&nftmodulev1.Module{}),
			},
			{
				Name:   feegrant.ModuleName,
				Config: appconfig.WrapAny(&feegrantmodulev1.Module{}),
			},
			{
				Name:   govtypes.ModuleName,
				Config: appconfig.WrapAny(&govmodulev1.Module{}),
			},
			{
				Name:   consensustypes.ModuleName,
				Config: appconfig.WrapAny(&consensusmodulev1.Module{}),
			},
			{
				Name:   circuittypes.ModuleName,
				Config: appconfig.WrapAny(&circuitmodulev1.Module{}),
			},
			{
				Name:   paramstypes.ModuleName,
				Config: appconfig.WrapAny(&paramsmodulev1.Module{}),
			},
			{
				Name:   epochstypes.ModuleName,
				Config: appconfig.WrapAny(&epochsmodulev1.Module{}),
			},
		}, landModuleConfigs, []*appv1alpha1.ModuleConfig{
			{
				Name:   validatorgovmoduletypes.ModuleName,
				Config: appconfig.WrapAny(&validatorgovmoduletypes.Module{}),
			}, {
				Name:   stablecoinmoduletypes.ModuleName,
				Config: appconfig.WrapAny(&stablecoinmoduletypes.Module{}),
			}}, ammModuleConfigs, builderfeeModuleConfigs, []*appv1alpha1.ModuleConfig{{
			Name:   paymsgmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&paymsgmoduletypes.Module{}),
		}}, emissionModuleConfigs, []*appv1alpha1.ModuleConfig{{
			Name:   treasurymoduletypes.ModuleName,
			Config: appconfig.WrapAny(&treasurymoduletypes.Module{}),
		}, {
			Name:   oraclemoduletypes.ModuleName,
			Config: appconfig.WrapAny(&oraclemoduletypes.Module{}),
		}, {
			Name:   enforcementmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&enforcementmoduletypes.Module{}),
		}, {
			Name:   aliasmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&aliasmoduletypes.Module{}),
		}}, custodyModuleConfigs, tokenisationModuleConfigs),
	})
)
