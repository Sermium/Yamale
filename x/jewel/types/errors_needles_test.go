package types_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The traceability app recognises the module's refusals by their text
// (clients/traceability/src/actions/errors.ts), because the client sees a code
// without its codespace. If a message here is reworded, the app would quietly
// fall back to the raw text; this fails first.
func TestTheTraceabilityAppsErrorNeedlesAreRegisteredTexts(t *testing.T) {
	ts, err := os.ReadFile("../../../clients/traceability/src/actions/errors.ts")
	require.NoError(t, err)
	src, err := os.ReadFile("errors.go")
	require.NoError(t, err)

	rule := regexp.MustCompile(`(?m)^\s*\[(?:'([^']*)'|"([^"]*)"),\s*'`)
	checked := 0
	for _, m := range rule.FindAllStringSubmatch(string(ts), -1) {
		needle := m[1] + m[2]
		// Rules from the first x/feegrant text onward belong to other modules.
		if strings.HasPrefix(needle, "fee-grant") {
			break
		}
		require.Contains(t, strings.ToLower(string(src)), strings.ToLower(needle),
			"errors.ts matches %q, which no x/jewel error says", needle)
		checked++
	}
	require.GreaterOrEqual(t, checked, 20, "the needle list was not read")
}
