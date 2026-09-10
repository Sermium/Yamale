package herd

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/herd/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
//
// The short descriptions are written for the person who will actually type
// these: a registrar, a veterinarian in a field, an officer holding a paper
// file. "Queries an animal by identifier" tells them nothing; "the certificate
// lookup — the number printed on the document" tells them which command to
// reach for.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Shows the attestation quorum, the challenge window and the birth interval now in force",
				},
				{
					RpcMethod: "Authorities",
					Use:       "authorities",
					Short:     "Lists the registry offices and the jurisdictions they administer",
				},
				{
					RpcMethod:      "Herd",
					Use:            "herd [id]",
					Short:          "Shows one herd: its keeper, species, declared size and where it is",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "HerdByRef",
					Use:            "herd-by-ref [reference]",
					Short:          "Finds a herd by the reference written on the paper file",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "reference"}},
				},
				{
					RpcMethod:      "Animal",
					Use:            "animal [id]",
					Short:          "Shows one animal: its level, tag, pedigree references and keeper",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "AnimalByNationalID",
					Use:            "animal-by-id [national-id]",
					Short:          "The certificate lookup: the identifier printed on the document",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "national_id"}},
				},
				{
					RpcMethod:      "AnimalByTag",
					Use:            "animal-by-tag [tag]",
					Short:          "Resolves a transponder code read with a hand-held reader",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "tag"}},
				},
				{
					RpcMethod: "Pedigree",
					Use:       "pedigree [animal-id] [depth]",
					Short:     "Walks the line upward — the thing a free-text certificate cannot do",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "animal_id"},
						{ProtoField: "depth"},
					},
				},
				{
					RpcMethod:      "Animals",
					Use:            "animals [herd-id]",
					Short:          "Lists the individually identified animals in one herd",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "herd_id"}},
				},
				{
					RpcMethod:      "HealthEvents",
					Use:            "health [herd-id]",
					Short:          "What a herd has been treated for — coverage, not a count of doses",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "herd_id"}},
				},
				{
					RpcMethod:      "Transfer",
					Use:            "transfer [id]",
					Short:          "Shows one transfer and who has attested to it",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "TransfersByAnimal",
					Use:            "provenance [animal-id]",
					Short:          "Every supervised handover an animal has been through, refusals included",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "animal_id"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: types.Msg_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "EnrolHerd",
					Use:       "enrol-herd [reference] [keeper] [species] [declared-size] [locality]",
					Short:     "Records a herd at level 1 — the level that can cover a national herd",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "reference"},
						{ProtoField: "keeper"},
						{ProtoField: "species"},
						{ProtoField: "declared_size"},
						{ProtoField: "locality"},
					},
				},
				{
					RpcMethod: "IdentifyAnimal",
					Use:       "identify [herd-id] [national-id] [tag] [name] [sex]",
					Short:     "Records one animal at level 2, bound to its record by a transponder",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "herd_id"},
						{ProtoField: "national_id"},
						{ProtoField: "tag"},
						{ProtoField: "name"},
						{ProtoField: "sex"},
					},
				},
				{
					RpcMethod: "CertifyAnimal",
					Use:       "certify [animal-id] [sire-id] [dam-id] [dna-hash] [photo-hash]",
					Short:     "Raises an animal to level 3, refusing a line that cannot be true",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "animal_id"},
						{ProtoField: "sire_id"},
						{ProtoField: "dam_id"},
						{ProtoField: "dna_hash"},
						{ProtoField: "photo_hash"},
					},
				},
				{
					RpcMethod: "RecordHealthEvent",
					Use:       "record-health [herd-id] [disease] [batch] [treated]",
					Short:     "Records a vaccination, which is what turns a campaign into coverage",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "herd_id"},
						{ProtoField: "disease"},
						{ProtoField: "batch"},
						{ProtoField: "treated"},
					},
				},
				{
					RpcMethod: "ProposeTransfer",
					Use:       "propose-transfer [animal-id] [to] [price]",
					Short:     "Opens a supervised handover. Only the keeper may",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "animal_id"},
						{ProtoField: "to"},
						{ProtoField: "price"},
					},
				},
				{
					RpcMethod:      "AcceptTransfer",
					Use:            "accept-transfer [transfer-id]",
					Short:          "The buyer agreeing to receive the animal",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "transfer_id"}},
				},
				{
					RpcMethod: "AttestTransfer",
					Use:       "attest-transfer [transfer-id] [evidence-hash]",
					Short:     "An accredited agent swearing the animal presented is the animal on the record",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "transfer_id"},
						{ProtoField: "evidence_hash"},
					},
				},
				{
					RpcMethod: "ObjectTransfer",
					Use:       "object-transfer [transfer-id] [reason]",
					Short:     "Stops a transfer. Open to anybody, because the person who recognises a stolen animal is rarely a party to the sale",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "transfer_id"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod:      "CompleteTransfer",
					Use:            "complete-transfer [transfer-id]",
					Short:          "Moves the animal, once the window has run. Anybody may pay the gas",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "transfer_id"}},
				},
				{
					RpcMethod: "RetireAnimal",
					Use:       "retire [animal-id] [reason]",
					Short:     "Records a death or slaughter. The record stays, because a pedigree refers to it",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "animal_id"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "CancelTransfer",
					Use:       "cancel-transfer [transfer-id] [reason]",
					Short:     "Withdraws a proposal the buyer never took up. Seller only, and only before an agent has attested",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "transfer_id"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "ResolveObjection",
					Use:       "resolve-objection [transfer-id] [uphold] [reason]",
					Short:     "An office deciding whether an objection had substance. Uphold stops the sale; dismiss restarts the challenge window",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "transfer_id"},
						{ProtoField: "uphold"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "FreezeAnimal",
					Use:       "freeze [animal-id] [reason]",
					Short:     "Stops an animal moving: a fraud inquiry, a court order, a sanitary hold",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "animal_id"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "ReleaseAnimal",
					Use:       "release [animal-id] [reason]",
					Short:     "Ends a hold and puts the animal back into circulation",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "animal_id"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "RegisterAuthority",
					Skip:      true, // governance only
				},
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // governance only
				},
			},
		},
	}
}
