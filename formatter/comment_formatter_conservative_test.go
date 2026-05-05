package formatter

import (
	"strings"
	"testing"

	"github.com/bhandras/llformat/internal/compat"
	"github.com/stretchr/testify/require"
)

func TestCommentFormatterOverflowModePreservesFittingLineCommentBlock(
	t *testing.T) {

	in := []byte(`package p

// Keep this deliberate short line break.
// It fits and should stay exactly as written.
func f() {}
`)

	f := compat.NewCommentFormatter(
		compat.CommentConfig{
			ColumnLimit: 80,
			Mode:        compat.CommentModeOverflow,
		},
	)
	out := f.FormatFile(in)

	require.Equal(t, string(in), string(out))
}

func TestCommentFormatterOverflowModeWrapsOverflowingProse(t *testing.T) {
	in := []byte(`package p

// This comment is plain prose that is intentionally long enough to exceed the
// configured column limit and should therefore be wrapped by overflow mode.
func f() {}
`)

	f := compat.NewCommentFormatter(
		compat.CommentConfig{
			ColumnLimit: 48,
			Mode:        compat.CommentModeOverflow,
		},
	)
	out := string(f.FormatFile(in))

	require.Contains(
		t, out, "// This comment is plain prose that is\n// "+
			"intentionally long enough to exceed the\n",
	)
	requireNoLineLongerThan(t, out, 48)
}

func TestCommentFormatterOverflowModePreservesPreformattedBlocks(t *testing.T) {
	in := []byte(`package p

// Name      Value      Meaning
// alpha     one        aligned table with a deliberately very long explanation
// beta      two        aligned table with another deliberately long explanation
func table() {}

// 1. first numbered step with a deliberately long explanation that should stay
//    attached to its author's manual wrapping
func numbered() {}

// ~~~
// command --with-a-deliberately-long-flag --and-a-deliberately-long-value
// ~~~
func fenced() {}
`)

	f := compat.NewCommentFormatter(
		compat.CommentConfig{
			ColumnLimit: 48,
			Mode:        compat.CommentModeOverflow,
		},
	)
	out := f.FormatFile(in)

	require.Equal(t, string(in), string(out))
}

func TestCommentFormatterOverflowModePreservesPunctuationRulers(t *testing.T) {
	in := []byte(`package p

type Example struct {
	// =============================================================================
	Field string
}
`)

	f := compat.NewCommentFormatter(
		compat.CommentConfig{
			ColumnLimit: 48,
			Mode:        compat.CommentModeOverflow,
		},
	)
	out := f.FormatFile(in)
	out2 := f.FormatFile(out)

	require.Equal(t, string(in), string(out))
	require.Equal(t, string(out), string(out2))
}

func TestCommentFormatterOverflowModePreservesGoExampleOutput(t *testing.T) {
	in := []byte(`package p

func ExampleWidget() {
	// Output:
	// first line stays separate even though this sentence is intentionally long
	// second line stays separate too
}
`)

	f := compat.NewCommentFormatter(
		compat.CommentConfig{
			ColumnLimit: 48,
			Mode:        compat.CommentModeOverflow,
		},
	)
	out := f.FormatFile(in)

	require.Equal(t, string(in), string(out))
}

func TestCommentFormatterProseModePreservesGoExampleOutput(t *testing.T) {
	in := []byte(`package p

func ExampleWidget() {
	// Unordered output:
	// alpha is intentionally first in this source fixture
	// beta remains on its own output line
}
`)

	f := compat.NewCommentFormatter(
		compat.CommentConfig{
			ColumnLimit: 80,
			Mode:        compat.CommentModeProse,
		},
	)
	out := f.FormatFile(in)

	require.Equal(t, string(in), string(out))
}

func TestCommentFormatterOverflowModeEmitsLongWordOnce(t *testing.T) {
	in := []byte(`package p

// SupercalifragilisticexpialidociousSupercalifragilisticexpialidocious
func f() {}
`)

	f := compat.NewCommentFormatter(
		compat.CommentConfig{
			ColumnLimit: 40,
			Mode:        compat.CommentModeOverflow,
		},
	)
	out := string(f.FormatFile(in))

	require.Equal(
		t, 1, strings.Count(
			out, "SupercalifragilisticexpialidociousSupercalifra"+
				"gilisticexpialidocious",
		),
	)
}

func TestCommentFormatterProseModeKeepsLegacyReflow(t *testing.T) {
	in := []byte(`package p

// Keep this deliberate short line break.
// Prose mode still normalizes fitting prose blocks.
func f() {}
`)

	f := compat.NewCommentFormatter(
		compat.CommentConfig{
			ColumnLimit: 80,
			Mode:        compat.CommentModeProse,
		},
	)
	out := string(f.FormatFile(in))

	require.Contains(
		t, out, "// Keep this deliberate short line break. Prose "+
			"mode still normalizes fitting\n",
	)
}

func TestCommentFormatterDoesNotReflowCommentsInsideRawString(t *testing.T) {
	in := []byte(`package p

const tmpl = ` + "`" + `// GeneratedThing documents an emitted type with a deliberately long line.
type GeneratedThing struct {}

// NewGeneratedThing constructs the emitted type with another deliberately long line.
func NewGeneratedThing() GeneratedThing {
	return GeneratedThing{}
}
` + "`" + `
`)

	f := compat.NewCommentFormatter(
		compat.CommentConfig{
			ColumnLimit: 48,
			Mode:        compat.CommentModeOverflow,
		},
	)
	out := f.FormatFile(in)

	require.Equal(t, string(in), string(out))
}
