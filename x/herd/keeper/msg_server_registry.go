package keeper

import (
	"context"
	"strings"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/herd/types"
)

// EnrolHerd records a herd at level 1.
//
// This is the message that has to be cheap, because it is the only one that can
// cover a national herd. Senegal has 21,185,146 head; at 1,000 FCFA per tag
// fitted, identifying them individually costs 96.6% of the entire PNDIES
// programme in tags alone. Level 1 is costed per herd instead, and it is what
// makes verifiable vaccination coverage possible at all.
//
// Nothing here is verified beyond the agent having stood in front of the herd.
// The declared size is declared, and the field says so. A registry that
// pretended otherwise would be claiming a census it never took.
func (m msgServer) EnrolHerd(
	ctx context.Context, msg *types.MsgEnrolHerd,
) (*types.MsgEnrolHerdResponse, error) {
	office, err := m.activeAuthority(ctx, msg.Creator)
	if err != nil {
		return nil, err
	}

	reference := strings.TrimSpace(msg.Reference)
	if reference == "" {
		return nil, types.ErrNoReference
	}
	if _, err := m.addressCodec.StringToBytes(msg.Keeper); err != nil {
		return nil, types.ErrInvalidKeeper.Wrapf("%q", msg.Keeper)
	}
	if msg.Species == types.SPECIES_UNSPECIFIED {
		return nil, types.ErrNoSpecies
	}

	// The registry's own reference must be unique, or two records claim to be
	// the same file and reconciliation with the paper world becomes guesswork.
	if existing, err := m.ByHerdRef.Get(ctx, reference); err == nil {
		return nil, types.ErrRefTaken.Wrapf("herd %d", existing)
	} else if !isNotFound(err) {
		return nil, err
	}

	id, err := nextID(ctx, m.NextHerdID)
	if err != nil {
		return nil, err
	}

	herd := types.Herd{
		Id:           id,
		Reference:    reference,
		Keeper:       msg.Keeper,
		Authority:    office.Address,
		Species:      msg.Species,
		DeclaredSize: msg.DeclaredSize,
		Locality:     msg.Locality,
		LocationHash: msg.LocationHash,
		Jurisdiction: office.Jurisdiction,
		Status:       types.STATUS_ACTIVE,
		RegisteredAt: blockHeight(ctx),
	}
	if err := m.Herd.Set(ctx, id, herd); err != nil {
		return nil, err
	}
	if err := m.ByHerdRef.Set(ctx, reference, id); err != nil {
		return nil, err
	}
	return &types.MsgEnrolHerdResponse{Id: id}, nil
}

// IdentifyAnimal records one animal at level 2.
func (m msgServer) IdentifyAnimal(
	ctx context.Context, msg *types.MsgIdentifyAnimal,
) (*types.MsgIdentifyAnimalResponse, error) {
	office, err := m.activeAuthority(ctx, msg.Creator)
	if err != nil {
		return nil, err
	}

	herd, err := m.Herd.Get(ctx, msg.HerdId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoHerd.Wrapf("herd %d", msg.HerdId)
		}
		return nil, err
	}

	nationalID := strings.TrimSpace(msg.NationalId)
	if nationalID == "" {
		return nil, types.ErrNoNationalID
	}
	if existing, err := m.ByNationalID.Get(ctx, nationalID); err == nil {
		return nil, types.ErrNationalIDTaken.Wrapf("animal %d", existing)
	} else if !isNotFound(err) {
		return nil, err
	}

	// A tag already fitted to another animal is either a misread or a swap, and
	// both are exactly what the registry exists to catch. Empty tags are not
	// indexed: level 1 has none, and indexing the empty string would make every
	// untagged animal collide with every other.
	tag := strings.TrimSpace(msg.Tag)
	if tag != "" {
		if existing, err := m.ByTag.Get(ctx, tag); err == nil {
			return nil, types.ErrTagTaken.Wrapf("animal %d", existing)
		} else if !isNotFound(err) {
			return nil, err
		}
	}

	id, err := nextID(ctx, m.NextAnimalID)
	if err != nil {
		return nil, err
	}

	// Level 2 requires a tag; without one this is a record of an animal nothing
	// physically points at, which is level 1 and belongs on the herd.
	level := types.LEVEL_INDIVIDUAL
	if tag == "" {
		return nil, types.ErrNoTag.Wrapf(
			"identifying an animal individually needs a transponder; a record with no " +
				"marker is a herd enrolment, not an animal")
	}

	animal := types.Animal{
		Id:           id,
		HerdId:       herd.Id,
		NationalId:   nationalID,
		Tag:          tag,
		Level:        level,
		Name:         msg.Name,
		Sex:          msg.Sex,
		Coat:         msg.Coat,
		BornOn:       strings.TrimSpace(msg.BornOn),
		BirthType:    msg.BirthType,
		Holder:       herd.Keeper,
		Authority:    office.Address,
		Status:       types.STATUS_ACTIVE,
		RegisteredAt: blockHeight(ctx),
	}
	if err := m.Animal.Set(ctx, id, animal); err != nil {
		return nil, err
	}
	if err := m.ByNationalID.Set(ctx, nationalID, id); err != nil {
		return nil, err
	}
	if err := m.ByTag.Set(ctx, tag, id); err != nil {
		return nil, err
	}
	if err := m.AnimalsByHerd.Set(ctx, collections.Join(herd.Id, id)); err != nil {
		return nil, err
	}
	return &types.MsgIdentifyAnimalResponse{Id: id}, nil
}

// CertifyAnimal raises an animal to level 3.
//
// The pedigree arrives as record ids, and every refusal in checkParentage is a
// declaration that cannot be true of living animals. None of them proves a
// pedigree — only the laboratory does that, which is why the DNA hash is
// required by default — but each refuses one that cannot be, and that is
// already more than a free-text line can do.
func (m msgServer) CertifyAnimal(
	ctx context.Context, msg *types.MsgCertifyAnimal,
) (*types.MsgCertifyAnimalResponse, error) {
	if _, err := m.activeAuthority(ctx, msg.Creator); err != nil {
		return nil, err
	}

	animal, err := m.Animal.Get(ctx, msg.AnimalId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoAnimal.Wrapf("animal %d", msg.AnimalId)
		}
		return nil, err
	}
	if animal.Status != types.STATUS_ACTIVE {
		return nil, types.ErrAnimalNotActive.Wrapf("animal %d is %s", animal.Id, animal.Status)
	}
	if animal.Tag == "" {
		return nil, types.ErrNoTag.Wrapf("animal %d", animal.Id)
	}

	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if params.RequireDnaForCertified && strings.TrimSpace(msg.DnaHash) == "" {
		return nil, types.ErrNoDNA.Wrapf("animal %d", animal.Id)
	}

	if err := m.checkParentage(ctx, animal, msg.SireId, msg.DamId); err != nil {
		return nil, err
	}

	animal.SireId = msg.SireId
	animal.DamId = msg.DamId
	animal.DnaHash = strings.TrimSpace(msg.DnaHash)
	animal.PhotoHash = strings.TrimSpace(msg.PhotoHash)
	animal.Level = types.LEVEL_CERTIFIED

	if err := m.Animal.Set(ctx, animal.Id, animal); err != nil {
		return nil, err
	}
	return &types.MsgCertifyAnimalResponse{}, nil
}

// RecordHealthEvent is what turns a campaign into coverage.
//
// A vaccination campaign without a registry produces a number of doses
// administered. That is not coverage, and coverage is the only figure
// eradication needs — PPR across 15,153,137 small ruminants being the case that
// pays for the whole level-1 tier.
//
// Signed by the agent rather than the office, because the signature IS the
// attestation: a named person saying they administered this batch to this herd.
func (m msgServer) RecordHealthEvent(
	ctx context.Context, msg *types.MsgRecordHealthEvent,
) (*types.MsgRecordHealthEventResponse, error) {
	if _, err := m.Herd.Get(ctx, msg.HerdId); err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoHerd.Wrapf("herd %d", msg.HerdId)
		}
		return nil, err
	}
	if strings.TrimSpace(msg.Disease) == "" {
		return nil, types.ErrNoDisease
	}
	if msg.Treated == 0 {
		return nil, types.ErrNoTreated
	}

	id, err := nextID(ctx, m.NextHealthEventID)
	if err != nil {
		return nil, err
	}

	// The number treated may differ from the herd's declared size, and that
	// difference is itself information — a declaration drifting from what an
	// agent counts is the signal an inspector wants. Forcing them equal would
	// destroy the only evidence that a declaration is stale.
	event := types.HealthEvent{
		Id:         id,
		HerdId:     msg.HerdId,
		Disease:    strings.TrimSpace(msg.Disease),
		Batch:      strings.TrimSpace(msg.Batch),
		Treated:    msg.Treated,
		Agent:      msg.Agent,
		RecordedAt: blockHeight(ctx),
	}
	if err := m.HealthEvent.Set(ctx, id, event); err != nil {
		return nil, err
	}
	if err := m.HealthByHerd.Set(ctx, collections.Join(msg.HerdId, id)); err != nil {
		return nil, err
	}
	return &types.MsgRecordHealthEventResponse{Id: id}, nil
}

// RetireAnimal records a death, a slaughter or a sale out of the country.
//
// The record stays. A pedigree refers to its ancestors, and deleting one would
// silently break every descendant's line — the animal would vanish and its
// children would appear to be founders.
func (m msgServer) RetireAnimal(
	ctx context.Context, msg *types.MsgRetireAnimal,
) (*types.MsgRetireAnimalResponse, error) {
	if _, err := m.activeAuthority(ctx, msg.Creator); err != nil {
		return nil, err
	}

	animal, err := m.Animal.Get(ctx, msg.AnimalId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoAnimal.Wrapf("animal %d", msg.AnimalId)
		}
		return nil, err
	}
	if animal.Status == types.STATUS_TRANSFER_PENDING {
		return nil, types.ErrAnimalNotActive.Wrapf(
			"animal %d has a transfer under way; resolve it before retiring the record",
			animal.Id)
	}

	animal.Status = types.STATUS_RETIRED
	if err := m.Animal.Set(ctx, animal.Id, animal); err != nil {
		return nil, err
	}

	// The tag is released. A transponder recovered from a dead animal is
	// legitimately refitted, and holding the index entry forever would make
	// that look like a duplicate — which is the check working against the
	// practice it exists to protect.
	if animal.Tag != "" {
		if err := m.ByTag.Remove(ctx, animal.Tag); err != nil {
			return nil, err
		}
	}
	return &types.MsgRetireAnimalResponse{}, nil
}

// AccreditAgent admits one person as an attestor, or strikes them off.
//
// Signed by an OFFICE, not by governance, and the difference is the whole
// reason this message exists. Attestation required the attestor to be an
// Authority until a sale was actually run end to end — and an Authority must be
// an x/group account admitted by a governance proposal, so every field
// veterinarian would have been a multi-signature institution with a
// parliamentary vote behind them. With a quorum of two, a single lawful sale
// needed two of those to exist first. That is not a high bar in a pastoral
// zone; it is an impossible one, and a lawful route nobody can take pushes the
// trade off the register entirely.
//
// Accreditation is the office's own act, which is also how it works off-chain:
// a national veterinary order admits and strikes off its own members without
// asking a legislature. What governance still decides is which offices exist,
// which is the level where that question belongs.
func (m msgServer) AccreditAgent(
	ctx context.Context, msg *types.MsgAccreditAgent,
) (*types.MsgAccreditAgentResponse, error) {
	office, err := m.activeAuthority(ctx, msg.Authority)
	if err != nil {
		return nil, err
	}
	if _, err := m.addressCodec.StringToBytes(msg.Agent); err != nil {
		return nil, types.ErrInvalidAgent.Wrapf("%q", msg.Agent)
	}

	// An office cannot accredit an office. Not tidiness: an office is a group
	// account whose members already decide together, and letting one hold a
	// plain-key agent record would give it a second identity that attests
	// alone — which is the independence rule defeated from inside.
	if _, err := m.Authority.Get(ctx, msg.Agent); err == nil {
		return nil, types.ErrInvalidAgent.Wrapf(
			"%s is a registry office, not a person", msg.Agent)
	} else if !isNotFound(err) {
		return nil, err
	}

	// Re-accrediting an agent already registered to a DIFFERENT office would
	// move them silently, and every past attestation of theirs was weighed for
	// independence against the office they belonged to at the time. The record
	// stays where it is; the other office must strike them off first.
	if existing, err := m.Agent.Get(ctx, msg.Agent); err == nil {
		if existing.Authority != office.Address {
			return nil, types.ErrInvalidAgent.Wrapf(
				"%s is accredited by %s and must be struck off there first",
				msg.Agent, existing.Authority)
		}
	} else if !isNotFound(err) {
		return nil, err
	}

	if err := m.Agent.Set(ctx, msg.Agent, types.Agent{
		Address: msg.Agent,
		Name:    msg.Name,
		// The office's own perimeter. An agent cannot be accredited into a
		// jurisdiction their office does not hold, so this is copied rather
		// than accepted from the message.
		Authority:    office.Address,
		Jurisdiction: office.Jurisdiction,
		Active:       msg.Active,
		AccreditedAt: blockTime(ctx),
	}); err != nil {
		return nil, err
	}
	return &types.MsgAccreditAgentResponse{}, nil
}
