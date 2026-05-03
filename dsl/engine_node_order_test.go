package dsl

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

type markBinaryEndAction struct {
	marker string
}

func (a markBinaryEndAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	n, ok := caps["node"]
	if !ok || n == nil {
		return nil, false
	}

	start := ctx.Fset.Position(n.Pos()).Offset
	end := ctx.Fset.Position(n.End()).Offset
	if start < 0 || end < 0 || end > len(ctx.Source) || start >= end {
		return nil, false
	}

	// Insertion at end keeps the expression parseable.
	out, err := ApplySingleEdit(ctx.Source, end, end, []byte(a.marker))
	if err != nil {
		return nil, false
	}
	if bytes.Equal(out, ctx.Source) {
		return nil, false
	}

	return out, true
}

func TestEngine_NodeOrderAffectsWhichNestedNodeIsRewrittenFirst(t *testing.T) {
	t.Parallel()

	// Outer binary ends with ')', inner ends with 'c'. So the marker will
	// land in different places depending on whether we select outer-first
	// or deepest-first.
	src := []byte(
		"package p\n\nfunc f(a, b, c bool) bool { return a && (b " +
			"&& c) }\n",
	)

	rules := []Rule{
		{
			Name:     "mark_binary_end",
			Priority: 100,
			Pattern: &NodePattern{
				Type: "BinaryExpr",
				Fields: map[string]FieldMatch{
					"Op": {
						Literal: "&&",
					},
				},
			},
			When: TrueCond{},
			Action: markBinaryEndAction{
				marker: " /*MARK*/",
			},
		},
	}

	t.Run(
		"preorder_picks_outer",
		func(t *testing.T) {
			e := NewEngine(rules)
			e.MaxIterations = 1
			e.NodeOrder = NodeOrderPreorder

			out := e.FormatFile(src)
			require.Contains(t, string(out), ") /*MARK*/")
			require.NotContains(t, string(out), "c /*MARK*/")
		},
	)

	t.Run(
		"deepest_first_picks_inner",
		func(t *testing.T) {
			e := NewEngine(rules)
			e.MaxIterations = 1
			e.NodeOrder = NodeOrderDeepestFirst

			out := e.FormatFile(src)
			require.Contains(t, string(out), "c /*MARK*/")
			require.NotContains(t, string(out), ") /*MARK*/")
		},
	)
}
