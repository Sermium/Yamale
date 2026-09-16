package market

import (
	"encoding/json"
	"fmt"

	"cosmossdk.io/core/appmodule"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"

	"yamale/blockchain/x/market/keeper"
	"yamale/blockchain/x/market/types"
)

var (
	_ module.AppModuleBasic = (*AppModule)(nil)
	_ module.AppModule      = (*AppModule)(nil)
	_ module.HasGenesis     = (*AppModule)(nil)

	_ appmodule.AppModule = (*AppModule)(nil)
)

// AppModule is the secondary market for tokenised shares.
//
// # No BeginBlocker and no EndBlocker, deliberately
//
// The temptation is obvious and there are two of them.
//
// The first is settling auctions automatically at their deadline. That means
// walking every open auction every block, on a chain where the overwhelming
// majority of blocks contain no expiring auction at all — an unbounded loop in
// the one place a chain cannot afford one, paid for by every validator rather
// than by anybody who wanted it. Settlement is a message instead, and ANYBODY
// may send it once the deadline has passed. That last point is what makes the
// absence safe: the winner does not need the seller's cooperation to collect,
// and the seller does not need the winner's to be paid.
//
// The second is matching the book at the end of a block, which is how a
// batch-auction exchange removes the advantage of being first in a block. It is
// a real design and it is not this one: it would mean crossing every book with
// resting interest, every block, with no bound on how many books there are or
// how deep they go. This module crosses inside the transaction that caused it,
// bounded by a parameter, and pays for it with the gas of whoever placed the
// order.
//
// The cost of having neither is that an expired auction sits unsettled until
// somebody sends a message, and its state says so plainly. A chain that had
// already unwound it would have made that call on the parties' behalf with
// nobody present.
type AppModule struct {
	cdc    codec.Codec
	keeper keeper.Keeper
}

func NewAppModule(cdc codec.Codec, keeper keeper.Keeper) AppModule {
	return AppModule{cdc: cdc, keeper: keeper}
}

// IsAppModule implements the appmodule.AppModule interface.
func (AppModule) IsAppModule() {}

// Name returns the module name.
func (AppModule) Name() string { return types.ModuleName }

// RegisterLegacyAminoCodec registers the amino codec.
func (AppModule) RegisterLegacyAminoCodec(*codec.LegacyAmino) {}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes.
func (AppModule) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) {
	if err := types.RegisterQueryHandlerClient(
		clientCtx.CmdContext, mux, types.NewQueryClient(clientCtx),
	); err != nil {
		panic(err)
	}
}

// RegisterInterfaces registers the module's types.
func (AppModule) RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	types.RegisterInterfaces(registrar)
}

// RegisterServices registers the Msg and Query services.
func (am AppModule) RegisterServices(registrar grpc.ServiceRegistrar) error {
	types.RegisterMsgServer(registrar, keeper.NewMsgServerImpl(am.keeper))
	types.RegisterQueryServer(registrar, keeper.NewQueryServerImpl(am.keeper))
	return nil
}

// DefaultGenesis returns an empty market with the protective limits.
func (am AppModule) DefaultGenesis(codec.JSONCodec) json.RawMessage {
	return am.cdc.MustMarshalJSON(types.DefaultGenesis())
}

// ValidateGenesis refuses a market whose guarantees are already broken.
func (am AppModule) ValidateGenesis(
	_ codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage,
) error {
	var genState types.GenesisState
	if err := am.cdc.UnmarshalJSON(bz, &genState); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err)
	}
	return genState.Validate()
}

// InitGenesis restores the market.
func (am AppModule) InitGenesis(ctx sdk.Context, _ codec.JSONCodec, gs json.RawMessage) {
	var genState types.GenesisState
	if err := am.cdc.UnmarshalJSON(gs, &genState); err != nil {
		panic(fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err))
	}
	if err := am.keeper.InitGenesis(ctx, genState); err != nil {
		panic(fmt.Errorf("failed to initialize %s genesis state: %w", types.ModuleName, err))
	}
}

// ExportGenesis writes the market out.
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

// ConsensusVersion is the module's state-breaking version.
func (AppModule) ConsensusVersion() uint64 { return 1 }
