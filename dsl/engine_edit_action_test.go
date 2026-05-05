package dsl

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/require"
)

type nodeTextIsCond struct {
	Want string
}

func (c nodeTextIsCond) Eval(caps Captures, ctx *Context) bool {
	n := resolveTarget(caps, "node")

	return string(ctx.NodeSource(n)) == c.Want
}

type replaceIdentEditAction struct {
	From string
	To   string
}

func (a *replaceIdentEditAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	return nil, false
}

func (a *replaceIdentEditAction) ExecuteEdits(caps Captures, ctx *Context) (
	[]Edit, bool, error) {

	n, ok := caps["node"].(*ast.Ident)
	if !ok || n == nil {
		return nil, false, nil
	}
	if n.Name != a.From {
		return nil, false, nil
	}

	start := ctx.Fset.Position(n.Pos()).Offset
	end := ctx.Fset.Position(n.End()).Offset

	return []Edit{
		{
			Start:   start,
			End:     end,
			Replace: []byte(a.To),
		},
	}, true, nil
}

type overlappingEditAction struct{}

func (a *overlappingEditAction) Execute(caps Captures, ctx *Context) ([]byte,
	bool) {

	return nil, false
}

func (a *overlappingEditAction) ExecuteEdits(caps Captures, ctx *Context) (
	[]Edit, bool, error) {

	n, ok := caps["node"].(*ast.Ident)
	if !ok || n == nil || n.Name != "foo" {
		return nil, false, nil
	}
	start := ctx.Fset.Position(n.Pos()).Offset
	end := ctx.Fset.Position(n.End()).Offset

	return []Edit{
		{
			Start:   start,
			End:     end,
			Replace: []byte("bar"),
		},
		{
			Start:   start,
			End:     end - 1,
			Replace: []byte("baz"),
		},
	}, true, nil
}

func TestEngineEditActionApplied(t *testing.T) {
	src := `package main

func foo() {
	foo()
}
`

	engine := NewEngine(
		[]Rule{
			{
				Name:     "replace_foo_with_bar",
				Pattern:  &NodePattern{Type: "Ident"},
				When:     nodeTextIsCond{Want: "foo"},
				Priority: 100,
				Action:   &replaceIdentEditAction{From: "foo", To: "bar"},
			},
		},
	)
	result, err := engine.Format([]byte(src))
	require.NoError(t, err)

	got := string(result)
	require.Contains(t, got, "func bar()")
	require.Contains(t, got, "\tbar()")
	require.NotContains(t, got, "foo")
}

func TestEngineEditActionOverlapIgnored(t *testing.T) {
	src := `package main

func foo() {
	foo()
}
`

	engine := NewEngine(
		[]Rule{
			{
				Name:     "overlap_edits",
				Pattern:  &NodePattern{Type: "Ident"},
				When:     nodeTextIsCond{Want: "foo"},
				Priority: 100,
				Action:   &overlappingEditAction{},
			},
		},
	)
	result, err := engine.Format([]byte(src))
	require.NoError(t, err)
	require.Equal(t, src, string(result))
}
