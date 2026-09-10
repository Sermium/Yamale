package types

import "cosmossdk.io/errors"

// Errors are numbered from 1 so that a zero code is never a herd error, and
// worded as what an agent standing in a field would need to be told. These
// reach a person through a handset with no signal and no manual, and "invalid
// request" tells them nothing they can act on.
var (
	ErrNotAuthority        = errors.Register(ModuleName, 1, "signer is not a registry office")
	ErrAuthorityInactive   = errors.Register(ModuleName, 2, "this registry office is not active")
	ErrInvalidJurisdiction = errors.Register(ModuleName, 3,
		"that is not a country this chain recognises")
	ErrOfficeNotGroup = errors.Register(ModuleName, 4,
		"a registry office must be a group account, so its decisions need several signatures")

	ErrNoReference   = errors.Register(ModuleName, 5, "a herd reference is required")
	ErrRefTaken      = errors.Register(ModuleName, 6, "this herd reference is already used")
	ErrInvalidKeeper = errors.Register(ModuleName, 7, "the keeper is not a valid account")
	ErrNoSpecies     = errors.Register(ModuleName, 8, "a species is required")
	ErrNoHerd        = errors.Register(ModuleName, 9, "no such herd")

	ErrNoNationalID    = errors.Register(ModuleName, 10, "a national identifier is required")
	ErrNationalIDTaken = errors.Register(ModuleName, 11, "this national identifier is already used")
	ErrTagTaken        = errors.Register(ModuleName, 12, "this tag is already fitted to another animal")
	ErrNoAnimal        = errors.Register(ModuleName, 13, "no such animal")
	ErrAnimalNotActive = errors.Register(ModuleName, 14, "this animal is not in a state that allows it")

	// The pedigree refusals. Each of these is a declaration that cannot be true
	// of living animals, and refusing them is the difference between a pedigree
	// and a list of names.
	ErrNoTag = errors.Register(ModuleName, 15,
		"an animal must carry a tag before it can be certified")
	ErrNoDNA = errors.Register(ModuleName, 16,
		"a certified animal needs a DNA profile; without one the parentage is the breeder's word")
	ErrParentMissing = errors.Register(ModuleName, 17, "the parent named is not a record here")
	ErrSireNotMale   = errors.Register(ModuleName, 18, "the animal named as sire is not male")
	ErrDamNotFemale  = errors.Register(ModuleName, 19, "the animal named as dam is not female")
	ErrParentYounger = errors.Register(ModuleName, 20,
		"the parent named was born after this animal")
	ErrPedigreeCycle = errors.Register(ModuleName, 21,
		"that would make the animal its own ancestor")
	ErrBirthTooClose = errors.Register(ModuleName, 22,
		"this dam already has a birth recorded too recently for another")

	ErrNoDisease     = errors.Register(ModuleName, 23, "a disease is required")
	ErrNoTreated     = errors.Register(ModuleName, 24, "the number treated must be positive")
	ErrNoHealthEvent = errors.Register(ModuleName, 25, "no such health event")

	ErrNotKeeper        = errors.Register(ModuleName, 26, "only the keeper may propose a transfer")
	ErrInvalidRecipient = errors.Register(ModuleName, 27, "the recipient is not a valid account")
	ErrSelfTransfer     = errors.Register(ModuleName, 28, "the recipient is already the keeper")
	ErrNoTransfer       = errors.Register(ModuleName, 29, "no such transfer")
	ErrTransferClosed   = errors.Register(ModuleName, 30, "this transfer is already finished")
	ErrNotBuyer         = errors.Register(ModuleName, 31, "only the named buyer may accept")
	ErrNotAccepted      = errors.Register(ModuleName, 32, "the buyer has not accepted yet")
	ErrAlreadyAttested  = errors.Register(ModuleName, 33, "this attestor has already attested")
	ErrNotIndependent   = errors.Register(ModuleName, 34,
		"an attestor from the proposing office is not independent")
	ErrQuorumNotMet     = errors.Register(ModuleName, 35, "not enough attestations yet")
	ErrWindowOpen       = errors.Register(ModuleName, 36, "the challenge window has not closed")
	ErrTransferObjected = errors.Register(ModuleName, 37, "somebody objected to this transfer")

	ErrInvalidLevel = errors.Register(ModuleName, 38, "a level is required")

	// The way out of every stop. An objection is open to anybody and costs
	// nothing, so each of these exists to make sure the stop it lifts is one a
	// named office answered for rather than one that simply expired.
	ErrNotSeller        = errors.Register(ModuleName, 39, "only the seller may withdraw a proposal")
	ErrNotObjected      = errors.Register(ModuleName, 40, "there is no objection on this transfer to resolve")
	ErrTransferAttested = errors.Register(ModuleName, 41,
		"an attested transfer can no longer be withdrawn; the buyer and an agent have acted on it")
	ErrNotHeld  = errors.Register(ModuleName, 42, "this animal is not under a hold")
	ErrNoReason = errors.Register(ModuleName, 43,
		"a reason is required; a stop or a release with no reason recorded is unreviewable")
	ErrWrongJurisdiction = errors.Register(ModuleName, 44,
		"this animal is not in your office's jurisdiction")
)
