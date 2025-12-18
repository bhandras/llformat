package layout

import (
	"strings"
)

type mode int

const (
	modeFlat mode = iota
	modeBreak
)

type frame struct {
	doc    Doc
	indent string
	mode   mode
}

// Render renders a document into a string with the given column limit.
// indent is the initial indentation for any broken lines.
func Render(doc Doc, colLimit int, tabStop int, indent string) string {
	return RenderAt(doc, colLimit, tabStop, indent, 0)
}

// RenderAt renders a document as if it started at startCol columns into the
// current line (used when replacing a sub-span of a line rather than the whole
// line).
//
// indent is the indentation for broken lines at the current nesting level (it
// should be a literal prefix written after newline, not a column count).
func RenderAt(doc Doc, colLimit int, tabStop int, indent string, startCol int) string {
	var out strings.Builder
	if startCol < 0 {
		startCol = 0
	}
	col := startCol

	stack := []frame{{doc: doc, indent: indent, mode: modeBreak}}

	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch d := f.doc.(type) {
		case Text:
			s := string(d)
			out.WriteString(s)
			col += visualWidth(s, tabStop, col)
		case Line:
			if f.mode == modeFlat {
				out.WriteByte(' ')
				col++
			} else {
				out.WriteByte('\n')
				out.WriteString(f.indent)
				col = visualWidth(f.indent, tabStop, 0)
			}
		case SoftLine:
			if f.mode == modeBreak {
				out.WriteByte('\n')
				out.WriteString(f.indent)
				col = visualWidth(f.indent, tabStop, 0)
			}
		case ForceBreak:
			// No output; acts only as a fit-check barrier.
		case Concat:
			// Push in reverse so we render in order.
			for i := len(d) - 1; i >= 0; i-- {
				stack = append(stack, frame{doc: d[i], indent: f.indent, mode: f.mode})
			}
		case Nest:
			stack = append(stack, frame{doc: d.Doc, indent: f.indent + d.Indent, mode: f.mode})
		case IfBreak:
			if f.mode == modeFlat {
				stack = append(stack, frame{doc: d.Flat, indent: f.indent, mode: f.mode})
			} else {
				stack = append(stack, frame{doc: d.Broken, indent: f.indent, mode: f.mode})
			}
		case Align:
			indent := f.indent
			if f.mode == modeBreak {
				indent = strings.Repeat(" ", col)
			}
			stack = append(stack, frame{doc: d.Doc, indent: indent, mode: f.mode})
		case IndentByCols:
			indent := f.indent
			if d.Cols > 0 {
				indent = indent + strings.Repeat(" ", d.Cols)
			}
			stack = append(stack, frame{doc: d.Doc, indent: indent, mode: f.mode})
		case Group:
			// Try flat: if it doesn't fit, render broken.
			if fitsAt(d.Doc, colLimit-col, tabStop, col) {
				stack = append(stack, frame{doc: d.Doc, indent: f.indent, mode: modeFlat})
			} else {
				stack = append(stack, frame{doc: d.Doc, indent: f.indent, mode: modeBreak})
			}
		default:
			// Unknown node: ignore.
		}
	}

	return out.String()
}

func fitsAt(doc Doc, remaining int, tabStop int, startCol int) bool {
	// Conservative fit check: any broken line forces failure.
	stack := []frame{{doc: doc, indent: "", mode: modeFlat}}
	if startCol < 0 {
		startCol = 0
	}
	col := startCol
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch d := f.doc.(type) {
		case Text:
			remaining -= visualWidth(string(d), tabStop, col)
			col += visualWidth(string(d), tabStop, col)
			if remaining < 0 {
				return false
			}
		case Line:
			// Flat line becomes a single space.
			remaining--
			col++
			if remaining < 0 {
				return false
			}
		case SoftLine:
			// Flat soft line is empty.
		case ForceBreak:
			return false
		case Concat:
			for i := len(d) - 1; i >= 0; i-- {
				stack = append(stack, frame{doc: d[i], indent: "", mode: modeFlat})
			}
		case Nest:
			stack = append(stack, frame{doc: d.Doc, indent: "", mode: modeFlat})
		case IfBreak:
			stack = append(stack, frame{doc: d.Flat, indent: "", mode: modeFlat})
		case Align:
			stack = append(stack, frame{doc: d.Doc, indent: "", mode: modeFlat})
		case IndentByCols:
			stack = append(stack, frame{doc: d.Doc, indent: "", mode: modeFlat})
		case Group:
			stack = append(stack, frame{doc: d.Doc, indent: "", mode: modeFlat})
		default:
			// Unknown node: ignore.
		}
	}
	return true
}

func visualWidth(s string, tabStop int, col int) int {
	w := 0
	for _, r := range s {
		if r == '\t' {
			advance := tabStop - ((col + w) % tabStop)
			w += advance
			continue
		}
		w++
	}
	return w
}
