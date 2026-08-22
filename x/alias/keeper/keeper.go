package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	storetypes "cosmossdk.io/core/store"
	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/alias/types"
)

// Keeper holds the identifier registry and the jurisdiction registry.
//
// Five collections, and the relationship between them is the module:
//
//   - Aliases        id                 -> Alias         the resolution anyone queries
//   - Owners         address            -> id            the reverse, derived, never exported
//   - Retired        id                                  tombstones, kept forever
//   - Jurisdictions  address            -> Jurisdiction  the perimeter, recorded
//   - Perimeter      (country, address)                  the reverse, derived, never exported
//   - ViewingKeys    (address, version) -> ViewingKey    every version ever published
//   - Regulators     country            -> Appointment   who holds the third key
//   - AuditorGrants  address            -> Grant         the time-boxed cross-account role
//   - RoleGrants     (holder, role, jurisdiction) -> RoleGrant   who may act, where
//   - GrantsByScope  (jurisdiction, role, holder)              the reverse, derived
//
// The role grants are here, beside the jurisdictions, because "who is where"
// and "who may act where" are one question asked from two ends. Split across
// two modules, every authority action becomes a cross-module read on the path
// that refuses it, and the two registries can disagree about what a country is;
// here the check is a lookup in the same store as the record it is checked
// against.
//
// The confidentiality registries are here rather than in x/paymsg because the sender of a
// payment has to resolve all of them at the moment it seals the payload — its
// own key, the payee's, the regulator of the declared settlement jurisdiction,
// and every live auditor. A registry that only covered accounts which happen to
// be somebody's payment customer would leave the regulator and the auditor
// unresolvable, which are two of the three parties the design exists to serve.
//
// Owners, Perimeter and GrantsByScope are rebuilt from their sources by
// InitGenesis rather than carried in genesis. A derived index emitted alongside
// its source is a second copy that can disagree with it, and an export that does
// not round-trip byte-for-byte breaks upgrades.
//
// The two registries are here together because an identifier's country prefix
// is only true if it is checked against a jurisdiction at the moment it is
// issued. Split across two modules that is a cross-module read on the path that
// mints identity; here it is a lookup that cannot be skipped.
type Keeper struct {
	cdc          codec.BinaryCodec
	addressCodec address.Codec
	authority    string
	logger       log.Logger

	// participants answers "who onboarded this account, and are they still
	// admitted". Read-only; see types.ParticipantKeeper.
	participants types.ParticipantKeeper

	// groups answers "is this address a group policy", asked once, when a role is
	// granted. Read-only; see types.GroupKeeper.
	groups types.GroupKeeper

	Schema        collections.Schema
	Params        collections.Item[types.Params]
	Aliases       collections.Map[string, types.Alias]
	Owners        collections.Map[string, string]
	Retired       collections.KeySet[string]
	Jurisdictions collections.Map[string, types.Jurisdiction]
	Perimeter     collections.KeySet[collections.Pair[string, string]]
	ViewingKeys   collections.Map[collections.Pair[string, uint64], types.ViewingKey]
	Regulators    collections.Map[string, types.RegulatorAppointment]
	AuditorGrants collections.Map[string, types.AuditorGrant]

	// RoleGrants is keyed (holder, role, jurisdiction). The role is stored as the
	// enum's int32 so the key sorts by role within a holder, which is what makes
	// "everything this account may do" one prefix scan.
	RoleGrants collections.Map[collections.Triple[string, int32, string], types.RoleGrant]
	// GrantsByScope is keyed (jurisdiction, role, holder). Derived, never
	// exported.
	GrantsByScope collections.KeySet[collections.Triple[string, int32, string]]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	addressCodec address.Codec,
	storeService storetypes.KVStoreService,
	logger log.Logger,
	authority string,
	participants types.ParticipantKeeper,
	groups types.GroupKeeper,
) Keeper {
	if _, err := addressCodec.StringToBytes(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,
		logger:       logger.With("module", "x/"+types.ModuleName),
		participants: participants,
		groups:       groups,

		Params: collections.NewItem(sb, types.ParamsKey, "params",
			codec.CollValue[types.Params](cdc)),
		Aliases: collections.NewMap(sb, types.AliasesKey, "aliases",
			collections.StringKey, codec.CollValue[types.Alias](cdc)),
		Owners: collections.NewMap(sb, types.OwnersKey, "owners",
			collections.StringKey, collections.StringValue),
		Retired: collections.NewKeySet(sb, types.RetiredKey, "retired",
			collections.StringKey),
		Jurisdictions: collections.NewMap(sb, types.JurisdictionsKey, "jurisdictions",
			collections.StringKey, codec.CollValue[types.Jurisdiction](cdc)),
		Perimeter: collections.NewKeySet(sb, types.PerimeterKey, "perimeter",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),
		ViewingKeys: collections.NewMap(sb, types.ViewingKeysKey, "viewingKeys",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key),
			codec.CollValue[types.ViewingKey](cdc)),
		Regulators: collections.NewMap(sb, types.RegulatorsKey, "regulators",
			collections.StringKey, codec.CollValue[types.RegulatorAppointment](cdc)),
		AuditorGrants: collections.NewMap(sb, types.AuditorGrantsKey, "auditorGrants",
			collections.StringKey, codec.CollValue[types.AuditorGrant](cdc)),
		RoleGrants: collections.NewMap(sb, types.RoleGrantsKey, "roleGrants",
			collections.TripleKeyCodec(collections.StringKey, collections.Int32Key, collections.StringKey),
			codec.CollValue[types.RoleGrant](cdc)),
		GrantsByScope: collections.NewKeySet(sb, types.GrantsByScopeKey, "grantsByScope",
			collections.TripleKeyCodec(collections.StringKey, collections.Int32Key, collections.StringKey)),
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

// CountryOf returns the country an account's identifier must carry.
//
// This is the rule the whole perimeter rests on, in one place so it cannot be
// implemented differently in two:
//
//  1. a recorded jurisdiction wins, always;
//  2. failing that, a named foundation administrator gets the reserved code,
//     because it belongs to no national perimeter and there is no country that
//     would be true of it;
//  3. failing that, refusal. Not a default, not a placeholder — an account
//     nobody has placed gets no identifier at all, so there is no account on
//     the rail whose perimeter is unknown.
//
// The exemption is deliberately last. A foundation administrator that has been
// recorded in a country uses that country: the exemption covers the *absence*
// of a jurisdiction and nothing else, which is as narrow as it can be made.
func (k Keeper) CountryOf(ctx context.Context, addr string) (string, error) {
	j, err := k.Jurisdictions.Get(ctx, addr)
	switch {
	case err == nil:
		return j.Country, nil
	case !errors.Is(err, collections.ErrNotFound):
		return "", err
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return "", err
	}
	if params.IsFoundationAdministrator(addr) {
		return types.FoundationCountry, nil
	}
	return "", types.ErrNoJurisdiction
}

// issue derives an unused identifier for an address in a country.
//
// Deterministic, so every validator computes the same answer — a
// nondeterministic derivation would not merely misbehave, it would halt the
// chain. The nonce climbs past anything already taken or tombstoned; at 32^8 a
// collision is not expected, and the loop must still terminate when one
// happens, which is what the bound is for.
func (k Keeper) issue(ctx context.Context, country, addr string) (string, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return "", err
	}

	const maxAttempts = 1000
	for nonce := uint64(0); nonce < maxAttempts; nonce++ {
		id := types.Derive(country, addr, nonce, int(params.PayloadLength))

		taken, err := k.Aliases.Has(ctx, id)
		if err != nil {
			return "", err
		}
		if taken {
			continue
		}
		// A tombstone blocks reissue as firmly as a live binding. Handing back
		// a retired identifier would send a payment aimed at a handle somebody
		// memorised into a stranger's account, silently.
		dead, err := k.Retired.Has(ctx, id)
		if err != nil {
			return "", err
		}
		if dead {
			continue
		}
		return id, nil
	}
	return "", types.ErrExhausted
}

// bind writes both directions of a binding at once. They are only ever written
// together; a half-written pair is a reverse lookup that disagrees with the
// forward one.
func (k Keeper) bind(ctx context.Context, id, addr string) error {
	height := sdk.UnwrapSDKContext(ctx).BlockHeight()
	if err := k.Aliases.Set(ctx, id, types.Alias{
		Id:                 id,
		Address:            addr,
		RegisteredAtHeight: height,
	}); err != nil {
		return err
	}
	return k.Owners.Set(ctx, addr, id)
}

// retire tombstones an identifier and drops both directions of its binding.
//
// The tombstone goes in first. Deriving a replacement before the old
// identifier is dead could hand back the one just given up, which is the single
// outcome rotation exists to prevent.
func (k Keeper) retire(ctx context.Context, id, addr string) error {
	if err := k.Retired.Set(ctx, id); err != nil {
		return err
	}
	if err := k.Aliases.Remove(ctx, id); err != nil {
		return err
	}
	return k.Owners.Remove(ctx, addr)
}

// record writes a jurisdiction and keeps the country index in step with it.
//
// Both directions together, and the old index entry removed first: a correction
// that left the previous country's entry behind would show an account inside a
// perimeter it has left, to the authority whose whole reason for querying is to
// know what it may act on.
func (k Keeper) record(ctx context.Context, addr, country, recordedBy string) error {
	if previous, err := k.Jurisdictions.Get(ctx, addr); err == nil {
		if err := k.Perimeter.Remove(ctx, collections.Join(previous.Country, addr)); err != nil {
			return err
		}
	} else if !errors.Is(err, collections.ErrNotFound) {
		return err
	}

	if err := k.Jurisdictions.Set(ctx, addr, types.Jurisdiction{
		Address:          addr,
		Country:          country,
		RecordedBy:       recordedBy,
		RecordedAtHeight: sdk.UnwrapSDKContext(ctx).BlockHeight(),
	}); err != nil {
		return err
	}
	return k.Perimeter.Set(ctx, collections.Join(country, addr))
}
