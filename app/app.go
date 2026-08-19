package app

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"

	clienthelpers "cosmossdk.io/client/v2/helpers"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	circuitkeeper "cosmossdk.io/x/circuit/keeper"
	feegrantkeeper "cosmossdk.io/x/feegrant/keeper"
	upgradekeeper "cosmossdk.io/x/upgrade/keeper"

	abci "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/mempool"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	consensuskeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"

	"yamale/blockchain/docs"
	aliasmodulekeeper "yamale/blockchain/x/alias/keeper"
	enforcementmodulekeeper "yamale/blockchain/x/enforcement/keeper"
	oraclemodulekeeper "yamale/blockchain/x/oracle/keeper"
	paymsgmodulekeeper "yamale/blockchain/x/paymsg/keeper"
	stablecoinmodulekeeper "yamale/blockchain/x/stablecoin/keeper"
	treasurymodulekeeper "yamale/blockchain/x/treasury/keeper"
	validatorgovmodulekeeper "yamale/blockchain/x/validatorgov/keeper"
)

const (
	// Name is the name of the application.
	Name = "blockchain"
	// AccountAddressPrefix is the prefix for accounts addresses.
	AccountAddressPrefix = "yml"
	// ChainCoinType is the coin type of the chain.
	ChainCoinType = 118
)

// DefaultNodeHome default home directories for the application daemon
var DefaultNodeHome string

var (
	_ runtime.AppI            = (*App)(nil)
	_ servertypes.Application = (*App)(nil)
)

// App extends an ABCI application, but with most of its parameters exported.
// They are exported for convenience in creating helper functions, as object
// capabilities aren't needed for testing.
type App struct {
	*runtime.App
	legacyAmino       *codec.LegacyAmino
	appCodec          codec.Codec
	txConfig          client.TxConfig
	interfaceRegistry codectypes.InterfaceRegistry

	// keepers
	// only keepers required by the app are exposed
	// the list of all modules is available in the app_config
	AuthKeeper            authkeeper.AccountKeeper
	BankKeeper            bankkeeper.Keeper
	StakingKeeper         *stakingkeeper.Keeper
	SlashingKeeper        slashingkeeper.Keeper
	DistrKeeper           distrkeeper.Keeper
	GovKeeper             *govkeeper.Keeper
	UpgradeKeeper         *upgradekeeper.Keeper
	AuthzKeeper           authzkeeper.Keeper
	ConsensusParamsKeeper consensuskeeper.Keeper
	CircuitBreakerKeeper  circuitkeeper.Keeper
	ParamsKeeper          paramskeeper.Keeper
	FeeGrantKeeper        feegrantkeeper.Keeper

	// The keepers of every profile-varying module are embedded from build-tagged
	// files rather than declared here, because a Go field cannot be removed by a
	// tag — and a field left in place would hold the zero keeper, which is
	// usable, silently wrong, and indistinguishable at the call site from a
	// working one. See profile_ibc_off.go for the tag scheme and why IBC is the
	// one tag that is opt-in.
	ibcKeepers
	emissionKeepers
	ammKeepers
	builderfeeKeepers
	custodyKeepers

	// simulation manager
	sm                 *module.SimulationManager
	ValidatorgovKeeper validatorgovmodulekeeper.Keeper
	StablecoinKeeper   stablecoinmodulekeeper.Keeper
	PaymsgKeeper       paymsgmodulekeeper.Keeper
	TreasuryKeeper     treasurymodulekeeper.Keeper
	OracleKeeper       oraclemodulekeeper.Keeper
	EnforcementKeeper  enforcementmodulekeeper.Keeper
	AliasKeeper        aliasmodulekeeper.Keeper
}

func init() {
	var err error
	clienthelpers.EnvPrefix = Name
	DefaultNodeHome, err = clienthelpers.GetNodeHomeDirectory("." + Name)
	if err != nil {
		panic(err)
	}
}

// AppConfig returns the default app config.
func AppConfig() depinject.Config {
	return depinject.Configs(
		appConfig,
		depinject.Supply(
			// supply custom module basics
			map[string]module.AppModuleBasic{
				genutiltypes.ModuleName: genutil.NewAppModuleBasic(genutiltypes.DefaultMessageValidator),
			},
		),
	)
}

// New returns a reference to an initialized App.
func New(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *App {
	var (
		app        = &App{}
		appBuilder *runtime.AppBuilder

		// Supplies the profile contributes on top of the ones every build needs.
		// The IBC keeper getter is one of them: it cannot be a plain nil in the
		// no-IBC build, because supplying it at all would put an ibc-go type into
		// the container and undo the exclusion.
		//
		// here alternative options can be supplied to the DI container.
		// those options can be used f.e to override the default behavior of some modules.
		// for instance supplying a custom address codec for not using bech32 addresses.
		// read the depinject documentation and depinject module wiring for more information
		// on available options and how to use them.
		supplies = append([]any{
			appOpts, // supply app options
			logger,  // supply logger
		}, app.ibcDepinjectSupplies()...)

		// merge the AppConfig and other configuration in one config
		appConfig = depinject.Configs(
			AppConfig(),
			depinject.Supply(supplies...),
		)
	)

	var appModules map[string]appmodule.AppModule

	// Appended rather than listed inline for the same reason as the config
	// splices: a keeper that is not in this build has no address to resolve
	// into, and depinject fails on a target whose type no module provides.
	outputs := append([]any{
		&appBuilder,
		&appModules,
		&app.appCodec,
		&app.legacyAmino,
		&app.txConfig,
		&app.interfaceRegistry,
		&app.AuthKeeper,
		&app.BankKeeper,
		&app.StakingKeeper,
		&app.SlashingKeeper,
		&app.DistrKeeper,
		&app.GovKeeper,
		&app.UpgradeKeeper,
		&app.AuthzKeeper,
		&app.ConsensusParamsKeeper,
		&app.CircuitBreakerKeeper,
		&app.ParamsKeeper,
		&app.FeeGrantKeeper,
		&app.ValidatorgovKeeper,
		&app.StablecoinKeeper,
		&app.PaymsgKeeper,
		&app.TreasuryKeeper,
		&app.OracleKeeper,
		&app.EnforcementKeeper,
		&app.AliasKeeper,
	}, slices.Concat(
		app.emissionDepinjectOutputs(),
		app.ammDepinjectOutputs(),
		app.builderfeeDepinjectOutputs(),
		app.custodyDepinjectOutputs(),
	)...)

	if err := depinject.Inject(appConfig, outputs...); err != nil {
		panic(err)
	}

	// The freeze is enforced on the bank keeper rather than in the ante chain.
	// An ante decorator only ever sees messages that arrived as transactions,
	// so a freeze enforced there would still be walked around by authz, by
	// interchain accounts, by a treasury spend or by a swap — all of which end
	// in a bank transfer. This is the one place every one of them passes
	// through.
	app.BankKeeper.AppendSendRestriction(app.EnforcementKeeper.SendRestriction)

	// add to default baseapp options
	// enable optimistic execution
	baseAppOptions = append(baseAppOptions, baseapp.SetOptimisticExecution())

	// Blocks are filled first-come, first-served.
	//
	// Stated rather than inherited. NoOpMempool is already baseapp's default, so
	// this line changes no behaviour today — it exists because the alternative is
	// one call away and would change the behaviour silently. With a no-op
	// mempool the application keeps no transaction pool of its own and
	// PrepareProposal takes CometBFT's transactions in the order they arrived,
	// which its flood mempool keeps in FIFO. Swapping in PriorityNonceMempool
	// would instead order a block by fee, and nothing in the diff would say so.
	//
	// That default matters more here than on a general-purpose chain. A fee
	// market decides whose payment settles first by who paid most to be there,
	// and this chain is being offered to states as payments infrastructure where
	// a citizen's transfer must not queue behind a trader's. Fees exist to price
	// spam, not priority.
	baseAppOptions = append(baseAppOptions, baseapp.SetMempool(mempool.NoOpMempool{}))

	// build app
	app.App = appBuilder.Build(db, traceStore, baseAppOptions...)

	anteHandler, err := newAnteHandler(app)
	if err != nil {
		panic(err)
	}
	app.SetAnteHandler(anteHandler)

	postHandler, err := newPostHandler(app)
	if err != nil {
		panic(err)
	}
	app.SetPostHandler(postHandler)

	// register legacy modules
	if err := app.registerIBCModules(appOpts); err != nil {
		panic(err)
	}

	/****  Module Options ****/

	// create the simulation manager and define the order of the modules for deterministic simulations
	overrideModules := map[string]module.AppModuleSimulation{
		authtypes.ModuleName: auth.NewAppModule(app.appCodec, app.AuthKeeper, authsims.RandomGenesisAccounts, nil),
	}
	app.sm = module.NewSimulationManagerFromAppModules(app.ModuleManager.Modules, overrideModules)

	app.sm.RegisterStoreDecoders()

	// A custom InitChainer sets if extra pre-init-genesis logic is required.
	// This is necessary for manually registered modules that do not support app wiring.
	// Manually set the module version map as shown below.
	// The upgrade module will automatically handle de-duplication of the module version map.
	app.SetInitChainer(func(ctx sdk.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
		// Refused before any state is written, and before the version map is
		// set: a genesis naming a module this profile does not link is the
		// wrong binary for this network, and the only safe thing a node can do
		// with it is decline to start. See genesis_sections.go.
		var appState map[string]json.RawMessage
		if err := json.Unmarshal(req.AppStateBytes, &appState); err != nil {
			return nil, fmt.Errorf("genesis app_state is not a JSON object: %w", err)
		}
		if err := CheckGenesisSections(appState, app.ModuleManager.ModuleNames()); err != nil {
			return nil, err
		}

		if err := app.UpgradeKeeper.SetModuleVersionMap(ctx, app.ModuleManager.GetVersionMap()); err != nil {
			return nil, err
		}
		return app.App.InitChainer(ctx, req)
	})

	// Before Load: runtime.App.Load installs its own PreBlocker only if none is
	// set, and by the time Load returns the app is sealed against further
	// handler changes. See profile_settlementfee_on.go.
	app.installProfileFeeRouting()

	// Before Load: the store loader an upgrade installs has to be in place
	// before the stores are opened, or the store it adds is missing at exactly
	// the height the upgrade takes effect.
	app.setupUpgradeHandlers()

	if err := app.Load(loadLatest); err != nil {
		panic(err)
	}

	return app
}

// GetSubspace returns a param subspace for a given module name.
func (app *App) GetSubspace(moduleName string) paramstypes.Subspace {
	subspace, _ := app.ParamsKeeper.GetSubspace(moduleName)
	return subspace
}

// LegacyAmino returns App's amino codec.
func (app *App) LegacyAmino() *codec.LegacyAmino {
	return app.legacyAmino
}

// AppCodec returns App's app codec.
func (app *App) AppCodec() codec.Codec {
	return app.appCodec
}

// InterfaceRegistry returns App's InterfaceRegistry.
func (app *App) InterfaceRegistry() codectypes.InterfaceRegistry {
	return app.interfaceRegistry
}

// TxConfig returns App's TxConfig
func (app *App) TxConfig() client.TxConfig {
	return app.txConfig
}

// GetKey returns the KVStoreKey for the provided store key.
func (app *App) GetKey(storeKey string) *storetypes.KVStoreKey {
	kvStoreKey, ok := app.UnsafeFindStoreKey(storeKey).(*storetypes.KVStoreKey)
	if !ok {
		return nil
	}
	return kvStoreKey
}

// SimulationManager implements the SimulationApp interface
func (app *App) SimulationManager() *module.SimulationManager {
	return app.sm
}

// RegisterAPIRoutes registers all application module routes with the provided
// API server.
func (app *App) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	app.App.RegisterAPIRoutes(apiSvr, apiConfig)
	// register swagger API in app.go so that other applications can override easily
	if err := server.RegisterSwaggerAPI(apiSvr.ClientCtx, apiSvr.Router, apiConfig.Swagger); err != nil {
		panic(err)
	}

	// register app's OpenAPI routes.
	docs.RegisterOpenAPIService(Name, apiSvr.Router, app.ModuleManager.ModuleNames())
}

// GetMaccPerms returns a copy of the module account permissions
//
// NOTE: This is solely to be used for testing purposes.
func GetMaccPerms() map[string][]string {
	dup := make(map[string][]string)
	for _, perms := range moduleAccPerms {
		dup[perms.GetAccount()] = perms.GetPermissions()
	}

	return dup
}

// BlockedAddresses returns all the app's blocked account addresses, keyed by
// bech32 address.
//
// blockAccAddrs and GetMaccPerms() are both keyed by module *name*, which is
// what x/bank's BlockedModuleAccountsOverride expects. Callers of this function
// — the simulation, and anything matching against a transaction's recipient —
// compare against real addresses, so the names have to be converted. Returning
// them unconverted produces a map that silently matches nothing, which looks
// exactly like a correctly configured chain with nothing blocked.
func BlockedAddresses() map[string]bool {
	result := make(map[string]bool)

	names := blockAccAddrs
	if len(names) == 0 {
		for name := range GetMaccPerms() {
			names = append(names, name)
		}
	}

	for _, name := range names {
		result[authtypes.NewModuleAddress(name).String()] = true
	}

	return result
}
