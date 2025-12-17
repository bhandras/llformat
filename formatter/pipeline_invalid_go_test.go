package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_DoesNotPanicOnInvalidGo(t *testing.T) {
	p := NewPipeline(PipelineConfig{
		ColumnLimit:          80,
		TabStop:              8,
		UseDSLMultiLineCalls: true,
		DSLMultiLineStyle:    "layout-args",
	})

	invalidSources := map[string]string{
		"two_packages": `package p
package q

func f() {}
`,
		"unterminated_func": `package p

func f() {
	_ = 1
`,
		"bad_tokens": `package p

func f() {
	_ = someVeryLongIdentifierNameThatIsVeryLong(,
}
`,
		"multiple_decl_blocks": `package p

import "fmt"
import "strings"

func f() {
	fmt.Println(strings.TrimSpace("x"))
}
`,
		"dangling_generic_bracket": `package p

func f() {
	_ = g[int(1)
}
`,
		"unterminated_block_comment": `package p

func f() {
	/* comment starts
	_ = 1 + 2
}
`,
	}

	for name, src := range invalidSources {
		t.Run(name, func(t *testing.T) {
			require.NotPanics(t, func() {
				out := p.Format([]byte(src))
				require.NotEmpty(t, out)
				require.NotContains(t, string(out), "\x00")
			})
		})
	}
}
