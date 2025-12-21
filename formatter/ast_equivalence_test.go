package formatter

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

// canonicalASTDump parses src and returns a stable structural dump of the AST
// that ignores position/scoping information (and comments). This is a
// lightweight semantic regression guard: if formatting only changes whitespace
// and comment layout, the AST structure should remain the same.
func canonicalASTDump(src []byte) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(
		fset, "in.go", src, parser.AllErrors|parser.ParseComments,
	)
	if err != nil {
		return "", err
	}

	stripASTMetadata(file)

	var b bytes.Buffer
	// Re-render the AST into canonical source form (gofmt). This is stable
	// across position differences and provides a practical semantic
	// equivalence check for formatting changes.
	if err := format.Node(&b, token.NewFileSet(), file); err != nil {
		return "", err
	}

	return b.String(), nil
}

func stripASTMetadata(node ast.Node) {
	ast.Inspect(node, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.File:
			// Scope contains pointer-heavy symbol information and
			// can create cycles.
			v.Scope = nil
			// Unresolved identifiers are not semantically relevant
			// for formatting.
			v.Unresolved = nil
			// Ignore comment groups; comment formatting is
			// intentionally allowed to change while preserving code
			// semantics.
			v.Comments = nil

		case *ast.Ident:
			// Obj links can create cycles; ignore them for
			// structural equivalence.
			v.Obj = nil

		case *ast.GenDecl:
			v.Doc = nil

		case *ast.FuncDecl:
			v.Doc = nil

		case *ast.Field:
			v.Doc = nil
			v.Comment = nil

		case *ast.ImportSpec:
			v.Doc = nil
			v.Comment = nil

		case *ast.TypeSpec:
			v.Doc = nil
			v.Comment = nil

		case *ast.ValueSpec:
			v.Doc = nil
			v.Comment = nil
		}

		return true
	})
}

func requireASTEquivalent(t *testing.T, before, after []byte) {
	t.Helper()

	a, err := canonicalASTDump(before)
	require.NoError(t, err)
	b, err := canonicalASTDump(after)
	require.NoError(t, err)
	require.Equal(t, a, b)
}
