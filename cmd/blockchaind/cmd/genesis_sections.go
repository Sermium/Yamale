package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/types/module"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"

	"yamale/blockchain/app"
)

// withProfileAwareGenesisValidation makes `genesis validate` refuse the same
// files the node refuses.
//
// The SDK's validator walks the registered modules and indexes into the genesis
// map, so a section belonging to a module this profile does not link is never
// looked at and the file is pronounced valid. The node then declines to start
// on it (see app/genesis_sections.go). One binary answering "valid" and
// "cannot start" about one file is how a ceremony ships the wrong genesis: the
// operator has already been told it checks out.
//
// The check is layered on rather than replacing the SDK command, so every
// validation the SDK performs still runs. It runs before them because a
// wrong-profile genesis makes every other complaint a red herring: a section
// this binary has no module for will also fail whatever module-specific
// validation happens to notice it first, and that error names a field.
func withProfileAwareGenesisValidation(genesisCmd *cobra.Command, basicManager module.BasicManager) *cobra.Command {
	known := make([]string, 0, len(basicManager))
	for name := range basicManager {
		known = append(known, name)
	}

	for _, sub := range genesisCmd.Commands() {
		if sub.Name() != "validate" {
			continue
		}

		sdkRunE := sub.RunE
		sub.RunE = func(cmd *cobra.Command, args []string) error {
			genesisFile := server.GetServerContextFromCmd(cmd).Config.GenesisFile()
			if len(args) > 0 {
				genesisFile = args[0]
			}

			genState, _, err := genutiltypes.GenesisStateFromGenFile(genesisFile)
			if err != nil {
				// Left to the SDK command, which has the better diagnostics for
				// a file that is malformed rather than mismatched.
				return sdkRunE(cmd, args)
			}
			if err := app.CheckGenesisSections(genState, known); err != nil {
				return fmt.Errorf("%s: %w", genesisFile, err)
			}

			return sdkRunE(cmd, args)
		}
	}

	return genesisCmd
}
