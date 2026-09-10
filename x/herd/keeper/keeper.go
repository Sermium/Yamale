package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"

	"yamale/blockchain/x/herd/types"
)

// Keeper holds the national livestock registry.
//
// Three of these collections carry the guarantees the module exists for, and
// every one of them is an index rather than a scan:
//
//   - ByNationalID makes an identifier mean one animal. It is the key printed
//     on a certificate, so a duplicate is two documents that both look genuine.
//   - ByTag resolves what an agent's reader actually returns in a field with no
//     signal, and refuses a tag already fitted to something else.
//   - AnimalsByHerd turns "list this herd" into a prefix walk. On a national
//     herd of twenty-one million, a scan is a query nobody runs twice.
type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec

	// Address capable of executing governance-gated messages: the x/gov module
	// account. Registry offices are admitted by governance and never by each
	// other, because an office that can admit offices can manufacture the
	// independent attestors a transfer quorum depends on.
	authority []byte

	// Asked once, at admission: is this office a group account?
	groupKeeper types.GroupKeeper

	Schema collections.Schema
	Params collections.Item[types.Params]

	Herd        collections.Map[uint64, types.Herd]
	Animal      collections.Map[uint64, types.Animal]
	HealthEvent collections.Map[uint64, types.HealthEvent]
	Transfer    collections.Map[uint64, types.Transfer]
	Authority   collections.Map[string, types.Authority]
	// The accredited people. An office is an x/group account admitted by
	// governance; an agent is a person with a plain key accredited by an
	// office. Conflating the two made every field veterinarian a
	// multi-signature institution with a parliamentary vote behind them.
	Agent collections.Map[string, types.Agent]

	// herd reference -> herd id. How an officer with a paper file searches.
	ByHerdRef collections.Map[string, uint64]
	// national identifier -> animal id. The uniqueness constraint on animals.
	ByNationalID collections.Map[string, uint64]
	// transponder code -> animal id. Empty tags are not indexed: level 1 has
	// no tag, and indexing the empty string would make every untagged animal
	// collide with every other.
	ByTag collections.Map[string, uint64]

	// (herd id, animal id) and (herd id, event id), so one herd's records are a
	// prefix walk. The value is empty — the pair itself is the information.
	AnimalsByHerd collections.KeySet[collections.Pair[uint64, uint64]]
	HealthByHerd  collections.KeySet[collections.Pair[uint64, uint64]]
	// (animal id, transfer id): one animal's provenance, objected transfers
	// included, because a refused sale is part of what a buyer should see.
	TransfersByAnimal collections.KeySet[collections.Pair[uint64, uint64]]

	NextHerdID        collections.Sequence
	NextAnimalID      collections.Sequence
	NextHealthEventID collections.Sequence
	NextTransferID    collections.Sequence
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,
	groupKeeper types.GroupKeeper,
) Keeper {
	if _, err := addressCodec.BytesToString(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		storeService: storeService,
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,
		groupKeeper:  groupKeeper,

		Params: collections.NewItem(sb, types.ParamsKey, "params",
			codec.CollValue[types.Params](cdc)),

		Herd: collections.NewMap(sb, types.HerdKey, "herd",
			collections.Uint64Key, codec.CollValue[types.Herd](cdc)),
		Animal: collections.NewMap(sb, types.AnimalKey, "animal",
			collections.Uint64Key, codec.CollValue[types.Animal](cdc)),
		HealthEvent: collections.NewMap(sb, types.HealthEventKey, "health_event",
			collections.Uint64Key, codec.CollValue[types.HealthEvent](cdc)),
		Transfer: collections.NewMap(sb, types.TransferKey, "transfer",
			collections.Uint64Key, codec.CollValue[types.Transfer](cdc)),
		Authority: collections.NewMap(sb, types.AuthorityKey, "authority",
			collections.StringKey, codec.CollValue[types.Authority](cdc)),
		Agent: collections.NewMap(sb, types.AgentKey, "agent",
			collections.StringKey, codec.CollValue[types.Agent](cdc)),

		ByHerdRef: collections.NewMap(sb, types.ByHerdRefKey, "by_herd_ref",
			collections.StringKey, collections.Uint64Value),
		ByNationalID: collections.NewMap(sb, types.ByNationalIDKey, "by_national_id",
			collections.StringKey, collections.Uint64Value),
		ByTag: collections.NewMap(sb, types.ByTagKey, "by_tag",
			collections.StringKey, collections.Uint64Value),

		AnimalsByHerd: collections.NewKeySet(sb, types.AnimalsByHerdKey, "animals_by_herd",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
		HealthByHerd: collections.NewKeySet(sb, types.HealthByHerdKey, "health_by_herd",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
		TransfersByAnimal: collections.NewKeySet(sb, types.TransfersByAnimalKey,
			"transfers_by_animal",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),

		NextHerdID:        collections.NewSequence(sb, types.NextHerdIDKey, "next_herd_id"),
		NextAnimalID:      collections.NewSequence(sb, types.NextAnimalIDKey, "next_animal_id"),
		NextHealthEventID: collections.NewSequence(sb, types.NextHealthEventIDKey, "next_health_id"),
		NextTransferID:    collections.NewSequence(sb, types.NextTransferIDKey, "next_transfer_id"),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

// GetAuthority returns the module's governance authority.
func (k Keeper) GetAuthority() []byte {
	return k.authority
}

// nextID hands out an id, skipping zero.
//
// Zero is never issued for any record here. In proto3 a zero id is
// indistinguishable from an unset field, and this module leans on that
// distinction in a place that matters: an animal's sire_id and dam_id are zero
// when the parentage is unknown, which is the honest and common case for a
// founder. If animal zero existed, every founder would appear to be the child
// of it.
//
// Skipped rather than offset, because genesis refuses an import whose counter
// sits behind an existing id, and an offset would trip that on the first record.
func nextID(ctx context.Context, seq collections.Sequence) (uint64, error) {
	id, err := seq.Next(ctx)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return seq.Next(ctx)
	}
	return id, nil
}

// activeAuthority resolves a signer to the registry office it is, and refuses
// anything else.
//
// Every write in this module goes through it. An office that has been struck
// off keeps its record — the history of what it did stays readable — but stops
// being able to act, which is why the check is on `active` rather than on
// existence.
func (k Keeper) activeAuthority(ctx context.Context, signer string) (types.Authority, error) {
	office, err := k.Authority.Get(ctx, signer)
	if err != nil {
		if isNotFound(err) {
			return types.Authority{}, types.ErrNotAuthority.Wrapf("%s", signer)
		}
		return types.Authority{}, err
	}
	if !office.Active {
		return types.Authority{}, types.ErrAuthorityInactive.Wrapf("%s", signer)
	}
	return office, nil
}

// activeAgent resolves a signer to the accredited person they are, and refuses
// anything else.
//
// An agent struck off keeps their record — every attestation they ever made
// names them, and "was this person accredited when they signed" has to stay
// answerable — so the check is on `active` rather than on existence, exactly as
// it is for an office.
func (k Keeper) activeAgent(ctx context.Context, signer string) (types.Agent, error) {
	agent, err := k.Agent.Get(ctx, signer)
	if err != nil {
		if isNotFound(err) {
			return types.Agent{}, types.ErrNotAgent.Wrapf("%s", signer)
		}
		return types.Agent{}, err
	}
	if !agent.Active {
		return types.Agent{}, types.ErrAgentInactive.Wrapf("%s", signer)
	}
	return agent, nil
}

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, collections.ErrNotFound)
}
