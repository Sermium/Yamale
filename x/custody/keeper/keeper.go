package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	storetypes "cosmossdk.io/core/store"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"

	"yamale/blockchain/x/custody/types"
)

// Keeper holds the custody registry.
//
// The collections and what each one exists to prevent:
//
//   - Assets        denom -> Asset            what may be issued at all
//   - Attestors     address                   who may say a deposit happened
//   - Deposits      id -> Deposit             the record, including refused ones
//   - Attestations  (id, attestor)            who said what, individually
//   - ExternalRefs  (denom, ref)              the replay guard
//   - Redemptions   id -> Redemption          burned claims awaiting payout
//   - Reserves      denom -> Reserve          what the custodian says it holds
//
// ExternalRefs is the one that matters most. Without it the same source-chain
// transaction can be attested again after crediting and mint a second claim
// against one deposit — which is not a bug that shows up in testing, because it
// needs somebody to try it.
type Keeper struct {
	cdc          codec.BinaryCodec
	addressCodec address.Codec
	authority    string
	logger       log.Logger
	bank         types.BankKeeper

	Schema       collections.Schema
	Params       collections.Item[types.Params]
	Assets       collections.Map[string, types.Asset]
	Attestors    collections.KeySet[string]
	Deposits     collections.Map[string, types.Deposit]
	Attestations collections.KeySet[collections.Pair[string, string]]
	ExternalRefs collections.KeySet[collections.Pair[string, string]]
	Redemptions  collections.Map[string, types.Redemption]
	// One attestor's statement of what is held, keyed (denom, attestor).
	// Reserves below is derived from these: writing the published figure
	// directly meant any single attestor could set it to anything.
	ReserveReports collections.Map[collections.Pair[string, string], types.ReserveReport]
	Reserves       collections.Map[string, types.Reserve]
	DepositSeq     collections.Sequence
	RedeemSeq      collections.Sequence
}

func NewKeeper(
	cdc codec.BinaryCodec,
	addressCodec address.Codec,
	storeService storetypes.KVStoreService,
	logger log.Logger,
	authority string,
	bank types.BankKeeper,
) Keeper {
	if _, err := addressCodec.StringToBytes(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)
	pair := collections.PairKeyCodec(collections.StringKey, collections.StringKey)

	k := Keeper{
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,
		logger:       logger.With("module", "x/"+types.ModuleName),
		bank:         bank,

		Params: collections.NewItem(sb, types.ParamsKey, "params",
			codec.CollValue[types.Params](cdc)),
		Assets: collections.NewMap(sb, types.AssetsKey, "assets",
			collections.StringKey, codec.CollValue[types.Asset](cdc)),
		Attestors: collections.NewKeySet(sb, types.AttestorsKey, "attestors",
			collections.StringKey),
		Deposits: collections.NewMap(sb, types.DepositsKey, "deposits",
			collections.StringKey, codec.CollValue[types.Deposit](cdc)),
		Attestations: collections.NewKeySet(sb, types.AttestationsKey, "attestations", pair),
		ExternalRefs: collections.NewKeySet(sb, types.ExternalRefsKey, "external_refs", pair),
		Redemptions: collections.NewMap(sb, types.RedemptionsKey, "redemptions",
			collections.StringKey, codec.CollValue[types.Redemption](cdc)),
		ReserveReports: collections.NewMap(sb, types.ReserveReportsKey, "reserve_reports",
			pair, codec.CollValue[types.ReserveReport](cdc)),
		Reserves: collections.NewMap(sb, types.ReservesKey, "reserves",
			collections.StringKey, codec.CollValue[types.Reserve](cdc)),
		DepositSeq: collections.NewSequence(sb, types.DepositSeqKey, "deposit_seq"),
		RedeemSeq:  collections.NewSequence(sb, types.RedemptionSeqKey, "redemption_seq"),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }
func (k Keeper) Logger() log.Logger   { return k.logger }

// fee takes the module's cut of an amount, rounding *up* so the rounding always
// favours the reserve rather than the claim. A fee that rounds down leaks a
// base unit per operation, and a custodian that leaks is eventually short.
func (k Keeper) fee(ctx context.Context, amount math.Int) (math.Int, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return math.Int{}, err
	}
	if params.FeeBps == 0 {
		return math.ZeroInt(), nil
	}
	bps := math.NewInt(int64(params.FeeBps))
	num := amount.Mul(bps)
	f := num.Quo(math.NewInt(10_000))
	if !num.Mod(math.NewInt(10_000)).IsZero() {
		f = f.Add(math.OneInt())
	}
	if f.GT(amount) {
		f = amount
	}
	return f, nil
}

// solvencyOf compares what the chain has issued with what was last attested as
// held. Issued is known for certain; held is taken on trust — which is exactly
// why the age is returned alongside it.
func (k Keeper) solvencyOf(ctx context.Context, denom string, height int64) (types.Solvency, error) {
	issued := k.bank.GetSupply(ctx, denom).Amount

	held := math.ZeroInt()
	age := int64(-1)
	if r, err := k.Reserves.Get(ctx, denom); err == nil {
		held = r.Held
		age = height - r.AsOfHeight
	}

	return types.Solvency{
		Denom:            denom,
		Issued:           issued,
		Held:             held,
		ReserveAgeBlocks: age,
		// Never attested is not solvent. Treating an absent reserve as zero and
		// zero issuance as "fine" would report a brand-new asset as healthy for
		// exactly as long as nobody deposits into it.
		Solvent: age >= 0 && held.GTE(issued),
	}, nil
}
