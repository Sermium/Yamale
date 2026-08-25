package enforcement

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmossdk.io/core/appmodule"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"yamale/blockchain/x/enforcement/keeper"
	"yamale/blockchain/x/enforcement/types"
)

var (
	_ module.AppModuleBasic = (*AppModule)(nil)
	_ module.AppModule      = (*AppModule)(nil)
	_ module.HasGenesis     = (*AppModule)(nil)
	// module.HasServices, not appmodule.HasServices. See RegisterServices: the
	// manager calls both when both are present, and only the Configurator form
	// can register a migration.
	_ module.HasServices = (*AppModule)(nil)

	_ appmodule.AppModule     = (*AppModule)(nil)
	_ appmodule.HasEndBlocker = (*AppModule)(nil)
)

// AppModule implements the AppModule interface that defines the inter-dependent methods that modules need to implement
type AppModule struct {
	cdc        codec.Codec
	keeper     keeper.Keeper
	authKeeper types.AuthKeeper
	bankKeeper types.BankKeeper
}

func NewAppModule(
	cdc codec.Codec,
	keeper keeper.Keeper,
	authKeeper types.AuthKeeper,
	bankKeeper types.BankKeeper,
) AppModule {
	return AppModule{
		cdc:        cdc,
		keeper:     keeper,
		authKeeper: authKeeper,
		bankKeeper: bankKeeper,
	}
}

// IsAppModule implements the appmodule.AppModule interface.
func (AppModule) IsAppModule() {}

// Name returns the name of the module as a string.
func (AppModule) Name() string {
	return types.ModuleName
}

// RegisterLegacyAminoCodec registers the amino codec
func (AppModule) RegisterLegacyAminoCodec(*codec.LegacyAmino) {}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the module.
func (AppModule) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) {
	if err := types.RegisterQueryHandlerClient(clientCtx.CmdContext, mux, types.NewQueryClient(clientCtx)); err != nil {
		panic(err)
	}
}

// RegisterInterfaces registers a module's interface types and their concrete implementations as proto.Message.
func (AppModule) RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	types.RegisterInterfaces(registrar)
}

// RegisterServices registers the module's services and its state migrations.
//
// It takes module.Configurator rather than the bare grpc.ServiceRegistrar it used
// to, and the difference is the only place a migration can be registered: the
// registrar can be handed a service and nothing else, and this module now has a
// migration. Only one of the two interfaces may be implemented — the manager
// calls both when both are present, which registers every service twice and
// panics on the duplicate.
func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), keeper.NewMsgServerImpl(am.keeper))
	types.RegisterQueryServer(cfg.QueryServer(), keeper.NewQueryServerImpl(am.keeper))

	// Panicking is right here, and for a sharper reason than in most modules. A
	// migration that failed to register would leave every node running the new
	// binary against v1 state, where the parameters still carry an
	// emergency_authority that no handler reads — so the chain would come up,
	// answer every query, and quietly disagree with its own stored parameters
	// about who may freeze an account.
	m := keeper.NewMigrator(am.keeper)
	if err := cfg.RegisterMigration(types.ModuleName, 1, func(ctx sdk.Context) error {
		return m.Migrate1to2(ctx)
	}); err != nil {
		panic(fmt.Sprintf("failed to register %s migration from 1 to 2: %v", types.ModuleName, err))
	}
}

// DefaultGenesis returns a default GenesisState for the module, marshalled to json.RawMessage.
func (am AppModule) DefaultGenesis(codec.JSONCodec) json.RawMessage {
	return am.cdc.MustMarshalJSON(types.DefaultGenesis())
}

// ValidateGenesis used to validate the GenesisState, given in its json.RawMessage form.
func (am AppModule) ValidateGenesis(_ codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var genState types.GenesisState
	if err := am.cdc.UnmarshalJSON(bz, &genState); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err)
	}

	return genState.Validate()
}

// InitGenesis performs the module's genesis initialization. It returns no validator updates.
func (am AppModule) InitGenesis(ctx sdk.Context, _ codec.JSONCodec, gs json.RawMessage) {
	var genState types.GenesisState
	if err := am.cdc.UnmarshalJSON(gs, &genState); err != nil {
		panic(fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err))
	}

	if err := am.keeper.InitGenesis(ctx, genState); err != nil {
		panic(fmt.Errorf("failed to initialize %s genesis state: %w", types.ModuleName, err))
	}
}

// ExportGenesis returns the module's exported genesis state as raw JSON bytes.
func (am AppModule) ExportGenesis(ctx sdk.Context, _ codec.JSONCodec) json.RawMessage {
	genState, err := am.keeper.ExportGenesis(ctx)
	if err != nil {
		panic(fmt.Errorf("failed to export %s genesis state: %w", types.ModuleName, err))
	}

	bz, err := am.cdc.MarshalJSON(genState)
	if err != nil {
		panic(fmt.Errorf("failed to marshal %s genesis state: %w", types.ModuleName, err))
	}

	return bz
}

// ConsensusVersion is a sequence number for state-breaking change of the module.
//
// 2 retired the emergency_authority parameter. The emergency path is authorised
// by ROLE_ENFORCEMENT_AUTHORITY covering the target's country now, and the
// migration rewrites the stored parameters without the reserved field. The
// address that held it is carried into a grant by the upgrade handler rather
// than by this module — see keeper.Migrator.Migrate1to2 for why the two halves
// are split.
func (AppModule) ConsensusVersion() uint64 { return 2 }

// EndBlock resolves the cases whose voting period ended at this height and
// lifts the provisional freezes that have run out.
//
// In EndBlock rather than BeginBlock so that a vote cast in this block counts
// in this block. Tallying at the start of a block would hold an account frozen
// for one more block than the validators asked for, every time.
func (am AppModule) EndBlock(ctx context.Context) error {
	return am.keeper.EndBlocker(ctx)
}
