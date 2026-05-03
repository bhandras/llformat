package formatter

import (
	"strings"
	"testing"

	"github.com/bhandras/llformat/width"
	"github.com/stretchr/testify/require"
)

func TestFuncSigFormatterNext_ReservesCommaWhenBreakingParams(t *testing.T) {
	// Regression: when we decide to break before the next parameter we
	// append a comma to the previous line. If width calculations don't
	// reserve space for that comma, the output can overflow by one column
	// (or land punctuation exactly on the column boundary).
	//
	// This is analogous to the comma-reserve bug we previously hit in call
	// argument packing.
	const (
		colLimit = 24
		tabStop  = 4
	)

	// Crafted so that: len("func f(" + "abcdefghijklm int") == colLimit but
	// `,` would overflow to colLimit+1 if appended later.
	const in = `package p

func f(abcdefghijklm int, b int) {}
`

	f := NewFuncSigFormatter(
		FuncSigConfig{
			ColumnLimit:                colLimit,
			TabStop:                    tabStop,
			CanonicalMultilineSigLists: true,
		},
	)

	out := string(f.FormatFile([]byte(in)))

	require.Contains(
		t, out, "func f(\n	abcdefghijklm int,\n	b int)", "ex"+
			"pected formatter to break before later comma "+
			"insertion would overflow",
	)

	lines := strings.Split(out, "\n")
	var sigLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "func f(") ||
			strings.HasPrefix(line, "\tabcdefghijklm int") || strings.HasPrefix(
			line, "	b int",
		) {

			sigLines = append(sigLines, line)
		}
	}
	require.NotEmpty(t, sigLines)

	for _, line := range sigLines {
		require.LessOrEqual(
			t, width.VisualLenWithTab(line, tabStop), colLimit,
			"signature line exceeds column limit: %q", line,
		)
	}
}
