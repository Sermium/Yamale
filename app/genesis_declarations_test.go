package app_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/app"
)

// A genesis whose validators are not all declared is refused.
//
// Raised by an independent review on 2026-08-31: x/validatorgov's own genesis
// validation refuses an INCOMPLETE declaration, but a validator admitted
// through gentx never appears in x/validatorgov's genesis at all — and that is
// exactly how a founding set is constituted. So the validators holding the most
// power on a new chain were the only ones the concentration ceilings could not
// reach.

func state(t *testing.T, sections map[string]any) map[string]json.RawMessage {
	t.Helper()
	out := map[string]json.RawMessage{}
	for name, body := range sections {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		out[name] = raw
	}
	return out
}

func gentxFor(operator string) map[string]any {
	return map[string]any{
		"body": map[string]any{
			"messages": []map[string]any{{
				"@type":             "/cosmos.staking.v1beta1.MsgCreateValidator",
				"validator_address": operator,
			}},
		},
	}
}

func TestAGentxValidatorMustBeDeclared(t *testing.T) {
	err := app.CheckGenesisDeclarations(state(t, map[string]any{
		"genutil": map[string]any{"gen_txs": []any{gentxFor("ymlvaloper1founder")}},
		"validatorgov": map[string]any{
			// Somebody else is declared, so the section exists and is not empty
			// — the failure must not depend on the module being absent.
			"approved_validator_map": []any{map[string]any{"candidate": "ymlvaloper1someoneelse"}},
		},
	}))
	require.Error(t, err, "a gentx validator with no declaration was accepted")
	require.Contains(t, err.Error(), "ymlvaloper1founder")
	require.Contains(t, err.Error(), "concentration")
}

func TestAStakingGenesisValidatorMustBeDeclared(t *testing.T) {
	err := app.CheckGenesisDeclarations(state(t, map[string]any{
		"staking": map[string]any{
			"validators": []any{map[string]any{"operator_address": "ymlvaloper1written"}},
			"params":     map[string]any{"bond_denom": "uyml"},
		},
		"validatorgov": map[string]any{"approved_validator_map": []any{}},
	}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "ymlvaloper1written")
}

func TestADeclaredValidatorPasses(t *testing.T) {
	require.NoError(t, app.CheckGenesisDeclarations(state(t, map[string]any{
		"genutil": map[string]any{"gen_txs": []any{gentxFor("ymlvaloper1founder")}},
		"validatorgov": map[string]any{
			"approved_validator_map": []any{map[string]any{"candidate": "ymlvaloper1founder"}},
		},
	})))
}

// Both founders named, so an operator reading the refusal knows how much work
// it is rather than fixing one and running it again.
func TestEveryUndeclaredValidatorIsNamedAtOnce(t *testing.T) {
	err := app.CheckGenesisDeclarations(state(t, map[string]any{
		"genutil": map[string]any{"gen_txs": []any{
			gentxFor("ymlvaloper1bbb"),
			gentxFor("ymlvaloper1aaa"),
		}},
		"validatorgov": map[string]any{"approved_validator_map": []any{}},
	}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "ymlvaloper1aaa")
	require.Contains(t, err.Error(), "ymlvaloper1bbb")
	// Sorted, because a genesis check whose message reorders between runs is
	// one nobody can diff.
	require.Less(t, indexOf(err.Error(), "aaa"), indexOf(err.Error(), "bbb"))
}

// A profile that does not link x/validatorgov has nothing to check against, and
// must not be refused for it.
func TestAProfileWithoutTheModuleIsNotRefused(t *testing.T) {
	require.NoError(t, app.CheckGenesisDeclarations(state(t, map[string]any{
		"genutil": map[string]any{"gen_txs": []any{gentxFor("ymlvaloper1founder")}},
	})))
}

// A genesis with no validators at all is a chain that cannot start, which is
// x/staking's complaint to make and not this one's.
func TestNoValidatorsIsNotThisChecksComplaint(t *testing.T) {
	require.NoError(t, app.CheckGenesisDeclarations(state(t, map[string]any{
		"validatorgov": map[string]any{"approved_validator_map": []any{}},
	})))
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// The spellings a real file actually uses.
//
// The generated Go tags say gentxs and approvedValidatorMap; the genesis this
// chain runs on says gen_txs and approved_validator_map. Unmarshalling the real
// file into the generated types yields empty lists with NO ERROR, so the first
// version of this check passed every genesis including the defective one.
//
// A check that silently finds nothing to check is worse than no check, so both
// spellings are pinned here rather than left to whichever the SDK happens to
// emit next.
func TestBothJSONSpellingsAreUnderstood(t *testing.T) {
	for _, spelling := range []struct {
		name, gentxKey, approvedKey string
	}{
		{"as the chain's own genesis writes them", "gen_txs", "approved_validator_map"},
		{"as the generated Go tags spell them", "gentxs", "approvedValidatorMap"},
	} {
		t.Run(spelling.name, func(t *testing.T) {
			err := app.CheckGenesisDeclarations(state(t, map[string]any{
				"genutil":      map[string]any{spelling.gentxKey: []any{gentxFor("ymlvaloper1undeclared")}},
				"validatorgov": map[string]any{spelling.approvedKey: []any{}},
			}))
			require.Error(t, err, "this spelling was not read, so the check passed silently")
			require.Contains(t, err.Error(), "ymlvaloper1undeclared")
		})
	}
}
