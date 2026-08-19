package keeper_test

import "yamale/blockchain/x/validatorgov/types"

// testDeclaration is a complete, valid declaration. Tests that are not about
// the register still need one, because a validator without a declaration
// belongs to no concentration group and genesis refuses it.
func testDeclaration(entity, owner, jurisdiction string) types.Declaration {
	return types.Declaration{
		LegalEntityId:     entity,
		BeneficialOwnerId: owner,
		Jurisdiction:      jurisdiction,
	}
}
