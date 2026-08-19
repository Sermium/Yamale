// Package integration provides a test environment that wires real x/auth and
// x/bank keepers over an in-memory multistore.
//
// The Ignite-scaffolded module fixtures construct their keeper with a nil
// BankKeeper, which is fine for testing params and query plumbing but makes it
// impossible to test the handlers that actually move money — every AMM swap,
// stablecoin mint, and payment instruction on this chain runs through x/bank.
// Env fills that gap: tests assert on real balances and real total supply, so
// accounting mistakes (double-spends, unbalanced mints, coins stranded in a
// module account) surface as test failures rather than as mainnet incidents.
package integration

import (
	"testing"

	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/codec"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// FunderModuleName is a module account that exists only inside tests. It holds
// the Minter permission purely so Fund can conjure the starting balances a test
// needs, keeping that minting out of the module under test's own accounting.
const FunderModuleName = "testfunder"

// GovModuleName mirrors each module's types.GovModuleName, duplicated here for
// the same reason: to avoid a dependency on x/gov.
const GovModuleName = "gov"

// Env is a module-under-test's world: a context backed by an in-memory
// multistore, plus the auth and bank keepers its handlers will call.
type Env struct {
	Ctx          sdk.Context
	Codec        codec.Codec
	AddressCodec address.Codec
	AuthKeeper   authkeeper.AccountKeeper
	BankKeeper   bankkeeper.BaseKeeper

	// Authority is the gov module account, the only signer the modules accept
	// on their Approve* and UpdateParams messages.
	Authority sdk.AccAddress

	// StoreService is the KV store service for the module under test, to be
	// handed to its NewKeeper.
	StoreService corestore.KVStoreService

	// stores holds every mounted module store, including the one above. Stores
	// are mounted when the multistore is built and cannot be added later, which
	// is why NewWith takes the whole list up front.
	stores map[string]corestore.KVStoreService

	// blocked is the live map the bank keeper consults, so Block can add to it
	// after construction.
	blocked map[string]bool
}

// Store returns the KV store service mounted for a module named in NewWith.
//
// It panics on an unmounted name rather than returning nil: a keeper handed a
// nil store fails much later and much less legibly, usually as a panic inside
// the handler under test.
func (e *Env) Store(moduleName string) corestore.KVStoreService {
	s, ok := e.stores[moduleName]
	if !ok {
		panic("no store mounted for module " + moduleName +
			"; name it in integration.NewWith")
	}
	return s
}

// Block marks an address as unable to receive funds, mirroring the chain's
// blocked module accounts. Tests need this to exercise the paths that must
// refuse to send somewhere unrecoverable.
func (e *Env) Block(addr sdk.AccAddress) {
	e.blocked[addr.String()] = true
}

// New builds an Env for the module named moduleName. Pass the module's own
// AppModuleBasic (and any other modules whose types appear in the test) so
// their protobuf types are registered on the codec.
//
// The module gets a module account with Minter, Burner and Staking permissions,
// matching what app_config.go grants every chain module in production.
func New(t *testing.T, moduleName string, mods ...module.AppModuleBasic) *Env {
	t.Helper()
	return NewWith(t, []string{moduleName}, mods...)
}

// NewWith builds an Env that mounts a store for every name in moduleNames, the
// first of which is the module under test and the one Env.StoreService points
// at.
//
// The extra stores exist for a rule one module enforces against another
// module's state: x/tokenisation refuses to fractionalise a parcel x/land has
// not authorised, and testing that against a hand-written stub of the registry
// would only prove the module can read a struct the test wrote itself. The
// rules being enforced are the other keeper's, so the other keeper is real.
func NewWith(t *testing.T, moduleNames []string, mods ...module.AppModuleBasic) *Env {
	t.Helper()
	require.NotEmpty(t, moduleNames, "at least one module must be named")

	basics := append([]module.AppModuleBasic{auth.AppModuleBasic{}, bank.AppModuleBasic{}}, mods...)
	encCfg := moduletestutil.MakeTestEncodingConfig(basics...)

	bech32Prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	addrCdc := addresscodec.NewBech32Codec(bech32Prefix)

	keys := map[string]*storetypes.KVStoreKey{
		authtypes.StoreKey: storetypes.NewKVStoreKey(authtypes.StoreKey),
		banktypes.StoreKey: storetypes.NewKVStoreKey(banktypes.StoreKey),
	}
	for _, name := range moduleNames {
		keys[name] = storetypes.NewKVStoreKey(name)
	}
	ctx := testutil.DefaultContextWithKeys(keys, nil, nil)

	authority := authtypes.NewModuleAddress(GovModuleName)
	authorityStr, err := addrCdc.BytesToString(authority)
	require.NoError(t, err)

	maccPerms := map[string][]string{
		authtypes.FeeCollectorName: nil,
		FunderModuleName:           {authtypes.Minter},
	}
	for _, name := range moduleNames {
		maccPerms[name] = []string{authtypes.Minter, authtypes.Burner, authtypes.Staking}
	}

	ak := authkeeper.NewAccountKeeper(
		encCfg.Codec,
		runtime.NewKVStoreService(keys[authtypes.StoreKey]),
		authtypes.ProtoBaseAccount,
		maccPerms,
		addrCdc,
		bech32Prefix,
		authorityStr,
	)
	require.NoError(t, ak.Params.Set(ctx, authtypes.DefaultParams()))

	blocked := map[string]bool{}
	bk := bankkeeper.NewBaseKeeper(
		encCfg.Codec,
		runtime.NewKVStoreService(keys[banktypes.StoreKey]),
		ak,
		blocked,
		authorityStr,
		log.NewNopLogger(),
	)
	require.NoError(t, bk.SetParams(ctx, banktypes.DefaultParams()))

	stores := make(map[string]corestore.KVStoreService, len(moduleNames))
	for _, name := range moduleNames {
		stores[name] = runtime.NewKVStoreService(keys[name])
	}

	return &Env{
		Ctx:          ctx,
		Codec:        encCfg.Codec,
		AddressCodec: addrCdc,
		AuthKeeper:   ak,
		BankKeeper:   bk,
		Authority:    authority,
		StoreService: stores[moduleNames[0]],
		stores:       stores,
		blocked:      blocked,
	}
}

// AuthorityString returns the gov module account's bech32 address.
func (e *Env) AuthorityString(t *testing.T) string {
	t.Helper()
	s, err := e.AddressCodec.BytesToString(e.Authority)
	require.NoError(t, err)
	return s
}

// Addr returns a fresh account address and its bech32 encoding.
func (e *Env) Addr(t *testing.T) (sdk.AccAddress, string) {
	t.Helper()
	var pub cryptotypes.PubKey = secp256k1.GenPrivKey().PubKey()
	addr := sdk.AccAddress(pub.Address())
	s, err := e.AddressCodec.BytesToString(addr)
	require.NoError(t, err)
	return addr, s
}

// Fund credits addr with coins, minted out of the test funder module.
func (e *Env) Fund(t *testing.T, addr sdk.AccAddress, coins sdk.Coins) {
	t.Helper()
	require.NoError(t, e.BankKeeper.MintCoins(e.Ctx, FunderModuleName, coins))
	require.NoError(t, e.BankKeeper.SendCoinsFromModuleToAccount(e.Ctx, FunderModuleName, addr, coins))
}

// FundModule credits a module account with coins, minted out of the test
// funder module. Used to stage balances the module under test expects to
// already exist, such as fees the ante handler would have collected.
func (e *Env) FundModule(t *testing.T, recipientModule string, coins sdk.Coins) {
	t.Helper()
	require.NoError(t, e.BankKeeper.MintCoins(e.Ctx, FunderModuleName, coins))
	require.NoError(t, e.BankKeeper.SendCoinsFromModuleToModule(e.Ctx, FunderModuleName, recipientModule, coins))
}

// NewFundedAddr returns a fresh address already holding coins.
func (e *Env) NewFundedAddr(t *testing.T, coins sdk.Coins) (sdk.AccAddress, string) {
	t.Helper()
	addr, s := e.Addr(t)
	e.Fund(t, addr, coins)
	return addr, s
}

// Balance returns addr's balance of denom.
func (e *Env) Balance(addr sdk.AccAddress, denom string) math.Int {
	return e.BankKeeper.GetBalance(e.Ctx, addr, denom).Amount
}

// ModuleBalance returns a module account's balance of denom.
func (e *Env) ModuleBalance(moduleName, denom string) math.Int {
	return e.BankKeeper.GetBalance(e.Ctx, e.AuthKeeper.GetModuleAddress(moduleName), denom).Amount
}

// Supply returns the total supply of denom.
func (e *Env) Supply(denom string) math.Int {
	return e.BankKeeper.GetSupply(e.Ctx, denom).Amount
}

// WithHeight returns a copy of the environment's context at the given block
// height, for handlers whose behavior depends on it.
func (e *Env) WithHeight(height int64) sdk.Context {
	return e.Ctx.WithBlockHeight(height)
}
