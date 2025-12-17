package formatter

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactCallFormatter_ParseSafeDoesNotRewriteUnparseableSources(t *testing.T) {
	in := []byte("package p\n\nfunc f() { \n") // invalid Go: unterminated block

	f := NewCompactCallFormatter(Config{
		ColumnLimit: 40,
		TabStop:     8,
		SkipGofmt:   true,
		ParseSafe:   true,
		// Enable selection so the formatter attempts to do work even without
		// specific targets.
		FallbackNonTargets: true,
	})

	out := f.FormatFile(in)
	require.Equal(t, string(in), string(out))
}

func TestCompactCallFormatter_ParseSafeKeepsValidOutputParseable(t *testing.T) {
	in := []byte(`package p

func f() {
	log.Infof("prefix %s %s %s %s %s", a, b, c, d, e)
}
`)

	f := NewCompactCallFormatter(Config{
		ColumnLimit:        30,
		TabStop:            8,
		SkipGofmt:          true,
		ParseSafe:          true,
		FallbackNonTargets: true,
	})

	out := f.FormatFile(in)

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "out.go", out, parser.AllErrors)
	require.NoError(t, err)
}
