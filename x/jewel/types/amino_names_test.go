package types_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// A Ledger signs amino JSON and refuses a message whose amino name is longer
// than 39 characters. Read from the proto source, which is where the name is
// declared; proto-check guards the generated descriptor against drift from it.
func TestEveryAminoNameFitsALedger(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "proto", "blockchain", "jewel", "v1", "tx.proto"))
	require.NoError(t, err)
	names := regexp.MustCompile(`option \(amino\.name\) = "([^"]+)"`).FindAllStringSubmatch(string(src), -1)
	require.Len(t, names, 23, "every Msg in tx.proto carries an amino name; a count of zero would pass for the wrong reason")
	for _, m := range names {
		require.LessOrEqualf(t, len(m[1]), 39, "amino name %q is %d characters; a Ledger refuses more than 39", m[1], len(m[1]))
	}
}
