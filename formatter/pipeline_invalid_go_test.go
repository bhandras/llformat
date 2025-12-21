package formatter

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeline_DoesNotPanicOnInvalidGo(t *testing.T) {
	type pipeline struct {
		name string
		cfg  PipelineConfig
	}

	pipelines := []pipeline{
		{
			name: "next_with_ownership",
			cfg: PipelineConfig{
				UseOwnershipRegistry: true,
				ColumnLimit:          80,
				TabStop:              8,
			},
		},
	}

	type invalidCase struct {
		name string
		src  []byte
	}

	invalidSources := []invalidCase{
		{
			name: "two_packages",
			src: []byte(`package p
package q

func f() {}
`),
		},
		{
			name: "unterminated_func",
			src: []byte(`package p

func f() {
	_ = 1
`),
		},
		{
			name: "bad_tokens",
			src: []byte(
				`package p

func f() {
	_ = someVeryLongIdentifierNameThatIsVeryLong(,
}
`,
			),
		},
		{
			name: "multiple_decl_blocks",
			src: []byte(
				`package p

import "fmt"
import "strings"

func f() {
	fmt.Println(strings.TrimSpace("x"))
}
`,
			),
		},
		{
			name: "dangling_generic_bracket",
			src: []byte(`package p

func f() {
	_ = g[int(1)
}
`),
		},
		{
			name: "unterminated_block_comment",
			src: []byte(
				`package p

func f() {
	/* comment starts
	_ = 1 + 2
}
`,
			),
		},
		{
			name: "stray_block_comment_end",
			src: []byte(`package p

*/

func f() {}
`),
		},
		{
			name: "nested_block_comment_like",
			src: []byte(
				`package p

func f() {
	/* outer /* inner */ still outer?
	_ = 1
}
`,
			),
		},
		{
			name: "unterminated_raw_string",
			src: []byte(
				"package p\n\nfunc f() {\n	_ = " +
					"`unterminated\n}\n",
			),
		},
		{
			name: "unterminated_interpreted_string",
			src: []byte(
				"package p\n\nfunc f() {\n	_ = " +
					"\"unterminated\n	_ = 2\n}\n",
			),
		},
		{
			name: "unterminated_rune",
			src:  []byte("package p\n\nfunc f() {\n	_ = 'x\n}\n"),
		},
		{
			name: "bad_escape_sequence",
			src: []byte(
				"package p\n\nfunc f() {\n	_ = " +
					"\"\\xZZ\"\n}\n",
			),
		},
		{
			name: "mismatched_braces",
			src: []byte(`package p

func f() {
	if true {
		_ = 1
}
`),
		},
		{
			name: "dangling_select",
			src: []byte(
				`package p

func f() {
	select {
	case <-make(chan struct{}):
}
`,
			),
		},
		{
			name: "dangling_go_stmt",
			src: []byte(`package p

func f() {
	go
}
`),
		},
		{
			name: "invalid_utf8_bytes",
			src: append(
				[]byte("package p\n\nfunc f() { _ = \""), 0xff,
				0xfe, 0xfd, '\n', '}', '\n',
			),
		},
	}

	for pipelineIndex := range pipelines {
		pl := pipelines[pipelineIndex]
		t.Run(
			pl.name,
			func(t *testing.T) {
				p := NewPipeline(pl.cfg)

				for _, tc := range invalidSources {
					t.Run(
						tc.name,
						func(t *testing.T) {
							require.NotPanics(
								t,
								func() {
									out := p.Format(
										tc.src,
									)
									require.NotEmpty(
										t,
										out,
									)
									require.False(
										t,
										bytes.Contains(
											out,
											[]byte{
												0,
											},
										),
										"output contained NUL byte",
									)
								},
							)
						},
					)
				}
			},
		)
	}
}
