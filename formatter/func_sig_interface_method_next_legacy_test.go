package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFuncSigFormatterNext_LegacyInterfaceMethod_DoesNotPartiallyBreakShortReturnList(
	t *testing.T) {

	// This reproduces a class of issues where interface methods end up with
	// a partially broken parenthesized return list, sometimes leaving a
	// trailing comma exactly on the column boundary: M(a, b) ([]T, error)
	//
	// In the next profile we prefer to break params earlier and keep `([]T,
	// error)` inline.
	const in = `package p

import "context"

type Invoice struct{}

type I interface {
	InvoicesAddedSince(ctx context.Context, sinceAddIndex uint64) ([]Invoice,
		error)
}
`

	f := NewFuncSigFormatter(
		FuncSigConfig{
			ColumnLimit:                80,
			TabStop:                    8,
			CanonicalMultilineSigLists: true,
		},
	)

	out := string(f.FormatFile([]byte(in)))

	require.Contains(
		t, out, "InvoicesAddedSince(ctx "+
			"context.Context,\n		sinceAddIndex "+
			"uint64) ([]Invoice, error)", "expected params to "+
			"break before forcing a multiline return list",
	)
	require.NotContains(
		t, out, "([]Invoice,\n		error)", "should not "+
			"partially break short parenthesized return list in "+
			"next profile",
	)
}
