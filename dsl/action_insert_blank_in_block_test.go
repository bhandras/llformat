package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInsertBlankBeforeFirstStmtInBlockAction(t *testing.T) {
	t.Run(
		"if",
		func(t *testing.T) {
			const src = `package p

func f(a, b bool) {
	if a &&
		b {
		// leading comment
		x()
	}
}
`
			fset := token.NewFileSet()
			file, err := parser.ParseFile(
				fset, "", src, parser.ParseComments,
			)
			require.NoError(t, err)

			var ifStmt *ast.IfStmt
			ast.Inspect(
				file,
				func(n ast.Node) bool {
					if s, ok := n.(*ast.IfStmt); ok {
						ifStmt = s

						return false
					}

					return true
				},
			)
			require.NotNil(t, ifStmt)

			ctx := NewContext(fset, []byte(src), 80, 8)
			out, changed := (&InsertBlankBeforeFirstStmtInBlockAction{
				Target: "node",
			}).Execute(Captures{"node": ifStmt},
				ctx,
			)
			require.True(t, changed)
			require.Contains(
				t, string(out),
				"{\n\n		// leading "+
					"comment\n		x()\n",
			)
		},
	)

	t.Run(
		"for",
		func(t *testing.T) {
			const src = `package p

func f(items []int, stop bool) {
	for i := 0; i < len(items) &&
		!stop; i++ {
		x()
	}
}
`
			fset := token.NewFileSet()
			file, err := parser.ParseFile(
				fset, "", src, parser.ParseComments,
			)
			require.NoError(t, err)

			var forStmt *ast.ForStmt
			ast.Inspect(
				file,
				func(n ast.Node) bool {
					if s, ok := n.(*ast.ForStmt); ok {
						forStmt = s

						return false
					}

					return true
				},
			)
			require.NotNil(t, forStmt)

			ctx := NewContext(fset, []byte(src), 80, 8)
			out, changed := (&InsertBlankBeforeFirstStmtInBlockAction{
				Target: "node",
			}).Execute(Captures{"node": forStmt},
				ctx,
			)
			require.True(t, changed)
			require.Contains(t, string(out), "{\n\n\t\tx()\n")
		},
	)

	t.Run(
		"case",
		func(t *testing.T) {
			const src = `package p

func f(v int) {
	switch v {
	case 1, 2, 3,
		4:
		// leading comment
		x()
	}
}
`
			fset := token.NewFileSet()
			file, err := parser.ParseFile(
				fset, "", src, parser.ParseComments,
			)
			require.NoError(t, err)

			var cc *ast.CaseClause
			ast.Inspect(
				file,
				func(n ast.Node) bool {
					if s, ok := n.(*ast.CaseClause); ok {
						cc = s

						return false
					}

					return true
				},
			)
			require.NotNil(t, cc)

			ctx := NewContext(fset, []byte(src), 80, 8)
			out, changed := (&InsertBlankBeforeFirstStmtInBlockAction{
				Target: "node",
			}).Execute(Captures{"node": cc},
				ctx,
			)
			require.True(t, changed)
			require.Contains(
				t, string(out),
				"4:\n\n		// leading "+
					"comment\n		x()\n",
			)
		},
	)
}
