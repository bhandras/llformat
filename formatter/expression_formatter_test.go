package formatter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatCompositeLiteralArg_OutdentsWhenIndentOverflow(t *testing.T) {
	formatGlobalsMu.Lock()
	oldColumnLimit := columnLimit
	oldTabStop := tabStop
	defer func() {
		columnLimit = oldColumnLimit
		tabStop = oldTabStop
		formatGlobalsMu.Unlock()
	}()

	columnLimit = 40
	tabStop = 8

	arg := "Item{Index: int64(1)}"
	contIndent := "\t\t\t"
	out, ok := FormatCompositeLiteralArg(arg, contIndent)
	require.True(t, ok)
	require.Contains(t, out, "\n			Index: "+
		"int64(1),\n			}")
	require.NotContains(t, out, "\n\t\t\t\tIndex:")

	for _, line := range strings.Split(out, "\n") {
		require.LessOrEqual(t, visualLen(line), columnLimit)
	}
}
