package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiLineCallFormatter_ParseSafeDoesNotRewriteUnparseableSources(t *testing.T) {
	in := []byte("package p\n\nfunc f() {\n\t_ = x.(T\n}\n") // invalid Go: broken type assertion

	f := NewMultiLineCallFormatter(MultiLineConfig{
		ColumnLimit:  40,
		TabStop:      8,
		SkipGofmt:    true,
		ParseSafe:    true,
		Excludes:     nil,
		UseASTSelection: false,
	})

	out := f.FormatFile(in)
	require.Equal(t, string(in), string(out))
}

func TestMultiLineCallFormatter_ParseSafeKeepsValidOutputParseable(t *testing.T) {
	in := []byte(`package p

func f() {
	_ = outerFunctionNameThatIsVeryLong(innerFunctionNameThatIsVeryLong(1, 2, 3, 4, 5, 6, 7, 8, 9))
}
`)

	f := NewMultiLineCallFormatter(MultiLineConfig{
		ColumnLimit:     40,
		TabStop:         8,
		SkipGofmt:       true,
		ParseSafe:       true,
		UseASTSelection: true,
	})

	out := f.FormatFile(in)
	requireASTEquivalent(t, in, out)
}

