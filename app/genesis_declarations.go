package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	validatorgovtypes "yamale/blockchain/x/validatorgov/types"
)

// CheckGenesisDeclarations refuses a genesis whose validators are not all
// declared.
//
// # The hole this closes
//
// Concentration ceilings — no legal entity, beneficial owner or jurisdiction
// above its share of the validator set — are computed from each validator's
// declaration. A validator with no ApprovedValidator record belongs to no group
// at all: it is counted in the total power that every other group's share is
// measured against, and it can never itself be demoted, because there is no
// group to demote it from.
//
// x/validatorgov's own genesis validation already refuses an ApprovedValidator
// carrying an incomplete declaration. That is not the gap. The gap is a
// validator that never appears in x/validatorgov's genesis at all, which is
// precisely how a founding set is constituted: gentx writes into x/staking and
// x/genutil, and nothing obliges it to write a declaration anywhere.
//
// So the founding validators — the ones holding the most power, on the chain
// where concentration matters most — were the only ones the ceilings could not
// reach. An independent review named this on 2026-08-31 and was right that it
// is a genesis-construction discipline issue rather than a logic defect; the
// answer to a discipline issue that decides whether a constitutional ceiling
// binds is to stop relying on discipline.
//
// # Why this lives in app and not in the module
//
// A module's Validate sees its own section and nothing else. This question
// spans three — x/staking's validators, x/genutil's gentxs, and
// x/validatorgov's approvals — so it can only be asked where the whole file is,
// which is here and in `genesis validate`.
func CheckGenesisDeclarations(appState map[string]json.RawMessage) error {
	// A profile that does not link x/validatorgov has no concentration
	// ceilings, so there is nothing for a declaration to be measured against
	// and demanding one would refuse a perfectly coherent genesis. Checked by
	// the section's presence rather than by a build tag, because this function
	// is also called from `genesis validate` on files built for other profiles.
	if _, ok := appState[validatorgovtypes.ModuleName]; !ok {
		return nil
	}

	declared, err := declaredOperators(appState)
	if err != nil {
		return err
	}

	seats, err := genesisValidatorOperators(appState)
	if err != nil {
		return err
	}

	var undeclared []string
	for _, operator := range seats {
		if _, ok := declared[operator]; !ok {
			undeclared = append(undeclared, operator)
		}
	}
	if len(undeclared) == 0 {
		return nil
	}
	sort.Strings(undeclared)

	return fmt.Errorf(
		"%s in this genesis %s no declaration, so %s outside every concentration ceiling: %s.\n"+
			"A validator with no legal entity, beneficial owner and jurisdiction is counted in the "+
			"power those ceilings are measured against and can never be demoted for exceeding one, "+
			"which exempts exactly the founding set the ceilings exist to bound.\n"+
			"Add an approved_validator_map entry with a complete declaration for each.",
		plural(len(undeclared), "validator", "validators"),
		plural(len(undeclared), "has", "have"),
		plural(len(undeclared), "it sits", "they sit"),
		strings.Join(undeclared, ", "))
}

// declaredOperators is every operator x/validatorgov's genesis approves.
//
// Read from raw JSON with BOTH key spellings rather than through the generated
// struct, and that is not defensive style. The protobuf JSON name is
// approved_validator_map and the Go struct tag is approvedValidatorMap; a real
// genesis on this chain uses the first, and unmarshalling it into the generated
// type yields an empty list with no error at all.
//
// A check that silently finds nothing to check is worse than no check: it
// reports every genesis as compliant, including the one that is not.
func declaredOperators(appState map[string]json.RawMessage) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	raw, ok := appState[validatorgovtypes.ModuleName]
	if !ok {
		// The module is not in this profile's genesis at all. Nothing to check
		// against, and refusing here would break every profile that does not
		// link it.
		return out, nil
	}
	var section map[string]json.RawMessage
	if err := json.Unmarshal(raw, &section); err != nil {
		return nil, fmt.Errorf("reading the %s genesis: %w", validatorgovtypes.ModuleName, err)
	}
	list, ok := firstPresent(section, "approved_validator_map", "approvedValidatorMap")
	if !ok {
		return out, nil
	}
	var approved []struct {
		Candidate string `json:"candidate"`
	}
	if err := json.Unmarshal(list, &approved); err != nil {
		return nil, fmt.Errorf("reading approved validators: %w", err)
	}
	for _, entry := range approved {
		// Presence only. Whether the declaration is COMPLETE is already
		// x/validatorgov's own Validate, and duplicating the rule here would
		// mean two places to change it.
		out[entry.Candidate] = struct{}{}
	}
	return out, nil
}

// firstPresent returns the first of several spellings a key may have.
func firstPresent(section map[string]json.RawMessage, names ...string) (json.RawMessage, bool) {
	for _, name := range names {
		if raw, ok := section[name]; ok {
			return raw, true
		}
	}
	return nil, false
}

// genesisValidatorOperators is every operator that will hold a seat at height
// one, from both places one can come from.
func genesisValidatorOperators(appState map[string]json.RawMessage) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	add := func(operator string) {
		if operator == "" {
			return
		}
		if _, ok := seen[operator]; ok {
			return
		}
		seen[operator] = struct{}{}
		out = append(out, operator)
	}

	// Validators written straight into the staking section.
	if raw, ok := appState[stakingtypes.ModuleName]; ok {
		var section map[string]json.RawMessage
		if err := json.Unmarshal(raw, &section); err != nil {
			return nil, fmt.Errorf("reading the staking genesis: %w", err)
		}
		if list, ok := firstPresent(section, "validators"); ok {
			var validators []struct {
				OperatorAddress string `json:"operator_address"`
			}
			if err := json.Unmarshal(list, &validators); err != nil {
				return nil, fmt.Errorf("reading the staking validators: %w", err)
			}
			for _, validator := range validators {
				add(validator.OperatorAddress)
			}
		}
	}

	// And the gentx path, which is how a founding set is actually assembled and
	// the reason this check exists. A gentx carries MsgCreateValidator, whose
	// validator_address is the operator; it is read out of the raw JSON rather
	// than through the tx decoder because this runs before an app exists to
	// supply one.
	if raw, ok := appState[genutiltypes.ModuleName]; ok {
		var section map[string]json.RawMessage
		if err := json.Unmarshal(raw, &section); err != nil {
			return nil, fmt.Errorf("reading the genutil genesis: %w", err)
		}
		// Both spellings again: the protobuf JSON name is gen_txs, the Go
		// struct tag is gentxs, and the genesis this chain actually runs on
		// uses gen_txs. See declaredOperators.
		var gentxs []json.RawMessage
		if list, ok := firstPresent(section, "gen_txs", "gentxs"); ok {
			if err := json.Unmarshal(list, &gentxs); err != nil {
				return nil, fmt.Errorf("reading the gentxs: %w", err)
			}
		}
		for _, gentx := range gentxs {
			operators, err := operatorsInGenTx(gentx)
			if err != nil {
				return nil, err
			}
			for _, operator := range operators {
				add(operator)
			}
		}
	}
	return out, nil
}

// operatorsInGenTx pulls every validator_address out of one gentx.
func operatorsInGenTx(gentx json.RawMessage) ([]string, error) {
	var tx struct {
		Body struct {
			Messages []map[string]any `json:"messages"`
		} `json:"body"`
	}
	if err := json.Unmarshal(gentx, &tx); err != nil {
		return nil, fmt.Errorf("reading a gentx: %w", err)
	}
	var out []string
	for _, msg := range tx.Body.Messages {
		if msg["@type"] != "/cosmos.staking.v1beta1.MsgCreateValidator" {
			continue
		}
		if operator, ok := msg["validator_address"].(string); ok {
			out = append(out, operator)
		}
	}
	return out, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
