package dsl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type markerAction struct {
	marker string
}

func (a markerAction) Execute(_ Captures, ctx *Context) ([]byte, bool) {
	out := append([]byte(a.marker), ctx.Source...)

	return out, true
}

func TestIsParseableCondGatesFileFallbackRule(t *testing.T) {
	const marker = "/*FALLBACK_APPLIED*/\n"

	rules := []Rule{
		{
			Name: "fallback_only_on_parse_failure",
			Pattern: &NodePattern{
				Type: "File",
			},
			When: &IsParseableCond{
				Want: false,
			},
			Priority: 0,
			Action: markerAction{
				marker: marker,
			},
		},
	}

	e := NewEngine(rules)
	e.MaxIterations = 1

	t.Run(
		"parseable_does_not_apply",
		func(t *testing.T) {
			src := []byte("package p\n\nfunc f() {}\n")
			out, err := e.Format(src)
			require.NoError(t, err)
			require.Equal(t, string(src), string(out))
		},
	)

	t.Run(
		"unparseable_applies",
		func(t *testing.T) {
			src := []byte("package p\n\nfunc f(\n")
			out, err := e.Format(src)
			require.NoError(t, err)
			require.Contains(t, string(out), marker)
		},
	)
}
