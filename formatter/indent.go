package formatter

import (
	"github.com/lightninglabs/llformat/text"
	"github.com/lightninglabs/llformat/width"
)

// Indent tracks indentation for a formatting context.
type Indent struct {
	Base    string // Leading whitespace of the line (spaces and tabs)
	TabStop int    // Tab width for visual width calculations
}

// IndentFromLine extracts the indentation from a line in the source.
// lineStart is the position of the first character of the line.
func IndentFromLine(src []byte, lineStart int, tabStop int) Indent {
	ws := text.LeadingWhitespace(src, lineStart)
	return Indent{
		Base:    string(ws),
		TabStop: tabStop,
	}
}

// IndentFromSource extracts the indentation for the line containing pos.
func IndentFromSource(src []byte, pos int, tabStop int) Indent {
	lineStart := text.LastLineStart(src, pos)
	return IndentFromLine(src, lineStart, tabStop)
}

// NewIndent creates an Indent with the given base whitespace.
func NewIndent(base string, tabStop int) Indent {
	return Indent{
		Base:    base,
		TabStop: tabStop,
	}
}

// Width returns the visual width of the base indentation.
func (i Indent) Width() int {
	return width.VisualLenWithTab(i.Base, i.TabStop)
}

// String returns the base indentation string.
func (i Indent) String() string {
	return i.Base
}

// Continuation returns a new Indent with one additional tab for continuation lines.
func (i Indent) Continuation() Indent {
	return Indent{
		Base:    i.Base + "\t",
		TabStop: i.TabStop,
	}
}

// WithExtra returns a new Indent with additional whitespace appended.
func (i Indent) WithExtra(extra string) Indent {
	return Indent{
		Base:    i.Base + extra,
		TabStop: i.TabStop,
	}
}

// WithSpaces returns a new Indent with n additional spaces.
func (i Indent) WithSpaces(n int) Indent {
	spaces := ""
	for j := 0; j < n; j++ {
		spaces += " "
	}
	return i.WithExtra(spaces)
}

// IsEmpty returns true if the indentation is empty.
func (i Indent) IsEmpty() bool {
	return len(i.Base) == 0
}

// Bytes returns the indentation as a byte slice.
func (i Indent) Bytes() []byte {
	return []byte(i.Base)
}

// FitsContent checks if content starting after this indentation fits within the column limit.
func (i Indent) FitsContent(content string, cfg BaseConfig) bool {
	return cfg.FitsInLimit(i.Width(), content)
}

// Column returns the column position after this indentation (same as Width).
func (i Indent) Column() int {
	return i.Width()
}
