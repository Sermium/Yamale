package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/herd/types"
)

type queryServer struct {
	Keeper
}

// NewQueryServerImpl returns the read side of the registry.
func NewQueryServerImpl(keeper Keeper) types.QueryServer {
	return queryServer{Keeper: keeper}
}

var _ types.QueryServer = queryServer{}

func (q queryServer) Params(
	ctx context.Context, _ *types.QueryParamsRequest,
) (*types.QueryParamsResponse, error) {
	params, err := q.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

func (q queryServer) Authorities(
	ctx context.Context, _ *types.QueryAuthoritiesRequest,
) (*types.QueryAuthoritiesResponse, error) {
	out := &types.QueryAuthoritiesResponse{}
	err := q.Keeper.Authority.Walk(ctx, nil, func(_ string, a types.Authority) (bool, error) {
		out.Authorities = append(out.Authorities, a)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Agents lists the accredited people, struck-off ones included.
//
// Included deliberately. Every attestation on the register names its attestor,
// and "was this person accredited when they signed" is a question about the
// past — a list that quietly dropped the struck-off could not answer it, and
// an old attestation would appear to have been made by nobody.
func (q queryServer) Agents(
	ctx context.Context, req *types.QueryAgentsRequest,
) (*types.QueryAgentsResponse, error) {
	out := &types.QueryAgentsResponse{}
	err := q.Keeper.Agent.Walk(ctx, nil, func(_ string, a types.Agent) (bool, error) {
		if req.Authority == "" || a.Authority == req.Authority {
			out.Agents = append(out.Agents, a)
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (q queryServer) Herd(
	ctx context.Context, req *types.QueryHerdRequest,
) (*types.QueryHerdResponse, error) {
	herd, err := q.Keeper.Herd.Get(ctx, req.Id)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "no herd %d", req.Id)
		}
		return nil, err
	}
	return &types.QueryHerdResponse{Herd: herd}, nil
}

func (q queryServer) HerdByRef(
	ctx context.Context, req *types.QueryHerdByRefRequest,
) (*types.QueryHerdResponse, error) {
	id, err := q.ByHerdRef.Get(ctx, req.Reference)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "no herd with reference %q", req.Reference)
		}
		return nil, err
	}
	return q.Herd(ctx, &types.QueryHerdRequest{Id: id})
}

func (q queryServer) Animal(
	ctx context.Context, req *types.QueryAnimalRequest,
) (*types.QueryAnimalResponse, error) {
	animal, err := q.Keeper.Animal.Get(ctx, req.Id)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "no animal %d", req.Id)
		}
		return nil, err
	}
	return &types.QueryAnimalResponse{Animal: animal}, nil
}

// AnimalByNationalID is the certificate lookup: the identifier printed on the
// document, resolved to the record it refers to. This is the query a buyer
// standing in a market makes.
//
// The identifier is canonicalised exactly as the write was, so the same animal
// answers to 221/682/102/l and 221/682/000102/L. Without this the register made
// one identifier mean one animal on the way in and still asked the reader to
// type the stored form on the way out. An identifier that cannot be
// canonicalised (a seven-digit serial, a missing part) is refused with the
// reason rather than reported as not found.
func (q queryServer) AnimalByNationalID(
	ctx context.Context, req *types.QueryAnimalByNationalIDRequest,
) (*types.QueryAnimalResponse, error) {
	key, err := CanonicalNationalID(req.NationalId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	id, err := q.ByNationalID.Get(ctx, key)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound,
				"no animal with national identifier %q", req.NationalId)
		}
		return nil, err
	}
	return q.Animal(ctx, &types.QueryAnimalRequest{Id: id})
}

// AnimalByTag resolves a transponder code read in the field, in any of the
// forms a reader or a person writes it (982 000 000 000 102, 982-000000000102):
// canonicalised as the write was.
func (q queryServer) AnimalByTag(
	ctx context.Context, req *types.QueryAnimalByTagRequest,
) (*types.QueryAnimalResponse, error) {
	key, err := CanonicalTag(req.Tag)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if key == "" {
		return nil, status.Error(codes.InvalidArgument, "no transponder code given")
	}
	id, err := q.ByTag.Get(ctx, key)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "no animal carrying tag %q", req.Tag)
		}
		return nil, err
	}
	return q.Animal(ctx, &types.QueryAnimalRequest{Id: id})
}

// Pedigree walks the line upward, breadth first.
//
// The endpoint the free-text certificate cannot have. A line reading
// "Classico / Atomique / Palmarin" can be printed but not followed; this
// returns the records, so a buyer can see what each ancestor actually is and a
// breeder can compute an inbreeding coefficient rather than assert one.
//
// Two things it is careful about. The walk is bounded by MaxPedigreeDepth
// whatever the caller asks for, because the graph was populated by whoever
// declared it. And when the bound is what stopped the walk, the response says
// so — otherwise a truncated line is indistinguishable from a founder, and a
// buyer would read "no further ancestors" where the truth is "we stopped
// looking".
func (q queryServer) Pedigree(
	ctx context.Context, req *types.QueryPedigreeRequest,
) (*types.QueryPedigreeResponse, error) {
	root, err := q.Keeper.Animal.Get(ctx, req.AnimalId)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "no animal %d", req.AnimalId)
		}
		return nil, err
	}

	depth := req.Depth
	if depth == 0 || depth > types.MaxPedigreeDepth {
		depth = types.MaxPedigreeDepth
	}

	out := &types.QueryPedigreeResponse{
		Nodes: []types.PedigreeNode{{Animal: root, Generation: 0, Path: ""}},
	}

	type step struct {
		id   uint64
		path string
	}
	seen := map[uint64]bool{root.Id: true}
	frontier := []step{{root.Id, ""}}

	for generation := uint32(1); generation <= depth && len(frontier) > 0; generation++ {
		var next []step
		for _, from := range frontier {
			animal, err := q.Keeper.Animal.Get(ctx, from.id)
			if err != nil {
				if isNotFound(err) {
					continue
				}
				return nil, err
			}
			for _, parent := range []struct {
				id   uint64
				side string
			}{{animal.SireId, "sire"}, {animal.DamId, "dam"}} {
				if parent.id == 0 || seen[parent.id] {
					continue
				}
				ancestor, err := q.Keeper.Animal.Get(ctx, parent.id)
				if err != nil {
					if isNotFound(err) {
						continue
					}
					return nil, err
				}
				seen[parent.id] = true
				path := parent.side
				if from.path != "" {
					path = from.path + "." + parent.side
				}
				out.Nodes = append(out.Nodes, types.PedigreeNode{
					Animal: ancestor, Generation: generation, Path: path,
				})
				next = append(next, step{parent.id, path})
			}
		}
		frontier = next
	}

	// Anything still on the frontier is an ancestor we did not fetch because
	// the bound ran out, not because the line ended.
	out.Truncated = len(frontier) > 0
	return out, nil
}

func (q queryServer) Animals(
	ctx context.Context, req *types.QueryAnimalsRequest,
) (*types.QueryAnimalsResponse, error) {
	out := &types.QueryAnimalsResponse{}
	rng := collections.NewPrefixedPairRange[uint64, uint64](req.HerdId)
	err := q.AnimalsByHerd.Walk(ctx, rng, func(key collections.Pair[uint64, uint64]) (bool, error) {
		animal, err := q.Keeper.Animal.Get(ctx, key.K2())
		if err != nil {
			if isNotFound(err) {
				return false, nil
			}
			return true, err
		}
		out.Animals = append(out.Animals, animal)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (q queryServer) HealthEvents(
	ctx context.Context, req *types.QueryHealthEventsRequest,
) (*types.QueryHealthEventsResponse, error) {
	out := &types.QueryHealthEventsResponse{}
	rng := collections.NewPrefixedPairRange[uint64, uint64](req.HerdId)
	err := q.HealthByHerd.Walk(ctx, rng, func(key collections.Pair[uint64, uint64]) (bool, error) {
		event, err := q.HealthEvent.Get(ctx, key.K2())
		if err != nil {
			if isNotFound(err) {
				return false, nil
			}
			return true, err
		}
		out.HealthEvents = append(out.HealthEvents, event)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (q queryServer) Transfer(
	ctx context.Context, req *types.QueryTransferRequest,
) (*types.QueryTransferResponse, error) {
	transfer, err := q.Keeper.Transfer.Get(ctx, req.Id)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "no transfer %d", req.Id)
		}
		return nil, err
	}
	return &types.QueryTransferResponse{Transfer: transfer}, nil
}

// TransfersByAnimal is one animal's provenance, objected transfers included.
// A refused sale is part of what a buyer should see: an animal somebody once
// tried to sell and was stopped from selling is worth knowing about.
func (q queryServer) TransfersByAnimal(
	ctx context.Context, req *types.QueryTransfersByAnimalRequest,
) (*types.QueryTransfersResponse, error) {
	out := &types.QueryTransfersResponse{}
	rng := collections.NewPrefixedPairRange[uint64, uint64](req.AnimalId)
	err := q.Keeper.TransfersByAnimal.Walk(ctx, rng,
		func(key collections.Pair[uint64, uint64]) (bool, error) {
			transfer, err := q.Keeper.Transfer.Get(ctx, key.K2())
			if err != nil {
				if isNotFound(err) {
					return false, nil
				}
				return true, err
			}
			out.Transfers = append(out.Transfers, transfer)
			return false, nil
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}
